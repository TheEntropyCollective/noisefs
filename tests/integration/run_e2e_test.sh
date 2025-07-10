#!/bin/bash
# Simple script to run the end-to-end test

set -e

echo "Starting NoiseFS End-to-End Test"
echo "================================"

# Check if IPFS is running
if ! curl -s http://localhost:5001/api/v0/id > /dev/null 2>&1; then
    echo "Error: IPFS daemon is not running on localhost:5001"
    echo "Please start IPFS with: ipfs daemon"
    exit 1
fi

echo "✓ IPFS daemon is running"

# Change to repository root
cd "$(dirname "$0")/../.."

# Run the integration test
echo ""
echo "Running end-to-end integration test..."
go test -v ./tests/integration -run TestEndToEndFlow -timeout 5m

echo ""
echo "Test completed!"