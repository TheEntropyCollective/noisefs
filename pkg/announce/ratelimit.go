package announce

import (
	"fmt"
	"sync"
	"time"
)

// Rate limiting constants
const (
	// Default rate limits
	DefaultMaxPerMinute = 10  // 10 announcements per minute
	DefaultMaxPerHour   = 100 // 100 per hour
	DefaultMaxPerDay    = 500 // 500 per day
	DefaultBurstSize    = 5   // Allow burst of 5
	
	// Cleanup timing
	DefaultCleanupInterval = 1 * time.Hour
	CleanupCutoffHours     = 24 // Remove records not seen in 24 hours
)

// RateLimiter provides rate limiting for announcements
type RateLimiter struct {
	// Configuration
	maxPerMinute    int
	maxPerHour      int
	maxPerDay       int
	burstSize       int
	cleanupInterval time.Duration
	
	// Tracking
	records map[string]*rateLimitRecord
	mu      sync.RWMutex
	
	// Cleanup
	stopCleanup chan struct{}
	wg          sync.WaitGroup
}

// rateLimitRecord tracks rate limit data for a key
type rateLimitRecord struct {
	minuteBucket  *timeBucket
	hourBucket    *timeBucket
	dayBucket     *timeBucket
	lastSeen      time.Time
}

// timeBucket tracks events in a time window
type timeBucket struct {
	count      int
	windowStart time.Time
	duration   time.Duration
}

// RateLimitConfig holds rate limiter configuration
type RateLimitConfig struct {
	MaxPerMinute    int
	MaxPerHour      int
	MaxPerDay       int
	BurstSize       int
	CleanupInterval time.Duration
}

// DefaultRateLimitConfig returns default rate limit configuration
func DefaultRateLimitConfig() *RateLimitConfig {
	return &RateLimitConfig{
		MaxPerMinute:    DefaultMaxPerMinute,
		MaxPerHour:      DefaultMaxPerHour,
		MaxPerDay:       DefaultMaxPerDay,
		BurstSize:       DefaultBurstSize,
		CleanupInterval: DefaultCleanupInterval,
	}
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(config *RateLimitConfig) *RateLimiter {
	if config == nil {
		config = DefaultRateLimitConfig()
	}
	
	rl := &RateLimiter{
		maxPerMinute:    config.MaxPerMinute,
		maxPerHour:      config.MaxPerHour,
		maxPerDay:       config.MaxPerDay,
		burstSize:       config.BurstSize,
		cleanupInterval: config.CleanupInterval,
		records:         make(map[string]*rateLimitRecord),
		stopCleanup:     make(chan struct{}),
	}
	
	// Start cleanup routine
	rl.wg.Add(1)
	go rl.cleanupLoop()
	
	return rl
}

// CheckLimit checks if an action is allowed for the given key
func (rl *RateLimiter) CheckLimit(key string) error {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	now := time.Now()
	
	// Get or create record
	record, exists := rl.records[key]
	if !exists {
		record = &rateLimitRecord{
			minuteBucket: &timeBucket{
				count:       0,
				windowStart: now,
				duration:    time.Minute,
			},
			hourBucket: &timeBucket{
				count:       0,
				windowStart: now,
				duration:    time.Hour,
			},
			dayBucket: &timeBucket{
				count:       0,
				windowStart: now,
				duration:    24 * time.Hour,
			},
			lastSeen: now,
		}
		rl.records[key] = record
	}
	
	// Update buckets
	record.minuteBucket.update(now)
	record.hourBucket.update(now)
	record.dayBucket.update(now)
	
	// Check burst limit (minute bucket)
	if record.minuteBucket.count >= rl.burstSize {
		timeUntilReset := record.minuteBucket.windowStart.Add(time.Minute).Sub(now)
		return fmt.Errorf("rate limit exceeded: burst limit reached, retry in %s", timeUntilReset.Round(time.Second))
	}
	
	// Check minute limit
	if record.minuteBucket.count >= rl.maxPerMinute {
		timeUntilReset := record.minuteBucket.windowStart.Add(time.Minute).Sub(now)
		return fmt.Errorf("rate limit exceeded: minute limit reached, retry in %s", timeUntilReset.Round(time.Second))
	}
	
	// Check hour limit
	if record.hourBucket.count >= rl.maxPerHour {
		timeUntilReset := record.hourBucket.windowStart.Add(time.Hour).Sub(now)
		return fmt.Errorf("rate limit exceeded: hour limit reached, retry in %s", timeUntilReset.Round(time.Minute))
	}
	
	// Check day limit
	if record.dayBucket.count >= rl.maxPerDay {
		timeUntilReset := record.dayBucket.windowStart.Add(24 * time.Hour).Sub(now)
		return fmt.Errorf("rate limit exceeded: daily limit reached, retry in %s", timeUntilReset.Round(time.Hour))
	}
	
	// Increment counters
	record.minuteBucket.count++
	record.hourBucket.count++
	record.dayBucket.count++
	record.lastSeen = now
	
	return nil
}

// GetStatus returns current rate limit status for a key
func (rl *RateLimiter) GetStatus(key string) RateLimitStatus {
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	
	status := RateLimitStatus{
		Key: key,
	}
	
	record, exists := rl.records[key]
	if !exists {
		status.MinuteRemaining = rl.maxPerMinute
		status.HourRemaining = rl.maxPerHour
		status.DayRemaining = rl.maxPerDay
		return status
	}
	
	now := time.Now()
	
	// Update buckets for accurate count
	record.minuteBucket.update(now)
	record.hourBucket.update(now)
	record.dayBucket.update(now)
	
	status.MinuteCount = record.minuteBucket.count
	status.MinuteRemaining = rl.maxPerMinute - record.minuteBucket.count
	status.MinuteReset = record.minuteBucket.windowStart.Add(time.Minute)
	
	status.HourCount = record.hourBucket.count
	status.HourRemaining = rl.maxPerHour - record.hourBucket.count
	status.HourReset = record.hourBucket.windowStart.Add(time.Hour)
	
	status.DayCount = record.dayBucket.count
	status.DayRemaining = rl.maxPerDay - record.dayBucket.count
	status.DayReset = record.dayBucket.windowStart.Add(24 * time.Hour)
	
	return status
}

// Reset resets rate limit for a key
func (rl *RateLimiter) Reset(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	delete(rl.records, key)
}

// Close stops the rate limiter
func (rl *RateLimiter) Close() {
	close(rl.stopCleanup)
	rl.wg.Wait()
}

// cleanupLoop periodically removes old records
func (rl *RateLimiter) cleanupLoop() {
	defer rl.wg.Done()
	
	ticker := time.NewTicker(rl.cleanupInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-rl.stopCleanup:
			return
		case <-ticker.C:
			rl.cleanup()
		}
	}
}

// cleanup removes stale records
func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	now := time.Now()
	cutoff := now.Add(-CleanupCutoffHours * time.Hour) // Remove records not seen in 24 hours
	
	for key, record := range rl.records {
		if record.lastSeen.Before(cutoff) {
			delete(rl.records, key)
		}
	}
}

// update refreshes the bucket if the window has expired
func (b *timeBucket) update(now time.Time) {
	if now.Sub(b.windowStart) >= b.duration {
		// Window has expired, reset
		b.count = 0
		b.windowStart = now
	}
}

// RateLimitStatus represents current rate limit status
type RateLimitStatus struct {
	Key             string
	MinuteCount     int
	MinuteRemaining int
	MinuteReset     time.Time
	HourCount       int
	HourRemaining   int
	HourReset       time.Time
	DayCount        int
	DayRemaining    int
	DayReset        time.Time
}

// RateLimitKey generates a rate limit key from various sources
func RateLimitKey(source string, identifier string) string {
	return fmt.Sprintf("%s:%s", source, identifier)
}