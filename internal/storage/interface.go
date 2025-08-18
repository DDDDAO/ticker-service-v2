package storage

import (
	"context"
	"time"
)

// Storage defines the interface for ticker data storage
type Storage interface {
	// Publish stores ticker data for a given exchange and symbol
	Publish(ctx context.Context, exchange, symbol string, data []byte) error
	
	// Get retrieves the latest ticker data for a given exchange and symbol
	Get(ctx context.Context, exchange, symbol string) ([]byte, error)
	
	// GetAll retrieves all ticker data for a given exchange
	GetAll(ctx context.Context, exchange string) (map[string][]byte, error)
	
	// Close closes the storage connection
	Close() error
	
	// IsConnected returns true if storage is available
	IsConnected() bool
}

// TickerData represents parsed ticker data
type TickerData struct {
	Symbol    string    `json:"symbol"`
	Last      float64   `json:"last"`
	Bid       float64   `json:"bid"`
	Ask       float64   `json:"ask"`
	High      float64   `json:"high"`
	Low       float64   `json:"low"`
	Volume    float64   `json:"volume"`
	Timestamp time.Time `json:"timestamp"`
}