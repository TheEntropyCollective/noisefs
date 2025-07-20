package fuse

import (
	"container/list"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/crypto"
	"github.com/TheEntropyCollective/noisefs/pkg/core/descriptors"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
)

// DirectoryCacheEntry represents a cached directory manifest
type DirectoryCacheEntry struct {
	Path         string
	Manifest     *descriptors.DirectoryManifest
	CID          string
	LastAccessed time.Time
	Size         int
}

// DirectoryCache implements an LRU cache for directory manifests
type DirectoryCache struct {
	maxSize        int
	currentSize    int
	ttl            time.Duration
	entries        map[string]*list.Element
	lru            *list.List
	mu             sync.RWMutex
	storageManager *storage.Manager

	// Key management for encryption
	keyResolver func(encryptionKeyID string) (*crypto.EncryptionKey, error)

	// Metrics
	hits   int64
	misses int64
}

// DirectoryCacheConfig holds configuration for the directory cache
type DirectoryCacheConfig struct {
	MaxSize       int           // Maximum number of cached manifests
	TTL           time.Duration // Time to live for cache entries
	EnableMetrics bool          // Enable cache metrics
}

// DefaultDirectoryCacheConfig returns default cache configuration
// NOTE: This function will be deprecated in favor of the centralized FUSE configuration.
// Use GetGlobalConfig().GetDirectoryCacheConfig() for new code.
func DefaultDirectoryCacheConfig() *DirectoryCacheConfig {
	return &DirectoryCacheConfig{
		MaxSize:       100,
		TTL:           30 * time.Minute,
		EnableMetrics: true,
	}
}

// NewDirectoryCacheFromGlobalConfig creates a new directory cache using the global FUSE configuration.
// This is the recommended way to create directory caches as it uses centralized configuration
// with environment variable overrides and validation.
func NewDirectoryCacheFromGlobalConfig(storageManager *storage.Manager) (*DirectoryCache, error) {
	config := GetGlobalConfig().GetDirectoryCacheConfig()
	return NewDirectoryCache(config, storageManager)
}

// NewDirectoryCache creates a new directory cache
func NewDirectoryCache(config *DirectoryCacheConfig, storageManager *storage.Manager) (*DirectoryCache, error) {
	if config == nil {
		config = DefaultDirectoryCacheConfig()
	}

	if storageManager == nil {
		return nil, fmt.Errorf("storage manager cannot be nil")
	}

	return &DirectoryCache{
		maxSize:        config.MaxSize,
		ttl:            config.TTL,
		entries:        make(map[string]*list.Element),
		lru:            list.New(),
		storageManager: storageManager,
	}, nil
}

// SetKeyResolver sets the key resolution function for retrieving encryption keys
func (dc *DirectoryCache) SetKeyResolver(resolver func(encryptionKeyID string) (*crypto.EncryptionKey, error)) {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	dc.keyResolver = resolver
}

// Get retrieves a manifest from the cache
func (dc *DirectoryCache) Get(path string) *descriptors.DirectoryManifest {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	element, exists := dc.entries[path]
	if !exists {
		dc.misses++
		return nil
	}

	entry := element.Value.(*DirectoryCacheEntry)

	// Check if entry has expired
	if time.Since(entry.LastAccessed) > dc.ttl {
		dc.removeElement(element)
		dc.misses++
		return nil
	}

	// Move to front (most recently used)
	dc.lru.MoveToFront(element)
	entry.LastAccessed = time.Now()

	dc.hits++
	return entry.Manifest
}

// Put adds a manifest to the cache
func (dc *DirectoryCache) Put(path string, manifest *descriptors.DirectoryManifest, cid string) {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	// Check if already exists
	if element, exists := dc.entries[path]; exists {
		dc.lru.MoveToFront(element)
		entry := element.Value.(*DirectoryCacheEntry)
		entry.Manifest = manifest
		entry.CID = cid
		entry.LastAccessed = time.Now()
		return
	}

	// Create new entry
	entry := &DirectoryCacheEntry{
		Path:         path,
		Manifest:     manifest,
		CID:          cid,
		LastAccessed: time.Now(),
		Size:         dc.estimateManifestSize(manifest),
	}

	// Add to cache
	element := dc.lru.PushFront(entry)
	dc.entries[path] = element
	dc.currentSize += entry.Size

	// Evict if necessary
	for dc.lru.Len() > dc.maxSize {
		dc.removeOldest()
	}
}

// LoadManifest loads a manifest from storage with caching
func (dc *DirectoryCache) LoadManifest(ctx context.Context, path string, manifestCID string, encryptionKey *crypto.EncryptionKey) (*descriptors.DirectoryManifest, error) {
	// Check cache first
	if manifest := dc.Get(path); manifest != nil {
		return manifest, nil
	}

	// Load from storage
	address := &storage.BlockAddress{
		ID: manifestCID,
	}

	block, err := dc.storageManager.Get(ctx, address)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve manifest block: %w", err)
	}

	// Decrypt manifest
	manifest, err := descriptors.DecryptManifest(block.Data, encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt manifest: %w", err)
	}

	// Cache the manifest
	dc.Put(path, manifest, manifestCID)

	return manifest, nil
}

// Prefetch loads manifests into cache proactively
func (dc *DirectoryCache) Prefetch(ctx context.Context, paths []string, getCID func(string) (string, *crypto.EncryptionKey)) error {
	for _, path := range paths {
		cid, key := getCID(path)
		if cid == "" || key == nil {
			continue
		}

		// Skip if already cached
		if dc.Get(path) != nil {
			continue
		}

		// Load manifest
		_, err := dc.LoadManifest(ctx, path, cid, key)
		if err != nil {
			// Log error but continue with other paths
			continue
		}
	}

	return nil
}

// Clear removes all entries from the cache
func (dc *DirectoryCache) Clear() {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	dc.entries = make(map[string]*list.Element)
	dc.lru = list.New()
	dc.currentSize = 0
}

// GetMetrics returns cache metrics
func (dc *DirectoryCache) GetMetrics() (hits, misses int64, hitRate float64) {
	dc.mu.RLock()
	defer dc.mu.RUnlock()

	hits = dc.hits
	misses = dc.misses

	total := hits + misses
	if total > 0 {
		hitRate = float64(hits) / float64(total)
	}

	return
}

// GetSize returns the current number of cached entries
func (dc *DirectoryCache) GetSize() int {
	dc.mu.RLock()
	defer dc.mu.RUnlock()

	return dc.lru.Len()
}

// removeElement removes an element from the cache
func (dc *DirectoryCache) removeElement(element *list.Element) {
	entry := element.Value.(*DirectoryCacheEntry)
	delete(dc.entries, entry.Path)
	dc.lru.Remove(element)
	dc.currentSize -= entry.Size
}

// removeOldest removes the least recently used entry
func (dc *DirectoryCache) removeOldest() {
	element := dc.lru.Back()
	if element != nil {
		dc.removeElement(element)
	}
}

// estimateManifestSize estimates the memory size of a manifest
func (dc *DirectoryCache) estimateManifestSize(manifest *descriptors.DirectoryManifest) int {
	// Basic estimation: 100 bytes per entry plus overhead
	return len(manifest.Entries)*100 + 1024
}

// WarmCache warms the cache with frequently accessed directories
func (dc *DirectoryCache) WarmCache(ctx context.Context, index *FileIndex) error {
	// Get all directories from index
	directories := make([]string, 0)
	for path, entry := range index.ListFiles() {
		if entry.Type == DirectoryEntryType && entry.DirectoryDescriptorCID != "" {
			directories = append(directories, path)
		}
	}

	// Sort by access frequency or other heuristics
	// For now, just load the first few
	maxWarm := 10
	if len(directories) < maxWarm {
		maxWarm = len(directories)
	}

	getCID := func(path string) (string, *crypto.EncryptionKey) {
		entry, exists := index.GetDirectory(path)
		if !exists {
			return "", nil
		}

		// Retrieve encryption key using the key resolver
		var key *crypto.EncryptionKey
		if entry.EncryptionKeyID != "" && dc.keyResolver != nil {
			resolvedKey, err := dc.keyResolver(entry.EncryptionKeyID)
			if err != nil {
				// Log error but continue - cache warming should not fail hard
				// In a production system, you might want to log this properly
				return "", nil
			}
			key = resolvedKey
		}

		return entry.DirectoryDescriptorCID, key
	}

	return dc.Prefetch(ctx, directories[:maxWarm], getCID)
}
