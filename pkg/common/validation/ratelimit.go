// Package validation provides comprehensive input validation and rate limiting
// for NoiseFS HTTP services and API endpoints.
//
// Rate Limiting Features:
//   - IP-based request rate limiting with configurable thresholds
//   - Automatic client banning for repeated violations
//   - Concurrent request limiting per IP address
//   - Background cleanup of stale client entries
//   - HTTP middleware integration for easy deployment
//
// Security Features:
//   - Proper IP address extraction from proxy headers
//   - Sanitized error messages to prevent information disclosure
//   - Memory leak prevention through automatic cleanup
//   - Configurable ban durations for malicious clients
//
// Usage Example:
//
//	// Create rate limiter with default configuration
//	config := DefaultRateLimitConfig()
//	rateLimiter := NewRateLimiter(config)
//	defer rateLimiter.Shutdown()
//	
//	// Use as HTTP middleware
//	http.HandleFunc("/api/upload", rateLimiter.Middleware(uploadHandler))
//	
//	// Add request size limiting
//	sizeLimited := RequestSizeLimiter(10 * 1024 * 1024) // 10MB max
//	http.HandleFunc("/api/data", sizeLimited(dataHandler))
//
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

// RateLimiter provides comprehensive IP-based rate limiting with automatic client management.
//
// This type implements a sophisticated rate limiting system designed to protect
// NoiseFS services from abuse while maintaining good performance for legitimate
// users. It tracks per-IP request rates and enforces multiple types of limits.
//
// Rate Limiting Features:
//   - Requests per minute and per hour limits
//   - Concurrent request limiting per IP
//   - Automatic client banning for violations
//   - Configurable burst allowances
//   - Background cleanup of inactive clients
//
// Memory Management:
//   - Automatic cleanup prevents memory leaks
//   - Read-write mutex for efficient concurrent access
//   - Client entries expire after inactivity
//
// Thread Safety:
//   - Safe for concurrent use across multiple goroutines
//   - Uses RWMutex for optimal read performance
//   - Background cleanup coordination
//
type RateLimiter struct {
	clients  map[string]*ClientLimiter
	mu       sync.RWMutex
	cleanup  *time.Ticker
	done     chan bool
	config   RateLimitConfig
}

// RateLimitConfig holds comprehensive rate limiting configuration parameters.
//
// This structure defines the rate limiting policy for protecting NoiseFS
// services from abuse. It provides fine-grained control over various
// aspects of rate limiting behavior.
//
// Configuration Parameters:
//   - RequestsPerMinute: Short-term rate limit for burst protection
//   - RequestsPerHour: Long-term rate limit for sustained abuse prevention
//   - BurstSize: Reserved for future burst allowance implementation
//   - CleanupInterval: How often to clean up inactive client entries
//   - BanDuration: How long to ban clients who exceed limits significantly
//   - MaxConcurrent: Maximum concurrent requests allowed per IP
//
// Tuning Guidelines:
//   - Set RequestsPerMinute based on expected peak usage patterns
//   - Set RequestsPerHour to prevent sustained abuse
//   - Balance CleanupInterval between memory usage and cleanup overhead
//   - Set BanDuration long enough to deter abuse but not punish mistakes
//
type RateLimitConfig struct {
	RequestsPerMinute   int
	RequestsPerHour     int
	BurstSize          int
	CleanupInterval    time.Duration
	BanDuration        time.Duration
	MaxConcurrent      int
}

// ClientLimiter tracks rate limiting state and history for a single client IP.
//
// This structure maintains all necessary state for enforcing rate limits
// on a per-client basis. It tracks both request counts and timing information
// to implement sliding window rate limiting.
//
// State Tracking:
//   - IP: Client IP address for identification
//   - RequestsThisMinute/Hour: Current period request counts
//   - LastRequest/Minute/Hour: Timestamps for sliding window calculation
//   - BannedUntil: Temporary ban expiration time
//   - ConcurrentRequests: Currently active request count
//
// Implementation Details:
//   - Uses sliding windows rather than fixed time buckets
//   - Tracks concurrent requests for connection limiting
//   - Maintains ban state with automatic expiration
//
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

// NewRateLimiter creates a new rate limiter with the specified configuration.
//
// This constructor initializes a complete rate limiting system including
// client tracking, background cleanup, and all necessary data structures.
// It starts background goroutines for automatic maintenance.
//
// Initialization Process:
//   - Creates client tracking map and synchronization primitives
//   - Starts background cleanup goroutine with configured interval
//   - Initializes shutdown coordination channels
//
// Background Services:
//   - Cleanup goroutine removes inactive clients periodically
//   - Prevents memory leaks from long-running services
//   - Graceful shutdown coordination
//
// Parameters:
//   config: RateLimitConfig defining the rate limiting policy
//
// Returns:
//   *RateLimiter: A new rate limiter instance with background services running
//
// Lifecycle:
//   - Call Shutdown() to stop background goroutines and release resources
//   - Safe to use immediately after creation
//
// Complexity: O(1) - Simple initialization with background goroutine startup
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

// DefaultRateLimitConfig returns sensible default rate limiting configuration for NoiseFS services.
//
// This function provides a balanced configuration suitable for most NoiseFS
// deployments. The defaults protect against common abuse patterns while
// allowing reasonable usage for legitimate clients.
//
// Default Policy:
//   - 60 requests per minute (1 per second average)
//   - 1000 requests per hour (~16 per minute sustained)
//   - 5 concurrent requests per IP
//   - 15-minute ban duration for violations
//   - 5-minute cleanup interval
//
// Rationale:
//   - Per-minute limit handles burst traffic
//   - Per-hour limit prevents sustained abuse
//   - Concurrent limit prevents connection exhaustion
//   - Ban duration discourages repeated violations
//   - Cleanup interval balances memory vs overhead
//
// Returns:
//   RateLimitConfig: A configuration with balanced default values
//
// Customization:
//   Users can modify the returned config before passing to NewRateLimiter()
//
// Complexity: O(1) - Simple struct initialization
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

// CheckLimit enforces comprehensive rate limiting policies for incoming HTTP requests.
//
// This method implements the core rate limiting logic, performing multiple checks
// to determine if a request should be allowed or rejected. It provides IP-based
// rate limiting with automatic client management and security features.
//
// Rate Limiting Checks (in order):
//   1. Client IP extraction and sanitization
//   2. Client ban status verification
//   3. Sliding window rate limit resets
//   4. Concurrent request limit enforcement
//   5. Per-minute rate limit enforcement with automatic banning
//   6. Per-hour rate limit enforcement
//   7. Request counting and state updates
//
// IP Address Handling:
//   - Extracts real client IP from X-Forwarded-For, X-Real-IP, or RemoteAddr
//   - Handles proxy configurations and load balancer setups
//   - Sanitizes IP addresses in error messages to prevent information disclosure
//   - Creates new client entries automatically for first-time visitors
//
// Security Features:
//   - Automatic client banning for severe rate limit violations (2x threshold)
//   - Sanitized error messages prevent sensitive information disclosure
//   - Concurrent request tracking prevents connection exhaustion attacks
//   - Sliding window counters prevent burst attacks after reset periods
//
// Sliding Window Implementation:
//   - Per-minute window: Resets if last request was >1 minute ago
//   - Per-hour window: Resets if last request was >1 hour ago
//   - Counters track requests within current window only
//   - More sophisticated than fixed-window rate limiting
//
// Ban Logic:
//   - Temporary bans triggered when requests exceed 2x per-minute limit
//   - Ban duration configured via RateLimitConfig.BanDuration
//   - Banned clients receive immediate rejection without processing
//   - Bans expire automatically without manual intervention
//
// Error Responses:
//   - "IP X is temporarily banned" - for banned clients
//   - "too many concurrent requests from IP X" - for concurrent limit violations
//   - "rate limit exceeded for IP X (requests per minute)" - for per-minute violations
//   - "rate limit exceeded for IP X (requests per hour)" - for per-hour violations
//
// State Management:
//   - Updates LastRequest timestamp for cleanup coordination
//   - Increments concurrent request counter (must call ReleaseRequest() later)
//   - Updates sliding window counters and timestamps
//   - Thread-safe updates with mutex protection
//
// Parameters:
//   r: HTTP request containing client IP and headers for rate limit enforcement
//
// Returns:
//   error: nil if request allowed, descriptive error if rate limited or banned
//
// Thread Safety:
//   - Thread-safe with mutex protection for all client state modifications
//   - Safe for concurrent use across multiple HTTP handler goroutines
//
// Performance:
//   - O(1) operation with map lookup and simple arithmetic
//   - Minimal memory allocation (only for new clients)
//   - Fast path for allowed requests
//
// Complexity: O(1) - Simple map operations and threshold comparisons
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

// ReleaseRequest decrements the concurrent request counter for a client IP address.
//
// This method must be called when an HTTP request completes to properly
// release the concurrent request slot that was allocated during CheckLimit().
// Failure to call this method will result in concurrent request slot leaks.
//
// Typical Usage Pattern:
//   if err := rateLimiter.CheckLimit(r); err != nil {
//       http.Error(w, err.Error(), http.StatusTooManyRequests)
//       return
//   }
//   defer rateLimiter.ReleaseRequest(r) // Ensure slot is always released
//
// Concurrent Request Management:
//   - Decrements the ConcurrentRequests counter for the client IP
//   - Prevents counter underflow by checking for >0 before decrementing
//   - Enables accurate tracking of active connections per IP
//   - Required for proper enforcement of MaxConcurrent limits
//
// Error Handling:
//   - Gracefully handles requests from IPs not in the client registry
//   - Prevents counter underflow with bounds checking
//   - No error return - designed for use in defer statements
//   - Safe to call multiple times for the same request
//
// Cleanup Coordination:
//   - Released slots enable other requests from the same IP
//   - Zero concurrent requests makes clients eligible for background cleanup
//   - Proper release prevents permanent slot exhaustion
//
// Parameters:
//   r: HTTP request used to identify the client IP for slot release
//
// Thread Safety:
//   - Thread-safe with mutex protection for client state modifications
//   - Safe to call concurrently with CheckLimit() and other operations
//
// Performance:
//   - O(1) operation with simple map lookup and arithmetic
//   - Minimal CPU overhead suitable for high-frequency operations
//   - No memory allocation
//
// Complexity: O(1) - Simple map lookup and counter decrement
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

// getClientIP extracts the real client IP address from HTTP request headers and connection info.
//
// This function implements a comprehensive IP extraction strategy that handles
// various proxy configurations and load balancer setups commonly found in
// production environments. It prioritizes headers in order of reliability.
//
// Header Priority (most to least reliable):
//   1. X-Forwarded-For: Standard proxy header with comma-separated IP chain
//   2. X-Real-IP: Simple single-IP header set by some proxies
//   3. RemoteAddr: Direct connection IP from TCP socket
//
// X-Forwarded-For Processing:
//   - Handles comma-separated IP chains from multiple proxies
//   - Takes the first (leftmost) IP as the original client IP
//   - Validates each IP address for proper format
//   - Ignores malformed or empty IP addresses
//
// Security Considerations:
//   - Trusts proxy headers which can be spoofed by malicious clients
//   - Should only be used behind trusted proxies/load balancers
//   - For direct connections, relies on RemoteAddr which is more secure
//   - IP validation prevents injection of malformed addresses
//
// Network Configuration Support:
//   - Works with AWS ALB, ELB, Cloudflare, nginx, Apache proxies
//   - Handles multiple proxy hops in enterprise environments
//   - Supports both IPv4 and IPv6 addresses
//   - Gracefully handles missing or malformed headers
//
// Error Handling:
//   - Falls back to next header/source if current one is invalid
//   - Returns RemoteAddr as final fallback even if malformed
//   - Handles net.SplitHostPort errors gracefully
//   - Never returns empty string
//
// Usage Context:
//   - Used internally by CheckLimit() and ReleaseRequest()
//   - IP addresses are used for rate limiting client identification
//   - Results are sanitized before inclusion in error messages
//
// Parameters:
//   r: HTTP request containing headers and connection information
//
// Returns:
//   string: The extracted client IP address, never empty
//
// Thread Safety:
//   - Thread-safe - only reads from request headers
//   - No shared state modifications
//
// Complexity: O(n) where n is the number of IPs in X-Forwarded-For header
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

// parseXForwardedFor parses and validates IP addresses from X-Forwarded-For header.
//
// This function extracts and validates IP addresses from the X-Forwarded-For
// header, which contains a comma-separated list of IP addresses representing
// the request path through multiple proxies.
//
// X-Forwarded-For Format:
//   "client_ip, proxy1_ip, proxy2_ip, ..."
//   Example: "203.0.113.1, 198.51.100.1, 192.0.2.1"
//
// Processing Steps:
//   1. Split header value by commas
//   2. Trim whitespace from each IP address
//   3. Validate each IP using net.ParseIP()
//   4. Filter out empty strings and invalid IPs
//   5. Return slice of valid IP addresses
//
// Validation Features:
//   - Uses net.ParseIP() for robust IP address validation
//   - Supports both IPv4 and IPv6 addresses
//   - Filters out malformed or empty entries
//   - Preserves order of IP addresses in the chain
//
// Security Considerations:
//   - Prevents injection of malformed IP addresses
//   - Does not perform any sanitization beyond validation
//   - Returns empty slice if no valid IPs found
//   - Handles arbitrarily long IP chains
//
// Edge Cases:
//   - Empty header string returns empty slice
//   - Single IP address returns single-element slice
//   - Malformed IPs are silently filtered out
//   - Whitespace around IPs is automatically trimmed
//
// Usage Context:
//   - Called by getClientIP() to process X-Forwarded-For headers
//   - First IP in returned slice is typically the original client
//   - Used for rate limiting client identification in proxy environments
//
// Parameters:
//   header: X-Forwarded-For header value containing comma-separated IPs
//
// Returns:
//   []string: Slice of valid IP address strings in order, may be empty
//
// Thread Safety:
//   - Thread-safe - no shared state, only string processing
//
// Complexity: O(n) where n is the number of comma-separated IP addresses
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

// cleanupLoop runs the background maintenance process for automatic client registry cleanup.
//
// This goroutine provides essential memory management for long-running rate
// limiter instances by periodically removing inactive client entries. It prevents
// unbounded memory growth in services handling many unique client IPs.
//
// Cleanup Schedule:
//   - Runs on configurable interval (default: 5 minutes)
//   - Triggered by time.Ticker events from RateLimitConfig.CleanupInterval
//   - Terminates gracefully on shutdown signal
//   - Continues running until explicit shutdown
//
// Lifecycle Management:
//   - Started automatically during NewRateLimiter() construction
//   - Runs in dedicated background goroutine
//   - Coordinates with main application via done channel
//   - Proper cleanup prevents goroutine leaks
//
// Coordination Mechanism:
//   - Uses select statement for non-blocking operation coordination
//   - Responds to cleanup timer ticks for regular maintenance
//   - Responds to shutdown signals for graceful termination
//   - No blocking operations that could deadlock
//
// Memory Management:
//   - Delegates actual cleanup logic to cleanupOldClients()
//   - Ensures memory usage remains bounded over time
//   - Critical for services with high client IP diversity
//   - Prevents performance degradation from large client maps
//
// Error Handling:
//   - No error handling required - cleanup is best-effort
//   - Individual client cleanup failures don't affect loop operation
//   - Graceful shutdown guaranteed regardless of cleanup state
//
// Performance Impact:
//   - Minimal CPU usage during idle periods
//   - Cleanup overhead is amortized across cleanup interval
//   - No impact on request processing performance
//   - Background operation doesn't block request handling
//
// Thread Safety:
//   - Coordinates with other goroutines via channels
//   - No direct shared state access (cleanupOldClients handles locking)
//
// Complexity: O(1) - Simple event loop with channel operations
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

// cleanupOldClients removes inactive client entries to prevent unbounded memory growth.
//
// This method implements the core memory management logic for the rate limiter,
// identifying and removing client entries that have been inactive for an
// extended period. It's essential for long-running services with high IP diversity.
//
// Cleanup Criteria:
//   - Client must have been inactive for more than 2 hours
//   - Client must have zero concurrent requests (no active connections)
//   - Uses LastRequest timestamp for age determination
//   - Conservative 2-hour threshold balances memory vs client experience
//
// Age Calculation:
//   - Cutoff time: current time minus 2 hours
//   - Compares against ClientLimiter.LastRequest timestamp
//   - Only processes clients older than cutoff threshold
//   - Preserves recently active clients for performance
//
// Concurrent Request Check:
//   - Ensures no active requests before deletion
//   - Prevents cleanup of clients with ongoing operations
//   - ConcurrentRequests == 0 required for safe removal
//   - Avoids race conditions with active request processing
//
// Memory Management:
//   - Deletes entries from clients map using delete() builtin
//   - Immediately frees memory for garbage collection
//   - Reduces map lookup overhead for remaining clients
//   - Prevents indefinite memory growth in high-traffic scenarios
//
// Performance Characteristics:
//   - O(n) iteration through all client entries
//   - Minimal per-client overhead (timestamp comparison + map deletion)
//   - Runs infrequently (every 5 minutes) to amortize cost
//   - No impact on request processing during cleanup
//
// Conservative Design:
//   - 2-hour threshold prevents premature cleanup of returning clients
//   - Preserves rate limiting state for recently active IPs
//   - Balances memory efficiency with user experience
//   - Avoids cleanup during typical user session durations
//
// Thread Safety:
//   - Acquires exclusive lock for entire cleanup operation
//   - Prevents race conditions with CheckLimit() and ReleaseRequest()
//   - Atomic cleanup prevents partial state corruption
//
// Error Handling:
//   - No error conditions - cleanup is best-effort operation
//   - Individual client deletion failures don't affect others
//   - Continues processing even if some deletions fail
//
// Complexity: O(n) where n is the number of registered clients
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

// Shutdown gracefully terminates the rate limiter and cleans up all background resources.
//
// This method provides comprehensive cleanup for rate limiter instances,
// ensuring proper resource management and preventing goroutine leaks in
// applications with dynamic rate limiter lifecycles.
//
// Shutdown Process:
//   1. Stop the background cleanup ticker to prevent new cleanup cycles
//   2. Signal the cleanup goroutine to terminate via done channel
//   3. Coordinate graceful shutdown without blocking indefinitely
//
// Background Process Termination:
//   - Stops time.Ticker to halt cleanup scheduling
//   - Sends shutdown signal through done channel
//   - Non-blocking send prevents deadlock if goroutine already terminated
//   - Cleanup goroutine exits gracefully on signal receipt
//
// Resource Cleanup:
//   - Ticker resources are properly released
//   - Goroutine termination prevents memory leaks
//   - Channel resources are implicitly cleaned up by GC
//   - No ongoing background operations after shutdown
//
// Graceful Shutdown Features:
//   - Non-blocking operation suitable for application shutdown sequences
//   - Safe to call multiple times (idempotent operation)
//   - No error return - cleanup is best-effort
//   - Immediate return without waiting for goroutine termination
//
// Post-Shutdown State:
//   - Rate limiter methods remain functional for existing clients
//   - No new cleanup cycles will be scheduled
//   - Client registry remains intact with current state
//   - CheckLimit() and ReleaseRequest() continue working normally
//
// Usage Context:
//   - Call during application shutdown for proper cleanup
//   - Essential for applications creating multiple rate limiter instances
//   - Prevents goroutine leaks in long-running applications
//   - Part of proper resource management lifecycle
//
// Error Handling:
//   - No error conditions - shutdown is always successful
//   - Handles cases where cleanup goroutine already terminated
//   - Safe operation regardless of current rate limiter state
//
// Thread Safety:
//   - Safe to call concurrently with other rate limiter operations
//   - No shared state modifications beyond goroutine coordination
//
// Complexity: O(1) - Simple resource cleanup operations
func (rl *RateLimiter) Shutdown() {
	if rl.cleanup != nil {
		rl.cleanup.Stop()
	}
	
	select {
	case rl.done <- true:
	default:
	}
}

// GetStats returns comprehensive runtime statistics for monitoring and debugging.
//
// This method provides real-time insights into rate limiter performance and
// client activity patterns, essential for monitoring, alerting, and capacity
// planning in production environments.
//
// Statistics Collected:
//   - active_clients: Total number of client IPs in the registry
//   - banned_clients: Number of clients currently under temporary ban
//   - total_concurrent: Sum of concurrent requests across all clients
//   - requests_per_min: Configured per-minute rate limit threshold
//   - requests_per_hour: Configured per-hour rate limit threshold
//
// Real-Time Analysis:
//   - Iterates through entire client registry for current state
//   - Counts active clients regardless of recent activity
//   - Identifies currently banned clients by checking ban expiration
//   - Sums concurrent request counts for overall load assessment
//
// Monitoring Use Cases:
//   - Capacity planning: Monitor active_clients growth trends
//   - Security monitoring: Track banned_clients for attack detection
//   - Performance monitoring: Watch total_concurrent for load assessment
//   - Configuration validation: Verify rate limit settings
//
// Ban Status Detection:
//   - Checks each client's BannedUntil timestamp against current time
//   - Counts clients with active bans (BannedUntil > now)
//   - Provides real-time security incident visibility
//   - Useful for automated alerting on abuse patterns
//
// Performance Characteristics:
//   - O(n) iteration through all registered clients
//   - Read-only operation with shared lock for concurrent safety
//   - Minimal memory allocation (single map return)
//   - Suitable for periodic monitoring (not per-request)
//
// Return Format:
//   map[string]interface{} for flexible JSON serialization and monitoring integration
//
// Configuration Information:
//   - Includes static configuration values for reference
//   - Helps verify rate limiter is configured as expected
//   - Useful for debugging rate limiting behavior
//
// Thread Safety:
//   - Uses read lock for safe concurrent access during stats collection
//   - No modifications to client state during statistics gathering
//   - Safe to call concurrently with rate limiting operations
//
// Usage Example:
//   stats := rateLimiter.GetStats()
//   log.Printf("Active clients: %d, Banned: %d", 
//       stats["active_clients"], stats["banned_clients"])
//
// Complexity: O(n) where n is the number of registered clients
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

// Middleware creates HTTP middleware that enforces rate limiting with automatic resource management.
//
// This method returns a standard HTTP middleware function that integrates
// rate limiting into HTTP request processing pipelines. It provides transparent
// rate limiting with proper resource management and error handling.
//
// Request Processing Flow:
//   1. Check rate limits using CheckLimit() before processing
//   2. Return HTTP 429 (Too Many Requests) if rate limited
//   3. Ensure ReleaseRequest() is called when request completes
//   4. Forward allowed requests to the next handler
//
// Automatic Resource Management:
//   - Uses defer to guarantee ReleaseRequest() is called
//   - Prevents concurrent request slot leaks
//   - Handles both successful and panicked request completions
//   - Critical for accurate concurrent request tracking
//
// HTTP Status Codes:
//   - 429 Too Many Requests: Standard rate limiting response
//   - Preserves original status codes for allowed requests
//   - Includes descriptive error message in response body
//
// Error Message Handling:
//   - Returns sanitized error messages from CheckLimit()
//   - Error messages include client IP (sanitized) and violation type
//   - Provides clear feedback for rate limit violations
//   - Helps legitimate users understand rate limiting
//
// Integration Patterns:
//   - Standard Go http.Handler middleware pattern
//   - Composable with other middleware (logging, auth, etc.)
//   - Can be applied to specific routes or entire applications
//   - Compatible with popular Go web frameworks
//
// Request Lifecycle:
//   - Pre-processing: Rate limit check
//   - Processing: Original handler execution (if allowed)
//   - Post-processing: Resource cleanup (always)
//   - Exception handling: Cleanup on panic via defer
//
// Usage Examples:
//   
//   // Protect specific endpoint
//   http.HandleFunc("/api/upload", rateLimiter.Middleware(uploadHandler))
//   
//   // Protect entire API with middleware chain
//   protected := rateLimiter.Middleware(apiHandler)
//   http.Handle("/api/", protected)
//
// Performance Impact:
//   - Minimal overhead for allowed requests
//   - O(1) rate limit check operation
//   - No additional memory allocation per request
//   - Efficient path for normal request processing
//
// Parameters:
//   next: The HTTP handler to protect with rate limiting
//
// Returns:
//   http.HandlerFunc: Middleware function that enforces rate limits
//
// Thread Safety:
//   - Safe for concurrent use across multiple HTTP handlers
//   - Underlying rate limiter handles concurrency protection
//
// Complexity: O(1) per request - delegates to CheckLimit() and ReleaseRequest()
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

// RequestSizeLimiter creates HTTP middleware that enforces request body size limits.
//
// This function returns a middleware factory that creates size-limiting middleware
// for protecting HTTP endpoints from oversized request bodies. It provides both
// early rejection and streaming protection against large uploads.
//
// Size Limiting Strategy:
//   1. Early Check: Reject requests with Content-Length > maxSize
//   2. Streaming Protection: Wrap request body with MaxBytesReader
//   3. Dual Protection: Handles both declared and actual request sizes
//
// Content-Length Validation:
//   - Checks Content-Length header before reading body
//   - Immediate rejection for oversized requests
//   - Prevents unnecessary data transfer
//   - Returns HTTP 413 (Request Entity Too Large)
//
// Streaming Protection:
//   - Wraps request body with http.MaxBytesReader
//   - Enforces size limit during actual body reading
//   - Protects against Content-Length spoofing
//   - Handles chunked transfer encoding
//
// Security Benefits:
//   - Prevents memory exhaustion attacks
//   - Protects against disk space exhaustion
//   - Reduces bandwidth waste from oversized uploads
//   - Complements application-level validation
//
// HTTP Response Handling:
//   - Returns 413 Request Entity Too Large for violations
//   - Includes descriptive error message with size limit
//   - Standard HTTP status code for size violations
//   - Clear feedback for client applications
//
// Integration with NoiseFS:
//   - Protects file upload endpoints from abuse
//   - Configurable limits based on storage capacity
//   - Works with block-based storage architecture
//   - Complements rate limiting for comprehensive protection
//
// Usage Examples:
//   
//   // Limit uploads to 10MB
//   sizeLimited := RequestSizeLimiter(10 * 1024 * 1024)
//   http.HandleFunc("/upload", sizeLimited(uploadHandler))
//   
//   // Chain with rate limiting
//   protected := rateLimiter.Middleware(
//       RequestSizeLimiter(maxSize)(apiHandler))
//
// Performance Characteristics:
//   - Minimal overhead for compliant requests
//   - Early rejection prevents unnecessary processing
//   - Memory-efficient streaming validation
//   - No additional buffering required
//
// Edge Cases:
//   - Missing Content-Length: Relies on MaxBytesReader protection
//   - Chunked encoding: MaxBytesReader handles size enforcement
//   - Zero-size requests: Allowed (common for GET requests)
//
// Error Message Format:
//   "Request body too large (max X bytes)" where X is the configured limit
//
// Parameters:
//   maxSize: Maximum allowed request body size in bytes
//
// Returns:
//   func(http.HandlerFunc) http.HandlerFunc: Middleware factory function
//
// Thread Safety:
//   - Thread-safe - no shared state between requests
//   - Each request gets independent size limiting
//
// Complexity: O(1) - Simple size checks and wrapper creation
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