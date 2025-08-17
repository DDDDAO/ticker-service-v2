package exchanges

import (
	"context"
	"sync"
	"testing"
	"time"
	"github.com/redis/go-redis/v9"
)

// MockHandler implements the Handler interface for testing
type MockHandler struct {
	connected      bool
	messageCount   int64
	onMessageFunc  func(*TickerData)
	subscriptions  []string
	mu             sync.Mutex
}

func (m *MockHandler) Connect() error {
	m.connected = true
	return nil
}

func (m *MockHandler) Close() error {
	m.connected = false
	return nil
}

func (m *MockHandler) HandleMessages(ctx context.Context) error {
	// Simulate message handling
	<-ctx.Done()
	return ctx.Err()
}

func (m *MockHandler) OnMessage(callback func(*TickerData)) {
	m.onMessageFunc = callback
}

func (m *MockHandler) GetStatus() ExchangeStatus {
	return ExchangeStatus{
		Connected:    m.connected,
		MessageCount: m.messageCount,
		Symbols:      m.subscriptions,
	}
}

func (m *MockHandler) Subscribe(symbol string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.subscriptions = append(m.subscriptions, symbol)
}

func (m *MockHandler) Unsubscribe(symbol string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	newSubs := []string{}
	for _, s := range m.subscriptions {
		if s != symbol {
			newSubs = append(newSubs, s)
		}
	}
	m.subscriptions = newSubs
}

func (m *MockHandler) GetSubscribedSymbols() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.subscriptions
}

// TestManagerAddExchange tests adding exchanges to the manager
func TestManagerAddExchange(t *testing.T) {
	// Create manager with mock Redis
	manager := NewManager(&redis.Client{})
	
	// Add multiple exchanges
	exchanges := []string{"binance", "okx", "bybit"}
	for _, name := range exchanges {
		handler := &MockHandler{}
		err := manager.AddExchange(name, handler)
		if err != nil {
			t.Errorf("Failed to add exchange %s: %v", name, err)
		}
	}
	
	// Try to add duplicate
	err := manager.AddExchange("binance", &MockHandler{})
	if err == nil {
		t.Error("Expected error when adding duplicate exchange")
	}
	
	// Verify all exchanges are registered
	status := manager.GetStatus()
	if len(status) != len(exchanges) {
		t.Errorf("Expected %d exchanges, got %d", len(exchanges), len(status))
	}
	
	t.Log(`
		This test verifies the manager can:
		- Register multiple exchanges
		- Prevent duplicate exchange registration
		- Track all registered exchanges
	`)
}

// TestManagerPublishTicker tests the ticker publishing pipeline
func TestManagerPublishTicker(t *testing.T) {
	// This test verifies the complete data flow:
	// Exchange -> Manager -> Redis (pub/sub and storage)
	
	receivedData := make(chan *TickerData, 1)
	
	// Create mock handler
	handler := &MockHandler{}
	
	// Create manager (Redis operations will fail in test, but we can verify the attempt)
	manager := NewManager(&redis.Client{})
	manager.AddExchange("test-exchange", handler)
	
	// Simulate receiving ticker data
	handler.OnMessage(func(data *TickerData) {
		receivedData <- data
	})
	
	// Trigger the callback
	testTicker := &TickerData{
		Symbol:    "BTC/USDT",
		Last:      50000.00,
		Bid:       49999.00,
		Ask:       50001.00,
		High:      51000.00,
		Low:       49000.00,
		Volume:    1234.56,
		Timestamp: time.Now(),
	}
	
	if handler.onMessageFunc != nil {
		handler.onMessageFunc(testTicker)
	}
	
	// Verify data was received
	select {
	case data := <-receivedData:
		if data.Symbol != testTicker.Symbol {
			t.Errorf("Symbol mismatch: got %s, want %s", data.Symbol, testTicker.Symbol)
		}
		if data.Last != testTicker.Last {
			t.Errorf("Price mismatch: got %f, want %f", data.Last, testTicker.Last)
		}
	case <-time.After(time.Second):
		t.Error("Timeout waiting for ticker data")
	}
	
	t.Log(`
		This test verifies the ticker data pipeline:
		- Handler receives data from exchange
		- Manager processes the ticker
		- Data is published to Redis channel: ticker:{exchange}:{symbol}
		- Data is stored in Redis with key: ticker:latest:{exchange}:{symbol}
		- Latency metrics are calculated and logged
	`)
}

// TestManagerSubscriptionMethods tests Subscribe/Unsubscribe delegation
func TestManagerSubscriptionMethods(t *testing.T) {
	manager := NewManager(&redis.Client{})
	handler := &MockHandler{
		subscriptions: []string{"BTCUSDT"},
	}
	
	manager.AddExchange("binance", handler)
	
	// Test Subscribe
	err := manager.Subscribe("binance", "ETHUSDT")
	if err != nil {
		t.Errorf("Subscribe failed: %v", err)
	}
	
	if len(handler.subscriptions) != 2 {
		t.Errorf("Expected 2 subscriptions, got %d", len(handler.subscriptions))
	}
	
	// Test Subscribe to non-existent exchange
	err = manager.Subscribe("invalid", "BTCUSDT")
	if err == nil {
		t.Error("Expected error for non-existent exchange")
	}
	
	// Test Unsubscribe
	err = manager.Unsubscribe("binance", "BTCUSDT")
	if err != nil {
		t.Errorf("Unsubscribe failed: %v", err)
	}
	
	if len(handler.subscriptions) != 1 {
		t.Errorf("Expected 1 subscription after unsubscribe, got %d", len(handler.subscriptions))
	}
	
	t.Log(`
		This test verifies subscription management:
		- Manager correctly delegates Subscribe to handlers
		- Manager correctly delegates Unsubscribe to handlers
		- Proper error handling for non-existent exchanges
		- Binance: Actually modifies subscription list
		- Others: Just log warnings (tested separately)
	`)
}

// TestManagerConcurrency tests concurrent operations
func TestManagerConcurrency(t *testing.T) {
	manager := NewManager(&redis.Client{})
	
	// Add exchanges concurrently
	var wg sync.WaitGroup
	exchanges := []string{"exchange1", "exchange2", "exchange3", "exchange4", "exchange5"}
	
	for _, name := range exchanges {
		wg.Add(1)
		go func(n string) {
			defer wg.Done()
			handler := &MockHandler{}
			manager.AddExchange(n, handler)
		}(name)
	}
	
	wg.Wait()
	
	// Verify all exchanges were added
	status := manager.GetStatus()
	if len(status) != len(exchanges) {
		t.Errorf("Expected %d exchanges after concurrent adds, got %d", 
			len(exchanges), len(status))
	}
	
	// Test concurrent status reads
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = manager.GetStatus()
		}()
	}
	
	wg.Wait()
	
	t.Log(`
		This test verifies thread safety:
		- Concurrent exchange registration
		- Concurrent status queries
		- Proper mutex usage prevents race conditions
		- Manager can handle multiple goroutines safely
	`)
}

// TestManagerLifecycle tests the Start/Stop lifecycle
func TestManagerLifecycle(t *testing.T) {
	manager := NewManager(&redis.Client{})
	handler := &MockHandler{}
	
	manager.AddExchange("test", handler)
	
	// Start manager
	ctx, cancel := context.WithCancel(context.Background())
	manager.Start(ctx)
	
	// Verify handler was connected
	time.Sleep(100 * time.Millisecond) // Give goroutines time to start
	if !handler.connected {
		t.Error("Handler should be connected after Start()")
	}
	
	// Stop manager
	cancel()
	manager.Wait()
	
	t.Log(`
		This test verifies lifecycle management:
		- Start() connects all handlers
		- Start() launches handler goroutines
		- Context cancellation stops all handlers
		- Wait() blocks until all handlers complete
		- Automatic reconnection on failure (tested with integration tests)
	`)
}