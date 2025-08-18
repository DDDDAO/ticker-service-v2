package exchanges

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ddddao/ticker-service-v2/internal/logger"
	"github.com/ddddao/ticker-service-v2/internal/storage"
	"github.com/sirupsen/logrus"
)

// Manager manages all exchange WebSocket connections
type Manager struct {
	exchanges map[string]Handler
	storage   storage.Storage
	mu        sync.RWMutex
	wg        sync.WaitGroup
}

// NewManager creates a new exchange manager
func NewManager(storage storage.Storage) *Manager {
	return &Manager{
		exchanges: make(map[string]Handler),
		storage:   storage,
	}
}

// AddExchange adds a new exchange handler
func (m *Manager) AddExchange(name string, handler Handler) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.exchanges[name]; exists {
		return fmt.Errorf("exchange %s already exists", name)
	}

	m.exchanges[name] = handler
	return nil
}

// Start starts all exchange handlers
func (m *Manager) Start(ctx context.Context) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for name, handler := range m.exchanges {
		m.wg.Add(1)
		go func(n string, h Handler) {
			defer m.wg.Done()
			m.runExchange(ctx, n, h)
		}(name, handler)
	}
}

// runExchange runs a single exchange handler with automatic reconnection
func (m *Manager) runExchange(ctx context.Context, name string, handler Handler) {
	log := logger.WithExchange(name)
	backoff := time.Second
	maxBackoff := time.Minute

	for {
		select {
		case <-ctx.Done():
			log.Info("Stopping exchange handler")
			handler.Close()
			return
		default:
		}

		log.Info("Connecting to exchange WebSocket")
		if err := handler.Connect(); err != nil {
			log.Errorf("Failed to connect: %v", err)
			time.Sleep(backoff)
			
			// Exponential backoff
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}

		// Reset backoff on successful connection
		backoff = time.Second

		// Set message handler
		handler.OnMessage(func(data *TickerData) {
			m.publishTicker(name, data)
		})

		// Start handling messages
		err := handler.HandleMessages(ctx)
		if err != nil {
			log.Errorf("WebSocket error: %v", err)
		}

		log.Warn("WebSocket disconnected, reconnecting...")
		handler.Close()
	}
}

// publishTicker publishes ticker data to storage
func (m *Manager) publishTicker(exchange string, data *TickerData) {
	ctx := context.Background()
	
	// Calculate processing latency
	processingLatency := time.Since(data.Timestamp).Milliseconds()
	
	// Store ticker data
	storeStart := time.Now()
	if err := m.storage.Publish(ctx, exchange, data.Symbol, []byte(data.ToJSON())); err != nil {
		logrus.WithFields(logrus.Fields{
			"exchange": exchange,
			"symbol":   data.Symbol,
			"app":      "ticker-service-v2",
		}).Errorf("Failed to store ticker: %v", err)
		return
	}
	storeLatency := time.Since(storeStart).Milliseconds()

	// Log performance metrics at warn level only for actual high latency
	// Processing latency < 50ms is expected since we use receive time
	if processingLatency > 50 || storeLatency > 20 {
		logrus.WithFields(logrus.Fields{
			"exchange":              exchange,
			"symbol":                data.Symbol,
			"processing_latency_ms": processingLatency,
			"store_latency_ms":      storeLatency,
			"app":                   "ticker-service-v2",
		}).Warn("High latency detected")
	} else if logger.ShouldLogTickerUpdates() {
		// Only log ticker updates if enabled
		logrus.WithFields(logrus.Fields{
			"exchange":              exchange,
			"symbol":                data.Symbol,
			"price":                 data.Last,
			"volume":                data.Volume,
			"processing_latency_ms": processingLatency,
			"store_latency_ms":      storeLatency,
			"app":                   "ticker-service-v2",
		}).Debug("Ticker received")
	}
}

// GetStatus returns the status of all exchanges
func (m *Manager) GetStatus() map[string]ExchangeStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := make(map[string]ExchangeStatus)
	for name, handler := range m.exchanges {
		status[name] = handler.GetStatus()
	}
	return status
}

// Subscribe subscribes to a symbol on an exchange
func (m *Manager) Subscribe(exchange, symbol string) error {
	m.mu.RLock()
	handler, exists := m.exchanges[exchange]
	m.mu.RUnlock()
	
	if !exists {
		return fmt.Errorf("exchange %s not found", exchange)
	}
	
	handler.Subscribe(symbol)
	return nil
}

// Unsubscribe unsubscribes from a symbol on an exchange
func (m *Manager) Unsubscribe(exchange, symbol string) error {
	m.mu.RLock()
	handler, exists := m.exchanges[exchange]
	m.mu.RUnlock()
	
	if !exists {
		return fmt.Errorf("exchange %s not found", exchange)
	}
	
	handler.Unsubscribe(symbol)
	return nil
}

// Wait waits for all exchange handlers to finish
func (m *Manager) Wait() {
	m.wg.Wait()
}