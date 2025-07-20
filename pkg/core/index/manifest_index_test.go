package index

import (
	"fmt"
	"testing"
)

// TestManifestIndexBasicOperations tests core manifest index functionality
func TestManifestIndexBasicOperations(t *testing.T) {
	config := DefaultManifestIndexConfig()
	config.ExpectedDirectories = 100
	config.CacheSize = 50
	
	index, err := NewManifestIndex(config)
	if err != nil {
		t.Fatalf("Failed to create manifest index: %v", err)
	}
	
	// Test directory indexing
	testDirectories := []struct {
		path         string
		manifestData []byte
	}{
		{"/home/user/documents", []byte("manifest_data_1")},
		{"/home/user/downloads", []byte("manifest_data_2")},
		{"/home/user/pictures/vacation", []byte("manifest_data_3")},
		{"/var/log/system", []byte("manifest_data_4")},
	}
	
	// Index directories
	for _, dir := range testDirectories {
		err := index.IndexDirectory(dir.path, dir.manifestData)
		if err != nil {
			t.Errorf("Failed to index directory %s: %v", dir.path, err)
		}
	}
	
	// Test directory lookups
	for _, dir := range testDirectories {
		manifestData, found, err := index.LookupDirectory(dir.path)
		if err != nil {
			t.Errorf("Error looking up directory %s: %v", dir.path, err)
			continue
		}
		
		if !found {
			t.Errorf("Directory %s should be found but wasn't", dir.path)
			continue
		}
		
		if string(manifestData) != string(dir.manifestData) {
			t.Errorf("Manifest data mismatch for %s: expected %s, got %s", 
				dir.path, string(dir.manifestData), string(manifestData))
		}
	}
	
	// Test non-existent directory lookup
	_, _, err = index.LookupDirectory("/nonexistent/path")
	if err != nil {
		t.Errorf("Error looking up non-existent directory: %v", err)
	}
	// Note: found might be true due to false positives, which is expected
}

// TestManifestIndexHierarchy tests hierarchical directory navigation
func TestManifestIndexHierarchy(t *testing.T) {
	config := DefaultManifestIndexConfig()
	config.UseHierarchyIndex = true
	config.MaxPathDepth = 5
	
	index, err := NewManifestIndex(config)
	if err != nil {
		t.Fatalf("Failed to create manifest index: %v", err)
	}
	
	// Test hierarchical paths
	hierarchyPaths := []string{
		"/",
		"/home",
		"/home/user",
		"/home/user/documents",
		"/home/user/documents/projects",
		"/home/user/documents/projects/secret",
	}
	
	// Index hierarchy
	for i, path := range hierarchyPaths {
		manifestData := []byte(fmt.Sprintf("manifest_%d", i))
		err := index.IndexDirectory(path, manifestData)
		if err != nil {
			t.Errorf("Failed to index hierarchy path %s: %v", path, err)
		}
	}
	
	// Verify hierarchy navigation
	for i, path := range hierarchyPaths {
		manifestData, found, err := index.LookupDirectory(path)
		if err != nil {
			t.Errorf("Error looking up hierarchy path %s: %v", path, err)
			continue
		}
		
		if !found {
			t.Errorf("Hierarchy path %s should be found but wasn't", path)
			continue
		}
		
		expectedData := fmt.Sprintf("manifest_%d", i)
		if string(manifestData) != expectedData {
			t.Errorf("Hierarchy manifest mismatch for %s: expected %s, got %s",
				path, expectedData, string(manifestData))
		}
	}
	
	// Verify hierarchy structure
	stats := index.GetStats()
	if stats.IndexedDirectories != uint64(len(hierarchyPaths)) {
		t.Errorf("Expected %d indexed directories, got %d", 
			len(hierarchyPaths), stats.IndexedDirectories)
	}
	
	if stats.PathSegments == 0 {
		t.Error("Path segments should be greater than 0")
	}
}

// TestManifestIndexCache tests manifest caching functionality
func TestManifestIndexCache(t *testing.T) {
	config := DefaultManifestIndexConfig()
	config.CacheSize = 3 // Small cache for testing LRU
	
	index, err := NewManifestIndex(config)
	if err != nil {
		t.Fatalf("Failed to create manifest index: %v", err)
	}
	
	// Index more directories than cache size
	testPaths := []string{
		"/cache/test/1",
		"/cache/test/2", 
		"/cache/test/3",
		"/cache/test/4", // This should evict the first entry
		"/cache/test/5", // This should evict the second entry
	}
	
	for i, path := range testPaths {
		manifestData := []byte(fmt.Sprintf("cached_manifest_%d", i))
		err := index.IndexDirectory(path, manifestData)
		if err != nil {
			t.Errorf("Failed to index path %s: %v", path, err)
		}
	}
	
	// First lookup should be from hierarchy (not cache)
	initialStats := index.GetStats()
	initialCacheHits := initialStats.CacheHits
	
	// Lookup a recent path (should be cached)
	_, found, err := index.LookupDirectory("/cache/test/5")
	if err != nil {
		t.Errorf("Error looking up cached path: %v", err)
	}
	if !found {
		t.Error("Recently indexed path should be found")
	}
	
	// Check cache hit statistics
	finalStats := index.GetStats()
	if finalStats.CacheHits <= initialCacheHits {
		t.Errorf("Cache hits should have increased: initial=%d, final=%d",
			initialCacheHits, finalStats.CacheHits)
	}
	
	t.Logf("Cache performance: %d hits out of %d lookups", 
		finalStats.CacheHits, finalStats.TotalLookups)
}

// TestManifestIndexPathEncryption tests path encryption and obfuscation
func TestManifestIndexPathEncryption(t *testing.T) {
	config := DefaultManifestIndexConfig()
	config.EnablePathBlinding = true
	config.PrivacyLevel = 4
	
	index, err := NewManifestIndex(config)
	if err != nil {
		t.Fatalf("Failed to create manifest index: %v", err)
	}
	
	// Test that same paths produce consistent encrypted results
	testPath := "/sensitive/encrypted/path"
	manifestData := []byte("encrypted_manifest_data")
	
	// Index the path
	err = index.IndexDirectory(testPath, manifestData)
	if err != nil {
		t.Errorf("Failed to index encrypted path: %v", err)
	}
	
	// Lookup should work with same path
	retrievedData, found, err := index.LookupDirectory(testPath)
	if err != nil {
		t.Errorf("Error looking up encrypted path: %v", err)
	}
	
	if !found {
		t.Error("Encrypted path should be found")
	}
	
	if string(retrievedData) != string(manifestData) {
		t.Errorf("Encrypted manifest mismatch: expected %s, got %s",
			string(manifestData), string(retrievedData))
	}
	
	// Test path obfuscation with similar paths
	similarPaths := []string{
		"/sensitive/encrypted/path",
		"/sensitive/encrypted/other",
		"/sensitive/different/path",
	}
	
	for i, path := range similarPaths {
		data := []byte(fmt.Sprintf("data_%d", i))
		err := index.IndexDirectory(path, data)
		if err != nil {
			t.Errorf("Failed to index similar path %s: %v", path, err)
		}
	}
	
	// All paths should be independently retrievable
	for i, path := range similarPaths {
		data, found, err := index.LookupDirectory(path)
		if err != nil {
			t.Errorf("Error looking up similar path %s: %v", path, err)
			continue
		}
		
		if !found {
			t.Errorf("Similar path %s should be found", path)
			continue
		}
		
		expectedData := fmt.Sprintf("data_%d", i)
		if string(data) != expectedData {
			t.Errorf("Similar path data mismatch for %s: expected %s, got %s",
				path, expectedData, string(data))
		}
	}
}

// TestManifestIndexStatistics tests statistics collection and reporting
func TestManifestIndexStatistics(t *testing.T) {
	config := DefaultManifestIndexConfig()
	config.ExpectedDirectories = 50
	
	index, err := NewManifestIndex(config)
	if err != nil {
		t.Fatalf("Failed to create manifest index: %v", err)
	}
	
	// Perform operations to generate statistics
	numOperations := 20
	for i := 0; i < numOperations; i++ {
		path := fmt.Sprintf("/stats/test/dir_%d", i)
		manifestData := []byte(fmt.Sprintf("stats_manifest_%d", i))
		
		err := index.IndexDirectory(path, manifestData)
		if err != nil {
			t.Errorf("Failed to index path for stats: %v", err)
			continue
		}
		
		// Perform lookups
		_, _, err = index.LookupDirectory(path)
		if err != nil {
			t.Errorf("Failed to lookup path for stats: %v", err)
		}
	}
	
	// Get and validate statistics
	stats := index.GetStats()
	
	if stats.IndexedDirectories != uint64(numOperations) {
		t.Errorf("Expected %d indexed directories, got %d", 
			numOperations, stats.IndexedDirectories)
	}
	
	if stats.TotalLookups < uint64(numOperations) {
		t.Errorf("Expected at least %d lookups, got %d", 
			numOperations, stats.TotalLookups)
	}
	
	if stats.FilterMemoryUsage == 0 {
		t.Error("Filter memory usage should be greater than 0")
	}
	
	if stats.PathSegments == 0 {
		t.Error("Path segments should be greater than 0")
	}
	
	if stats.LastUpdated.IsZero() {
		t.Error("Last updated time should be set")
	}
	
	t.Logf("Manifest Index Statistics:")
	t.Logf("  Indexed Directories: %d", stats.IndexedDirectories)
	t.Logf("  Total Lookups: %d", stats.TotalLookups)
	t.Logf("  Successful Lookups: %d", stats.SuccessfulLookups)
	t.Logf("  Cache Hits: %d", stats.CacheHits)
	t.Logf("  Path Segments: %d", stats.PathSegments)
	t.Logf("  Filter Memory: %d bytes", stats.FilterMemoryUsage)
	t.Logf("  Hierarchy Memory: %d bytes", stats.HierarchyMemoryUsage)
	t.Logf("  Cache Memory: %d bytes", stats.CacheMemoryUsage)
	t.Logf("  Average Lookup Time: %v", stats.AverageLookupTime)
}

// TestManifestIndexErrorHandling tests error conditions and validation
func TestManifestIndexErrorHandling(t *testing.T) {
	// Test invalid configuration
	invalidConfigs := []*ManifestIndexConfig{
		{PathEncryptionKey: []byte("short")}, // Too short key
		{PathEncryptionKey: make([]byte, 32), MaxPathDepth: 0}, // Invalid depth
		{PathEncryptionKey: make([]byte, 32), MaxPathDepth: 25}, // Depth too high
		{PathEncryptionKey: make([]byte, 32), FalsePositiveRate: 0}, // Invalid FPR
		{PathEncryptionKey: make([]byte, 32), FalsePositiveRate: 1}, // Invalid FPR
	}
	
	for i, config := range invalidConfigs {
		t.Run(fmt.Sprintf("InvalidConfig_%d", i), func(t *testing.T) {
			_, err := NewManifestIndex(config)
			if err == nil {
				t.Error("Expected error for invalid configuration")
			}
		})
	}
	
	// Test valid index with invalid operations
	config := DefaultManifestIndexConfig()
	index, err := NewManifestIndex(config)
	if err != nil {
		t.Fatalf("Failed to create manifest index: %v", err)
	}
	
	// Test empty directory path
	err = index.IndexDirectory("", []byte("data"))
	if err == nil {
		t.Error("Expected error when indexing empty directory path")
	}
	
	_, _, err = index.LookupDirectory("")
	if err == nil {
		t.Error("Expected error when looking up empty directory path")
	}
	
	// Test nil manifest data (should be allowed)
	err = index.IndexDirectory("/test/nil", nil)
	if err != nil {
		t.Errorf("Indexing with nil manifest data should be allowed: %v", err)
	}
}

// TestManifestIndexConfiguration tests different configuration options
func TestManifestIndexConfiguration(t *testing.T) {
	testConfigs := []struct {
		name         string
		privacyLevel int
		useHierarchy bool
		cacheSize    int
	}{
		{"Low Privacy", 1, false, 100},
		{"Medium Privacy", 3, true, 500},
		{"High Privacy", 5, true, 1000},
	}
	
	for _, tc := range testConfigs {
		t.Run(tc.name, func(t *testing.T) {
			config := DefaultManifestIndexConfig()
			config.PrivacyLevel = tc.privacyLevel
			config.UseHierarchyIndex = tc.useHierarchy
			config.CacheSize = tc.cacheSize
			
			index, err := NewManifestIndex(config)
			if err != nil {
				t.Fatalf("Failed to create manifest index: %v", err)
			}
			
			// Perform operations
			for i := 0; i < 10; i++ {
				path := fmt.Sprintf("/config/test/%s/%d", tc.name, i)
				manifestData := []byte(fmt.Sprintf("config_manifest_%d", i))
				
				err := index.IndexDirectory(path, manifestData)
				if err != nil {
					t.Errorf("Failed to index directory: %v", err)
				}
			}
			
			stats := index.GetStats()
			t.Logf("%s - Indexed: %d, Memory: %d bytes", 
				tc.name, stats.IndexedDirectories, 
				stats.FilterMemoryUsage+stats.HierarchyMemoryUsage+stats.CacheMemoryUsage)
		})
	}
}

// BenchmarkManifestIndex benchmarks manifest index operations
func BenchmarkManifestIndex(b *testing.B) {
	config := DefaultManifestIndexConfig()
	config.ExpectedDirectories = 10000
	
	index, err := NewManifestIndex(config)
	if err != nil {
		b.Fatalf("Failed to create manifest index: %v", err)
	}
	
	// Pre-populate with some data
	for i := 0; i < 1000; i++ {
		path := fmt.Sprintf("/bench/dir_%d", i)
		manifestData := []byte(fmt.Sprintf("bench_manifest_%d", i))
		index.IndexDirectory(path, manifestData)
	}
	
	b.Run("IndexDirectory", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			path := fmt.Sprintf("/bench/index_%d", i)
			manifestData := []byte(fmt.Sprintf("index_manifest_%d", i))
			index.IndexDirectory(path, manifestData)
		}
	})
	
	b.Run("LookupDirectory", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			path := fmt.Sprintf("/bench/dir_%d", i%1000)
			index.LookupDirectory(path)
		}
	})
	
	b.Run("GetStats", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			index.GetStats()
		}
	})
}

// BenchmarkManifestIndexScaling benchmarks performance at different scales
func BenchmarkManifestIndexScaling(b *testing.B) {
	scales := []uint64{1000, 10000, 100000}
	
	for _, scale := range scales {
		b.Run(fmt.Sprintf("Scale_%d", scale), func(b *testing.B) {
			config := DefaultManifestIndexConfig()
			config.ExpectedDirectories = scale
			
			index, err := NewManifestIndex(config)
			if err != nil {
				b.Fatalf("Failed to create manifest index: %v", err)
			}
			
			b.ResetTimer()
			
			for i := 0; i < b.N; i++ {
				path := fmt.Sprintf("/scale/test_%d", i)
				manifestData := []byte(fmt.Sprintf("scale_manifest_%d", i))
				index.IndexDirectory(path, manifestData)
			}
		})
	}
}

// TestManifestIndexPathUtilities tests path utility functions
func TestManifestIndexPathUtilities(t *testing.T) {
	testCases := []struct {
		input    string
		expected []string
	}{
		{"", []string{"/"}},
		{"/", []string{"/"}},
		{"/home", []string{"home"}},
		{"/home/user", []string{"home", "user"}},
		{"/home/user/documents", []string{"home", "user", "documents"}},
		{"home/user", []string{"home", "user"}}, // No leading slash
	}
	
	for _, tc := range testCases {
		t.Run(fmt.Sprintf("SplitPath_%s", tc.input), func(t *testing.T) {
			result := splitPath(tc.input)
			
			if len(result) != len(tc.expected) {
				t.Errorf("Length mismatch for %s: expected %d, got %d",
					tc.input, len(tc.expected), len(result))
				return
			}
			
			for i, component := range result {
				if component != tc.expected[i] {
					t.Errorf("Component mismatch for %s at index %d: expected %s, got %s",
						tc.input, i, tc.expected[i], component)
				}
			}
		})
	}
}

// TestManifestIndexPathEncryptionConsistency tests encryption consistency
func TestManifestIndexPathEncryptionConsistency(t *testing.T) {
	config := DefaultManifestIndexConfig()
	
	obfuscator := &PathObfuscator{
		encryptionKey: config.PathEncryptionKey,
		blindingKeys:  generateBlindingKeys(config.PrivacyLevel),
		noisePaths:    generateNoisePaths(100),
	}
	
	testPath := "/consistency/test/path"
	
	// Encrypt the same path multiple times
	encrypted1, segments1, err1 := obfuscator.EncryptPath(testPath)
	if err1 != nil {
		t.Fatalf("First encryption failed: %v", err1)
	}
	
	encrypted2, segments2, err2 := obfuscator.EncryptPath(testPath)
	if err2 != nil {
		t.Fatalf("Second encryption failed: %v", err2)
	}
	
	// Results should be identical
	if string(encrypted1) != string(encrypted2) {
		t.Error("Encrypted paths should be identical for same input")
	}
	
	if len(segments1) != len(segments2) {
		t.Errorf("Segment count mismatch: %d vs %d", len(segments1), len(segments2))
	}
	
	for i := range segments1 {
		if string(segments1[i]) != string(segments2[i]) {
			t.Errorf("Segment %d mismatch", i)
		}
	}
	
	t.Logf("Path encryption consistency verified for: %s", testPath)
	t.Logf("  Encrypted length: %d bytes", len(encrypted1))
	t.Logf("  Segment count: %d", len(segments1))
}