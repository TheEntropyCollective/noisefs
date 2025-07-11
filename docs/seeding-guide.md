# NoiseFS Seeding Guide

## Overview

The NoiseFS seeding system (`noisefs-seed`) bootstraps the network with privacy-preserving public domain content. This ensures that early adopters have adequate anonymity protection through a diverse pool of reusable blocks.

## Quick Start

```bash
# Build the seeding tool
go build -o bin/noisefs-seed ./cmd/noisefs-seed

# Run with standard profile (recommended)
./bin/noisefs-seed -profile standard -ipfs http://127.0.0.1:5001

# Preview without execution
./bin/noisefs-seed -profile standard -dry-run
```

## Seeding Profiles

### Minimal Profile
- **Size**: ~500MB
- **Blocks**: 250-350
- **Use Case**: Testing and development
- **Note**: May not meet minimum pool requirements (500 blocks)

### Standard Profile (Recommended)
- **Size**: ~2GB
- **Blocks**: 800-1000
- **Use Case**: Production deployments
- **Features**: Includes video content for better block diversity

### Maximum Profile
- **Size**: ~10GB
- **Blocks**: 2000-3000
- **Use Case**: High-traffic nodes or seed servers
- **Features**: Maximum content diversity

## Privacy Requirements

For adequate privacy protection, the universal block pool must have:

1. **Minimum 500 total blocks**
2. **At least 50 blocks per size class** (64KB, 128KB, 256KB, 512KB, 1MB)
3. **At least 50% public domain content**
4. **Diverse content types** (text, images, audio, video)

## How It Works

### Phase 1: Content Download
Downloads public domain content from verified sources:
- **Books**: Project Gutenberg classics
- **Images**: Wikimedia Commons artwork
- **Audio**: Public domain music and speeches
- **Videos**: Classic films and educational content
- **Documents**: Government publications and research

### Phase 2: Block Generation
Processes content into NoiseFS blocks:
- Splits files into standard block sizes
- Generates deterministic genesis blocks
- Ensures even distribution across size classes
- Creates ~3x reuse potential through overlap

### Phase 3: Pool Initialization
Stores blocks in IPFS and validates requirements:
- Uploads all blocks to IPFS network
- Creates block index and CID mappings
- Validates minimum requirements
- Generates pool configuration

## Best Practices

### 1. Network Setup
```bash
# Ensure IPFS is running
ipfs daemon &

# Verify connectivity
ipfs swarm peers | wc -l  # Should show connected peers
```

### 2. Storage Requirements
- **Minimal**: 1GB free space
- **Standard**: 5GB free space
- **Maximum**: 20GB free space

### 3. Seeding Strategy

**For New Networks:**
1. Start with standard profile
2. Run on multiple initial nodes
3. Use different random seeds for variety

**For Existing Networks:**
1. Check current pool size first
2. Add blocks incrementally
3. Focus on underrepresented size classes

### 4. Verification
```bash
# Check pool statistics
cat seed-data/pool/pool.json

# Verify block distribution
ls seed-data/blocks/*.block | wc -l

# Test block retrieval
ipfs cat <CID> | head -c 100
```

## Advanced Options

### Custom Configuration
```bash
# Skip phases for testing
./bin/noisefs-seed -skip-download  # Use existing downloads
./bin/noisefs-seed -skip-generate  # Use existing blocks
./bin/noisefs-seed -skip-init      # Skip IPFS upload

# Performance tuning
./bin/noisefs-seed -parallel 8     # More parallel downloads
```

### Video Quality Control
```bash
# Limit video quality for bandwidth
./bin/noisefs-seed -video-quality 480p  # Lower quality
./bin/noisefs-seed -video-quality 1080p # Higher quality
```

## Troubleshooting

### "Pool validation failed"
**Cause**: Not enough blocks generated
**Solution**: Use standard or maximum profile

### "IPFS connection error"
**Cause**: IPFS daemon not running or wrong endpoint
**Solution**: 
```bash
ipfs daemon &
./bin/noisefs-seed -ipfs http://127.0.0.1:5001
```

### "Download errors"
**Cause**: Network issues or rate limiting
**Solution**: Retry with fewer parallel downloads
```bash
./bin/noisefs-seed -parallel 2
```

## Legal Considerations

All seeded content is verified public domain:
- Books: Published before 1928 or explicitly public domain
- Images: CC0 or public domain from Wikimedia
- Audio: Public domain recordings
- Videos: Classic films with expired copyrights
- Documents: Government publications

The seeding system provides plausible deniability by ensuring all initial network content is legally distributable.

## Monitoring

After seeding, monitor the pool health:

```bash
# Check NoiseFS stats
noisefs -stats

# Verify block availability
noisefs test-retrieve <descriptor>

# Monitor IPFS storage
ipfs repo stat
```

## Conclusion

Proper seeding is crucial for NoiseFS privacy guarantees. The automated seeding system ensures new networks start with adequate anonymity protection through a diverse pool of reusable public domain blocks. Always use at least the standard profile for production deployments.