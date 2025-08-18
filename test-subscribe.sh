#!/bin/bash

echo "Testing dynamic subscription for all exchanges"
echo "=============================================="

# Test subscribing to a new symbol for each exchange
SYMBOL="dot-usdt"

echo ""
echo "Testing Binance subscription for $SYMBOL"
curl -X POST "http://localhost:8080/api/subscribe" \
  -H "Content-Type: application/json" \
  -d "{\"exchange\":\"binance\",\"symbol\":\"$SYMBOL\"}"
echo ""

echo ""
echo "Testing OKX subscription for $SYMBOL"
curl -X POST "http://localhost:8080/api/subscribe" \
  -H "Content-Type: application/json" \
  -d "{\"exchange\":\"okx\",\"symbol\":\"$SYMBOL\"}"
echo ""

echo ""
echo "Testing Bybit subscription for $SYMBOL"
curl -X POST "http://localhost:8080/api/subscribe" \
  -H "Content-Type: application/json" \
  -d "{\"exchange\":\"bybit\",\"symbol\":\"$SYMBOL\"}"
echo ""

echo ""
echo "Testing Bitget subscription for $SYMBOL"
curl -X POST "http://localhost:8080/api/subscribe" \
  -H "Content-Type: application/json" \
  -d "{\"exchange\":\"bitget\",\"symbol\":\"$SYMBOL\"}"
echo ""

echo ""
echo "Testing Gate subscription for $SYMBOL"
curl -X POST "http://localhost:8080/api/subscribe" \
  -H "Content-Type: application/json" \
  -d "{\"exchange\":\"gate\",\"symbol\":\"$SYMBOL\"}"
echo ""

echo ""
echo "Waiting 5 seconds for reconnections to complete..."
sleep 5

echo ""
echo "Checking if DOT-USDT ticker data is available:"
echo "=============================================="
for exchange in binance okx bybit bitget gate; do
  echo -n "$exchange: "
  curl -s "http://localhost:8080/api/ticker/$exchange/$SYMBOL" | jq -r '.price // "Not available"'
done