package exchanges

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ddddao/ticker-service-v2/internal/config"
	"github.com/ddddao/ticker-service-v2/internal/logger"
	"github.com/gorilla/websocket"
)

// OKXHandlerFixed is an improved OKX WebSocket handler that handles protocol quirks
type OKXHandlerFixed struct {
	config          config.ExchangeConfig
	conn            *websocket.Conn
	callback        func(*TickerData)
	status          ExchangeStatus
	messageCount    int64
	errorCount      int64
	reconnectCount  int64
	requestID       int64
	mu              sync.RWMutex
	writeMu         sync.Mutex
	symbolConverter *SymbolConverter
	lastDataTime    time.Time  // Track when we last received actual data
}

// NewOKXHandlerFixed creates a new improved OKX handler
func NewOKXHandlerFixed(cfg config.ExchangeConfig) *OKXHandlerFixed {
	return &OKXHandlerFixed{
		config: cfg,
		status: ExchangeStatus{
			Connected: false,
			Symbols:   cfg.Symbols,
		},
		symbolConverter: NewSymbolConverter(),
		lastDataTime:    time.Now(),
	}
}

// Connect establishes WebSocket connection to OKX
func (h *OKXHandlerFixed) Connect() error {
	log := logger.WithExchange("okx")
	log.Infof("Connecting to %s", h.config.WSURL)

	// Create custom dialer with specific settings for OKX
	dialer := websocket.Dialer{
		HandshakeTimeout:  30 * time.Second,
		ReadBufferSize:    8192,
		WriteBufferSize:   8192,
		EnableCompression: false,
	}
	
	// Add headers that OKX might expect
	headers := make(map[string][]string)
	headers["User-Agent"] = []string{"ticker-service-v2/1.0"}
	
	conn, _, err := dialer.Dial(h.config.WSURL, headers)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	h.mu.Lock()
	h.conn = conn
	h.status.Connected = true
	h.status.LastMessage = time.Now()
	h.lastDataTime = time.Now()
	h.mu.Unlock()

	// Subscribe to ticker channels
	if err := h.subscribe(); err != nil {
		h.Close()
		return fmt.Errorf("failed to subscribe: %w", err)
	}

	// Start ping/pong handler with OKX-specific timing
	go h.keepAlive()

	atomic.AddInt64(&h.reconnectCount, 1)
	log.Info("Connected successfully")
	return nil
}

// HandleMessages handles incoming WebSocket messages with better error handling
func (h *OKXHandlerFixed) HandleMessages(ctx context.Context) error {
	log := logger.WithExchange("okx")
	
	// Track consecutive errors
	consecutiveErrors := 0
	const maxConsecutiveErrors = 10

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		h.mu.RLock()
		conn := h.conn
		lastData := h.lastDataTime
		h.mu.RUnlock()

		if conn == nil {
			return fmt.Errorf("connection is nil")
		}

		// Set read deadline
		conn.SetReadDeadline(time.Now().Add(90 * time.Second))

		_, message, err := conn.ReadMessage()
		if err != nil {
			// Check if it's the specific OKX protocol error
			errStr := err.Error()
			if strings.Contains(errStr, "RSV1 set") && strings.Contains(errStr, "bad opcode") {
				// This is the known OKX quirk - check if we're still getting data
				timeSinceLastData := time.Since(lastData)
				
				if timeSinceLastData < 5*time.Minute {
					// We're still getting data, this is just OKX being weird
					log.Debug("OKX protocol quirk detected, but data is flowing normally")
					consecutiveErrors = 0 // Reset error counter
					continue // Just ignore this error and continue
				} else {
					// No data for 5 minutes, we should reconnect
					log.Warnf("OKX protocol error and no data for %v, reconnecting", timeSinceLastData)
					return err
				}
			}
			
			// Handle other errors normally
			atomic.AddInt64(&h.errorCount, 1)
			consecutiveErrors++
			
			if consecutiveErrors >= maxConsecutiveErrors {
				log.Errorf("Too many consecutive errors (%d), reconnecting", consecutiveErrors)
				return err
			}
			
			if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Errorf("WebSocket error: %v", err)
			}
			return err
		}

		// Reset error counter on successful read
		consecutiveErrors = 0
		atomic.AddInt64(&h.messageCount, 1)

		// Parse and process message
		if err := h.processMessage(message); err != nil {
			// Don't count processing errors as fatal
			log.Debugf("Failed to process message: %v", err)
		}

		h.mu.Lock()
		h.status.LastMessage = time.Now()
		h.mu.Unlock()
	}
}

// processMessage processes a single WebSocket message
func (h *OKXHandlerFixed) processMessage(data []byte) error {
	// Check if it's a pong message
	var pong struct {
		Op string `json:"op"`
		Event string `json:"event"`
		Code string `json:"code"`
		Msg string `json:"msg"`
	}
	
	if err := json.Unmarshal(data, &pong); err == nil {
		// Handle various OKX response types
		if pong.Op == "pong" {
			return nil // Ignore pong messages
		}
		if pong.Event == "subscribe" {
			if pong.Code != "0" {
				return fmt.Errorf("subscription error: %s", pong.Msg)
			}
			return nil // Subscription confirmation
		}
		if pong.Event == "error" {
			return fmt.Errorf("OKX error: %s", pong.Msg)
		}
	}

	// Parse ticker message
	var msg OKXTickerMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		// Not a ticker message, might be a system message
		return nil
	}

	// Process ticker data
	if len(msg.Data) > 0 {
		h.mu.Lock()
		h.lastDataTime = time.Now() // Update last data time
		h.mu.Unlock()
		
		for _, ticker := range msg.Data {
			if err := h.processTicker(&ticker, msg.Arg.InstID); err != nil {
				return err
			}
		}
	}

	return nil
}

// processTicker processes a ticker message (same as original)
func (h *OKXHandlerFixed) processTicker(data *struct {
	InstType  string `json:"instType"`
	InstID    string `json:"instId"`
	Last      string `json:"last"`
	LastSz    string `json:"lastSz"`
	AskPx     string `json:"askPx"`
	AskSz     string `json:"askSz"`
	BidPx     string `json:"bidPx"`
	BidSz     string `json:"bidSz"`
	Open24h   string `json:"open24h"`
	High24h   string `json:"high24h"`
	Low24h    string `json:"low24h"`
	Vol24h    string `json:"vol24h"`
	VolCcy24h string `json:"volCcy24h"`
	Timestamp string `json:"ts"`
}, instID string) error {
	timestamp := time.Now()

	// Convert to normalized ticker data
	symbol := strings.Replace(instID, "-SWAP", "", 1)
	ticker := &TickerData{
		Symbol:    h.symbolConverter.WebSocketToStorage("okx", symbol),
		Timestamp: timestamp,
	}

	// Parse price
	if last, err := strconv.ParseFloat(data.Last, 64); err == nil {
		ticker.Last = last
	}

	// Parse volume
	if vol, err := strconv.ParseFloat(data.Vol24h, 64); err == nil {
		ticker.Volume = vol
	}

	// Send to callback
	if h.callback != nil {
		h.callback(ticker)
	}

	return nil
}

// keepAlive sends periodic pings to keep the connection alive
func (h *OKXHandlerFixed) keepAlive() {
	// OKX recommends ping every 30 seconds
	ticker := time.NewTicker(25 * time.Second) // Slightly more frequent
	defer ticker.Stop()

	for range ticker.C {
		h.mu.RLock()
		conn := h.conn
		connected := h.status.Connected
		h.mu.RUnlock()

		if !connected || conn == nil {
			return
		}

		// Send ping in OKX format
		pingMsg := map[string]string{"op": "ping"}
		
		h.writeMu.Lock()
		err := conn.WriteJSON(pingMsg)
		h.writeMu.Unlock()
		
		if err != nil {
			// Only log as debug since OKX might close the connection oddly
			logger.WithExchange("okx").Debugf("Failed to send ping: %v", err)
			return
		}
	}
}

// subscribe sends subscription message to OKX
func (h *OKXHandlerFixed) subscribe() error {
	args := make([]map[string]string, 0, len(h.config.Symbols))
	for _, symbol := range h.config.Symbols {
		// Convert symbol format: btc-usdt -> BTC-USDT
		wsSymbol := h.symbolConverter.ConfigToWebSocket("okx", symbol)
		args = append(args, map[string]string{
			"channel": "tickers",
			"instId":  wsSymbol,
		})
	}

	subscribeMsg := map[string]interface{}{
		"op":   "subscribe",
		"args": args,
	}

	// Protect WebSocket write with mutex
	h.writeMu.Lock()
	defer h.writeMu.Unlock()
	
	if err := h.conn.WriteJSON(subscribeMsg); err != nil {
		return fmt.Errorf("failed to send subscribe message: %w", err)
	}

	logger.WithExchange("okx").Infof("Subscribed to %d symbols", len(h.config.Symbols))
	return nil
}

// Close closes the WebSocket connection
func (h *OKXHandlerFixed) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.conn != nil {
		h.conn.Close()
		h.conn = nil
	}
	h.status.Connected = false
}

// OnMessage sets the callback for ticker data
func (h *OKXHandlerFixed) OnMessage(callback func(*TickerData)) {
	h.callback = callback
}

// GetStatus returns the current status
func (h *OKXHandlerFixed) GetStatus() ExchangeStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()

	status := h.status
	status.MessageCount = atomic.LoadInt64(&h.messageCount)
	status.ErrorCount = atomic.LoadInt64(&h.errorCount)
	status.ReconnectCount = atomic.LoadInt64(&h.reconnectCount)
	return status
}

// Subscribe adds a new symbol by reconnecting with updated symbol list
func (h *OKXHandlerFixed) Subscribe(symbol string) {
	h.mu.Lock()
	
	// Check if already subscribed
	for _, s := range h.config.Symbols {
		if s == symbol {
			h.mu.Unlock()
			logger.WithSymbol("okx", symbol).Info("Already subscribed to symbol")
			return
		}
	}
	
	// Add to symbol list
	h.config.Symbols = append(h.config.Symbols, symbol)
	h.mu.Unlock()
	
	logger.WithSymbol("okx", symbol).Info("Adding symbol - reconnecting WebSocket")
	
	// Reconnect with new symbol list
	h.reconnect()
}

// Unsubscribe removes a symbol by reconnecting with updated symbol list
func (h *OKXHandlerFixed) Unsubscribe(symbol string) {
	h.mu.Lock()
	
	// Find and remove symbol
	newSymbols := make([]string, 0)
	found := false
	for _, s := range h.config.Symbols {
		if s != symbol {
			newSymbols = append(newSymbols, s)
		} else {
			found = true
		}
	}
	
	if !found {
		h.mu.Unlock()
		logger.WithSymbol("okx", symbol).Warn("Symbol not in subscription list")
		return
	}
	
	h.config.Symbols = newSymbols
	h.mu.Unlock()
	
	logger.WithSymbol("okx", symbol).Info("Removing symbol - reconnecting WebSocket")
	
	// Reconnect with updated symbol list
	h.reconnect()
}

// reconnect triggers a reconnection with current symbol list
func (h *OKXHandlerFixed) reconnect() {
	h.Close()
	// The manager will handle the reconnection
}