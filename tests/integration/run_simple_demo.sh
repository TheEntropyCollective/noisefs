#!/bin/bash
# Run the simple demo test

set -e

echo "Running NoiseFS Simple Demo Test"
echo "================================"

# Check if IPFS is running
if ! curl -s http://localhost:5001/api/v0/id > /dev/null 2>&1; then
    echo "⚠ Warning: IPFS daemon is not running on localhost:5001"
    echo "Some tests will be skipped. To run all tests, start IPFS with: ipfs daemon"
    export SKIP_IPFS_TESTS=1
fi

# Change to repository root
cd "$(dirname "$0")/../.."

# Run the simple demo test
echo ""
echo "Running simple demo test..."
go test -v ./tests/integration -run TestSimpleUploadDownload -timeout 2m

echo ""
echo "Running block anonymization test..."
go test -v ./tests/integration -run TestBlockAnonymization -timeout 1m

echo ""
echo "Tests completed!"