package main

import (
	"context"
	"fmt"
	"log"
	"time"

	pb "github.com/ddddao/ticker-service-v2/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	// Connect to gRPC server
	conn, err := grpc.Dial("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	client := pb.NewTickerServiceClient(conn)
	ctx := context.Background()

	// Test 1: Health check
	health, err := client.HealthCheck(ctx, &pb.Empty{})
	if err != nil {
		log.Printf("Health check failed: %v", err)
	} else {
		fmt.Printf("Health: %+v\n", health)
	}

	// Test 2: Get ticker with timing
	start := time.Now()
	ticker, err := client.GetTicker(ctx, &pb.GetTickerRequest{
		Exchange: "bybit",
		Symbol:   "btc-usdt",
	})
	elapsed := time.Since(start)
	
	if err != nil {
		log.Printf("GetTicker failed: %v", err)
	} else {
		fmt.Printf("Ticker: %+v\n", ticker)
		fmt.Printf("gRPC call took: %v\n", elapsed)
	}

	// Test 3: Test auto-subscription speed
	fmt.Println("\nTesting auto-subscription for new symbol...")
	start = time.Now()
	ticker, err = client.GetTicker(ctx, &pb.GetTickerRequest{
		Exchange: "bybit",
		Symbol:   "dot-usdt",  // Not in config
	})
	elapsed = time.Since(start)
	
	if err != nil {
		log.Printf("Auto-subscription test failed: %v", err)
	} else {
		fmt.Printf("Auto-subscribed ticker: %+v\n", ticker)
		fmt.Printf("Auto-subscription took: %v\n", elapsed)
	}
}