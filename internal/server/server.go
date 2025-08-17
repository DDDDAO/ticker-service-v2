package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ddddao/ticker-service-v2/internal/config"
	"github.com/ddddao/ticker-service-v2/internal/exchanges"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
)

// Server represents the HTTP server
type Server struct {
	*http.Server
	manager *exchanges.Manager
	redis   *redis.Client
}

// New creates a new HTTP server
func New(cfg config.ServerConfig, manager *exchanges.Manager, redisClient *redis.Client) *Server {
	mux := http.NewServeMux()
	
	s := &Server{
		Server: &http.Server{
			Addr:    fmt.Sprintf(":%d", cfg.Port),
			Handler: mux,
		},
		manager: manager,
		redis:   redisClient,
	}

	// Register routes
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/status", s.handleStatus)
	mux.HandleFunc("/api/ticker/", s.handleTicker)
	mux.HandleFunc("/api/subscribe", s.handleSubscribe)
	mux.HandleFunc("/api/unsubscribe", s.handleUnsubscribe)
	mux.HandleFunc("/metrics", s.handleMetrics)

	return s
}

// handleHealth handles health check requests
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	// Check Redis connection
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	redisOK := s.redis.Ping(ctx).Err() == nil

	response := map[string]interface{}{
		"status": "ok",
		"redis":  redisOK,
		"time":   time.Now().UTC(),
		"app":    "ticker-service-v2",
	}

	if !redisOK {
		w.WriteHeader(http.StatusServiceUnavailable)
		response["status"] = "degraded"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleStatus handles status requests
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	status := s.manager.GetStatus()
	
	response := map[string]interface{}{
		"app":       "ticker-service-v2",
		"exchanges": status,
		"time":      time.Now().UTC(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleTicker handles ticker requests
func (s *Server) handleTicker(w http.ResponseWriter, r *http.Request) {
	// Parse path: /api/ticker/{exchange}/{symbol}
	// Symbol might be like BTC-USDT or BTC/USDT
	path := strings.TrimPrefix(r.URL.Path, "/api/ticker/")
	
	// Find the first slash to separate exchange from symbol
	firstSlash := strings.Index(path, "/")
	if firstSlash == -1 {
		http.Error(w, "Invalid path format. Expected: /api/ticker/{exchange}/{symbol}", http.StatusBadRequest)
		return
	}
	
	exchange := path[:firstSlash]
	symbol := path[firstSlash+1:]
	
	// Validate format: should be symbol-quote (e.g., btc-usdt)
	if !strings.Contains(symbol, "-") {
		http.Error(w, "Invalid symbol format. Use symbol-quote format (e.g., btc-usdt)", http.StatusBadRequest)
		return
	}
	
	// Keep symbol in lowercase-dash format (btc-usdt)
	// This is the exact symbol name - no conversion needed
	storageSymbol := strings.ToLower(symbol)

	// Get latest ticker from Redis
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	// Redis key format: ticker:{exchange}:{symbol}
	key := fmt.Sprintf("ticker:%s:%s", exchange, storageSymbol)
	data, err := s.redis.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			// Try to auto-subscribe to the symbol (only works for Binance)
			// Input symbol is already in config format (btc-usdt)
			// Use the original symbol (in config format) for subscription
			if subscribeErr := s.manager.Subscribe(exchange, strings.ToLower(symbol)); subscribeErr == nil {
				// Wait a moment for data to arrive (only for Binance which supports dynamic subscription)
				if exchange == "binance" {
					time.Sleep(2 * time.Second)
					
					// Try to get the data again
					data, err = s.redis.Get(ctx, key).Result()
					if err == nil {
						// Success! Continue to return the data
						goto returnData
					}
				}
			}
			
			// Different error messages based on exchange capabilities
			if exchange == "binance" {
				http.Error(w, "Ticker not found. Try again in a few seconds if this is a valid symbol.", http.StatusNotFound)
			} else {
				http.Error(w, fmt.Sprintf("Symbol not available on %s. This exchange requires pre-configuration of symbols.", exchange), http.StatusNotFound)
			}
		} else {
			logrus.WithFields(logrus.Fields{
				"exchange": exchange,
				"symbol":   symbol,
				"error":    err,
				"app":      "ticker-service-v2",
			}).Error("Failed to get ticker from Redis")
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}
	
returnData:

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(data))
}

// handleMetrics handles Prometheus metrics requests
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	status := s.manager.GetStatus()
	
	// Simple Prometheus format metrics
	metrics := []string{
		"# HELP ticker_websocket_connected WebSocket connection status (1=connected, 0=disconnected)",
		"# TYPE ticker_websocket_connected gauge",
	}

	for exchange, stat := range status {
		connected := 0
		if stat.Connected {
			connected = 1
		}
		metrics = append(metrics, fmt.Sprintf("ticker_websocket_connected{exchange=\"%s\"} %d", exchange, connected))
	}

	metrics = append(metrics,
		"# HELP ticker_messages_total Total number of messages received",
		"# TYPE ticker_messages_total counter",
	)

	for exchange, stat := range status {
		metrics = append(metrics, fmt.Sprintf("ticker_messages_total{exchange=\"%s\"} %d", exchange, stat.MessageCount))
	}

	metrics = append(metrics,
		"# HELP ticker_errors_total Total number of errors",
		"# TYPE ticker_errors_total counter",
	)

	for exchange, stat := range status {
		metrics = append(metrics, fmt.Sprintf("ticker_errors_total{exchange=\"%s\"} %d", exchange, stat.ErrorCount))
	}

	metrics = append(metrics,
		"# HELP ticker_reconnects_total Total number of reconnections",
		"# TYPE ticker_reconnects_total counter",
	)

	for exchange, stat := range status {
		metrics = append(metrics, fmt.Sprintf("ticker_reconnects_total{exchange=\"%s\"} %d", exchange, stat.ReconnectCount))
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.Write([]byte(strings.Join(metrics, "\n")))
}

// handleSubscribe handles subscription requests
func (s *Server) handleSubscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	// Parse request body
	var req struct {
		Exchange string `json:"exchange"`
		Symbol   string `json:"symbol"`
	}
	
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	
	// Validate inputs
	if req.Exchange == "" || req.Symbol == "" {
		http.Error(w, "Exchange and symbol are required", http.StatusBadRequest)
		return
	}
	
	// Subscribe
	if err := s.manager.Subscribe(req.Exchange, req.Symbol); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	
	response := map[string]interface{}{
		"status":   "subscribed",
		"exchange": req.Exchange,
		"symbol":   req.Symbol,
		"time":     time.Now().UTC(),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleUnsubscribe handles unsubscription requests
func (s *Server) handleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	// Parse request body
	var req struct {
		Exchange string `json:"exchange"`
		Symbol   string `json:"symbol"`
	}
	
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	
	// Validate inputs
	if req.Exchange == "" || req.Symbol == "" {
		http.Error(w, "Exchange and symbol are required", http.StatusBadRequest)
		return
	}
	
	// Unsubscribe
	if err := s.manager.Unsubscribe(req.Exchange, req.Symbol); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	
	response := map[string]interface{}{
		"status":   "unsubscribed",
		"exchange": req.Exchange,
		"symbol":   req.Symbol,
		"time":     time.Now().UTC(),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}