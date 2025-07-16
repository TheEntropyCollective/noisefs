package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/core/crypto"
)

// DirectoryManager manages directory operations with LRU caching
type DirectoryManager struct {
	storageManager *Manager
	cache          *DirectoryCache
	encryptionKey  *crypto.EncryptionKey
	config         *DirectoryManagerConfig
	
	// Metrics
	cacheHits   int64
	cacheMisses int64
	statsMutex  sync.RWMutex
}

// DirectoryManagerConfig holds configuration for the directory manager
type DirectoryManagerConfig struct {
	CacheSize         int           // Maximum number of cached directory manifests
	CacheTTL          time.Duration // Time to live for cached entries
	MaxManifestSize   int64         // Maximum size of a directory manifest
	ReconstructionTTL time.Duration // TTL for reconstruction operations
	EnableMetrics     bool          // Enable metrics collection
	CompressionLevel  int           // Compression level for manifests (0-9)
}

// DefaultDirectoryManagerConfig returns default configuration
func DefaultDirectoryManagerConfig() *DirectoryManagerConfig {
	return &DirectoryManagerConfig{
		CacheSize:         100,
		CacheTTL:          30 * time.Minute,
		MaxManifestSize:   10 * 1024 * 1024, // 10MB
		ReconstructionTTL: 5 * time.Minute,
		EnableMetrics:     true,
		CompressionLevel:  6,
	}
}

// NewDirectoryManager creates a new directory manager
func NewDirectoryManager(storageManager *Manager, encryptionKey *crypto.EncryptionKey, config *DirectoryManagerConfig) (*DirectoryManager, error) {
	if storageManager == nil {
		return nil, fmt.Errorf("storage manager cannot be nil")
	}
	
	if encryptionKey == nil {
		return nil, fmt.Errorf("encryption key cannot be nil")
	}
	
	if config == nil {
		config = DefaultDirectoryManagerConfig()
	}
	
	cache, err := NewDirectoryCache(config.CacheSize, config.CacheTTL)
	if err != nil {
		return nil, fmt.Errorf("failed to create directory cache: %w", err)
	}
	
	return &DirectoryManager{
		storageManager: storageManager,
		cache:          cache,
		encryptionKey:  encryptionKey,
		config:         config,
	}, nil
}

// StoreDirectoryManifest stores a directory manifest in the storage backend
func (dm *DirectoryManager) StoreDirectoryManifest(ctx context.Context, dirPath string, manifest *blocks.DirectoryManifest) (string, error) {
	// Encrypt the manifest
	encryptedManifest, err := blocks.EncryptManifest(manifest, dm.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt manifest: %w", err)
	}
	
	// Check manifest size
	if int64(len(encryptedManifest)) > dm.config.MaxManifestSize {
		return "", fmt.Errorf("manifest too large: %d bytes (max: %d)", len(encryptedManifest), dm.config.MaxManifestSize)
	}
	
	// Create block from encrypted manifest
	manifestBlock, err := blocks.NewBlock(encryptedManifest)
	if err != nil {
		return "", fmt.Errorf("failed to create manifest block: %w", err)
	}
	
	// Store the block using the storage manager
	address, err := dm.storageManager.Put(ctx, manifestBlock)
	if err != nil {
		return "", fmt.Errorf("failed to store manifest block: %w", err)
	}
	
	// Cache the manifest
	dm.cache.Put(dirPath, manifest)
	
	return address.ID, nil
}

// RetrieveDirectoryManifest retrieves a directory manifest from storage
func (dm *DirectoryManager) RetrieveDirectoryManifest(ctx context.Context, dirPath string, manifestCID string) (*blocks.DirectoryManifest, error) {
	// Check cache first
	if cachedManifest := dm.cache.Get(dirPath); cachedManifest != nil {
		dm.incrementCacheHits()
		return cachedManifest, nil
	}
	
	dm.incrementCacheMisses()
	
	// Retrieve from storage
	address := &BlockAddress{
		ID:          manifestCID,
		BackendType: dm.storageManager.config.DefaultBackend,
	}
	
	manifestBlock, err := dm.storageManager.Get(ctx, address)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve manifest block: %w", err)
	}
	
	// Decrypt the manifest
	manifest, err := dm.decryptManifest(manifestBlock.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt manifest: %w", err)
	}
	
	// Cache the manifest
	dm.cache.Put(dirPath, manifest)
	
	return manifest, nil
}

// ReconstructDirectory reconstructs a directory from its manifest
func (dm *DirectoryManager) ReconstructDirectory(ctx context.Context, manifestCID string, targetPath string) (*DirectoryReconstructionResult, error) {
	// Retrieve the manifest
	manifest, err := dm.RetrieveDirectoryManifest(ctx, targetPath, manifestCID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve manifest: %w", err)
	}
	
	// Start reconstruction
	result := &DirectoryReconstructionResult{
		TargetPath:       targetPath,
		ManifestCID:      manifestCID,
		TotalEntries:     len(manifest.Entries),
		ProcessedEntries: 0,
		StartTime:        time.Now(),
		Status:           "in_progress",
		Errors:           make([]ReconstructionError, 0),
	}
	
	// Process each entry
	for i, entry := range manifest.Entries {
		// Check for cancellation
		select {
		case <-ctx.Done():
			result.Status = "cancelled"
			result.EndTime = time.Now()
			return result, ctx.Err()
		default:
		}
		
		// Decrypt filename
		dirKey, err := crypto.DeriveDirectoryKey(dm.encryptionKey, targetPath)
		if err != nil {
			result.Errors = append(result.Errors, ReconstructionError{
				EntryIndex: i,
				Error:      fmt.Errorf("failed to derive directory key: %w", err),
			})
			continue
		}
		
		filename, err := crypto.DecryptFileName(entry.EncryptedName, dirKey)
		if err != nil {
			result.Errors = append(result.Errors, ReconstructionError{
				EntryIndex: i,
				Error:      fmt.Errorf("failed to decrypt filename: %w", err),
			})
			continue
		}
		
		// Create entry result
		entryResult := ReconstructionEntryResult{
			Index:         i,
			DecryptedName: filename,
			CID:           entry.CID,
			Type:          entry.Type,
			Size:          entry.Size,
			ModifiedAt:    entry.ModifiedAt,
		}
		
		result.Entries = append(result.Entries, entryResult)
		result.ProcessedEntries++
	}
	
	result.Status = "completed"
	result.EndTime = time.Now()
	
	return result, nil
}

// decryptManifest decrypts a manifest from encrypted data
func (dm *DirectoryManager) decryptManifest(encryptedData []byte) (*blocks.DirectoryManifest, error) {
	// Decrypt the data
	decryptedData, err := crypto.Decrypt(encryptedData, dm.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt data: %w", err)
	}
	
	// Unmarshal the manifest
	var manifest blocks.DirectoryManifest
	if err := json.Unmarshal(decryptedData, &manifest); err != nil {
		return nil, fmt.Errorf("failed to unmarshal manifest: %w", err)
	}
	
	return &manifest, nil
}

// GetCacheStats returns cache statistics
func (dm *DirectoryManager) GetCacheStats() *DirectoryCacheStats {
	dm.statsMutex.RLock()
	defer dm.statsMutex.RUnlock()
	
	return &DirectoryCacheStats{
		CacheHits:   dm.cacheHits,
		CacheMisses: dm.cacheMisses,
		CacheSize:   dm.cache.Size(),
		MaxSize:     dm.cache.MaxSize(),
		HitRate:     dm.calculateHitRate(),
	}
}

// calculateHitRate calculates cache hit rate
func (dm *DirectoryManager) calculateHitRate() float64 {
	total := dm.cacheHits + dm.cacheMisses
	if total == 0 {
		return 0
	}
	return float64(dm.cacheHits) / float64(total)
}

// incrementCacheHits increments cache hit counter
func (dm *DirectoryManager) incrementCacheHits() {
	if dm.config.EnableMetrics {
		dm.statsMutex.Lock()
		dm.cacheHits++
		dm.statsMutex.Unlock()
	}
}

// incrementCacheMisses increments cache miss counter
func (dm *DirectoryManager) incrementCacheMisses() {
	if dm.config.EnableMetrics {
		dm.statsMutex.Lock()
		dm.cacheMisses++
		dm.statsMutex.Unlock()
	}
}

// ClearCache clears the directory cache
func (dm *DirectoryManager) ClearCache() {
	dm.cache.Clear()
}

// DirectoryCache implements LRU cache for directory manifests
type DirectoryCache struct {
	cache    map[string]*DirectoryCacheEntry
	head     *DirectoryCacheEntry
	tail     *DirectoryCacheEntry
	maxSize  int
	ttl      time.Duration
	mutex    sync.RWMutex
}

// DirectoryCacheEntry represents a cache entry
type DirectoryCacheEntry struct {
	key       string
	manifest  *blocks.DirectoryManifest
	timestamp time.Time
	prev      *DirectoryCacheEntry
	next      *DirectoryCacheEntry
}

// NewDirectoryCache creates a new LRU cache
func NewDirectoryCache(maxSize int, ttl time.Duration) (*DirectoryCache, error) {
	if maxSize <= 0 {
		return nil, fmt.Errorf("cache size must be positive")
	}
	
	cache := &DirectoryCache{
		cache:   make(map[string]*DirectoryCacheEntry),
		maxSize: maxSize,
		ttl:     ttl,
	}
	
	// Initialize doubly linked list
	cache.head = &DirectoryCacheEntry{}
	cache.tail = &DirectoryCacheEntry{}
	cache.head.next = cache.tail
	cache.tail.prev = cache.head
	
	return cache, nil
}

// Get retrieves a manifest from cache
func (dc *DirectoryCache) Get(key string) *blocks.DirectoryManifest {
	dc.mutex.Lock()
	defer dc.mutex.Unlock()
	
	entry, exists := dc.cache[key]
	if !exists {
		return nil
	}
	
	// Check TTL
	if time.Since(entry.timestamp) > dc.ttl {
		dc.removeEntry(entry)
		return nil
	}
	
	// Move to head (most recently used)
	dc.moveToHead(entry)
	
	return entry.manifest
}

// Put stores a manifest in cache
func (dc *DirectoryCache) Put(key string, manifest *blocks.DirectoryManifest) {
	dc.mutex.Lock()
	defer dc.mutex.Unlock()
	
	// Check if entry already exists
	if entry, exists := dc.cache[key]; exists {
		// Update existing entry
		entry.manifest = manifest
		entry.timestamp = time.Now()
		dc.moveToHead(entry)
		return
	}
	
	// Create new entry
	entry := &DirectoryCacheEntry{
		key:       key,
		manifest:  manifest,
		timestamp: time.Now(),
	}
	
	dc.cache[key] = entry
	dc.addToHead(entry)
	
	// Check if we need to evict
	if len(dc.cache) > dc.maxSize {
		dc.removeTail()
	}
}

// Size returns current cache size
func (dc *DirectoryCache) Size() int {
	dc.mutex.RLock()
	defer dc.mutex.RUnlock()
	return len(dc.cache)
}

// MaxSize returns maximum cache size
func (dc *DirectoryCache) MaxSize() int {
	return dc.maxSize
}

// Clear clears the cache
func (dc *DirectoryCache) Clear() {
	dc.mutex.Lock()
	defer dc.mutex.Unlock()
	
	dc.cache = make(map[string]*DirectoryCacheEntry)
	dc.head.next = dc.tail
	dc.tail.prev = dc.head
}

// addToHead adds entry to head of list
func (dc *DirectoryCache) addToHead(entry *DirectoryCacheEntry) {
	entry.prev = dc.head
	entry.next = dc.head.next
	
	dc.head.next.prev = entry
	dc.head.next = entry
}

// removeEntry removes entry from list
func (dc *DirectoryCache) removeEntry(entry *DirectoryCacheEntry) {
	entry.prev.next = entry.next
	entry.next.prev = entry.prev
	
	delete(dc.cache, entry.key)
}

// moveToHead moves entry to head of list
func (dc *DirectoryCache) moveToHead(entry *DirectoryCacheEntry) {
	// Remove from current position in list
	entry.prev.next = entry.next
	entry.next.prev = entry.prev
	
	// Add to head
	entry.prev = dc.head
	entry.next = dc.head.next
	dc.head.next.prev = entry
	dc.head.next = entry
}

// removeTail removes entry from tail of list
func (dc *DirectoryCache) removeTail() {
	if dc.tail.prev == dc.head {
		return // Cache is empty
	}
	
	dc.removeEntry(dc.tail.prev)
}

// DirectoryReconstructionResult represents the result of directory reconstruction
type DirectoryReconstructionResult struct {
	TargetPath       string                        `json:"target_path"`
	ManifestCID      string                        `json:"manifest_cid"`
	TotalEntries     int                           `json:"total_entries"`
	ProcessedEntries int                           `json:"processed_entries"`
	StartTime        time.Time                     `json:"start_time"`
	EndTime          time.Time                     `json:"end_time"`
	Status           string                        `json:"status"` // "in_progress", "completed", "cancelled", "failed"
	Entries          []ReconstructionEntryResult   `json:"entries"`
	Errors           []ReconstructionError         `json:"errors"`
}

// ReconstructionEntryResult represents a reconstructed directory entry
type ReconstructionEntryResult struct {
	Index         int                     `json:"index"`
	DecryptedName string                  `json:"decrypted_name"`
	CID           string                  `json:"cid"`
	Type          blocks.DescriptorType   `json:"type"`
	Size          int64                   `json:"size"`
	ModifiedAt    time.Time               `json:"modified_at"`
}

// ReconstructionError represents an error during reconstruction
type ReconstructionError struct {
	EntryIndex int   `json:"entry_index"`
	Error      error `json:"error"`
}

// DirectoryCacheStats represents cache statistics
type DirectoryCacheStats struct {
	CacheHits   int64   `json:"cache_hits"`
	CacheMisses int64   `json:"cache_misses"`
	CacheSize   int     `json:"cache_size"`
	MaxSize     int     `json:"max_size"`
	HitRate     float64 `json:"hit_rate"`
}

// DirectoryManagerStats represents directory manager statistics
type DirectoryManagerStats struct {
	CacheStats            *DirectoryCacheStats `json:"cache_stats"`
	ManifestsStored       int64                `json:"manifests_stored"`
	ManifestsRetrieved    int64                `json:"manifests_retrieved"`
	ReconstructionsTotal  int64                `json:"reconstructions_total"`
	ReconstructionsActive int64                `json:"reconstructions_active"`
	AverageManifestSize   int64                `json:"average_manifest_size"`
}

// GetStats returns comprehensive directory manager statistics
func (dm *DirectoryManager) GetStats() *DirectoryManagerStats {
	return &DirectoryManagerStats{
		CacheStats: dm.GetCacheStats(),
		// Additional stats would be implemented here
	}
}

// Health check for directory manager
func (dm *DirectoryManager) HealthCheck() *DirectoryManagerHealth {
	cacheStats := dm.GetCacheStats()
	
	healthy := true
	var issues []string
	
	// Check cache health
	if cacheStats.HitRate < 0.5 && cacheStats.CacheHits+cacheStats.CacheMisses > 100 {
		healthy = false
		issues = append(issues, "low cache hit rate")
	}
	
	// Check storage manager health
	if !dm.storageManager.IsConnected() {
		healthy = false
		issues = append(issues, "storage manager not connected")
	}
	
	return &DirectoryManagerHealth{
		Healthy:    healthy,
		Issues:     issues,
		CacheStats: cacheStats,
		LastCheck:  time.Now(),
	}
}

// DirectoryManagerHealth represents health status
type DirectoryManagerHealth struct {
	Healthy    bool                 `json:"healthy"`
	Issues     []string             `json:"issues"`
	CacheStats *DirectoryCacheStats `json:"cache_stats"`
	LastCheck  time.Time            `json:"last_check"`
}