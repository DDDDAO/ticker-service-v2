package exchanges

import (
	"fmt"
	"strings"

	"github.com/ddddao/ticker-service-v2/internal/config"
)

// NewHandler creates a new exchange handler based on the exchange name
func NewHandler(name string, cfg config.ExchangeConfig) (Handler, error) {
	switch strings.ToLower(name) {
	case "binance":
		return NewBinanceHandler(cfg), nil
	case "okx":
		return NewOKXHandler(cfg), nil
	case "bybit":
		return NewBybitHandler(cfg), nil
	case "bitget":
		return NewBitgetHandler(cfg), nil
	case "gate":
		return NewGateHandler(cfg), nil
	default:
		return nil, fmt.Errorf("unsupported exchange: %s", name)
	}
}