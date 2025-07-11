#!/bin/bash
# NoiseFS Automated Seeding Script

set -e

# Default values
PROFILE="${1:-standard}"
OUTPUT_DIR="${2:-./seed-data}"
IPFS_API="${3:-http://localhost:5001}"

echo "🌱 NoiseFS Automated Seeding"
echo "============================"
echo "Profile: $PROFILE"
echo "Output: $OUTPUT_DIR"
echo "IPFS: $IPFS_API"
echo

# Check if IPFS is running
if ! curl -s "$IPFS_API/api/v0/id" > /dev/null; then
    echo "❌ Error: IPFS daemon is not running at $IPFS_API"
    echo "Please start IPFS with: ipfs daemon"
    exit 1
fi

# Build the seeder if needed
if [ ! -f "./noisefs-seed" ]; then
    echo "Building noisefs-seed..."
    go build -o noisefs-seed ./cmd/noisefs-seed
fi

# Run the seeder
./noisefs-seed \
    -profile="$PROFILE" \
    -output="$OUTPUT_DIR" \
    -ipfs="$IPFS_API" \
    -verbose

echo
echo "✅ Seeding complete!"
echo
echo "Next steps:"
echo "1. Start NoiseFS with the seeded pool: noisefs daemon -pool=$OUTPUT_DIR/pool"
echo "2. Test file upload: noisefs upload myfile.txt"
echo "3. Check stats: noisefs -stats"