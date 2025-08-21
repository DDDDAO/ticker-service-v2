#!/bin/bash

# Ticker Service Monitoring Script
# Usage: ./scripts/monitor.sh [server_url]

SERVER=${1:-"http://localhost:8080"}

echo "================================"
echo "Ticker Service Status Monitor"
echo "Server: $SERVER"
echo "================================"
echo ""

# Check health
echo "1. Health Check:"
echo "----------------"
curl -s "$SERVER/health" | jq '.' || echo "Failed to connect"
echo ""

# Check exchange status
echo "2. Exchange Status:"
echo "-------------------"
STATUS=$(curl -s "$SERVER/status")
if [ $? -eq 0 ]; then
    echo "$STATUS" | jq '.exchanges | to_entries[] | {
        exchange: .key,
        connected: .value.connected,
        message_count: .value.message_count,
        error_count: .value.error_count,
        last_message: .value.last_message,
        reconnect_count: .value.reconnect_count
    }'
else
    echo "Failed to get status"
fi
echo ""

# Check specific tickers
echo "3. Sample Ticker Data:"
echo "----------------------"
EXCHANGES=("binance" "okx" "bybit" "bitget" "gate")

for EXCHANGE in "${EXCHANGES[@]}"; do
    # Gate uses underscore format (btc_usdt), others use dash (btc-usdt)
    if [ "$EXCHANGE" = "gate" ]; then
        SYMBOL="btc_usdt"
    else
        SYMBOL="btc-usdt"
    fi
    
    echo -n "$EXCHANGE/$SYMBOL: "
    TICKER=$(curl -s "$SERVER/api/ticker/$EXCHANGE/$SYMBOL" 2>/dev/null)
    if [ -n "$TICKER" ] && [ "$TICKER" != "null" ]; then
        echo "$TICKER" | jq -c '{price: .last, volume: .volume, time: .timestamp}' 2>/dev/null || echo "No data"
    else
        echo "No data"
    fi
done
echo ""

# Check if all symbols are receiving data
echo "4. Symbol Coverage Check:"
echo "-------------------------"
echo "$STATUS" | jq -r '.exchanges | to_entries[] | "\(.key): \(.value.symbols | length) symbols subscribed"' 2>/dev/null
echo ""

# Identify disconnected exchanges
echo "5. Connection Issues:"
echo "--------------------"
DISCONNECTED=$(echo "$STATUS" | jq -r '.exchanges | to_entries[] | select(.value.connected == false) | .key' 2>/dev/null)
if [ -z "$DISCONNECTED" ]; then
    echo "✓ All exchanges connected"
else
    echo "⚠ Disconnected exchanges:"
    echo "$DISCONNECTED"
fi
echo ""

# Check for stale data (no updates in last 60 seconds)
echo "6. Stale Data Check (>60s):"
echo "---------------------------"
CURRENT_TIME=$(date +%s)
echo "$STATUS" | jq -r --arg ct "$CURRENT_TIME" '.exchanges | to_entries[] | 
    select(.value.last_message != null) | 
    {
        exchange: .key,
        last_message: .value.last_message,
        current_time: ($ct | tonumber),
        last_time: (.value.last_message | sub("\\.[0-9]+Z$"; "Z") | fromdateiso8601)
    } | 
    select((.current_time - .last_time) > 60) | 
    "\(.exchange): Last update \(.current_time - .last_time) seconds ago"' 2>/dev/null || echo "✓ All exchanges updating"
echo ""

# Summary
echo "7. Summary Statistics:"
echo "----------------------"
echo "$STATUS" | jq '{
    total_messages: [.exchanges[].message_count] | add,
    total_errors: [.exchanges[].error_count] | add,
    total_reconnects: [.exchanges[].reconnect_count] | add,
    connected_count: [.exchanges[] | select(.connected == true)] | length,
    total_count: [.exchanges[]] | length
}' 2>/dev/null