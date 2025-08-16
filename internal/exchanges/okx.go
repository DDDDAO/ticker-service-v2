package exchanges

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ddddao/ticker-service-v2/internal/config"
	"github.com/ddddao/ticker-service-v2/internal/logger"
	"github.com/gorilla/websocket"
)

// OKXHandler handles OKX WebSocket connections
type OKXHandler struct {
	config         config.ExchangeConfig
	conn           *websocket.Conn
	callback       func(*TickerData)
	status         ExchangeStatus
	mu             sync.RWMutex
	messageCount   int64
	errorCount     int64
	reconnectCount int64
}

// OKXTickerMessage represents OKX ticker WebSocket message
type OKXTickerMessage struct {
	Arg struct {
		Channel string `json:"channel"`
		InstID  string `json:"instId"`
	} `json:"arg"`
	Data []struct {
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
	} `json:"data"`
}

// NewOKXHandler creates a new OKX WebSocket handler
func NewOKXHandler(cfg config.ExchangeConfig) *OKXHandler {
	return &OKXHandler{
		config: cfg,
		status: ExchangeStatus{
			Symbols: cfg.Symbols,
		},
	}
}

// Connect establishes WebSocket connection to OKX
func (h *OKXHandler) Connect() error {
	log := logger.WithExchange("okx")
	log.Infof("Connecting to %s", h.config.WSURL)

	conn, _, err := websocket.DefaultDialer.Dial(h.config.WSURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	h.mu.Lock()
	h.conn = conn
	h.status.Connected = true
	h.status.LastMessage = time.Now()
	h.mu.Unlock()

	// Subscribe to ticker channels
	if err := h.subscribe(); err != nil {
		h.Close()
		return fmt.Errorf("failed to subscribe: %w", err)
	}

	// Start ping/pong handler
	go h.keepAlive()

	atomic.AddInt64(&h.reconnectCount, 1)
	log.Info("Connected successfully")
	return nil
}

// subscribe sends subscription message to OKX
func (h *OKXHandler) subscribe() error {
	args := make([]map[string]string, 0, len(h.config.Symbols))
	for _, symbol := range h.config.Symbols {
		args = append(args, map[string]string{
			"channel": "tickers",
			"instId":  symbol,
		})
	}

	subscribeMsg := map[string]interface{}{
		"op":   "subscribe",
		"args": args,
	}

	h.mu.RLock()
	conn := h.conn
	h.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("connection is nil")
	}

	return conn.WriteJSON(subscribeMsg)
}

// Close closes the WebSocket connection
func (h *OKXHandler) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.conn != nil {
		h.status.Connected = false
		err := h.conn.Close()
		h.conn = nil
		return err
	}
	return nil
}

// HandleMessages handles incoming WebSocket messages
func (h *OKXHandler) HandleMessages(ctx context.Context) error {
	log := logger.WithExchange("okx")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		h.mu.RLock()
		conn := h.conn
		h.mu.RUnlock()

		if conn == nil {
			return fmt.Errorf("connection is nil")
		}

		// Set read deadline
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))

		_, message, err := conn.ReadMessage()
		if err != nil {
			atomic.AddInt64(&h.errorCount, 1)
			if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Errorf("WebSocket error: %v", err)
			}
			return err
		}

		atomic.AddInt64(&h.messageCount, 1)

		// Parse and process message
		if err := h.processMessage(message); err != nil {
			atomic.AddInt64(&h.errorCount, 1)
			log.Errorf("Failed to process message: %v", err)
		}

		h.mu.Lock()
		h.status.LastMessage = time.Now()
		h.mu.Unlock()
	}
}

// processMessage processes a single WebSocket message
func (h *OKXHandler) processMessage(data []byte) error {
	// Check if it's a pong message
	var pong struct {
		Op string `json:"op"`
	}
	if err := json.Unmarshal(data, &pong); err == nil && pong.Op == "pong" {
		return nil // Ignore pong messages
	}

	// Parse ticker message
	var msg OKXTickerMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return fmt.Errorf("failed to parse message: %w", err)
	}

	// Process ticker data
	for _, ticker := range msg.Data {
		if err := h.processTicker(&ticker, msg.Arg.InstID); err != nil {
			return err
		}
	}

	return nil
}

// processTicker processes a ticker message
func (h *OKXHandler) processTicker(data *struct {
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
	// Use current time as timestamp since exchanges send cached/delayed data
	timestamp := time.Now()

	// Convert to normalized ticker data
	ticker := &TickerData{
		Symbol:    normalizeOKXSymbol(instID),
		Timestamp: timestamp,
	}

	// Parse prices
	fmt.Sscanf(data.Last, "%f", &ticker.Last)
	fmt.Sscanf(data.BidPx, "%f", &ticker.Bid)
	fmt.Sscanf(data.AskPx, "%f", &ticker.Ask)
	fmt.Sscanf(data.High24h, "%f", &ticker.High)
	fmt.Sscanf(data.Low24h, "%f", &ticker.Low)
	fmt.Sscanf(data.Vol24h, "%f", &ticker.Volume)

	// Call callback if set
	if h.callback != nil {
		h.callback(ticker)
	}

	return nil
}

// OnMessage sets the message callback
func (h *OKXHandler) OnMessage(callback func(*TickerData)) {
	h.callback = callback
}

// GetStatus returns the current status
func (h *OKXHandler) GetStatus() ExchangeStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()

	status := h.status
	status.MessageCount = atomic.LoadInt64(&h.messageCount)
	status.ErrorCount = atomic.LoadInt64(&h.errorCount)
	status.ReconnectCount = atomic.LoadInt64(&h.reconnectCount)
	return status
}

// keepAlive sends periodic pings to keep the connection alive
func (h *OKXHandler) keepAlive() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		h.mu.RLock()
		conn := h.conn
		connected := h.status.Connected
		h.mu.RUnlock()

		if !connected || conn == nil {
			return
		}

		// Send ping message (OKX format)
		pingMsg := map[string]string{"op": "ping"}
		if err := conn.WriteJSON(pingMsg); err != nil {
			logger.WithExchange("okx").Errorf("Failed to send ping: %v", err)
			return
		}
	}
}

// Subscribe adds a new symbol (not implemented for OKX - requires reconnection)
func (h *OKXHandler) Subscribe(symbol string) {
	logger.WithSymbol("okx", symbol).Warn("Dynamic subscription not supported - requires reconnection")
}

// Unsubscribe removes a symbol (not implemented for OKX - requires reconnection)
func (h *OKXHandler) Unsubscribe(symbol string) {
	logger.WithSymbol("okx", symbol).Warn("Dynamic unsubscription not supported - requires reconnection")
}

// GetSubscribedSymbols returns list of subscribed symbols
func (h *OKXHandler) GetSubscribedSymbols() []string {
	return h.config.Symbols
}

// normalizeOKXSymbol converts OKX symbol format to standard format
func normalizeOKXSymbol(symbol string) string {
	// Convert BTC-USDT to BTC/USDT
	return strings.ReplaceAll(symbol, "-", "/")
}