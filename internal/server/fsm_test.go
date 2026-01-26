package server

import (
	"bytes"
	"io"
	"testing"
	"time"

	pb "github.com/codemaestro64/flux/internal/gen/v1"
	"github.com/codemaestro64/flux/internal/types"
	"github.com/hashicorp/raft"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRateLimiterFSM_Apply(t *testing.T) {
	fsm := NewFSM()
	key := "test-key"
	ts := time.Now().UnixNano()

	// Test CmdSetLimit
	setCmd := types.Command{
		Type:      types.CmdSetLimit,
		Key:       key,
		Rate:      100.0,
		Burst:     50,
		Timestamp: ts,
	}
	setBytes, _ := setCmd.Marshal()
	fsm.Apply(&raft.Log{Data: setBytes})

	fsm.mu.RLock()
	limiter, exists := fsm.limiter[key]
	fsm.mu.RUnlock()
	require.True(t, exists)
	assert.Equal(t, 50, limiter.Burst())

	// Test CmdAllow
	allowCmd := types.Command{
		Type:      types.CmdAllow,
		Key:       key,
		Tokens:    10,
		Timestamp: ts,
	}
	allowBytes, _ := allowCmd.Marshal()
	res := fsm.Apply(&raft.Log{Data: allowBytes}).(*pb.AllowResponse)

	assert.True(t, res.Allowed)
	assert.Equal(t, float64(40), res.TokensRemaining)
}

func TestRateLimiterFSM_SnapshotRestore(t *testing.T) {
	fsm := NewFSM()
	key := "snap-key"
	ts := time.Now().UnixNano()

	// Set initial state
	setCmd := types.Command{Type: types.CmdSetLimit, Key: key, Rate: 10, Burst: 10, Timestamp: ts}
	setBytes, _ := setCmd.Marshal()
	fsm.Apply(&raft.Log{Data: setBytes})

	// Create Snapshot
	snap, err := fsm.Snapshot()
	require.NoError(t, err)

	// Persist to buffer
	buf := new(bytes.Buffer)
	sink := &testSnapshotSink{internal: buf}
	err = snap.Persist(sink)
	require.NoError(t, err)

	// Restore into a fresh FSM
	newFSM := NewFSM()
	err = newFSM.Restore(io.NopCloser(buf))
	require.NoError(t, err)

	// Verify restored state
	newFSM.mu.RLock()
	lim, exists := newFSM.limiter[key]
	newFSM.mu.RUnlock()
	assert.True(t, exists)
	assert.Equal(t, 10, lim.Burst())
}

type testSnapshotSink struct {
	internal *bytes.Buffer
}

func (s *testSnapshotSink) Write(p []byte) (n int, err error) { return s.internal.Write(p) }
func (s *testSnapshotSink) Close() error                      { return nil }
func (s *testSnapshotSink) ID() string                        { return "test" }
func (s *testSnapshotSink) Cancel() error                     { return nil }
