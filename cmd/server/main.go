package main

import (
	"context"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/codemaestro64/flux/internal/gen/v1"
	"github.com/codemaestro64/flux/internal/server"
)

func main() {
	nodeID := flag.String("id", "", "Node ID")
	raftAddr := flag.String("raft-addr", "", "Raft listen address")
	grpcAddr := flag.String("grpc-addr", "0.0.0.0:50051", "gRPC listen address")
	dataDir := flag.String("data-dir", "data", "Raft data directory")
	bootstrap := flag.Bool("bootstrap", false, "Bootstrap cluster")

	join := flag.Bool("join", false, "Join an existing cluster")
	leaderAddr := flag.String("leader-addr", "", "Leader gRPC address for joining")

	flag.Parse()

	if *join {
		clientJoin(*nodeID, *raftAddr, *leaderAddr)
		return
	}

	node, err := server.NewRaftNode(*nodeID, *raftAddr, *grpcAddr, *dataDir, *bootstrap)
	if err != nil {
		log.Fatalf("failed to create raft node: %v", err)
	}

	lis, err := net.Listen("tcp", *grpcAddr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	s := grpc.NewServer()
	pb.RegisterRateLimiterServer(s, &server.RateLimiterServer{Node: node})

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		<-sigCh
		log.Println("Shutting down...")
		s.GracefulStop()
		node.Raft.Shutdown()
		os.Exit(0)
	}()

	log.Printf("Starting node %s (gRPC: %s, Raft: %s)", *nodeID, *grpcAddr, *raftAddr)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

func clientJoin(nodeID, raftAddr, leaderAddr string) {
	conn, err := grpc.Dial(leaderAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()

	client := pb.NewRateLimiterClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err = client.Join(ctx, &pb.JoinRequest{NodeId: nodeID, Address: raftAddr})
	if err != nil {
		log.Fatalf("could not join cluster: %v", err)
	}
	log.Printf("Successfully joined node %s at %s to cluster via leader %s", nodeID, raftAddr, leaderAddr)
}
