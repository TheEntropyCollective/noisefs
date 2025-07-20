package validation

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
	
	"github.com/TheEntropyCollective/noisefs/pkg/security"
)

// RateLimiter provides rate limiting functionality
type RateLimiter struct {
	clients  map[string]*ClientLimiter
	mu       sync.RWMutex
	cleanup  *time.Ticker
	done     chan bool
	config   RateLimitConfig
}

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	RequestsPerMinute   int
	RequestsPerHour     int
	BurstSize          int
	CleanupInterval    time.Duration
	BanDuration        time.Duration
	MaxConcurrent      int
}

// ClientLimiter tracks rate limiting for a single client
type ClientLimiter struct {
	IP                string
	RequestsThisMinute int
	RequestsThisHour   int
	LastRequest        time.Time
	LastMinute         time.Time
	LastHour           time.Time
	BannedUntil        time.Time
	ConcurrentRequests int
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(config RateLimitConfig) *RateLimiter {
	rl := &RateLimiter{
		clients: make(map[string]*ClientLimiter),
		done:    make(chan bool),
		config:  config,
	}
	
	// Start cleanup goroutine
	rl.cleanup = time.NewTicker(config.CleanupInterval)
	go rl.cleanupLoop()
	
	return rl
}

// DefaultRateLimitConfig returns sensible default rate limiting configuration
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		RequestsPerMinute: 60,    // 1 request per second average
		RequestsPerHour:   1000,  // ~16 requests per minute sustained
		BurstSize:         10,    // Allow bursts up to 10 requests
		CleanupInterval:   5 * time.Minute,
		BanDuration:       15 * time.Minute,
		MaxConcurrent:     5,     // Max 5 concurrent requests per IP
	}
}

// CheckLimit checks if a request should be allowed
func (rl *RateLimiter) CheckLimit(r *http.Request) error {
	ip := getClientIP(r)
	
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	client, exists := rl.clients[ip]
	if !exists {
		client = &ClientLimiter{
			IP:         ip,
			LastMinute: time.Now(),
			LastHour:   time.Now(),
		}
		rl.clients[ip] = client
	}
	
	now := time.Now()
	
	// Check if client is banned
	if now.Before(client.BannedUntil) {
		// Sanitize IP address in error message
		sanitizedIP := security.SanitizeString(ip, "", false)
		return fmt.Errorf("IP %s is temporarily banned", sanitizedIP)
	}
	
	// Reset counters if needed
	if now.Sub(client.LastMinute) >= time.Minute {
		client.RequestsThisMinute = 0
		client.LastMinute = now
	}
	
	if now.Sub(client.LastHour) >= time.Hour {
		client.RequestsThisHour = 0
		client.LastHour = now
	}
	
	// Check concurrent requests
	if client.ConcurrentRequests >= rl.config.MaxConcurrent {
		// Sanitize IP address in error message
		sanitizedIP := security.SanitizeString(ip, "", false)
		return fmt.Errorf("too many concurrent requests from IP %s", sanitizedIP)
	}
	
	// Check rate limits
	if client.RequestsThisMinute >= rl.config.RequestsPerMinute {
		// Ban for repeated violations
		if client.RequestsThisMinute > rl.config.RequestsPerMinute*2 {
			client.BannedUntil = now.Add(rl.config.BanDuration)
		}
		// Sanitize IP address in error message
		sanitizedIP := security.SanitizeString(ip, "", false)
		return fmt.Errorf("rate limit exceeded for IP %s (requests per minute)", sanitizedIP)
	}
	
	if client.RequestsThisHour >= rl.config.RequestsPerHour {
		// Sanitize IP address in error message
		sanitizedIP := security.SanitizeString(ip, "", false)
		return fmt.Errorf("rate limit exceeded for IP %s (requests per hour)", sanitizedIP)
	}
	
	// Allow request - increment counters
	client.RequestsThisMinute++
	client.RequestsThisHour++
	client.LastRequest = now
	client.ConcurrentRequests++
	
	return nil
}

// ReleaseRequest releases a concurrent request slot
func (rl *RateLimiter) ReleaseRequest(r *http.Request) {
	ip := getClientIP(r)
	
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	if client, exists := rl.clients[ip]; exists {
		if client.ConcurrentRequests > 0 {
			client.ConcurrentRequests--
		}
	}
}

// getClientIP extracts the real client IP from the request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		// Take the first IP in the chain
		ips := parseXForwardedFor(xff)
		if len(ips) > 0 {
			return ips[0]
		}
	}
	
	// Check X-Real-IP header
	xri := r.Header.Get("X-Real-IP")
	if xri != "" {
		return xri
	}
	
	// Fall back to RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	
	return host
}

// parseXForwardedFor parses the X-Forwarded-For header
func parseXForwardedFor(header string) []string {
	var ips []string
	
	for _, ip := range strings.Split(header, ",") {
		ip = strings.TrimSpace(ip)
		if ip != "" && net.ParseIP(ip) != nil {
			ips = append(ips, ip)
		}
	}
	
	return ips
}

// cleanupLoop periodically cleans up old client entries
func (rl *RateLimiter) cleanupLoop() {
	for {
		select {
		case <-rl.cleanup.C:
			rl.cleanupOldClients()
		case <-rl.done:
			return
		}
	}
}

// cleanupOldClients removes old client entries to prevent memory leaks
func (rl *RateLimiter) cleanupOldClients() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	cutoff := time.Now().Add(-2 * time.Hour)
	
	for ip, client := range rl.clients {
		if client.LastRequest.Before(cutoff) && client.ConcurrentRequests == 0 {
			delete(rl.clients, ip)
		}
	}
}

// Shutdown stops the rate limiter cleanup
func (rl *RateLimiter) Shutdown() {
	if rl.cleanup != nil {
		rl.cleanup.Stop()
	}
	
	select {
	case rl.done <- true:
	default:
	}
}

// GetStats returns rate limiter statistics
func (rl *RateLimiter) GetStats() map[string]interface{} {
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	
	activeClients := 0
	bannedClients := 0
	totalConcurrent := 0
	
	now := time.Now()
	for _, client := range rl.clients {
		activeClients++
		totalConcurrent += client.ConcurrentRequests
		
		if now.Before(client.BannedUntil) {
			bannedClients++
		}
	}
	
	return map[string]interface{}{
		"active_clients":    activeClients,
		"banned_clients":    bannedClients,
		"total_concurrent":  totalConcurrent,
		"requests_per_min":  rl.config.RequestsPerMinute,
		"requests_per_hour": rl.config.RequestsPerHour,
	}
}

// Middleware creates an HTTP middleware for rate limiting
func (rl *RateLimiter) Middleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check rate limit
		if err := rl.CheckLimit(r); err != nil {
			http.Error(w, err.Error(), http.StatusTooManyRequests)
			return
		}
		
		// Ensure we release the request slot when done
		defer rl.ReleaseRequest(r)
		
		// Call next handler
		next(w, r)
	}
}

// RequestSizeLimiter provides request size limiting middleware
func RequestSizeLimiter(maxSize int64) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// Limit request body size
			if r.ContentLength > maxSize {
				http.Error(w, fmt.Sprintf("Request body too large (max %d bytes)", maxSize), http.StatusRequestEntityTooLarge)
				return
			}
			
			// Wrap the request body with a size-limited reader
			r.Body = http.MaxBytesReader(w, r.Body, maxSize)
			
			next(w, r)
		}
	}
}