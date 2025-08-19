#!/bin/bash

# Deployment script for ticker-service-v2 with gRPC support

set -e

echo "🚀 Deploying ticker-service-v2 with gRPC support..."

# Build Docker image (includes proto generation)
echo "🐳 Building Docker image..."
docker build -t ticker-service-v2:latest .

# Stop existing container if running
echo "🛑 Stopping existing container..."
docker-compose -f docker-compose.prod.yml down || true

# Start new container
echo "✨ Starting new container with gRPC support..."
docker-compose -f docker-compose.prod.yml up -d

# Wait for service to be healthy
echo "⏳ Waiting for service to be healthy..."
sleep 5

# Check health
echo "🏥 Checking service health..."
curl -s http://localhost:8091/health | jq . || echo "HTTP health check failed"

echo "✅ Deployment complete!"
echo ""
echo "Service endpoints:"
echo "  - HTTP API: http://localhost:8091"
echo "  - gRPC API: localhost:50051"
echo ""
echo "To view logs: docker-compose -f docker-compose.prod.yml logs -f"
echo ""
echo "Test commands:"
echo "  HTTP: curl http://localhost:8091/api/ticker/binance/btc-usdt | jq ."
echo "  gRPC: grpcurl -plaintext localhost:50051 ticker.TickerService/HealthCheck"