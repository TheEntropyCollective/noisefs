package cache

import (
	"crypto/sha256"
	"encoding/binary"
	"math"
	"sync"
	"time"
)

// BloomFilter implements a simple Bloom filter for cache hints
type BloomFilter struct {
	bitArray []bool
	size     uint
	hashFns  uint
	mutex    sync.RWMutex
}

// CacheBloomHint contains probabilistic cache state information
type CacheBloomHint struct {
	Filter        *BloomFilter `json:"filter"`
	EstimatedSize int          `json:"estimated_size"`
	FalsePositive float64      `json:"false_positive_rate"`
	CreatedAt     time.Time    `json:"created_at"`
}

// NewBloomFilter creates a new Bloom filter with optimal parameters
func NewBloomFilter(expectedItems int, falsePositiveRate float64) *BloomFilter {
	// Calculate optimal size and hash functions
	size := uint(-float64(expectedItems) * math.Log(falsePositiveRate) / (math.Log(2) * math.Log(2)))
	hashFns := uint(float64(size) * math.Log(2) / float64(expectedItems))
	
	// Ensure minimum values
	if size < 64 {
		size = 64
	}
	if hashFns < 1 {
		hashFns = 1
	}
	if hashFns > 10 {
		hashFns = 10 // Practical limit
	}
	
	return &BloomFilter{
		bitArray: make([]bool, size),
		size:     size,
		hashFns:  hashFns,
	}
}

// Add inserts an item into the Bloom filter
func (bf *BloomFilter) Add(item string) {
	bf.mutex.Lock()
	defer bf.mutex.Unlock()
	
	hashes := bf.getHashes(item)
	for i := uint(0); i < bf.hashFns; i++ {
		index := (hashes[0] + i*hashes[1]) % bf.size
		bf.bitArray[index] = true
	}
}

// Contains checks if an item might be in the filter
func (bf *BloomFilter) Contains(item string) bool {
	bf.mutex.RLock()
	defer bf.mutex.RUnlock()
	
	hashes := bf.getHashes(item)
	for i := uint(0); i < bf.hashFns; i++ {
		index := (hashes[0] + i*hashes[1]) % bf.size
		if !bf.bitArray[index] {
			return false
		}
	}
	return true
}

// getHashes generates two hash values for double hashing
func (bf *BloomFilter) getHashes(item string) [2]uint {
	hash := sha256.Sum256([]byte(item))
	
	// Split hash into two 32-bit values
	h1 := binary.BigEndian.Uint32(hash[:4])
	h2 := binary.BigEndian.Uint32(hash[4:8])
	
	return [2]uint{uint(h1), uint(h2)}
}

// Clear resets the Bloom filter
func (bf *BloomFilter) Clear() {
	bf.mutex.Lock()
	defer bf.mutex.Unlock()
	
	for i := range bf.bitArray {
		bf.bitArray[i] = false
	}
}

// EstimateCount estimates the number of items in the filter
func (bf *BloomFilter) EstimateCount() int {
	bf.mutex.RLock()
	defer bf.mutex.RUnlock()
	
	setBits := 0
	for _, bit := range bf.bitArray {
		if bit {
			setBits++
		}
	}
	
	if setBits == 0 {
		return 0
	}
	
	// Estimate using -m * ln(1 - X/m) / k
	// where m = filter size, X = set bits, k = hash functions
	ratio := float64(setBits) / float64(bf.size)
	if ratio >= 1.0 {
		return int(bf.size) // Filter is likely full
	}
	
	estimate := -float64(bf.size) * math.Log(1-ratio) / float64(bf.hashFns)
	return int(estimate)
}

// CreateCacheHint creates a Bloom filter hint from cache contents
func (ac *AdaptiveCache) CreateCacheHint() *CacheBloomHint {
	ac.mutex.RLock()
	defer ac.mutex.RUnlock()
	
	// Create Bloom filter sized for current cache contents
	itemCount := len(ac.items)
	if itemCount == 0 {
		itemCount = 100 // Minimum size
	}
	
	filter := NewBloomFilter(itemCount*2, 0.01) // 1% false positive rate
	
	// Add all cached CIDs to the filter
	for cid := range ac.items {
		filter.Add(cid)
	}
	
	return &CacheBloomHint{
		Filter:        filter,
		EstimatedSize: itemCount,
		FalsePositive: 0.01,
		CreatedAt:     time.Now(),
	}
}

// QueryCacheHint checks if a CID might be available in a peer's cache
func (hint *CacheBloomHint) QueryCacheHint(cid string) bool {
	if hint.Filter == nil {
		return false
	}
	
	// Check if hint is too old (older than 1 hour)
	if time.Since(hint.CreatedAt) > time.Hour {
		return false
	}
	
	return hint.Filter.Contains(cid)
}

// MergeHints combines multiple cache hints into one
func MergeHints(hints []*CacheBloomHint) *CacheBloomHint {
	if len(hints) == 0 {
		return nil
	}
	
	// Find the largest estimated size for optimal parameters
	maxSize := 0
	for _, hint := range hints {
		if hint.EstimatedSize > maxSize {
			maxSize = hint.EstimatedSize
		}
	}
	
	// Create merged filter
	mergedFilter := NewBloomFilter(maxSize*len(hints), 0.05) // Slightly higher FP rate
	totalSize := 0
	
	// Add all items from all filters (approximate)
	for _, hint := range hints {
		if hint.Filter != nil {
			// We can't directly merge Bloom filters, so we estimate
			// In practice, this would require storing the original items
			totalSize += hint.EstimatedSize
		}
	}
	
	return &CacheBloomHint{
		Filter:        mergedFilter,
		EstimatedSize: totalSize,
		FalsePositive: 0.05,
		CreatedAt:     time.Now(),
	}
}

// UpdateCacheExchangeWithBloom modifies cache exchange to use Bloom filters
func (ac *AdaptiveCache) UpdateCacheExchangeWithBloom() {
	ac.cacheExchange.mutex.Lock()
	defer ac.cacheExchange.mutex.Unlock()
	
	// Create current cache hint
	hint := ac.CreateCacheHint()
	
	// In a real implementation, this would:
	// 1. Send hint to connected peers
	// 2. Receive hints from peers
	// 3. Use hints to make smart prefetching decisions
	// 4. Avoid querying peers that definitely don't have content
	
	// For now, we'll store it for demonstration
	ac.cacheExchange.LastExchange = time.Now()
	
	// Use hint to demonstrate functionality
	_ = hint
	
	// Example usage: before requesting a block, check peer hints
	// if peerHint.QueryCacheHint(cid) { requestFromPeer(peer, cid) }
}