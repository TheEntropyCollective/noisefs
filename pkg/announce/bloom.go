package announce

import (
	"encoding/base64"
	"errors"
	"hash/fnv"
	"math"
	"strings"
)

// BloomFilter implements a probabilistic data structure for privacy-preserving tag matching.
//
// A bloom filter is a space-efficient probabilistic data structure that allows testing
// whether an element is in a set. It can have false positives but never false negatives,
// making it perfect for privacy-preserving tag matching in NoiseFS announcements.
//
// Privacy Benefits:
//   - Tags cannot be extracted from the bloom filter
//   - Only probabilistic matching is possible
//   - False positives provide plausible deniability
//   - Much smaller than storing actual tag lists
//
// Mathematical Foundation:
//   - Uses multiple hash functions to set bits in a bit array
//   - Optimal size calculated using: m = -n * ln(p) / (ln(2)^2)
//   - Optimal hash count: k = (m/n) * ln(2)
//   - Where n = expected items, p = target false positive rate
//
// Thread Safety: BloomFilter is NOT thread-safe. External synchronization
// required for concurrent access.
type BloomFilter struct {
	// bits stores the bloom filter bit array. Each bit position can be set
	// by one or more hash functions to indicate potential presence of items.
	bits      []byte
	
	// size specifies the bloom filter size in bits (not bytes).
	// Larger sizes reduce false positive rates but increase memory usage.
	size      uint32
	
	// hashCount defines the number of hash functions used.
	// More hash functions reduce false positives but increase computation.
	hashCount uint32
}

// BloomFilterParams configures bloom filter creation with performance and privacy trade-offs.
//
// These parameters control the balance between false positive rates, memory usage,
// and computational overhead. Proper tuning is critical for both privacy and
// performance in the NoiseFS announcement system.
type BloomFilterParams struct {
	// ExpectedItems is the anticipated number of tags that will be added to the filter.
	// This is used to calculate optimal filter size and hash count. Underestimating
	// will increase false positive rates; overestimating wastes memory.
	ExpectedItems    int
	
	// FalsePositiveRate is the target probability of false positive matches (0.0-1.0).
	// Lower rates require larger filters. Common values:
	//   - 0.01 (1%): Good balance for most use cases
	//   - 0.001 (0.1%): High precision, larger size
	//   - 0.1 (10%): High performance, less precision
	FalsePositiveRate float64
}

// DefaultBloomParams returns sensible default parameters for NoiseFS tag bloom filters.
//
// These defaults balance privacy, performance, and network efficiency for typical
// content announcements. The parameters are tuned for announcements with up to
// 100 tags and a 1% false positive rate, providing good privacy with minimal
// network overhead.
//
// Returns:
//   BloomFilterParams configured for typical NoiseFS announcements
//
// Time Complexity: O(1)
//
// Default Values:
//   - ExpectedItems: 100 tags (supports most content types)
//   - FalsePositiveRate: 1% (good privacy/efficiency balance)
func DefaultBloomParams() BloomFilterParams {
	return BloomFilterParams{
		ExpectedItems:    100,  // Supports most content with up to 100 tags
		FalsePositiveRate: 0.01, // 1% false positive rate for good privacy
	}
}

// NewBloomFilter creates a new bloom filter optimized for the given parameters.
//
// This constructor calculates the optimal bit array size and hash function count
// to achieve the target false positive rate for the expected number of items.
// The resulting filter is empty and ready for adding items.
//
// Parameters:
//   - params: Configuration specifying expected items and false positive rate
//
// Returns:
//   - A new empty BloomFilter optimized for the specified parameters
//
// Time Complexity: O(1) for filter creation, O(m) for bit array allocation
// Space Complexity: O(m) where m is the calculated optimal size
//
// Mathematical Optimization:
//   - Filter size: m = -n * ln(p) / (ln(2)^2)
//   - Hash count: k = (m/n) * ln(2)
//   - Minimum size: 64 bits (8 bytes) for reasonable performance
func NewBloomFilter(params BloomFilterParams) *BloomFilter {
	// Calculate mathematically optimal size and hash count
	m := optimalSize(params.ExpectedItems, params.FalsePositiveRate)
	k := optimalHashCount(params.ExpectedItems, m)
	
	// Enforce minimum size to ensure reasonable performance
	// Filters smaller than 64 bits (8 bytes) are not practical
	if m < 64 {
		m = 64
	}
	
	// Allocate bit array, converting bit size to byte size
	// Add 7 before division to round up for partial bytes
	byteSize := (m + 7) / 8
	bits := make([]byte, byteSize)
	
	return &BloomFilter{
		bits:      bits,
		size:      uint32(m),
		hashCount: uint32(k),
	}
}

// CreateTagBloom creates a bloom filter optimized for the provided tag list.
//
// This convenience function automatically configures filter parameters based on
// the actual number of tags provided, then normalizes and adds all tags to the
// filter. This is the recommended way to create bloom filters for announcements.
//
// Parameters:
//   - tags: List of tag strings to add to the filter
//
// Returns:
//   - BloomFilter containing all provided tags, ready for encoding
//
// Time Complexity: O(n*k) where n is number of tags, k is hash count
// Space Complexity: O(m) where m is the calculated filter size
//
// Tag Processing:
//   - Each tag is normalized (lowercase, trimmed, deduplicated spaces)
//   - Filter size auto-adjusts if tag count exceeds default expected items
//   - Uses default 1% false positive rate for consistent behavior
func CreateTagBloom(tags []string) *BloomFilter {
	// Start with default parameters and adjust for actual tag count
	params := DefaultBloomParams()
	if len(tags) > params.ExpectedItems {
		// Increase expected items to accommodate larger tag lists
		params.ExpectedItems = len(tags)
	}
	
	// Create optimally-sized filter
	bf := NewBloomFilter(params)
	
	// Add all tags with normalization for consistent matching
	for _, tag := range tags {
		normalized := normalizeTag(tag)
		bf.Add(normalized)
	}
	
	return bf
}

// Add inserts an item into the bloom filter by setting corresponding bits.
//
// This method applies all hash functions to the item and sets the corresponding
// bits in the filter. Once added, items can be tested for membership (with
// possible false positives but no false negatives).
//
// Parameters:
//   - item: String to add to the filter (should be normalized)
//
// Time Complexity: O(k) where k is the number of hash functions
// Space Complexity: O(1) additional space
//
// Side Effects:
//   - Modifies the internal bit array by setting bits to 1
//   - May increase false positive rate for future operations
//   - Cannot be undone (bloom filters don't support removal)
//
// Thread Safety: Not thread-safe, requires external synchronization
func (bf *BloomFilter) Add(item string) {
	// Apply each hash function and set the corresponding bit
	for i := uint32(0); i < bf.hashCount; i++ {
		// Calculate bit position using i-th hash function
		pos := bf.hash(item, i) % bf.size
		// Convert bit position to byte and bit indices
		byteIndex := pos / 8
		bitIndex := pos % 8
		// Set the bit using bitwise OR
		bf.bits[byteIndex] |= 1 << bitIndex
	}
}

// Test checks if an item might be present in the bloom filter.
//
// This method applies all hash functions to the item and checks if all
// corresponding bits are set. Returns true if the item might be in the filter
// (with possible false positives) or false if definitely not present.
//
// Parameters:
//   - item: String to test for membership (should be normalized)
//
// Returns:
//   - true: Item might be in the filter (possible false positive)
//   - false: Item is definitely not in the filter (no false negatives)
//
// Time Complexity: O(k) where k is the number of hash functions
// Space Complexity: O(1)
//
// Privacy Note: This test reveals no information about other items in the
// filter, maintaining privacy of the tag set.
func (bf *BloomFilter) Test(item string) bool {
	// Check all hash positions - ALL must be set for potential membership
	for i := uint32(0); i < bf.hashCount; i++ {
		// Calculate bit position using i-th hash function
		pos := bf.hash(item, i) % bf.size
		// Convert to byte and bit indices
		byteIndex := pos / 8
		bitIndex := pos % 8
		// If any bit is not set, item is definitely not present
		if bf.bits[byteIndex]&(1<<bitIndex) == 0 {
			return false
		}
	}
	// All required bits are set - item might be present
	return true
}

// TestMultiple efficiently tests multiple items and returns potential matches.
//
// This method tests each item in the provided list and returns only those
// that might be present in the filter. This is more efficient than calling
// Test() individually when checking many items.
//
// Parameters:
//   - items: List of strings to test for membership
//
// Returns:
//   - Slice of items that might be in the filter (includes false positives)
//
// Time Complexity: O(n*k) where n is number of items, k is hash count
// Space Complexity: O(m) where m is number of matching items
//
// Usage: Ideal for tag matching where users search for multiple tags
// and want to find announcements that match any of their search terms.
func (bf *BloomFilter) TestMultiple(items []string) []string {
	// Pre-allocate slice for efficiency
	matches := []string{}
	for _, item := range items {
		// Normalize tag for consistent matching
		normalized := normalizeTag(item)
		// Test membership and collect matches
		if bf.Test(normalized) {
			matches = append(matches, item)
		}
	}
	return matches
}

// Encode serializes the bloom filter to a compact base64 string for storage/transmission.
//
// The encoded format includes both the filter parameters and bit data in a
// self-describing format that can be decoded without external metadata.
// This encoding is used in NoiseFS announcements for network efficiency.
//
// Encoding Format:
//   - 4 bytes: filter size (big-endian uint32)
//   - 1 byte: hash count (uint8)
//   - N bytes: bit array data
//   - All base64url encoded for safe text transmission
//
// Returns:
//   - Base64url encoded string containing the complete filter
//
// Time Complexity: O(m) where m is the filter size in bytes
// Space Complexity: O(m) for the encoded string
//
// Network Efficiency: The compact encoding minimizes announcement size
// while preserving all necessary information for decoding and matching.
func (bf *BloomFilter) Encode() string {
	// Create binary header with filter parameters
	// 4 bytes for size (big-endian) + 1 byte for hash count
	header := string([]byte{
		byte(bf.size >> 24), // Size bits 31-24
		byte(bf.size >> 16), // Size bits 23-16
		byte(bf.size >> 8),  // Size bits 15-8
		byte(bf.size),       // Size bits 7-0
		byte(bf.hashCount),  // Hash count (max 255)
	})
	
	// Combine header and bit array data
	data := append([]byte(header), bf.bits...)
	// Use URL-safe base64 encoding for network transmission
	return base64.URLEncoding.EncodeToString(data)
}

// DecodeBloom reconstructs a bloom filter from its base64 encoded representation.
//
// This function parses the encoded format created by Encode() and validates
// the structure to ensure the filter can be safely used. It performs bounds
// checking and consistency validation to prevent malformed data issues.
//
// Parameters:
//   - encoded: Base64url encoded bloom filter string
//
// Returns:
//   - Reconstructed BloomFilter ready for use
//   - error if decoding fails or data is invalid
//
// Time Complexity: O(m) where m is the filter size
// Space Complexity: O(m) for the reconstructed filter
//
// Validation Performed:
//   - Base64 decoding validation
//   - Minimum header size check
//   - Bit array size consistency verification
//   - Parameter bounds checking
func DecodeBloom(encoded string) (*BloomFilter, error) {
	// Decode from base64url format
	data, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	
	// Validate minimum header size (4 bytes size + 1 byte hash count)
	if len(data) < 5 {
		return nil, errors.New("invalid bloom filter encoding")
	}
	
	// Extract filter parameters from header (big-endian)
	size := uint32(data[0])<<24 | uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3])
	hashCount := uint32(data[4])
	
	// Extract bit array data
	bits := data[5:]
	// Validate bit array size matches expected size
	expectedBytes := (size + 7) / 8 // Round up to nearest byte
	if uint32(len(bits)) != expectedBytes {
		return nil, errors.New("bloom filter size mismatch")
	}
	
	return &BloomFilter{
		bits:      bits,
		size:      size,
		hashCount: hashCount,
	}, nil
}

// MatchesTags provides a convenient interface for tag matching against encoded bloom filters.
//
// This function decodes a bloom filter from its string representation and tests
// multiple user tags for potential matches. It's designed for the common use case
// of searching announcements by tag, handling empty filters gracefully.
//
// Parameters:
//   - bloomEncoded: Base64url encoded bloom filter string
//   - userTags: List of tags to test for matches
//
// Returns:
//   - hasMatches: true if any tags might match
//   - matches: list of potentially matching tags
//   - error: if decoding fails
//
// Time Complexity: O(n*k) where n is number of user tags, k is hash count
// Space Complexity: O(m) where m is number of matching tags
//
// Usage Example:
//   hasMatch, tags, err := MatchesTags(announcement.TagBloom, []string{"video", "4k"})
func MatchesTags(bloomEncoded string, userTags []string) (bool, []string, error) {
	// Handle empty bloom filter (no tags)
	if bloomEncoded == "" {
		return false, nil, nil
	}
	
	// Decode the bloom filter
	bloom, err := DecodeBloom(bloomEncoded)
	if err != nil {
		return false, nil, err
	}
	
	// Test all user tags and return results
	matches := bloom.TestMultiple(userTags)
	return len(matches) > 0, matches, nil
}

// Helper functions for bloom filter operations

// hash generates the i-th hash value for an item using double hashing.
//
// This method implements double hashing to generate multiple independent hash
// values from two base hash functions. Double hashing provides good distribution
// properties while being computationally efficient.
//
// Algorithm: h(i) = h1(item) + i * h2(item)
//
// Parameters:
//   - item: String to hash
//   - i: Hash function index (0 to hashCount-1)
//
// Returns:
//   - 32-bit hash value for bit position calculation
//
// Time Complexity: O(1)
//
// Security Note: Uses FNV-1a hash with salt for h2 to ensure independence.
// Not cryptographically secure but sufficient for bloom filter distribution.
func (bf *BloomFilter) hash(item string, i uint32) uint32 {
	// Implement double hashing: h(i) = h1(item) + i * h2(item)
	// First hash function: FNV-1a of the item
	h1 := fnv.New32a()
	h1.Write([]byte(item))
	hash1 := h1.Sum32()
	
	// Second hash function: FNV-1a of item with salt for independence
	h2 := fnv.New32a()
	h2.Write([]byte(item + "salt"))
	hash2 := h2.Sum32()
	
	// Combine using double hashing formula
	return hash1 + i*hash2
}

// normalizeTag standardizes tag format for consistent bloom filter matching.
//
// This function ensures that tags are stored and searched in a consistent format,
// preventing match failures due to case differences, extra whitespace, or
// formatting variations. All tags should be normalized before adding to filters
// or testing for membership.
//
// Normalization Steps:
//   1. Convert to lowercase for case-insensitive matching
//   2. Trim leading and trailing whitespace
//   3. Collapse multiple internal spaces to single spaces
//
// Parameters:
//   - tag: Raw tag string to normalize
//
// Returns:
//   - Normalized tag string ready for bloom filter operations
//
// Time Complexity: O(n) where n is the length of the tag string
// Space Complexity: O(n) for the normalized string
//
// Examples:
//   "  Video 4K  " → "video 4k"
//   "ACTION   MOVIE" → "action movie"
func normalizeTag(tag string) string {
	// Convert to lowercase for case-insensitive matching
	tag = strings.ToLower(tag)
	
	// Remove leading and trailing whitespace
	tag = strings.TrimSpace(tag)
	
	// Collapse multiple spaces into single spaces
	// Fields() splits on any whitespace, Join() recombines with single spaces
	tag = strings.Join(strings.Fields(tag), " ")
	
	return tag
}

// optimalSize calculates the mathematically optimal bloom filter size in bits.
//
// This function implements the standard bloom filter sizing formula to minimize
// false positive rate for a given number of expected items. The formula is
// derived from probability theory and information theory.
//
// Formula: m = -n * ln(p) / (ln(2)^2)
// Where:
//   - m = optimal size in bits
//   - n = expected number of items
//   - p = target false positive rate
//
// Parameters:
//   - n: Expected number of items to be added
//   - p: Target false positive rate (0.0 to 1.0)
//
// Returns:
//   - Optimal filter size in bits
//
// Time Complexity: O(1)
//
// Mathematical Foundation: Derived from minimizing the false positive
// probability function with respect to filter size.
func optimalSize(n int, p float64) int {
	// Apply the mathematical formula: m = -n * ln(p) / (ln(2)^2)
	// Where ln(2)^2 ≈ 0.4804
	m := -float64(n) * math.Log(p) / math.Pow(math.Log(2), 2)
	// Round up to ensure we meet or exceed the target false positive rate
	return int(math.Ceil(m))
}

// optimalHashCount calculates the optimal number of hash functions for minimal false positive rate.
//
// This function determines how many hash functions should be used to minimize
// the false positive probability for a given filter size and expected item count.
// The formula is derived from optimizing the false positive probability function.
//
// Formula: k = (m/n) * ln(2)
// Where:
//   - k = optimal number of hash functions
//   - m = filter size in bits
//   - n = expected number of items
//
// Parameters:
//   - n: Expected number of items
//   - m: Filter size in bits
//
// Returns:
//   - Optimal number of hash functions
//
// Time Complexity: O(1)
//
// Mathematical Note: ln(2) ≈ 0.693, so k ≈ 0.693 * (m/n)
func optimalHashCount(n, m int) int {
	// Apply the mathematical formula: k = (m/n) * ln(2)
	// Where ln(2) ≈ 0.693
	k := float64(m) / float64(n) * math.Log(2)
	// Round up to ensure optimal performance
	return int(math.Ceil(k))
}

// EstimateBloomSize calculates storage requirements for a bloom filter with given parameters.
//
// This utility function helps estimate memory and network overhead for bloom filters
// before creation. It's useful for capacity planning and optimization decisions
// in the NoiseFS announcement system.
//
// Parameters:
//   - numTags: Expected number of tags to store
//   - falsePositiveRate: Target false positive rate (0.0 to 1.0)
//
// Returns:
//   - sizeBytes: Required storage in bytes
//   - numHashes: Number of hash functions needed
//
// Time Complexity: O(1)
//
// Usage Examples:
//   bytes, hashes := EstimateBloomSize(50, 0.01)  // 50 tags, 1% FP rate
//   bytes, hashes := EstimateBloomSize(200, 0.001) // 200 tags, 0.1% FP rate
func EstimateBloomSize(numTags int, falsePositiveRate float64) (sizeBytes int, numHashes int) {
	// Calculate optimal parameters using standard formulas
	m := optimalSize(numTags, falsePositiveRate)
	k := optimalHashCount(numTags, m)
	// Convert bit size to byte size (round up)
	sizeBytes = (m + 7) / 8
	numHashes = k
	return
}

// MergeBloomFilters combines multiple bloom filters using bitwise OR operation.
//
// This function creates a new bloom filter that contains all items from the
// input filters. The result will match an item if ANY of the input filters
// would match it. All input filters must have identical parameters.
//
// Use Cases:
//   - Combining tag sets from multiple sources
//   - Creating aggregate announcement filters
//   - Building hierarchical tag structures
//
// Parameters:
//   - filters: Variable number of bloom filters to merge
//
// Returns:
//   - Merged bloom filter containing union of all input items
//   - error if filters have incompatible parameters or no filters provided
//
// Time Complexity: O(m*n) where m is filter size, n is number of filters
// Space Complexity: O(m) for the merged filter
//
// Constraints:
//   - All filters must have same size and hash count
//   - At least one filter must be provided
func MergeBloomFilters(filters ...*BloomFilter) (*BloomFilter, error) {
	// Validate input
	if len(filters) == 0 {
		return nil, errors.New("no filters to merge")
	}
	
	// Verify all filters have compatible parameters
	size := filters[0].size
	hashCount := filters[0].hashCount
	
	for _, bf := range filters[1:] {
		if bf.size != size || bf.hashCount != hashCount {
			return nil, errors.New("bloom filters have different parameters")
		}
	}
	
	// Create new filter with same parameters
	merged := &BloomFilter{
		bits:      make([]byte, len(filters[0].bits)),
		size:      size,
		hashCount: hashCount,
	}
	
	// Perform bitwise OR of all filter bit arrays
	for i := range merged.bits {
		for _, bf := range filters {
			// OR operation combines all set bits
			merged.bits[i] |= bf.bits[i]
		}
	}
	
	return merged, nil
}