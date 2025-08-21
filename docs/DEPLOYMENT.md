# Deployment Guide & Troubleshooting

## Performance Monitoring

### Check System Status
```bash
# Monitor all exchanges and check for stale data
./scripts/monitor.sh http://your-server:8080

# Check Redis latency
./scripts/check-redis-latency.sh
```

### Available Endpoints
- `GET /health` - Service health with Redis connectivity
- `GET /status` - Detailed exchange connection status
- `GET /metrics` - Prometheus metrics
- `GET /api/ticker/{exchange}/{symbol}` - Latest ticker data

## Common Issues & Solutions

### 1. High Redis Latency (store_latency_ms > 20ms)

**Symptoms:**
- Logs showing "High latency detected" with store_latency_ms > 100ms
- Slow ticker updates

**Solutions:**

1. **Adjust warning thresholds** if Redis is remote:
   ```yaml
   # config.yaml or environment variables
   server:
     store_latency_warn_ms: 200  # Increase for remote Redis
   ```
   Or via environment:
   ```bash
   export TICKER_SERVER_STORE_LATENCY_WARN_MS=200
   ```

2. **Optimize Redis deployment:**
   - Deploy Redis in the same region/network as the service
   - Use Redis cluster for load distribution
   - Enable connection pooling
   - Monitor Redis CPU/memory usage

3. **Consider local Redis or in-memory storage:**
   ```yaml
   redis:
     addr: ""  # Empty for in-memory storage
   ```

### 2. OKX WebSocket Errors

**Symptoms:**
- "RSV1 set, RSV2 set, RSV3 set, bad opcode 11" errors
- OKX disconnecting every 8-10 minutes

**Solution:**
The fix has been applied - compression is now disabled for OKX connections. If issues persist:
- Check OKX API status page
- Monitor reconnection patterns with `/status` endpoint

### 3. Checking Ticker Data Flow

**Quick health check:**
```bash
# Check if all exchanges are connected
curl -s http://localhost:8080/status | jq '.exchanges | to_entries[] | select(.value.connected == false)'

# Check specific ticker
curl -s http://localhost:8080/api/ticker/binance/btc-usdt | jq '.'

# Check message counts
curl -s http://localhost:8080/status | jq '.exchanges | to_entries[] | {exchange: .key, messages: .value.message_count}'
```

## Deployment Best Practices

### 1. Redis Configuration
- **Same Region**: Deploy Redis in the same cloud region
- **Persistent Storage**: Enable AOF or RDB persistence
- **Memory**: Monitor memory usage, set maxmemory policy
- **Security**: Use password authentication, SSL if across networks

### 2. Service Configuration
```yaml
# Production config.yaml
server:
  port: 8080
  processing_latency_warn_ms: 100  # Adjust based on your infrastructure
  store_latency_warn_ms: 50        # Higher for cloud Redis

redis:
  addr: "redis-cluster.internal:6379"
  password: "${REDIS_PASSWORD}"    # Use environment variable
  db: 0

logging:
  level: "warn"       # Reduce log volume
  format: "json"      # Structured logging
  production: true    # Only errors
  ticker_updates: false
```

### 3. Monitoring Setup
- Use Prometheus to scrape `/metrics` endpoint
- Set up alerts for:
  - Disconnected exchanges (> 1 minute)
  - High error rates
  - Excessive reconnections
  - Stale data (no updates > 60s)

### 4. Environment Variables
```bash
# Production deployment
export TICKER_REDIS_ADDR="redis.production.internal:6379"
export TICKER_REDIS_PASSWORD="your-secure-password"
export TICKER_SERVER_STORE_LATENCY_WARN_MS=100
export TICKER_LOGGING_PRODUCTION=true
```

### 5. Docker Deployment
```bash
# Build with optimization
docker build -t ticker-service-v2:latest .

# Run with proper resource limits
docker run -d \
  --name ticker-service \
  --restart unless-stopped \
  --memory="512m" \
  --cpus="1.0" \
  -p 8080:8080 \
  -p 50051:50051 \
  -e TICKER_REDIS_ADDR="redis:6379" \
  -v ./config.yaml:/app/config.yaml \
  ticker-service-v2:latest
```

## Troubleshooting Commands

```bash
# Check container logs
docker logs -f ticker-service --tail 100

# Check Redis latency from container
docker exec ticker-service redis-cli -h redis --latency

# Monitor real-time metrics
watch -n 5 'curl -s http://localhost:8080/status | jq ".exchanges | to_entries[] | {exchange: .key, connected: .value.connected, messages: .value.message_count}"'

# Check for memory leaks
docker stats ticker-service
```

## Performance Tuning

### For Low Latency Requirements
1. Use local Redis or in-memory storage
2. Deploy service close to exchanges' servers (AWS Tokyo for Asian exchanges)
3. Use dedicated network connections
4. Increase WebSocket buffer sizes in exchange handlers

### For High Throughput
1. Run multiple instances with different symbol sets
2. Use Redis Cluster with read replicas
3. Enable Redis pipelining
4. Consider message batching

## Health Monitoring Script

Create a cron job for continuous monitoring:
```bash
#!/bin/bash
# /etc/cron.d/ticker-monitor
*/5 * * * * /path/to/scripts/monitor.sh http://localhost:8080 >> /var/log/ticker-monitor.log 2>&1
```