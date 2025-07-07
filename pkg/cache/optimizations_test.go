package cache

import (
	"fmt"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/logging"
)

func TestReadAheadCache(t *testing.T) {
	// Create underlying cache
	underlying := NewMemoryCache(100)
	logger := logging.NewLogger(logging.DefaultConfig())
	
	// Create read-ahead cache
	config := ReadAheadConfig{
		ReadAheadSize:  4,
		WorkerCount:    2,
		MaxPatterns:    100,
		PatternTimeout: time.Minute,
	}
	cache := NewReadAheadCache(underlying, config, logger)
	defer cache.Close()
	
	// Test basic operations
	block := &blocks.Block{}
	err := cache.Store("test1", block)
	if err != nil {
		t.Fatalf("Failed to store block: %v", err)
	}
	
	// Test retrieval
	retrieved, err := cache.Get("test1")
	if err != nil {
		t.Fatalf("Failed to get block: %v", err)
	}
	if retrieved != block {
		t.Error("Retrieved block doesn't match stored block")
	}
	
	// Test statistics
	stats := cache.GetStats()
	if stats.ReadAheadHits != 1 {
		t.Errorf("Expected 1 hit, got %d", stats.ReadAheadHits)
	}
}

func TestWriteBackCache(t *testing.T) {
	// Create underlying cache
	underlying := NewMemoryCache(100)
	logger := logging.NewLogger(logging.DefaultConfig())
	
	// Create write-back cache
	config := WriteBackConfig{
		BufferSize:    10,
		FlushInterval: 100 * time.Millisecond,
		FlushWorkers:  1,
	}
	cache := NewWriteBackCache(underlying, config, logger)
	defer cache.Close()
	
	// Test buffered write
	block := &blocks.Block{}
	err := cache.Store("test1", block)
	if err != nil {
		t.Fatalf("Failed to store block: %v", err)
	}
	
	// Test immediate retrieval from buffer
	retrieved, err := cache.Get("test1")
	if err != nil {
		t.Fatalf("Failed to get block: %v", err)
	}
	if retrieved != block {
		t.Error("Retrieved block doesn't match stored block")
	}
	
	// Wait for flush
	time.Sleep(200 * time.Millisecond)
	
	// Test statistics
	stats := cache.GetStats()
	if stats.BufferedWrites != 1 {
		t.Errorf("Expected 1 buffered write, got %d", stats.BufferedWrites)
	}
}

func TestLRUEvictionPolicy(t *testing.T) {
	policy := NewLRUEvictionPolicy()
	block := &blocks.Block{}
	
	// Add some blocks
	policy.OnStore("block1", block)
	policy.OnStore("block2", block)
	policy.OnStore("block3", block)
	
	// Access block1 to make it most recent
	policy.OnAccess("block1")
	
	// Select victim (should be block2, oldest)
	victim, ok := policy.SelectVictim()
	if !ok {
		t.Error("Expected victim to be found")
	}
	if victim != "block2" {
		t.Errorf("Expected victim to be block2, got %s", victim)
	}
}

func TestLFUEvictionPolicy(t *testing.T) {
	policy := NewLFUEvictionPolicy()
	block := &blocks.Block{}
	
	// Add some blocks
	policy.OnStore("block1", block)
	policy.OnStore("block2", block)
	policy.OnStore("block3", block)
	
	// Access block1 multiple times
	policy.OnAccess("block1")
	policy.OnAccess("block1")
	policy.OnAccess("block2")
	
	// Select victim (should be block3, least frequent)
	victim, ok := policy.SelectVictim()
	if !ok {
		t.Error("Expected victim to be found")
	}
	if victim != "block3" {
		t.Errorf("Expected victim to be block3, got %s", victim)
	}
}

func TestTTLEvictionPolicy(t *testing.T) {
	policy := NewTTLEvictionPolicy(50 * time.Millisecond)
	block := &blocks.Block{}
	
	// Add a block
	policy.OnStore("block1", block)
	
	// Wait for expiration
	time.Sleep(100 * time.Millisecond)
	
	// Select victim (should be block1, expired)
	victim, ok := policy.SelectVictim()
	if !ok {
		t.Error("Expected victim to be found")
	}
	if victim != "block1" {
		t.Errorf("Expected victim to be block1, got %s", victim)
	}
}

func TestEvictingCache(t *testing.T) {
	// Create underlying cache
	underlying := NewMemoryCache(3) // Small capacity
	policy := NewLRUEvictionPolicy()
	logger := logging.NewLogger(logging.DefaultConfig())
	
	// Create evicting cache
	cache := NewEvictingCache(underlying, policy, 3, logger)
	
	block := &blocks.Block{}
	
	// Fill cache to capacity
	cache.Store("block1", block)
	cache.Store("block2", block)
	cache.Store("block3", block)
	
	if cache.Size() != 3 {
		t.Errorf("Expected cache size 3, got %d", cache.Size())
	}
	
	// Add one more block (should trigger eviction)
	cache.Store("block4", block)
	
	// Cache should still be at capacity
	if cache.Size() != 3 {
		t.Errorf("Expected cache size 3 after eviction, got %d", cache.Size())
	}
	
	// block1 should have been evicted (LRU)
	if cache.Has("block1") {
		t.Error("Expected block1 to be evicted")
	}
}

func TestStatisticsCache(t *testing.T) {
	// Create underlying cache
	underlying := NewMemoryCache(100)
	logger := logging.NewLogger(logging.DefaultConfig())
	
	// Create statistics cache
	cache := NewStatisticsCache(underlying, logger)
	
	block := &blocks.Block{}
	
	// Test store operation
	err := cache.Store("test1", block)
	if err != nil {
		t.Fatalf("Failed to store block: %v", err)
	}
	
	// Test hit
	_, err = cache.Get("test1")
	if err != nil {
		t.Fatalf("Failed to get block: %v", err)
	}
	
	// Test miss
	_, err = cache.Get("nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent block")
	}
	
	// Check statistics
	stats := cache.GetStats().GetSnapshot()
	if stats.Stores != 1 {
		t.Errorf("Expected 1 store, got %d", stats.Stores)
	}
	if stats.Hits != 1 {
		t.Errorf("Expected 1 hit, got %d", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("Expected 1 miss, got %d", stats.Misses)
	}
	if stats.TotalRequests != 2 {
		t.Errorf("Expected 2 total requests, got %d", stats.TotalRequests)
	}
	if stats.HitRate != 0.5 {
		t.Errorf("Expected hit rate 0.5, got %f", stats.HitRate)
	}
}

func TestCacheStatsJSON(t *testing.T) {
	stats := NewCacheStats()
	
	// Record some operations
	stats.RecordHit("test1", time.Millisecond)
	stats.RecordMiss("test2", 2*time.Millisecond)
	
	// Test JSON serialization
	jsonData, err := stats.ToJSON()
	if err != nil {
		t.Fatalf("Failed to serialize stats to JSON: %v", err)
	}
	
	if len(jsonData) == 0 {
		t.Error("Expected non-empty JSON data")
	}
}

func TestAdaptiveEvictionPolicy(t *testing.T) {
	logger := logging.NewLogger(logging.DefaultConfig())
	policy := NewAdaptiveEvictionPolicy(logger)
	
	block := &blocks.Block{}
	
	// Add some blocks
	policy.OnStore("block1", block)
	policy.OnStore("block2", block)
	policy.OnStore("block3", block)
	
	// Access patterns
	policy.OnAccess("block1")
	policy.OnAccess("block1")
	policy.OnAccess("block2")
	
	// Select victim
	victim, ok := policy.SelectVictim()
	if !ok {
		t.Error("Expected victim to be found")
	}
	
	// Should return some victim (exact choice depends on policy weights)
	if victim == "" {
		t.Error("Expected non-empty victim CID")
	}
}

func BenchmarkReadAheadCache(b *testing.B) {
	underlying := NewMemoryCache(1000)
	logger := logging.NewLogger(logging.DefaultConfig())
	config := ReadAheadConfig{
		ReadAheadSize:  4,
		WorkerCount:    2,
		MaxPatterns:    100,
		PatternTimeout: time.Minute,
	}
	cache := NewReadAheadCache(underlying, config, logger)
	defer cache.Close()
	
	block := &blocks.Block{}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cid := fmt.Sprintf("block%d", i%100)
		cache.Store(cid, block)
		cache.Get(cid)
	}
}

func BenchmarkWriteBackCache(b *testing.B) {
	underlying := NewMemoryCache(1000)
	logger := logging.NewLogger(logging.DefaultConfig())
	config := WriteBackConfig{
		BufferSize:    100,
		FlushInterval: time.Second,
		FlushWorkers:  2,
	}
	cache := NewWriteBackCache(underlying, config, logger)
	defer cache.Close()
	
	block := &blocks.Block{}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cid := fmt.Sprintf("block%d", i)
		cache.Store(cid, block)
	}
}

func BenchmarkEvictingCache(b *testing.B) {
	underlying := NewMemoryCache(1000)
	policy := NewLRUEvictionPolicy()
	logger := logging.NewLogger(logging.DefaultConfig())
	cache := NewEvictingCache(underlying, policy, 500, logger)
	
	block := &blocks.Block{}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cid := fmt.Sprintf("block%d", i)
		cache.Store(cid, block)
		if i%10 == 0 {
			cache.Get(cid)
		}
	}
}