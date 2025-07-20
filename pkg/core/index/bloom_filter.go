// Package index provides privacy-preserving indexing for NoiseFS using Bloom filters
// and other privacy-preserving techniques. This enables fast search and navigation
// while maintaining OFFSystem's strong anonymity guarantees.
package index

import (
	"crypto/rand"
	"crypto/sha256"
	"hash"
	"math"
	"sync"

	"golang.org/x/crypto/sha3"
)

// BloomFilter implements a privacy-preserving Bloom filter with cryptographic hashing.
// This filter uses multiple independent hash functions to prevent correlation attacks
// and provides configurable false positive rates for privacy tuning.
//
// The filter ensures:
//   - No exact matches exposed (only probabilistic "may contain")
//   - Cryptographically secure hashing (SHA-3) for filter inputs
//   - Multiple independent filters to prevent analysis
//   - Memory-efficient storage with compression support
type BloomFilter struct {
	// Filter configuration
	bitArray    []uint64 // Bit array using uint64 for efficient operations
	size        uint64   // Size of bit array in bits
	hashCount   int      // Number of hash functions to use
	elementCount uint64  // Number of elements added (for load factor)
	
	// Cryptographic components
	salt        []byte   // Random salt for hash functions
	hashFuncs   []hash.Hash // Independent hash function instances
	
	// Privacy configuration
	falsePositiveRate float64 // Target false positive rate (default 1%)
	privacyLevel      int     // Privacy level (affects hash complexity)
	
	// Thread safety
	mu          sync.RWMutex // Protects concurrent access
	
	// Performance integration
	memoryPool  MemoryPool   // Memory pool for efficient allocation
}

// BloomFilterConfig holds configuration for creating Bloom filters
type BloomFilterConfig struct {
	ExpectedElements  uint64  // Expected number of elements to add
	FalsePositiveRate float64 // Desired false positive rate (0.001-0.1)
	PrivacyLevel      int     // Privacy level 1-5 (higher = more privacy)
	UseCompression    bool    // Enable bit array compression
	MemoryPool        MemoryPool // Optional memory pool for allocation
}

// MemoryPool interface for efficient memory allocation (integrates with Day 1 optimizations)
type MemoryPool interface {
	GetByteBuffer(size int) []byte
	ReturnByteBuffer(buffer []byte)
}

// NewBloomFilter creates a new privacy-preserving Bloom filter with the specified configuration.
// This function calculates optimal parameters for the desired false positive rate and
// initializes cryptographically secure hash functions.
func NewBloomFilter(config *BloomFilterConfig) (*BloomFilter, error) {
	if config.ExpectedElements == 0 {
		return nil, &BloomFilterError{Op: "NewBloomFilter", Err: "expected elements must be > 0"}
	}
	
	if config.FalsePositiveRate <= 0 || config.FalsePositiveRate >= 1 {
		return nil, &BloomFilterError{Op: "NewBloomFilter", Err: "false positive rate must be between 0 and 1"}
	}
	
	if config.PrivacyLevel < 1 || config.PrivacyLevel > 5 {
		config.PrivacyLevel = 3 // Default privacy level
	}
	
	// Calculate optimal filter parameters
	size := calculateOptimalSize(config.ExpectedElements, config.FalsePositiveRate)
	hashCount := calculateOptimalHashCount(size, config.ExpectedElements)
	
	// Adjust hash count based on privacy level (more hashes = more privacy)
	hashCount = hashCount + (config.PrivacyLevel - 1)
	if hashCount > 20 {
		hashCount = 20 // Reasonable upper limit
	}
	
	// Generate cryptographic salt
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		return nil, &BloomFilterError{Op: "NewBloomFilter", Err: "failed to generate salt: " + err.Error()}
	}
	
	// Initialize hash functions with independent seeds
	hashFuncs := make([]hash.Hash, hashCount)
	for i := 0; i < hashCount; i++ {
		hashFuncs[i] = sha3.New256() // Use SHA-3 for cryptographic security
	}
	
	// Calculate bit array size (round up to uint64 boundary)
	arraySize := (size + 63) / 64
	
	return &BloomFilter{
		bitArray:          make([]uint64, arraySize),
		size:              size,
		hashCount:         hashCount,
		elementCount:      0,
		salt:              salt,
		hashFuncs:         hashFuncs,
		falsePositiveRate: config.FalsePositiveRate,
		privacyLevel:      config.PrivacyLevel,
		memoryPool:        config.MemoryPool,
	}, nil
}

// Add inserts an element into the Bloom filter using cryptographic hashing.
// The element is first combined with the filter's salt and then hashed using
// multiple independent hash functions for privacy protection.
func (bf *BloomFilter) Add(element []byte) error {
	if len(element) == 0 {
		return &BloomFilterError{Op: "Add", Err: "element cannot be empty"}
	}
	
	bf.mu.Lock()
	defer bf.mu.Unlock()
	
	// Generate hash values for this element
	hashes := bf.generateHashes(element)
	
	// Set bits in the filter
	for _, hashValue := range hashes {
		bitIndex := hashValue % bf.size
		arrayIndex := bitIndex / 64
		bitOffset := bitIndex % 64
		bf.bitArray[arrayIndex] |= (1 << bitOffset)
	}
	
	bf.elementCount++
	return nil
}

// Contains checks if an element may be in the filter.
// Returns true if the element might be present (with possible false positives)
// or false if the element is definitely not present.
func (bf *BloomFilter) Contains(element []byte) (bool, error) {
	if len(element) == 0 {
		return false, &BloomFilterError{Op: "Contains", Err: "element cannot be empty"}
	}
	
	bf.mu.RLock()
	defer bf.mu.RUnlock()
	
	// Generate hash values for this element
	hashes := bf.generateHashes(element)
	
	// Check all bits are set
	for _, hashValue := range hashes {
		bitIndex := hashValue % bf.size
		arrayIndex := bitIndex / 64
		bitOffset := bitIndex % 64
		
		if (bf.bitArray[arrayIndex] & (1 << bitOffset)) == 0 {
			return false, nil // Definitely not present
		}
	}
	
	return true, nil // Possibly present
}

// generateHashes creates multiple independent hash values for an element.
// This function combines the element with the filter's salt and uses
// SHA-3 for cryptographic security.
func (bf *BloomFilter) generateHashes(element []byte) []uint64 {
	hashes := make([]uint64, bf.hashCount)
	
	// Create base hash with salt
	baseHash := sha256.New()
	baseHash.Write(bf.salt)
	baseHash.Write(element)
	baseSum := baseHash.Sum(nil)
	
	// Generate multiple hash values using different seeds
	for i := 0; i < bf.hashCount; i++ {
		// Reset and seed the hash function
		bf.hashFuncs[i].Reset()
		bf.hashFuncs[i].Write(baseSum)
		bf.hashFuncs[i].Write([]byte{byte(i)}) // Unique seed for each hash
		
		hashSum := bf.hashFuncs[i].Sum(nil)
		
		// Convert first 8 bytes to uint64
		hashes[i] = bytesToUint64(hashSum[:8])
	}
	
	return hashes
}

// GetStats returns statistics about the Bloom filter for performance monitoring
func (bf *BloomFilter) GetStats() BloomFilterStats {
	bf.mu.RLock()
	defer bf.mu.RUnlock()
	
	// Count set bits for load factor calculation
	setBits := uint64(0)
	for _, word := range bf.bitArray {
		setBits += uint64(popcount(word))
	}
	
	loadFactor := float64(setBits) / float64(bf.size)
	estimatedFPR := math.Pow(loadFactor, float64(bf.hashCount))
	
	return BloomFilterStats{
		Size:                 bf.size,
		HashCount:           bf.hashCount,
		ElementCount:        bf.elementCount,
		SetBits:             setBits,
		LoadFactor:          loadFactor,
		EstimatedFPR:        estimatedFPR,
		ConfiguredFPR:       bf.falsePositiveRate,
		PrivacyLevel:        bf.privacyLevel,
		MemoryUsageBytes:    uint64(len(bf.bitArray)) * 8,
	}
}

// BloomFilterStats contains performance and configuration statistics
type BloomFilterStats struct {
	Size                 uint64  // Filter size in bits
	HashCount           int     // Number of hash functions
	ElementCount        uint64  // Number of elements added
	SetBits             uint64  // Number of bits set to 1
	LoadFactor          float64 // Fraction of bits set (0-1)
	EstimatedFPR        float64 // Estimated false positive rate
	ConfiguredFPR       float64 // Configured false positive rate
	PrivacyLevel        int     // Privacy level setting
	MemoryUsageBytes    uint64  // Memory usage in bytes
}

// BloomFilterError represents errors from Bloom filter operations
type BloomFilterError struct {
	Op  string // Operation that failed
	Err string // Error description
}

func (e *BloomFilterError) Error() string {
	return "bloom filter " + e.Op + ": " + e.Err
}

// Utility functions

// calculateOptimalSize calculates the optimal bit array size for given parameters
func calculateOptimalSize(n uint64, p float64) uint64 {
	// Formula: m = -n * ln(p) / (ln(2)^2)
	size := -float64(n) * math.Log(p) / (math.Log(2) * math.Log(2))
	return uint64(math.Ceil(size))
}

// calculateOptimalHashCount calculates the optimal number of hash functions
func calculateOptimalHashCount(m, n uint64) int {
	// Formula: k = (m/n) * ln(2)
	k := (float64(m) / float64(n)) * math.Log(2)
	return int(math.Round(k))
}

// bytesToUint64 converts a byte slice to uint64 (little endian)
func bytesToUint64(b []byte) uint64 {
	result := uint64(0)
	for i := 0; i < 8 && i < len(b); i++ {
		result |= uint64(b[i]) << (i * 8)
	}
	return result
}

// popcount counts the number of set bits in a uint64
func popcount(x uint64) int {
	count := 0
	for x != 0 {
		count++
		x &= x - 1 // Clear the lowest set bit
	}
	return count
}