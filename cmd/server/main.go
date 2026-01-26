package main

import (
	"context"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	pb "github.com/codemaestro64/flux/internal/gen/v1"
	"github.com/codemaestro64/flux/internal/server"
)

var (
	nodeID      = flag.String("id", "node1", "Node ID")
	raftAddr    = flag.String("raft", "127.0.0.1:50051", "Raft bind address")
	grpcAddr    = flag.String("grpc", "127.0.0.1:50052", "gRPC listen address")
	metricsAddr = flag.String("metrics", "127.0.0.1:9090", "Prometheus metrics address")
	dataDir     = flag.String("data", "./data", "Data directory")
	bootstrap   = flag.Bool("bootstrap", false, "Bootstrap new cluster")
)

func main() {
	flag.Parse()

	raftDir := filepath.Join(*dataDir, *nodeID)
	if err := os.MkdirAll(raftDir, 0755); err != nil {
		log.Fatal(err)
	}

	node, err := server.NewRaftNode(*nodeID, *raftAddr, *grpcAddr, raftDir, *bootstrap)
	if err != nil {
		log.Fatalf("failed to create raft node: %v", err)
	}

	lis, err := net.Listen("tcp", *grpcAddr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterRateLimiterServer(grpcServer, &server.RateLimiterServer{Node: node})
	reflection.Register(grpcServer)

	// Metrics server
	go server.StartMetricsServer(*metricsAddr)

	// Graceful shutdown
	_, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down...")
		cancel()

		grpcServer.GracefulStop()

		future := node.Raft.Shutdown()
		if err := future.Error(); err != nil {
			log.Printf("Raft shutdown error: %v", err)
		}

		log.Println("Shutdown complete")
		os.Exit(0)
	}()

	log.Printf("Starting gRPC server on %s (Raft: %s)", *grpcAddr, *raftAddr)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("gRPC serve failed: %v", err)
	}
}
