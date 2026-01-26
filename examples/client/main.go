package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"strings"
	"sync"
	"time"

	pb "github.com/codemaestro64/flux/internal/gen/v1"
)

var (
	endpoints = flag.String("endpoints", "localhost:50052,localhost:50054,localhost:50056", "comma-separated list of gRPC endpoints")
	requests  = flag.Int("requests", 1000, "number of requests to send")
	batchSize = flag.Int("batch", 50, "batch size for streaming")
	keyPrefix = flag.String("key-prefix", "user", "key prefix")
)

func main() {
	flag.Parse()

	endpointList := strings.Split(*endpoints, ",")
	client := NewSmartClient(endpointList)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Send many single Allow requests
	log.Printf("Sending %d single Allow requests...", *requests)
	start := time.Now()
	for i := 0; i < *requests; i++ {
		key := fmt.Sprintf("%s-%d", *keyPrefix, rand.Intn(1000))
		resp, err := client.Allow(ctx, key, 1)
		if err != nil {
			log.Printf("Allow failed: %v", err)
			continue
		}
		if i%100 == 0 {
			log.Printf("Key: %s → Allowed: %v, Remaining: %.2f", key, resp.Allowed, resp.TokensRemaining)
		}
	}
	log.Printf("Single requests completed in %v", time.Since(start))

	// Batch streaming
	log.Printf("Sending batch streaming with batch size %d...", *batchSize)
	start = time.Now()
	stream, err := client.BatchAllow(ctx)
	if err != nil {
		log.Fatalf("Failed to open stream: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < *requests; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := fmt.Sprintf("%s-batch-%d", *keyPrefix, idx)
			if err := stream.Send(&pb.AllowRequest{Key: key, Tokens: 1}); err != nil {
				log.Printf("Send failed: %v", err)
			}
		}(i)

		if (i+1)%*batchSize == 0 || i == *requests-1 {
			// drain responses
			for j := 0; j < (i+1)%*batchSize; j++ {
				resp, err := stream.Recv()
				if err == io.EOF {
					break
				}
				if err != nil {
					log.Printf("Recv failed: %v", err)
					break
				}
				if j%10 == 0 {
					log.Printf("Batch response: %s → %v (%.2f remaining)", resp.Key, resp.Allowed, resp.TokensRemaining)
				}
			}
		}
	}

	wg.Wait()
	stream.CloseSend()
	log.Printf("Batch streaming completed in %v", time.Since(start))
}
