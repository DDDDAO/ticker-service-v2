#!/bin/bash

# Performance Test Script for Ticker Service
# This script helps diagnose performance issues between environments

echo "========================================="
echo "Ticker Service Performance Test"
echo "========================================="
echo ""

# Test 1: System Information
echo "1. System Information:"
echo "----------------------"
echo "Hostname: $(hostname)"
echo "OS: $(uname -s) $(uname -r)"
echo "CPU Cores: $(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo "Unknown")"
echo "CPU Model: $(lscpu 2>/dev/null | grep "Model name" | cut -d: -f2 | xargs || sysctl -n machdep.cpu.brand_string 2>/dev/null || echo "Unknown")"
echo "Memory: $(free -h 2>/dev/null | grep Mem | awk '{print $2}' || vm_stat 2>/dev/null | grep "Pages free" | awk '{print $3}' || echo "Unknown")"
echo "Load Average: $(uptime | awk -F'load average:' '{print $2}')"
echo ""

# Test 2: CPU Performance (simple benchmark)
echo "2. CPU Performance Test:"
echo "------------------------"
echo -n "Simple CPU benchmark (lower is better): "
START=$(date +%s%N 2>/dev/null || python3 -c 'import time; print(int(time.time() * 1000000000))')
for i in {1..1000000}; do
    :
done
END=$(date +%s%N 2>/dev/null || python3 -c 'import time; print(int(time.time() * 1000000000))')
DURATION=$((($END - $START) / 1000000))
echo "${DURATION}ms"
echo ""

# Test 3: Memory Allocation Test
echo "3. Memory Allocation Test:"
echo "--------------------------"
echo -n "Allocating and accessing 100MB memory: "
START=$(date +%s%N 2>/dev/null || python3 -c 'import time; print(int(time.time() * 1000000000))')
python3 -c "
import time
data = bytearray(100 * 1024 * 1024)
for i in range(0, len(data), 4096):
    data[i] = 1
"
END=$(date +%s%N 2>/dev/null || python3 -c 'import time; print(int(time.time() * 1000000000))')
DURATION=$((($END - $START) / 1000000))
echo "${DURATION}ms"
echo ""

# Test 4: Mutex/Lock Performance (Go specific)
echo "4. Go Mutex Performance Test:"
echo "-----------------------------"
cat > /tmp/mutex_test.go << 'EOF'
package main

import (
    "fmt"
    "sync"
    "time"
)

func main() {
    var mu sync.RWMutex
    data := make(map[string]string)
    
    // Test write lock performance
    start := time.Now()
    for i := 0; i < 10000; i++ {
        mu.Lock()
        data[fmt.Sprintf("key%d", i)] = fmt.Sprintf("value%d", i)
        mu.Unlock()
    }
    writeTime := time.Since(start)
    
    // Test read lock performance
    start = time.Now()
    for i := 0; i < 10000; i++ {
        mu.RLock()
        _ = data[fmt.Sprintf("key%d", i)]
        mu.RUnlock()
    }
    readTime := time.Since(start)
    
    fmt.Printf("Write locks (10k ops): %v\n", writeTime)
    fmt.Printf("Read locks (10k ops): %v\n", readTime)
    fmt.Printf("Avg write lock: %.2fµs\n", float64(writeTime.Microseconds())/10000)
    fmt.Printf("Avg read lock: %.2fµs\n", float64(readTime.Microseconds())/10000)
}
EOF

if command -v go &> /dev/null; then
    go run /tmp/mutex_test.go
else
    echo "Go not installed - skipping mutex test"
fi
echo ""

# Test 5: Docker Performance (if applicable)
echo "5. Container/Docker Check:"
echo "--------------------------"
if [ -f /.dockerenv ]; then
    echo "Running inside Docker container"
    echo "Container limits:"
    cat /proc/self/cgroup 2>/dev/null | grep -E "memory|cpu" | head -5 || echo "Unable to detect limits"
elif command -v docker &> /dev/null; then
    echo "Docker is installed but not running in container"
else
    echo "Not running in Docker"
fi
echo ""

# Test 6: Service-specific metrics
echo "6. Ticker Service Metrics:"
echo "--------------------------"
SERVER_URL="${1:-http://localhost:8091}"
if curl -s "$SERVER_URL/health" > /dev/null 2>&1; then
    echo "Service is running at $SERVER_URL"
    
    # Make 10 rapid requests to test response time
    echo -n "Average API response time (10 requests): "
    TOTAL=0
    for i in {1..10}; do
        START=$(date +%s%N 2>/dev/null || python3 -c 'import time; print(int(time.time() * 1000000000))')
        curl -s "$SERVER_URL/api/ticker/binance/btc-usdt" > /dev/null
        END=$(date +%s%N 2>/dev/null || python3 -c 'import time; print(int(time.time() * 1000000000))')
        DURATION=$((($END - $START) / 1000000))
        TOTAL=$(($TOTAL + $DURATION))
    done
    AVG=$(($TOTAL / 10))
    echo "${AVG}ms"
    
    # Get current status
    STATUS=$(curl -s "$SERVER_URL/status")
    TOTAL_MESSAGES=$(echo "$STATUS" | jq '[.exchanges[].message_count] | add' 2>/dev/null)
    echo "Total messages processed: $TOTAL_MESSAGES"
else
    echo "Service not running at $SERVER_URL"
fi
echo ""

# Test 7: Analysis
echo "7. Performance Analysis:"
echo "------------------------"
if [ "$DURATION" -gt 100 ]; then
    echo "⚠️  CPU performance is SLOW (>100ms for simple loop)"
fi

if command -v go &> /dev/null; then
    MUTEX_TIME=$(go run /tmp/mutex_test.go 2>/dev/null | grep "Avg write lock" | awk '{print $4}' | cut -d'.' -f1)
    if [ ! -z "$MUTEX_TIME" ] && [ "$MUTEX_TIME" -gt 10 ]; then
        echo "⚠️  Mutex operations are SLOW (>10µs average)"
    fi
fi

echo ""
echo "Expected performance on modern hardware:"
echo "- CPU benchmark: <50ms"
echo "- Memory allocation: <200ms"
echo "- Mutex operations: <1µs"
echo "- API response: <10ms"
echo ""

echo "Common causes of high latency:"
echo "1. **CPU throttling** - Check if CPU governor is set to 'powersave'"
echo "2. **Memory pressure** - System might be swapping"
echo "3. **Container limits** - Docker CPU/memory limits too restrictive"
echo "4. **Virtual CPU** - VPS/cloud instances may have 'noisy neighbors'"
echo "5. **Background processes** - Other services consuming resources"
echo ""

# Cleanup
rm -f /tmp/mutex_test.go 2>/dev/null