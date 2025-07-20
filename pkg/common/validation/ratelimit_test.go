package validation

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewRateLimiter(t *testing.T) {
	config := DefaultRateLimitConfig()
	rl := NewRateLimiter(config)
	defer rl.Shutdown()
	
	if rl.clients == nil {
		t.Error("Expected clients map to be initialized")
	}
	
	if rl.done == nil {
		t.Error("Expected done channel to be initialized")
	}
	
	if rl.cleanup == nil {
		t.Error("Expected cleanup ticker to be initialized")
	}
	
	if rl.config.RequestsPerMinute != config.RequestsPerMinute {
		t.Errorf("Expected requests per minute %d, got %d", config.RequestsPerMinute, rl.config.RequestsPerMinute)
	}
}

func TestDefaultRateLimitConfig(t *testing.T) {
	config := DefaultRateLimitConfig()
	
	if config.RequestsPerMinute != 60 {
		t.Errorf("Expected 60 requests per minute, got %d", config.RequestsPerMinute)
	}
	
	if config.RequestsPerHour != 1000 {
		t.Errorf("Expected 1000 requests per hour, got %d", config.RequestsPerHour)
	}
	
	if config.BurstSize != 10 {
		t.Errorf("Expected burst size 10, got %d", config.BurstSize)
	}
	
	if config.CleanupInterval != 5*time.Minute {
		t.Errorf("Expected cleanup interval 5m, got %v", config.CleanupInterval)
	}
	
	if config.BanDuration != 15*time.Minute {
		t.Errorf("Expected ban duration 15m, got %v", config.BanDuration)
	}
	
	if config.MaxConcurrent != 5 {
		t.Errorf("Expected max concurrent 5, got %d", config.MaxConcurrent)
	}
}

func TestCheckLimit_NewClient(t *testing.T) {
	config := RateLimitConfig{
		RequestsPerMinute: 10,
		RequestsPerHour:   100,
		MaxConcurrent:     5,
		CleanupInterval:   time.Minute,
		BanDuration:       time.Minute,
	}
	rl := NewRateLimiter(config)
	defer rl.Shutdown()
	
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	
	err := rl.CheckLimit(req)
	if err != nil {
		t.Errorf("Expected no error for new client, got: %v", err)
	}
	
	// Verify client was created
	rl.mu.RLock()
	client, exists := rl.clients["192.168.1.100"]
	rl.mu.RUnlock()
	
	if !exists {
		t.Error("Expected client to be created")
	}
	
	if client.RequestsThisMinute != 1 {
		t.Errorf("Expected 1 request this minute, got %d", client.RequestsThisMinute)
	}
	
	if client.ConcurrentRequests != 1 {
		t.Errorf("Expected 1 concurrent request, got %d", client.ConcurrentRequests)
	}
}

func TestCheckLimit_RateLimit(t *testing.T) {
	config := RateLimitConfig{
		RequestsPerMinute: 2,
		RequestsPerHour:   100,
		MaxConcurrent:     5,
		CleanupInterval:   time.Minute,
		BanDuration:       time.Minute,
	}
	rl := NewRateLimiter(config)
	defer rl.Shutdown()
	
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	
	// First two requests should pass
	for i := 0; i < 2; i++ {
		err := rl.CheckLimit(req)
		if err != nil {
			t.Errorf("Request %d should pass, got error: %v", i+1, err)
		}
	}
	
	// Third request should fail
	err := rl.CheckLimit(req)
	if err == nil {
		t.Error("Expected rate limit error")
	}
	
	if !strings.Contains(err.Error(), "rate limit exceeded") {
		t.Errorf("Expected rate limit error message, got: %v", err)
	}
}

func TestCheckLimit_ConcurrentLimit(t *testing.T) {
	config := RateLimitConfig{
		RequestsPerMinute: 100,
		RequestsPerHour:   1000,
		MaxConcurrent:     2,
		CleanupInterval:   time.Minute,
		BanDuration:       time.Minute,
	}
	rl := NewRateLimiter(config)
	defer rl.Shutdown()
	
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	
	// First two requests should pass
	for i := 0; i < 2; i++ {
		err := rl.CheckLimit(req)
		if err != nil {
			t.Errorf("Request %d should pass, got error: %v", i+1, err)
		}
	}
	
	// Third concurrent request should fail
	err := rl.CheckLimit(req)
	if err == nil {
		t.Error("Expected concurrent request limit error")
	}
	
	if !strings.Contains(err.Error(), "too many concurrent requests") {
		t.Errorf("Expected concurrent limit error message, got: %v", err)
	}
}

func TestCheckLimit_BanTrigger(t *testing.T) {
	config := RateLimitConfig{
		RequestsPerMinute: 2,
		RequestsPerHour:   100,
		MaxConcurrent:     10,
		CleanupInterval:   time.Minute,
		BanDuration:       100 * time.Millisecond,
	}
	rl := NewRateLimiter(config)
	defer rl.Shutdown()
	
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	
	// Manually create a client that exceeds 2x the rate limit
	rl.mu.Lock()
	rl.clients["192.168.1.100"] = &ClientLimiter{
		IP:                 "192.168.1.100",
		RequestsThisMinute: 5, // Exceeds 2 * RequestsPerMinute (4)
		LastMinute:         time.Now(),
		LastHour:           time.Now(),
	}
	rl.mu.Unlock()
	
	// This should trigger ban setting and return rate limit error
	err := rl.CheckLimit(req)
	if err == nil {
		t.Error("Expected rate limit error")
	}
	
	if !strings.Contains(err.Error(), "rate limit exceeded") {
		t.Errorf("Expected rate limit error message, got: %v", err)
	}
	
	// Verify ban was set
	rl.mu.RLock()
	client := rl.clients["192.168.1.100"]
	banned := time.Now().Before(client.BannedUntil)
	rl.mu.RUnlock()
	
	if !banned {
		t.Error("Expected client to be banned after exceeding 2x rate limit")
	}
	
	// Next request should detect the ban
	err = rl.CheckLimit(req)
	if err == nil {
		t.Error("Expected ban error")
	}
	
	if !strings.Contains(err.Error(), "temporarily banned") {
		t.Errorf("Expected ban error message, got: %v", err)
	}
}

func TestCheckLimit_BannedClient(t *testing.T) {
	config := RateLimitConfig{
		RequestsPerMinute: 1,
		RequestsPerHour:   100,
		MaxConcurrent:     5,
		CleanupInterval:   time.Minute,
		BanDuration:       100 * time.Millisecond,
	}
	rl := NewRateLimiter(config)
	defer rl.Shutdown()
	
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	
	// Manually set up a banned client to test ban detection
	rl.mu.Lock()
	rl.clients["192.168.1.100"] = &ClientLimiter{
		IP:          "192.168.1.100",
		BannedUntil: time.Now().Add(time.Minute),
		LastMinute:  time.Now(),
		LastHour:    time.Now(),
	}
	rl.mu.Unlock()
	
	// Should get ban error
	err := rl.CheckLimit(req)
	if err == nil {
		t.Error("Expected ban error")
	}
	
	if !strings.Contains(err.Error(), "temporarily banned") {
		t.Errorf("Expected ban error message, got: %v", err)
	}
	
	// Test ban expiration by setting BannedUntil to the past
	rl.mu.Lock()
	rl.clients["192.168.1.100"].BannedUntil = time.Now().Add(-time.Minute)
	rl.mu.Unlock()
	
	// Should be able to make request again (first request should be allowed)
	err = rl.CheckLimit(req)
	if err != nil && strings.Contains(err.Error(), "temporarily banned") {
		t.Error("Client should no longer be banned")
	}
}

func TestReleaseRequest(t *testing.T) {
	config := DefaultRateLimitConfig()
	rl := NewRateLimiter(config)
	defer rl.Shutdown()
	
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	
	// Make a request
	err := rl.CheckLimit(req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	
	// Verify concurrent count
	rl.mu.RLock()
	client := rl.clients["192.168.1.100"]
	if client.ConcurrentRequests != 1 {
		t.Errorf("Expected 1 concurrent request, got %d", client.ConcurrentRequests)
	}
	rl.mu.RUnlock()
	
	// Release the request
	rl.ReleaseRequest(req)
	
	// Verify concurrent count decreased
	rl.mu.RLock()
	client = rl.clients["192.168.1.100"]
	if client.ConcurrentRequests != 0 {
		t.Errorf("Expected 0 concurrent requests, got %d", client.ConcurrentRequests)
	}
	rl.mu.RUnlock()
}

func TestReleaseRequest_NonexistentClient(t *testing.T) {
	config := DefaultRateLimitConfig()
	rl := NewRateLimiter(config)
	defer rl.Shutdown()
	
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	
	// Release request for non-existent client should not panic
	rl.ReleaseRequest(req)
}

func TestGetClientIP(t *testing.T) {
	tests := []struct {
		name           string
		remoteAddr     string
		xForwardedFor  string
		xRealIP        string
		expectedIP     string
	}{
		{
			name:       "remote addr only",
			remoteAddr: "192.168.1.100:12345",
			expectedIP: "192.168.1.100",
		},
		{
			name:          "x-forwarded-for single IP",
			remoteAddr:    "10.0.0.1:12345",
			xForwardedFor: "192.168.1.100",
			expectedIP:    "192.168.1.100",
		},
		{
			name:          "x-forwarded-for multiple IPs",
			remoteAddr:    "10.0.0.1:12345",
			xForwardedFor: "192.168.1.100, 10.0.0.2, 10.0.0.3",
			expectedIP:    "192.168.1.100",
		},
		{
			name:        "x-real-ip",
			remoteAddr:  "10.0.0.1:12345",
			xRealIP:     "192.168.1.100",
			expectedIP:  "192.168.1.100",
		},
		{
			name:          "x-forwarded-for takes precedence",
			remoteAddr:    "10.0.0.1:12345",
			xForwardedFor: "192.168.1.100",
			xRealIP:       "192.168.1.200",
			expectedIP:    "192.168.1.100",
		},
		{
			name:       "invalid remote addr",
			remoteAddr: "invalid",
			expectedIP: "invalid",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			req.RemoteAddr = tt.remoteAddr
			
			if tt.xForwardedFor != "" {
				req.Header.Set("X-Forwarded-For", tt.xForwardedFor)
			}
			
			if tt.xRealIP != "" {
				req.Header.Set("X-Real-IP", tt.xRealIP)
			}
			
			ip := getClientIP(req)
			if ip != tt.expectedIP {
				t.Errorf("Expected IP %s, got %s", tt.expectedIP, ip)
			}
		})
	}
}

func TestParseXForwardedFor(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		expected []string
	}{
		{
			name:     "single IP",
			header:   "192.168.1.100",
			expected: []string{"192.168.1.100"},
		},
		{
			name:     "multiple IPs",
			header:   "192.168.1.100, 10.0.0.2, 172.16.0.3",
			expected: []string{"192.168.1.100", "10.0.0.2", "172.16.0.3"},
		},
		{
			name:     "with whitespace",
			header:   "  192.168.1.100  ,  10.0.0.2  ",
			expected: []string{"192.168.1.100", "10.0.0.2"},
		},
		{
			name:     "invalid IPs mixed",
			header:   "192.168.1.100, invalid, 10.0.0.2",
			expected: []string{"192.168.1.100", "10.0.0.2"},
		},
		{
			name:     "empty",
			header:   "",
			expected: nil,
		},
		{
			name:     "all invalid",
			header:   "invalid, also-invalid",
			expected: nil,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseXForwardedFor(tt.header)
			
			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d IPs, got %d", len(tt.expected), len(result))
				return
			}
			
			for i, ip := range result {
				if ip != tt.expected[i] {
					t.Errorf("Expected IP[%d] = %s, got %s", i, tt.expected[i], ip)
				}
			}
		})
	}
}

func TestCleanupOldClients(t *testing.T) {
	config := RateLimitConfig{
		RequestsPerMinute: 100,
		RequestsPerHour:   1000,
		MaxConcurrent:     5,
		CleanupInterval:   time.Millisecond, // Very short for testing
		BanDuration:       time.Minute,
	}
	rl := NewRateLimiter(config)
	defer rl.Shutdown()
	
	// Add some test clients
	rl.mu.Lock()
	oldTime := time.Now().Add(-3 * time.Hour)
	recentTime := time.Now().Add(-30 * time.Minute)
	
	rl.clients["old_client"] = &ClientLimiter{
		IP:                "192.168.1.100",
		LastRequest:       oldTime,
		ConcurrentRequests: 0,
	}
	
	rl.clients["recent_client"] = &ClientLimiter{
		IP:                "192.168.1.101",
		LastRequest:       recentTime,
		ConcurrentRequests: 0,
	}
	
	rl.clients["busy_client"] = &ClientLimiter{
		IP:                "192.168.1.102",
		LastRequest:       oldTime,
		ConcurrentRequests: 1, // Has active requests
	}
	rl.mu.Unlock()
	
	// Trigger cleanup
	rl.cleanupOldClients()
	
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	
	// Old client with no concurrent requests should be removed
	if _, exists := rl.clients["old_client"]; exists {
		t.Error("Expected old_client to be removed")
	}
	
	// Recent client should remain
	if _, exists := rl.clients["recent_client"]; !exists {
		t.Error("Expected recent_client to remain")
	}
	
	// Busy client should remain (has concurrent requests)
	if _, exists := rl.clients["busy_client"]; !exists {
		t.Error("Expected busy_client to remain")
	}
}

func TestShutdown(t *testing.T) {
	config := DefaultRateLimitConfig()
	rl := NewRateLimiter(config)
	
	// Verify cleanup is running
	if rl.cleanup == nil {
		t.Error("Expected cleanup ticker to be initialized")
	}
	
	rl.Shutdown()
	
	// Give some time for goroutine to exit
	time.Sleep(10 * time.Millisecond)
	
	// Shutdown should be idempotent
	rl.Shutdown()
}

func TestGetStats(t *testing.T) {
	config := DefaultRateLimitConfig()
	rl := NewRateLimiter(config)
	defer rl.Shutdown()
	
	// Add some test data
	now := time.Now()
	rl.mu.Lock()
	rl.clients["client1"] = &ClientLimiter{
		IP:                "192.168.1.100",
		ConcurrentRequests: 2,
		BannedUntil:       now.Add(time.Minute),
	}
	rl.clients["client2"] = &ClientLimiter{
		IP:                "192.168.1.101",
		ConcurrentRequests: 1,
		BannedUntil:       now.Add(-time.Minute), // Not banned
	}
	rl.mu.Unlock()
	
	stats := rl.GetStats()
	
	if stats["active_clients"] != 2 {
		t.Errorf("Expected 2 active clients, got %v", stats["active_clients"])
	}
	
	if stats["banned_clients"] != 1 {
		t.Errorf("Expected 1 banned client, got %v", stats["banned_clients"])
	}
	
	if stats["total_concurrent"] != 3 {
		t.Errorf("Expected 3 total concurrent, got %v", stats["total_concurrent"])
	}
	
	if stats["requests_per_min"] != config.RequestsPerMinute {
		t.Errorf("Expected requests_per_min %d, got %v", config.RequestsPerMinute, stats["requests_per_min"])
	}
}

func TestMiddleware(t *testing.T) {
	config := RateLimitConfig{
		RequestsPerMinute: 2,
		RequestsPerHour:   100,
		MaxConcurrent:     5,
		CleanupInterval:   time.Minute,
		BanDuration:       time.Minute,
	}
	rl := NewRateLimiter(config)
	defer rl.Shutdown()
	
	handlerCalled := false
	handler := func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	}
	
	middleware := rl.Middleware(handler)
	
	// Test successful request
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	rr := httptest.NewRecorder()
	
	middleware(rr, req)
	
	if !handlerCalled {
		t.Error("Expected handler to be called")
	}
	
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}
	
	// Test rate limited request  
	// The first request was already processed and released
	// Now make a second request
	handlerCalled = false
	rr = httptest.NewRecorder()
	middleware(rr, req) // Second request
	
	if !handlerCalled {
		t.Error("Expected handler to be called for second request")
	}
	
	// Third request should be rate limited (exceeds 2 requests per minute)
	handlerCalled = false
	rr = httptest.NewRecorder()
	middleware(rr, req) // Third request - should be rate limited
	
	if handlerCalled {
		t.Error("Expected handler not to be called for rate limited request")
	}
	
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("Expected status 429, got %d", rr.Code)
	}
}

func TestRequestSizeLimiter(t *testing.T) {
	maxSize := int64(1024)
	limiter := RequestSizeLimiter(maxSize)
	
	handlerCalled := false
	handler := func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	}
	
	middleware := limiter(handler)
	
	// Test request within size limit
	req := httptest.NewRequest("POST", "/test", strings.NewReader("small content"))
	req.ContentLength = 13
	rr := httptest.NewRecorder()
	
	middleware(rr, req)
	
	if !handlerCalled {
		t.Error("Expected handler to be called for small request")
	}
	
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}
	
	// Test request exceeding size limit
	handlerCalled = false
	req = httptest.NewRequest("POST", "/test", strings.NewReader("large content"))
	req.ContentLength = maxSize + 1
	rr = httptest.NewRecorder()
	
	middleware(rr, req)
	
	if handlerCalled {
		t.Error("Expected handler not to be called for large request")
	}
	
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("Expected status 413, got %d", rr.Code)
	}
}

func TestRateLimiter_CounterReset(t *testing.T) {
	config := RateLimitConfig{
		RequestsPerMinute: 5,
		RequestsPerHour:   100,
		MaxConcurrent:     10,
		CleanupInterval:   time.Minute,
		BanDuration:       time.Minute,
	}
	rl := NewRateLimiter(config)
	defer rl.Shutdown()
	
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	
	// Make some requests
	for i := 0; i < 3; i++ {
		err := rl.CheckLimit(req)
		if err != nil {
			t.Fatalf("Request %d failed: %v", i+1, err)
		}
	}
	
	// Verify counter
	rl.mu.RLock()
	client := rl.clients["192.168.1.100"]
	if client.RequestsThisMinute != 3 {
		t.Errorf("Expected 3 requests this minute, got %d", client.RequestsThisMinute)
	}
	rl.mu.RUnlock()
	
	// Manually reset the minute counter by setting LastMinute to past
	rl.mu.Lock()
	client.LastMinute = time.Now().Add(-2 * time.Minute)
	rl.mu.Unlock()
	
	// Next request should reset the counter
	err := rl.CheckLimit(req)
	if err != nil {
		t.Fatalf("Request after reset failed: %v", err)
	}
	
	rl.mu.RLock()
	client = rl.clients["192.168.1.100"]
	if client.RequestsThisMinute != 1 {
		t.Errorf("Expected 1 request this minute after reset, got %d", client.RequestsThisMinute)
	}
	rl.mu.RUnlock()
}

func TestRateLimiter_ConcurrentSafety(t *testing.T) {
	config := RateLimitConfig{
		RequestsPerMinute: 1000,
		RequestsPerHour:   10000,
		MaxConcurrent:     100,
		CleanupInterval:   time.Minute,
		BanDuration:       time.Minute,
	}
	rl := NewRateLimiter(config)
	defer rl.Shutdown()
	
	numGoroutines := 50
	requestsPerGoroutine := 20
	
	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*requestsPerGoroutine)
	
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			
			for j := 0; j < requestsPerGoroutine; j++ {
				req := httptest.NewRequest("GET", "/test", nil)
				req.RemoteAddr = fmt.Sprintf("192.168.1.%d:12345", goroutineID%10) // 10 different IPs
				
				err := rl.CheckLimit(req)
				if err != nil {
					errors <- err
				} else {
					// Release the request
					rl.ReleaseRequest(req)
				}
			}
		}(i)
	}
	
	wg.Wait()
	close(errors)
	
	// Count errors
	errorCount := 0
	for err := range errors {
		if err != nil {
			errorCount++
		}
	}
	
	// Should have some successful requests
	successCount := (numGoroutines * requestsPerGoroutine) - errorCount
	if successCount == 0 {
		t.Error("Expected some successful requests in concurrent test")
	}
	
	// Verify no data races by checking final state
	stats := rl.GetStats()
	if stats["active_clients"].(int) > 10 {
		t.Errorf("Expected at most 10 active clients, got %d", stats["active_clients"])
	}
}

// Benchmark tests for performance validation
func BenchmarkCheckLimit(b *testing.B) {
	config := DefaultRateLimitConfig()
	rl := NewRateLimiter(config)
	defer rl.Shutdown()
	
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rl.CheckLimit(req)
		rl.ReleaseRequest(req)
	}
}

func BenchmarkGetClientIP(b *testing.B) {
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	req.Header.Set("X-Forwarded-For", "10.0.0.1, 10.0.0.2, 192.168.1.100")
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		getClientIP(req)
	}
}

func BenchmarkParseXForwardedFor(b *testing.B) {
	header := "192.168.1.100, 10.0.0.2, 172.16.0.3, 203.0.113.1"
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parseXForwardedFor(header)
	}
}

func BenchmarkMiddleware(b *testing.B) {
	config := DefaultRateLimitConfig()
	rl := NewRateLimiter(config)
	defer rl.Shutdown()
	
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}
	
	middleware := rl.Middleware(handler)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = fmt.Sprintf("192.168.1.%d:12345", i%100) // Vary IP to avoid rate limiting
		rr := httptest.NewRecorder()
		
		middleware(rr, req)
	}
}