#!/bin/bash

# Simple round-trip test for NoiseFS
set -e

echo "Starting NoiseFS round-trip test..."

# Check if IPFS is running
if ! curl -s http://localhost:5001/api/v0/id > /dev/null 2>&1; then
    echo "Error: IPFS daemon is not running on localhost:5001"
    echo "Please start IPFS daemon with: ipfs daemon"
    exit 1
fi

# Clean up any previous test files
rm -f test_file.txt test_file_downloaded.txt

# Create test file
echo "Creating test file..."
echo "This is a test file for NoiseFS round-trip testing. It contains some sample data to verify that upload and download work correctly." > test_file.txt

# Build NoiseFS CLI
echo "Building NoiseFS CLI..."
go build -o noisefs-test ./cmd/noisefs

# Test upload
echo "Testing upload..."
UPLOAD_OUTPUT=$(./noisefs-test -upload test_file.txt 2>&1)
echo "$UPLOAD_OUTPUT"

# Extract descriptor CID from output
DESCRIPTOR_CID=$(echo "$UPLOAD_OUTPUT" | grep "Descriptor CID:" | cut -d' ' -f3)

if [ -z "$DESCRIPTOR_CID" ]; then
    echo "Error: Failed to extract descriptor CID from upload output"
    exit 1
fi

echo "Descriptor CID: $DESCRIPTOR_CID"

# Test download
echo "Testing download..."
./noisefs-test -download "$DESCRIPTOR_CID" -output test_file_downloaded.txt

# Compare files
echo "Comparing original and downloaded files..."
if diff test_file.txt test_file_downloaded.txt > /dev/null; then
    echo "✅ SUCCESS: Round-trip test passed! Files are identical."
else
    echo "❌ FAILURE: Downloaded file differs from original"
    echo "Original file:"
    cat test_file.txt
    echo "Downloaded file:"
    cat test_file_downloaded.txt
    exit 1
fi

# Clean up
rm -f test_file.txt test_file_downloaded.txt noisefs-test

echo "Round-trip test completed successfully!"