# Integration with dddd-trader-v2

This guide explains how to integrate ticker-service-v2 with dddd-trader-v2 for improved market data fetching.

## Overview

The ticker-service-v2 provides centralized WebSocket connections to multiple exchanges, reducing connection overhead and improving performance. The dddd-trader-v2 can use this service instead of establishing its own connections to each exchange.

## Setup

### 1. Create Shared Docker Network

Both services need to be on the same Docker network to communicate:

```bash
# Run this on your server
docker network create dddd-network
```

Or use the provided script:
```bash
./scripts/setup-network.sh
```

### 2. Deploy ticker-service-v2

The service is configured to use the shared network:

```bash
cd /home/bz/deploy/ticker-service-v2
docker compose -f docker-compose.yml up -d  # for dev
# or
docker compose -f docker-compose.prod.yml up -d  # for production
```

### 3. Deploy dddd-trader-v2

The trader service will automatically detect and use the ticker service:

```bash
cd /home/bz/deploy/dddd-trader-v2
docker compose -f docker-compose.yml up -d  # for dev
# or
docker compose -f docker-compose.prod.yml up -d  # for production
```

## Communication

- **Network**: Both services communicate via Docker network `dddd-network`
- **Endpoint**: ticker-service-v2 is accessible at:
  - Development: `http://ticker-service-v2-dev:8080`
  - Production: `http://ticker-service-v2:8080`
- **Port Mapping**:
  - ticker-service-v2: External port 8091 → Internal port 8080
  - dddd-trader-v2: External port 8080 → Internal port 8080

## Configuration

### Enable/Disable Ticker Service in dddd-trader-v2

Set environment variable in dddd-trader-v2:

```bash
# In .env file
USE_TICKER_SERVICE=true  # Enable ticker service (default)
USE_TICKER_SERVICE=false # Disable and use direct connections
```

### Custom Ticker Service Host

If running outside Docker or using a custom setup:

```bash
# In .env file
TICKER_SERVICE_HOST=your-host
TICKER_SERVICE_PORT=8091
```

## Features

### Automatic Fallback
- If ticker-service-v2 is unavailable, dddd-trader-v2 automatically falls back to direct exchange connections
- Health checks run every 30 seconds to detect service availability

### Supported Exchanges
- ✅ Binance
- ✅ OKX
- ✅ Bybit
- ✅ Bitget
- ✅ Gate.io
- ❌ HyperLiquid (uses direct connection)
- ❌ XT (uses direct connection)
- ❌ Aster (uses direct connection)

### Performance Benefits
1. **Reduced Connections**: Single WebSocket per exchange instead of multiple
2. **Lower Latency**: Data is already available in memory
3. **Better Resource Usage**: Centralized connection management
4. **Automatic Reconnection**: Handled by ticker-service-v2

## Monitoring

### Check Service Health

```bash
# From host machine
curl http://localhost:8091/health

# Response
{
  "status": "ok",
  "storage": true,
  "storage_type": "memory",
  "time": "2025-08-18T05:26:55Z",
  "app": "ticker-service-v2"
}
```

### Check Connection Status

```bash
curl http://localhost:8091/status

# Shows WebSocket connection status for all exchanges
```

### View Logs

```bash
# ticker-service-v2 logs
docker logs -f ticker-service-v2-dev

# Check if dddd-trader-v2 is using ticker service
docker logs dddd-trader-v2-dev | grep "ticker-service"
```

## Troubleshooting

### Services Can't Communicate

1. Verify both containers are on the same network:
```bash
docker network inspect dddd-network
```

2. Check container names match configuration:
```bash
docker ps --format "table {{.Names}}\t{{.Networks}}"
```

3. Test connectivity from dddd-trader-v2:
```bash
docker exec dddd-trader-v2-dev curl http://ticker-service-v2-dev:8080/health
```

### High Latency Warnings

These are normal and can be ignored:
- Storage latency spikes during garbage collection
- Processing latency during high load
- Thresholds are very aggressive (>20ms storage, >50ms processing)

### OKX Reconnections

OKX WebSocket reconnects when subscribing to new symbols - this is expected behavior.

## API Endpoints

The ticker-service-v2 provides:

- `GET /health` - Service health check
- `GET /status` - WebSocket connection status
- `GET /api/ticker/{exchange}/{symbol}` - Get ticker data
- `POST /api/subscribe` - Subscribe to new symbol
- `GET /metrics` - Prometheus metrics