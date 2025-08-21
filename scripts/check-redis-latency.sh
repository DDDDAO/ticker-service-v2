#!/bin/bash

# Redis Latency Check Script
# This helps diagnose the high store_latency_ms issues

echo "Redis Latency Analysis"
echo "======================"
echo ""

# Get Redis connection info from environment or use defaults
REDIS_HOST=${REDIS_HOST:-"localhost"}
REDIS_PORT=${REDIS_PORT:-"6379"}
REDIS_PASSWORD=${REDIS_PASSWORD:-""}

# Build redis-cli command
if [ -n "$REDIS_PASSWORD" ]; then
    REDIS_CMD="redis-cli -h $REDIS_HOST -p $REDIS_PORT -a $REDIS_PASSWORD"
else
    REDIS_CMD="redis-cli -h $REDIS_HOST -p $REDIS_PORT"
fi

echo "Testing Redis at $REDIS_HOST:$REDIS_PORT"
echo ""

# 1. Basic ping test
echo "1. Ping Test (10 iterations):"
echo "-----------------------------"
for i in {1..10}; do
    START=$(date +%s%N)
    $REDIS_CMD PING > /dev/null 2>&1
    END=$(date +%s%N)
    LATENCY=$((($END - $START) / 1000000))
    echo "Iteration $i: ${LATENCY}ms"
done
echo ""

# 2. Redis latency monitoring
echo "2. Redis Built-in Latency Stats:"
echo "--------------------------------"
$REDIS_CMD --latency-history 2>/dev/null || echo "Latency monitoring not available"
echo ""

# 3. Check Redis INFO for performance issues
echo "3. Redis Performance Metrics:"
echo "-----------------------------"
$REDIS_CMD INFO stats 2>/dev/null | grep -E "instantaneous_ops_per_sec|total_commands_processed|rejected_connections|keyspace_hits|keyspace_misses|used_memory_human|used_memory_peak_human" || echo "Could not get Redis stats"
echo ""

# 4. Check Redis slow log
echo "4. Redis Slow Log (last 10):"
echo "----------------------------"
$REDIS_CMD SLOWLOG GET 10 2>/dev/null || echo "No slow queries logged"
echo ""

# 5. Test actual ticker operations
echo "5. Ticker Operation Simulation:"
echo "-------------------------------"
TEST_KEY="ticker:test:btc-usdt"
TEST_VALUE='{"symbol":"btc-usdt","last":50000,"volume":1000,"timestamp":"2024-01-01T00:00:00Z"}'

# Test SET operation
echo -n "SET operation: "
START=$(date +%s%N)
$REDIS_CMD SET "$TEST_KEY" "$TEST_VALUE" EX 60 > /dev/null 2>&1
END=$(date +%s%N)
SET_LATENCY=$((($END - $START) / 1000000))
echo "${SET_LATENCY}ms"

# Test GET operation
echo -n "GET operation: "
START=$(date +%s%N)
$REDIS_CMD GET "$TEST_KEY" > /dev/null 2>&1
END=$(date +%s%N)
GET_LATENCY=$((($END - $START) / 1000000))
echo "${GET_LATENCY}ms"

# Test PUBLISH operation (what ticker service uses)
echo -n "PUBLISH operation: "
START=$(date +%s%N)
$REDIS_CMD PUBLISH "$TEST_KEY" "$TEST_VALUE" > /dev/null 2>&1
END=$(date +%s%N)
PUB_LATENCY=$((($END - $START) / 1000000))
echo "${PUB_LATENCY}ms"

# Clean up
$REDIS_CMD DEL "$TEST_KEY" > /dev/null 2>&1

echo ""
echo "6. Analysis:"
echo "-----------"
if [ $SET_LATENCY -gt 20 ] || [ $GET_LATENCY -gt 20 ] || [ $PUB_LATENCY -gt 20 ]; then
    echo "⚠️  High latency detected (>20ms threshold)"
    echo "Possible causes:"
    echo "- Network latency between service and Redis"
    echo "- Redis server overload"
    echo "- Insufficient Redis memory"
    echo "- Too many concurrent connections"
    echo ""
    echo "Recommendations:"
    echo "1. Check if Redis is on same network/region as service"
    echo "2. Monitor Redis CPU and memory usage"
    echo "3. Consider using Redis connection pooling"
    echo "4. Enable Redis persistence if not already"
    echo "5. Consider Redis cluster for load distribution"
else
    echo "✓ Redis latency within acceptable range (<20ms)"
fi