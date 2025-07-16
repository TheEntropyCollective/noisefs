package search

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/storage"
)

func BenchmarkSearchIndexing(b *testing.B) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "noisefs-search-bench")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test configuration
	config := SearchConfig{
		IndexPath:      filepath.Join(tmpDir, "search", "bench.bleve"),
		Workers:        4,
		BatchSize:      100,
		ContentPreview: 500,
		SupportedTypes: []string{"txt", "md", "json"},
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
	storageManager, _ := storage.NewManager(storageConfig)

	// Create search manager
	searchManager, err := NewSearchManager(config, fileIndex, storageManager)
	if err != nil {
		b.Fatalf("Failed to create search manager: %v", err)
	}

	if err := searchManager.Start(); err != nil {
		b.Fatalf("Failed to start search manager: %v", err)
	}
	defer searchManager.Stop()

	// Generate test data
	numFiles := 1000
	for i := 0; i < numFiles; i++ {
		path := fmt.Sprintf("documents/file%d.txt", i)
		cid := fmt.Sprintf("QmTest%d", i)
		size := int64(1024 + i*100)
		fileIndex.AddFile(path, cid, size)
	}

	b.ResetTimer()

	// Benchmark indexing
	b.Run("IndexFiles", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			path := fmt.Sprintf("documents/bench%d.txt", i)
			metadata := FileMetadata{
				Path:          path,
				DescriptorCID: fmt.Sprintf("QmBench%d", i),
				Size:          int64(1024),
				ModifiedAt:    time.Now(),
				MimeType:      "text/plain",
				FileType:      "txt",
			}

			if err := searchManager.UpdateIndex(path, metadata); err != nil {
				b.Errorf("Failed to index: %v", err)
			}
		}

		// Wait for indexing to complete
		time.Sleep(500 * time.Millisecond)
	})
}

func BenchmarkSearchQueries(b *testing.B) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "noisefs-search-query-bench")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Setup search manager
	config := SearchConfig{
		IndexPath:      filepath.Join(tmpDir, "search", "query.bleve"),
		Workers:        2,
		BatchSize:      50,
		ContentPreview: 200,
	}

	fileIndex := NewMockFileIndex()
	storageConfig := &storage.Config{
		DefaultBackend: "mock",
		Backends: map[string]*storage.BackendConfig{
			"mock": {Type: "mock", Enabled: true},
		},
	}
	storageManager, _ := storage.NewManager(storageConfig)

	searchManager, _ := NewSearchManager(config, fileIndex, storageManager)
	searchManager.Start()
	defer searchManager.Stop()

	// Pre-populate index with test data
	for i := 0; i < 10000; i++ {
		path := fmt.Sprintf("test/file%d.txt", i)
		metadata := FileMetadata{
			Path:           path,
			DescriptorCID:  fmt.Sprintf("Qm%d", i),
			Size:           int64(1024 * (i%100 + 1)),
			ModifiedAt:     time.Now().Add(-time.Duration(i) * time.Hour),
			MimeType:       "text/plain",
			FileType:       "txt",
			ContentPreview: fmt.Sprintf("This is test file number %d with some content", i),
		}

		searchManager.UpdateIndex(path, metadata)
	}

	// Wait for indexing
	time.Sleep(1 * time.Second)

	// Benchmark different query types
	b.Run("SimpleTextSearch", func(b *testing.B) {
		options := SearchOptions{MaxResults: 20}
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			query := fmt.Sprintf("file%d", i%1000)
			_, err := searchManager.Search(query, options)
			if err != nil {
				b.Errorf("Search failed: %v", err)
			}
		}
	})

	b.Run("WildcardSearch", func(b *testing.B) {
		filters := MetadataFilters{
			NamePattern: "file*.txt",
		}
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			_, err := searchManager.SearchMetadata(filters)
			if err != nil {
				b.Errorf("Metadata search failed: %v", err)
			}
		}
	})

	b.Run("RangeSearch", func(b *testing.B) {
		filters := MetadataFilters{
			SizeRange: &SizeRange{
				Min: 1024,
				Max: 50 * 1024,
			},
		}
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			_, err := searchManager.SearchMetadata(filters)
			if err != nil {
				b.Errorf("Range search failed: %v", err)
			}
		}
	})
}

func TestSearchConcurrency(t *testing.T) {
	// Test concurrent indexing and searching
	tmpDir, err := os.MkdirTemp("", "noisefs-search-concurrent")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	config := SearchConfig{
		IndexPath: filepath.Join(tmpDir, "search", "concurrent.bleve"),
		Workers:   8,
		BatchSize: 100,
	}

	fileIndex := NewMockFileIndex()
	storageConfig := &storage.Config{
		DefaultBackend: "mock",
		Backends: map[string]*storage.BackendConfig{
			"mock": {Type: "mock", Enabled: true},
		},
	}
	storageManager, _ := storage.NewManager(storageConfig)

	searchManager, _ := NewSearchManager(config, fileIndex, storageManager)
	searchManager.Start()
	defer searchManager.Stop()

	// Run concurrent operations
	done := make(chan bool)
	errors := make(chan error, 100)

	// Concurrent indexing
	for i := 0; i < 10; i++ {
		go func(worker int) {
			for j := 0; j < 100; j++ {
				path := fmt.Sprintf("worker%d/file%d.txt", worker, j)
				metadata := FileMetadata{
					Path:          path,
					DescriptorCID: fmt.Sprintf("Qm%d%d", worker, j),
					Size:          int64(1024),
					ModifiedAt:    time.Now(),
				}

				if err := searchManager.UpdateIndex(path, metadata); err != nil {
					errors <- err
				}
			}
			done <- true
		}(i)
	}

	// Concurrent searching
	for i := 0; i < 5; i++ {
		go func(searcher int) {
			for j := 0; j < 50; j++ {
				query := fmt.Sprintf("file%d", j)
				options := SearchOptions{MaxResults: 10}

				if _, err := searchManager.Search(query, options); err != nil {
					errors <- err
				}
			}
			done <- true
		}(i)
	}

	// Wait for all operations to complete
	for i := 0; i < 15; i++ {
		select {
		case <-done:
			// Operation completed
		case err := <-errors:
			t.Errorf("Concurrent operation failed: %v", err)
		case <-time.After(10 * time.Second):
			t.Fatal("Timeout waiting for concurrent operations")
		}
	}

	// Verify final state
	stats, err := searchManager.GetIndexStats()
	if err != nil {
		t.Fatalf("Failed to get stats: %v", err)
	}

	if stats.DocumentCount < 1000 {
		t.Errorf("Expected at least 1000 documents, got %d", stats.DocumentCount)
	}
}

func TestSearchMemoryUsage(t *testing.T) {
	// Test memory usage with large content
	tmpDir, err := os.MkdirTemp("", "noisefs-search-memory")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	config := SearchConfig{
		IndexPath:      filepath.Join(tmpDir, "search", "memory.bleve"),
		Workers:        2,
		BatchSize:      10,
		ContentPreview: 1024,             // 1KB preview
		MaxFileSize:    10 * 1024 * 1024, // 10MB max
	}

	fileIndex := NewMockFileIndex()
	storageConfig := &storage.Config{
		DefaultBackend: "mock",
		Backends: map[string]*storage.BackendConfig{
			"mock": {Type: "mock", Enabled: true},
		},
	}
	storageManager, _ := storage.NewManager(storageConfig)

	searchManager, _ := NewSearchManager(config, fileIndex, storageManager)
	searchManager.Start()
	defer searchManager.Stop()

	// Create large content
	largeContent := make([]byte, 5*1024*1024) // 5MB
	for i := range largeContent {
		largeContent[i] = byte('A' + (i % 26))
	}

	// Index large file
	metadata := FileMetadata{
		Path:           "large/file.txt",
		DescriptorCID:  "QmLarge",
		Size:           int64(len(largeContent)),
		ModifiedAt:     time.Now(),
		Content:        string(largeContent[:config.ContentPreview]), // Only preview
		ContentPreview: string(largeContent[:config.ContentPreview]),
	}

	if err := searchManager.UpdateIndex(metadata.Path, metadata); err != nil {
		t.Fatalf("Failed to index large file: %v", err)
	}

	// Wait for indexing
	time.Sleep(500 * time.Millisecond)

	// Search and verify preview
	results, err := searchManager.Search("file", SearchOptions{MaxResults: 10})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results.Results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results.Results))
	}

	if len(results.Results) > 0 && len(results.Results[0].Preview) > config.ContentPreview+100 {
		t.Errorf("Preview too large: %d bytes", len(results.Results[0].Preview))
	}
}
