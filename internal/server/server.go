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
	"github.com/ddddao/ticker-service-v2/internal/storage"
	"github.com/sirupsen/logrus"
)

// Server represents the HTTP server
type Server struct {
	*http.Server
	manager *exchanges.Manager
	storage storage.Storage
}

// New creates a new HTTP server
func New(cfg config.ServerConfig, manager *exchanges.Manager, storage storage.Storage) *Server {
	mux := http.NewServeMux()
	
	s := &Server{
		Server: &http.Server{
			Addr:    fmt.Sprintf(":%d", cfg.Port),
			Handler: mux,
		},
		manager: manager,
		storage: storage,
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
	// Check storage connection
	storageOK := s.storage.IsConnected()
	storageType := "memory"
	
	// Check if it's Redis storage
	if _, ok := s.storage.(*storage.RedisStorage); ok {
		storageType = "redis"
	}

	response := map[string]interface{}{
		"status":       "ok",
		"storage":      storageOK,
		"storage_type": storageType,
		"time":         time.Now().UTC(),
		"app":          "ticker-service-v2",
	}

	if !storageOK {
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

	// Get latest ticker from storage
	// Use longer timeout to allow for auto-subscription
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	data, err := s.storage.Get(ctx, exchange, storageSymbol)
	if err != nil {
		// Try to auto-subscribe to the symbol
		// Input symbol is already in config format (btc-usdt)
		// Use the original symbol (in config format) for subscription
		logrus.WithFields(logrus.Fields{
			"exchange": exchange,
			"symbol":   symbol,
			"app":      "ticker-service-v2",
		}).Info("Symbol not found in storage, attempting auto-subscription")
		
		if subscribeErr := s.manager.Subscribe(exchange, strings.ToLower(symbol)); subscribeErr == nil {
			// Wait for data to arrive (all exchanges support dynamic subscription)
			// Try multiple times with short delays
			for i := 0; i < 5; i++ {
				time.Sleep(500 * time.Millisecond)
				
				// Try to get the data again
				data, err = s.storage.Get(ctx, exchange, storageSymbol)
				if err == nil {
					// Success! Continue to return the data
					logrus.WithFields(logrus.Fields{
						"exchange": exchange,
						"symbol":   symbol,
						"attempt":  i + 1,
						"app":      "ticker-service-v2",
					}).Info("Auto-subscription successful, data received")
					goto returnData
				}
			}
			
			// If still no data after waiting, return appropriate message
			http.Error(w, fmt.Sprintf("Subscribed to %s on %s. Please try again in a few seconds for data to arrive.", symbol, exchange), http.StatusAccepted)
			return
		}
		
		// Subscription failed - symbol might not be valid
		http.Error(w, fmt.Sprintf("Failed to subscribe to %s on %s. Symbol may not be valid or available.", symbol, exchange), http.StatusNotFound)
		logrus.WithFields(logrus.Fields{
				"exchange": exchange,
				"symbol":   symbol,
				"error":    err,
				"app":      "ticker-service-v2",
			}).Error("Failed to get ticker from storage and auto-subscription failed")
		return
	}
	
returnData:

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
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