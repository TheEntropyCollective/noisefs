package announce

import (
	"encoding/base64"
	"errors"
	"hash/fnv"
	"math"
	"strings"
)

// BloomFilter represents a bloom filter for privacy-preserving tag matching
type BloomFilter struct {
	bits      []byte
	size      uint32 // size in bits
	hashCount uint32 // number of hash functions
}

// BloomFilterParams holds parameters for bloom filter creation
type BloomFilterParams struct {
	ExpectedItems    int     // Expected number of items
	FalsePositiveRate float64 // Target false positive rate (e.g., 0.01 for 1%)
}

// DefaultBloomParams returns default bloom filter parameters
func DefaultBloomParams() BloomFilterParams {
	return BloomFilterParams{
		ExpectedItems:    100,  // Up to 100 tags
		FalsePositiveRate: 0.01, // 1% false positive rate
	}
}

// NewBloomFilter creates a new bloom filter with specified parameters
func NewBloomFilter(params BloomFilterParams) *BloomFilter {
	// Calculate optimal size and hash count
	m := optimalSize(params.ExpectedItems, params.FalsePositiveRate)
	k := optimalHashCount(params.ExpectedItems, m)
	
	// Ensure minimum size
	if m < 64 {
		m = 64
	}
	
	// Allocate bit array
	byteSize := (m + 7) / 8
	bits := make([]byte, byteSize)
	
	return &BloomFilter{
		bits:      bits,
		size:      uint32(m),
		hashCount: uint32(k),
	}
}

// CreateTagBloom creates a bloom filter from a list of tags
func CreateTagBloom(tags []string) *BloomFilter {
	params := DefaultBloomParams()
	if len(tags) > params.ExpectedItems {
		params.ExpectedItems = len(tags)
	}
	
	bf := NewBloomFilter(params)
	
	for _, tag := range tags {
		normalized := normalizeTag(tag)
		bf.Add(normalized)
	}
	
	return bf
}

// Add adds an item to the bloom filter
func (bf *BloomFilter) Add(item string) {
	for i := uint32(0); i < bf.hashCount; i++ {
		pos := bf.hash(item, i) % bf.size
		byteIndex := pos / 8
		bitIndex := pos % 8
		bf.bits[byteIndex] |= 1 << bitIndex
	}
}

// Test checks if an item might be in the bloom filter
func (bf *BloomFilter) Test(item string) bool {
	for i := uint32(0); i < bf.hashCount; i++ {
		pos := bf.hash(item, i) % bf.size
		byteIndex := pos / 8
		bitIndex := pos % 8
		if bf.bits[byteIndex]&(1<<bitIndex) == 0 {
			return false
		}
	}
	return true
}

// TestMultiple tests multiple items and returns matches
func (bf *BloomFilter) TestMultiple(items []string) []string {
	matches := []string{}
	for _, item := range items {
		normalized := normalizeTag(item)
		if bf.Test(normalized) {
			matches = append(matches, item)
		}
	}
	return matches
}

// Encode encodes the bloom filter to base64
func (bf *BloomFilter) Encode() string {
	// Format: size:hashCount:base64(bits)
	header := string([]byte{
		byte(bf.size >> 24),
		byte(bf.size >> 16),
		byte(bf.size >> 8),
		byte(bf.size),
		byte(bf.hashCount),
	})
	
	data := append([]byte(header), bf.bits...)
	return base64.URLEncoding.EncodeToString(data)
}

// DecodeBloom decodes a bloom filter from base64
func DecodeBloom(encoded string) (*BloomFilter, error) {
	data, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	
	if len(data) < 5 {
		return nil, errors.New("invalid bloom filter encoding")
	}
	
	// Extract header
	size := uint32(data[0])<<24 | uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3])
	hashCount := uint32(data[4])
	
	// Extract bits
	bits := data[5:]
	expectedBytes := (size + 7) / 8
	if uint32(len(bits)) != expectedBytes {
		return nil, errors.New("bloom filter size mismatch")
	}
	
	return &BloomFilter{
		bits:      bits,
		size:      size,
		hashCount: hashCount,
	}, nil
}

// MatchesTags checks if any of the user's tags match the bloom filter
func MatchesTags(bloomEncoded string, userTags []string) (bool, []string, error) {
	if bloomEncoded == "" {
		return false, nil, nil
	}
	
	bloom, err := DecodeBloom(bloomEncoded)
	if err != nil {
		return false, nil, err
	}
	
	matches := bloom.TestMultiple(userTags)
	return len(matches) > 0, matches, nil
}

// Helper functions

// hash generates the i-th hash for an item
func (bf *BloomFilter) hash(item string, i uint32) uint32 {
	// Use double hashing: h(i) = h1 + i*h2
	h1 := fnv.New32a()
	h1.Write([]byte(item))
	hash1 := h1.Sum32()
	
	h2 := fnv.New32a()
	h2.Write([]byte(item + "salt"))
	hash2 := h2.Sum32()
	
	return hash1 + i*hash2
}

// normalizeTag normalizes a tag for consistent matching
func normalizeTag(tag string) string {
	// Convert to lowercase
	tag = strings.ToLower(tag)
	
	// Trim spaces
	tag = strings.TrimSpace(tag)
	
	// Remove extra spaces
	tag = strings.Join(strings.Fields(tag), " ")
	
	return tag
}

// optimalSize calculates optimal bloom filter size in bits
func optimalSize(n int, p float64) int {
	// m = -n * ln(p) / (ln(2)^2)
	m := -float64(n) * math.Log(p) / math.Pow(math.Log(2), 2)
	return int(math.Ceil(m))
}

// optimalHashCount calculates optimal number of hash functions
func optimalHashCount(n, m int) int {
	// k = (m/n) * ln(2)
	k := float64(m) / float64(n) * math.Log(2)
	return int(math.Ceil(k))
}

// EstimateBloomSize estimates the size of a bloom filter for given parameters
func EstimateBloomSize(numTags int, falsePositiveRate float64) (sizeBytes int, numHashes int) {
	m := optimalSize(numTags, falsePositiveRate)
	k := optimalHashCount(numTags, m)
	sizeBytes = (m + 7) / 8
	numHashes = k
	return
}

// MergeBloomFilters merges multiple bloom filters (OR operation)
func MergeBloomFilters(filters ...*BloomFilter) (*BloomFilter, error) {
	if len(filters) == 0 {
		return nil, errors.New("no filters to merge")
	}
	
	// Check compatibility
	size := filters[0].size
	hashCount := filters[0].hashCount
	
	for _, bf := range filters[1:] {
		if bf.size != size || bf.hashCount != hashCount {
			return nil, errors.New("bloom filters have different parameters")
		}
	}
	
	// Create merged filter
	merged := &BloomFilter{
		bits:      make([]byte, len(filters[0].bits)),
		size:      size,
		hashCount: hashCount,
	}
	
	// OR all bits
	for i := range merged.bits {
		for _, bf := range filters {
			merged.bits[i] |= bf.bits[i]
		}
	}
	
	return merged, nil
}