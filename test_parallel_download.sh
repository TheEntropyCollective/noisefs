#!/bin/bash

# Test script for parallel download functionality
# This script uploads a test file and then downloads it to compare performance

set -e

echo "=== NoiseFS Parallel Download Test ==="
echo

# Create a test file of reasonable size (10MB)
echo "Creating 10MB test file..."
dd if=/dev/urandom of=test_file_10mb.bin bs=1M count=10 2>/dev/null

# Build the latest version
echo "Building NoiseFS..."
go build -o noisefs-test cmd/noisefs/main.go 2>/dev/null || {
    echo "Build failed, but continuing with existing binary if available"
}

# Start IPFS daemon if not running
if ! pgrep -x "ipfs" > /dev/null; then
    echo "Starting IPFS daemon..."
    ipfs daemon &
    IPFS_PID=$!
    sleep 5
fi

# Upload the test file
echo
echo "Uploading test file..."
UPLOAD_OUTPUT=$(./noisefs-test upload test_file_10mb.bin 2>&1 || ./noisefs upload test_file_10mb.bin 2>&1)
DESCRIPTOR_CID=$(echo "$UPLOAD_OUTPUT" | grep "Descriptor CID:" | awk '{print $3}')

if [ -z "$DESCRIPTOR_CID" ]; then
    echo "Failed to get descriptor CID from upload"
    echo "Upload output: $UPLOAD_OUTPUT"
    exit 1
fi

echo "Uploaded with Descriptor CID: $DESCRIPTOR_CID"

# Download the file with parallel processing
echo
echo "Downloading with parallel processing..."
time ./noisefs-test download "$DESCRIPTOR_CID" downloaded_file.bin 2>&1 || time ./noisefs download "$DESCRIPTOR_CID" downloaded_file.bin 2>&1

# Verify the downloaded file
echo
echo "Verifying downloaded file..."
if cmp -s test_file_10mb.bin downloaded_file.bin; then
    echo "✓ File verification successful - files match!"
else
    echo "✗ File verification failed - files don't match!"
    exit 1
fi

# Show file sizes
echo
echo "File sizes:"
ls -lh test_file_10mb.bin downloaded_file.bin

# Cleanup
echo
echo "Cleaning up test files..."
rm -f test_file_10mb.bin downloaded_file.bin

# Kill IPFS daemon if we started it
if [ ! -z "$IPFS_PID" ]; then
    kill $IPFS_PID 2>/dev/null || true
fi

echo
echo "=== Test completed successfully! ==="