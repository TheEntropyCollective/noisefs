package index

import (
	"fmt"
	"testing"
	"time"
)

// TestPrivacyIndexBasicOperations tests core privacy index functionality
func TestPrivacyIndexBasicOperations(t *testing.T) {
	config := DefaultPrivacyIndexConfig()
	config.ExpectedFiles = 100
	config.ExpectedDirectories = 10
	
	index, err := NewPrivacyIndex(config)
	if err != nil {
		t.Fatalf("Failed to create privacy index: %v", err)
	}
	
	// Test filename indexing
	testFiles := []struct {
		filename string
		metadata FileMetadata
	}{
		{
			filename: "encrypted_file_1",
			metadata: FileMetadata{
				Size:        1024,
				ModTime:     time.Now(),
				ContentType: "text/plain",
			},
		},
		{
			filename: "encrypted_file_2", 
			metadata: FileMetadata{
				Size:        2048,
				ModTime:     time.Now().Add(-24 * time.Hour),
				ContentType: "image/jpeg",
			},
		},
	}
	
	// Index filenames
	for _, file := range testFiles {
		err := index.IndexFilename([]byte(file.filename), file.metadata)
		if err != nil {
			t.Errorf("Failed to index filename %s: %v", file.filename, err)
		}
	}
	
	// Query existing filenames
	for _, file := range testFiles {
		found, err := index.QueryFilename([]byte(file.filename))
		if err != nil {
			t.Errorf("Error querying filename %s: %v", file.filename, err)
		}
		if !found {
			t.Errorf("Filename %s should be found but wasn't", file.filename)
		}
	}
	
	// Query non-existent filename
	_, err = index.QueryFilename([]byte("nonexistent_file"))
	if err != nil {
		t.Errorf("Error querying non-existent filename: %v", err)
	}
	// Note: found might be true due to false positives, which is expected
	
	// Test content indexing
	contentFingerprints := []string{"content_hash_1", "content_hash_2"}
	for i, fingerprint := range contentFingerprints {
		blockCID := fmt.Sprintf("block_cid_%d", i)
		err := index.IndexContent([]byte(fingerprint), blockCID)
		if err != nil {
			t.Errorf("Failed to index content fingerprint %s: %v", fingerprint, err)
		}
	}
}

// TestPrivacyIndexStatistics tests statistics collection and monitoring
func TestPrivacyIndexStatistics(t *testing.T) {
	config := DefaultPrivacyIndexConfig()
	config.ExpectedFiles = 50
	
	index, err := NewPrivacyIndex(config)
	if err != nil {
		t.Fatalf("Failed to create privacy index: %v", err)
	}
	
	// Perform some operations
	numOperations := 10
	for i := 0; i < numOperations; i++ {
		filename := fmt.Sprintf("test_file_%d", i)
		metadata := FileMetadata{
			Size:        int64(1024 * (i + 1)),
			ModTime:     time.Now(),
			ContentType: "application/octet-stream",
		}
		
		err := index.IndexFilename([]byte(filename), metadata)
		if err != nil {
			t.Errorf("Failed to index filename: %v", err)
		}
		
		// Query the same filename
		_, err = index.QueryFilename([]byte(filename))
		if err != nil {
			t.Errorf("Failed to query filename: %v", err)
		}
	}
	
	// Get and validate statistics
	stats := index.GetStats()
	
	if stats.TotalQueries < uint64(numOperations) {
		t.Errorf("Expected at least %d queries, got %d", numOperations, stats.TotalQueries)
	}
	
	if stats.MemoryUsage == 0 {
		t.Error("Memory usage should be greater than 0")
	}
	
	if stats.AnonymitySetSize <= 0 {
		t.Error("Anonymity set size should be positive")
	}
	
	if stats.DifferentialBudget < 0 || stats.DifferentialBudget > 1 {
		t.Errorf("Differential privacy budget should be between 0 and 1, got %f", stats.DifferentialBudget)
	}
	
	t.Logf("Index Statistics:")
	t.Logf("  Total Queries: %d", stats.TotalQueries)
	t.Logf("  Successful Queries: %d", stats.SuccessfulQueries)
	t.Logf("  Memory Usage: %d bytes", stats.MemoryUsage)
	t.Logf("  Average Query Time: %v", stats.AverageQueryTime)
	t.Logf("  Anonymity Set Size: %d", stats.AnonymitySetSize)
	t.Logf("  Differential Budget: %.3f", stats.DifferentialBudget)
	t.Logf("  Filter Load Factors:")
	t.Logf("    Filename: %.4f", stats.FilenameFilterLoad)
	t.Logf("    Content: %.4f", stats.ContentFilterLoad)
	t.Logf("    Metadata: %.4f", stats.MetadataFilterLoad)
	t.Logf("    Directory: %.4f", stats.DirectoryFilterLoad)
}

// TestPrivacyIndexConfiguration tests different privacy configurations
func TestPrivacyIndexConfiguration(t *testing.T) {
	testConfigs := []struct {
		name         string
		privacyLevel int
		fpr          float64
	}{
		{"Low Privacy", 1, 0.05},
		{"Medium Privacy", 3, 0.01},
		{"High Privacy", 5, 0.001},
	}
	
	for _, tc := range testConfigs {
		t.Run(tc.name, func(t *testing.T) {
			config := DefaultPrivacyIndexConfig()
			config.PrivacyLevel = tc.privacyLevel
			config.FalsePositiveRate = tc.fpr
			config.ExpectedFiles = 100
			
			index, err := NewPrivacyIndex(config)
			if err != nil {
				t.Fatalf("Failed to create privacy index: %v", err)
			}
			
			// Perform some operations
			for i := 0; i < 10; i++ {
				filename := fmt.Sprintf("privacy_test_%d", i)
				metadata := FileMetadata{Size: 1024, ModTime: time.Now()}
				
				err := index.IndexFilename([]byte(filename), metadata)
				if err != nil {
					t.Errorf("Failed to index filename: %v", err)
				}
			}
			
			stats := index.GetStats()
			t.Logf("%s - Memory: %d bytes, Queries: %d", tc.name, stats.MemoryUsage, stats.TotalQueries)
		})
	}
}

// TestPrivacyIndexErrorHandling tests error conditions
func TestPrivacyIndexErrorHandling(t *testing.T) {
	// Test invalid configurations
	invalidConfigs := []*PrivacyIndexConfig{
		{PrivacyLevel: 0}, // Invalid privacy level
		{PrivacyLevel: 6}, // Invalid privacy level
		{FalsePositiveRate: 0}, // Invalid FPR
		{FalsePositiveRate: 1}, // Invalid FPR
		{MinAnonymitySet: 0}, // Invalid anonymity set
	}
	
	for i, config := range invalidConfigs {
		t.Run(fmt.Sprintf("InvalidConfig_%d", i), func(t *testing.T) {
			_, err := NewPrivacyIndex(config)
			if err == nil {
				t.Error("Expected error for invalid configuration")
			}
		})
	}
	
	// Test valid index with invalid operations
	config := DefaultPrivacyIndexConfig()
	config.ExpectedFiles = 10
	
	index, err := NewPrivacyIndex(config)
	if err != nil {
		t.Fatalf("Failed to create privacy index: %v", err)
	}
	
	// Test empty filename
	err = index.IndexFilename([]byte{}, FileMetadata{})
	if err == nil {
		t.Error("Expected error when indexing empty filename")
	}
	
	_, err = index.QueryFilename([]byte{})
	if err == nil {
		t.Error("Expected error when querying empty filename")
	}
	
	// Test empty content fingerprint
	err = index.IndexContent([]byte{}, "block_cid")
	if err == nil {
		t.Error("Expected error when indexing empty content fingerprint")
	}
}

// TestPrivacyIndexMaintenance tests maintenance operations
func TestPrivacyIndexMaintenance(t *testing.T) {
	config := DefaultPrivacyIndexConfig()
	config.ExpectedFiles = 50
	
	index, err := NewPrivacyIndex(config)
	if err != nil {
		t.Fatalf("Failed to create privacy index: %v", err)
	}
	
	// Perform maintenance (should be no-op initially)
	err = index.Maintenance()
	if err != nil {
		t.Errorf("Maintenance failed: %v", err)
	}
	
	// Add some data
	for i := 0; i < 20; i++ {
		filename := fmt.Sprintf("maintenance_test_%d", i)
		metadata := FileMetadata{Size: 1024, ModTime: time.Now()}
		
		index.IndexFilename([]byte(filename), metadata)
		index.QueryFilename([]byte(filename))
	}
	
	// Get initial stats
	statsBefore := index.GetStats()
	
	// Perform maintenance again
	err = index.Maintenance()
	if err != nil {
		t.Errorf("Maintenance failed: %v", err)
	}
	
	// Get stats after maintenance
	statsAfter := index.GetStats()
	
	// Stats should be updated
	if !statsAfter.LastUpdated.After(statsBefore.LastUpdated) {
		t.Error("Statistics should be updated after maintenance")
	}
	
	t.Logf("Before maintenance: Queries=%d, Budget=%.3f", 
		statsBefore.TotalQueries, statsBefore.DifferentialBudget)
	t.Logf("After maintenance: Queries=%d, Budget=%.3f", 
		statsAfter.TotalQueries, statsAfter.DifferentialBudget)
}

// TestPrivacyIndexMetadataAttributes tests privacy-preserving metadata indexing
func TestPrivacyIndexMetadataAttributes(t *testing.T) {
	config := DefaultPrivacyIndexConfig()
	config.AttributeObfuscation = true
	config.TemporalBlurring = true
	config.ExpectedFiles = 20
	
	index, err := NewPrivacyIndex(config)
	if err != nil {
		t.Fatalf("Failed to create privacy index: %v", err)
	}
	
	// Test files with different size ranges
	testCases := []struct {
		filename string
		size     int64
		expected string
	}{
		{"tiny_file", 500, "tiny"},
		{"small_file", 50 * 1024, "small"},
		{"medium_file", 5 * 1024 * 1024, "medium"},
		{"large_file", 50 * 1024 * 1024, "large"},
		{"huge_file", 500 * 1024 * 1024, "huge"},
	}
	
	for _, tc := range testCases {
		t.Run(tc.filename, func(t *testing.T) {
			metadata := FileMetadata{
				Size:        tc.size,
				ModTime:     time.Now(),
				ContentType: "application/test",
			}
			
			err := index.IndexFilename([]byte(tc.filename), metadata)
			if err != nil {
				t.Errorf("Failed to index file %s: %v", tc.filename, err)
			}
			
			// Verify the file can be found
			found, err := index.QueryFilename([]byte(tc.filename))
			if err != nil {
				t.Errorf("Error querying file %s: %v", tc.filename, err)
			}
			if !found {
				t.Errorf("File %s should be found but wasn't", tc.filename)
			}
		})
	}
	
	stats := index.GetStats()
	t.Logf("Metadata indexing completed: %d queries, %.4f metadata filter load", 
		stats.TotalQueries, stats.MetadataFilterLoad)
}

// BenchmarkPrivacyIndex benchmarks privacy index operations
func BenchmarkPrivacyIndex(b *testing.B) {
	config := DefaultPrivacyIndexConfig()
	config.ExpectedFiles = 10000
	
	index, err := NewPrivacyIndex(config)
	if err != nil {
		b.Fatalf("Failed to create privacy index: %v", err)
	}
	
	// Pre-populate with some data
	for i := 0; i < 1000; i++ {
		filename := fmt.Sprintf("bench_file_%d", i)
		metadata := FileMetadata{Size: 1024, ModTime: time.Now()}
		index.IndexFilename([]byte(filename), metadata)
	}
	
	b.Run("IndexFilename", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			filename := fmt.Sprintf("bench_index_%d", i)
			metadata := FileMetadata{Size: 1024, ModTime: time.Now()}
			index.IndexFilename([]byte(filename), metadata)
		}
	})
	
	b.Run("QueryFilename", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			filename := fmt.Sprintf("bench_file_%d", i%1000)
			index.QueryFilename([]byte(filename))
		}
	})
	
	b.Run("IndexContent", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			fingerprint := fmt.Sprintf("content_hash_%d", i)
			blockCID := fmt.Sprintf("block_%d", i)
			index.IndexContent([]byte(fingerprint), blockCID)
		}
	})
	
	b.Run("GetStats", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			index.GetStats()
		}
	})
}