# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository Overview

This is a high-performance WebSocket ticker service for cryptocurrency exchanges written in Go. It provides real-time ticker data from multiple exchanges through concurrent WebSocket connections with Redis pub/sub for data distribution.

## Architecture

### Core Components

1. **Exchange Handlers** (`internal/exchanges/`)
   - Interface-based design with `Handler` interface for all exchange implementations
   - Each exchange runs in its own goroutine for concurrent processing
   - Automatic reconnection with exponential backoff
   - Normalized ticker data structure across all exchanges

2. **Manager** (`internal/exchanges/manager.go`)
   - Orchestrates multiple exchange connections
   - Lifecycle management for all handlers
   - Aggregates status from all exchanges

3. **Server** (`internal/server/`)
   - HTTP API endpoints for health checks and ticker retrieval
   - Prometheus metrics endpoint
   - WebSocket connection status monitoring

4. **Redis Integration** (`internal/redis/`)
   - Pub/sub pattern for ticker data distribution
   - Channel format: `ticker:{exchange}:{symbol}`
   - Persistent connection with automatic reconnection

5. **Configuration** (`internal/config/`)
   - Viper-based configuration management
   - Environment variable support
   - YAML config file support

## Development Commands

### Building and Running

```bash
# Install dependencies
go mod download
go mod tidy

# Build the binary
make build
# or directly:
go build -o ticker-service cmd/ticker/main.go

# Run locally with config file
make run
# or directly:
go run cmd/ticker/main.go -config config.yaml

# Development mode with live reload (requires air)
make dev
```

### Testing

```bash
# Run all tests
make test
# or directly:
go test -v ./...

# Run tests with coverage
make test-coverage
# or directly:
go test -v -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Code Quality

```bash
# Format code
make fmt
# or directly:
go fmt ./...
goimports -w .

# Run linter (requires golangci-lint)
make lint
# or directly:
golangci-lint run
```

### Docker Operations

```bash
# Build Docker image
make docker-build
# or directly:
docker build -t ticker-service-v2:latest .

# Run Docker container
make docker-run
# or directly:
docker run -d --name ticker-service -p 8080:8080 ticker-service-v2:latest
```

### Development Tools Installation

```bash
# Install all development tools
make install-tools
```

## Configuration Structure

The service uses a hierarchical configuration system:

- **Server Configuration**: Port settings
- **Redis Configuration**: Connection details (addr, password, db)
- **Logging Configuration**: Level and format (json/text)
- **Exchange Configuration**: Per-exchange settings including:
  - `enabled`: Toggle exchange on/off
  - `ws_url`: WebSocket endpoint
  - `symbols`: List of symbols to subscribe

Configuration can be provided via:
1. `config.yaml` file (default)
2. Environment variables (prefix: `TICKER_`)
3. Command-line flags

## Exchange Integration Pattern

When adding new exchanges:

1. Create new handler in `internal/exchanges/` implementing the `Handler` interface
2. Register in `internal/exchanges/factory.go`
3. Add configuration section in `config.yaml`
4. Implement WebSocket message parsing specific to the exchange
5. Normalize data to `TickerData` structure

## API Endpoints

- `GET /health` - Service health check with Redis connectivity
- `GET /status` - WebSocket connection status for all exchanges
- `GET /metrics` - Prometheus metrics
- `GET /api/ticker/:exchange/:symbol` - Retrieve latest ticker from Redis

## Redis Channel Format

Ticker data is published to Redis channels with the format:
```
ticker:{exchange}:{symbol}
```

Example: `ticker:binance:btcusdt`

## Error Handling Patterns

- Exponential backoff for reconnection (2s, 4s, 8s... up to 60s)
- Structured logging with logrus for debugging
- Graceful shutdown with context cancellation
- Panic recovery in goroutines

## Performance Considerations

- Concurrent WebSocket connections using goroutines
- Non-blocking Redis publish operations
- Memory-efficient ticker caching
- Connection pooling and reuse
- Sub-millisecond internal processing latency

## Testing Approach

When writing tests:
1. Use table-driven tests for multiple scenarios
2. Mock external dependencies (Redis, WebSocket)
3. Test error conditions and edge cases
4. Verify goroutine cleanup in tests
5. Use context with timeout for integration tests