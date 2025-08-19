package grpc

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/ddddao/ticker-service-v2/internal/exchanges"
	"github.com/ddddao/ticker-service-v2/internal/storage"
	pb "github.com/ddddao/ticker-service-v2/proto"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	pb.UnimplementedTickerServiceServer
	grpcServer *grpc.Server
	manager    *exchanges.Manager
	storage    storage.Storage
	port       int
}

// NewServer creates a new gRPC server
func NewServer(port int, manager *exchanges.Manager, storage storage.Storage) *Server {
	// Configure gRPC server with optimizations for low latency
	opts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(1024 * 1024 * 4), // 4MB
		grpc.MaxSendMsgSize(1024 * 1024 * 4), // 4MB
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    10 * time.Second, // Send keepalive every 10s
			Timeout: 5 * time.Second,  // Wait 5s for keepalive ack
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             5 * time.Second, // Minimum time between client pings
			PermitWithoutStream: true,            // Allow keepalive without active streams
		}),
	}

	grpcServer := grpc.NewServer(opts...)
	server := &Server{
		grpcServer: grpcServer,
		manager:    manager,
		storage:    storage,
		port:       port,
	}

	pb.RegisterTickerServiceServer(grpcServer, server)
	return server
}

// Start starts the gRPC server
func (s *Server) Start() error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	logrus.WithField("port", s.port).Info("Starting gRPC server")
	
	go func() {
		if err := s.grpcServer.Serve(lis); err != nil {
			logrus.WithError(err).Error("gRPC server failed")
		}
	}()

	return nil
}

// Stop stops the gRPC server
func (s *Server) Stop() {
	logrus.Info("Stopping gRPC server")
	s.grpcServer.GracefulStop()
}

// GetTicker implements the GetTicker RPC
func (s *Server) GetTicker(ctx context.Context, req *pb.GetTickerRequest) (*pb.TickerData, error) {
	if req.Exchange == "" || req.Symbol == "" {
		return nil, status.Error(codes.InvalidArgument, "exchange and symbol are required")
	}

	// Get from storage with timeout
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	data, err := s.storage.Get(ctx, req.Exchange, req.Symbol)
	if err != nil {
		// Try to auto-subscribe and wait for data
		logrus.WithFields(logrus.Fields{
			"exchange": req.Exchange,
			"symbol":   req.Symbol,
		}).Debug("Symbol not found, attempting auto-subscription")
		
		if subscribeErr := s.manager.Subscribe(req.Exchange, req.Symbol); subscribeErr == nil {
			// Wait for data to arrive, check more frequently
			for i := 0; i < 50; i++ {
				time.Sleep(100 * time.Millisecond) // Check every 100ms instead of 500ms
				
				data, err = s.storage.Get(ctx, req.Exchange, req.Symbol)
				if err == nil {
					// Got data, break out of loop and continue to parse it
					logrus.WithFields(logrus.Fields{
						"exchange": req.Exchange,
						"symbol":   req.Symbol,
						"waitTime": fmt.Sprintf("%dms", (i+1)*100),
					}).Debug("Auto-subscription successful, data received")
					break
				}
			}
			
			if err != nil {
				return nil, status.Errorf(codes.DeadlineExceeded, "ticker not found after 5s wait")
			}
		} else {
			return nil, status.Errorf(codes.NotFound, "ticker not found and subscription failed: %v", subscribeErr)
		}
	}

	// Parse the stored JSON data
	var tickerData exchanges.TickerData
	if err := json.Unmarshal(data, &tickerData); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to parse ticker data: %v", err)
	}

	return &pb.TickerData{
		Symbol:    tickerData.Symbol,
		Last:      tickerData.Last,
		Bid:       tickerData.Bid,
		Ask:       tickerData.Ask,
		High:      tickerData.High,
		Low:       tickerData.Low,
		Volume:    tickerData.Volume,
		Timestamp: tickerData.Timestamp.Format(time.RFC3339),
		Exchange:  req.Exchange,
	}, nil
}

// GetBatchTickers implements the GetBatchTickers RPC
func (s *Server) GetBatchTickers(ctx context.Context, req *pb.BatchTickerRequest) (*pb.BatchTickerResponse, error) {
	response := &pb.BatchTickerResponse{
		Tickers: make(map[string]*pb.TickerData),
	}

	// Process all requests in parallel with short timeout
	ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()

	ch := make(chan struct {
		key    string
		ticker *pb.TickerData
	}, len(req.Requests))

	for _, r := range req.Requests {
		go func(r *pb.GetTickerRequest) {
			ticker, err := s.GetTicker(ctx, r)
			if err == nil {
				ch <- struct {
					key    string
					ticker *pb.TickerData
				}{
					key:    fmt.Sprintf("%s:%s", r.Exchange, r.Symbol),
					ticker: ticker,
				}
			}
		}(r)
	}

	// Collect results
	timeout := time.After(200 * time.Millisecond)
	for i := 0; i < len(req.Requests); i++ {
		select {
		case result := <-ch:
			response.Tickers[result.key] = result.ticker
		case <-timeout:
			break
		case <-ctx.Done():
			break
		}
	}

	return response, nil
}

// StreamTickers implements the StreamTickers RPC for real-time updates
func (s *Server) StreamTickers(req *pb.StreamTickerRequest, stream pb.TickerService_StreamTickersServer) error {
	// This would connect to Redis pub/sub and stream updates
	// For now, we'll implement a simple polling version
	
	ticker := time.NewTicker(100 * time.Millisecond) // Stream updates every 100ms
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			for _, sub := range req.Subscriptions {
				data, err := s.storage.Get(stream.Context(), sub.Exchange, sub.Symbol)
				if err != nil {
					continue
				}

				var tickerData exchanges.TickerData
				if err := json.Unmarshal(data, &tickerData); err != nil {
					continue
				}

				if err := stream.Send(&pb.TickerData{
					Symbol:    tickerData.Symbol,
					Last:      tickerData.Last,
					Bid:       tickerData.Bid,
					Ask:       tickerData.Ask,
					High:      tickerData.High,
					Low:       tickerData.Low,
					Volume:    tickerData.Volume,
					Timestamp: tickerData.Timestamp.Format(time.RFC3339),
					Exchange:  sub.Exchange,
				}); err != nil {
					return err
				}
			}
		case <-stream.Context().Done():
			return stream.Context().Err()
		}
	}
}

// HealthCheck implements the HealthCheck RPC
func (s *Server) HealthCheck(ctx context.Context, _ *pb.Empty) (*pb.HealthResponse, error) {
	storageOK := s.storage.IsConnected()
	storageType := "memory"
	
	if _, ok := s.storage.(*storage.RedisStorage); ok {
		storageType = "redis"
	}

	return &pb.HealthResponse{
		Status:      "ok",
		StorageOk:   storageOK,
		StorageType: storageType,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}, nil
}