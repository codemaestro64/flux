package server

import (
	"os"
	"testing"
	"time"

	pb "github.com/codemaestro64/flux/internal/gen/v1"
	"github.com/codemaestro64/flux/internal/types"
	"github.com/hashicorp/raft"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRaftNode_Lifecycle(t *testing.T) {
	raftDir, _ := os.MkdirTemp("", "flux-raft-test-*")
	defer os.RemoveAll(raftDir)

	// Test NewRaftNode and Bootstrap
	node, err := NewRaftNode("node-1", "127.0.0.1:7001", "127.0.0.1:7002", raftDir, true)
	require.NoError(t, err)
	defer node.Raft.Shutdown()

	// Wait for leader election
	require.Eventually(t, func() bool {
		return node.Raft.State() == raft.Leader
	}, 10*time.Second, 100*time.Millisecond)

	// Test Apply method
	cmd := types.Command{
		Type:      types.CmdSetLimit,
		Key:       "global",
		Rate:      10,
		Burst:     10,
		Timestamp: time.Now().UnixNano(),
	}

	resp, err := node.Apply(cmd, time.Second)
	assert.NoError(t, err)

	res, ok := resp.(*pb.SetLimitResponse)
	require.True(t, ok)
	assert.True(t, res.Success)

	// Test Leader string return
	leaderAddr := node.Leader()
	assert.NotEmpty(t, leaderAddr)
}
