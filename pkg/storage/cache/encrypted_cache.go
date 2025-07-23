package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/core/crypto"
)

// EncryptedPersistentCache provides an encrypted persistent cache implementation
type EncryptedPersistentCache struct {
	mu            sync.RWMutex
	cache         map[string]*CacheEntry
	maxSize       int
	maxAge        time.Duration
	persistPath   string
	encryptionKey *crypto.EncryptionKey
	encrypted     bool

	// LRU tracking
	accessOrder map[string]time.Time

	// Security settings
	secureMemory  bool
	antiForensics bool
}

// CacheEntry represents a cached block with metadata
type CacheEntry struct {
	Block      *blocks.Block `json:"block"`
	AccessTime time.Time     `json:"access_time"`
	HitCount   int           `json:"hit_count"`
	Size       int           `json:"size"`
}

// PersistentCacheData represents the serialized cache structure
type PersistentCacheData struct {
	Version string                 `json:"version"`
	Entries map[string]*CacheEntry `json:"entries"`
}

// NewEncryptedPersistentCache creates a new encrypted persistent cache
func NewEncryptedPersistentCache(maxSize int, persistPath, password string, secureMemory, antiForensics bool) (*EncryptedPersistentCache, error) {
	cache := &EncryptedPersistentCache{
		cache:         make(map[string]*CacheEntry),
		maxSize:       maxSize,
		maxAge:        24 * time.Hour, // 24 hour default
		persistPath:   persistPath,
		accessOrder:   make(map[string]time.Time),
		secureMemory:  secureMemory,
		antiForensics: antiForensics,
	}

	// Setup encryption if password provided
	if password != "" {
		encKey, err := crypto.GenerateKey(password)
		if err != nil {
			return nil, fmt.Errorf("failed to generate encryption key: %w", err)
		}
		cache.encryptionKey = encKey
		cache.encrypted = true
	}

	// Try to load existing cache
	if err := cache.loadFromDisk(); err != nil {
		// Log error but continue with empty cache
		fmt.Printf("Warning: Failed to load cache from disk: %v\n", err)
	}

	return cache, nil
}

// Get retrieves a block from the cache
func (c *EncryptedPersistentCache) Get(cid string) (*blocks.Block, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.cache[cid]
	if !exists {
		return nil, false
	}

	// Check if entry has expired
	if time.Since(entry.AccessTime) > c.maxAge {
		delete(c.cache, cid)
		delete(c.accessOrder, cid)
		return nil, false
	}

	// Update access statistics
	entry.AccessTime = time.Now()
	entry.HitCount++
	c.accessOrder[cid] = entry.AccessTime

	return entry.Block, true
}

// Put stores a block in the cache
func (c *EncryptedPersistentCache) Put(cid string, block *blocks.Block) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if we need to evict entries
	if len(c.cache) >= c.maxSize {
		c.evictLRU()
	}

	entry := &CacheEntry{
		Block:      block,
		AccessTime: time.Now(),
		HitCount:   1,
		Size:       len(block.Data),
	}

	c.cache[cid] = entry
	c.accessOrder[cid] = entry.AccessTime

	// Periodically persist to disk (every 10 entries)
	if len(c.cache)%10 == 0 {
		go c.saveToDisk()
	}
}

// evictLRU removes the least recently used entry
func (c *EncryptedPersistentCache) evictLRU() {
	if len(c.cache) == 0 {
		return
	}

	var oldestCID string
	var oldestTime time.Time = time.Now()

	for cid, accessTime := range c.accessOrder {
		if accessTime.Before(oldestTime) {
			oldestTime = accessTime
			oldestCID = cid
		}
	}

	if oldestCID != "" {
		// Securely clear block data if anti-forensics enabled
		if c.antiForensics {
			if entry, exists := c.cache[oldestCID]; exists && entry.Block != nil {
				crypto.SecureZero(entry.Block.Data)
			}
		}

		delete(c.cache, oldestCID)
		delete(c.accessOrder, oldestCID)
	}
}

// Size returns the current number of cached entries
func (c *EncryptedPersistentCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.cache)
}

// Clear removes all entries from the cache
func (c *EncryptedPersistentCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Securely clear block data if anti-forensics enabled
	if c.antiForensics {
		for _, entry := range c.cache {
			if entry.Block != nil {
				crypto.SecureZero(entry.Block.Data)
			}
		}
	}

	c.cache = make(map[string]*CacheEntry)
	c.accessOrder = make(map[string]time.Time)
}

// loadFromDisk loads the cache from persistent storage
func (c *EncryptedPersistentCache) loadFromDisk() error {
	if c.persistPath == "" {
		return nil // No persistence configured
	}

	if _, err := os.Stat(c.persistPath); os.IsNotExist(err) {
		return nil // No existing cache file
	}

	data, err := os.ReadFile(c.persistPath)
	if err != nil {
		return fmt.Errorf("failed to read cache file: %w", err)
	}

	var cacheData []byte

	// Try to decrypt if encryption is enabled
	if c.encrypted && c.encryptionKey != nil {
		// Try encrypted format first
		if decrypted, err := c.tryDecryptCache(data); err == nil {
			cacheData = decrypted
		} else {
			// Fallback to unencrypted
			cacheData = data
		}
	} else {
		cacheData = data
	}

	var persistentData PersistentCacheData
	if err := json.Unmarshal(cacheData, &persistentData); err != nil {
		return fmt.Errorf("failed to parse cache data: %w", err)
	}

	// Load entries, filtering out expired ones
	now := time.Now()
	for cid, entry := range persistentData.Entries {
		if now.Sub(entry.AccessTime) <= c.maxAge {
			c.cache[cid] = entry
			c.accessOrder[cid] = entry.AccessTime
		}
	}

	return nil
}

// tryDecryptCache attempts to decrypt cache data
func (c *EncryptedPersistentCache) tryDecryptCache(encryptedData []byte) ([]byte, error) {
	if !c.encrypted || c.encryptionKey == nil {
		return nil, fmt.Errorf("encryption not enabled")
	}

	// Parse encrypted format
	var encCache struct {
		Version   string `json:"version"`
		Encrypted bool   `json:"encrypted"`
		Salt      []byte `json:"salt"`
		Data      []byte `json:"data"`
	}

	if err := json.Unmarshal(encryptedData, &encCache); err != nil {
		return nil, fmt.Errorf("invalid encrypted cache format: %w", err)
	}

	if !encCache.Encrypted {
		return nil, fmt.Errorf("not an encrypted cache")
	}

	// Derive key and decrypt
	key, err := crypto.DeriveKey(string(c.encryptionKey.Key), encCache.Salt)
	if err != nil {
		return nil, fmt.Errorf("failed to derive key: %w", err)
	}

	decryptedData, err := crypto.Decrypt(encCache.Data, key)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt cache: %w", err)
	}

	// Clear key
	crypto.SecureZero(key.Key)

	return decryptedData, nil
}

// saveToDisk saves the cache to persistent storage
func (c *EncryptedPersistentCache) saveToDisk() error {
	if c.persistPath == "" {
		return nil // No persistence configured
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	// Create cache data structure
	persistentData := PersistentCacheData{
		Version: "1.0",
		Entries: c.cache,
	}

	// Serialize cache data
	cacheData, err := json.MarshalIndent(persistentData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cache data: %w", err)
	}

	var finalData []byte

	// Encrypt if enabled
	if c.encrypted && c.encryptionKey != nil {
		encryptedData, err := crypto.Encrypt(cacheData, c.encryptionKey)
		if err != nil {
			return fmt.Errorf("failed to encrypt cache: %w", err)
		}

		encCache := struct {
			Version   string `json:"version"`
			Encrypted bool   `json:"encrypted"`
			Salt      []byte `json:"salt"`
			Data      []byte `json:"data"`
		}{
			Version:   "1.0-encrypted",
			Encrypted: true,
			Salt:      c.encryptionKey.Salt,
			Data:      encryptedData,
		}

		finalData, err = json.MarshalIndent(encCache, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal encrypted cache: %w", err)
		}
	} else {
		finalData = cacheData
	}

	// Ensure directory exists
	dir := filepath.Dir(c.persistPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Write atomically
	tmpPath := c.persistPath + ".tmp"
	if err := os.WriteFile(tmpPath, finalData, 0600); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	if err := os.Rename(tmpPath, c.persistPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename cache file: %w", err)
	}

	return nil
}

// Flush forces a save to disk
func (c *EncryptedPersistentCache) Flush() error {
	return c.saveToDisk()
}

// Cleanup securely clears sensitive data
func (c *EncryptedPersistentCache) Cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Securely clear cache entries if anti-forensics enabled
	if c.antiForensics {
		for _, entry := range c.cache {
			if entry.Block != nil {
				crypto.SecureZero(entry.Block.Data)
			}
		}
	}

	// Clear encryption key
	if c.encryptionKey != nil {
		crypto.SecureZero(c.encryptionKey.Key)
		crypto.SecureZero(c.encryptionKey.Salt)
	}

	// Clear cache
	c.cache = make(map[string]*CacheEntry)
	c.accessOrder = make(map[string]time.Time)

	// Trigger garbage collection if secure memory is enabled
	if c.secureMemory {
		runtime.GC()
	}
}

// GetStats returns cache statistics
func (c *EncryptedPersistentCache) GetStats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	totalSize := 0
	totalHits := 0
	for _, entry := range c.cache {
		totalSize += entry.Size
		totalHits += entry.HitCount
	}

	return map[string]interface{}{
		"entries":     len(c.cache),
		"max_size":    c.maxSize,
		"total_bytes": totalSize,
		"total_hits":  totalHits,
		"encrypted":   c.encrypted,
	}
}
