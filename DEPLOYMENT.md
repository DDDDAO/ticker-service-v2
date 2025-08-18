# Ticker Service Deployment Guide

Simple deployment guide for ticker-service-v2 using Docker Compose.

## Prerequisites

- Docker & Docker Compose
- Redis instance (optional - can use in-memory storage)

## Configuration

### Config Files

The service uses YAML config files for all configuration:

- `config.yaml` - Development configuration (in-memory storage by default)
- `config.production.yaml` - Production configuration (Redis by default)

#### Storage Configuration

**Option 1: Use in-memory storage (no Redis needed)**
```yaml
redis:
  addr: ""  # Leave empty for in-memory storage
```

**Option 2: Use Redis**
```yaml
redis:
  addr: your-redis-host:6379  # Set your Redis address
  password: "your-password"    # Add password if needed
  db: 0
```

> **Note**: In-memory storage is suitable for single-instance deployments. Data is lost on restart. For production with multiple instances or persistence needs, use Redis.

## Deployment

### Development Environment

```bash
# Build the Docker image
docker build -t ticker-service-v2-dev .

# Start the service
docker-compose up -d

# View logs
docker-compose logs -f app

# Stop the service
docker-compose down
```

### Production Environment

```bash
# Build production image
docker build -t ticker-service-v2 .

# Start with production config
docker-compose -f docker-compose.prod.yml up -d

# View logs
docker-compose -f docker-compose.prod.yml logs -f app

# Stop the service
docker-compose -f docker-compose.prod.yml down
```

### Using Makefile Commands

The project includes a Makefile with convenient commands:

```bash
# Build and run locally
make build
make run

# Docker operations
make docker-build
make docker-run
make docker-stop

# Docker Compose operations
make compose-up        # Start with docker-compose
make compose-down      # Stop services
make compose-logs      # View logs

# Testing
make test              # Run tests
make test-coverage     # Run tests with coverage

# Health check
make health            # Check service health
```

## Health Checks

```bash
# Check service health
curl http://localhost:8080/health

# Check exchange status
curl http://localhost:8080/status

# Get ticker data
curl http://localhost:8080/api/ticker/binance/btc-usdt
```

## Monitoring

### View Logs

```bash
# Docker logs
docker logs -f ticker-service-v2-dev

# Docker Compose logs
docker-compose logs -f app

# Production logs
docker-compose -f docker-compose.prod.yml logs -f app
```

### Redis Monitoring

```bash
# Connect to Redis
redis-cli -h your-redis-host -p 6379

# Monitor ticker channels
redis-cli PSUBSCRIBE "ticker:*"

# Check published data
redis-cli MONITOR
```

## Troubleshooting

### Common Issues

#### WebSocket Connection Failures

Check logs for connection errors:
```bash
docker logs ticker-service-v2-dev | grep -i "error\|failed"
```

#### Redis Connection Issues

Test Redis connection:
```bash
redis-cli -h your-redis-host -p 6379 ping
```

#### Missing Ticker Data

Check specific exchange status:
```bash
curl http://localhost:8080/status | jq '.exchanges.binance'
```

Test subscription:
```bash
curl -X POST http://localhost:8080/api/subscribe \
  -H "Content-Type: application/json" \
  -d '{"exchange":"binance","symbol":"dot-usdt"}'
```

### Debug Mode

Enable debug logging:
```bash
# Set in .env file
TICKER_LOG_LEVEL=debug

# Restart service
docker-compose restart
```

## System Requirements

### Resource Recommendations

- **Development**: 2GB RAM, 2 CPU cores
- **Production**: 4GB RAM, 4 CPU cores
- **File descriptors**: 65536 (for WebSocket connections)

### Docker Resource Limits

The docker-compose files include appropriate resource limits:
- Development: 2GB memory limit
- Production: 4GB memory limit
- ulimit configured for 65536 file descriptors

## Updating

```bash
# Pull latest code
git pull origin main

# Rebuild image
docker build -t ticker-service-v2 .

# Restart service
docker-compose -f docker-compose.prod.yml down
docker-compose -f docker-compose.prod.yml up -d
```

## Rollback

```bash
# Tag current version before update
docker tag ticker-service-v2:latest ticker-service-v2:backup

# If rollback needed
docker tag ticker-service-v2:backup ticker-service-v2:latest
docker-compose -f docker-compose.prod.yml up -d
```