# Ticker Service V2

High-performance WebSocket ticker service for cryptocurrency exchanges written in Go.

## Features

- Concurrent WebSocket connections to multiple exchanges
- Automatic reconnection with exponential backoff
- Redis pub/sub for data distribution
- Structured JSON logging for Google Cloud
- Low-latency ticker updates
- Health checks and monitoring endpoints

## Architecture

The service uses Go's goroutines to manage concurrent WebSocket connections to different exchanges. Each exchange handler runs in its own goroutine and publishes ticker updates to Redis channels.

## Supported Exchanges

- Binance
- OKX
- Bybit
- Bitget
- Gate.io

## Quick Start

```bash
# Install dependencies
go mod download

# Run locally
go run cmd/ticker/main.go

# Build
go build -o ticker-service cmd/ticker/main.go

# Run with config
./ticker-service -config config.yaml
```

## Configuration

Configuration can be provided via environment variables or config file:

```yaml
server:
  port: 8080

redis:
  addr: localhost:6379
  password: ""
  db: 0

exchanges:
  binance:
    enabled: true
    symbols:
      - BTC/USDT
      - ETH/USDT
  okx:
    enabled: true
    symbols:
      - BTC-USDT
      - ETH-USDT

logging:
  level: info
  format: json
```

## API Endpoints

### Core Endpoints

- `GET /health` - Health check
- `GET /metrics` - Prometheus metrics
- `GET /status` - WebSocket connection status
- `GET /api/ticker/:exchange/:symbol` - Get latest ticker
  - Symbol format: `{base}-{quote}` (e.g., `btc-usdt`, `eth-usdt`)
  - Auto-subscribes to new symbols (2-second delay for first request)
- `POST /api/subscribe` - Subscribe to a symbol
  - Body: `{"exchange": "binance", "symbol": "BTCUSDT"}`
- `POST /api/unsubscribe` - Unsubscribe from a symbol
  - Body: `{"exchange": "binance", "symbol": "BTCUSDT"}`

### Symbol Format

The ticker API uses a standardized format: `{base}-{quote}`

Examples:
- `btc-usdt` - Bitcoin against USDT
- `eth-usdt` - Ethereum against USDT
- `bnb-usdt` - Binance Coin against USDT

Currently supported quote currencies:
- `usdt` - Tether USD

Future support planned for:
- `usdc` - USD Coin
- `busd` - Binance USD

## Performance

- Handles 1000+ concurrent WebSocket connections
- Sub-millisecond internal processing latency
- Automatic connection pooling and reuse
- Memory-efficient ticker caching
