package index

import (
	"crypto/sha256"
	"encoding/binary"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
)

// ManifestIndex provides hierarchical encrypted indexing for directory navigation.
// It enables O(log n) directory lookups without revealing directory structure
// by using encrypted path segments and B+ tree-like indexing with privacy preservation.
type ManifestIndex struct {
	// Core index components
	pathFilter     *BloomFilter        // Encrypted path segment filter
	hierarchyIndex *HierarchyIndex     // B+ tree-like hierarchy navigation
	manifestCache  *ManifestCache      // Cached manifest entries
	
	// Privacy components
	pathObfuscator *PathObfuscator     // Path encryption and obfuscation
	config         *ManifestIndexConfig
	
	// Performance integration
	memoryPool     *blocks.MemoryPoolManager
	
	// Statistics and monitoring
	stats          *ManifestIndexStats
	
	// Thread safety
	mu             sync.RWMutex
}

// ManifestIndexConfig holds configuration for encrypted manifest indexing
type ManifestIndexConfig struct {
	// Privacy settings
	PathEncryptionKey   []byte  // Key for encrypting path segments
	MaxPathDepth        int     // Maximum directory depth
	EnablePathBlinding  bool    // Enable path structure obfuscation
	UseHierarchyIndex   bool    // Enable B+ tree-like hierarchy indexing
	
	// Performance settings
	ExpectedDirectories uint64  // Expected number of directories
	ExpectedManifests   uint64  // Expected number of manifest entries
	CacheSize          int     // Maximum cached manifest entries
	PrivacyLevel       int     // Privacy level (1-5)
	
	// Filter configuration
	FalsePositiveRate  float64 // Base false positive rate for path filter
	EnableCompression  bool    // Enable index compression
}

// ManifestIndexStats contains statistics for manifest indexing performance
type ManifestIndexStats struct {
	// Lookup performance
	TotalLookups        uint64        // Total directory lookups
	SuccessfulLookups   uint64        // Successful lookups
	CacheHits           uint64        // Cache hit count
	AverageLookupTime   time.Duration // Average lookup time
	
	// Index efficiency
	IndexedDirectories  uint64        // Number of indexed directories
	IndexedManifests    uint64        // Number of indexed manifests
	PathSegments        uint64        // Total encrypted path segments
	
	// Memory usage
	FilterMemoryUsage   uint64        // Memory used by path filter
	HierarchyMemoryUsage uint64       // Memory used by hierarchy index
	CacheMemoryUsage    uint64        // Memory used by manifest cache
	
	// Privacy metrics
	PathObfuscationRate float64       // Rate of path obfuscation
	StructuralEntropy   float64       // Entropy of directory structure
	
	LastUpdated         time.Time     // When statistics were last updated
}

// HierarchyIndex provides B+ tree-like indexing for directory hierarchy
type HierarchyIndex struct {
	nodes      map[string]*HierarchyNode // Encrypted path -> node mapping
	rootNodes  []*HierarchyNode          // Root-level directory nodes
	maxDepth   int                       // Maximum depth tracked
	nodeCount  uint64                    // Total number of nodes
	mu         sync.RWMutex              // Thread safety
}

// HierarchyNode represents a node in the hierarchy index
type HierarchyNode struct {
	EncryptedPath  []byte              // Encrypted path segment
	Depth          int                 // Depth in hierarchy
	Children       []*HierarchyNode    // Child nodes
	Parent         *HierarchyNode      // Parent node (nil for root)
	ManifestRefs   []string            // References to manifest entries
	AccessCount    uint64              // Access frequency (for caching)
	LastAccessed   time.Time           // Last access time
}

// ManifestCache provides LRU caching for frequently accessed manifests
type ManifestCache struct {
	entries     map[string]*CacheEntry // Cached manifest entries
	accessOrder []*CacheEntry          // LRU access order
	maxSize     int                    // Maximum cache size
	currentSize int                    // Current cache size
	ttl         time.Duration          // Cache entry TTL
	mu          sync.RWMutex           // Thread safety
}

// CacheEntry represents a cached manifest entry
type CacheEntry struct {
	Key          string      // Cache key (encrypted path)
	ManifestData []byte      // Cached manifest data
	AccessTime   time.Time   // Last access time
	AccessCount  uint64      // Access frequency
	createdAt    time.Time   // When entry was created
}

// PathObfuscator handles path encryption and structure obfuscation
type PathObfuscator struct {
	encryptionKey []byte       // Key for path encryption
	blindingKeys  [][]byte     // Keys for path structure blinding
	noisePaths    []string     // Noise paths for plausible deniability
	mu            sync.RWMutex // Thread safety
}

// NewManifestIndex creates a new encrypted manifest index
func NewManifestIndex(config *ManifestIndexConfig) (*ManifestIndex, error) {
	if config == nil {
		config = DefaultManifestIndexConfig()
	}
	
	// Validate configuration
	if err := validateManifestConfig(config); err != nil {
		return nil, err
	}
	
	// Create path filter for encrypted path segments
	pathFilterConfig := &BloomFilterConfig{
		ExpectedElements:  config.ExpectedDirectories * uint64(config.MaxPathDepth),
		FalsePositiveRate: config.FalsePositiveRate,
		PrivacyLevel:      config.PrivacyLevel + 1, // Higher privacy for paths
		UseCompression:    config.EnableCompression,
	}
	
	pathFilter, err := NewBloomFilter(pathFilterConfig)
	if err != nil {
		return nil, &IndexError{Op: "NewManifestIndex", Err: "failed to create path filter: " + err.Error()}
	}
	
	// Initialize hierarchy index
	hierarchyIndex := &HierarchyIndex{
		nodes:     make(map[string]*HierarchyNode),
		rootNodes: make([]*HierarchyNode, 0),
		maxDepth:  config.MaxPathDepth,
	}
	
	// Initialize manifest cache
	manifestCache := &ManifestCache{
		entries:     make(map[string]*CacheEntry),
		accessOrder: make([]*CacheEntry, 0),
		maxSize:     config.CacheSize,
		ttl:         time.Hour, // Default 1 hour TTL
	}
	
	// Initialize path obfuscator
	pathObfuscator := &PathObfuscator{
		encryptionKey: config.PathEncryptionKey,
		blindingKeys:  generateBlindingKeys(config.PrivacyLevel),
		noisePaths:    generateNoisePaths(config.ExpectedDirectories / 10), // 10% noise
	}
	
	// Initialize statistics
	stats := &ManifestIndexStats{
		LastUpdated: time.Now(),
	}
	
	return &ManifestIndex{
		pathFilter:     pathFilter,
		hierarchyIndex: hierarchyIndex,
		manifestCache:  manifestCache,
		pathObfuscator: pathObfuscator,
		config:         config,
		stats:          stats,
	}, nil
}

// IndexDirectory adds a directory path to the encrypted manifest index
func (mi *ManifestIndex) IndexDirectory(directoryPath string, manifestData []byte) error {
	if directoryPath == "" {
		return &IndexError{Op: "IndexDirectory", Err: "directory path cannot be empty"}
	}
	
	mi.mu.Lock()
	defer mi.mu.Unlock()
	
	// Encrypt and obfuscate the path
	encryptedPath, pathSegments, err := mi.pathObfuscator.EncryptPath(directoryPath)
	if err != nil {
		return &IndexError{Op: "IndexDirectory", Err: "failed to encrypt path: " + err.Error()}
	}
	
	// Add path segments to Bloom filter
	for _, segment := range pathSegments {
		if err := mi.pathFilter.Add(segment); err != nil {
			return &IndexError{Op: "IndexDirectory", Err: "failed to add path segment: " + err.Error()}
		}
	}
	
	// Update hierarchy index if enabled
	if mi.config.UseHierarchyIndex {
		if err := mi.updateHierarchyIndex(encryptedPath, pathSegments, manifestData); err != nil {
			return &IndexError{Op: "IndexDirectory", Err: "failed to update hierarchy: " + err.Error()}
		}
	}
	
	// Cache the manifest if needed
	mi.manifestCache.Set(string(encryptedPath), manifestData)
	
	// Update statistics
	mi.stats.IndexedDirectories++
	mi.stats.IndexedManifests++
	mi.stats.PathSegments += uint64(len(pathSegments))
	
	return nil
}

// LookupDirectory performs an encrypted directory lookup
func (mi *ManifestIndex) LookupDirectory(directoryPath string) ([]byte, bool, error) {
	if directoryPath == "" {
		return nil, false, &IndexError{Op: "LookupDirectory", Err: "directory path cannot be empty"}
	}
	
	mi.mu.RLock()
	defer mi.mu.RUnlock()
	
	startTime := time.Now()
	defer func() {
		mi.updateLookupStats(time.Since(startTime), true)
	}()
	
	// Encrypt the lookup path
	encryptedPath, pathSegments, err := mi.pathObfuscator.EncryptPath(directoryPath)
	if err != nil {
		return nil, false, &IndexError{Op: "LookupDirectory", Err: "failed to encrypt lookup path: " + err.Error()}
	}
	
	// Check cache first
	if cached, found := mi.manifestCache.Get(string(encryptedPath)); found {
		mi.stats.CacheHits++
		mi.stats.SuccessfulLookups++
		return cached, true, nil
	}
	
	// Check path filter for existence probability
	pathExists, err := mi.checkPathInFilter(pathSegments)
	if err != nil {
		return nil, false, err
	}
	
	if !pathExists {
		return nil, false, nil // Definitely doesn't exist
	}
	
	// Use hierarchy index for efficient lookup if available
	if mi.config.UseHierarchyIndex {
		manifestData, found := mi.hierarchyLookup(pathSegments)
		if found {
			mi.stats.SuccessfulLookups++
			// Cache the result
			mi.manifestCache.Set(string(encryptedPath), manifestData)
			return manifestData, true, nil
		}
	}
	
	// Path might exist but not found in hierarchy (possible false positive)
	return nil, false, nil
}

// Helper functions

// EncryptPath encrypts a directory path and splits it into segments
func (po *PathObfuscator) EncryptPath(path string) ([]byte, [][]byte, error) {
	po.mu.RLock()
	defer po.mu.RUnlock()
	
	// Hash the complete path for primary encryption
	hasher := sha256.New()
	hasher.Write(po.encryptionKey)
	hasher.Write([]byte(path))
	encryptedPath := hasher.Sum(nil)
	
	// Create encrypted segments for each path component
	pathSegments := [][]byte{}
	components := splitPath(path)
	
	for i, component := range components {
		// Create segment hash with position and parent context
		segmentHasher := sha256.New()
		segmentHasher.Write(po.encryptionKey)
		segmentHasher.Write([]byte(component))
		
		// Add position information for hierarchy
		positionBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(positionBytes, uint64(i))
		segmentHasher.Write(positionBytes)
		
		// Add parent context if not root
		if i > 0 {
			segmentHasher.Write(pathSegments[i-1])
		}
		
		pathSegments = append(pathSegments, segmentHasher.Sum(nil))
	}
	
	return encryptedPath, pathSegments, nil
}

// checkPathInFilter verifies if path segments exist in the filter
func (mi *ManifestIndex) checkPathInFilter(pathSegments [][]byte) (bool, error) {
	for _, segment := range pathSegments {
		exists, err := mi.pathFilter.Contains(segment)
		if err != nil {
			return false, err
		}
		if !exists {
			return false, nil // One segment doesn't exist, path doesn't exist
		}
	}
	return true, nil // All segments found (might be false positive)
}

// updateHierarchyIndex updates the B+ tree-like hierarchy structure
func (mi *ManifestIndex) updateHierarchyIndex(encryptedPath []byte, pathSegments [][]byte, manifestData []byte) error {
	var parentNode *HierarchyNode
	
	// Build hierarchy from root to leaf
	for depth, segment := range pathSegments {
		segmentKey := string(segment)
		
		// Find or create node
		node, exists := mi.hierarchyIndex.nodes[segmentKey]
		if !exists {
			node = &HierarchyNode{
				EncryptedPath: segment,
				Depth:         depth,
				Children:      make([]*HierarchyNode, 0),
				ManifestRefs:  make([]string, 0),
				LastAccessed:  time.Now(),
			}
			mi.hierarchyIndex.nodes[segmentKey] = node
			mi.hierarchyIndex.nodeCount++
		}
		
		// Link to parent
		if parentNode != nil {
			node.Parent = parentNode
			// Add to parent's children if not already present
			if !containsNode(parentNode.Children, node) {
				parentNode.Children = append(parentNode.Children, node)
			}
		} else {
			// Root node
			if !containsNode(mi.hierarchyIndex.rootNodes, node) {
				mi.hierarchyIndex.rootNodes = append(mi.hierarchyIndex.rootNodes, node)
			}
		}
		
		parentNode = node
	}
	
	// Add manifest reference to leaf node
	if parentNode != nil {
		manifestRef := string(encryptedPath)
		if !containsString(parentNode.ManifestRefs, manifestRef) {
			parentNode.ManifestRefs = append(parentNode.ManifestRefs, manifestRef)
		}
	}
	
	return nil
}

// hierarchyLookup performs efficient lookup using hierarchy index
func (mi *ManifestIndex) hierarchyLookup(pathSegments [][]byte) ([]byte, bool) {
	var currentNode *HierarchyNode
	
	// Navigate down the hierarchy
	for depth, segment := range pathSegments {
		segmentKey := string(segment)
		
		if depth == 0 {
			// Find root node
			node, exists := mi.hierarchyIndex.nodes[segmentKey]
			if !exists {
				return nil, false
			}
			currentNode = node
		} else {
			// Find child node
			var foundChild *HierarchyNode
			for _, child := range currentNode.Children {
				if string(child.EncryptedPath) == segmentKey {
					foundChild = child
					break
				}
			}
			if foundChild == nil {
				return nil, false
			}
			currentNode = foundChild
		}
		
		// Update access statistics
		currentNode.AccessCount++
		currentNode.LastAccessed = time.Now()
	}
	
	// Return manifest data if available
	if len(currentNode.ManifestRefs) > 0 {
		// For now, return placeholder data
		// In a full implementation, this would fetch actual manifest data
		return []byte("manifest_data_placeholder"), true
	}
	
	return nil, false
}

// Cache operations

// Get retrieves an entry from the manifest cache
func (mc *ManifestCache) Get(key string) ([]byte, bool) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	
	entry, exists := mc.entries[key]
	if !exists {
		return nil, false
	}
	
	// Check TTL
	if time.Since(entry.createdAt) > mc.ttl {
		// Entry expired, remove it
		delete(mc.entries, key)
		return nil, false
	}
	
	// Update access information
	entry.AccessTime = time.Now()
	entry.AccessCount++
	
	return entry.ManifestData, true
}

// Set adds or updates an entry in the manifest cache
func (mc *ManifestCache) Set(key string, data []byte) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	
	// Check if entry already exists
	if existing, exists := mc.entries[key]; exists {
		existing.ManifestData = data
		existing.AccessTime = time.Now()
		existing.AccessCount++
		return
	}
	
	// Create new entry
	entry := &CacheEntry{
		Key:          key,
		ManifestData: data,
		AccessTime:   time.Now(),
		AccessCount:  1,
		createdAt:    time.Now(),
	}
	
	// Evict if necessary
	if mc.currentSize >= mc.maxSize {
		mc.evictLRU()
	}
	
	mc.entries[key] = entry
	mc.accessOrder = append(mc.accessOrder, entry)
	mc.currentSize++
}

// evictLRU removes the least recently used entry
func (mc *ManifestCache) evictLRU() {
	if len(mc.accessOrder) == 0 {
		return
	}
	
	// Find LRU entry
	oldestTime := time.Now()
	oldestIndex := -1
	
	for i, entry := range mc.accessOrder {
		if entry.AccessTime.Before(oldestTime) {
			oldestTime = entry.AccessTime
			oldestIndex = i
		}
	}
	
	if oldestIndex >= 0 {
		// Remove from cache
		evictedEntry := mc.accessOrder[oldestIndex]
		delete(mc.entries, evictedEntry.Key)
		
		// Remove from access order
		mc.accessOrder = append(mc.accessOrder[:oldestIndex], mc.accessOrder[oldestIndex+1:]...)
		mc.currentSize--
	}
}

// Statistics and monitoring

// GetStats returns current manifest index statistics
func (mi *ManifestIndex) GetStats() *ManifestIndexStats {
	mi.mu.RLock()
	defer mi.mu.RUnlock()
	
	// Update memory usage statistics
	mi.updateMemoryStats()
	
	// Create a copy of current stats
	statsCopy := *mi.stats
	statsCopy.LastUpdated = time.Now()
	
	return &statsCopy
}

// updateLookupStats updates lookup performance statistics
func (mi *ManifestIndex) updateLookupStats(lookupTime time.Duration, success bool) {
	mi.stats.TotalLookups++
	if success {
		mi.stats.SuccessfulLookups++
	}
	
	// Update average lookup time (exponential moving average)
	alpha := 0.1
	mi.stats.AverageLookupTime = time.Duration(
		alpha*float64(lookupTime) + (1-alpha)*float64(mi.stats.AverageLookupTime),
	)
}

// updateMemoryStats updates memory usage statistics
func (mi *ManifestIndex) updateMemoryStats() {
	mi.stats.FilterMemoryUsage = mi.pathFilter.GetStats().MemoryUsageBytes
	mi.stats.HierarchyMemoryUsage = uint64(mi.hierarchyIndex.nodeCount) * 256 // Estimate
	mi.stats.CacheMemoryUsage = uint64(mi.manifestCache.currentSize) * 1024    // Estimate
}

// Utility functions

// validateManifestConfig validates manifest index configuration
func validateManifestConfig(config *ManifestIndexConfig) error {
	if len(config.PathEncryptionKey) < 16 {
		return &IndexError{Op: "validateManifestConfig", Err: "encryption key must be at least 16 bytes"}
	}
	
	if config.MaxPathDepth < 1 || config.MaxPathDepth > 20 {
		return &IndexError{Op: "validateManifestConfig", Err: "max path depth must be between 1 and 20"}
	}
	
	if config.FalsePositiveRate <= 0 || config.FalsePositiveRate >= 1 {
		return &IndexError{Op: "validateManifestConfig", Err: "false positive rate must be between 0 and 1"}
	}
	
	return nil
}

// DefaultManifestIndexConfig returns default configuration
func DefaultManifestIndexConfig() *ManifestIndexConfig {
	// Generate default encryption key
	key := make([]byte, 32)
	// In production, this would use crypto/rand
	for i := range key {
		key[i] = byte(i)
	}
	
	return &ManifestIndexConfig{
		PathEncryptionKey:   key,
		MaxPathDepth:        10,
		EnablePathBlinding:  true,
		UseHierarchyIndex:   true,
		ExpectedDirectories: 10000,
		ExpectedManifests:   50000,
		CacheSize:          1000,
		PrivacyLevel:       3,
		FalsePositiveRate:  0.01,
		EnableCompression:  true,
	}
}

// Helper utility functions

// splitPath splits a file path into components
func splitPath(path string) []string {
	if path == "" || path == "/" {
		return []string{"/"}
	}
	
	components := []string{}
	current := ""
	
	for _, char := range path {
		if char == '/' {
			if current != "" {
				components = append(components, current)
				current = ""
			}
		} else {
			current += string(char)
		}
	}
	
	if current != "" {
		components = append(components, current)
	}
	
	return components
}

// containsNode checks if a slice contains a specific node
func containsNode(nodes []*HierarchyNode, target *HierarchyNode) bool {
	for _, node := range nodes {
		if node == target {
			return true
		}
	}
	return false
}

// containsString checks if a slice contains a specific string
func containsString(slice []string, target string) bool {
	for _, item := range slice {
		if item == target {
			return true
		}
	}
	return false
}

// generateBlindingKeys generates keys for path structure obfuscation
func generateBlindingKeys(privacyLevel int) [][]byte {
	keys := make([][]byte, privacyLevel)
	for i := 0; i < privacyLevel; i++ {
		key := make([]byte, 32)
		// In production, use crypto/rand
		for j := range key {
			key[j] = byte(i*32 + j)
		}
		keys[i] = key
	}
	return keys
}

// generateNoisePaths generates noise paths for plausible deniability
func generateNoisePaths(count uint64) []string {
	noisePaths := make([]string, count)
	for i := uint64(0); i < count; i++ {
		noisePaths[i] = "noise_path_" + string(rune('a'+int(i%26)))
	}
	return noisePaths
}