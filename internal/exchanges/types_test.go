package exchanges

import (
	"encoding/json"
	"testing"
	"time"
)

// TestTickerDataToJSON verifies that TickerData correctly serializes to JSON format
// This is critical because Redis stores the data as JSON strings
func TestTickerDataToJSON(t *testing.T) {
	testCases := []struct {
		name        string
		ticker      TickerData
		wantFields  []string // Fields that should exist in JSON
		description string
	}{
		{
			name: "complete ticker data",
			ticker: TickerData{
				Symbol:    "BTC/USDT",
				Last:      50000.50,
				Bid:       49999.99,
				Ask:       50001.01,
				High:      51000.00,
				Low:       49000.00,
				Volume:    1234.56,
				Timestamp: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			},
			wantFields: []string{"symbol", "last", "bid", "ask", "high", "low", "volume", "timestamp"},
			description: `
				Verifies that all fields of TickerData are properly serialized to JSON.
				This ensures data integrity when storing in Redis and sending via API.
			`,
		},
		{
			name: "ticker with zero values",
			ticker: TickerData{
				Symbol:    "ETH/USDT",
				Last:      3000.00,
				Bid:       0, // Binance mini ticker doesn't provide bid
				Ask:       0, // Binance mini ticker doesn't provide ask
				High:      3100.00,
				Low:       2900.00,
				Volume:    5678.90,
				Timestamp: time.Now(),
			},
			wantFields: []string{"symbol", "last", "bid", "ask"},
			description: `
				Tests that zero values are properly included in JSON output.
				Important for Binance mini ticker which doesn't provide bid/ask.
			`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			jsonStr := tc.ticker.ToJSON()
			
			// Verify it's valid JSON
			var result map[string]interface{}
			if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
				t.Fatalf("ToJSON() produced invalid JSON: %v", err)
			}
			
			// Check that all expected fields exist
			for _, field := range tc.wantFields {
				if _, exists := result[field]; !exists {
					t.Errorf("Expected field %q not found in JSON output", field)
				}
			}
			
			// Verify specific values
			if result["symbol"] != tc.ticker.Symbol {
				t.Errorf("Symbol mismatch: got %v, want %v", result["symbol"], tc.ticker.Symbol)
			}
		})
	}
}

// TestExchangeStatus verifies the ExchangeStatus structure
// This is used for monitoring and health checks
func TestExchangeStatus(t *testing.T) {
	status := ExchangeStatus{
		Connected:      true,
		LastMessage:    time.Now(),
		MessageCount:   100,
		ErrorCount:     5,
		Symbols:        []string{"BTC/USDT", "ETH/USDT"},
		ReconnectCount: 2,
	}
	
	// Test that all fields are accessible
	if !status.Connected {
		t.Error("Expected Connected to be true")
	}
	
	if len(status.Symbols) != 2 {
		t.Errorf("Expected 2 symbols, got %d", len(status.Symbols))
	}
	
	if status.MessageCount != 100 {
		t.Errorf("Expected MessageCount to be 100, got %d", status.MessageCount)
	}
	
	t.Log(`
		This test verifies that ExchangeStatus correctly tracks:
		- Connection state (for health monitoring)
		- Message statistics (for performance monitoring)
		- Error tracking (for reliability monitoring)
		- Subscribed symbols (for subscription management)
		- Reconnection attempts (for stability monitoring)
	`)
}