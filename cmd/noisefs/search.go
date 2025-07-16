package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/fuse"
	"github.com/TheEntropyCollective/noisefs/pkg/search"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	// Mock backend registration is handled in backends.go
	"github.com/TheEntropyCollective/noisefs/pkg/util"
)

// fileIndexAdapter adapts fuse.FileIndex to search.FileIndexInterface
type fileIndexAdapter struct {
	fileIndex *fuse.FileIndex
}

func (a *fileIndexAdapter) AddFile(path, descriptorCID string, fileSize int64) {
	a.fileIndex.AddFile(path, descriptorCID, fileSize)
}

func (a *fileIndexAdapter) RemoveFile(path string) bool {
	return a.fileIndex.RemoveFile(path)
}

func (a *fileIndexAdapter) UpdateFile(path, descriptorCID string, fileSize int64) bool {
	return a.fileIndex.UpdateFile(path, descriptorCID, fileSize)
}

func (a *fileIndexAdapter) ListFiles() map[string]interface{} {
	entries := a.fileIndex.ListFiles()
	result := make(map[string]interface{})
	for k, v := range entries {
		result[k] = map[string]interface{}{
			"DescriptorCID": v.DescriptorCID,
			"FileSize":      v.FileSize,
			"ModifiedAt":    v.ModifiedAt,
		}
	}
	return result
}

func (a *fileIndexAdapter) GetFile(path string) (interface{}, bool) {
	entry, exists := a.fileIndex.GetFile(path)
	if !exists {
		return nil, false
	}
	return map[string]interface{}{
		"DescriptorCID": entry.DescriptorCID,
		"FileSize":      entry.FileSize,
		"ModifiedAt":    entry.ModifiedAt,
	}, true
}

func (a *fileIndexAdapter) GetDirectory(path string) ([]interface{}, bool) {
	entry, exists := a.fileIndex.GetDirectory(path)
	if !exists {
		return nil, false
	}
	// For directories, return a single entry
	return []interface{}{
		map[string]interface{}{
			"Path":          path,
			"DescriptorCID": entry.DescriptorCID,
			"FileSize":      entry.FileSize,
			"ModifiedAt":    entry.ModifiedAt,
		},
	}, true
}

// handleSearchCommand handles the search subcommand
func handleSearchCommand(args []string) {
	searchCmd := flag.NewFlagSet("search", flag.ExitOnError)
	
	// Search options
	query := searchCmd.String("q", "", "Search query")
	namePattern := searchCmd.String("name", "", "Search by filename pattern (supports wildcards)")
	pathPattern := searchCmd.String("path", "", "Search by path pattern")
	directory := searchCmd.String("dir", "", "Search within specific directory")
	recursive := searchCmd.Bool("r", true, "Search recursively in directories")
	
	// Filter options
	fileTypes := searchCmd.String("type", "", "File types to search (comma-separated)")
	minSize := searchCmd.String("min-size", "", "Minimum file size (e.g., 1MB, 500KB)")
	maxSize := searchCmd.String("max-size", "", "Maximum file size")
	modifiedAfter := searchCmd.String("modified-after", "", "Files modified after date (YYYY-MM-DD)")
	modifiedBefore := searchCmd.String("modified-before", "", "Files modified before date (YYYY-MM-DD)")
	
	// Output options
	maxResults := searchCmd.Int("max", 20, "Maximum number of results")
	offset := searchCmd.Int("offset", 0, "Result offset for pagination")
	sortBy := searchCmd.String("sort", "score", "Sort by: score, name, size, modified, path")
	sortOrder := searchCmd.String("order", "desc", "Sort order: asc, desc")
	jsonOutput := searchCmd.Bool("json", false, "Output results in JSON format")
	showScore := searchCmd.Bool("show-score", false, "Show relevance score")
	highlight := searchCmd.Bool("highlight", true, "Highlight matching terms")
	
	// Index management
	rebuildIndex := searchCmd.Bool("rebuild", false, "Rebuild search index")
	indexStats := searchCmd.Bool("stats", false, "Show search index statistics")
	
	// Configuration
	indexPath := searchCmd.String("index", "", "Custom file index path")
	searchIndexPath := searchCmd.String("search-index", "", "Custom search index path")
	
	searchCmd.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: noisefs search [options] [query]\n\n")
		fmt.Fprintf(os.Stderr, "Search for files in NoiseFS using full-text search and metadata filters.\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  noisefs search \"important document\"     # Full-text search\n")
		fmt.Fprintf(os.Stderr, "  noisefs search --name \"*.pdf\"           # Search by filename\n")
		fmt.Fprintf(os.Stderr, "  noisefs search --type pdf,docx          # Search by file type\n")
		fmt.Fprintf(os.Stderr, "  noisefs search --min-size 1MB           # Search by size\n")
		fmt.Fprintf(os.Stderr, "  noisefs search --stats                  # Show index statistics\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		searchCmd.PrintDefaults()
	}
	
	if err := searchCmd.Parse(args); err != nil {
		return
	}
	
	// Get query from remaining args if not specified with -q
	remainingArgs := searchCmd.Args()
	if *query == "" && len(remainingArgs) > 0 {
		*query = strings.Join(remainingArgs, " ")
	}
	
	// Initialize file index
	if *indexPath == "" {
		var err error
		*indexPath, err = fuse.GetDefaultIndexPath()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}
	
	fileIndex := fuse.NewFileIndex(*indexPath)
	if err := fileIndex.LoadIndex(); err != nil {
		fmt.Fprintf(os.Stderr, "Error loading file index: %v\n", err)
		os.Exit(1)
	}
	
	// Initialize storage manager (required for search)
	// Use mock backend to avoid IPFS dependency for search testing
	storageConfig := &storage.Config{
		DefaultBackend: "mock",
		Backends: map[string]*storage.BackendConfig{
			"mock": {
				Type:    "mock",
				Enabled: true,
				Priority: 100,
				Connection: &storage.ConnectionConfig{
					Endpoint: "memory://mock",
				},
			},
		},
		HealthCheck: &storage.HealthCheckConfig{
			Enabled: false, // Disable health checks for mock
		},
	}
	
	storageManager, err := storage.NewManager(storageConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating storage manager: %v\n", err)
		os.Exit(1)
	}
	
	// Initialize search manager
	searchConfig := search.DefaultSearchConfig()
	if *searchIndexPath != "" {
		searchConfig.IndexPath = *searchIndexPath
	}
	
	// Create adapter for file index
	indexAdapter := &fileIndexAdapter{fileIndex: fileIndex}
	
	searchManager, err := search.NewSearchManager(searchConfig, indexAdapter, storageManager)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating search manager: %v\n", err)
		os.Exit(1)
	}
	
	if err := searchManager.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting search manager: %v\n", err)
		os.Exit(1)
	}
	defer searchManager.Stop()
	
	// Handle special operations
	if *rebuildIndex {
		fmt.Println("Rebuilding search index...")
		if err := searchManager.RebuildIndex(); err != nil {
			fmt.Fprintf(os.Stderr, "Error rebuilding index: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Index rebuild queued successfully")
		return
	}
	
	if *indexStats {
		showIndexStats(searchManager, *jsonOutput)
		return
	}
	
	// Build search options
	options := search.SearchOptions{
		MaxResults: *maxResults,
		Offset:     *offset,
		Highlight:  *highlight,
		Directory:  *directory,
		Recursive:  *recursive,
	}
	
	// Parse sort options
	switch *sortBy {
	case "name":
		options.SortBy = search.SortByName
	case "size":
		options.SortBy = search.SortBySize
	case "modified":
		options.SortBy = search.SortByModified
	case "path":
		options.SortBy = search.SortByPath
	default:
		options.SortBy = search.SortByScore
	}
	
	if *sortOrder == "asc" {
		options.SortOrder = search.SortAsc
	} else {
		options.SortOrder = search.SortDesc
	}
	
	// Parse file types
	if *fileTypes != "" {
		options.FileTypes = strings.Split(*fileTypes, ",")
		for i := range options.FileTypes {
			options.FileTypes[i] = strings.TrimSpace(options.FileTypes[i])
		}
	}
	
	// Parse size ranges
	if *minSize != "" || *maxSize != "" {
		options.SizeRange = &search.SizeRange{}
		if *minSize != "" {
			min, err := util.ParseSize(*minSize)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Invalid min-size: %v\n", err)
				os.Exit(1)
			}
			options.SizeRange.Min = min
		}
		if *maxSize != "" {
			max, err := util.ParseSize(*maxSize)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Invalid max-size: %v\n", err)
				os.Exit(1)
			}
			options.SizeRange.Max = max
		}
	}
	
	// Parse time ranges
	if *modifiedAfter != "" || *modifiedBefore != "" {
		options.TimeRange = &search.TimeRange{}
		if *modifiedAfter != "" {
			start, err := time.Parse("2006-01-02", *modifiedAfter)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Invalid modified-after date: %v\n", err)
				os.Exit(1)
			}
			options.TimeRange.Start = start
		}
		if *modifiedBefore != "" {
			end, err := time.Parse("2006-01-02", *modifiedBefore)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Invalid modified-before date: %v\n", err)
				os.Exit(1)
			}
			options.TimeRange.End = end.Add(24 * time.Hour) // Include the entire day
		}
	}
	
	// Perform search
	var results *search.SearchResults
	if *query != "" {
		// Full-text search
		results, err = searchManager.Search(*query, options)
	} else if *namePattern != "" || *pathPattern != "" || len(options.FileTypes) > 0 {
		// Metadata search
		filters := search.MetadataFilters{
			NamePattern: *namePattern,
			PathPattern: *pathPattern,
			FileTypes:   options.FileTypes,
			SizeRange:   options.SizeRange,
			TimeRange:   options.TimeRange,
			Directory:   options.Directory,
			Recursive:   options.Recursive,
		}
		results, err = searchManager.SearchMetadata(filters)
	} else {
		fmt.Fprintf(os.Stderr, "No search query or filters specified\n")
		searchCmd.Usage()
		os.Exit(1)
	}
	
	if err != nil {
		fmt.Fprintf(os.Stderr, "Search failed: %v\n", err)
		os.Exit(1)
	}
	
	// Display results
	if *jsonOutput {
		displayJSONResults(results)
	} else {
		displayTextResults(results, *showScore)
	}
}

func showIndexStats(searchManager *search.SearchManager, jsonOutput bool) {
	stats, err := searchManager.GetIndexStats()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting index stats: %v\n", err)
		os.Exit(1)
	}
	
	if jsonOutput {
		data, _ := json.MarshalIndent(stats, "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Println("Search Index Statistics")
		fmt.Println("======================")
		fmt.Printf("Document Count:    %d\n", stats.DocumentCount)
		fmt.Printf("Index Size:        %s\n", stats.IndexSizeHuman)
		fmt.Printf("Search Queries:    %d\n", stats.SearchCount)
		fmt.Printf("Avg Search Time:   %v\n", stats.AvgSearchTime)
		fmt.Printf("Queue Size:        %d\n", stats.QueueSize)
		fmt.Printf("Workers:           %d\n", stats.Workers)
		fmt.Printf("Error Count:       %d\n", stats.ErrorCount)
		if !stats.LastIndexTime.IsZero() {
			fmt.Printf("Last Indexed:      %s\n", stats.LastIndexTime.Format("2006-01-02 15:04:05"))
		}
		if !stats.LastOptimized.IsZero() {
			fmt.Printf("Last Optimized:    %s\n", stats.LastOptimized.Format("2006-01-02 15:04:05"))
		}
	}
}

func displayJSONResults(results *search.SearchResults) {
	data, _ := json.MarshalIndent(results, "", "  ")
	fmt.Println(string(data))
}

func displayTextResults(results *search.SearchResults, showScore bool) {
	if results.Total == 0 {
		fmt.Println("No results found")
		return
	}
	
	fmt.Printf("Found %d results (showing %d-%d):\n\n", 
		results.Total, 
		results.Offset+1, 
		min(results.Offset+len(results.Results), results.Total))
	
	for i, result := range results.Results {
		fmt.Printf("%d. %s\n", results.Offset+i+1, result.Path)
		
		if showScore {
			fmt.Printf("   Score: %.4f\n", result.Score)
		}
		
		fmt.Printf("   Size: %s | Modified: %s", 
			util.FormatSize(result.Size),
			result.ModifiedAt.Format("2006-01-02 15:04"))
		
		if result.MimeType != "" && result.MimeType != "application/octet-stream" {
			fmt.Printf(" | Type: %s", result.MimeType)
		}
		
		fmt.Println()
		
		if result.Preview != "" {
			preview := result.Preview
			if len(preview) > 200 {
				preview = preview[:200] + "..."
			}
			fmt.Printf("   %s\n", preview)
		}
		
		if len(result.Highlights) > 0 {
			fmt.Printf("   Matches: %s\n", strings.Join(result.Highlights[:min(3, len(result.Highlights))], " ... "))
		}
		
		fmt.Println()
	}
	
	// Show pagination info
	if results.HasMore {
		fmt.Printf("More results available. Use --offset %d to see next page.\n", 
			results.Offset+len(results.Results))
	}
	
	fmt.Printf("\nSearch completed in %v\n", results.TimeTaken)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}