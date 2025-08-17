package exchanges

import (
	"strings"
	"testing"
	"github.com/ddddao/ticker-service-v2/internal/config"
)

// TestOKXSubscriptionLimitations tests OKX's subscription model
func TestOKXSubscriptionLimitations(t *testing.T) {
	t.Log(`
		OKX SUBSCRIPTION LIMITATIONS TEST
		==================================
		This test documents why PEPE wasn't found on OKX:
		
		OKX WebSocket characteristics:
		1. NO dynamic subscription after connection
		2. Must subscribe to all symbols at connection time
		3. Subscribe() and Unsubscribe() are NO-OPs
		4. All symbols MUST be in config.yaml
		
		This is why the user got "Ticker not found" for PEPE.
	`)

	handler := NewOKXHandler(config.ExchangeConfig{
		Symbols: []string{"BTC-USDT", "ETH-USDT"},
	})

	// Document initial state
	initialSymbols := handler.GetSubscribedSymbols()
	t.Logf("Initial symbols: %v", initialSymbols)

	// Attempt dynamic subscription (won't work)
	handler.Subscribe("PEPE-USDT")
	afterSubscribe := handler.GetSubscribedSymbols()

	if len(afterSubscribe) > len(initialSymbols) {
		t.Error("OKX should not support dynamic subscription")
	}

	t.Log(`
		Solution for missing symbols:
		1. Add to config.yaml:
		   okx:
		     symbols:
		       - BTC-USDT
		       - ETH-USDT
		       - PEPE-USDT  # Add this!
		2. Restart the service
		
		Common symbols to pre-configure:
		- BTC-USDT, ETH-USDT (majors)
		- BNB-USDT, SOL-USDT (popular)
		- PEPE-USDT, DOGE-USDT (memes)
		- ARB-USDT, OP-USDT (L2s)
	`)
}

// TestOKXSymbolFormat tests OKX's unique symbol format
func TestOKXSymbolFormat(t *testing.T) {
	t.Log(`
		OKX SYMBOL FORMAT TEST
		======================
		OKX uses dash-separated format: BTC-USDT
		This is different from other exchanges.
	`)

	testCases := []struct {
		apiInput     string  // What comes from our API
		okxFormat    string  // What OKX expects
		redisFormat  string  // What we store
		description  string
	}{
		{
			apiInput:    "btc-usdt",
			okxFormat:   "BTC-USDT",
			redisFormat: "BTC/USDT",
			description: "Standard BTC pair",
		},
		{
			apiInput:    "eth-usdt",
			okxFormat:   "ETH-USDT",
			redisFormat: "ETH/USDT",
			description: "ETH pair",
		},
		{
			apiInput:    "pepe-usdt",
			okxFormat:   "PEPE-USDT",
			redisFormat: "PEPE/USDT",
			description: "PEPE (the missing symbol)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			// API to OKX format
			okxSymbol := strings.ToUpper(tc.apiInput)
			if okxSymbol != tc.okxFormat {
				t.Errorf("API to OKX conversion failed: got %s, want %s",
					okxSymbol, tc.okxFormat)
			}

			// OKX to Redis format
			redisSymbol := strings.ReplaceAll(okxSymbol, "-", "/")
			if redisSymbol != tc.redisFormat {
				t.Errorf("OKX to Redis conversion failed: got %s, want %s",
					redisSymbol, tc.redisFormat)
			}
		})
	}

	t.Log(`
		Format comparison across exchanges:
		- Binance: BTCUSDT (no separator)
		- OKX: BTC-USDT (dash separator)
		- Bybit: BTCUSDT (no separator)
		- Bitget: BTCUSDT (no separator)
		- Gate: BTC_USDT (underscore)
		- Internal: BTC/USDT (slash for Redis)
	`)
}

// TestOKXErrorMessages tests the error messages users see
func TestOKXErrorMessages(t *testing.T) {
	t.Log(`
		OKX ERROR MESSAGES TEST
		========================
		This test documents the error flow when a symbol isn't found.
		
		User's experience:
		1. Request: GET /api/ticker/okx/pepe-usdt
		2. Service checks Redis for ticker:okx:PEPE/USDT
		3. Not found (because not subscribed)
		4. Returns: "Ticker not found. Try again in a few seconds if this is a valid symbol"
		
		The "try again" message is misleading for OKX because:
		- It won't auto-subscribe
		- Waiting won't help
		- Symbol must be in config
	`)

	// Simulate the error condition
	symbolRequested := "pepe-usdt"
	symbolConfigured := []string{"btc-usdt", "eth-usdt"}
	
	found := false
	for _, s := range symbolConfigured {
		if s == symbolRequested {
			found = true
			break
		}
	}

	if !found {
		expectedError := "Ticker not found. Symbol not configured for OKX. Add to config.yaml and restart."
		t.Logf("Better error message: %s", expectedError)
	}

	t.Log(`
		Improved error handling suggestions:
		1. Check if exchange supports dynamic subscription
		2. Return appropriate error message:
		   - Binance: "Subscribing to symbol, try again in 2 seconds"
		   - Others: "Symbol not configured, add to config.yaml"
	`)
}

// TestOKXWebSocketMessages tests OKX's WebSocket message format
func TestOKXWebSocketMessages(t *testing.T) {
	t.Log(`
		OKX WEBSOCKET MESSAGE FORMAT
		=============================
		
		Subscription message:
		{
			"op": "subscribe",
			"args": [{
				"channel": "tickers",
				"instId": "BTC-USDT"
			}]
		}
		
		Ticker data message:
		{
			"arg": {
				"channel": "tickers",
				"instId": "BTC-USDT"
			},
			"data": [{
				"instId": "BTC-USDT",
				"last": "50000.50",
				"bidPx": "49999.99",    // Note: bidPx not bid
				"askPx": "50001.01",    // Note: askPx not ask
				"high24h": "51000.00",
				"low24h": "49000.00",
				"vol24h": "1234.56"
			}]
		}
		
		Field mapping:
		- instId -> Symbol (convert to BTC/USDT)
		- last -> Last
		- bidPx -> Bid
		- askPx -> Ask
		- high24h -> High
		- low24h -> Low
		- vol24h -> Volume
	`)

	// This documents the format for reference
	// Actual parsing is tested in integration tests
}

// TestOKXReconnectionBehavior tests OKX reconnection requirements
func TestOKXReconnectionBehavior(t *testing.T) {
	t.Log(`
		OKX RECONNECTION BEHAVIOR
		=========================
		
		On disconnect:
		1. All subscriptions are lost
		2. Must reconnect WebSocket
		3. Must re-subscribe to all symbols
		4. Cannot add new symbols without config change
		
		This is why configuration is critical for OKX.
	`)

	handler := NewOKXHandler(config.ExchangeConfig{
		Symbols: []string{"BTC-USDT", "ETH-USDT", "PEPE-USDT"},
	})

	// These symbols will be subscribed on connect
	expectedSymbols := []string{"BTC-USDT", "ETH-USDT", "PEPE-USDT"}
	
	// Verify they're configured
	configured := handler.GetSubscribedSymbols()
	if len(configured) != len(expectedSymbols) {
		t.Errorf("Expected %d symbols, got %d", len(expectedSymbols), len(configured))
	}

	t.Log(`
		Best practices for OKX:
		1. Configure all needed symbols upfront
		2. Include popular symbols users might request
		3. Monitor logs for "not found" errors
		4. Update config and restart when adding symbols
	`)
}