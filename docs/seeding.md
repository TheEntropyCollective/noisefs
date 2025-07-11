# NoiseFS Automated Seeding System

## Overview

The NoiseFS seeding system automatically downloads public domain content and generates the initial block pool required for privacy-preserving operation. It ensures adequate anonymity for early network participants by providing a diverse set of blocks.

## Quick Start

```bash
# Standard seeding (recommended)
./scripts/seed-noisefs.sh

# Minimal seeding (faster, less privacy)
./scripts/seed-noisefs.sh minimal

# Maximum seeding (best privacy, takes longer)
./scripts/seed-noisefs.sh maximum
```

## Seeding Profiles

### Minimal (~500MB, 5-10 minutes)
- 500+ blocks minimum
- Basic privacy protection
- Suitable for testing

### Standard (~2GB, 15-30 minutes)
- 2500+ blocks
- Good privacy protection
- Includes video content
- Recommended for most users

### Maximum (~10GB, 60-120 minutes)
- 10000+ blocks
- Maximum privacy protection
- Full content diversity
- Best for initial network nodes

## Content Sources

The seeder downloads from these public domain sources:

### Books (Project Gutenberg)
- Classic literature
- Historical texts
- Reference works

### Images (Wikimedia Commons)
- Art reproductions
- Historical photos
- Scientific diagrams

### Audio (Internet Archive)
- Classical music
- Historical recordings
- Public speeches

### Videos (Internet Archive)
- Early films
- Educational content
- Public domain documentaries

### Documents
- Government publications
- Historical documents
- Scientific papers

## Privacy Requirements

For adequate privacy, the initial pool must have:

1. **Minimum 500 blocks** per node
2. **50%+ public domain ratio**
3. **Multiple block sizes** (64KB-1MB)
4. **Content diversity** across types

## Advanced Usage

### Custom Configuration

```bash
noisefs-seed \
  -profile=standard \
  -output=./my-seed-data \
  -ipfs=http://localhost:5001 \
  -parallel=8 \
  -video-quality=1080p
```

### Partial Seeding

Skip phases you've already completed:

```bash
# Skip download, only generate blocks
noisefs-seed -skip-download

# Skip block generation, only initialize pool
noisefs-seed -skip-download -skip-generate

# Preview without downloading
noisefs-seed -dry-run
```

### Video Quality Options

Control video download quality:
- `480p` - Standard definition
- `720p` - HD (default)
- `1080p` - Full HD

## Block Generation

The seeder generates blocks using:

1. **Variable sizing** - Multiple block sizes for efficiency
2. **Content padding** - Deterministic padding for consistency
3. **Genesis blocks** - Known starting blocks for bootstrap
4. **Reuse optimization** - Maximizes block sharing potential

## Pool Initialization

After generating blocks, the seeder:

1. Uploads all blocks to IPFS
2. Creates block index by size
3. Generates pool metadata
4. Validates privacy requirements

## Validation

The pool is validated for:

- Total block count (≥500)
- Public domain ratio (≥50%)
- Block size coverage
- Content diversity score

## Output Structure

```
seed-data/
├── downloads/          # Downloaded content
│   ├── books/
│   ├── images/
│   ├── audio/
│   ├── videos/
│   └── documents/
├── blocks/            # Generated blocks
│   ├── *.block
│   └── genesis/
├── pool/              # Pool configuration
│   ├── pool.json
│   ├── block_index.json
│   ├── genesis.json
│   └── cid_mapping.json
└── reports/           # Seeding reports
    └── seed-report-*.txt
```

## Troubleshooting

### IPFS Connection Error
```
Error: IPFS daemon is not running
```
**Solution**: Start IPFS with `ipfs daemon`

### Download Failures
```
Warning: X download errors occurred
```
**Solution**: Re-run with `-skip-download` to retry failed downloads

### Insufficient Blocks
```
Pool does not meet requirements
```
**Solution**: Use a larger profile or increase `-blocks-per-size`

## Performance Tips

1. **Use parallel downloads**: `-parallel=8` for faster downloads
2. **Skip videos for speed**: Use minimal profile
3. **Reuse existing data**: Use `-skip-download` if retrying
4. **Check disk space**: Ensure enough space for profile

## Security Notes

- All content is from verified public domain sources
- Block generation uses cryptographic hashing
- No private data is included in the pool
- Genesis blocks are deterministic for verification