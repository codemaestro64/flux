package server

import (
	"context"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/hashicorp/raft"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/codemaestro64/flux/internal/gen/v1"
	"github.com/codemaestro64/flux/internal/types"
)

// Global metrics for cluster-wide observability
var (
	allowCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ratelimit_allow_requests_total",
			Help: "Total number of allow requests",
		},
		[]string{"key", "allowed"},
	)

	allowLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ratelimit_allow_latency_seconds",
			Help:    "Latency of allow requests",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"key"},
	)

	batchAllowCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ratelimit_batch_allow_requests_total",
			Help: "Total batch allow requests",
		},
		[]string{"batch_size"},
	)

	raftLeaderGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "raft_is_leader",
		Help: "1 if this node is Raft leader",
	})
)

func init() {
	prometheus.MustRegister(allowCounter, allowLatency, batchAllowCounter, raftLeaderGauge)
}

// RateLimiterServer handles incoming gRPC requests and interfaces with the Raft node
type RateLimiterServer struct {
	pb.UnimplementedRateLimiterServer
	Node *RaftNode
}

// Allow processes a single limit check. Only the leader can accept writes to the replicated log
func (s *RateLimiterServer) Allow(ctx context.Context, req *pb.AllowRequest) (*pb.AllowResponse, error) {
	// Only the leader can process state-changing or consistent read commands
	if s.Node.Raft.State() != raft.Leader {
		return nil, status.Errorf(codes.Unavailable, "not leader, current leader is: %s", s.Node.Leader())
	}

	cmd := types.Command{
		Type:      types.CmdAllow,
		Key:       req.Key,
		Tokens:    req.Tokens,
		Timestamp: time.Now().UnixNano(),
	}

	// Propose the command to the Raft cluster
	res, err := s.Node.Apply(cmd, 5*time.Second)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "apply failed: %v", err)
	}

	// Assert the response from the FSM Apply method
	resp, ok := res.(*pb.AllowResponse)
	if !ok {
		return nil, status.Error(codes.Internal, "invalid response type from FSM")
	}

	return resp, nil
}

func (s *RateLimiterServer) SetLimit(ctx context.Context, req *pb.SetLimitRequest) (*pb.SetLimitResponse, error) {
	if s.Node.Raft.State() != raft.Leader {
		return nil, status.Errorf(codes.Unavailable, "not leader")
	}

	cmd := types.Command{
		Type:      types.CmdSetLimit,
		Key:       req.Key,
		Rate:      req.Rate,
		Burst:     req.Burst,
		Timestamp: time.Now().UnixNano(),
	}

	_, err := s.Node.Apply(cmd, 5*time.Second)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "apply failed: %v", err)
	}

	return &pb.SetLimitResponse{Success: true}, nil
}

// BatchAllow processes a bidirectional stream of requests.
func (s *RateLimiterServer) BatchAllow(stream pb.RateLimiter_BatchAllowServer) error {
	if s.Node.Raft.State() != raft.Leader {
		return status.Error(codes.Unavailable, "not leader")
	}

	var streamMu sync.Mutex
	var wg sync.WaitGroup

	// Track the first error encountered to terminate the stream
	var once sync.Once
	var firstErr error
	setErr := func(err error) {
		once.Do(func() { firstErr = err })
	}

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		wg.Add(1)
		go func(r *pb.AllowRequest) {
			defer wg.Done()

			cmd := types.Command{
				Type:      types.CmdAllow,
				Key:       r.Key,
				Tokens:    r.Tokens,
				Timestamp: time.Now().UnixNano(),
			}

			respI, applyErr := s.Node.Apply(cmd, 10*time.Second)
			if applyErr != nil {
				setErr(applyErr)
				return
			}

			resp := respI.(*pb.AllowResponse)

			streamMu.Lock()
			sendErr := stream.Send(resp)
			streamMu.Unlock()

			if sendErr != nil {
				setErr(sendErr)
			}
		}(req)
	}

	wg.Wait()
	return firstErr
}

func (s *RateLimiterServer) Join(ctx context.Context, req *pb.JoinRequest) (*pb.JoinResponse, error) {
	if s.Node.Raft.State() != raft.Leader {
		return nil, status.Errorf(codes.Unavailable, "not leader")
	}

	if err := s.Node.JoinCluster(req.NodeId, req.Address); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to join cluster: %v", err)
	}

	return &pb.JoinResponse{Success: true}, nil
}

// StartMetricsServer initializes the Prometheus scrape endpoint
func StartMetricsServer(addr string) {
	http.Handle("/metrics", promhttp.Handler())
	go http.ListenAndServe(addr, nil)
}
