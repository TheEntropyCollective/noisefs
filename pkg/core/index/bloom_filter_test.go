package index

import (
	"fmt"
	"math/rand"
	"testing"
	"time"
)

// TestBloomFilterBasicOperations tests basic Bloom filter functionality
func TestBloomFilterBasicOperations(t *testing.T) {
	config := &BloomFilterConfig{
		ExpectedElements:  1000,
		FalsePositiveRate: 0.01,
		PrivacyLevel:      3,
		UseCompression:    false,
	}
	
	bf, err := NewBloomFilter(config)
	if err != nil {
		t.Fatalf("Failed to create Bloom filter: %v", err)
	}
	
	// Test adding elements
	testElements := []string{"file1.txt", "file2.txt", "file3.txt"}
	for _, elem := range testElements {
		err := bf.Add([]byte(elem))
		if err != nil {
			t.Errorf("Failed to add element %s: %v", elem, err)
		}
	}
	
	// Test checking existing elements
	for _, elem := range testElements {
		contains, err := bf.Contains([]byte(elem))
		if err != nil {
			t.Errorf("Error checking element %s: %v", elem, err)
		}
		if !contains {
			t.Errorf("Element %s should be present but was not found", elem)
		}
	}
	
	// Test checking non-existing elements (should be false with high probability)
	nonExistentElements := []string{"nonexistent1.txt", "nonexistent2.txt"}
	falsePositives := 0
	for _, elem := range nonExistentElements {
		contains, err := bf.Contains([]byte(elem))
		if err != nil {
			t.Errorf("Error checking element %s: %v", elem, err)
		}
		if contains {
			falsePositives++
		}
	}
	
	// Should have few false positives for such a small test
	if falsePositives > len(nonExistentElements)/2 {
		t.Logf("High false positive rate: %d/%d", falsePositives, len(nonExistentElements))
	}
}

// TestBloomFilterPrivacyLevels tests different privacy levels
func TestBloomFilterPrivacyLevels(t *testing.T) {
	testCases := []struct {
		privacyLevel int
		name         string
	}{
		{1, "Low Privacy"},
		{3, "Medium Privacy"},
		{5, "High Privacy"},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := &BloomFilterConfig{
				ExpectedElements:  1000,
				FalsePositiveRate: 0.01,
				PrivacyLevel:      tc.privacyLevel,
			}
			
			bf, err := NewBloomFilter(config)
			if err != nil {
				t.Fatalf("Failed to create Bloom filter: %v", err)
			}
			
			// Privacy level should affect hash count
			stats := bf.GetStats()
			expectedMinHashes := 7 + (tc.privacyLevel - 1) // Base + privacy adjustment
			if stats.HashCount < expectedMinHashes {
				t.Errorf("Expected at least %d hash functions for privacy level %d, got %d", 
					expectedMinHashes, tc.privacyLevel, stats.HashCount)
			}
			
			t.Logf("Privacy level %d uses %d hash functions", tc.privacyLevel, stats.HashCount)
		})
	}
}

// TestBloomFilterStats tests statistics calculation
func TestBloomFilterStats(t *testing.T) {
	config := &BloomFilterConfig{
		ExpectedElements:  100,
		FalsePositiveRate: 0.01,
		PrivacyLevel:      2,
	}
	
	bf, err := NewBloomFilter(config)
	if err != nil {
		t.Fatalf("Failed to create Bloom filter: %v", err)
	}
	
	// Add some elements
	numElements := 50
	for i := 0; i < numElements; i++ {
		elem := fmt.Sprintf("element_%d", i)
		err := bf.Add([]byte(elem))
		if err != nil {
			t.Errorf("Failed to add element: %v", err)
		}
	}
	
	stats := bf.GetStats()
	
	// Verify basic statistics
	if stats.ElementCount != uint64(numElements) {
		t.Errorf("Expected element count %d, got %d", numElements, stats.ElementCount)
	}
	
	if stats.LoadFactor <= 0 || stats.LoadFactor >= 1 {
		t.Errorf("Load factor should be between 0 and 1, got %f", stats.LoadFactor)
	}
	
	if stats.MemoryUsageBytes == 0 {
		t.Error("Memory usage should be greater than 0")
	}
	
	t.Logf("Stats: Elements=%d, LoadFactor=%.4f, EstimatedFPR=%.6f, Memory=%d bytes", 
		stats.ElementCount, stats.LoadFactor, stats.EstimatedFPR, stats.MemoryUsageBytes)
}

// TestBloomFilterErrorHandling tests error conditions
func TestBloomFilterErrorHandling(t *testing.T) {
	// Test invalid configuration
	invalidConfigs := []*BloomFilterConfig{
		{ExpectedElements: 0}, // Zero elements
		{ExpectedElements: 100, FalsePositiveRate: 0}, // Invalid FPR
		{ExpectedElements: 100, FalsePositiveRate: 1}, // Invalid FPR
	}
	
	for i, config := range invalidConfigs {
		t.Run(fmt.Sprintf("InvalidConfig_%d", i), func(t *testing.T) {
			_, err := NewBloomFilter(config)
			if err == nil {
				t.Error("Expected error for invalid configuration")
			}
		})
	}
	
	// Test valid filter with invalid operations
	config := &BloomFilterConfig{
		ExpectedElements:  100,
		FalsePositiveRate: 0.01,
		PrivacyLevel:      2,
	}
	
	bf, err := NewBloomFilter(config)
	if err != nil {
		t.Fatalf("Failed to create Bloom filter: %v", err)
	}
	
	// Test empty element
	err = bf.Add([]byte{})
	if err == nil {
		t.Error("Expected error when adding empty element")
	}
	
	_, err = bf.Contains([]byte{})
	if err == nil {
		t.Error("Expected error when checking empty element")
	}
}

// TestBloomFilterConcurrency tests thread safety
func TestBloomFilterConcurrency(t *testing.T) {
	config := &BloomFilterConfig{
		ExpectedElements:  1000,
		FalsePositiveRate: 0.01,
		PrivacyLevel:      2,
	}
	
	bf, err := NewBloomFilter(config)
	if err != nil {
		t.Fatalf("Failed to create Bloom filter: %v", err)
	}
	
	// Run concurrent operations
	const numGoroutines = 10
	const opsPerGoroutine = 100
	
	done := make(chan bool, numGoroutines)
	
	// Concurrent adds
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			for j := 0; j < opsPerGoroutine; j++ {
				elem := fmt.Sprintf("concurrent_%d_%d", id, j)
				bf.Add([]byte(elem))
			}
			done <- true
		}(i)
	}
	
	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			for j := 0; j < opsPerGoroutine; j++ {
				elem := fmt.Sprintf("concurrent_%d_%d", id, j)
				bf.Contains([]byte(elem))
			}
			done <- true
		}(i)
	}
	
	// Wait for all operations to complete
	for i := 0; i < numGoroutines*2; i++ {
		<-done
	}
	
	// Verify final state
	stats := bf.GetStats()
	if stats.ElementCount == 0 {
		t.Error("No elements were added during concurrent operations")
	}
	
	t.Logf("Concurrent test completed: %d elements added", stats.ElementCount)
}

// BenchmarkBloomFilterOperations benchmarks basic operations
func BenchmarkBloomFilterOperations(b *testing.B) {
	config := &BloomFilterConfig{
		ExpectedElements:  10000,
		FalsePositiveRate: 0.01,
		PrivacyLevel:      3,
	}
	
	bf, err := NewBloomFilter(config)
	if err != nil {
		b.Fatalf("Failed to create Bloom filter: %v", err)
	}
	
	// Pre-populate with some elements
	for i := 0; i < 1000; i++ {
		elem := fmt.Sprintf("element_%d", i)
		bf.Add([]byte(elem))
	}
	
	b.Run("Add", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			elem := fmt.Sprintf("bench_add_%d", i)
			bf.Add([]byte(elem))
		}
	})
	
	b.Run("Contains", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			elem := fmt.Sprintf("element_%d", i%1000)
			bf.Contains([]byte(elem))
		}
	})
	
	b.Run("GetStats", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			bf.GetStats()
		}
	})
}

// BenchmarkBloomFilterScaling benchmarks filter performance at different scales
func BenchmarkBloomFilterScaling(b *testing.B) {
	sizes := []uint64{1000, 10000, 100000}
	
	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size_%d", size), func(b *testing.B) {
			config := &BloomFilterConfig{
				ExpectedElements:  size,
				FalsePositiveRate: 0.01,
				PrivacyLevel:      3,
			}
			
			bf, err := NewBloomFilter(config)
			if err != nil {
				b.Fatalf("Failed to create Bloom filter: %v", err)
			}
			
			b.ResetTimer()
			
			for i := 0; i < b.N; i++ {
				elem := fmt.Sprintf("scaling_test_%d", i)
				bf.Add([]byte(elem))
			}
		})
	}
}

// TestBloomFilterAccuracy tests false positive rates
func TestBloomFilterAccuracy(t *testing.T) {
	config := &BloomFilterConfig{
		ExpectedElements:  1000,
		FalsePositiveRate: 0.01, // 1% target
		PrivacyLevel:      2,
	}
	
	bf, err := NewBloomFilter(config)
	if err != nil {
		t.Fatalf("Failed to create Bloom filter: %v", err)
	}
	
	// Add known elements
	knownElements := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		elem := fmt.Sprintf("known_%d", i)
		knownElements[elem] = true
		bf.Add([]byte(elem))
	}
	
	// Test with random elements not in the set
	rand.Seed(time.Now().UnixNano())
	falsePositives := 0
	totalTests := 10000
	
	for i := 0; i < totalTests; i++ {
		elem := fmt.Sprintf("unknown_%d", rand.Intn(100000))
		if knownElements[elem] {
			continue // Skip if accidentally generated a known element
		}
		
		contains, err := bf.Contains([]byte(elem))
		if err != nil {
			t.Errorf("Error checking element: %v", err)
			continue
		}
		
		if contains {
			falsePositives++
		}
	}
	
	actualFPR := float64(falsePositives) / float64(totalTests)
	expectedFPR := config.FalsePositiveRate
	
	// Allow some tolerance (should be within 3x of expected rate)
	if actualFPR > expectedFPR*3 {
		t.Errorf("False positive rate too high: actual=%.4f, expected=%.4f", actualFPR, expectedFPR)
	}
	
	t.Logf("False positive rate: actual=%.4f (%.1f%%), expected=%.4f (%.1f%%)", 
		actualFPR, actualFPR*100, expectedFPR, expectedFPR*100)
	t.Logf("False positives: %d/%d tests", falsePositives, totalTests)
	
	// Log filter statistics
	stats := bf.GetStats()
	t.Logf("Filter stats: LoadFactor=%.4f, EstimatedFPR=%.6f", 
		stats.LoadFactor, stats.EstimatedFPR)
}