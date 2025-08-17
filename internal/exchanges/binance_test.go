package exchanges

import (
	"testing"
	"github.com/ddddao/ticker-service-v2/internal/config"
)

// TestNewBinanceHandler verifies that the Binance handler correctly initializes
// with proper symbol parsing and subscription list creation
func TestNewBinanceHandler(t *testing.T) {
	testCases := []struct {
		name           string
		configSymbols  []string
		wantSubscribed map[string]bool
		description    string
	}{
		{
			name: "symbols with @ticker suffix",
			configSymbols: []string{
				"btcusdt@ticker",
				"ethusdt@ticker",
				"bnbusdt@ticker",
			},
			wantSubscribed: map[string]bool{
				"BTCUSDT": true,
				"ETHUSDT": true,
				"BNBUSDT": true,
			},
			description: `
				Tests that the handler correctly strips @ticker suffix from config symbols.
				This is important for backward compatibility with old config format.
			`,
		},
		{
			name: "symbols without suffix",
			configSymbols: []string{
				"BTCUSDT",
				"ETHUSDT",
			},
			wantSubscribed: map[string]bool{
				"BTCUSDT": true,
				"ETHUSDT": true,
			},
			description: `
				Tests that plain symbols (without @ticker) are correctly uppercased.
				This supports the simplified configuration format.
			`,
		},
		{
			name: "mixed case symbols",
			configSymbols: []string{
				"btcusdt",
				"EthUsdt",
				"BNBusdt@ticker",
			},
			wantSubscribed: map[string]bool{
				"BTCUSDT": true,
				"ETHUSDT": true,
				"BNBUSDT": true,
			},
			description: `
				Verifies case-insensitive handling of symbols.
				Users might provide symbols in various cases, all should be normalized to uppercase.
			`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.ExchangeConfig{
				Symbols: tc.configSymbols,
			}
			
			handler := NewBinanceHandler(cfg)
			
			// Check that subscribed symbols match expected
			if len(handler.subscribedSymbols) != len(tc.wantSubscribed) {
				t.Errorf("Subscribed symbols count mismatch: got %d, want %d",
					len(handler.subscribedSymbols), len(tc.wantSubscribed))
			}
			
			for symbol, want := range tc.wantSubscribed {
				if got := handler.subscribedSymbols[symbol]; got != want {
					t.Errorf("Symbol %s subscription mismatch: got %v, want %v",
						symbol, got, want)
				}
			}
		})
	}
}

// TestBinanceSubscribeUnsubscribe tests dynamic subscription management
// This is unique to Binance which supports the !miniTicker@arr stream
func TestBinanceSubscribeUnsubscribe(t *testing.T) {
	handler := NewBinanceHandler(config.ExchangeConfig{
		Symbols: []string{"btcusdt@ticker"},
	})
	
	// Initial state - only BTCUSDT should be subscribed
	if !handler.subscribedSymbols["BTCUSDT"] {
		t.Error("BTCUSDT should be initially subscribed")
	}
	
	// Test subscribing to a new symbol
	handler.Subscribe("ETHUSDT")
	if !handler.subscribedSymbols["ETHUSDT"] {
		t.Error("ETHUSDT should be subscribed after Subscribe()")
	}
	
	// Test unsubscribing
	handler.Unsubscribe("BTCUSDT")
	if handler.subscribedSymbols["BTCUSDT"] {
		t.Error("BTCUSDT should be unsubscribed after Unsubscribe()")
	}
	
	// Test GetSubscribedSymbols
	symbols := handler.GetSubscribedSymbols()
	if len(symbols) != 1 || symbols[0] != "ETHUSDT" {
		t.Errorf("GetSubscribedSymbols() returned unexpected result: %v", symbols)
	}
	
	t.Log(`
		This test verifies Binance's unique dynamic subscription capability:
		- Subscribe() adds symbols to the filter without reconnecting
		- Unsubscribe() removes symbols from the filter
		- The handler continues receiving all symbols via !miniTicker@arr
		- Only processes symbols in the subscription list
	`)
}

// TestNormalizeBinanceSymbol tests symbol format conversion
func TestNormalizeBinanceSymbol(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
		description string
	}{
		{
			input:    "BTCUSDT",
			expected: "BTC/USDT",
			description: "Standard USDT pair normalization",
		},
		{
			input:    "ETHUSDT",
			expected: "ETH/USDT",
			description: "ETH pair normalization",
		},
		{
			input:    "BTCBUSD",
			expected: "BTCBUSD", // Doesn't end with USDT, no change
			description: "Non-USDT pairs are left unchanged",
		},
		{
			input:    "BNB",
			expected: "BNB",
			description: "Short symbols without quote currency are unchanged",
		},
		{
			input:    "SOLUSDT",
			expected: "SOL/USDT",
			description: "3-letter base currency normalization",
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := normalizeBinanceSymbol(tc.input)
			if result != tc.expected {
				t.Errorf("normalizeBinanceSymbol(%s) = %s, want %s",
					tc.input, result, tc.expected)
			}
		})
	}
	
	t.Log(`
		This test ensures correct symbol format conversion:
		- Binance format: BTCUSDT (no separator)
		- Internal format: BTC/USDT (with slash separator)
		- This normalization is critical for Redis key consistency
	`)
}

// TestParseFloat tests the helper function for parsing Binance's mixed-type fields
func TestParseFloat(t *testing.T) {
	testCases := []struct {
		name     string
		input    interface{}
		expected float64
		description string
	}{
		{
			name:     "string number",
			input:    "12345.67",
			expected: 12345.67,
			description: "Binance often sends numbers as strings",
		},
		{
			name:     "float64 number",
			input:    float64(12345.67),
			expected: 12345.67,
			description: "Sometimes Binance sends actual numbers",
		},
		{
			name:     "integer",
			input:    int(12345),
			expected: 12345.0,
			description: "Integer values should convert to float",
		},
		{
			name:     "invalid string",
			input:    "not-a-number",
			expected: 0,
			description: "Invalid strings should return 0",
		},
		{
			name:     "nil value",
			input:    nil,
			expected: 0,
			description: "Nil values should return 0",
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := parseFloat(tc.input)
			if result != tc.expected {
				t.Errorf("parseFloat(%v) = %f, want %f",
					tc.input, result, tc.expected)
			}
		})
	}
	
	t.Log(`
		This test verifies the parseFloat helper function which handles:
		- Binance's inconsistent JSON types (string vs number)
		- This was a critical bug fix for parsing ticker data
		- The function must handle strings, floats, ints, and nil gracefully
	`)
}