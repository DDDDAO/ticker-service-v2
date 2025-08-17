package exchanges

import (
	"strings"
)

// SymbolConverter handles formatting conversions between config format and exchange WebSocket format
// It does NOT change symbol names - 1000pepe-usdt stays 1000pepe-usdt
type SymbolConverter struct{}

// NewSymbolConverter creates a new symbol converter
func NewSymbolConverter() *SymbolConverter {
	return &SymbolConverter{}
}

// ConfigToWebSocket converts from config format to WebSocket subscription format
// Examples:
// - Binance: btc-usdt → BTCUSDT, 1000pepe-usdt → 1000PEPEUSDT
// - OKX: btc-usdt → BTC-USDT
// - Bybit: btc-usdt → BTCUSDT, shib1000-usdt → SHIB1000USDT
// - Gate: btc-usdt → BTC_USDT
func (sc *SymbolConverter) ConfigToWebSocket(exchange, symbol string) string {
	switch exchange {
	case "binance":
		// btc-usdt → BTCUSDT
		// 1000pepe-usdt → 1000PEPEUSDT
		return strings.ToUpper(strings.ReplaceAll(symbol, "-", ""))
		
	case "okx":
		// btc-usdt → BTC-USDT
		return strings.ToUpper(symbol)
		
	case "bybit":
		// btc-usdt → BTCUSDT
		// shib1000-usdt → SHIB1000USDT
		return strings.ToUpper(strings.ReplaceAll(symbol, "-", ""))
		
	case "bitget":
		// btc-usdt → BTCUSDT
		return strings.ToUpper(strings.ReplaceAll(symbol, "-", ""))
		
	case "gate":
		// btc-usdt → BTC_USDT
		return strings.ToUpper(strings.ReplaceAll(symbol, "-", "_"))
		
	default:
		return strings.ToUpper(symbol)
	}
}

// WebSocketToStorage converts from WebSocket format back to storage format (lowercase-dash)
// Examples:
// - BTCUSDT → btc-usdt
// - 1000PEPEUSDT → 1000pepe-usdt
// - SHIB1000USDT → shib1000-usdt
// - BTC_USDT → btc-usdt
func (sc *SymbolConverter) WebSocketToStorage(exchange, wsSymbol string) string {
	storageSymbol := strings.ToLower(wsSymbol)
	
	switch exchange {
	case "binance", "bybit", "bitget":
		// BTCUSDT → btc-usdt
		// 1000PEPEUSDT → 1000pepe-usdt
		// SHIB1000USDT → shib1000-usdt
		if strings.HasSuffix(storageSymbol, "usdt") {
			base := storageSymbol[:len(storageSymbol)-4]
			return base + "-usdt"
		}
		
	case "okx":
		// BTC-USDT → btc-usdt (already has dash)
		return storageSymbol
		
	case "gate":
		// BTC_USDT → btc-usdt
		return strings.ReplaceAll(storageSymbol, "_", "-")
	}

	return storageSymbol
}