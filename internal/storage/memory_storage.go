package storage

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// MemoryStorage implements Storage interface using in-memory map
type MemoryStorage struct {
	data            map[string]map[string]*tickerEntry // exchange -> symbol -> ticker
	mu              sync.RWMutex
	ttl             time.Duration
	cleanupInterval time.Duration
	stopCleanup     chan bool
}

type tickerEntry struct {
	Data      []byte
	Timestamp time.Time
}

// NewMemoryStorage creates a new in-memory storage
func NewMemoryStorage() *MemoryStorage {
	logrus.WithField("app", "ticker-service-v2").Info("Using in-memory storage (Redis not configured)")

	m := &MemoryStorage{
		data:            make(map[string]map[string]*tickerEntry),
		ttl:             5 * time.Minute,  // Keep data for 5 minutes (Binance sends updates every second)
		cleanupInterval: 60 * time.Second, // Clean up every minute
		stopCleanup:     make(chan bool),
	}

	// Start cleanup goroutine
	go m.cleanupExpired()

	return m
}

// Publish stores ticker data in memory
func (m *MemoryStorage) Publish(ctx context.Context, exchange, symbol string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.data[exchange] == nil {
		m.data[exchange] = make(map[string]*tickerEntry)
	}

	m.data[exchange][symbol] = &tickerEntry{
		Data:      data,
		Timestamp: time.Now(),
	}

	return nil
}

// Get retrieves ticker data from memory
func (m *MemoryStorage) Get(ctx context.Context, exchange, symbol string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	exchangeData, exists := m.data[exchange]
	if !exists {
		return nil, fmt.Errorf("exchange %s not found", exchange)
	}

	entry, exists := exchangeData[symbol]
	if !exists {
		return nil, fmt.Errorf("ticker data not found for %s:%s", exchange, symbol)
	}

	// Return data even if stale - better than no data
	// The cleanup goroutine will remove very old data
	return entry.Data, nil
}

// GetAll retrieves all ticker data for an exchange
func (m *MemoryStorage) GetAll(ctx context.Context, exchange string) (map[string][]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	exchangeData, exists := m.data[exchange]
	if !exists {
		return make(map[string][]byte), nil
	}

	result := make(map[string][]byte)
	now := time.Now()

	for symbol, entry := range exchangeData {
		// Skip expired entries
		if now.Sub(entry.Timestamp) > m.ttl {
			continue
		}
		result[symbol] = entry.Data
	}

	return result, nil
}

// Close stops the cleanup goroutine
func (m *MemoryStorage) Close() error {
	close(m.stopCleanup)
	return nil
}

// IsConnected always returns true for memory storage
func (m *MemoryStorage) IsConnected() bool {
	return true
}

// cleanupExpired removes expired entries periodically
func (m *MemoryStorage) cleanupExpired() {
	ticker := time.NewTicker(m.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.removeExpired()
		case <-m.stopCleanup:
			return
		}
	}
}

// removeExpired removes expired entries from memory
func (m *MemoryStorage) removeExpired() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	removedCount := 0

	for exchange, exchangeData := range m.data {
		for symbol, entry := range exchangeData {
			if now.Sub(entry.Timestamp) > m.ttl {
				delete(exchangeData, symbol)
				removedCount++
			}
		}

		// Remove exchange if no symbols left
		if len(exchangeData) == 0 {
			delete(m.data, exchange)
		}
	}

	if removedCount > 0 {
		logrus.Debugf("Cleaned up %d expired ticker entries", removedCount)
	}
}

// GetStats returns storage statistics (for debugging)
func (m *MemoryStorage) GetStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	totalEntries := 0
	exchangeStats := make(map[string]int)

	for exchange, exchangeData := range m.data {
		count := len(exchangeData)
		exchangeStats[exchange] = count
		totalEntries += count
	}

	return map[string]interface{}{
		"type":          "memory",
		"total_entries": totalEntries,
		"exchanges":     exchangeStats,
		"ttl_seconds":   m.ttl.Seconds(),
	}
}

// Subscribe is a no-op for memory storage (no pub/sub support)
func (m *MemoryStorage) Subscribe(ctx context.Context, pattern string) (<-chan string, error) {
	// Memory storage doesn't support pub/sub
	return nil, fmt.Errorf("pub/sub not supported in memory storage")
}
