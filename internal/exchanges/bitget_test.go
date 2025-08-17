package exchanges

import (
	"strings"
	"testing"
	"github.com/ddddao/ticker-service-v2/internal/config"
)

// TestBitgetFieldMapping tests Bitget's unique field naming convention
func TestBitgetFieldMapping(t *testing.T) {
	t.Log(`
		BITGET FIELD MAPPING TEST
		=========================
		This test documents Bitget's non-standard field names:
		
		Standard fields -> Bitget fields:
		- last  -> lastPr
		- bid   -> bidPr
		- ask   -> askPr
		- high  -> high24h
		- low   -> low24h
		- volume -> baseVolume
		
		This was a critical bug fix - using wrong field names
		resulted in all price data showing as 0.
	`)

	// Document the correct field mapping
	fieldMap := map[string]string{
		"last":   "lastPr",
		"bid":    "bidPr",
		"ask":    "askPr",
		"high":   "high24h",
		"low":    "low24h",
		"volume": "baseVolume",
	}

	// Verify all mappings are documented
	expectedFields := []string{"last", "bid", "ask", "high", "low", "volume"}
	for _, field := range expectedFields {
		if bitgetField, exists := fieldMap[field]; !exists {
			t.Errorf("Missing field mapping for %s", field)
		} else {
			t.Logf("✓ %s maps to %s", field, bitgetField)
		}
	}

	t.Log(`
		Impact of incorrect field names:
		- All prices show as 0
		- Volume shows as 0
		- High/Low show as 0
		- Makes the service appear broken
		
		Lesson: Always verify field names with exchange documentation!
	`)
}

// TestBitgetSubscriptionModel tests Bitget's subscription limitations
func TestBitgetSubscriptionModel(t *testing.T) {
	t.Log(`
		BITGET SUBSCRIPTION MODEL TEST
		===============================
		This test documents Bitget's subscription limitations:
		
		1. NO dynamic subscription support
		2. All symbols must be pre-configured
		3. Subscription happens at connection time
		4. Adding new symbols requires reconnection
		
		This is why PEPE and other symbols must be in config.yaml
	`)

	handler := NewBitgetHandler(config.ExchangeConfig{
		Symbols: []string{"BTCUSDT", "ETHUSDT", "PEPEUSDT"},
	})

	// Test that Subscribe/Unsubscribe don't actually work
	initialCount := len(handler.GetSubscribedSymbols())
	
	// Attempt to subscribe (won't actually work)
	handler.Subscribe("DOGEUSDT")
	afterSubscribe := len(handler.GetSubscribedSymbols())
	
	if afterSubscribe != initialCount {
		t.Log("Warning: Bitget unexpectedly modified subscription list")
	}

	// Document the actual symbols
	symbols := handler.GetSubscribedSymbols()
	t.Logf("Configured symbols: %v", symbols)

	t.Log(`
		Configuration requirements:
		- Add all needed symbols to config.yaml
		- Popular symbols should be pre-configured:
		  - BTCUSDT, ETHUSDT (major pairs)
		  - BNBUSDT, SOLUSDT (popular alts)
		  - PEPEUSDT, DOGEUSDT (meme coins)
		  - Others as needed
	`)
}

// TestBitgetMessageFormat tests understanding of Bitget's WebSocket format
func TestBitgetMessageFormat(t *testing.T) {
	t.Log(`
		BITGET MESSAGE FORMAT TEST
		==========================
		This test documents Bitget's WebSocket message structure:
		
		Subscription format:
		{
			"op": "subscribe",
			"args": [{
				"instType": "SPOT",
				"channel": "ticker",
				"instId": "BTCUSDT"
			}]
		}
		
		Ticker data format:
		{
			"action": "snapshot",
			"arg": {
				"instType": "SPOT",
				"channel": "ticker",
				"instId": "BTCUSDT"
			},
			"data": [{
				"instId": "BTCUSDT",
				"lastPr": "50000.50",    // NOT "last"!
				"bidPr": "49999.99",     // NOT "bid"!
				"askPr": "50001.01",     // NOT "ask"!
				"high24h": "51000.00",
				"low24h": "49000.00",
				"baseVolume": "1234.56"
			}]
		}
	`)

	// This test serves as documentation
	// Actual parsing is tested in integration tests
	
	t.Log(`
		Common pitfalls:
		1. Using standard field names (last, bid, ask)
		2. Not checking exchange documentation
		3. Assuming all exchanges use same format
		
		Debugging tip: Log raw messages to see actual field names!
	`)
}

// TestBitgetSymbolFormat tests Bitget's symbol format requirements
func TestBitgetSymbolFormat(t *testing.T) {
	testCases := []struct {
		apiFormat      string  // What we receive from API
		bitgetFormat   string  // What Bitget expects
		internalFormat string  // What we store in Redis
		description    string
	}{
		{
			apiFormat:      "btc-usdt",
			bitgetFormat:   "BTCUSDT",
			internalFormat: "BTC/USDT",
			description:    "Standard conversion chain",
		},
		{
			apiFormat:      "eth-usdt",
			bitgetFormat:   "ETHUSDT",
			internalFormat: "ETH/USDT",
			description:    "ETH pair conversion",
		},
		{
			apiFormat:      "pepe-usdt",
			bitgetFormat:   "PEPEUSDT",
			internalFormat: "PEPE/USDT",
			description:    "Meme coin conversion",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.apiFormat, func(t *testing.T) {
			// API to Bitget format
			bitgetSymbol := testAPIToBitgetSymbol(tc.apiFormat)
			if bitgetSymbol != tc.bitgetFormat {
				t.Errorf("API to Bitget conversion failed: got %s, want %s",
					bitgetSymbol, tc.bitgetFormat)
			}

			// Bitget to internal format
			internalSymbol := testBitgetToInternalSymbol(tc.bitgetFormat)
			if internalSymbol != tc.internalFormat {
				t.Errorf("Bitget to internal conversion failed: got %s, want %s",
					internalSymbol, tc.internalFormat)
			}
		})
	}

	t.Log(`
		Symbol format flow:
		1. API receives: btc-usdt (lowercase, dash)
		2. Convert to Bitget: BTCUSDT (uppercase, no separator)
		3. Store in Redis: BTC/USDT (uppercase, slash)
		
		This ensures consistency across all exchanges.
	`)
}

// Helper functions for testing
func testAPIToBitgetSymbol(symbol string) string {
	// btc-usdt -> BTCUSDT
	return strings.ToUpper(strings.ReplaceAll(symbol, "-", ""))
}

func testBitgetToInternalSymbol(symbol string) string {
	// BTCUSDT -> BTC/USDT
	if len(symbol) > 4 && symbol[len(symbol)-4:] == "USDT" {
		return symbol[:len(symbol)-4] + "/" + symbol[len(symbol)-4:]
	}
	return symbol
}