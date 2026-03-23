package server

import (
	"encoding/gob"
	"io"
	"sync"
	"time"

	"github.com/hashicorp/raft"
	"golang.org/x/time/rate"

	pb "github.com/codemaestro64/flux/internal/gen/v1"
	"github.com/codemaestro64/flux/internal/types"
)

// RateLimiterFSM implements the raft.FSM interface
// It bridges the replicated log to the local token bucket state
type RateLimiterFSM struct {
	mu      sync.RWMutex
	limiter map[string]*rate.Limiter
}

// NewFSM initializes an empty state machine
func NewFSM() *RateLimiterFSM {
	return &RateLimiterFSM{
		limiter: make(map[string]*rate.Limiter),
	}
}

// Apply is called by Raft after a log entry is committed by a quorum
// It must be deterministic; all nodes must arrive at the exact same state
func (f *RateLimiterFSM) Apply(log *raft.Log) interface{} {
	cmd, err := types.UnmarshalCommand(log.Data)
	if err != nil {
		return err
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	// Use the leader-provided timestamp for all time-based calculations
	// This ensures that token refills are calculated identically across the cluster
	applyTime := time.Unix(0, cmd.Timestamp)

	switch cmd.Type {
	case types.CmdAllow:
		lim, exists := f.limiter[cmd.Key]
		if !exists {
			// Initialize with default conservative limits if not explicitly set
			lim = rate.NewLimiter(1, 1)
			f.limiter[cmd.Key] = lim
		}

		// AllowN consumes tokens atomically against the provided logical time
		allowed := lim.AllowN(applyTime, int(cmd.Tokens))

		return &pb.AllowResponse{
			Key:             cmd.Key,
			Allowed:         allowed,
			TokensRemaining: lim.TokensAt(applyTime),
		}

	case types.CmdSetLimit:
		// Upsert logic for updating bucket configuration (TPS/Burst)
		lim := rate.NewLimiter(rate.Limit(cmd.Rate), int(cmd.Burst))
		f.limiter[cmd.Key] = lim
		return &pb.SetLimitResponse{Success: true}
	}
	return nil
}

// fsmSnapshot represents a point-in-time state of all rate limiters for persistence
type fsmSnapshot struct {
	Data map[string]struct {
		Rate   float64
		Burst  int
		Tokens float64
		Last   time.Time
	}
}

// Snapshot is called by Raft to compact the log or prepare for state transfer
func (f *RateLimiterFSM) Snapshot() (raft.FSMSnapshot, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	snap := fsmSnapshot{Data: make(map[string]struct {
		Rate   float64
		Burst  int
		Tokens float64
		Last   time.Time
	})}

	// Capture the current bucket levels and configurations
	now := time.Now()
	for k, lim := range f.limiter {
		snap.Data[k] = struct {
			Rate   float64
			Burst  int
			Tokens float64
			Last   time.Time
		}{
			Rate:   float64(lim.Limit()),
			Burst:  lim.Burst(),
			Tokens: lim.TokensAt(now),
			Last:   now,
		}
	}

	return &fsmSnapshotImpl{data: snap}, nil
}

// fsmSnapshotImpl handles the serialization of the captured state
type fsmSnapshotImpl struct{ data fsmSnapshot }

// Persist writes the snapshot to the given sink
func (s *fsmSnapshotImpl) Persist(sink raft.SnapshotSink) error {
	err := func() error {
		if err := gob.NewEncoder(sink).Encode(s.data); err != nil {
			return err
		}
		return sink.Close()
	}()

	if err != nil {
		sink.Cancel()
	}
	return err
}

// Release is a no-op
func (s *fsmSnapshotImpl) Release() {}

// Restore replaces the entire local state with a snapshot from the leader or stable storage
func (f *RateLimiterFSM) Restore(rc io.ReadCloser) error {
	defer rc.Close()

	var snap fsmSnapshot
	if err := gob.NewDecoder(rc).Decode(&snap); err != nil {
		return err
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	// Reset and re-initialize map from decoded data
	f.limiter = make(map[string]*rate.Limiter)
	for k, v := range snap.Data {
		lim := rate.NewLimiter(rate.Limit(v.Rate), v.Burst)
		// Drain limiter to restore saved token level
		tokensToConsume := v.Burst - int(v.Tokens)
		if tokensToConsume > 0 {
			lim.AllowN(v.Last, tokensToConsume)
		}
		f.limiter[k] = lim
	}
	return nil
}
