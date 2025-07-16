package search

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/storage"
)

func TestSearchManager(t *testing.T) {
	// Create temporary directory for test
	tmpDir, err := os.MkdirTemp("", "noisefs-search-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test configuration
	config := SearchConfig{
		IndexPath:      filepath.Join(tmpDir, "search", "test.bleve"),
		MaxIndexSize:   1024 * 1024 * 10, // 10MB
		Workers:        2,
		BatchSize:      10,
		ContentPreview: 200,
		SupportedTypes: []string{"txt", "md"},
		DefaultResults: 20,
		MaxResults:     100,
		EncryptIndex:   false,
	}

	// Create file index
	fileIndex := NewMockFileIndex()

	// Create mock storage manager
	storageConfig := &storage.Config{
		DefaultBackend: "mock",
		Backends: map[string]*storage.BackendConfig{
			"mock": {
				Type:    "mock",
				Enabled: true,
				Connection: &storage.ConnectionConfig{
					Endpoint: "memory://mock",
				},
			},
		},
		HealthCheck: &storage.HealthCheckConfig{
			Enabled: false, // Disable health checks for tests
		},
		Distribution: &storage.DistributionConfig{
			Strategy: "single",
		},
	}
	storageManager, err := storage.NewManager(storageConfig)
	if err != nil {
		t.Fatalf("Failed to create storage manager: %v", err)
	}

	// Create search manager
	searchManager, err := NewSearchManager(config, fileIndex, storageManager)
	if err != nil {
		t.Fatalf("Failed to create search manager: %v", err)
	}

	// Start the search manager
	err = searchManager.Start()
	if err != nil {
		t.Fatalf("Failed to start search manager: %v", err)
	}
	defer searchManager.Stop()

	// Test 1: Add files to index
	t.Run("IndexFiles", func(t *testing.T) {
		// Add test files to file index
		testFiles := []struct {
			path string
			cid  string
			size int64
		}{
			{"documents/readme.txt", "QmTest1", 1024},
			{"documents/guide.md", "QmTest2", 2048},
			{"images/photo.jpg", "QmTest3", 4096},
		}

		for _, tf := range testFiles {
			fileIndex.AddFile(tf.path, tf.cid, tf.size)
		}

		// Index the files
		for _, tf := range testFiles {
			metadata := FileMetadata{
				Path:          tf.path,
				DescriptorCID: tf.cid,
				Size:          tf.size,
				ModifiedAt:    time.Now(),
				MimeType:      getMimeType(tf.path),
				FileType:      getFileType(tf.path),
			}

			err := searchManager.UpdateIndex(tf.path, metadata)
			if err != nil {
				t.Errorf("Failed to index file %s: %v", tf.path, err)
			}
		}

		// Wait for indexing to complete
		searchManager.WaitForIndexing()

		// Check index stats
		stats, err := searchManager.GetIndexStats()
		if err != nil {
			t.Fatalf("Failed to get index stats: %v", err)
		}

		
		if stats.DocumentCount != 3 {
			t.Errorf("Expected 3 documents, got %d", stats.DocumentCount)
		}
	})

	// Test 2: Search by filename
	t.Run("SearchByFilename", func(t *testing.T) {
		options := SearchOptions{
			MaxResults: 10,
		}

		results, err := searchManager.Search("readme", options)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		if results.Total != 1 {
			t.Errorf("Expected 1 result, got %d", results.Total)
		}

		if len(results.Results) != 1 {
			t.Errorf("Expected 1 result item, got %d", len(results.Results))
			return // Prevent panic on next line
		}

		if results.Results[0].Path != "documents/readme.txt" {
			t.Errorf("Expected path 'documents/readme.txt', got '%s'", results.Results[0].Path)
		}
	})

	// Test 3: Search by file type
	t.Run("SearchByFileType", func(t *testing.T) {
		filters := MetadataFilters{
			FileTypes: []string{"txt", "md"},
		}

		results, err := searchManager.SearchMetadata(filters)
		if err != nil {
			t.Fatalf("Metadata search failed: %v", err)
		}

		if results.Total != 2 {
			t.Errorf("Expected 2 results, got %d", results.Total)
		}
	})

	// Test 4: Remove from index
	t.Run("RemoveFromIndex", func(t *testing.T) {
		err := searchManager.RemoveFromIndex("documents/readme.txt")
		if err != nil {
			t.Fatalf("Failed to remove from index: %v", err)
		}

		// Wait for removal to complete
		searchManager.WaitForIndexing()

		// Search again
		results, err := searchManager.Search("readme", SearchOptions{MaxResults: 10})
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		if results.Total != 0 {
			t.Errorf("Expected 0 results after removal, got %d", results.Total)
		}
	})
}

func getMimeType(path string) string {
	ext := filepath.Ext(path)
	switch ext {
	case ".txt":
		return "text/plain"
	case ".md":
		return "text/markdown"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	default:
		return "application/octet-stream"
	}
}

func getFileType(path string) string {
	ext := filepath.Ext(path)
	if ext != "" && ext[0] == '.' {
		return ext[1:]
	}
	return ""
}

func TestSearchOptions(t *testing.T) {
	// Test search options validation and defaults
	t.Run("DefaultOptions", func(t *testing.T) {
		options := SearchOptions{}

		if options.MaxResults != 0 {
			t.Errorf("Expected MaxResults to be 0 (unset), got %d", options.MaxResults)
		}

		if options.SortOrder != "" {
			t.Errorf("Expected empty SortOrder, got %s", options.SortOrder)
		}
	})

	t.Run("TimeRange", func(t *testing.T) {
		now := time.Now()
		yesterday := now.Add(-24 * time.Hour)

		tr := &TimeRange{
			Start: yesterday,
			End:   now,
		}

		if tr.Start.After(tr.End) {
			t.Errorf("Invalid time range: start after end")
		}
	})
}

func TestContentExtractor(t *testing.T) {
	// Create mock storage
	storageConfig := &storage.Config{
		DefaultBackend: "mock",
		Backends: map[string]*storage.BackendConfig{
			"mock": {
				Type:    "mock",
				Enabled: true,
				Connection: &storage.ConnectionConfig{
					Endpoint: "memory://mock",
				},
			},
		},
		HealthCheck: &storage.HealthCheckConfig{
			Enabled: false, // Disable health checks for tests
		},
		Distribution: &storage.DistributionConfig{
			Strategy: "single",
		},
	}
	storageManager, err := storage.NewManager(storageConfig)
	if err != nil {
		t.Fatalf("Failed to create storage manager: %v", err)
	}

	config := DefaultSearchConfig()
	extractor := NewContentExtractor(storageManager, config)

	t.Run("ShouldExtractContent", func(t *testing.T) {
		tests := []struct {
			path     string
			expected bool
		}{
			{"file.txt", true},
			{"document.md", true},
			{"data.json", true},
			{"image.jpg", false},
			{"binary.exe", false},
		}

		for _, test := range tests {
			result := extractor.shouldExtractContent(test.path)
			if result != test.expected {
				t.Errorf("For %s: expected %v, got %v", test.path, test.expected, result)
			}
		}
	})

	t.Run("CreatePreview", func(t *testing.T) {
		longText := "This is a long text that should be truncated. " +
			"It contains multiple sentences and paragraphs. " +
			"The preview should cut at a reasonable point. " +
			"This sentence should not be included in the preview."

		// Make sure the text is longer than ContentPreview to trigger truncation
		for len(longText) <= config.ContentPreview {
			longText += " Additional text to make it longer."
		}

		preview := extractor.createPreview(longText)

		if len(preview) > config.ContentPreview+10 {
			t.Errorf("Preview too long: %d chars", len(preview))
		}

		if !strings.HasSuffix(preview, "...") {
			t.Errorf("Preview should end with '...', got: %s", preview)
		}
	})

	t.Run("CleanText", func(t *testing.T) {
		dirtyText := "Hello\tWorld\n\nMultiple   spaces\x00\x01"
		cleaned := extractor.cleanText(dirtyText)

		if strings.Contains(cleaned, "\t") {
			t.Errorf("Cleaned text still contains tabs")
		}

		if strings.Contains(cleaned, "\x00") {
			t.Errorf("Cleaned text still contains null bytes")
		}

		if strings.Contains(cleaned, "  ") {
			t.Errorf("Cleaned text still contains multiple spaces")
		}
	})
}
