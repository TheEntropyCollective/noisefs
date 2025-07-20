package search

import (
	"context"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/index"
)

func TestPrivacySearchEngineIntegration(t *testing.T) {
	// Create index manager for testing
	config := index.DefaultIndexManagerConfig()
	indexManager, err := index.NewIndexManager(config)
	if err != nil {
		t.Fatalf("Failed to create index manager: %v", err)
	}
	
	// Create privacy search engine
	searchConfig := DefaultPrivacySearchConfig()
	searchEngine, err := NewPrivacySearchEngine(indexManager, searchConfig)
	if err != nil {
		t.Fatalf("Failed to create privacy search engine: %v", err)
	}
	
	// Test basic search functionality
	ctx := context.Background()
	
	options := map[string]interface{}{
		"privacy_level": 3,
		"session_id":    "test_session_123",
		"max_results":   10,
	}
	
	response, err := searchEngine.Search(ctx, "test document", options)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	
	// Verify response structure
	if response == nil {
		t.Fatal("Response is nil")
	}
	
	if response.Query == nil {
		t.Fatal("Response query is nil")
	}
	
	if response.PrivacyLevel != 3 {
		t.Errorf("Expected privacy level 3, got %d", response.PrivacyLevel)
	}
	
	if response.SearchID == "" {
		t.Error("Search ID should not be empty")
	}
	
	// Test that dummy queries were generated for privacy level 3
	if response.DummyQueries == 0 {
		t.Error("Expected dummy queries for privacy level 3")
	}
	
	// Test stats collection
	stats := searchEngine.GetStats()
	if stats == nil {
		t.Fatal("Stats should not be nil")
	}
	
	if stats.TotalQueries == 0 {
		t.Error("Total queries should be > 0")
	}
	
	if stats.SuccessfulQueries == 0 {
		t.Error("Successful queries should be > 0")
	}
}

func TestSearchQueryValidation(t *testing.T) {
	validator := NewQueryValidator()
	
	// Test valid query
	query := &SearchQuery{
		Query:        "test document",
		Type:         FilenameSearch,
		MaxResults:   100,
		PrivacyLevel: 3,
		SessionID:    "test_session",
		RequestTime:  time.Now(),
	}
	
	result, err := validator.ValidateQuery(query, "127.0.0.1")
	if err != nil {
		t.Fatalf("Validation failed: %v", err)
	}
	
	if !result.Valid {
		t.Error("Query should be valid")
	}
	
	if result.Blocked {
		t.Error("Query should not be blocked")
	}
	
	// Test malicious query
	maliciousQuery := &SearchQuery{
		Query:        "SELECT * FROM users WHERE password = '1'",
		Type:         FilenameSearch,
		MaxResults:   100,
		PrivacyLevel: 1,
		SessionID:    "test_session",
		RequestTime:  time.Now(),
	}
	
	result, err = validator.ValidateQuery(maliciousQuery, "127.0.0.1")
	if err != nil {
		t.Fatalf("Validation failed: %v", err)
	}
	
	if result.Valid {
		t.Error("Malicious query should not be valid")
	}
	
	if len(result.SecurityIssues) == 0 {
		t.Error("Security issues should be detected")
	}
}

func TestPrivacyTransformation(t *testing.T) {
	transformer := NewPrivacyQueryTransformer()
	
	query := &SearchQuery{
		Query:        "confidential document",
		Type:         FilenameSearch,
		MaxResults:   100,
		PrivacyLevel: 4,
		SessionID:    "test_session",
		RequestTime:  time.Now(),
	}
	
	result, err := transformer.Transform(query)
	if err != nil {
		t.Fatalf("Transformation failed: %v", err)
	}
	
	if result == nil {
		t.Fatal("Transform result should not be nil")
	}
	
	if result.TransformedQuery == nil {
		t.Fatal("Transformed query should not be nil")
	}
	
	// For privacy level 4, should have dummy queries
	if len(result.DummyQueries) == 0 {
		t.Error("Expected dummy queries for privacy level 4")
	}
	
	// Should have timing delay for privacy level 4
	if result.TimingDelay == 0 {
		t.Error("Expected timing delay for privacy level 4")
	}
	
	// Should have k-anonymity group
	if len(result.KAnonymityGroup) == 0 {
		t.Error("Expected k-anonymity group")
	}
	
	// Should have non-zero privacy cost
	if result.PrivacyCost == 0 {
		t.Error("Expected non-zero privacy cost")
	}
}

func TestSearchExecutorBasic(t *testing.T) {
	// Create basic index manager
	config := index.DefaultIndexManagerConfig()
	indexManager, err := index.NewIndexManager(config)
	if err != nil {
		t.Fatalf("Failed to create index manager: %v", err)
	}
	
	// Create search executor
	searchConfig := DefaultPrivacySearchConfig()
	executor := NewSearchExecutor(indexManager, searchConfig)
	
	query := &SearchQuery{
		Query:           "test",
		ObfuscatedQuery: "test",
		Type:            FilenameSearch,
		MaxResults:      10,
		PrivacyLevel:    2,
		SessionID:       "test_session",
		RequestTime:     time.Now(),
	}
	
	ctx := context.Background()
	results, err := executor.ExecuteSearch(ctx, query)
	if err != nil {
		t.Fatalf("Executor search failed: %v", err)
	}
	
	// Results should be non-nil (even if empty)
	if results == nil {
		t.Fatal("Results should not be nil")
	}
	
	// Should be able to handle empty results
	if len(results) < 0 {
		t.Error("Results length should be >= 0")
	}
}

func TestSearchCoordinatorBasic(t *testing.T) {
	config := DefaultPrivacySearchConfig()
	coordinator := NewSearchCoordinator(config)
	
	if coordinator == nil {
		t.Fatal("Coordinator should not be nil")
	}
	
	// Test session manager
	sessionCount := coordinator.sessionManager.GetActiveSessionCount()
	if sessionCount < 0 {
		t.Error("Session count should be >= 0")
	}
	
	// Test session update
	query := &SearchQuery{
		Query:        "test",
		SessionID:    "test_session_coord",
		PrivacyLevel: 3,
		RequestTime:  time.Now(),
	}
	
	err := coordinator.sessionManager.UpdateSession("test_session_coord", query)
	if err != nil {
		t.Fatalf("Session update failed: %v", err)
	}
	
	// Session count should increase
	newSessionCount := coordinator.sessionManager.GetActiveSessionCount()
	if newSessionCount != sessionCount+1 {
		t.Errorf("Expected session count %d, got %d", sessionCount+1, newSessionCount)
	}
}

func BenchmarkPrivacySearchEngine(b *testing.B) {
	// Setup
	config := index.DefaultIndexManagerConfig()
	indexManager, err := index.NewIndexManager(config)
	if err != nil {
		b.Fatalf("Failed to create index manager: %v", err)
	}
	
	searchConfig := DefaultPrivacySearchConfig()
	searchEngine, err := NewPrivacySearchEngine(indexManager, searchConfig)
	if err != nil {
		b.Fatalf("Failed to create privacy search engine: %v", err)
	}
	
	ctx := context.Background()
	options := map[string]interface{}{
		"privacy_level": 3,
		"session_id":    "bench_session",
		"max_results":   10,
	}
	
	// Benchmark
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := searchEngine.Search(ctx, "benchmark test", options)
		if err != nil {
			b.Fatalf("Search failed: %v", err)
		}
	}
}