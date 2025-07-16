package search

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search/query"
)

// Implement SearchService interface methods

// Search performs a full-text search with the given query and options
func (sm *SearchManager) Search(queryStr string, options SearchOptions) (*SearchResults, error) {
	if !sm.started {
		return nil, fmt.Errorf("search manager not started")
	}
	
	// Check cache first
	cacheKey := GenerateCacheKey(queryStr, options)
	if cachedResult, found := sm.resultCache.Get(cacheKey); found {
		return cachedResult, nil
	}
	
	startTime := time.Now()
	
	// Build the search query
	var q query.Query
	if queryStr == "" {
		q = bleve.NewMatchAllQuery()
	} else {
		q = bleve.NewQueryStringQuery(queryStr)
	}
	
	// Apply filters if provided
	if len(options.Filters) > 0 || options.TimeRange != nil || options.SizeRange != nil {
		queries := []query.Query{q}
		
		// Add filter queries
		for field, value := range options.Filters {
			switch v := value.(type) {
			case string:
				termQuery := bleve.NewTermQuery(v)
				termQuery.SetField(field)
				queries = append(queries, termQuery)
			case []string:
				// Handle multiple values as OR
				subQueries := make([]query.Query, len(v))
				for i, val := range v {
					termQuery := bleve.NewTermQuery(val)
					termQuery.SetField(field)
					subQueries[i] = termQuery
				}
				queries = append(queries, bleve.NewDisjunctionQuery(subQueries...))
			}
		}
		
		// Add time range filter
		if options.TimeRange != nil {
			timeQuery := bleve.NewDateRangeQuery(options.TimeRange.Start, options.TimeRange.End)
			timeQuery.SetField("modified")
			queries = append(queries, timeQuery)
		}
		
		// Add size range filter
		if options.SizeRange != nil {
			sizeQuery := bleve.NewNumericRangeQuery(
				float64Ptr(float64(options.SizeRange.Min)),
				float64Ptr(float64(options.SizeRange.Max)),
			)
			sizeQuery.SetField("size")
			queries = append(queries, sizeQuery)
		}
		
		// Combine all queries
		q = bleve.NewConjunctionQuery(queries...)
	}
	
	// Apply directory scoping
	if options.Directory != "" {
		dirQuery := bleve.NewTermQuery(options.Directory)
		dirQuery.SetField("directory")
		
		if options.Recursive {
			// For recursive search, use prefix query
			prefixQuery := bleve.NewPrefixQuery(options.Directory)
			prefixQuery.SetField("directory")
			// Create a disjunction query for the directory
			disjunctionQuery := bleve.NewDisjunctionQuery(dirQuery, prefixQuery)
			q = bleve.NewConjunctionQuery(q, disjunctionQuery)
		} else {
			q = bleve.NewConjunctionQuery(q, dirQuery)
		}
	}
	
	// Create search request
	searchRequest := bleve.NewSearchRequest(q)
	
	// Set pagination
	if options.MaxResults == 0 {
		options.MaxResults = int(sm.config.DefaultResults)
	}
	searchRequest.Size = options.MaxResults
	searchRequest.From = options.Offset
	
	// Set sorting
	if options.SortBy != "" {
		sortField := string(options.SortBy)
		if options.SortOrder == SortDesc {
			searchRequest.SortBy([]string{"-" + sortField})
		} else {
			searchRequest.SortBy([]string{sortField})
		}
	}
	
	// Set fields to include
	searchRequest.Fields = []string{"*"}
	
	// Enable highlighting if requested
	if options.Highlight {
		searchRequest.Highlight = bleve.NewHighlight()
	}
	
	// Enable facets if requested
	for _, facetField := range options.Facets {
		searchRequest.AddFacet(facetField, bleve.NewFacetRequest(facetField, 10))
	}
	
	// Execute search
	searchResult, err := sm.bleveIndex.Search(searchRequest)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}
	
	// Convert to our result format
	results := &SearchResults{
		Query:       queryStr,
		QueryType:   "full-text",
		Options:     options,
		Results:     make([]SearchResult, 0, len(searchResult.Hits)),
		Total:       int(searchResult.Total),
		MaxScore:    searchResult.MaxScore,
		TimeTaken:   time.Since(startTime),
		TimeTakenMS: time.Since(startTime).Milliseconds(),
		Offset:      options.Offset,
		Limit:       options.MaxResults,
		HasMore:     searchResult.Total > uint64(options.Offset+len(searchResult.Hits)),
	}
	
	// Convert hits to results
	for _, hit := range searchResult.Hits {
		result := SearchResult{
			Path:  hit.ID,
			Score: hit.Score,
		}
		
		// Extract fields
		if path, ok := hit.Fields["path"].(string); ok {
			result.Path = path
		}
		if cid, ok := hit.Fields["descriptor_cid"].(string); ok {
			result.DescriptorCID = cid
		}
		if size, ok := hit.Fields["size"].(float64); ok {
			result.Size = int64(size)
		}
		if modifiedStr, ok := hit.Fields["modified"].(string); ok {
			if modified, err := time.Parse(time.RFC3339, modifiedStr); err == nil {
				result.ModifiedAt = modified
			}
		}
		if mimeType, ok := hit.Fields["mime_type"].(string); ok {
			result.MimeType = mimeType
		}
		if dir, ok := hit.Fields["directory"].(string); ok {
			result.Directory = dir
		}
		if preview, ok := hit.Fields["preview"].(string); ok {
			result.Preview = preview
		}
		
		// Add highlights
		if options.Highlight && hit.Fragments != nil {
			for _, fragments := range hit.Fragments {
				result.Highlights = append(result.Highlights, fragments...)
			}
		}
		
		// Set file type from path if not already set
		if result.FileType == "" {
			result.FileType = strings.TrimPrefix(filepath.Ext(result.Path), ".")
		}
		// TODO: IndexedAt should come from the index
		
		results.Results = append(results.Results, result)
	}
	
	// Convert facets
	if len(searchResult.Facets) > 0 {
		results.Facets = make(map[string]FacetResult)
		for name, facet := range searchResult.Facets {
			facetResult := FacetResult{
				Field:   name,
				Values:  make([]FacetValue, 0, len(facet.Terms.Terms())),
				Total:   facet.Total,
				Other:   facet.Other,
				Missing: facet.Missing,
			}
			
			for _, term := range facet.Terms.Terms() {
				facetResult.Values = append(facetResult.Values, FacetValue{
					Value:      term.Term,
					Count:      term.Count,
					Percentage: float64(term.Count) / float64(facet.Total) * 100,
				})
			}
			
			results.Facets[name] = facetResult
		}
	}
	
	// Update search metrics
	sm.updateSearchMetrics(time.Since(startTime))
	
	// Cache the results
	sm.resultCache.Put(cacheKey, results)
	
	return results, nil
}

// SearchMetadata performs a metadata-only search without content analysis
func (sm *SearchManager) SearchMetadata(filters MetadataFilters) (*SearchResults, error) {
	if !sm.started {
		return nil, fmt.Errorf("search manager not started")
	}
	
	// Check cache first
	cacheKey := GenerateMetadataCacheKey(filters)
	if cachedResult, found := sm.resultCache.Get(cacheKey); found {
		return cachedResult, nil
	}
	
	// Convert metadata filters to search options
	options := SearchOptions{
		MaxResults: 100, // Default for metadata search
		Filters:    make(map[string]interface{}),
	}
	
	// Build query from filters
	queries := []query.Query{}
	
	// Name pattern
	if filters.NamePattern != "" {
		if strings.Contains(filters.NamePattern, "*") {
			q := bleve.NewWildcardQuery(filters.NamePattern)
			q.SetField("filename")
			queries = append(queries, q)
		} else {
			q := bleve.NewMatchQuery(filters.NamePattern)
			q.SetField("filename")
			queries = append(queries, q)
		}
	}
	
	// Path pattern
	if filters.PathPattern != "" {
		if strings.Contains(filters.PathPattern, "*") {
			q := bleve.NewWildcardQuery(filters.PathPattern)
			q.SetField("path")
			queries = append(queries, q)
		} else {
			q := bleve.NewPrefixQuery(filters.PathPattern)
			q.SetField("path")
			queries = append(queries, q)
		}
	}
	
	// Size range
	if filters.SizeRange != nil {
		options.SizeRange = filters.SizeRange
	} else if filters.MinSize != nil || filters.MaxSize != nil {
		options.SizeRange = &SizeRange{}
		if filters.MinSize != nil {
			options.SizeRange.Min = *filters.MinSize
		}
		if filters.MaxSize != nil {
			options.SizeRange.Max = *filters.MaxSize
		}
	}
	
	// Time range
	if filters.TimeRange != nil {
		options.TimeRange = filters.TimeRange
	}
	
	// File types
	if len(filters.FileTypes) > 0 {
		options.FileTypes = filters.FileTypes
	}
	
	// MIME types
	if len(filters.MimeTypes) > 0 {
		options.Filters["mime_type"] = filters.MimeTypes
	}
	
	// Directory
	if filters.Directory != "" {
		options.Directory = filters.Directory
		options.Recursive = filters.Recursive
	}
	
	// Use the regular search with appropriate query
	queryStr := ""
	if filters.NamePattern != "" {
		queryStr = filters.NamePattern
	}
	
	results, err := sm.Search(queryStr, options)
	if err != nil {
		return nil, err
	}
	
	// Cache the metadata search results separately
	sm.resultCache.Put(cacheKey, results)
	
	return results, nil
}

// UpdateIndex updates the search index for a specific file
func (sm *SearchManager) UpdateIndex(path string, metadata FileMetadata) error {
	if !sm.started {
		return fmt.Errorf("search manager not started")
	}
	
	// Create index request
	req := IndexRequest{
		Operation: "update",
		Path:      path,
		CID:       metadata.DescriptorCID,
		Metadata: map[string]interface{}{
			"mime_type": metadata.MimeType,
			"file_type": metadata.FileType,
			"preview":   metadata.ContentPreview,
			"content":   metadata.Content,
			"language":  metadata.Language,
			"tags":      metadata.Tags,
		},
		Priority:  5, // Normal priority
		Timestamp: time.Now(),
	}
	
	// Invalidate cache when content changes
	sm.resultCache.Clear()
	
	// Queue the request
	select {
	case sm.indexQueue <- req:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("indexing queue is full")
	}
}

// RemoveFromIndex removes a file from the search index
func (sm *SearchManager) RemoveFromIndex(path string) error {
	if !sm.started {
		return fmt.Errorf("search manager not started")
	}
	
	// Create delete request
	req := IndexRequest{
		Operation: "delete",
		Path:      path,
		Priority:  10, // High priority for deletes
		Timestamp: time.Now(),
	}
	
	// Invalidate cache when content changes
	sm.resultCache.Clear()
	
	// Queue the request
	select {
	case sm.indexQueue <- req:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("indexing queue is full")
	}
}

// RebuildIndex rebuilds the entire search index from the file index
func (sm *SearchManager) RebuildIndex() error {
	if !sm.started {
		return fmt.Errorf("search manager not started")
	}
	
	// Get all files from the file index
	files := sm.fileIndex.ListFiles()
	
	// Queue each file for indexing
	for path, entry := range files {
		// Extract descriptor CID from entry
		descriptorCID := ""
		if entryMap, ok := entry.(map[string]interface{}); ok {
			if cid, ok := entryMap["DescriptorCID"].(string); ok {
				descriptorCID = cid
			}
		}
		
		req := IndexRequest{
			Operation: "add",
			Path:      path,
			CID:       descriptorCID,
			Priority:  1, // Low priority for rebuild
			Timestamp: time.Now(),
		}
		
		// Queue with timeout
		select {
		case sm.indexQueue <- req:
			// Successfully queued
		case <-time.After(100 * time.Millisecond):
			// Skip if queue is full, we'll get it next time
			continue
		}
	}
	
	return nil
}

// GetIndexStats returns statistics about the search index
func (sm *SearchManager) GetIndexStats() (*IndexStats, error) {
	if !sm.started {
		return nil, fmt.Errorf("search manager not started")
	}
	
	// Get document count from Bleve
	docCount, err := sm.bleveIndex.DocCount()
	if err != nil {
		return nil, fmt.Errorf("failed to get document count: %w", err)
	}
	
	// Get metrics
	sm.metrics.mutex.RLock()
	metrics := sm.metrics
	sm.metrics.mutex.RUnlock()
	
	// Calculate index size (approximate)
	indexInfo, err := os.Stat(sm.indexPath)
	var indexSize int64
	if err == nil && indexInfo.IsDir() {
		indexSize = sm.calculateDirSize(sm.indexPath)
	}
	
	// Get cache stats
	cacheStats := sm.resultCache.GetStats()
	
	stats := &IndexStats{
		DocumentCount:   int64(docCount),
		IndexSize:       indexSize,
		IndexSizeHuman:  formatBytes(indexSize),
		LastIndexTime:   metrics.LastIndexTime,
		SearchCount:     metrics.SearchQueries,
		AvgSearchTime:   time.Duration(metrics.AvgSearchTimeMs * float64(time.Millisecond)),
		QueueSize:       len(sm.indexQueue),
		Workers:         sm.config.Workers,
		BatchSize:       sm.config.BatchSize,
		LastOptimized:   time.Now(), // TODO: Track this properly
		ErrorCount:      metrics.IndexingErrors,
		CacheStats:      &cacheStats,
	}
	
	return stats, nil
}

// OptimizeIndex optimizes the search index for better performance
func (sm *SearchManager) OptimizeIndex() error {
	if !sm.started {
		return fmt.Errorf("search manager not started")
	}
	
	// Bleve doesn't have a direct optimize method, but we can
	// trigger a merge by updating the merge policy
	// For now, this is a no-op placeholder
	return nil
}

// Close gracefully shuts down the search service
func (sm *SearchManager) Close() error {
	return sm.Stop()
}

// Helper methods

func (sm *SearchManager) updateSearchMetrics(duration time.Duration) {
	sm.metrics.mutex.Lock()
	defer sm.metrics.mutex.Unlock()
	
	sm.metrics.SearchQueries++
	
	// Update average search time (simple moving average)
	if sm.metrics.AvgSearchTimeMs == 0 {
		sm.metrics.AvgSearchTimeMs = float64(duration.Milliseconds())
	} else {
		sm.metrics.AvgSearchTimeMs = (sm.metrics.AvgSearchTimeMs*float64(sm.metrics.SearchQueries-1) + 
			float64(duration.Milliseconds())) / float64(sm.metrics.SearchQueries)
	}
}

func (sm *SearchManager) calculateDirSize(path string) int64 {
	var size int64
	filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func float64Ptr(f float64) *float64 {
	return &f
}