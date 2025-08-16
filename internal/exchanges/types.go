package exchanges

import (
	"context"
	"encoding/json"
	"time"
)

// TickerData represents normalized ticker data
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

// ToJSON converts ticker data to JSON string
func (t *TickerData) ToJSON() string {
	data, _ := json.Marshal(t)
	return string(data)
}

// ExchangeStatus represents the status of an exchange connection
type ExchangeStatus struct {
	Connected      bool      `json:"connected"`
	LastMessage    time.Time `json:"last_message"`
	MessageCount   int64     `json:"message_count"`
	ErrorCount     int64     `json:"error_count"`
	Symbols        []string  `json:"symbols"`
	ReconnectCount int64     `json:"reconnect_count"`
}

// Handler defines the interface for exchange WebSocket handlers
type Handler interface {
	Connect() error
	Close() error
	HandleMessages(ctx context.Context) error
	OnMessage(callback func(*TickerData))
	GetStatus() ExchangeStatus
	Subscribe(symbol string)
	Unsubscribe(symbol string)
	GetSubscribedSymbols() []string
}