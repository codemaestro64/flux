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

// Global metrics for cluster-wide observability.
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

// RateLimiterServer handles incoming gRPC requests and interfaces with the Raft node.
type RateLimiterServer struct {
	pb.UnimplementedRateLimiterServer
	Node *RaftNode
}

// Allow processes a single limit check. Only the leader can accept writes to the replicated log.
func (s *RateLimiterServer) Allow(ctx context.Context, req *pb.AllowRequest) (*pb.AllowResponse, error) {
	// Verify leadership before attempting to propose a log entry.
	if s.Node.Raft.State() != raft.Leader {
		return nil, status.Error(codes.Unavailable, "not leader")
	}

	start := time.Now()

	// Create the command with a leader-generated timestamp to ensure
	// deterministic state updates across all cluster replicas.
	cmd := types.Command{
		Type:      types.CmdAllow,
		Key:       req.Key,
		Tokens:    req.Tokens,
		Timestamp: time.Now().UnixNano(),
	}

	// Commit the command to the Raft log and wait for quorum acknowledgment.
	respI, err := s.Node.Apply(cmd, 10*time.Second)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "apply failed: %v", err)
	}

	resp := respI.(*pb.AllowResponse)

	// Update metrics based on the FSM decision.
	allowedStr := "false"
	if resp.Allowed {
		allowedStr = "true"
	}
	allowCounter.WithLabelValues(req.Key, allowedStr).Inc()
	allowLatency.WithLabelValues(req.Key).Observe(time.Since(start).Seconds())

	return resp, nil
}

// BatchAllow processes a bidirectional stream of requests.
func (s *RateLimiterServer) BatchAllow(stream pb.RateLimiter_BatchAllowServer) error {
	if s.Node.Raft.State() != raft.Leader {
		return status.Error(codes.Unavailable, "not leader")
	}

	// streamMu is required because gRPC streams do not support concurrent Send calls.
	var streamMu sync.Mutex
	var wg sync.WaitGroup

	// Track the first error encountered to terminate the stream.
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

		// Process each request in the stream concurrently to leverage Raft's pipeline.
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

// StartMetricsServer initializes the Prometheus scrape endpoint.
func StartMetricsServer(addr string) {
	http.Handle("/metrics", promhttp.Handler())
	go http.ListenAndServe(addr, nil)
}
