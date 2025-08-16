package exchanges

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ddddao/ticker-service-v2/internal/logger"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
)

// Manager manages all exchange WebSocket connections
type Manager struct {
	exchanges map[string]Handler
	redis     *redis.Client
	mu        sync.RWMutex
	wg        sync.WaitGroup
}

// NewManager creates a new exchange manager
func NewManager(redisClient *redis.Client) *Manager {
	return &Manager{
		exchanges: make(map[string]Handler),
		redis:     redisClient,
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

// publishTicker publishes ticker data to Redis
func (m *Manager) publishTicker(exchange string, data *TickerData) {
	ctx := context.Background()
	channel := fmt.Sprintf("ticker:%s:%s", exchange, data.Symbol)
	
	// Publish to Redis channel
	if err := m.redis.Publish(ctx, channel, data.ToJSON()).Err(); err != nil {
		logrus.WithFields(logrus.Fields{
			"exchange": exchange,
			"symbol":   data.Symbol,
			"app":      "ticker-service-v2",
		}).Errorf("Failed to publish ticker: %v", err)
		return
	}

	// Also store latest ticker in Redis with TTL
	key := fmt.Sprintf("ticker:latest:%s:%s", exchange, data.Symbol)
	if err := m.redis.Set(ctx, key, data.ToJSON(), 30*time.Second).Err(); err != nil {
		logrus.WithFields(logrus.Fields{
			"exchange": exchange,
			"symbol":   data.Symbol,
			"app":      "ticker-service-v2",
		}).Errorf("Failed to store ticker: %v", err)
	}

	// Only log ticker updates if enabled
	if logger.ShouldLogTickerUpdates() {
		// Log with latency info
		latency := time.Since(data.Timestamp).Milliseconds()
		logrus.WithFields(logrus.Fields{
			"exchange": exchange,
			"symbol":   data.Symbol,
			"price":    data.Last,
			"volume":   data.Volume,
			"latency":  latency,
			"app":      "ticker-service-v2",
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

// Wait waits for all exchange handlers to finish
func (m *Manager) Wait() {
	m.wg.Wait()
}