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

// BybitHandler handles Bybit WebSocket connections
type BybitHandler struct {
	config         config.ExchangeConfig
	conn           *websocket.Conn
	callback       func(*TickerData)
	status         ExchangeStatus
	mu             sync.RWMutex
	messageCount   int64
	errorCount     int64
	reconnectCount int64
	tickerCache    map[string]*TickerData // Cache for merging delta updates
	cacheMu        sync.RWMutex
}

// BybitTickerData represents the ticker data structure
type BybitTickerData struct {
	Symbol            string `json:"symbol"`
	LastPrice         string `json:"lastPrice"`
	Bid1Price         string `json:"bid1Price"`
	Bid1Size          string `json:"bid1Size"`
	Ask1Price         string `json:"ask1Price"`
	Ask1Size          string `json:"ask1Size"`
	HighPrice24h      string `json:"highPrice24h"`
	LowPrice24h       string `json:"lowPrice24h"`
	PrevPrice24h      string `json:"prevPrice24h"`
	Volume24h         string `json:"volume24h"`
	Turnover24h       string `json:"turnover24h"`
	Price24hPcnt      string `json:"price24hPcnt"`
	UsdIndexPrice     string `json:"usdIndexPrice"`
	PrevPrice1h       string `json:"prevPrice1h"`
	MarkPrice         string `json:"markPrice"`
	IndexPrice        string `json:"indexPrice"`
	OpenInterest      string `json:"openInterest"`
	OpenInterestValue string `json:"openInterestValue"`
	TotalTurnover     string `json:"totalTurnover"`
	TotalVolume       string `json:"totalVolume"`
	NextFundingTime   string `json:"nextFundingTime"`
	FundingRate       string `json:"fundingRate"`
	Bid1Iv            string `json:"bid1Iv"`
	Ask1Iv            string `json:"ask1Iv"`
	MarkIv            string `json:"markIv"`
	UnderlyingPrice   string `json:"underlyingPrice"`
	Delta             string `json:"delta"`
	Gamma             string `json:"gamma"`
	Vega              string `json:"vega"`
	Theta             string `json:"theta"`
}

// BybitTickerMessage represents Bybit ticker WebSocket message
// Data can be either a single object or an array
type BybitTickerMessage struct {
	Topic string          `json:"topic"`
	Type  string          `json:"type"`
	Ts    int64           `json:"ts"`
	Data  json.RawMessage `json:"data"` // Can be object or array
}

// NewBybitHandler creates a new Bybit WebSocket handler
func NewBybitHandler(cfg config.ExchangeConfig) *BybitHandler {
	return &BybitHandler{
		config: cfg,
		status: ExchangeStatus{
			Symbols: cfg.Symbols,
		},
		tickerCache: make(map[string]*TickerData),
	}
}

// Connect establishes WebSocket connection to Bybit
func (h *BybitHandler) Connect() error {
	log := logger.WithExchange("bybit")
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

// subscribe sends subscription message to Bybit
func (h *BybitHandler) subscribe() error {
	args := make([]string, 0, len(h.config.Symbols))
	for _, symbol := range h.config.Symbols {
		// Bybit uses format: tickers.{symbol}
		args = append(args, fmt.Sprintf("tickers.%s", symbol))
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
func (h *BybitHandler) Close() error {
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
func (h *BybitHandler) HandleMessages(ctx context.Context) error {
	log := logger.WithExchange("bybit")

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
func (h *BybitHandler) processMessage(data []byte) error {
	// Check if it's a pong or success message
	var response struct {
		Op      string `json:"op"`
		Success bool   `json:"success"`
		RetMsg  string `json:"ret_msg"`
	}
	if err := json.Unmarshal(data, &response); err == nil {
		if response.Op == "pong" || response.Op == "subscribe" {
			return nil // Ignore pong and subscription confirmation messages
		}
	}

	// Parse ticker message
	var msg BybitTickerMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return fmt.Errorf("failed to parse message: %w", err)
	}

	// Process ticker data (both snapshot and delta updates)
	if msg.Topic != "" && (msg.Type == "snapshot" || msg.Type == "delta") {
		return h.processTicker(&msg)
	}
	
	// Debug log unhandled messages (only if verbose logging is enabled)
	if logger.ShouldLogTickerUpdates() && msg.Topic != "" {
		logger.WithExchange("bybit").Debugf("Unhandled message type: %s for topic: %s", msg.Type, msg.Topic)
	}

	return nil
}

// processTicker processes a ticker message
func (h *BybitHandler) processTicker(msg *BybitTickerMessage) error {
	// Try to parse as single object first
	var data BybitTickerData
	if err := json.Unmarshal(msg.Data, &data); err == nil {
		return h.processTickerData(&data, msg.Ts)
	}
	
	// Try to parse as array
	var dataArray []BybitTickerData
	if err := json.Unmarshal(msg.Data, &dataArray); err == nil {
		for _, item := range dataArray {
			if err := h.processTickerData(&item, msg.Ts); err != nil {
				return err
			}
		}
		return nil
	}
	
	return fmt.Errorf("unable to parse Bybit ticker data")
}

// processTickerData processes individual ticker data
func (h *BybitHandler) processTickerData(data *BybitTickerData, ts int64) error {
	symbol := normalizeBybitSymbol(data.Symbol)
	
	// Get or create cached ticker
	h.cacheMu.Lock()
	cachedTicker, exists := h.tickerCache[symbol]
	if !exists {
		cachedTicker = &TickerData{
			Symbol: symbol,
		}
		h.tickerCache[symbol] = cachedTicker
	}
	
	// Update timestamp
	cachedTicker.Timestamp = time.Unix(0, ts*int64(time.Millisecond))
	
	// Merge new data with cached data (only update non-empty fields)
	if data.LastPrice != "" {
		cachedTicker.Last, _ = strconv.ParseFloat(data.LastPrice, 64)
	}
	if data.Bid1Price != "" {
		cachedTicker.Bid, _ = strconv.ParseFloat(data.Bid1Price, 64)
	}
	if data.Ask1Price != "" {
		cachedTicker.Ask, _ = strconv.ParseFloat(data.Ask1Price, 64)
	}
	if data.HighPrice24h != "" {
		cachedTicker.High, _ = strconv.ParseFloat(data.HighPrice24h, 64)
	}
	if data.LowPrice24h != "" {
		cachedTicker.Low, _ = strconv.ParseFloat(data.LowPrice24h, 64)
	}
	if data.Volume24h != "" {
		cachedTicker.Volume, _ = strconv.ParseFloat(data.Volume24h, 64)
	}
	
	// Create a copy for callback to avoid race conditions
	tickerCopy := *cachedTicker
	h.cacheMu.Unlock()

	// Call callback with the merged ticker data
	if h.callback != nil {
		h.callback(&tickerCopy)
	}

	return nil
}

// OnMessage sets the message callback
func (h *BybitHandler) OnMessage(callback func(*TickerData)) {
	h.callback = callback
}

// GetStatus returns the current status
func (h *BybitHandler) GetStatus() ExchangeStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()

	status := h.status
	status.MessageCount = atomic.LoadInt64(&h.messageCount)
	status.ErrorCount = atomic.LoadInt64(&h.errorCount)
	status.ReconnectCount = atomic.LoadInt64(&h.reconnectCount)
	return status
}

// keepAlive sends periodic pings to keep the connection alive
func (h *BybitHandler) keepAlive() {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		h.mu.RLock()
		conn := h.conn
		connected := h.status.Connected
		h.mu.RUnlock()

		if !connected || conn == nil {
			return
		}

		// Send ping message (Bybit format)
		pingMsg := map[string]interface{}{
			"op":       "ping",
			"req_id":   fmt.Sprintf("%d", time.Now().Unix()),
		}
		if err := conn.WriteJSON(pingMsg); err != nil {
			logger.WithExchange("bybit").Errorf("Failed to send ping: %v", err)
			return
		}
	}
}

// Subscribe adds a new symbol (not implemented for Bybit - requires reconnection)
func (h *BybitHandler) Subscribe(symbol string) {
	logger.WithSymbol("bybit", symbol).Warn("Dynamic subscription not supported - requires reconnection")
}

// Unsubscribe removes a symbol (not implemented for Bybit - requires reconnection)
func (h *BybitHandler) Unsubscribe(symbol string) {
	logger.WithSymbol("bybit", symbol).Warn("Dynamic unsubscription not supported - requires reconnection")
}

// GetSubscribedSymbols returns list of subscribed symbols
func (h *BybitHandler) GetSubscribedSymbols() []string {
	return h.config.Symbols
}

// normalizeBybitSymbol converts Bybit symbol format to standard format
func normalizeBybitSymbol(symbol string) string {
	// BTCUSDT -> BTC/USDT
	if len(symbol) > 4 && symbol[len(symbol)-4:] == "USDT" {
		return symbol[:len(symbol)-4] + "/USDT"
	}
	return symbol
}