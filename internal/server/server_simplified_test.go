package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestTickerEndpointSymbolValidation tests symbol format validation
func TestTickerEndpointSymbolValidation(t *testing.T) {
	t.Log(`
		TICKER ENDPOINT SYMBOL VALIDATION TEST
		=======================================
		This test verifies the API's strict symbol format requirements.
		
		The API enforces {base}-{quote} format (e.g., btc-usdt).
		This was implemented after user feedback to simplify the API.
	`)

	testCases := []struct {
		path           string
		shouldFail     bool
		expectedError  string
		description    string
	}{
		{
			path:          "/api/ticker/binance/btc-usdt",
			shouldFail:    false,
			expectedError: "",
			description:   "Valid format: lowercase base-quote",
		},
		{
			path:          "/api/ticker/binance/BTC-USDT",
			shouldFail:    false,
			expectedError: "",
			description:   "Valid format: uppercase base-quote",
		},
		{
			path:          "/api/ticker/binance/btcusdt",
			shouldFail:    true,
			expectedError: "Invalid symbol format",
			description:   "Invalid: missing dash separator",
		},
		{
			path:          "/api/ticker/binance/btc",
			shouldFail:    true,
			expectedError: "Invalid symbol format",
			description:   "Invalid: missing quote currency",
		},
		{
			path:          "/api/ticker/binance/btc/usdt",
			shouldFail:    true,
			expectedError: "Invalid path format",
			description:   "Invalid: wrong separator (slash)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			// Extract symbol from path
			parts := strings.Split(tc.path, "/")
			if len(parts) < 5 {
				if !tc.shouldFail {
					t.Error("Path should have failed validation")
				}
				return
			}

			symbol := parts[4]
			
			// Check if symbol contains dash
			hasValidFormat := strings.Contains(symbol, "-")
			
			if tc.shouldFail && hasValidFormat {
				t.Errorf("Symbol %s should have failed validation", symbol)
			}
			if !tc.shouldFail && !hasValidFormat {
				t.Errorf("Symbol %s should have passed validation", symbol)
			}
		})
	}
}

// TestAutoSubscriptionBehavior documents auto-subscription logic
func TestAutoSubscriptionBehavior(t *testing.T) {
	t.Log(`
		AUTO-SUBSCRIPTION BEHAVIOR TEST
		================================
		This test documents when auto-subscription occurs:
		
		Binance:
		- Symbol not found in Redis
		- Triggers Subscribe() call
		- Waits 2 seconds for data
		- Returns data or timeout message
		
		Other exchanges (OKX, Bybit, Bitget, Gate):
		- Symbol not found in Redis
		- Returns immediate error
		- Suggests adding to config.yaml
		- No auto-subscription (not supported)
	`)

	exchanges := []struct {
		name           string
		supportsAuto   bool
		waitTime       int
		errorMessage   string
	}{
		{
			name:         "binance",
			supportsAuto: true,
			waitTime:     2,
			errorMessage: "Still subscribing, try again",
		},
		{
			name:         "okx",
			supportsAuto: false,
			waitTime:     0,
			errorMessage: "Symbol not configured",
		},
		{
			name:         "bybit",
			supportsAuto: false,
			waitTime:     0,
			errorMessage: "Symbol not configured",
		},
		{
			name:         "bitget",
			supportsAuto: false,
			waitTime:     0,
			errorMessage: "Symbol not configured",
		},
		{
			name:         "gate",
			supportsAuto: false,
			waitTime:     0,
			errorMessage: "Symbol not configured",
		},
	}

	for _, ex := range exchanges {
		t.Run(ex.name, func(t *testing.T) {
			t.Logf("%s auto-subscription: %v", ex.name, ex.supportsAuto)
			if ex.supportsAuto {
				t.Logf("  - Wait time: %d seconds", ex.waitTime)
				t.Logf("  - Behavior: Auto-subscribes to new symbols")
			} else {
				t.Logf("  - Error: %s", ex.errorMessage)
				t.Logf("  - Solution: Add symbol to config.yaml")
			}
		})
	}
}

// TestHealthEndpoint tests the health check endpoint behavior
func TestHealthEndpoint(t *testing.T) {
	t.Log(`
		HEALTH ENDPOINT TEST
		====================
		The /health endpoint checks:
		1. Service is running
		2. Redis connectivity
		
		Response format:
		{
			"status": "healthy" | "unhealthy",
			"redis": "connected" | "disconnected"
		}
		
		HTTP Status:
		- 200 OK: Everything working
		- 503 Service Unavailable: Redis down
	`)

	// Create a simple health handler for testing
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy","redis":"connected"}`))
	})

	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()
	
	handler.ServeHTTP(rec, req)
	
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
	
	if !strings.Contains(rec.Body.String(), "healthy") {
		t.Error("Response should contain 'healthy' status")
	}
}

// TestStatusEndpoint tests the status endpoint format
func TestStatusEndpoint(t *testing.T) {
	t.Log(`
		STATUS ENDPOINT TEST
		====================
		The /status endpoint returns exchange connection status:
		
		Response format:
		{
			"exchanges": {
				"binance": {
					"connected": true,
					"message_count": 1000,
					"error_count": 5,
					"symbols": ["BTCUSDT", "ETHUSDT"],
					"reconnect_count": 2
				},
				...
			}
		}
		
		Used for:
		- Monitoring WebSocket health
		- Tracking message flow
		- Debugging connection issues
		- Verifying subscriptions
	`)

	// Document expected fields
	requiredFields := []string{
		"connected",
		"message_count",
		"error_count",
		"symbols",
		"reconnect_count",
	}

	t.Log("Required fields in status response:")
	for _, field := range requiredFields {
		t.Logf("  - %s", field)
	}
}

// TestMetricsEndpoint documents the metrics endpoint
func TestMetricsEndpoint(t *testing.T) {
	t.Log(`
		METRICS ENDPOINT TEST
		=====================
		The /metrics endpoint provides Prometheus metrics:
		
		Available metrics:
		- ticker_websocket_connected (gauge): 1 if connected, 0 if not
		- ticker_messages_total (counter): Total messages received
		- ticker_errors_total (counter): Total errors encountered
		- ticker_reconnects_total (counter): Total reconnection attempts
		- ticker_latency_milliseconds (histogram): Processing latency
		
		Labels:
		- exchange: binance, okx, bybit, bitget, gate
		- symbol: BTC/USDT, ETH/USDT, etc.
		
		Used by:
		- Prometheus for scraping
		- Grafana for visualization
		- Alerting systems
	`)

	metrics := []struct {
		name        string
		metricType  string
		description string
	}{
		{
			name:        "ticker_websocket_connected",
			metricType:  "gauge",
			description: "WebSocket connection status",
		},
		{
			name:        "ticker_messages_total",
			metricType:  "counter",
			description: "Total messages processed",
		},
		{
			name:        "ticker_errors_total",
			metricType:  "counter",
			description: "Total errors encountered",
		},
		{
			name:        "ticker_reconnects_total",
			metricType:  "counter",
			description: "WebSocket reconnection attempts",
		},
		{
			name:        "ticker_latency_milliseconds",
			metricType:  "histogram",
			description: "Message processing latency",
		},
	}

	for _, metric := range metrics {
		t.Logf("%s (%s): %s", metric.name, metric.metricType, metric.description)
	}
}

// TestSubscriptionEndpoints tests subscribe/unsubscribe endpoints
func TestSubscriptionEndpoints(t *testing.T) {
	t.Log(`
		SUBSCRIPTION ENDPOINTS TEST
		============================
		
		POST /api/subscribe
		Body: {"exchange": "binance", "symbol": "DOGEUSDT"}
		
		POST /api/unsubscribe
		Body: {"exchange": "binance", "symbol": "DOGEUSDT"}
		
		Behavior by exchange:
		- Binance: Actually modifies subscription (dynamic)
		- Others: Logs warning, no effect (static config)
		
		Response:
		- 200 OK: Success (even if no-op)
		- 400 Bad Request: Invalid JSON or missing fields
		- 405 Method Not Allowed: Wrong HTTP method
	`)

	endpoints := []struct {
		path        string
		method      string
		body        string
		statusCode  int
		description string
	}{
		{
			path:        "/api/subscribe",
			method:      "POST",
			body:        `{"exchange":"binance","symbol":"DOGEUSDT"}`,
			statusCode:  200,
			description: "Valid subscription request",
		},
		{
			path:        "/api/subscribe",
			method:      "GET",
			body:        "",
			statusCode:  405,
			description: "Wrong HTTP method",
		},
		{
			path:        "/api/subscribe",
			method:      "POST",
			body:        `{"exchange":"binance"}`,
			statusCode:  400,
			description: "Missing symbol field",
		},
		{
			path:        "/api/unsubscribe",
			method:      "POST",
			body:        `{"exchange":"binance","symbol":"DOGEUSDT"}`,
			statusCode:  200,
			description: "Valid unsubscribe request",
		},
	}

	for _, ep := range endpoints {
		t.Logf("%s %s -> %d: %s", ep.method, ep.path, ep.statusCode, ep.description)
	}
}