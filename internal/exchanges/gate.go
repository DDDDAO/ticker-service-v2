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

// GateHandler handles Gate.io WebSocket connections
type GateHandler struct {
	config         config.ExchangeConfig
	conn           *websocket.Conn
	callback       func(*TickerData)
	status         ExchangeStatus
	mu             sync.RWMutex
	messageCount   int64
	errorCount     int64
	reconnectCount int64
	requestID      int64
}

// GateTickerMessage represents Gate.io ticker WebSocket message
type GateTickerMessage struct {
	Time    int64  `json:"time"`
	TimeMs  int64  `json:"time_ms"`
	Channel string `json:"channel"`
	Event   string `json:"event"`
	Result  struct {
		CurrencyPair     string `json:"currency_pair"`
		Last             string `json:"last"`
		LowestAsk        string `json:"lowest_ask"`
		HighestBid       string `json:"highest_bid"`
		ChangePercentage string `json:"change_percentage"`
		BaseVolume       string `json:"base_volume"`
		QuoteVolume      string `json:"quote_volume"`
		High24h          string `json:"high_24h"`
		Low24h           string `json:"low_24h"`
	} `json:"result"`
}

// NewGateHandler creates a new Gate.io WebSocket handler
func NewGateHandler(cfg config.ExchangeConfig) *GateHandler {
	return &GateHandler{
		config: cfg,
		status: ExchangeStatus{
			Symbols: cfg.Symbols,
		},
		requestID: 1,
	}
}

// Connect establishes WebSocket connection to Gate.io
func (h *GateHandler) Connect() error {
	log := logger.WithExchange("gate")
	
	// Gate.io requires the endpoint to be spot specific
	wsURL := h.config.WSURL
	if !strings.Contains(wsURL, "/ws/v4/") {
		wsURL = strings.TrimSuffix(wsURL, "/") + "/"
	}
	wsURL = wsURL + "spot"
	
	log.Infof("Connecting to %s", wsURL)

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
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

// subscribe sends subscription message to Gate.io
func (h *GateHandler) subscribe() error {
	for _, symbol := range h.config.Symbols {
		subscribeMsg := map[string]interface{}{
			"time":    time.Now().Unix(),
			"id":      atomic.AddInt64(&h.requestID, 1),
			"channel": "spot.tickers",
			"event":   "subscribe",
			"payload": []string{symbol},
		}

		h.mu.RLock()
		conn := h.conn
		h.mu.RUnlock()

		if conn == nil {
			return fmt.Errorf("connection is nil")
		}

		if err := conn.WriteJSON(subscribeMsg); err != nil {
			return err
		}
	}

	return nil
}

// Close closes the WebSocket connection
func (h *GateHandler) Close() error {
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
func (h *GateHandler) HandleMessages(ctx context.Context) error {
	log := logger.WithExchange("gate")

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
func (h *GateHandler) processMessage(data []byte) error {
	// Check if it's a pong or subscription response
	var response struct {
		Time   int64  `json:"time"`
		Event  string `json:"event"`
		Error  struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		Result struct {
			Status string `json:"status"`
		} `json:"result"`
		Channel string `json:"channel"`
	}
	
	if err := json.Unmarshal(data, &response); err == nil {
		if response.Event == "subscribe" && response.Result.Status == "success" {
			return nil // Subscription confirmed
		}
		if response.Channel == "spot.pong" {
			return nil // Pong response
		}
		if response.Error.Code != 0 {
			return fmt.Errorf("gate error: %s", response.Error.Message)
		}
	}

	// Parse ticker message
	var msg GateTickerMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return fmt.Errorf("failed to parse message: %w", err)
	}

	// Process ticker data
	if msg.Channel == "spot.tickers" && msg.Event == "update" {
		return h.processTicker(&msg)
	}

	return nil
}

// processTicker processes a ticker message
func (h *GateHandler) processTicker(msg *GateTickerMessage) error {
	// Convert to normalized ticker data
	ticker := &TickerData{
		Symbol:    normalizeGateSymbol(msg.Result.CurrencyPair),
		Timestamp: time.Unix(msg.Time, msg.TimeMs*int64(time.Millisecond)),
	}

	// Parse prices
	ticker.Last, _ = strconv.ParseFloat(msg.Result.Last, 64)
	ticker.Bid, _ = strconv.ParseFloat(msg.Result.HighestBid, 64)
	ticker.Ask, _ = strconv.ParseFloat(msg.Result.LowestAsk, 64)
	ticker.High, _ = strconv.ParseFloat(msg.Result.High24h, 64)
	ticker.Low, _ = strconv.ParseFloat(msg.Result.Low24h, 64)
	ticker.Volume, _ = strconv.ParseFloat(msg.Result.BaseVolume, 64)

	// Call callback if set
	if h.callback != nil {
		h.callback(ticker)
	}

	return nil
}

// OnMessage sets the message callback
func (h *GateHandler) OnMessage(callback func(*TickerData)) {
	h.callback = callback
}

// GetStatus returns the current status
func (h *GateHandler) GetStatus() ExchangeStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()

	status := h.status
	status.MessageCount = atomic.LoadInt64(&h.messageCount)
	status.ErrorCount = atomic.LoadInt64(&h.errorCount)
	status.ReconnectCount = atomic.LoadInt64(&h.reconnectCount)
	return status
}

// keepAlive sends periodic pings to keep the connection alive
func (h *GateHandler) keepAlive() {
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

		// Send ping message (Gate.io format)
		pingMsg := map[string]interface{}{
			"time":    time.Now().Unix(),
			"id":      atomic.AddInt64(&h.requestID, 1),
			"channel": "spot.ping",
		}
		if err := conn.WriteJSON(pingMsg); err != nil {
			logger.WithExchange("gate").Errorf("Failed to send ping: %v", err)
			return
		}
	}
}

// normalizeGateSymbol converts Gate.io symbol format to standard format
func normalizeGateSymbol(symbol string) string {
	// BTC_USDT -> BTC/USDT
	return strings.ReplaceAll(symbol, "_", "/")
}