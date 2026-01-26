package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"sync"
	"time"

	pb "github.com/codemaestro64/flux/internal/gen/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// SmartClient with leader discovery and forwarding
type SmartClient struct {
	conns     map[string]*grpc.ClientConn
	clients   map[string]pb.RateLimiterClient
	endpoints []string
	mu        sync.Mutex
}

func NewSmartClient(endpoints []string) *SmartClient {
	c := &SmartClient{
		conns:     make(map[string]*grpc.ClientConn),
		clients:   make(map[string]pb.RateLimiterClient),
		endpoints: endpoints,
	}
	for _, ep := range endpoints {
		conn, err := grpc.Dial(ep, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Printf("Failed to dial %s: %v", ep, err)
			continue
		}
		c.conns[ep] = conn
		c.clients[ep] = pb.NewRateLimiterClient(conn)
	}
	return c
}

func (c *SmartClient) getClient() (pb.RateLimiterClient, string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Try random endpoint first
	idx := rand.Intn(len(c.endpoints))
	ep := c.endpoints[idx]
	if client, ok := c.clients[ep]; ok {
		return client, ep
	}
	return nil, ""
}

func (c *SmartClient) Allow(ctx context.Context, key string, tokens uint32) (*pb.AllowResponse, error) {
	for attempt := 0; attempt < 5; attempt++ {
		client, usedEp := c.getClient()
		if client == nil {
			return nil, fmt.Errorf("no available endpoints")
		}

		resp, err := client.Allow(ctx, &pb.AllowRequest{Key: key, Tokens: tokens})
		if err == nil {
			return resp, nil
		}

		if strings.Contains(err.Error(), "not leader") {
			log.Printf("Node %s is not leader, retrying...", usedEp)
			time.Sleep(200 * time.Millisecond)
			continue
		}
		return nil, err
	}
	return nil, fmt.Errorf("failed after retries - possible cluster issue")
}

func (c *SmartClient) BatchAllow(ctx context.Context) (pb.RateLimiter_BatchAllowClient, error) {
	for attempt := 0; attempt < 5; attempt++ {
		client, usedEp := c.getClient()
		if client == nil {
			return nil, fmt.Errorf("no available endpoints")
		}

		stream, err := client.BatchAllow(ctx)
		if err == nil {
			return stream, nil
		}

		if strings.Contains(err.Error(), "not leader") {
			log.Printf("Node %s is not leader for streaming, retrying...", usedEp)
			time.Sleep(200 * time.Millisecond)
			continue
		}
		return nil, err
	}
	return nil, fmt.Errorf("failed to open stream after retries")
}
