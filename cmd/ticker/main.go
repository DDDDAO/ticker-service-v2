package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ddddao/ticker-service-v2/internal/config"
	"github.com/ddddao/ticker-service-v2/internal/exchanges"
	"github.com/ddddao/ticker-service-v2/internal/logger"
	"github.com/ddddao/ticker-service-v2/internal/redis"
	"github.com/ddddao/ticker-service-v2/internal/server"
	"github.com/sirupsen/logrus"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "", "path to config file")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		logrus.Fatalf("Failed to load config: %v", err)
	}

	// Initialize logger
	logger.Init(cfg.Logging)

	// Initialize Redis client
	redisClient, err := redis.NewClient(cfg.Redis)
	if err != nil {
		logrus.Fatalf("Failed to connect to Redis: %v", err)
	}
	defer redisClient.Close()

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize exchange manager
	manager := exchanges.NewManager(redisClient)

	// Start enabled exchanges
	for name, exchCfg := range cfg.Exchanges {
		if !exchCfg.Enabled {
			continue
		}

		handler, err := exchanges.NewHandler(name, exchCfg)
		if err != nil {
			logrus.Errorf("Failed to create handler for %s: %v", name, err)
			continue
		}

		if err := manager.AddExchange(name, handler); err != nil {
			logrus.Errorf("Failed to add exchange %s: %v", name, err)
			continue
		}

		logrus.Infof("Added exchange: %s with %d symbols", name, len(exchCfg.Symbols))
	}

	// Start the manager
	manager.Start(ctx)

	// Start HTTP server
	srv := server.New(cfg.Server, manager, redisClient)
	go func() {
		logrus.Infof("Starting HTTP server on port %d", cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logrus.Fatalf("HTTP server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logrus.Info("Shutting down ticker service...")

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	// Stop manager
	cancel()

	// Shutdown HTTP server
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logrus.Errorf("HTTP server shutdown error: %v", err)
	}

	// Wait for all goroutines to finish
	manager.Wait()

	logrus.Info("Ticker service shutdown complete")
}