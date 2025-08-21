package storage

import (
	"context"
	"fmt"
	"hash/fnv"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

const numShards = 32 // Number of shards to reduce lock contention

// MemoryStorageOptimized implements Storage interface with sharded locks
type MemoryStorageOptimized struct {
	shards          [numShards]*memoryShard
	ttl             time.Duration
	cleanupInterval time.Duration
	stopCleanup     chan bool
}

type memoryShard struct {
	mu   sync.RWMutex
	data map[string]map[string]*tickerEntry // exchange -> symbol -> ticker
}

// NewMemoryStorageOptimized creates a new optimized in-memory storage
func NewMemoryStorageOptimized() *MemoryStorageOptimized {
	logrus.WithField("app", "ticker-service-v2").Info("Using optimized in-memory storage with sharding")

	m := &MemoryStorageOptimized{
		ttl:             5 * time.Minute,
		cleanupInterval: 60 * time.Second,
		stopCleanup:     make(chan bool),
	}

	// Initialize shards
	for i := 0; i < numShards; i++ {
		m.shards[i] = &memoryShard{
			data: make(map[string]map[string]*tickerEntry),
		}
	}

	// Start cleanup goroutine
	go m.cleanupExpired()

	return m
}

// getShard returns the shard for a given exchange-symbol pair
func (m *MemoryStorageOptimized) getShard(exchange, symbol string) *memoryShard {
	h := fnv.New32a()
	h.Write([]byte(exchange + ":" + symbol))
	return m.shards[h.Sum32()%numShards]
}

// Publish stores ticker data in memory with minimal lock contention
func (m *MemoryStorageOptimized) Publish(ctx context.Context, exchange, symbol string, data []byte) error {
	shard := m.getShard(exchange, symbol)
	
	shard.mu.Lock()
	defer shard.mu.Unlock()

	if shard.data[exchange] == nil {
		shard.data[exchange] = make(map[string]*tickerEntry)
	}

	shard.data[exchange][symbol] = &tickerEntry{
		Data:      data,
		Timestamp: time.Now(),
	}

	return nil
}

// Get retrieves ticker data from memory
func (m *MemoryStorageOptimized) Get(ctx context.Context, exchange, symbol string) ([]byte, error) {
	shard := m.getShard(exchange, symbol)
	
	shard.mu.RLock()
	defer shard.mu.RUnlock()

	exchangeData, exists := shard.data[exchange]
	if !exists {
		return nil, fmt.Errorf("exchange %s not found", exchange)
	}

	entry, exists := exchangeData[symbol]
	if !exists {
		return nil, fmt.Errorf("ticker data not found for %s:%s", exchange, symbol)
	}

	return entry.Data, nil
}

// GetAll retrieves all ticker data for an exchange
func (m *MemoryStorageOptimized) GetAll(ctx context.Context, exchange string) (map[string][]byte, error) {
	result := make(map[string][]byte)
	now := time.Now()

	// Collect from all shards
	for i := 0; i < numShards; i++ {
		shard := m.shards[i]
		shard.mu.RLock()
		
		if exchangeData, exists := shard.data[exchange]; exists {
			for symbol, entry := range exchangeData {
				// Skip expired entries
				if now.Sub(entry.Timestamp) <= m.ttl {
					result[symbol] = entry.Data
				}
			}
		}
		
		shard.mu.RUnlock()
	}

	return result, nil
}

// Close stops the cleanup goroutine
func (m *MemoryStorageOptimized) Close() error {
	close(m.stopCleanup)
	return nil
}

// IsConnected always returns true for memory storage
func (m *MemoryStorageOptimized) IsConnected() bool {
	return true
}

// cleanupExpired removes expired entries periodically
func (m *MemoryStorageOptimized) cleanupExpired() {
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

// removeExpired removes expired entries from all shards
func (m *MemoryStorageOptimized) removeExpired() {
	now := time.Now()
	totalRemoved := 0

	for i := 0; i < numShards; i++ {
		shard := m.shards[i]
		shard.mu.Lock()
		
		for exchange, exchangeData := range shard.data {
			for symbol, entry := range exchangeData {
				if now.Sub(entry.Timestamp) > m.ttl {
					delete(exchangeData, symbol)
					totalRemoved++
				}
			}
			
			// Remove exchange if no symbols left
			if len(exchangeData) == 0 {
				delete(shard.data, exchange)
			}
		}
		
		shard.mu.Unlock()
	}

	if totalRemoved > 0 {
		logrus.Debugf("Cleaned up %d expired ticker entries", totalRemoved)
	}
}

// GetStats returns storage statistics
func (m *MemoryStorageOptimized) GetStats() map[string]interface{} {
	totalEntries := 0
	exchangeStats := make(map[string]int)

	for i := 0; i < numShards; i++ {
		shard := m.shards[i]
		shard.mu.RLock()
		
		for exchange, exchangeData := range shard.data {
			count := len(exchangeData)
			exchangeStats[exchange] += count
			totalEntries += count
		}
		
		shard.mu.RUnlock()
	}

	return map[string]interface{}{
		"type":          "memory_optimized",
		"total_entries": totalEntries,
		"exchanges":     exchangeStats,
		"ttl_seconds":   m.ttl.Seconds(),
		"num_shards":    numShards,
	}
}

// Subscribe is a no-op for memory storage
func (m *MemoryStorageOptimized) Subscribe(ctx context.Context, pattern string) (<-chan string, error) {
	return nil, fmt.Errorf("pub/sub not supported in memory storage")
}