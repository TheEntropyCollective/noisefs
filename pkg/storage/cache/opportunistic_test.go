package cache

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
)

func TestOpportunisticFetcher_BasicOperation(t *testing.T) {
	// Setup
	baseCache := NewMemoryCache(1000)
	altruisticConfig := &AltruisticCacheConfig{
		MinPersonalCache: 50 * 1024,
		EnableAltruistic: true,
		EvictionCooldown: 100 * time.Millisecond,
	}
	cache := NewAltruisticCache(baseCache, altruisticConfig, 100*1024)
	
	healthTracker := NewBlockHealthTracker(nil)
	
	// Mock fetcher
	fetchCount := int32(0)
	fetcher := func(ctx context.Context, cid string) ([]byte, error) {
		atomic.AddInt32(&fetchCount, 1)
		return make([]byte, 1024), nil // 1KB blocks
	}
	
	config := &OpportunisticConfig{
		MinFlexPoolFree: 0.3,
		CheckInterval:   100 * time.Millisecond,
		MaxBlockSize:    10 * 1024,
		ValueThreshold:  1.0,
		BatchSize:       5,
		MaxConcurrent:   2,
	}
	
	of := NewOpportunisticFetcher(cache, healthTracker, fetcher, config)
	
	// Add some valuable blocks to health tracker
	for i := 0; i < 5; i++ {
		hint := BlockHint{
			ReplicationBucket: ReplicationLow,
			HighEntropy:       true,
			Size:              1024,
		}
		healthTracker.UpdateBlockHealth(fmt.Sprintf("block-%d", i), hint)
	}
	
	// Start fetcher
	err := of.Start()
	if err != nil {
		t.Fatalf("Failed to start fetcher: %v", err)
	}
	
	// Let it run
	time.Sleep(500 * time.Millisecond)
	
	// Stop
	of.Stop()
	
	// Check that blocks were fetched
	count := atomic.LoadInt32(&fetchCount)
	if count == 0 {
		t.Error("No blocks were fetched")
	}
	
	// Check stats
	stats := of.GetStats()
	if stats["fetch_count"].(int64) != int64(count) {
		t.Error("Stats don't match fetch count")
	}
}

func TestOpportunisticFetcher_SpaceConstraints(t *testing.T) {
	// Setup small cache that's mostly full
	baseCache := NewMemoryCache(100)
	altruisticConfig := &AltruisticCacheConfig{
		MinPersonalCache: 8 * 1024,
		EnableAltruistic: true,
	}
	cache := NewAltruisticCache(baseCache, altruisticConfig, 10*1024) // 10KB total
	
	// Fill cache to leave only 20% free (below threshold)
	for i := 0; i < 8; i++ {
		data := make([]byte, 1024) // 1KB blocks
		block := &blocks.Block{Data: data}
		cache.StoreWithOrigin(fmt.Sprintf("existing-%d", i), block, PersonalBlock)
	}
	
	healthTracker := NewBlockHealthTracker(nil)
	
	fetchCount := int32(0)
	fetcher := func(ctx context.Context, cid string) ([]byte, error) {
		atomic.AddInt32(&fetchCount, 1)
		return make([]byte, 1024), nil
	}
	
	config := &OpportunisticConfig{
		MinFlexPoolFree: 0.3, // Need 30% free
		CheckInterval:   100 * time.Millisecond,
	}
	
	of := NewOpportunisticFetcher(cache, healthTracker, fetcher, config)
	
	// Add valuable blocks
	healthTracker.UpdateBlockHealth("valuable", BlockHint{
		ReplicationBucket: ReplicationLow,
		HighEntropy:       true,
		Size:              1024,
	})
	
	// Start and run
	of.Start()
	time.Sleep(300 * time.Millisecond)
	of.Stop()
	
	// Should not fetch due to space constraints
	if atomic.LoadInt32(&fetchCount) > 0 {
		t.Error("Should not fetch when flex pool is too full")
	}
}

func TestOpportunisticFetcher_ErrorHandling(t *testing.T) {
	// Setup
	baseCache := NewMemoryCache(1000)
	altruisticConfig := &AltruisticCacheConfig{
		MinPersonalCache: 10 * 1024,
		EnableAltruistic: true,
	}
	cache := NewAltruisticCache(baseCache, altruisticConfig, 100*1024)
	
	healthTracker := NewBlockHealthTracker(nil)
	
	// Fetcher that fails for specific blocks
	fetchAttempts := make(map[string]int)
	fetcher := func(ctx context.Context, cid string) ([]byte, error) {
		fetchAttempts[cid]++
		
		if cid == "error-block" {
			return nil, fmt.Errorf("fetch error")
		}
		
		return make([]byte, 1024), nil
	}
	
	config := &OpportunisticConfig{
		MinFlexPoolFree:  0.1,
		CheckInterval:    100 * time.Millisecond,
		MaxErrorRetries:  3,
		ErrorBackoff:     200 * time.Millisecond,
		FetchCooldown:    100 * time.Millisecond,
		ValueThreshold:   2.0,
		BatchSize:        20,
		MaxConcurrent:    3,
		MaxBlockSize:     16 * 1024 * 1024,
		MaxBandwidthMBps: 10,
	}
	
	of := NewOpportunisticFetcher(cache, healthTracker, fetcher, config)
	
	// Add blocks including error block
	healthTracker.UpdateBlockHealth("error-block", BlockHint{
		ReplicationBucket: ReplicationLow,
		Size:              1024,
	})
	healthTracker.UpdateBlockHealth("good-block", BlockHint{
		ReplicationBucket: ReplicationLow,
		Size:              1024,
	})
	
	// Start
	of.Start()
	time.Sleep(500 * time.Millisecond)
	of.Stop()
	
	// Check error handling
	stats := of.GetStats()
	if stats["error_count"].(int64) == 0 {
		t.Error("Should have recorded errors")
	}
	
	// Error block should not be retried excessively
	if attempts, ok := fetchAttempts["error-block"]; ok {
		if attempts > config.MaxErrorRetries+1 {
			t.Errorf("Error block retried too many times: %d", attempts)
		}
	}
	
	// Good block should have been fetched
	if _, ok := fetchAttempts["good-block"]; !ok {
		t.Error("Good block should have been fetched")
	}
}

func TestOpportunisticFetcher_AntiThrashing(t *testing.T) {
	// Setup
	baseCache := NewMemoryCache(100)
	altruisticConfig := &AltruisticCacheConfig{
		MinPersonalCache: 5 * 1024,
		EnableAltruistic: true,
		EvictionCooldown: 500 * time.Millisecond,
	}
	cache := NewAltruisticCache(baseCache, altruisticConfig, 10*1024)
	
	healthTracker := NewBlockHealthTracker(nil)
	
	fetchCount := make(map[string]int)
	fetcher := func(ctx context.Context, cid string) ([]byte, error) {
		fetchCount[cid]++
		return make([]byte, 1024), nil
	}
	
	config := &OpportunisticConfig{
		MinFlexPoolFree: 0.1,
		CheckInterval:   100 * time.Millisecond,
		FetchCooldown:   300 * time.Millisecond,
	}
	
	of := NewOpportunisticFetcher(cache, healthTracker, fetcher, config)
	
	// Add valuable block
	healthTracker.UpdateBlockHealth("block1", BlockHint{
		ReplicationBucket: ReplicationLow,
		HighEntropy:       true,
		Size:              1024,
	})
	
	// Start
	of.Start()
	
	// Run for multiple check intervals
	time.Sleep(600 * time.Millisecond)
	
	of.Stop()
	
	// Block should not be fetched multiple times due to cooldown
	if count, ok := fetchCount["block1"]; ok {
		if count > 2 {
			t.Errorf("Block fetched too many times: %d", count)
		}
	}
}

func TestOpportunisticFetcher_ConcurrentFetching(t *testing.T) {
	// Setup
	baseCache := NewMemoryCache(1000)
	altruisticConfig := &AltruisticCacheConfig{
		MinPersonalCache: 10 * 1024,
		EnableAltruistic: true,
	}
	cache := NewAltruisticCache(baseCache, altruisticConfig, 100*1024)
	
	healthTracker := NewBlockHealthTracker(nil)
	
	// Track concurrent fetches
	activeFeatures := int32(0)
	maxConcurrent := int32(0)
	
	fetcher := func(ctx context.Context, cid string) ([]byte, error) {
		current := atomic.AddInt32(&activeFeatures, 1)
		
		// Update max
		for {
			max := atomic.LoadInt32(&maxConcurrent)
			if current <= max || atomic.CompareAndSwapInt32(&maxConcurrent, max, current) {
				break
			}
		}
		
		// Simulate work
		time.Sleep(50 * time.Millisecond)
		
		atomic.AddInt32(&activeFeatures, -1)
		return make([]byte, 1024), nil
	}
	
	config := &OpportunisticConfig{
		MinFlexPoolFree: 0.1,
		CheckInterval:   100 * time.Millisecond,
		MaxConcurrent:   3,
		BatchSize:       10,
	}
	
	of := NewOpportunisticFetcher(cache, healthTracker, fetcher, config)
	
	// Add many valuable blocks
	for i := 0; i < 20; i++ {
		healthTracker.UpdateBlockHealth(fmt.Sprintf("block-%d", i), BlockHint{
			ReplicationBucket: ReplicationLow,
			Size:              1024,
		})
	}
	
	// Start
	of.Start()
	time.Sleep(500 * time.Millisecond)
	of.Stop()
	
	// Check concurrency limit was respected
	if atomic.LoadInt32(&maxConcurrent) > int32(config.MaxConcurrent) {
		t.Errorf("Exceeded max concurrent: %d > %d", maxConcurrent, config.MaxConcurrent)
	}
}

func TestOpportunisticFetcher_PauseResume(t *testing.T) {
	// Setup
	baseCache := NewMemoryCache(1000)
	altruisticConfig := &AltruisticCacheConfig{
		MinPersonalCache: 10 * 1024,
		EnableAltruistic: true,
	}
	cache := NewAltruisticCache(baseCache, altruisticConfig, 100*1024)
	
	healthTracker := NewBlockHealthTracker(nil)
	
	fetchTimes := make(map[time.Time]bool)
	var fetchTimesMutex sync.RWMutex
	fetcher := func(ctx context.Context, cid string) ([]byte, error) {
		fetchTimesMutex.Lock()
		fetchTimes[time.Now()] = true
		fetchTimesMutex.Unlock()
		return make([]byte, 1024), nil
	}
	
	config := &OpportunisticConfig{
		MinFlexPoolFree:  0.1,
		CheckInterval:    50 * time.Millisecond,
		MaxErrorRetries:  3,
		ErrorBackoff:     200 * time.Millisecond,
		FetchCooldown:    100 * time.Millisecond,
		ValueThreshold:   2.0,
		BatchSize:        20,
		MaxConcurrent:    3,
		MaxBlockSize:     16 * 1024 * 1024,
		MaxBandwidthMBps: 10,
	}
	
	of := NewOpportunisticFetcher(cache, healthTracker, fetcher, config)
	
	// Add blocks
	for i := 0; i < 10; i++ {
		healthTracker.UpdateBlockHealth(fmt.Sprintf("block-%d", i), BlockHint{
			ReplicationBucket: ReplicationLow,
			Size:              1024,
		})
	}
	
	// Start
	of.Start()
	
	// Let it run
	time.Sleep(100 * time.Millisecond)
	
	// Pause
	pauseTime := time.Now()
	of.PauseForDuration(200 * time.Millisecond)
	
	// Wait during pause
	time.Sleep(150 * time.Millisecond)
	
	// Check no fetches during pause
	fetchesDuringPause := 0
	fetchTimesMutex.RLock()
	for fetchTime := range fetchTimes {
		if fetchTime.After(pauseTime) && fetchTime.Before(pauseTime.Add(150*time.Millisecond)) {
			fetchesDuringPause++
		}
	}
	fetchTimesMutex.RUnlock()
	
	if fetchesDuringPause > 0 {
		t.Error("Should not fetch during pause")
	}
	
	// Wait for resume and verify fetching continues
	time.Sleep(150 * time.Millisecond)
	
	of.Stop()
	
	// Should have some fetches after pause
	stats := of.GetStats()
	if stats["fetch_count"].(int64) == 0 {
		t.Error("Should have fetched blocks")
	}
}