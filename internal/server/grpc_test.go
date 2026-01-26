package server

import (
	"context"
	"os"
	"testing"
	"time"

	pb "github.com/codemaestro64/flux/internal/gen/v1"
	"github.com/hashicorp/raft"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRateLimiterServer_Handlers(t *testing.T) {
	raftDir, err := os.MkdirTemp("", "flux-grpc-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(raftDir)

	// Node-1: Use port 0 for automatic assignment
	node, err := NewRaftNode("node-1", "127.0.0.1:0", "127.0.0.1:0", raftDir, true)
	require.NoError(t, err)
	defer node.Raft.Shutdown()

	srv := &RateLimiterServer{Node: node}

	// Wait for Leader election
	require.Eventually(t, func() bool {
		return node.Raft.State() == raft.Leader
	}, 10*time.Second, 100*time.Millisecond, "Node failed to become leader")

	ctx := context.Background()

	t.Run("SetLimit Handler", func(t *testing.T) {
		req := &pb.SetLimitRequest{Key: "api_key", Rate: 100, Burst: 100}
		resp, err := srv.SetLimit(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.True(t, resp.Success)
	})

	t.Run("Allow Handler", func(t *testing.T) {
		req := &pb.AllowRequest{Key: "api_key", Tokens: 1}
		resp, err := srv.Allow(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.True(t, resp.Allowed)
		assert.Equal(t, float64(99), resp.TokensRemaining)
	})
}
