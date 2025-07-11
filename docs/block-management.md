# NoiseFS Block Management System

## Overview

The Block Management System is the foundational component of NoiseFS, implementing the core OFFSystem (Owner-Free File System) architecture. It provides cryptographic anonymization through XOR operations and enforces mandatory block reuse with public domain content mixing for legal protection.

## Core Architecture

### OFFSystem Principles

NoiseFS implements the OFFSystem architecture with these core principles:

1. **No Original Content Storage**: Files are never stored in their original form
2. **Mathematical Anonymization**: All blocks appear as cryptographically random data
3. **Mandatory Block Reuse**: Every block MUST be part of multiple files
4. **Public Domain Mixing**: Files include public domain content for legal protection
5. **Plausible Deniability**: Storage nodes cannot determine what content they host

### Block Implementation

The core block structure is defined in `pkg/core/blocks/block.go`:

```go
// Block represents a data block in the NoiseFS system
type Block struct {
    ID   string  // SHA-256 content hash
    Data []byte  // Block data
}

const DefaultBlockSize = 128 * 1024  // 128 KiB standard block size
```

### 3-Tuple XOR Anonymization

NoiseFS uses a 3-tuple XOR scheme for anonymization:

```
AnonymizedBlock = SourceBlock ⊕ Randomizer1 ⊕ Randomizer2
```

This is implemented in the `XOR3` method:

```go
// XOR3 performs XOR operation between three blocks (data XOR randomizer1 XOR randomizer2)
// This implements the 3-tuple anonymization used in OFFSystem
func (b *Block) XOR3(randomizer1, randomizer2 *Block) (*Block, error) {
    if len(b.Data) != len(randomizer1.Data) {
        return nil, errors.New("data block and randomizer1 must have the same size")
    }
    if len(b.Data) != len(randomizer2.Data) {
        return nil, errors.New("data block and randomizer2 must have the same size")
    }
    
    result := make([]byte, len(b.Data))
    for i := range b.Data {
        result[i] = b.Data[i] ^ randomizer1.Data[i] ^ randomizer2.Data[i]
    }
    
    return NewBlock(result)
}
```

## Mandatory Block Reuse System

### Universal Block Pool

The `UniversalBlockPool` (in `pkg/privacy/reuse/universal_pool.go`) manages a mandatory pool of reusable blocks that includes public domain content:

```go
type UniversalBlockPool struct {
    blocks           map[string]*PoolBlock      // CID -> PoolBlock
    blocksBySize     map[int][]string          // size -> []CID
    publicDomainCIDs map[string]bool           // Track public domain blocks
    config           *PoolConfig
    initialized      bool
}

type PoolBlock struct {
    CID              string
    Block            *blocks.Block
    Size             int
    IsPublicDomain   bool
    Source           string    // "bootstrap", "upload", "genesis"
    ContentType      string    // "text", "image", "audio", etc.
    UsageCount       int64
    PopularityScore  float64
}
```

### Pool Initialization

The pool is initialized with:

1. **Genesis Blocks**: Deterministic blocks for each standard size
2. **Public Domain Content**: Blocks from Project Gutenberg, artwork, etc.

```go
func (pool *UniversalBlockPool) Initialize() error {
    // Generate deterministic genesis blocks
    if err := pool.generateGenesisBlocks(); err != nil {
        return err
    }
    
    // Load public domain blocks from bootstrap content
    if err := pool.loadPublicDomainBlocks(); err != nil {
        return err
    }
    
    // Validate pool meets minimum requirements
    return pool.validatePool()
}
```

### Reuse Enforcement

The `ReuseEnforcer` (in `pkg/privacy/reuse/enforcer.go`) validates that uploads meet reuse requirements:

```go
type ReusePolicy struct {
    MinReuseRatio        float64  // Minimum % of blocks that must be reused (default: 50%)
    PublicDomainRatio    float64  // Minimum % of public domain blocks (default: 30%)
    PopularBlockRatio    float64  // Minimum % of popular blocks (default: 40%)
    MaxNewBlocks         int      // Maximum new blocks per upload (default: 10)
    MinFileAssociations  int      // Minimum files each block must serve (default: 3)
    EnforcementLevel     string   // "strict", "moderate", "permissive"
}
```

Validation process:

```go
func (enforcer *ReuseEnforcer) ValidateUpload(
    descriptor *descriptors.Descriptor, 
    fileData []byte,
) (*ValidationResult, error) {
    result := &ValidationResult{
        Valid:      true,
        Violations: make([]string, 0),
    }
    
    // Extract block CIDs from descriptor
    blockCIDs := enforcer.extractBlockCIDs(descriptor)
    
    // Check reuse ratio
    enforcer.checkReuseRatio(blockCIDs, result)
    
    // Check public domain ratio
    enforcer.checkPublicDomainRatio(blockCIDs, result)
    
    // Check new block limit
    if result.NewBlockCount > enforcer.policy.MaxNewBlocks {
        result.Violations = append(result.Violations, 
            fmt.Sprintf("too many new blocks: %d", result.NewBlockCount))
    }
    
    return result, nil
}
```

## Public Domain Mixing

### Legal Protection Through Mixing

The `PublicDomainMixer` (in `pkg/privacy/reuse/mixer.go`) ensures every file includes public domain content for legal protection:

```go
type PublicDomainMixer struct {
    pool            *UniversalBlockPool
    config          *MixerConfig
    mixingStrategy  MixingStrategy
}

type MixerConfig struct {
    MinPublicDomainRatio float64  // Minimum % of public domain blocks (default: 30%)
    MixingAlgorithm      string   // "deterministic", "random", "optimal"
    VerificationLevel    string   // "strict", "moderate", "basic"
    LegalCompliance      bool     // Enable extra legal protections
}
```

### Mixing Process

Files are mixed with public domain content during upload:

```go
func (mixer *PublicDomainMixer) MixFileWithPublicDomain(
    fileBlocks []*blocks.Block,
) (*descriptors.Descriptor, *MixingPlan, error) {
    // Create mixing plan
    plan, err := mixer.mixingStrategy.DetermineOptimalMixing(fileBlocks)
    
    // Execute mixing to create descriptor
    descriptor, err := mixer.executeMixingPlan(fileBlocks, plan)
    
    // Generate legal attestation
    attestation, err := mixer.generateLegalAttestation(fileBlocks, plan)
    plan.LegalAttestation = attestation
    
    return descriptor, plan, nil
}
```

### Legal Attestation

Each mixed file includes a legal attestation proving public domain content inclusion:

```go
type LegalAttestation struct {
    AttestationID        string
    FileHash             string
    PublicDomainSources  []string  // Sources of public domain content
    LicenseProofs        []string  // Proof of public domain status
    MixingTimestamp      time.Time
    ComplianceCertificate string
}
```

## Block Selection Process

When creating a file, the system selects randomizer blocks from the universal pool:

```go
func (pool *UniversalBlockPool) GetRandomizerBlock(size int) (*PoolBlock, error) {
    blocks := pool.blocksBySize[size]
    if len(blocks) == 0 {
        return nil, fmt.Errorf("no blocks available for size %d", size)
    }
    
    // Select random block from pool
    index := rand.Intn(len(blocks))
    cid := blocks[index]
    poolBlock := pool.blocks[cid]
    
    // Update usage statistics
    poolBlock.UsageCount++
    poolBlock.LastUsed = time.Now()
    poolBlock.PopularityScore = pool.calculatePopularity(poolBlock)
    
    return poolBlock, nil
}
```

## Security Considerations

### Threat Model

The block management system defends against:

1. **Content Analysis**: Anonymized blocks are indistinguishable from random
2. **Censorship Attempts**: Removing blocks affects multiple unrelated files
3. **Legal Challenges**: Public domain mixing provides plausible deniability

### Implementation Security

1. **Constant-Time XOR**: The XOR3 implementation avoids timing side-channels
2. **Secure Randomness**: Uses `crypto/rand` for block generation
3. **Hash Integrity**: SHA-256 for content addressing

## Performance Characteristics

### Storage Efficiency

- **Naive Storage**: 3x overhead (source + 2 randomizers)
- **With Mandatory Reuse**: 1.8-2.2x overhead
- **Public Domain Blocks**: Shared across many files

### Cache Performance

- Popular blocks are cached across the network
- Public domain blocks have high cache hit rates
- Randomizer selection considers popularity

## Future Enhancements

The following optimizations are planned but not yet implemented:

1. **Sensitivity-Based Selection**: Different strategies based on content sensitivity
2. **ML-Based Optimization**: Machine learning for optimal randomizer selection
3. **Advanced Caching**: Multi-tier cache integration
4. **Zero-Knowledge Proofs**: Prove compliance without revealing content

## Conclusion

The Block Management System provides the cryptographic foundation for NoiseFS through:

1. **3-Tuple XOR Anonymization**: Information-theoretic security
2. **Mandatory Block Reuse**: Every block serves multiple files
3. **Public Domain Mixing**: Legal protection through content mixing
4. **Reuse Enforcement**: Automated compliance validation

This architecture ensures that storage providers have plausible deniability while achieving efficient storage through mandatory block sharing. The public domain mixing provides additional legal protection by ensuring that every stored block contains legitimate public domain content.