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

// BinanceHandler handles Binance WebSocket connections
type BinanceHandler struct {
	config         config.ExchangeConfig
	conn           *websocket.Conn
	callback       func(*TickerData)
	status         ExchangeStatus
	mu             sync.RWMutex
	messageCount   int64
	errorCount     int64
	reconnectCount int64
	subscribedSymbols map[string]bool // Track which symbols we want to receive
	symbolsMu         sync.RWMutex
}

// BinanceMiniTickerMessage represents Binance mini ticker WebSocket message
// Using !miniTicker@arr stream for all symbols at once
type BinanceMiniTickerMessage struct {
	EventType string      `json:"e"` // Event type (24hrMiniTicker)
	EventTime int64       `json:"E"` // Event time
	Symbol    string      `json:"s"` // Symbol
	LastPrice interface{} `json:"c"` // Close price (last price)
	OpenPrice interface{} `json:"o"` // Open price
	High      interface{} `json:"h"` // High price
	Low       interface{} `json:"l"` // Low price
	Volume    interface{} `json:"v"` // Total traded base asset volume
	QuoteVol  interface{} `json:"q"` // Total traded quote asset volume
}

// BinanceTickerMessage represents old format for compatibility
type BinanceTickerMessage struct {
	EventType          string      `json:"e"` // Event type
	EventTime          int64       `json:"E"` // Event time
	Symbol             string      `json:"s"` // Symbol
	PriceChange        interface{} `json:"p"` // Price change
	PriceChangePercent interface{} `json:"P"` // Price change percent
	WeightedAvgPrice   interface{} `json:"w"` // Weighted average price
	PrevClosePrice     interface{} `json:"x"` // Previous day's close price
	LastPrice          interface{} `json:"c"` // Current day's close price (last price)
	CloseQty           interface{} `json:"Q"` // Close trade's quantity
	BidPrice           interface{} `json:"b"` // Best bid price
	BidQty             interface{} `json:"B"` // Best bid quantity
	AskPrice           interface{} `json:"a"` // Best ask price
	AskQty             interface{} `json:"A"` // Best ask quantity
	OpenPrice          interface{} `json:"o"` // Open price
	High               interface{} `json:"h"` // High price
	Low                interface{} `json:"l"` // Low price
	Volume             interface{} `json:"v"` // Total traded base asset volume
	QuoteVol           interface{} `json:"q"` // Total traded quote asset volume
	OpenTime           int64       `json:"O"` // Statistics open time
	CloseTime          int64       `json:"C"` // Statistics close time
	FirstID            int64       `json:"F"` // First trade ID
	LastID             int64       `json:"L"` // Last trade ID
	Count              int64       `json:"n"` // Total number of trades
}

// NewBinanceHandler creates a new Binance WebSocket handler
func NewBinanceHandler(cfg config.ExchangeConfig) *BinanceHandler {
	// Convert symbol list to a map for faster lookups
	subscribed := make(map[string]bool)
	for _, symbol := range cfg.Symbols {
		// Convert format: "btcusdt@ticker" -> "BTCUSDT"
		if idx := strings.Index(symbol, "@"); idx > 0 {
			subscribed[strings.ToUpper(symbol[:idx])] = true
		} else {
			subscribed[strings.ToUpper(symbol)] = true
		}
	}
	
	return &BinanceHandler{
		config:            cfg,
		subscribedSymbols: subscribed,
		status: ExchangeStatus{
			Symbols: cfg.Symbols,
		},
	}
}

// Connect establishes WebSocket connection to Binance
func (h *BinanceHandler) Connect() error {
	// Use !miniTicker@arr stream for all symbols
	url := fmt.Sprintf("%s/ws/!miniTicker@arr", h.config.WSURL)

	log := logger.WithExchange("binance")
	log.Infof("Connecting to %s", url)

	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	h.mu.Lock()
	h.conn = conn
	h.status.Connected = true
	h.status.LastMessage = time.Now()
	h.mu.Unlock()

	// Send ping periodically to keep connection alive
	go h.keepAlive()

	atomic.AddInt64(&h.reconnectCount, 1)
	log.Info("Connected successfully")
	return nil
}

// Close closes the WebSocket connection
func (h *BinanceHandler) Close() error {
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
func (h *BinanceHandler) HandleMessages(ctx context.Context) error {
	log := logger.WithExchange("binance")

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
func (h *BinanceHandler) processMessage(data []byte) error {
	// !miniTicker@arr sends an array of mini tickers
	var tickers []BinanceMiniTickerMessage
	if err := json.Unmarshal(data, &tickers); err != nil {
		// Try parsing as single message for backward compatibility
		var msg BinanceTickerMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return fmt.Errorf("failed to parse message: %w", err)
		}
		return h.processTicker(&msg)
	}

	// Process each ticker in the array
	processedCount := 0
	for _, ticker := range tickers {
		// Only process symbols we're interested in
		h.symbolsMu.RLock()
		_, shouldProcess := h.subscribedSymbols[ticker.Symbol]
		h.symbolsMu.RUnlock()
		
		if shouldProcess {
			if err := h.processMiniTicker(&ticker); err != nil {
				logger.WithSymbol("binance", ticker.Symbol).Errorf("Failed to process ticker: %v", err)
			}
			processedCount++
		}
	}
	
	if logger.ShouldLogTickerUpdates() {
		logger.WithExchange("binance").Debugf("Processed %d/%d tickers", processedCount, len(tickers))
	}

	return nil
}

// processMiniTicker processes a mini ticker message
func (h *BinanceHandler) processMiniTicker(msg *BinanceMiniTickerMessage) error {
	if msg.EventType != "24hrMiniTicker" && msg.EventType != "" {
		return nil // Skip non-ticker messages
	}

	// Convert to normalized ticker data
	ticker := &TickerData{
		Symbol:    normalizeBinanceSymbol(msg.Symbol),
		Timestamp: time.Unix(0, msg.EventTime*int64(time.Millisecond)),
	}

	// Parse prices using helper function
	ticker.Last = parseFloat(msg.LastPrice)
	ticker.High = parseFloat(msg.High)
	ticker.Low = parseFloat(msg.Low)
	ticker.Volume = parseFloat(msg.Volume)
	
	// Mini ticker doesn't have bid/ask, set to 0
	ticker.Bid = 0
	ticker.Ask = 0

	// Call callback if set
	if h.callback != nil {
		h.callback(ticker)
	}

	return nil
}

// processTicker processes a full ticker message (backward compatibility)
func (h *BinanceHandler) processTicker(msg *BinanceTickerMessage) error {
	if msg.EventType != "24hrTicker" && msg.EventType != "" {
		return nil // Skip non-ticker messages
	}

	// Convert to normalized ticker data
	ticker := &TickerData{
		Symbol:    normalizeSymbol(msg.Symbol),
		Timestamp: time.Unix(0, msg.EventTime*int64(time.Millisecond)),
	}

	// Parse prices - handle both string and number formats
	ticker.Last = parseFloat(msg.LastPrice)
	ticker.Bid = parseFloat(msg.BidPrice)
	ticker.Ask = parseFloat(msg.AskPrice)
	ticker.High = parseFloat(msg.High)
	ticker.Low = parseFloat(msg.Low)
	ticker.Volume = parseFloat(msg.Volume)

	// Call callback if set
	if h.callback != nil {
		h.callback(ticker)
	}

	return nil
}

// OnMessage sets the message callback
func (h *BinanceHandler) OnMessage(callback func(*TickerData)) {
	h.callback = callback
}

// Subscribe adds a new symbol to track
func (h *BinanceHandler) Subscribe(symbol string) {
	h.symbolsMu.Lock()
	defer h.symbolsMu.Unlock()
	
	// Normalize symbol to uppercase
	normalizedSymbol := strings.ToUpper(symbol)
	h.subscribedSymbols[normalizedSymbol] = true
	
	logger.WithSymbol("binance", symbol).Info("Subscribed to symbol")
}

// Unsubscribe removes a symbol from tracking
func (h *BinanceHandler) Unsubscribe(symbol string) {
	h.symbolsMu.Lock()
	defer h.symbolsMu.Unlock()
	
	// Normalize symbol to uppercase
	normalizedSymbol := strings.ToUpper(symbol)
	delete(h.subscribedSymbols, normalizedSymbol)
	
	logger.WithSymbol("binance", symbol).Info("Unsubscribed from symbol")
}

// GetSubscribedSymbols returns list of subscribed symbols
func (h *BinanceHandler) GetSubscribedSymbols() []string {
	h.symbolsMu.RLock()
	defer h.symbolsMu.RUnlock()
	
	symbols := make([]string, 0, len(h.subscribedSymbols))
	for symbol := range h.subscribedSymbols {
		symbols = append(symbols, symbol)
	}
	return symbols
}

// GetStatus returns the current status
func (h *BinanceHandler) GetStatus() ExchangeStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()

	status := h.status
	status.MessageCount = atomic.LoadInt64(&h.messageCount)
	status.ErrorCount = atomic.LoadInt64(&h.errorCount)
	status.ReconnectCount = atomic.LoadInt64(&h.reconnectCount)
	
	// Update symbols list with current subscriptions
	status.Symbols = h.GetSubscribedSymbols()
	
	return status
}

// normalizeBinanceSymbol converts Binance symbol format to standard format
func normalizeBinanceSymbol(symbol string) string {
	// BTCUSDT -> BTC/USDT
	if len(symbol) > 4 && symbol[len(symbol)-4:] == "USDT" {
		return symbol[:len(symbol)-4] + "/USDT"
	}
	return symbol
}

// keepAlive sends periodic pings to keep the connection alive
func (h *BinanceHandler) keepAlive() {
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

		if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
			logger.WithExchange("binance").Errorf("Failed to send ping: %v", err)
			return
		}
	}
}

// normalizeSymbol converts Binance symbol format to standard format
func normalizeSymbol(symbol string) string {
	// Convert BTCUSDT to BTC/USDT
	if strings.HasSuffix(symbol, "USDT") {
		base := strings.TrimSuffix(symbol, "USDT")
		return fmt.Sprintf("%s/USDT", base)
	}
	return symbol
}

// parseFloat converts interface{} to float64, handling both string and number formats
func parseFloat(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case string:
		var f float64
		fmt.Sscanf(val, "%f", &f)
		return f
	default:
		return 0
	}
}
