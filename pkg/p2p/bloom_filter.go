package p2p

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"sync"
)

// BloomFilter is a space-efficient probabilistic data structure
// for tracking block availability across peers
type BloomFilter struct {
	bitArray    []byte
	size        uint64
	hashCount   uint32
	elementCount uint64
	mutex       sync.RWMutex
}

// NewBloomFilter creates a new bloom filter with the specified parameters
// expectedElements: expected number of elements to be added
// falsePositiveRate: desired false positive rate (e.g., 0.01 for 1%)
func NewBloomFilter(expectedElements uint64, falsePositiveRate float64) *BloomFilter {
	// Calculate optimal size and hash count
	size := optimalSize(expectedElements, falsePositiveRate)
	hashCount := optimalHashCount(size, expectedElements)
	
	return &BloomFilter{
		bitArray:     make([]byte, (size+7)/8), // Round up to nearest byte
		size:         size,
		hashCount:    hashCount,
		elementCount: 0,
	}
}

// Add adds an element to the bloom filter
func (bf *BloomFilter) Add(element string) {
	bf.mutex.Lock()
	defer bf.mutex.Unlock()
	
	hashes := bf.generateHashes(element)
	
	for i := uint32(0); i < bf.hashCount; i++ {
		index := hashes[i] % bf.size
		byteIndex := index / 8
		bitIndex := index % 8
		bf.bitArray[byteIndex] |= 1 << bitIndex
	}
	
	bf.elementCount++
}

// Contains checks if an element might be in the bloom filter
// Returns true if the element might be present (with possibility of false positive)
// Returns false if the element is definitely not present
func (bf *BloomFilter) Contains(element string) bool {
	bf.mutex.RLock()
	defer bf.mutex.RUnlock()
	
	hashes := bf.generateHashes(element)
	
	for i := uint32(0); i < bf.hashCount; i++ {
		index := hashes[i] % bf.size
		byteIndex := index / 8
		bitIndex := index % 8
		
		if bf.bitArray[byteIndex]&(1<<bitIndex) == 0 {
			return false
		}
	}
	
	return true
}

// EstimateMatches estimates how many elements from a list might be in the filter
func (bf *BloomFilter) EstimateMatches(elements []string) int {
	matches := 0
	for _, element := range elements {
		if bf.Contains(element) {
			matches++
		}
	}
	return matches
}

// FalsePositiveRate calculates the current false positive rate
func (bf *BloomFilter) FalsePositiveRate() float64 {
	bf.mutex.RLock()
	defer bf.mutex.RUnlock()
	
	if bf.elementCount == 0 {
		return 0.0
	}
	
	// Calculate the probability that a bit is still 0
	// P(bit is 0) = (1 - 1/m)^(k*n)
	// where m = size, k = hashCount, n = elementCount
	probBitZero := math.Pow(1.0-1.0/float64(bf.size), float64(bf.hashCount*uint32(bf.elementCount)))
	
	// False positive rate = (1 - P(bit is 0))^k
	return math.Pow(1.0-probBitZero, float64(bf.hashCount))
}

// Clear resets the bloom filter
func (bf *BloomFilter) Clear() {
	bf.mutex.Lock()
	defer bf.mutex.Unlock()
	
	for i := range bf.bitArray {
		bf.bitArray[i] = 0
	}
	bf.elementCount = 0
}

// ElementCount returns the number of elements added
func (bf *BloomFilter) ElementCount() uint64 {
	bf.mutex.RLock()
	defer bf.mutex.RUnlock()
	return bf.elementCount
}

// Size returns the size of the bit array
func (bf *BloomFilter) Size() uint64 {
	return bf.size
}

// HashCount returns the number of hash functions used
func (bf *BloomFilter) HashCount() uint32 {
	return bf.hashCount
}

// Merge combines this bloom filter with another one
// Both filters must have the same size and hash count
func (bf *BloomFilter) Merge(other *BloomFilter) error {
	if bf.size != other.size || bf.hashCount != other.hashCount {
		return ErrIncompatibleFilters
	}
	
	bf.mutex.Lock()
	defer bf.mutex.Unlock()
	other.mutex.RLock()
	defer other.mutex.RUnlock()
	
	for i := range bf.bitArray {
		bf.bitArray[i] |= other.bitArray[i]
	}
	
	// Element count is approximate after merge
	bf.elementCount += other.elementCount
	
	return nil
}

// MarshalBinary implements binary marshaling
func (bf *BloomFilter) MarshalBinary() ([]byte, error) {
	bf.mutex.RLock()
	defer bf.mutex.RUnlock()
	
	// Create a buffer for: size(8) + hashCount(4) + elementCount(8) + bitArray
	bufferSize := 8 + 4 + 8 + len(bf.bitArray)
	buffer := make([]byte, bufferSize)
	
	offset := 0
	
	// Write size
	binary.BigEndian.PutUint64(buffer[offset:], bf.size)
	offset += 8
	
	// Write hash count
	binary.BigEndian.PutUint32(buffer[offset:], bf.hashCount)
	offset += 4
	
	// Write element count
	binary.BigEndian.PutUint64(buffer[offset:], bf.elementCount)
	offset += 8
	
	// Write bit array
	copy(buffer[offset:], bf.bitArray)
	
	return buffer, nil
}

// UnmarshalBinary implements binary unmarshaling
func (bf *BloomFilter) UnmarshalBinary(data []byte) error {
	if len(data) < 20 { // Minimum size for headers
		return ErrInvalidData
	}
	
	bf.mutex.Lock()
	defer bf.mutex.Unlock()
	
	offset := 0
	
	// Read size
	bf.size = binary.BigEndian.Uint64(data[offset:])
	offset += 8
	
	// Read hash count
	bf.hashCount = binary.BigEndian.Uint32(data[offset:])
	offset += 4
	
	// Read element count
	bf.elementCount = binary.BigEndian.Uint64(data[offset:])
	offset += 8
	
	// Read bit array
	bitArraySize := len(data) - offset
	bf.bitArray = make([]byte, bitArraySize)
	copy(bf.bitArray, data[offset:])
	
	return nil
}

// generateHashes generates multiple hash values for an element
func (bf *BloomFilter) generateHashes(element string) []uint64 {
	// Use double hashing: h1 + i*h2
	hash1 := bf.hash1(element)
	hash2 := bf.hash2(element)
	
	hashes := make([]uint64, bf.hashCount)
	for i := uint32(0); i < bf.hashCount; i++ {
		hashes[i] = hash1 + uint64(i)*hash2
	}
	
	return hashes
}

// hash1 generates the first hash using SHA-256
func (bf *BloomFilter) hash1(element string) uint64 {
	hasher := sha256.New()
	hasher.Write([]byte(element))
	hash := hasher.Sum(nil)
	
	return binary.BigEndian.Uint64(hash[:8])
}

// hash2 generates the second hash using SHA-256 with salt
func (bf *BloomFilter) hash2(element string) uint64 {
	hasher := sha256.New()
	hasher.Write([]byte("salt" + element))
	hash := hasher.Sum(nil)
	
	return binary.BigEndian.Uint64(hash[:8])
}

// optimalSize calculates the optimal bit array size
func optimalSize(expectedElements uint64, falsePositiveRate float64) uint64 {
	// m = -(n * ln(p)) / (ln(2)^2)
	// where n = expectedElements, p = falsePositiveRate
	size := -float64(expectedElements) * math.Log(falsePositiveRate) / (math.Ln2 * math.Ln2)
	return uint64(math.Ceil(size))
}

// optimalHashCount calculates the optimal number of hash functions
func optimalHashCount(size, expectedElements uint64) uint32 {
	// k = (m/n) * ln(2)
	// where m = size, n = expectedElements
	if expectedElements == 0 {
		return 1
	}
	
	hashCount := float64(size) / float64(expectedElements) * math.Ln2
	return uint32(math.Ceil(hashCount))
}

// Common errors
var (
	ErrIncompatibleFilters = fmt.Errorf("bloom filters are incompatible for merging")
	ErrInvalidData        = fmt.Errorf("invalid bloom filter data")
)

// BlockAvailabilityTracker manages bloom filters for block availability across peers
type BlockAvailabilityTracker struct {
	peerFilters map[string]*BloomFilter // peer ID -> bloom filter
	mutex       sync.RWMutex
}

// NewBlockAvailabilityTracker creates a new block availability tracker
func NewBlockAvailabilityTracker() *BlockAvailabilityTracker {
	return &BlockAvailabilityTracker{
		peerFilters: make(map[string]*BloomFilter),
	}
}

// UpdatePeerInventory updates the block inventory for a peer
func (bat *BlockAvailabilityTracker) UpdatePeerInventory(peerID string, blocks []string) {
	bat.mutex.Lock()
	defer bat.mutex.Unlock()
	
	// Create new bloom filter for this peer
	filter := NewBloomFilter(uint64(len(blocks)*2), 0.01) // 1% false positive rate
	
	for _, block := range blocks {
		filter.Add(block)
	}
	
	bat.peerFilters[peerID] = filter
}

// GetPeersWithBlock returns peer IDs that might have the specified block
func (bat *BlockAvailabilityTracker) GetPeersWithBlock(blockCID string) []string {
	bat.mutex.RLock()
	defer bat.mutex.RUnlock()
	
	var peers []string
	for peerID, filter := range bat.peerFilters {
		if filter.Contains(blockCID) {
			peers = append(peers, peerID)
		}
	}
	
	return peers
}

// EstimateBlockAvailability estimates how many peers might have the block
func (bat *BlockAvailabilityTracker) EstimateBlockAvailability(blockCID string) int {
	return len(bat.GetPeersWithBlock(blockCID))
}

// GetPeerInventorySize returns the estimated number of blocks a peer has
func (bat *BlockAvailabilityTracker) GetPeerInventorySize(peerID string) uint64 {
	bat.mutex.RLock()
	defer bat.mutex.RUnlock()
	
	if filter, exists := bat.peerFilters[peerID]; exists {
		return filter.ElementCount()
	}
	
	return 0
}

// RemovePeer removes a peer's inventory information
func (bat *BlockAvailabilityTracker) RemovePeer(peerID string) {
	bat.mutex.Lock()
	defer bat.mutex.Unlock()
	
	delete(bat.peerFilters, peerID)
}

// GetStats returns statistics about block availability
func (bat *BlockAvailabilityTracker) GetStats() map[string]interface{} {
	bat.mutex.RLock()
	defer bat.mutex.RUnlock()
	
	totalBlocks := uint64(0)
	totalPeers := len(bat.peerFilters)
	
	for _, filter := range bat.peerFilters {
		totalBlocks += filter.ElementCount()
	}
	
	avgBlocksPerPeer := float64(0)
	if totalPeers > 0 {
		avgBlocksPerPeer = float64(totalBlocks) / float64(totalPeers)
	}
	
	return map[string]interface{}{
		"total_peers":          totalPeers,
		"estimated_total_blocks": totalBlocks,
		"avg_blocks_per_peer":  avgBlocksPerPeer,
	}
}