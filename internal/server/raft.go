package server

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/codemaestro64/flux/internal/types"
	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb/v2"
)

// RaftNode wraps the Raft instance and its associated storage/network layers
type RaftNode struct {
	Raft          *raft.Raft
	FSM           *RateLimiterFSM
	raftDir       string
	raftBind      string
	grpcAddr      string
	snapshotStore raft.SnapshotStore
}

// NewRaftNode initializes the Raft storage, transport, and state machine
func NewRaftNode(nodeID, raftBind, grpcAddr, raftDir string, bootstrap bool) (*RaftNode, error) {
	fsm := NewFSM()

	// Ensure the data directory exists for BoltDB and snapshot storage
	if err := os.MkdirAll(raftDir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir all: %w", err)
	}

	// BoltDB used for the log store replicated commands, and stable store cluster metadata
	logStore, err := raftboltdb.NewBoltStore(filepath.Join(raftDir, "raft-log.db"))
	if err != nil {
		return nil, fmt.Errorf("log store: %w", err)
	}
	stableStore, err := raftboltdb.NewBoltStore(filepath.Join(raftDir, "raft-stable.db"))
	if err != nil {
		return nil, fmt.Errorf("stable store: %w", err)
	}

	// Snapshot store prevents the Raft log from growing indefinitely
	// Retain the last 2 snapshots to allow for safe recovery
	snapshotStore, err := raft.NewFileSnapshotStore(raftDir, 2, os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("snapshot store: %w", err)
	}

	addr, err := net.ResolveTCPAddr("tcp", raftBind)
	if err != nil {
		return nil, err
	}
	transport, err := raft.NewTCPTransport(raftBind, addr, 3, 10*time.Second, os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("transport: %w", err)
	}

	config := raft.DefaultConfig()
	config.LocalID = raft.ServerID(nodeID)

	// Create the Raft instance
	r, err := raft.NewRaft(config, fsm, logStore, stableStore, snapshotStore, transport)
	if err != nil {
		return nil, fmt.Errorf("new raft: %w", err)
	}

	// Only bootstrap if this is the first node in a new cluster
	if bootstrap {
		configuration := raft.Configuration{
			Servers: []raft.Server{{
				ID:      config.LocalID,
				Address: transport.LocalAddr(),
			}},
		}
		r.BootstrapCluster(configuration)
	}

	return &RaftNode{
		Raft:          r,
		FSM:           fsm,
		raftDir:       raftDir,
		raftBind:      raftBind,
		grpcAddr:      grpcAddr,
		snapshotStore: snapshotStore,
	}, nil
}

// JoinCluster adds a new server to the existing cluster configuration
// This must be called on the Leader node
func (n *RaftNode) JoinCluster(nodeID, addr string) error {
	if n.Raft.State() != raft.Leader {
		return fmt.Errorf("not leader")
	}

	configFuture := n.Raft.GetConfiguration()
	if err := configFuture.Error(); err != nil {
		return fmt.Errorf("failed to get configuration: %v", err)
	}

	// Check if the node is already a member to avoid unnecessary re-configurations
	for _, srv := range configFuture.Configuration().Servers {
		if srv.ID == raft.ServerID(nodeID) || srv.Address == raft.ServerAddress(addr) {
			if srv.ID == raft.ServerID(nodeID) && srv.Address == raft.ServerAddress(addr) {
				return nil // Node already exists
			}
			// ID or address mismatch, remove old entry before re-adding.
			future := n.Raft.RemoveServer(srv.ID, 0, 0)
			if err := future.Error(); err != nil {
				return fmt.Errorf("error removing existing node %s: %v", nodeID, err)
			}
		}
	}

	// Add the new voter to the cluster
	addFuture := n.Raft.AddVoter(raft.ServerID(nodeID), raft.ServerAddress(addr), 0, 0)
	return addFuture.Error()
}

// Apply submits a command to the Raft log for replication and FSM application
func (n *RaftNode) Apply(cmd types.Command, timeout time.Duration) (interface{}, error) {
	data, err := cmd.Marshal()
	if err != nil {
		return nil, err
	}

	f := n.Raft.Apply(data, timeout)
	if err := f.Error(); err != nil {
		return nil, err
	}

	return f.Response(), nil
}

// Leader returns the network address of the current cluster leader
func (n *RaftNode) Leader() string {
	return string(n.Raft.Leader())
}
