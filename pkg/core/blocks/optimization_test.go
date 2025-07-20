package blocks

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"
)

// TestMemoryPoolManager validates memory pool functionality
func TestMemoryPoolManager(t *testing.T) {
	pool := NewMemoryPoolManager()
	
	// Test byte buffer allocation and reuse
	t.Run("ByteBufferPool", func(t *testing.T) {
		// Test small buffer
		smallBuffer := pool.GetByteBuffer(1024)
		if cap(smallBuffer) < 1024 {
			t.Errorf("Small buffer capacity too small: %d", cap(smallBuffer))
		}
		pool.ReturnByteBuffer(smallBuffer)
		
		// Test medium buffer
		mediumBuffer := pool.GetByteBuffer(32 * 1024)
		if cap(mediumBuffer) < 32*1024 {
			t.Errorf("Medium buffer capacity too small: %d", cap(mediumBuffer))
		}
		pool.ReturnByteBuffer(mediumBuffer)
		
		// Test large buffer
		largeBuffer := pool.GetByteBuffer(512 * 1024)
		if cap(largeBuffer) < 512*1024 {
			t.Errorf("Large buffer capacity too small: %d", cap(largeBuffer))
		}
		pool.ReturnByteBuffer(largeBuffer)
	})
	
	// Test BlockInfo pooling
	t.Run("BlockInfoPool", func(t *testing.T) {
		info1 := pool.GetBlockInfo()
		if info1 == nil {
			t.Error("Expected non-nil BlockInfo")
		}
		
		// Modify the info
		info1.CID = "test-cid"
		info1.Size = 1024
		info1.Popularity = 10
		
		pool.ReturnBlockInfo(info1)
		
		// Get another info - should be reset
		info2 := pool.GetBlockInfo()
		if info2.CID != "" || info2.Size != 0 || info2.Popularity != 0 {
			t.Error("BlockInfo was not properly reset")
		}
		
		pool.ReturnBlockInfo(info2)
	})
	
	// Test DirectoryManifest pooling
	t.Run("DirectoryManifestPool", func(t *testing.T) {
		manifest1 := pool.GetDirectoryManifest()
		if manifest1 == nil {
			t.Error("Expected non-nil DirectoryManifest")
		}
		
		// Add some entries
		entry := DirectoryEntry{
			EncryptedName: []byte("test-file"),
			CID:          "test-cid",
			Type:         FileType,
		}
		manifest1.AddEntry(entry)
		
		if len(manifest1.Entries) != 1 {
			t.Error("Entry was not added to manifest")
		}
		
		pool.ReturnDirectoryManifest(manifest1)
		
		// Get another manifest - should be reset
		manifest2 := pool.GetDirectoryManifest()
		if len(manifest2.Entries) != 0 {
			t.Error("DirectoryManifest was not properly reset")
		}
		
		pool.ReturnDirectoryManifest(manifest2)
	})
	
	// Test pool metrics
	t.Run("PoolMetrics", func(t *testing.T) {
		// Reset metrics by creating new pool
		testPool := NewMemoryPoolManager()
		
		// Perform some allocations
		for i := 0; i < 10; i++ {
			buffer := testPool.GetByteBuffer(1024)
			testPool.ReturnByteBuffer(buffer)
		}
		
		metrics := testPool.GetPoolMetrics()
		if metrics.TotalAllocations != 10 {
			t.Errorf("Expected 10 total allocations, got %d", metrics.TotalAllocations)
		}
		
		if metrics.HitRate <= 0 {
			t.Errorf("Expected positive hit rate, got %f", metrics.HitRate)
		}
		
		t.Logf("Pool metrics: %+v", metrics)
	})
}

// TestWorkerPoolOptimizer validates enhanced worker pool functionality
func TestWorkerPoolOptimizer(t *testing.T) {
	config := DefaultWorkerPoolConfig()
	config.MinWorkers = 2
	config.MaxWorkers = 8
	config.QueueSize = 100
	
	pool := NewWorkerPoolOptimizer(config)
	defer pool.Shutdown(5 * time.Second)
	
	t.Run("BasicWorkExecution", func(t *testing.T) {
		var completedTasks int32
		var mu sync.Mutex
		var completionTimes []time.Duration
		
		// Submit work items
		for i := 0; i < 20; i++ {
			workID := fmt.Sprintf("task-%d", i)
			
			item := WorkItem{
				ID: workID,
				Task: func(ctx context.Context) error {
					// Simulate work
					time.Sleep(10 * time.Millisecond)
					return nil
				},
				Priority: i % 3, // Vary priority
				Callback: func(result WorkResult) {
					mu.Lock()
					completedTasks++
					completionTimes = append(completionTimes, result.Duration)
					mu.Unlock()
				},
			}
			
			err := pool.SubmitWork(item)
			if err != nil {
				t.Errorf("Failed to submit work item %s: %v", workID, err)
			}
		}
		
		// Wait for completion
		timeout := time.After(5 * time.Second)
		for {
			mu.Lock()
			completed := completedTasks
			mu.Unlock()
			
			if completed >= 20 {
				break
			}
			
			select {
			case <-timeout:
				t.Errorf("Timeout waiting for task completion, completed: %d/20", completed)
				return
			case <-time.After(100 * time.Millisecond):
				// Continue waiting
			}
		}
		
		// Verify all tasks completed
		mu.Lock()
		if completedTasks != 20 {
			t.Errorf("Expected 20 completed tasks, got %d", completedTasks)
		}
		
		// Verify completion times are reasonable
		for i, duration := range completionTimes {
			if duration > 100*time.Millisecond {
				t.Errorf("Task %d took too long: %v", i, duration)
			}
		}
		mu.Unlock()
	})
	
	t.Run("WorkerPoolMetrics", func(t *testing.T) {
		// Wait a moment for any pending work to complete
		time.Sleep(100 * time.Millisecond)
		
		metrics := pool.GetMetrics()
		
		if metrics.ActiveWorkers < config.MinWorkers {
			t.Errorf("Expected at least %d active workers, got %d", config.MinWorkers, metrics.ActiveWorkers)
		}
		
		if metrics.ActiveWorkers > config.MaxWorkers {
			t.Errorf("Expected at most %d active workers, got %d", config.MaxWorkers, metrics.ActiveWorkers)
		}
		
		if metrics.CompletedTasks < 20 {
			t.Errorf("Expected at least 20 completed tasks, got %d", metrics.CompletedTasks)
		}
		
		t.Logf("Worker pool metrics: %+v", metrics)
	})
	
	t.Run("AdaptiveScaling", func(t *testing.T) {
		// Create pool with adaptive scaling enabled
		adaptiveConfig := DefaultWorkerPoolConfig()
		adaptiveConfig.MinWorkers = 1
		adaptiveConfig.MaxWorkers = 4
		adaptiveConfig.AdaptiveScaling = true
		
		adaptivePool := NewWorkerPoolOptimizer(adaptiveConfig)
		defer adaptivePool.Shutdown(5 * time.Second)
		
		// Submit many tasks to trigger scaling
		for i := 0; i < 50; i++ {
			item := WorkItem{
				ID: fmt.Sprintf("adaptive-task-%d", i),
				Task: func(ctx context.Context) error {
					time.Sleep(50 * time.Millisecond) // Longer tasks
					return nil
				},
			}
			adaptivePool.SubmitWork(item)
		}
		
		// Allow time for scaling decisions
		time.Sleep(2 * time.Second)
		
		metrics := adaptivePool.GetMetrics()
		t.Logf("Adaptive scaling metrics: %+v", metrics)
		
		// Should have scaled up due to high queue load
		if metrics.ActiveWorkers <= 1 {
			t.Errorf("Expected worker scaling up, but workers: %d", metrics.ActiveWorkers)
		}
	})
}

// BenchmarkMemoryPoolPerformance compares memory allocation with and without pooling
func BenchmarkMemoryPoolPerformance(b *testing.B) {
	pool := NewMemoryPoolManager()
	
	b.Run("WithoutPooling", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			// Allocate buffer without pooling
			buffer := make([]byte, 4096)
			_ = buffer
		}
	})
	
	b.Run("WithPooling", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			// Allocate buffer with pooling
			buffer := pool.GetByteBuffer(4096)
			pool.ReturnByteBuffer(buffer)
		}
	})
	
	b.Run("BlockInfoWithoutPooling", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			// Allocate BlockInfo without pooling
			info := &BlockInfo{}
			_ = info
		}
	})
	
	b.Run("BlockInfoWithPooling", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			// Allocate BlockInfo with pooling
			info := pool.GetBlockInfo()
			pool.ReturnBlockInfo(info)
		}
	})
}

// BenchmarkWorkerPoolPerformance compares worker pool performance
func BenchmarkWorkerPoolPerformance(b *testing.B) {
	config := DefaultWorkerPoolConfig()
	config.MaxWorkers = runtime.NumCPU()
	
	pool := NewWorkerPoolOptimizer(config)
	defer pool.Shutdown(5 * time.Second)
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		completed := make(chan struct{})
		
		item := WorkItem{
			ID: fmt.Sprintf("bench-task-%d", i),
			Task: func(ctx context.Context) error {
				// Minimal work to measure pool overhead
				return nil
			},
			Callback: func(result WorkResult) {
				close(completed)
			},
		}
		
		pool.SubmitWork(item)
		<-completed
	}
}

// TestOptimizedAllocationFunctions validates convenience allocation functions
func TestOptimizedAllocationFunctions(t *testing.T) {
	t.Run("OptimizedBlockAllocation", func(t *testing.T) {
		buffer, cleanup := OptimizedBlockAllocation(8192)
		
		if cap(buffer) < 8192 {
			t.Errorf("Buffer capacity too small: %d", cap(buffer))
		}
		
		// Use buffer
		buffer = append(buffer, []byte("test data")...)
		
		// Clean up
		cleanup()
	})
	
	t.Run("OptimizedBlockInfoAllocation", func(t *testing.T) {
		info, cleanup := OptimizedBlockInfoAllocation()
		
		if info == nil {
			t.Error("Expected non-nil BlockInfo")
		}
		
		// Use info
		info.CID = "test-cid"
		info.Size = 1024
		
		// Clean up
		cleanup()
	})
	
	t.Run("OptimizedManifestAllocation", func(t *testing.T) {
		manifest, cleanup := OptimizedManifestAllocation()
		
		if manifest == nil {
			t.Error("Expected non-nil DirectoryManifest")
		}
		
		// Use manifest
		entry := DirectoryEntry{
			EncryptedName: []byte("test"),
			CID:          "test-cid",
			Type:         FileType,
		}
		manifest.AddEntry(entry)
		
		// Clean up
		cleanup()
	})
}

// TestIntegrationWithExistingComponents validates integration with existing code
func TestIntegrationWithExistingComponents(t *testing.T) {
	// Test that optimizations don't break existing functionality
	
	t.Run("DirectoryManifestIntegration", func(t *testing.T) {
		// Get manifest from pool
		manifest, cleanup := OptimizedManifestAllocation()
		defer cleanup()
		
		// Use like normal DirectoryManifest
		entry := DirectoryEntry{
			EncryptedName: []byte("integration-test"),
			CID:          "integration-cid",
			Type:         FileType,
		}
		
		err := manifest.AddEntry(entry)
		if err != nil {
			t.Errorf("Failed to add entry to pooled manifest: %v", err)
		}
		
		if len(manifest.Entries) != 1 {
			t.Error("Entry was not added correctly")
		}
		
		snapshot := manifest.GetSnapshot()
		if len(snapshot.Entries) != 1 {
			t.Error("Snapshot did not reflect added entry")
		}
	})
}

func init() {
	// Ensure we have consistent test results
	runtime.GOMAXPROCS(runtime.NumCPU())
}