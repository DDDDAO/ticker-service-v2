#!/bin/bash

# Script to create shared Docker network for service communication

NETWORK_NAME="dddd-network"

# Check if network exists
if docker network ls | grep -q "$NETWORK_NAME"; then
    echo "Network '$NETWORK_NAME' already exists"
else
    echo "Creating network '$NETWORK_NAME'..."
    docker network create "$NETWORK_NAME"
    echo "Network '$NETWORK_NAME' created successfully"
fi

# Show network details
echo ""
echo "Network details:"
docker network inspect "$NETWORK_NAME" | grep -E '"Name"|"Driver"|"Subnet"'