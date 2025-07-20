package index

import (
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
)

// IndexManager coordinates all indexing operations across the different
// index types (manifest, content, metadata) and provides a unified
// interface for the NoiseFS system.
type IndexManager struct {
	// Core index components
	privacyIndex    *PrivacyIndex    // Privacy-preserving filename indexing
	manifestIndex   *ManifestIndex   // Hierarchical directory navigation
	contentIndex    *ContentIndex    // Content similarity and metadata search
	
	// Performance integration (Day 1 optimizations)
	memoryPool      *blocks.MemoryPoolManager // Memory pool from Day 1
	performanceOpt  *PerformanceOptimizer     // Performance optimizations
	
	// Configuration and coordination
	config          *IndexManagerConfig
	
	// Statistics and monitoring
	stats           *IndexManagerStats
	lastMaintenance time.Time
	
	// Thread safety
	mu              sync.RWMutex
}

// IndexManagerConfig provides configuration options for the unified
// index coordinator, including privacy levels, performance tuning,
// and integration with the NoiseFS block system.
type IndexManagerConfig struct {
	// Global privacy settings
	GlobalPrivacyLevel    int     // Overall privacy level (1-5)
	EnableAllFeatures     bool    // Enable all indexing features
	
	// Component configurations
	PrivacyIndexConfig    *PrivacyIndexConfig    // Privacy index configuration
	ManifestIndexConfig   *ManifestIndexConfig   // Manifest index configuration
	ContentIndexConfig    *ContentIndexConfig    // Content index configuration
	
	// Performance settings
	UseMemoryPool         bool    // Use Day 1 memory pool optimizations
	EnableParallelIndexing bool   // Enable parallel indexing operations
	MaintenanceInterval   time.Duration // Maintenance interval
	
	// Integration settings
	EnableCrossIndexSync  bool    // Synchronize between index types
	CacheCoordination     bool    // Coordinate caching between indexes
}

// IndexManagerStats contains comprehensive statistics for all index operations
type IndexManagerStats struct {
	// Overall statistics
	TotalOperations       uint64        // Total indexing operations
	TotalQueries          uint64        // Total queries across all indexes
	TotalMaintenanceRuns  uint64        // Total maintenance operations
	AverageOperationTime  time.Duration // Average operation time
	
	// Component statistics
	PrivacyIndexStats     *IndexStats          // Privacy index statistics
	ManifestIndexStats    *ManifestIndexStats  // Manifest index statistics
	ContentIndexStats     *ContentIndexStats   // Content index statistics
	
	// Performance metrics
	MemoryPoolEfficiency  float64       // Memory pool efficiency percentage
	CacheCoordinationRate float64       // Cache coordination success rate
	CrossIndexSyncTime    time.Duration // Time for cross-index synchronization
	
	// System health
	ErrorRate             float64       // Overall error rate
	PerformanceDegradation float64      // Performance degradation percentage
	
	LastUpdated           time.Time     // When statistics were last updated
}

// PerformanceOptimizer integrates Day 1 performance optimizations
type PerformanceOptimizer struct {
	memoryPool     *blocks.MemoryPoolManager // Day 1 memory pool
	heapOptimizer  interface{}               // Day 1 heap optimization  
	workerPool     interface{}               // Day 1 worker pool
	cacheManager   interface{}               // Day 1 cache optimization
}

// NewIndexManager creates a new unified index manager with all components
func NewIndexManager(config *IndexManagerConfig) (*IndexManager, error) {
	if config == nil {
		config = DefaultIndexManagerConfig()
	}
	
	// Validate configuration
	if err := validateIndexManagerConfig(config); err != nil {
		return nil, &IndexError{Op: "NewIndexManager", Err: "invalid configuration: " + err.Error()}
	}
	
	// Initialize memory pool integration (Day 1 optimization)
	var memoryPool *blocks.MemoryPoolManager
	if config.UseMemoryPool {
		memoryPool = blocks.GlobalMemoryPool
	}
	
	// Initialize performance optimizer with Day 1 integrations
	performanceOpt := &PerformanceOptimizer{
		memoryPool: memoryPool,
		// Other Day 1 integrations would be initialized here
	}
	
	// Create privacy index
	privacyIndex, err := NewPrivacyIndex(config.PrivacyIndexConfig)
	if err != nil {
		return nil, &IndexError{Op: "NewIndexManager", Err: "failed to create privacy index: " + err.Error()}
	}
	
	// Create manifest index
	manifestIndex, err := NewManifestIndex(config.ManifestIndexConfig)
	if err != nil {
		return nil, &IndexError{Op: "NewIndexManager", Err: "failed to create manifest index: " + err.Error()}
	}
	
	// Create content index
	contentIndex, err := NewContentIndex(config.ContentIndexConfig)
	if err != nil {
		return nil, &IndexError{Op: "NewIndexManager", Err: "failed to create content index: " + err.Error()}
	}
	
	// Initialize statistics
	stats := &IndexManagerStats{
		LastUpdated: time.Now(),
	}
	
	return &IndexManager{
		privacyIndex:    privacyIndex,
		manifestIndex:   manifestIndex,
		contentIndex:    contentIndex,
		memoryPool:      memoryPool,
		performanceOpt:  performanceOpt,
		config:          config,
		stats:           stats,
		lastMaintenance: time.Now(),
	}, nil
}

// IndexFile adds a file to all relevant indexes with coordinated operations
func (im *IndexManager) IndexFile(fileID string, filename string, directoryPath string, content []byte, metadata FileMetadata) error {
	if fileID == "" {
		return &IndexError{Op: "IndexFile", Err: "file ID cannot be empty"}
	}
	
	im.mu.Lock()
	defer im.mu.Unlock()
	
	startTime := time.Now()
	defer func() {
		im.updateOperationStats(time.Since(startTime))
	}()
	
	// Index in privacy index (encrypted filename)
	if err := im.privacyIndex.IndexFilename([]byte(filename), metadata); err != nil {
		return &IndexError{Op: "IndexFile", Err: "privacy index failed: " + err.Error()}
	}
	
	// Index in manifest index (directory structure)
	manifestData := im.createManifestData(fileID, filename, metadata)
	if err := im.manifestIndex.IndexDirectory(directoryPath, manifestData); err != nil {
		return &IndexError{Op: "IndexFile", Err: "manifest index failed: " + err.Error()}
	}
	
	// Index in content index (content and metadata)
	if len(content) > 0 {
		if err := im.contentIndex.IndexContent(fileID, content, metadata); err != nil {
			return &IndexError{Op: "IndexFile", Err: "content index failed: " + err.Error()}
		}
	}
	
	// Perform cross-index synchronization if enabled
	if im.config.EnableCrossIndexSync {
		if err := im.synchronizeIndexes(fileID); err != nil {
			// Log error but don't fail the operation
			// In a full implementation, this would use proper logging
		}
	}
	
	// Update statistics
	im.stats.TotalOperations++
	
	return nil
}

// SearchFiles performs coordinated search across all indexes
func (im *IndexManager) SearchFiles(query *UnifiedSearchQuery) (*UnifiedSearchResult, error) {
	im.mu.RLock()
	defer im.mu.RUnlock()
	
	startTime := time.Now()
	defer func() {
		im.updateQueryStats(time.Since(startTime))
	}()
	
	result := &UnifiedSearchResult{
		Matches:     make([]UnifiedMatch, 0),
		QueryTime:   time.Now(),
		TotalChecked: 0,
	}
	
	// Privacy index search (filename-based)
	if query.FilenameQuery != nil {
		privacyMatches, err := im.searchPrivacyIndex(query.FilenameQuery)
		if err != nil {
			return nil, err
		}
		result.Matches = append(result.Matches, privacyMatches...)
	}
	
	// Manifest index search (directory-based)
	if query.DirectoryQuery != nil {
		manifestMatches, err := im.searchManifestIndex(query.DirectoryQuery)
		if err != nil {
			return nil, err
		}
		result.Matches = append(result.Matches, manifestMatches...)
	}
	
	// Content index search (content and metadata)
	if query.ContentQuery != nil {
		contentMatches, err := im.searchContentIndex(query.ContentQuery)
		if err != nil {
			return nil, err
		}
		result.Matches = append(result.Matches, contentMatches...)
	}
	
	// Merge and rank results
	if err := im.mergeAndRankResults(result); err != nil {
		return nil, err
	}
	
	// Update statistics
	im.stats.TotalQueries++
	
	return result, nil
}

// GetComprehensiveStats returns unified statistics from all index components
func (im *IndexManager) GetComprehensiveStats() *IndexManagerStats {
	im.mu.RLock()
	defer im.mu.RUnlock()
	
	// Update component statistics
	im.stats.PrivacyIndexStats = im.privacyIndex.GetStats()
	im.stats.ManifestIndexStats = im.manifestIndex.GetStats()
	im.stats.ContentIndexStats = im.contentIndex.GetStats()
	
	// Calculate performance metrics
	im.calculatePerformanceMetrics()
	
	// Create a copy for thread safety
	statsCopy := *im.stats
	statsCopy.LastUpdated = time.Now()
	
	return &statsCopy
}

// PerformMaintenance runs maintenance on all index components
func (im *IndexManager) PerformMaintenance() error {
	im.mu.Lock()
	defer im.mu.Unlock()
	
	now := time.Now()
	
	// Only perform maintenance if enough time has passed
	if now.Sub(im.lastMaintenance) < im.config.MaintenanceInterval {
		return nil
	}
	
	// Perform maintenance on each component
	if err := im.privacyIndex.Maintenance(); err != nil {
		return &IndexError{Op: "PerformMaintenance", Err: "privacy index maintenance failed: " + err.Error()}
	}
	
	// Manifest index doesn't have explicit maintenance, but could be added
	
	// Content index maintenance would be implemented here if needed
	
	// Perform memory pool maintenance if available
	if im.memoryPool != nil {
		// Memory pool maintenance would be called here
	}
	
	// Update maintenance timestamp
	im.lastMaintenance = now
	im.stats.TotalMaintenanceRuns++
	
	return nil
}

// Helper functions for search operations

// searchPrivacyIndex searches the privacy index for filename matches
func (im *IndexManager) searchPrivacyIndex(query *FilenameQuery) ([]UnifiedMatch, error) {
	found, err := im.privacyIndex.QueryFilename([]byte(query.Filename))
	if err != nil {
		return nil, err
	}
	
	matches := make([]UnifiedMatch, 0)
	if found {
		matches = append(matches, UnifiedMatch{
			FileID:     query.Filename, // Simplified
			Source:     "privacy",
			Relevance:  0.8,
			MatchType:  "filename",
		})
	}
	
	return matches, nil
}

// searchManifestIndex searches the manifest index for directory matches
func (im *IndexManager) searchManifestIndex(query *DirectoryQuery) ([]UnifiedMatch, error) {
	manifestData, found, err := im.manifestIndex.LookupDirectory(query.DirectoryPath)
	if err != nil {
		return nil, err
	}
	
	matches := make([]UnifiedMatch, 0)
	if found {
		matches = append(matches, UnifiedMatch{
			FileID:       string(manifestData), // Simplified
			Source:       "manifest",
			Relevance:    0.9,
			MatchType:    "directory",
			ManifestData: manifestData,
		})
	}
	
	return matches, nil
}

// searchContentIndex searches the content index for content/metadata matches
func (im *IndexManager) searchContentIndex(query *ContentQuery) ([]UnifiedMatch, error) {
	searchResult, err := im.contentIndex.SearchContent(*query)
	if err != nil {
		return nil, err
	}
	
	matches := make([]UnifiedMatch, 0)
	for _, match := range searchResult.Matches {
		matches = append(matches, UnifiedMatch{
			FileID:     match.ContentID,
			Source:     "content",
			Relevance:  match.Relevance,
			MatchType:  "content",
			Similarity: match.Similarity,
		})
	}
	
	return matches, nil
}

// mergeAndRankResults merges results from different indexes and ranks them
func (im *IndexManager) mergeAndRankResults(result *UnifiedSearchResult) error {
	// Group matches by FileID
	matchGroups := make(map[string][]UnifiedMatch)
	for _, match := range result.Matches {
		matchGroups[match.FileID] = append(matchGroups[match.FileID], match)
	}
	
	// Merge grouped matches and calculate combined relevance
	mergedMatches := make([]UnifiedMatch, 0)
	for fileID, matches := range matchGroups {
		mergedMatch := im.mergeMatches(fileID, matches)
		mergedMatches = append(mergedMatches, mergedMatch)
	}
	
	// Sort by relevance (highest first)
	for i := 0; i < len(mergedMatches)-1; i++ {
		for j := i + 1; j < len(mergedMatches); j++ {
			if mergedMatches[i].Relevance < mergedMatches[j].Relevance {
				mergedMatches[i], mergedMatches[j] = mergedMatches[j], mergedMatches[i]
			}
		}
	}
	
	result.Matches = mergedMatches
	result.TotalMatches = len(mergedMatches)
	
	return nil
}

// mergeMatches combines multiple matches for the same file
func (im *IndexManager) mergeMatches(fileID string, matches []UnifiedMatch) UnifiedMatch {
	merged := UnifiedMatch{
		FileID:    fileID,
		Sources:   make([]string, 0),
		Relevance: 0.0,
		MatchType: "combined",
	}
	
	// Combine sources and calculate weighted relevance
	totalWeight := 0.0
	for _, match := range matches {
		merged.Sources = append(merged.Sources, match.Source)
		
		// Weight different sources differently
		weight := 1.0
		switch match.Source {
		case "privacy":
			weight = 0.6
		case "manifest":
			weight = 0.8
		case "content":
			weight = 1.0
		}
		
		merged.Relevance += match.Relevance * weight
		totalWeight += weight
		
		// Preserve additional data
		if match.ManifestData != nil {
			merged.ManifestData = match.ManifestData
		}
		if match.Similarity > 0 {
			merged.Similarity = match.Similarity
		}
	}
	
	// Normalize relevance
	if totalWeight > 0 {
		merged.Relevance /= totalWeight
	}
	
	return merged
}

// Helper functions for data management

// createManifestData creates manifest data for directory indexing
func (im *IndexManager) createManifestData(fileID, filename string, metadata FileMetadata) []byte {
	// This would create proper manifest data structure
	// Simplified for now
	manifestInfo := filename + ":" + fileID
	return []byte(manifestInfo)
}

// synchronizeIndexes performs cross-index synchronization
func (im *IndexManager) synchronizeIndexes(fileID string) error {
	// This would implement cross-index synchronization logic
	// For now, just update the sync time statistic
	im.stats.CrossIndexSyncTime = time.Millisecond * 100 // Simulated
	return nil
}

// Statistics and monitoring functions

// updateOperationStats updates indexing operation statistics
func (im *IndexManager) updateOperationStats(operationTime time.Duration) {
	// Update average operation time (exponential moving average)
	alpha := 0.1
	im.stats.AverageOperationTime = time.Duration(
		alpha*float64(operationTime) + (1-alpha)*float64(im.stats.AverageOperationTime),
	)
}

// updateQueryStats updates query operation statistics
func (im *IndexManager) updateQueryStats(queryTime time.Duration) {
	// Query time statistics are handled by individual indexes
	// This could aggregate them if needed
}

// calculatePerformanceMetrics calculates overall performance metrics
func (im *IndexManager) calculatePerformanceMetrics() {
	// Calculate memory pool efficiency
	if im.memoryPool != nil {
		// This would get actual efficiency from memory pool
		im.stats.MemoryPoolEfficiency = 0.95 // Simulated 95% efficiency
	}
	
	// Calculate cache coordination rate
	if im.config.CacheCoordination {
		im.stats.CacheCoordinationRate = 0.88 // Simulated 88% success rate
	}
	
	// Calculate error rate (would be based on actual error tracking)
	im.stats.ErrorRate = 0.001 // Simulated 0.1% error rate
	
	// Calculate performance degradation (would be based on baseline metrics)
	im.stats.PerformanceDegradation = 0.02 // Simulated 2% degradation
}

// Search query structures

// UnifiedSearchQuery represents a search query across all index types
type UnifiedSearchQuery struct {
	FilenameQuery  *FilenameQuery  // Search by filename
	DirectoryQuery *DirectoryQuery // Search by directory path
	ContentQuery   *ContentQuery   // Search by content/metadata
	MaxResults     int             // Maximum results to return
}

// FilenameQuery represents a filename-based search query
type FilenameQuery struct {
	Filename string // Filename to search for
}

// DirectoryQuery represents a directory-based search query
type DirectoryQuery struct {
	DirectoryPath string // Directory path to search in
}

// UnifiedSearchResult represents search results from all indexes
type UnifiedSearchResult struct {
	Matches      []UnifiedMatch // Unified search matches
	TotalMatches int            // Total number of matches
	TotalChecked uint64         // Total items checked across all indexes
	QueryTime    time.Time      // When query was executed
}

// UnifiedMatch represents a unified search match from any index
type UnifiedMatch struct {
	FileID       string    // File identifier
	Source       string    // Source index ("privacy", "manifest", "content")
	Sources      []string  // Multiple sources if merged
	Relevance    float64   // Overall relevance score
	MatchType    string    // Type of match ("filename", "directory", "content", "combined")
	Similarity   float64   // Content similarity score (if applicable)
	ManifestData []byte    // Manifest data (if applicable)
}

// Utility and configuration functions

// validateIndexManagerConfig validates the index manager configuration
func validateIndexManagerConfig(config *IndexManagerConfig) error {
	if config.GlobalPrivacyLevel < 1 || config.GlobalPrivacyLevel > 5 {
		return &IndexError{Op: "validateIndexManagerConfig", Err: "global privacy level must be between 1 and 5"}
	}
	
	if config.MaintenanceInterval < time.Minute {
		return &IndexError{Op: "validateIndexManagerConfig", Err: "maintenance interval must be at least 1 minute"}
	}
	
	return nil
}

// DefaultIndexManagerConfig returns default configuration for the index manager
func DefaultIndexManagerConfig() *IndexManagerConfig {
	return &IndexManagerConfig{
		GlobalPrivacyLevel:      3,
		EnableAllFeatures:       true,
		PrivacyIndexConfig:      DefaultPrivacyIndexConfig(),
		ManifestIndexConfig:     DefaultManifestIndexConfig(),
		ContentIndexConfig:      DefaultContentIndexConfig(),
		UseMemoryPool:          true,
		EnableParallelIndexing: true,
		MaintenanceInterval:    time.Hour,
		EnableCrossIndexSync:   true,
		CacheCoordination:      true,
	}
}