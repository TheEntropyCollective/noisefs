package search

import (
	"testing"
	"time"
)

func TestSearchResultCache(t *testing.T) {
	cache := NewSearchResultCache(3, 100*time.Millisecond)

	// Test basic put/get
	results1 := &SearchResults{
		Query: "test query",
		Total: 5,
	}
	
	key1 := "key1"
	cache.Put(key1, results1)
	
	retrieved, found := cache.Get(key1)
	if !found {
		t.Error("Expected to find cached result")
	}
	if retrieved.Query != "test query" {
		t.Errorf("Expected query 'test query', got '%s'", retrieved.Query)
	}

	// Test TTL expiration
	time.Sleep(150 * time.Millisecond)
	_, found = cache.Get(key1)
	if found {
		t.Error("Expected result to be expired")
	}

	// Test LRU eviction
	cache.Clear()
	cache.Put("key1", &SearchResults{Query: "query1"})
	cache.Put("key2", &SearchResults{Query: "query2"})
	cache.Put("key3", &SearchResults{Query: "query3"})
	
	// This should evict key1
	cache.Put("key4", &SearchResults{Query: "query4"})
	
	_, found = cache.Get("key1")
	if found {
		t.Error("Expected key1 to be evicted")
	}
	
	_, found = cache.Get("key4")
	if !found {
		t.Error("Expected key4 to be present")
	}

	// Test cache stats
	stats := cache.GetStats()
	if stats.Size != 3 {
		t.Errorf("Expected cache size 3, got %d", stats.Size)
	}
	if stats.MaxSize != 3 {
		t.Errorf("Expected max size 3, got %d", stats.MaxSize)
	}
}

func TestCacheKeyGeneration(t *testing.T) {
	options1 := SearchOptions{
		MaxResults: 20,
		SortBy:     SortByScore,
	}
	
	options2 := SearchOptions{
		MaxResults: 20,
		SortBy:     SortByScore,
	}
	
	options3 := SearchOptions{
		MaxResults: 30,
		SortBy:     SortByScore,
	}

	key1 := GenerateCacheKey("test query", options1)
	key2 := GenerateCacheKey("test query", options2)
	key3 := GenerateCacheKey("test query", options3)
	
	if key1 != key2 {
		t.Error("Expected identical options to generate same cache key")
	}
	
	if key1 == key3 {
		t.Error("Expected different options to generate different cache keys")
	}

	// Test metadata cache key
	filters1 := MetadataFilters{
		NamePattern: "*.txt",
		Directory:   "/test",
	}
	
	filters2 := MetadataFilters{
		NamePattern: "*.txt",
		Directory:   "/test",
	}
	
	metaKey1 := GenerateMetadataCacheKey(filters1)
	metaKey2 := GenerateMetadataCacheKey(filters2)
	
	if metaKey1 != metaKey2 {
		t.Error("Expected identical filters to generate same cache key")
	}
}

func TestCacheCleanExpired(t *testing.T) {
	cache := NewSearchResultCache(10, 50*time.Millisecond)
	
	// Add some entries
	cache.Put("key1", &SearchResults{Query: "query1"})
	cache.Put("key2", &SearchResults{Query: "query2"})
	cache.Put("key3", &SearchResults{Query: "query3"})
	
	if cache.Size() != 3 {
		t.Errorf("Expected size 3, got %d", cache.Size())
	}
	
	// Wait for expiration
	time.Sleep(60 * time.Millisecond)
	
	// Clean expired entries
	removed := cache.CleanExpired()
	if removed != 3 {
		t.Errorf("Expected to remove 3 entries, removed %d", removed)
	}
	
	if cache.Size() != 0 {
		t.Errorf("Expected size 0 after cleanup, got %d", cache.Size())
	}
}