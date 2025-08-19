# gRPC Setup for Ticker Service

This document describes the gRPC implementation for ultra-low latency ticker data access.

## Performance Improvements

- **HTTP latency**: 200-1000ms (with DNS overhead)
- **gRPC latency**: 5-50ms (with persistent connections)
- **With caching**: <1ms for repeated calls within 1 second

## Architecture

### Server Side (Go)
- gRPC server running on port 50051
- Protocol buffers for efficient binary serialization
- Keep-alive connections to avoid reconnection overhead
- Concurrent request handling with goroutines

### Client Side (TypeScript)
- gRPC client with automatic reconnection
- 100ms timeout for ultra-low latency
- 1-second TTL cache to avoid repeated calls
- Batch operations support

## Deployment

### 1. Build and Deploy Ticker Service

```bash
cd ticker-service-v2

# Quick deploy (automated)
./deploy.sh

# Or manual steps:
make proto          # Generate protobuf code
make build          # Build Go binary
docker-compose -f docker-compose.prod.yml up -d
```

### 2. Configure dddd-trader

Environment variables:
```bash
# Use gRPC (recommended)
TICKER_GRPC_HOST=ticker-service-v2  # or ticker-service-v2-dev
TICKER_GRPC_PORT=50051

# Or fallback to HTTP
TICKER_SERVICE_HOST=ticker-service-v2
TICKER_SERVICE_PORT=8080

# Or use direct IP to bypass DNS (fastest)
TICKER_SERVICE_IP=172.18.0.5  # Get from: docker inspect ticker-service-v2
```

### 3. Verify Setup

Test gRPC connection:
```bash
# Install grpcurl
brew install grpcurl  # macOS
# or
go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest

# Test health check
grpcurl -plaintext localhost:50051 ticker.TickerService/HealthCheck

# Get ticker data
grpcurl -plaintext -d '{"exchange":"binance","symbol":"btc-usdt"}' \
  localhost:50051 ticker.TickerService/GetTicker
```

## Docker Compose Configuration

Both dev and prod docker-compose files have been updated to expose gRPC port:

```yaml
services:
  app:
    ports:
      - "8091:8080"   # HTTP API
      - "50051:50051" # gRPC API
```

## Code Changes

### Ticker Service (Go)
- `internal/grpc/server.go` - gRPC server implementation
- `proto/ticker.proto` - Protocol buffer definitions
- `cmd/ticker/main.go` - Starts both HTTP and gRPC servers

### DDDD Trader (TypeScript)
- `src/markets/ticker-grpc.client.ts` - gRPC client implementation
- `src/markets/ticker.client.interface.ts` - Common interface
- `src/markets/markets.service.ts` - Uses gRPC client when available

## Monitoring

Check service status:
```bash
# HTTP health check
curl http://localhost:8091/health | jq .

# gRPC health check
grpcurl -plaintext localhost:50051 ticker.TickerService/HealthCheck

# View logs
docker-compose -f docker-compose.prod.yml logs -f
```

## Troubleshooting

### High Latency Issues
1. Check DNS resolution: Use IP address instead of hostname
2. Verify network: Ensure containers are on same Docker network
3. Check Redis connection: Ensure Redis is accessible and fast

### Connection Errors
1. Verify ports are exposed: `docker ps` should show 50051
2. Check firewall rules: Port 50051 must be accessible
3. Verify proto file: Both client and server must use same proto

### Performance Testing
```bash
# Test HTTP latency
time curl http://localhost:8091/api/ticker/binance/btc-usdt

# Test gRPC latency (with grpcurl)
time grpcurl -plaintext -d '{"exchange":"binance","symbol":"btc-usdt"}' \
  localhost:50051 ticker.TickerService/GetTicker

# Load test with multiple requests
for i in {1..100}; do
  grpcurl -plaintext -d '{"exchange":"binance","symbol":"btc-usdt"}' \
    localhost:50051 ticker.TickerService/GetTicker &
done
wait
```

## Benefits of gRPC

1. **Binary Protocol**: Protocol buffers are more efficient than JSON
2. **HTTP/2**: Multiplexing, header compression, server push
3. **Persistent Connections**: No TCP handshake overhead
4. **Type Safety**: Generated code ensures type safety
5. **Streaming**: Real-time ticker updates (future enhancement)

## Future Enhancements

1. **Bidirectional Streaming**: Real-time ticker updates
2. **Load Balancing**: Multiple ticker service instances
3. **Circuit Breaker**: Automatic fallback to HTTP
4. **Metrics**: Prometheus metrics for gRPC calls
5. **TLS**: Secure gRPC connections in production