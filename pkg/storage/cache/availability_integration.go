package cache

import (
	"context"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/storage"
)

// AvailabilityIntegration provides availability checking by leveraging 
// the existing storage manager's health monitoring capabilities
type AvailabilityIntegration struct {
	mu             sync.RWMutex
	storageManager *storage.Manager
	cache          *AvailabilityCache
	config         *AvailabilityConfig
}

// AvailabilityConfig holds configuration for availability checking
type AvailabilityConfig struct {
	// Cache TTL for availability results
	CacheTTL time.Duration `json:"cache_ttl"`
	
	// Timeout for availability checks
	CheckTimeout time.Duration `json:"check_timeout"`
	
	// Background refresh interval
	RefreshInterval time.Duration `json:"refresh_interval"`
	
	// Maximum concurrent checks
	MaxConcurrentChecks int `json:"max_concurrent_checks"`
	
	// Minimum availability threshold for health scoring
	MinAvailabilityThreshold float64 `json:"min_availability_threshold"`
}

// AvailabilityStatus represents the availability status of a randomizer
type AvailabilityStatus struct {
	CID               string        `json:"cid"`
	Available         bool          `json:"available"`
	LastChecked       time.Time     `json:"last_checked"`
	CheckDuration     time.Duration `json:"check_duration"`
	HealthyBackends   int           `json:"healthy_backends"`
	TotalBackends     int           `json:"total_backends"`
	AvailabilityScore float64       `json:"availability_score"` // 0-1
	Error             string        `json:"error,omitempty"`
}

// AvailabilityCache caches availability results with TTL
type AvailabilityCache struct {
	mu      sync.RWMutex
	entries map[string]*AvailabilityStatus
	ttl     time.Duration
}

// AvailabilityMetrics provides aggregated availability metrics
type AvailabilityMetrics struct {
	TotalRandomizers     int64   `json:"total_randomizers"`
	AvailableRandomizers int64   `json:"available_randomizers"`
	OverallAvailability  float64 `json:"overall_availability"`
	AverageCheckDuration time.Duration `json:"average_check_duration"`
	LastUpdateTime       time.Time `json:"last_update_time"`
	BackendHealth        map[string]float64 `json:"backend_health"`
}

// DefaultAvailabilityConfig returns default configuration
func DefaultAvailabilityConfig() *AvailabilityConfig {
	return &AvailabilityConfig{
		CacheTTL:                 5 * time.Minute,
		CheckTimeout:             10 * time.Second,
		RefreshInterval:          2 * time.Minute,
		MaxConcurrentChecks:      10,
		MinAvailabilityThreshold: 0.5,
	}
}

// NewAvailabilityIntegration creates a new availability integration
func NewAvailabilityIntegration(storageManager *storage.Manager, config *AvailabilityConfig) *AvailabilityIntegration {
	if config == nil {
		config = DefaultAvailabilityConfig()
	}
	
	return &AvailabilityIntegration{
		storageManager: storageManager,
		cache:          NewAvailabilityCache(config.CacheTTL),
		config:         config,
	}
}

// CheckAvailability checks if the specified randomizers are available
func (ai *AvailabilityIntegration) CheckAvailability(ctx context.Context, cids []string) map[string]*AvailabilityStatus {
	result := make(map[string]*AvailabilityStatus)
	
	// Check cache first
	uncachedCIDs := make([]string, 0)
	for _, cid := range cids {
		if status := ai.cache.Get(cid); status != nil {
			result[cid] = status
		} else {
			uncachedCIDs = append(uncachedCIDs, cid)
		}
	}
	
	// If all results are cached, return immediately
	if len(uncachedCIDs) == 0 {
		return result
	}
	
	// Check uncached CIDs using concurrent validation
	freshResults := ai.checkAvailabilityConcurrent(ctx, uncachedCIDs)
	
	// Merge results and update cache
	for cid, status := range freshResults {
		result[cid] = status
		ai.cache.Set(cid, status)
	}
	
	return result
}

// checkAvailabilityConcurrent performs concurrent availability checks
func (ai *AvailabilityIntegration) checkAvailabilityConcurrent(ctx context.Context, cids []string) map[string]*AvailabilityStatus {
	result := make(map[string]*AvailabilityStatus)
	resultMu := sync.Mutex{}
	
	// Create semaphore to limit concurrent checks
	sem := make(chan struct{}, ai.config.MaxConcurrentChecks)
	var wg sync.WaitGroup
	
	for _, cid := range cids {
		wg.Add(1)
		go func(cid string) {
			defer wg.Done()
			
			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()
			
			// Check availability for this CID
			status := ai.checkSingleAvailability(ctx, cid)
			
			// Store result
			resultMu.Lock()
			result[cid] = status
			resultMu.Unlock()
		}(cid)
	}
	
	wg.Wait()
	return result
}

// checkSingleAvailability checks availability for a single CID
func (ai *AvailabilityIntegration) checkSingleAvailability(ctx context.Context, cid string) *AvailabilityStatus {
	startTime := time.Now()
	
	// Create timeout context
	checkCtx, cancel := context.WithTimeout(ctx, ai.config.CheckTimeout)
	defer cancel()
	
	status := &AvailabilityStatus{
		CID:         cid,
		LastChecked: startTime,
	}
	
	// Create block address for checking
	address := &storage.BlockAddress{
		ID:          cid,
		BackendType: storage.BackendTypeIPFS, // Default to IPFS
		CreatedAt:   time.Now(),
	}
	
	// Check availability using storage manager
	available, err := ai.storageManager.Has(checkCtx, address)
	if err != nil {
		status.Available = false
		status.Error = err.Error()
		status.AvailabilityScore = 0.0
	} else {
		status.Available = available
		status.AvailabilityScore = ai.calculateAvailabilityScore(available)
	}
	
	// Get backend health information
	status.HealthyBackends, status.TotalBackends = ai.getBackendHealthCounts()
	
	status.CheckDuration = time.Since(startTime)
	
	return status
}

// calculateAvailabilityScore calculates availability score based on backend health
func (ai *AvailabilityIntegration) calculateAvailabilityScore(available bool) float64 {
	if !available {
		return 0.0
	}
	
	// Get backend health information
	healthyBackends, totalBackends := ai.getBackendHealthCounts()
	
	if totalBackends == 0 {
		return 0.0
	}
	
	// Base score from availability
	baseScore := 1.0
	
	// Adjust based on backend health
	backendHealthRatio := float64(healthyBackends) / float64(totalBackends)
	
	// Combine base availability with backend health
	score := baseScore * backendHealthRatio
	
	// Apply minimum threshold
	if score < ai.config.MinAvailabilityThreshold {
		score = score * 0.5 // Reduce score if below threshold
	}
	
	return score
}

// getBackendHealthCounts returns healthy and total backend counts
func (ai *AvailabilityIntegration) getBackendHealthCounts() (healthy, total int) {
	// Get available backends
	availableBackends := ai.storageManager.GetAvailableBackends()
	total = len(availableBackends)
	
	// Count healthy backends
	for _, backend := range availableBackends {
		if backend.IsConnected() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			healthStatus := backend.HealthCheck(ctx)
			cancel()
			
			if healthStatus != nil && healthStatus.Healthy {
				healthy++
			}
		}
	}
	
	return healthy, total
}

// GetAvailabilityMetrics returns aggregated availability metrics
func (ai *AvailabilityIntegration) GetAvailabilityMetrics() *AvailabilityMetrics {
	ai.mu.RLock()
	defer ai.mu.RUnlock()
	
	metrics := &AvailabilityMetrics{
		LastUpdateTime: time.Now(),
		BackendHealth:  make(map[string]float64),
	}
	
	// Get cached entries
	entries := ai.cache.GetAllEntries()
	
	if len(entries) == 0 {
		return metrics
	}
	
	// Calculate aggregate metrics
	var totalCheckDuration time.Duration
	availableCount := int64(0)
	
	for _, status := range entries {
		metrics.TotalRandomizers++
		totalCheckDuration += status.CheckDuration
		
		if status.Available {
			availableCount++
		}
	}
	
	metrics.AvailableRandomizers = availableCount
	metrics.OverallAvailability = float64(availableCount) / float64(metrics.TotalRandomizers)
	metrics.AverageCheckDuration = totalCheckDuration / time.Duration(metrics.TotalRandomizers)
	
	// Get backend health metrics
	healthyBackends, totalBackends := ai.getBackendHealthCounts()
	if totalBackends > 0 {
		metrics.BackendHealth["overall"] = float64(healthyBackends) / float64(totalBackends)
	}
	
	return metrics
}

// GetAvailabilityScore returns the overall availability score for health monitoring
func (ai *AvailabilityIntegration) GetAvailabilityScore() float64 {
	metrics := ai.GetAvailabilityMetrics()
	
	if metrics.TotalRandomizers == 0 {
		return 1.0 // No data yet, assume healthy
	}
	
	// Weight overall availability with backend health
	availabilityScore := metrics.OverallAvailability
	backendHealthScore := metrics.BackendHealth["overall"]
	
	// Combine both scores (weighted average)
	return (availabilityScore * 0.7) + (backendHealthScore * 0.3)
}

// NewAvailabilityCache creates a new availability cache
func NewAvailabilityCache(ttl time.Duration) *AvailabilityCache {
	return &AvailabilityCache{
		entries: make(map[string]*AvailabilityStatus),
		ttl:     ttl,
	}
}

// Get retrieves an availability status from cache if still valid
func (ac *AvailabilityCache) Get(cid string) *AvailabilityStatus {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	
	status, exists := ac.entries[cid]
	if !exists {
		return nil
	}
	
	// Check if entry is still valid
	if time.Since(status.LastChecked) > ac.ttl {
		return nil
	}
	
	return status
}

// Set stores an availability status in cache
func (ac *AvailabilityCache) Set(cid string, status *AvailabilityStatus) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	
	ac.entries[cid] = status
}

// GetAllEntries returns all cached entries (for metrics calculation)
func (ac *AvailabilityCache) GetAllEntries() map[string]*AvailabilityStatus {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	
	result := make(map[string]*AvailabilityStatus)
	
	// Only return non-expired entries
	for cid, status := range ac.entries {
		if time.Since(status.LastChecked) <= ac.ttl {
			result[cid] = status
		}
	}
	
	return result
}

// Cleanup removes expired entries from cache
func (ac *AvailabilityCache) Cleanup() {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	
	now := time.Now()
	for cid, status := range ac.entries {
		if now.Sub(status.LastChecked) > ac.ttl {
			delete(ac.entries, cid)
		}
	}
}