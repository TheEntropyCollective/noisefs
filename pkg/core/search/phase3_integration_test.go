package search

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/fuse"
)

// TestAdvancedResultProcessorIntegration tests the advanced result processor
func TestAdvancedResultProcessorIntegration(t *testing.T) {
	// Create advanced result processor
	config := DefaultResultPrivacyConfig()
	processor := NewAdvancedResultProcessor(config)
	
	// Create test query
	query := &SearchQuery{
		Query:           "test document",
		ObfuscatedQuery: "test document",
		Type:            FilenameSearch,
		MaxResults:      10,
		PrivacyLevel:    3,
		SessionID:       "test_session_phase3",
		RequestTime:     time.Now(),
	}
	
	// Create test results
	results := []SearchResult{
		{
			FileID:       "file1",
			Filename:     "test1.txt",
			Relevance:    0.9,
			MatchType:    "filename",
			Similarity:   0.8,
			Sources:      []string{"privacy"},
			IndexSource:  "privacy",
			PrivacyLevel: 3,
		},
		{
			FileID:       "file2",
			Filename:     "test2.txt",
			Relevance:    0.7,
			MatchType:    "filename",
			Similarity:   0.6,
			Sources:      []string{"manifest"},
			IndexSource:  "manifest",
			PrivacyLevel: 3,
		},
	}
	
	// Process results
	ctx := context.Background()
	processedResults, err := processor.ProcessResults(ctx, results, query)
	if err != nil {
		t.Fatalf("Result processing failed: %v", err)
	}
	
	// Verify processing
	if processedResults == nil {
		t.Fatal("Processed results should not be nil")
	}
	
	if len(processedResults) == 0 {
		t.Fatal("Should have processed results")
	}
	
	// Verify privacy protection was applied
	for _, result := range processedResults {
		if result.PrivacyLevel != query.PrivacyLevel {
			t.Errorf("Privacy level not set correctly: expected %d, got %d", 
				query.PrivacyLevel, result.PrivacyLevel)
		}
		
		// For privacy level 3+, should have noise applied
		if query.PrivacyLevel >= 3 && result.NoiseLevel == 0 {
			t.Error("Expected noise to be applied for privacy level 3+")
		}
	}
	
	// Test with different privacy levels
	highPrivacyQuery := &SearchQuery{
		Query:           "confidential data",
		ObfuscatedQuery: "confidential data",
		Type:            ContentSearch,
		MaxResults:      5,
		PrivacyLevel:    5,
		SessionID:       "test_session_high_privacy",
		RequestTime:     time.Now(),
	}
	
	highPrivacyResults, err := processor.ProcessResults(ctx, results, highPrivacyQuery)
	if err != nil {
		t.Fatalf("High privacy result processing failed: %v", err)
	}
	
	// Verify higher privacy protection
	for _, result := range highPrivacyResults {
		if result.NoiseLevel <= 0 {
			t.Error("Expected significant noise for privacy level 5")
		}
		
		if result.PrivacyLevel != 5 {
			t.Errorf("Privacy level not set correctly for high privacy: expected 5, got %d", 
				result.PrivacyLevel)
		}
	}
}

// TestPrivacyCacheManagerIntegration tests the privacy cache manager
func TestPrivacyCacheManagerIntegration(t *testing.T) {
	// Create cache manager
	config := DefaultCachePrivacyConfig()
	cacheManager := NewPrivacyCacheManager(config)
	
	// Create test query
	query := &SearchQuery{
		Query:           "cacheable query",
		ObfuscatedQuery: "cacheable query",
		Type:            FilenameSearch,
		MaxResults:      10,
		PrivacyLevel:    2, // Low enough to allow caching
		SessionID:       "cache_test_session",
		RequestTime:     time.Now(),
	}
	
	// Test cache miss
	results, found := cacheManager.GetCachedResults(query)
	if found {
		t.Error("Should not find results in empty cache")
	}
	if results != nil {
		t.Error("Results should be nil for cache miss")
	}
	
	// Create test results to cache
	testResults := []SearchResult{
		{
			FileID:       "cached_file1",
			Filename:     "cached1.txt",
			Relevance:    0.9,
			MatchType:    "filename",
			PrivacyLevel: 2,
		},
		{
			FileID:       "cached_file2",
			Filename:     "cached2.txt",
			Relevance:    0.8,
			MatchType:    "filename",
			PrivacyLevel: 2,
		},
	}
	
	// Cache the results
	err := cacheManager.CacheResults(query, testResults)
	if err != nil {
		t.Fatalf("Failed to cache results: %v", err)
	}
	
	// Test cache hit
	cachedResults, found := cacheManager.GetCachedResults(query)
	if !found {
		t.Error("Should find cached results")
	}
	if cachedResults == nil {
		t.Fatal("Cached results should not be nil")
	}
	if len(cachedResults) != len(testResults) {
		t.Errorf("Cached results count mismatch: expected %d, got %d", 
			len(testResults), len(cachedResults))
	}
	
	// Test high privacy level query (should not be cached)
	highPrivacyQuery := &SearchQuery{
		Query:           "high privacy query",
		ObfuscatedQuery: "high privacy query",
		Type:            ContentSearch,
		MaxResults:      10,
		PrivacyLevel:    5, // Too high for caching
		SessionID:       "high_privacy_session",
		RequestTime:     time.Now(),
	}
	
	// Attempt to cache high privacy results
	err = cacheManager.CacheResults(highPrivacyQuery, testResults)
	// Should not error, but should skip caching
	if err != nil {
		t.Fatalf("High privacy caching should not error: %v", err)
	}
	
	// Should not find in cache
	_, found = cacheManager.GetCachedResults(highPrivacyQuery)
	if found {
		t.Error("High privacy query should not be cached")
	}
	
	// Test cache invalidation
	err = cacheManager.InvalidateCache(query.SessionID, query.PrivacyLevel)
	if err != nil {
		t.Fatalf("Cache invalidation failed: %v", err)
	}
	
	// Should not find results after invalidation
	_, found = cacheManager.GetCachedResults(query)
	if found {
		t.Error("Should not find results after cache invalidation")
	}
	
	// Test analytics
	analytics := cacheManager.GetAnalytics()
	if analytics == nil {
		t.Fatal("Analytics should not be nil")
	}
	if analytics.totalHits == 0 && analytics.totalMisses == 0 {
		t.Error("Analytics should have recorded some activity")
	}
}

// TestEnhancedSessionManagerIntegration tests the enhanced session manager
func TestEnhancedSessionManagerIntegration(t *testing.T) {
	// Create session manager
	config := DefaultSessionConfig()
	sessionManager := NewEnhancedSessionManager(config)
	
	// Create client info
	clientInfo := &ClientInfo{
		IPAddress:         "127.0.0.1",
		UserAgent:         "TestClient/1.0",
		Platform:          "test",
		Browser:           "test",
		DeviceFingerprint: "test_fingerprint",
	}
	
	// Create session
	session, err := sessionManager.CreateSession("test_user", clientInfo)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	
	if session == nil {
		t.Fatal("Session should not be nil")
	}
	if session.SessionID == "" {
		t.Error("Session ID should not be empty")
	}
	if session.UserID != "test_user" {
		t.Errorf("User ID mismatch: expected 'test_user', got '%s'", session.UserID)
	}
	if !session.IsActive {
		t.Error("Session should be active")
	}
	if session.IsExpired {
		t.Error("Session should not be expired")
	}
	
	// Test session retrieval
	retrievedSession, err := sessionManager.GetSession(session.SessionID)
	if err != nil {
		t.Fatalf("Failed to retrieve session: %v", err)
	}
	if retrievedSession.SessionID != session.SessionID {
		t.Errorf("Retrieved session ID mismatch: expected '%s', got '%s'", 
			session.SessionID, retrievedSession.SessionID)
	}
	
	// Test session update with query
	query := &SearchQuery{
		Query:           "session test query",
		ObfuscatedQuery: "session test query",
		Type:            FilenameSearch,
		MaxResults:      10,
		PrivacyLevel:    3,
		SessionID:       session.SessionID,
		RequestTime:     time.Now(),
	}
	
	err = sessionManager.UpdateSession(session.SessionID, query)
	if err != nil {
		t.Fatalf("Failed to update session: %v", err)
	}
	
	// Verify session was updated
	updatedSession, err := sessionManager.GetSession(session.SessionID)
	if err != nil {
		t.Fatalf("Failed to retrieve updated session: %v", err)
	}
	
	if updatedSession.TotalQueries != 1 {
		t.Errorf("Total queries should be 1, got %d", updatedSession.TotalQueries)
	}
	if updatedSession.QueriesByType[FilenameSearch] != 1 {
		t.Errorf("Filename search count should be 1, got %d", 
			updatedSession.QueriesByType[FilenameSearch])
	}
	if updatedSession.QueriesByPrivacyLevel[3] != 1 {
		t.Errorf("Privacy level 3 count should be 1, got %d", 
			updatedSession.QueriesByPrivacyLevel[3])
	}
	if updatedSession.MaxPrivacyLevelUsed != 3 {
		t.Errorf("Max privacy level should be 3, got %d", 
			updatedSession.MaxPrivacyLevelUsed)
	}
	
	// Test query result recording
	queryResult := &QueryResult{
		ResultCount:       5,
		PrivacyBudgetUsed: 0.1,
		CacheHit:          false,
		Duration:          time.Millisecond * 100,
	}
	
	err = sessionManager.RecordQueryResult(session.SessionID, queryResult)
	if err != nil {
		t.Fatalf("Failed to record query result: %v", err)
	}
	
	// Verify budget was updated
	finalSession, err := sessionManager.GetSession(session.SessionID)
	if err != nil {
		t.Fatalf("Failed to retrieve final session: %v", err)
	}
	
	if finalSession.PrivacyBudgetUsed != queryResult.PrivacyBudgetUsed {
		t.Errorf("Privacy budget used mismatch: expected %f, got %f", 
			queryResult.PrivacyBudgetUsed, finalSession.PrivacyBudgetUsed)
	}
	
	expectedRemaining := config.PrivacyBudgetPerSession - queryResult.PrivacyBudgetUsed
	if finalSession.PrivacyBudgetRemaining != expectedRemaining {
		t.Errorf("Privacy budget remaining mismatch: expected %f, got %f", 
			expectedRemaining, finalSession.PrivacyBudgetRemaining)
	}
	
	// Test active session count
	activeCount := sessionManager.GetActiveSessionCount()
	if activeCount != 1 {
		t.Errorf("Active session count should be 1, got %d", activeCount)
	}
	
	// Test analytics
	analytics := sessionManager.GetAnalytics()
	if analytics == nil {
		t.Fatal("Analytics should not be nil")
	}
	if analytics.TotalSessions == 0 {
		t.Error("Total sessions should be > 0")
	}
	if analytics.ActiveSessions == 0 {
		t.Error("Active sessions should be > 0")
	}
	if analytics.TotalQueries == 0 {
		t.Error("Total queries should be > 0")
	}
}

// TestPrivacySearchAnalyticsIntegration tests the privacy search analytics
func TestPrivacySearchAnalyticsIntegration(t *testing.T) {
	// Create analytics system
	config := DefaultAnalyticsConfig()
	analytics := NewPrivacySearchAnalytics(config)
	
	// Create test search metric
	metric := &SearchMetric{
		MetricType:        "search_execution",
		QueryType:         FilenameSearch,
		PrivacyLevel:      3,
		ResponseTime:      time.Millisecond * 150,
		ResultCount:       5,
		CacheHit:          false,
		NoiseLevel:        0.02,
		PrivacyBudgetUsed: 0.05,
		Timestamp:         time.Now(),
		SessionID:         "analytics_test_session",
	}
	
	// Record metric
	err := analytics.RecordSearchMetric(metric)
	if err != nil {
		t.Fatalf("Failed to record search metric: %v", err)
	}
	
	// Test aggregated metrics
	period := TimePeriod{
		StartTime: time.Now().Add(-time.Hour),
		EndTime:   time.Now(),
		Duration:  time.Hour,
	}
	
	aggregatedMetrics, err := analytics.GetAggregatedMetrics(period, SummaryLevel)
	if err != nil {
		t.Fatalf("Failed to get aggregated metrics: %v", err)
	}
	
	if aggregatedMetrics == nil {
		t.Fatal("Aggregated metrics should not be nil")
	}
	if aggregatedMetrics.SearchMetrics == nil {
		t.Fatal("Search metrics should not be nil")
	}
	if aggregatedMetrics.PerformanceMetrics == nil {
		t.Fatal("Performance metrics should not be nil")
	}
	if aggregatedMetrics.PrivacyMetrics == nil {
		t.Fatal("Privacy metrics should not be nil")
	}
	
	// Test privacy insights
	insights, err := analytics.GetPrivacyInsights()
	if err != nil {
		t.Fatalf("Failed to get privacy insights: %v", err)
	}
	
	if insights == nil {
		t.Fatal("Privacy insights should not be nil")
	}
	if insights.GeneratedAt.IsZero() {
		t.Error("Insights generation time should be set")
	}
	
	// Test report generation
	if config.GenerateReports {
		report, err := analytics.GenerateAnalyticsReport("default", period)
		if err != nil {
			t.Fatalf("Failed to generate analytics report: %v", err)
		}
		
		if report == nil {
			t.Fatal("Analytics report should not be nil")
		}
		if report.ReportID == "" {
			t.Error("Report ID should not be empty")
		}
		if report.GeneratedAt.IsZero() {
			t.Error("Report generation time should be set")
		}
	}
	
	// Test multiple metrics to verify aggregation
	for i := 0; i < 10; i++ {
		testMetric := &SearchMetric{
			MetricType:        "search_execution",
			QueryType:         ContentSearch,
			PrivacyLevel:      2,
			ResponseTime:      time.Millisecond * time.Duration(100+i*10),
			ResultCount:       i + 1,
			CacheHit:          i%2 == 0,
			NoiseLevel:        float64(i) * 0.01,
			PrivacyBudgetUsed: float64(i) * 0.01,
			Timestamp:         time.Now().Add(-time.Duration(i) * time.Minute),
			SessionID:         fmt.Sprintf("analytics_session_%d", i),
		}
		
		err := analytics.RecordSearchMetric(testMetric)
		if err != nil {
			t.Fatalf("Failed to record metric %d: %v", i, err)
		}
	}
	
	// Get updated aggregated metrics
	updatedMetrics, err := analytics.GetAggregatedMetrics(period, AggregateLevel)
	if err != nil {
		t.Fatalf("Failed to get updated aggregated metrics: %v", err)
	}
	
	// Verify metrics were aggregated
	if updatedMetrics.RecordCount == 0 {
		t.Error("Record count should be > 0 after recording multiple metrics")
	}
}

// TestPhase3FullIntegration tests the complete Phase 3 integration
func TestPhase3FullIntegration(t *testing.T) {
	// Create file index for integration
	fileIndex := fuse.NewFileIndex("/tmp/test_index.json")
	err := fileIndex.LoadIndex()
	if err != nil {
		t.Fatalf("Failed to load file index: %v", err)
	}
	
	// Add some test files to the index
	fileIndex.AddFile("test1.txt", "QmTest1", 1000)
	fileIndex.AddFile("test2.txt", "QmTest2", 2000)
	fileIndex.AddFile("document.pdf", "QmDoc1", 5000)
	
	// Create privacy search engine with Phase 3 components
	searchConfig := DefaultPrivacySearchConfig()
	searchEngine, err := NewPrivacySearchEngine(fileIndex, searchConfig)
	if err != nil {
		t.Fatalf("Failed to create privacy search engine: %v", err)
	}
	
	// Create Phase 3 components
	resultProcessorConfig := DefaultResultPrivacyConfig()
	resultProcessor := NewAdvancedResultProcessor(resultProcessorConfig)
	
	cacheConfig := DefaultCachePrivacyConfig()
	cacheManager := NewPrivacyCacheManager(cacheConfig)
	
	sessionConfig := DefaultSessionConfig()
	sessionManager := NewEnhancedSessionManager(sessionConfig)
	
	analyticsConfig := DefaultAnalyticsConfig()
	analytics := NewPrivacySearchAnalytics(analyticsConfig)
	
	// Create client info
	clientInfo := &ClientInfo{
		IPAddress:         "127.0.0.1",
		UserAgent:         "TestClient/1.0",
		Platform:          "test",
		Browser:           "test",
		DeviceFingerprint: "integration_test",
	}
	
	// Create session
	session, err := sessionManager.CreateSession("integration_test_user", clientInfo)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	
	// Test search with full Phase 3 pipeline
	ctx := context.Background()
	searchOptions := map[string]interface{}{
		"privacy_level": 3,
		"session_id":    session.SessionID,
		"max_results":   10,
	}
	
	// Execute search
	searchResponse, err := searchEngine.Search(ctx, "integration test document", searchOptions)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	
	if searchResponse == nil {
		t.Fatal("Search response should not be nil")
	}
	
	// Test result processing with advanced result processor
	if len(searchResponse.Results) > 0 {
		processedResults, err := resultProcessor.ProcessResults(ctx, searchResponse.Results, searchResponse.Query)
		if err != nil {
			t.Fatalf("Result processing failed: %v", err)
		}
		
		// Verify enhanced processing
		for _, result := range processedResults {
			if result.PrivacyLevel != searchResponse.Query.PrivacyLevel {
				t.Errorf("Privacy level not applied correctly in result processing")
			}
			if searchResponse.Query.PrivacyLevel >= 3 && result.NoiseLevel == 0 {
				t.Error("Expected noise to be applied by advanced result processor")
			}
		}
		
		// Test caching with processed results
		err = cacheManager.CacheResults(searchResponse.Query, processedResults)
		if err != nil {
			t.Fatalf("Failed to cache processed results: %v", err)
		}
		
		// Test cache retrieval
		cachedResults, found := cacheManager.GetCachedResults(searchResponse.Query)
		if !found {
			t.Error("Should find cached results")
		}
		if len(cachedResults) != len(processedResults) {
			t.Errorf("Cached results count mismatch")
		}
	}
	
	// Update session with search activity
	err = sessionManager.UpdateSession(session.SessionID, searchResponse.Query)
	if err != nil {
		t.Fatalf("Failed to update session: %v", err)
	}
	
	// Record query result
	queryResult := &QueryResult{
		ResultCount:       len(searchResponse.Results),
		PrivacyBudgetUsed: 0.1,
		CacheHit:          false,
		Duration:          time.Millisecond * 200,
	}
	
	err = sessionManager.RecordQueryResult(session.SessionID, queryResult)
	if err != nil {
		t.Fatalf("Failed to record query result: %v", err)
	}
	
	// Record analytics
	metric := &SearchMetric{
		MetricType:        "integration_test",
		QueryType:         searchResponse.Query.Type,
		PrivacyLevel:      searchResponse.Query.PrivacyLevel,
		ResponseTime:      queryResult.Duration,
		ResultCount:       queryResult.ResultCount,
		CacheHit:          queryResult.CacheHit,
		NoiseLevel:        0.02,
		PrivacyBudgetUsed: queryResult.PrivacyBudgetUsed,
		Timestamp:         time.Now(),
		SessionID:         session.SessionID,
	}
	
	err = analytics.RecordSearchMetric(metric)
	if err != nil {
		t.Fatalf("Failed to record analytics metric: %v", err)
	}
	
	// Verify session state
	finalSession, err := sessionManager.GetSession(session.SessionID)
	if err != nil {
		t.Fatalf("Failed to retrieve final session: %v", err)
	}
	
	if finalSession.TotalQueries == 0 {
		t.Error("Session should have recorded queries")
	}
	if finalSession.PrivacyBudgetUsed == 0 {
		t.Error("Session should have used privacy budget")
	}
	if finalSession.MaxPrivacyLevelUsed != searchResponse.Query.PrivacyLevel {
		t.Error("Session should track maximum privacy level used")
	}
	
	// Test analytics aggregation
	period := TimePeriod{
		StartTime: time.Now().Add(-time.Hour),
		EndTime:   time.Now(),
		Duration:  time.Hour,
	}
	
	aggregatedMetrics, err := analytics.GetAggregatedMetrics(period, SummaryLevel)
	if err != nil {
		t.Fatalf("Failed to get aggregated metrics: %v", err)
	}
	
	if aggregatedMetrics == nil {
		t.Fatal("Should have aggregated metrics")
	}
	
	// Test privacy insights
	insights, err := analytics.GetPrivacyInsights()
	if err != nil {
		t.Fatalf("Failed to get privacy insights: %v", err)
	}
	
	if insights == nil {
		t.Fatal("Should have privacy insights")
	}
	
	// Test cache analytics
	cacheAnalytics := cacheManager.GetAnalytics()
	if cacheAnalytics == nil {
		t.Fatal("Should have cache analytics")
	}
	
	// Test session analytics
	sessionAnalytics := sessionManager.GetAnalytics()
	if sessionAnalytics == nil {
		t.Fatal("Should have session analytics")
	}
	if sessionAnalytics.TotalSessions == 0 {
		t.Error("Should have recorded sessions")
	}
	if sessionAnalytics.TotalQueries == 0 {
		t.Error("Should have recorded queries")
	}
	
	// Test search engine stats with Phase 3 enhancements
	searchStats := searchEngine.GetStats()
	if searchStats == nil {
		t.Fatal("Should have search stats")
	}
	if searchStats.TotalQueries == 0 {
		t.Error("Should have recorded search queries")
	}
}

// BenchmarkPhase3Components benchmarks Phase 3 components
func BenchmarkPhase3Components(b *testing.B) {
	// Setup components
	resultProcessor := NewAdvancedResultProcessor(DefaultResultPrivacyConfig())
	cacheManager := NewPrivacyCacheManager(DefaultCachePrivacyConfig())
	sessionManager := NewEnhancedSessionManager(DefaultSessionConfig())
	analytics := NewPrivacySearchAnalytics(DefaultAnalyticsConfig())
	
	// Create test data
	query := &SearchQuery{
		Query:           "benchmark test",
		ObfuscatedQuery: "benchmark test",
		Type:            FilenameSearch,
		MaxResults:      10,
		PrivacyLevel:    3,
		SessionID:       "benchmark_session",
		RequestTime:     time.Now(),
	}
	
	results := []SearchResult{
		{FileID: "bench1", Relevance: 0.9, PrivacyLevel: 3},
		{FileID: "bench2", Relevance: 0.8, PrivacyLevel: 3},
		{FileID: "bench3", Relevance: 0.7, PrivacyLevel: 3},
	}
	
	ctx := context.Background()
	
	b.Run("AdvancedResultProcessor", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := resultProcessor.ProcessResults(ctx, results, query)
			if err != nil {
				b.Fatalf("Result processing failed: %v", err)
			}
		}
	})
	
	b.Run("PrivacyCacheManager", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			err := cacheManager.CacheResults(query, results)
			if err != nil {
				b.Fatalf("Caching failed: %v", err)
			}
			_, _ = cacheManager.GetCachedResults(query)
		}
	})
	
	b.Run("EnhancedSessionManager", func(b *testing.B) {
		// Create session once
		session, err := sessionManager.CreateSession("bench_user", &ClientInfo{})
		if err != nil {
			b.Fatalf("Session creation failed: %v", err)
		}
		
		for i := 0; i < b.N; i++ {
			err := sessionManager.UpdateSession(session.SessionID, query)
			if err != nil {
				b.Fatalf("Session update failed: %v", err)
			}
		}
	})
	
	b.Run("PrivacySearchAnalytics", func(b *testing.B) {
		metric := &SearchMetric{
			MetricType:        "benchmark",
			QueryType:         query.Type,
			PrivacyLevel:      query.PrivacyLevel,
			ResponseTime:      time.Millisecond * 100,
			ResultCount:       len(results),
			Timestamp:         time.Now(),
			SessionID:         query.SessionID,
		}
		
		for i := 0; i < b.N; i++ {
			err := analytics.RecordSearchMetric(metric)
			if err != nil {
				b.Fatalf("Analytics recording failed: %v", err)
			}
		}
	})
}