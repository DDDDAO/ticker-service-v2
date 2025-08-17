package exchanges

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ddddao/ticker-service-v2/internal/config"
	"github.com/ddddao/ticker-service-v2/internal/logger"
	"github.com/gorilla/websocket"
)

// BitgetHandler handles Bitget WebSocket connections
type BitgetHandler struct {
	config         config.ExchangeConfig
	conn           *websocket.Conn
	callback       func(*TickerData)
	status         ExchangeStatus
	mu             sync.RWMutex
	messageCount   int64
	errorCount     int64
	reconnectCount int64
	symbolConverter *SymbolConverter
}

// BitgetTickerMessage represents Bitget ticker WebSocket message
type BitgetTickerMessage struct {
	Action string `json:"action"`
	Arg    struct {
		InstType string `json:"instType"`
		Channel  string `json:"channel"`
		InstID   string `json:"instId"`
	} `json:"arg"`
	Data []struct {
		InstID       string `json:"instId"`
		Last         string `json:"last"`        // Last price
		BestBid      string `json:"bestBid"`     // Best bid price
		BidSz        string `json:"bidSz"`       // Bid size
		BestAsk      string `json:"bestAsk"`     // Best ask price
		AskSz        string `json:"askSz"`       // Ask size
		Open24h      string `json:"open24h"`     // 24h open
		High24h      string `json:"high24h"`     // 24h high
		Low24h       string `json:"low24h"`      // 24h low
		Change24h    string `json:"change24h"`   // 24h change
		BaseVolume   string `json:"baseVolume"`  // Base volume
		QuoteVolume  string `json:"quoteVolume"` // Quote volume
		OpenUtc      string `json:"openUtc"`
		ChangeUtc24h string `json:"changeUtc24h"`
		Timestamp    string `json:"ts"`
	} `json:"data"`
	Ts int64 `json:"ts"`
}

// NewBitgetHandler creates a new Bitget WebSocket handler
func NewBitgetHandler(cfg config.ExchangeConfig) *BitgetHandler {
	return &BitgetHandler{
		config: cfg,
		status: ExchangeStatus{
			Symbols: cfg.Symbols,
		},
		symbolConverter: NewSymbolConverter(),
	}
}

// Connect establishes WebSocket connection to Bitget
func (h *BitgetHandler) Connect() error {
	log := logger.WithExchange("bitget")
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

// subscribe sends subscription message to Bitget
func (h *BitgetHandler) subscribe() error {
	args := make([]map[string]string, 0, len(h.config.Symbols))
	for _, symbol := range h.config.Symbols {
		// Convert symbol format: btc-usdt -> BTCUSDT
		wsSymbol := h.symbolConverter.ConfigToWebSocket("bitget", symbol)
		args = append(args, map[string]string{
			"instType": "mc",
			"channel":  "ticker",
			"instId":   wsSymbol,
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
func (h *BitgetHandler) Close() error {
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
func (h *BitgetHandler) HandleMessages(ctx context.Context) error {
	log := logger.WithExchange("bitget")

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
func (h *BitgetHandler) processMessage(data []byte) error {
	// Check if it's a plain pong message
	if string(data) == "pong" {
		return nil
	}
	
	// Check if it's a pong or subscription response
	var response struct {
		Op   string `json:"op"`
		Code string `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(data, &response); err == nil {
		if response.Op == "pong" || response.Code == "0" {
			return nil // Ignore pong and success messages
		}
	}

	// Log raw message for debugging
	log := logger.WithExchange("bitget")
	log.Debugf("Raw message: %s", string(data))

	// Parse ticker message
	var msg BitgetTickerMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Errorf("Failed to parse ticker message: %v", err)
		return fmt.Errorf("failed to parse message: %w", err)
	}

	// Process ticker data
	if msg.Action == "snapshot" || msg.Action == "update" {
		log.Debugf("Processing %d tickers, action: %s", len(msg.Data), msg.Action)
		for _, ticker := range msg.Data {
			if err := h.processTicker(&ticker); err != nil {
				return err
			}
		}
	} else {
		log.Debugf("Ignoring message with action: %s", msg.Action)
	}

	return nil
}

// processTicker processes a ticker message
func (h *BitgetHandler) processTicker(data *struct {
	InstID       string `json:"instId"`
	Last         string `json:"last"`        // Last price
	BestBid      string `json:"bestBid"`     // Best bid price
	BidSz        string `json:"bidSz"`       // Bid size
	BestAsk      string `json:"bestAsk"`     // Best ask price
	AskSz        string `json:"askSz"`       // Ask size
	Open24h      string `json:"open24h"`     // 24h open
	High24h      string `json:"high24h"`     // 24h high
	Low24h       string `json:"low24h"`      // 24h low
	Change24h    string `json:"change24h"`   // 24h change
	BaseVolume   string `json:"baseVolume"`  // Base volume
	QuoteVolume  string `json:"quoteVolume"` // Quote volume
	OpenUtc      string `json:"openUtc"`
	ChangeUtc24h string `json:"changeUtc24h"`
	Timestamp    string `json:"ts"`
}) error {
	// Use current time as timestamp since exchanges send cached/delayed data
	timestamp := time.Now()

	// Convert to normalized ticker data
	ticker := &TickerData{
		Symbol:    h.symbolConverter.WebSocketToStorage("bitget", data.InstID),
		Timestamp: timestamp,
	}

	// Parse prices using correct field names
	ticker.Last, _ = strconv.ParseFloat(data.Last, 64)
	ticker.Bid, _ = strconv.ParseFloat(data.BestBid, 64)
	ticker.Ask, _ = strconv.ParseFloat(data.BestAsk, 64)
	ticker.High, _ = strconv.ParseFloat(data.High24h, 64)
	ticker.Low, _ = strconv.ParseFloat(data.Low24h, 64)
	ticker.Volume, _ = strconv.ParseFloat(data.BaseVolume, 64)

	// Call callback if set
	if h.callback != nil {
		h.callback(ticker)
	}

	return nil
}

// OnMessage sets the message callback
func (h *BitgetHandler) OnMessage(callback func(*TickerData)) {
	h.callback = callback
}

// GetStatus returns the current status
func (h *BitgetHandler) GetStatus() ExchangeStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()

	status := h.status
	status.MessageCount = atomic.LoadInt64(&h.messageCount)
	status.ErrorCount = atomic.LoadInt64(&h.errorCount)
	status.ReconnectCount = atomic.LoadInt64(&h.reconnectCount)
	return status
}

// keepAlive sends periodic pings to keep the connection alive
func (h *BitgetHandler) keepAlive() {
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

		// Send ping message (Bitget format)
		pingMsg := "ping"
		if err := conn.WriteMessage(websocket.TextMessage, []byte(pingMsg)); err != nil {
			logger.WithExchange("bitget").Errorf("Failed to send ping: %v", err)
			return
		}
	}
}

// Subscribe adds a new symbol (not implemented for Bitget - requires reconnection)
func (h *BitgetHandler) Subscribe(symbol string) {
	logger.WithSymbol("bitget", symbol).Warn("Dynamic subscription not supported - requires reconnection")
}

// Unsubscribe removes a symbol (not implemented for Bitget - requires reconnection)
func (h *BitgetHandler) Unsubscribe(symbol string) {
	logger.WithSymbol("bitget", symbol).Warn("Dynamic unsubscription not supported - requires reconnection")
}

// GetSubscribedSymbols returns list of subscribed symbols
func (h *BitgetHandler) GetSubscribedSymbols() []string {
	return h.config.Symbols
}

// normalizeBitgetSymbol converts Bitget symbol format to standard format
func normalizeBitgetSymbol(symbol string) string {
	// BTCUSDT -> BTC/USDT
	if len(symbol) > 4 && symbol[len(symbol)-4:] == "USDT" {
		return symbol[:len(symbol)-4] + "/USDT"
	}
	return symbol
}