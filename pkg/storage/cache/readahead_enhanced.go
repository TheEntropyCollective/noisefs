package cache

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/core/descriptors"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
)

// BlockRetriever defines the interface for fetching blocks
type BlockRetriever interface {
	RetrieveBlock(cid string) (*blocks.Block, error)
}

// StorageManagerAdapter adapts a storage.Manager to the BlockRetriever interface
type StorageManagerAdapter struct {
	storageManager *storage.Manager
}

// NewStorageManagerAdapter creates a new storage manager adapter
func NewStorageManagerAdapter(storageManager *storage.Manager) BlockRetriever {
	return &StorageManagerAdapter{
		storageManager: storageManager,
	}
}

// RetrieveBlock implements the BlockRetriever interface
func (a *StorageManagerAdapter) RetrieveBlock(cid string) (*blocks.Block, error) {
	address := &storage.BlockAddress{ID: cid}
	return a.storageManager.Get(context.Background(), address)
}

// SequentialAccessTracker tracks sequential access patterns for files
type SequentialAccessTracker struct {
	mu              sync.RWMutex
	filePatterns    map[string]*FileAccessPattern // Keyed by descriptor CID
	blockToFile     map[string]string             // Maps block CID to descriptor CID
	descriptorCache map[string]*descriptors.Descriptor
	maxPatterns     int
}

// FileAccessPattern tracks access pattern for a specific file
type FileAccessPattern struct {
	DescriptorCID  string
	Descriptor     *descriptors.Descriptor
	LastBlockIndex int
	LastAccessTime time.Time
	IsSequential   bool
	Direction      int // 1 for forward, -1 for backward
	AccessCount    int
	HitCount       int // Number of times prefetch was useful
}

// NewSequentialAccessTracker creates a new sequential access tracker
func NewSequentialAccessTracker(maxPatterns int) *SequentialAccessTracker {
	return &SequentialAccessTracker{
		filePatterns:    make(map[string]*FileAccessPattern),
		blockToFile:     make(map[string]string),
		descriptorCache: make(map[string]*descriptors.Descriptor),
		maxPatterns:     maxPatterns,
	}
}

// TrackAccess tracks a block access and returns prefetch suggestions
func (sat *SequentialAccessTracker) TrackAccess(blockCID string) ([]string, bool) {
	sat.mu.Lock()
	defer sat.mu.Unlock()

	// Look up which file this block belongs to
	descriptorCID, exists := sat.blockToFile[blockCID]
	if !exists {
		// Unknown block, no prefetch suggestions
		return nil, false
	}

	pattern, exists := sat.filePatterns[descriptorCID]
	if !exists {
		return nil, false
	}

	// Find the block index in the descriptor
	blockIndex := -1
	for i, block := range pattern.Descriptor.Blocks {
		if block.DataCID == blockCID {
			blockIndex = i
			break
		}
	}

	if blockIndex == -1 {
		return nil, false
	}

	// Update access pattern
	now := time.Now()
	timeSinceLastAccess := now.Sub(pattern.LastAccessTime)

	// Detect sequential access
	if pattern.AccessCount > 0 {
		expectedIndex := pattern.LastBlockIndex + pattern.Direction
		if blockIndex == expectedIndex && timeSinceLastAccess < 5*time.Second {
			pattern.IsSequential = true
			pattern.HitCount++
		} else if blockIndex == pattern.LastBlockIndex-1 && timeSinceLastAccess < 5*time.Second {
			// Backward sequential access detected
			pattern.IsSequential = true
			pattern.Direction = -1
		} else if blockIndex != pattern.LastBlockIndex {
			// Non-sequential access
			pattern.IsSequential = false
		}
	}

	pattern.LastBlockIndex = blockIndex
	pattern.LastAccessTime = now
	pattern.AccessCount++

	// Generate prefetch suggestions if sequential
	if pattern.IsSequential {
		return sat.generatePrefetchList(pattern, blockIndex), true
	}

	return nil, false
}

// RegisterDescriptor registers a descriptor for tracking
func (sat *SequentialAccessTracker) RegisterDescriptor(descriptorCID string, desc *descriptors.Descriptor) {
	sat.mu.Lock()
	defer sat.mu.Unlock()

	// Clean up if we're at capacity
	if len(sat.filePatterns) >= sat.maxPatterns {
		sat.cleanupOldestPattern()
	}

	// Create new pattern
	pattern := &FileAccessPattern{
		DescriptorCID:  descriptorCID,
		Descriptor:     desc,
		LastBlockIndex: -1,
		Direction:      1,
		IsSequential:   false,
	}

	sat.filePatterns[descriptorCID] = pattern
	sat.descriptorCache[descriptorCID] = desc

	// Map all blocks to this descriptor
	for _, block := range desc.Blocks {
		sat.blockToFile[block.DataCID] = descriptorCID
	}
}

// generatePrefetchList generates a list of blocks to prefetch
func (sat *SequentialAccessTracker) generatePrefetchList(pattern *FileAccessPattern, currentIndex int) []string {
	prefetchList := make([]string, 0, 4) // Default prefetch 4 blocks

	totalBlocks := len(pattern.Descriptor.Blocks)

	for i := 1; i <= 4; i++ {
		nextIndex := currentIndex + (i * pattern.Direction)

		// Check bounds
		if nextIndex < 0 || nextIndex >= totalBlocks {
			break
		}

		nextBlock := pattern.Descriptor.Blocks[nextIndex]
		prefetchList = append(prefetchList, nextBlock.DataCID)
	}

	return prefetchList
}

// cleanupOldestPattern removes the oldest access pattern
func (sat *SequentialAccessTracker) cleanupOldestPattern() {
	var oldestCID string
	var oldestTime time.Time

	for cid, pattern := range sat.filePatterns {
		if oldestCID == "" || pattern.LastAccessTime.Before(oldestTime) {
			oldestCID = cid
			oldestTime = pattern.LastAccessTime
		}
	}

	if oldestCID != "" {
		pattern := sat.filePatterns[oldestCID]
		// Remove block mappings
		for _, block := range pattern.Descriptor.Blocks {
			delete(sat.blockToFile, block.DataCID)
		}
		delete(sat.filePatterns, oldestCID)
		delete(sat.descriptorCache, oldestCID)
	}
}

// EnhancedReadAheadWorker handles prefetching with real block fetching
type EnhancedReadAheadWorker struct {
	cache       Cache
	fetcher     BlockRetriever
	tracker     *SequentialAccessTracker
	prefetchMap sync.Map // Track what's being prefetched to avoid duplicates
}

// NewEnhancedReadAheadWorker creates a new enhanced read-ahead worker
func NewEnhancedReadAheadWorker(cache Cache, fetcher BlockRetriever) *EnhancedReadAheadWorker {
	return &EnhancedReadAheadWorker{
		cache:   cache,
		fetcher: fetcher,
		tracker: NewSequentialAccessTracker(100),
	}
}

// ProcessReadAheadRequest processes a read-ahead request with actual fetching
func (w *EnhancedReadAheadWorker) ProcessReadAheadRequest(blockCID string) {
	// Get prefetch suggestions from tracker
	prefetchList, isSequential := w.tracker.TrackAccess(blockCID)
	if !isSequential || len(prefetchList) == 0 {
		return
	}

	// Prefetch blocks asynchronously
	for _, cid := range prefetchList {
		// Check if already being prefetched
		if _, loaded := w.prefetchMap.LoadOrStore(cid, true); loaded {
			continue
		}

		go func(blockCID string) {
			defer w.prefetchMap.Delete(blockCID)

			// Check if already in cache
			if w.cache.Has(blockCID) {
				return
			}

			// Fetch from IPFS
			block, err := w.fetcher.RetrieveBlock(blockCID)
			if err != nil {
				// Log error but don't fail the whole operation
				return
			}

			// Store in cache
			_ = w.cache.Store(blockCID, block)
		}(cid)
	}
}

// ExtractDescriptorCID attempts to extract descriptor CID from block metadata
// In a real implementation, this might be stored in block metadata or derived from naming convention
func ExtractDescriptorCID(blockCID string) (string, bool) {
	// This is a placeholder - in reality, we'd need a way to map blocks to descriptors
	// Options include:
	// 1. Store descriptor CID in block metadata
	// 2. Use a naming convention
	// 3. Maintain a separate index

	// For now, check if the CID contains a descriptor reference
	if strings.Contains(blockCID, "_desc_") {
		parts := strings.Split(blockCID, "_desc_")
		if len(parts) >= 2 {
			return parts[1], true
		}
	}

	return "", false
}

// IntegrateReadAheadWithStorage creates a read-ahead enabled storage client
func IntegrateReadAheadWithStorage(storageManager *storage.Manager, cache Cache) *ReadAheadStorageClient {
	return &ReadAheadStorageClient{
		storageManager: storageManager,
		cache:          cache,
		worker:         NewEnhancedReadAheadWorker(cache, NewStorageManagerAdapter(storageManager)),
	}
}

// IntegrateReadAheadWithIPFS creates a read-ahead enabled storage client
// This function is deprecated, use IntegrateReadAheadWithStorage instead
func IntegrateReadAheadWithIPFS(storageManager *storage.Manager, cache Cache) *ReadAheadStorageClient {
	return IntegrateReadAheadWithStorage(storageManager, cache)
}

// ReadAheadStorageClient wraps a storage manager with read-ahead functionality
type ReadAheadStorageClient struct {
	storageManager *storage.Manager
	cache          Cache
	worker         *EnhancedReadAheadWorker
}

// ReadAheadIPFSClient is deprecated, use ReadAheadStorageClient instead
type ReadAheadIPFSClient = ReadAheadStorageClient

// RetrieveBlock retrieves a block with read-ahead
func (c *ReadAheadStorageClient) RetrieveBlock(cid string) (*blocks.Block, error) {
	// Try cache first
	if block, err := c.cache.Get(cid); err == nil {
		// Trigger read-ahead for next blocks
		go c.worker.ProcessReadAheadRequest(cid)
		return block, nil
	}

	// Retrieve from storage manager
	address := &storage.BlockAddress{ID: cid}
	block, err := c.storageManager.Get(context.Background(), address)
	if err != nil {
		return nil, err
	}

	// Store in cache
	c.cache.Store(cid, block)

	// Trigger read-ahead for next blocks
	go c.worker.ProcessReadAheadRequest(cid)

	return block, nil
}

// StoreBlock stores a block
func (c *ReadAheadStorageClient) StoreBlock(block *blocks.Block) (string, error) {
	address, err := c.storageManager.Put(context.Background(), block)
	if err != nil {
		return "", err
	}

	// Also store in cache
	_ = c.cache.Store(address.ID, block)

	return address.ID, nil
}

// RegisterDescriptor registers a descriptor for sequential access tracking
func (c *ReadAheadStorageClient) RegisterDescriptor(descriptorCID string, desc *descriptors.Descriptor) {
	c.worker.tracker.RegisterDescriptor(descriptorCID, desc)
}
