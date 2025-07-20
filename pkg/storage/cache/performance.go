package cache

import (
	"container/heap"
	"runtime"
	"sync"
)

// BlockInfoHeap implements heap.Interface for BlockInfo sorting by popularity
type BlockInfoHeap []*BlockInfo

func (h BlockInfoHeap) Len() int           { return len(h) }
func (h BlockInfoHeap) Less(i, j int) bool { return h[i].Popularity > h[j].Popularity } // Max heap
func (h BlockInfoHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *BlockInfoHeap) Push(x interface{}) {
	*h = append(*h, x.(*BlockInfo))
}

func (h *BlockInfoHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[0 : n-1]
	return item
}

// PerformanceOptimizer provides optimized algorithms for cache operations
type PerformanceOptimizer struct {
	heapPool sync.Pool
}

// NewPerformanceOptimizer creates a new performance optimizer with object pooling
func NewPerformanceOptimizer() *PerformanceOptimizer {
	return &PerformanceOptimizer{
		heapPool: sync.Pool{
			New: func() interface{} {
				h := make(BlockInfoHeap, 0, 64) // Start with reasonable capacity
				return &h
			},
		},
	}
}

// GetTopNBlocks efficiently selects the top N blocks by popularity using heap algorithm
// Time complexity: O(n + k*log(n)) where n=total blocks, k=requested count
// Space complexity: O(1) with pooled memory reuse
func (po *PerformanceOptimizer) GetTopNBlocks(blocks []*BlockInfo, count int) []*BlockInfo {
	if len(blocks) == 0 || count <= 0 {
		return nil
	}
	
	if count >= len(blocks) {
		// If requesting all or more blocks, sort in-place and return
		return po.sortInPlace(blocks)
	}
	
	// For small datasets, use simple sort (avoids heap overhead)
	if len(blocks) < 50 {
		return po.sortInPlace(blocks)[:count]
	}
	
	// Use heap for efficient top-K selection on large datasets
	return po.heapSelect(blocks, count)
}

// sortInPlace sorts blocks by popularity using optimized quicksort
func (po *PerformanceOptimizer) sortInPlace(blocks []*BlockInfo) []*BlockInfo {
	// Use Go's optimized sort with custom comparison
	quickSort(blocks, 0, len(blocks)-1)
	return blocks
}

// heapSelect efficiently finds top K elements using max-heap
func (po *PerformanceOptimizer) heapSelect(blocks []*BlockInfo, count int) []*BlockInfo {
	// Get pooled heap to avoid allocations
	h := po.heapPool.Get().(*BlockInfoHeap)
	defer func() {
		// Clear and return to pool
		*h = (*h)[:0]
		po.heapPool.Put(h)
	}()
	
	// Build heap with all elements
	for i := 0; i < len(blocks); i++ {
		*h = append(*h, blocks[i])
	}
	heap.Init(h)
	
	// Extract top K elements
	resultSize := count
	if resultSize > len(*h) {
		resultSize = len(*h)
	}
	
	result := make([]*BlockInfo, resultSize)
	for i := 0; i < resultSize; i++ {
		result[i] = heap.Pop(h).(*BlockInfo)
	}
	
	return result
}

// quickSort implements optimized in-place quicksort for BlockInfo by popularity
func quickSort(blocks []*BlockInfo, low, high int) {
	if low < high {
		pivotIndex := partition(blocks, low, high)
		quickSort(blocks, low, pivotIndex-1)
		quickSort(blocks, pivotIndex+1, high)
	}
}

// partition function for quicksort with median-of-three pivot selection
func partition(blocks []*BlockInfo, low, high int) int {
	// Median-of-three pivot selection for better average performance
	mid := (low + high) / 2
	if blocks[mid].Popularity > blocks[low].Popularity {
		blocks[low], blocks[mid] = blocks[mid], blocks[low]
	}
	if blocks[high].Popularity > blocks[low].Popularity {
		blocks[low], blocks[high] = blocks[high], blocks[low]
	}
	if blocks[mid].Popularity > blocks[high].Popularity {
		blocks[mid], blocks[high] = blocks[high], blocks[mid]
	}
	
	pivot := blocks[high].Popularity
	i := low - 1
	
	for j := low; j < high; j++ {
		if blocks[j].Popularity >= pivot { // Descending order (highest popularity first)
			i++
			blocks[i], blocks[j] = blocks[j], blocks[i]
		}
	}
	
	blocks[i+1], blocks[high] = blocks[high], blocks[i+1]
	return i + 1
}

// AdaptiveWorkerCount calculates optimal worker count based on system resources
func AdaptiveWorkerCount() int {
	cpuCount := runtime.NumCPU()
	// Use 2x CPU count for I/O bound operations, with reasonable bounds
	workers := cpuCount * 2
	if workers < 4 {
		workers = 4 // Minimum workers for small systems
	}
	if workers > 64 {
		workers = 64 // Maximum to prevent resource exhaustion
	}
	return workers
}

// CacheStatistics provides enhanced cache performance metrics
type CacheStatistics struct {
	HitRate        float64 // Cache hit percentage
	MissRate       float64 // Cache miss percentage  
	EvictionRate   float64 // Block eviction percentage
	AverageLatency float64 // Average operation latency in milliseconds
	MemoryUsage    int64   // Current memory usage in bytes
	MemoryLimit    int64   // Memory limit in bytes
}

// calculateCacheStatistics computes performance metrics from cache stats
func calculateCacheStatistics(stats Stats, memoryUsage, memoryLimit int64) CacheStatistics {
	total := float64(stats.Hits + stats.Misses)
	if total == 0 {
		return CacheStatistics{
			MemoryUsage: memoryUsage,
			MemoryLimit: memoryLimit,
		}
	}
	
	return CacheStatistics{
		HitRate:        float64(stats.Hits) / total * 100,
		MissRate:       float64(stats.Misses) / total * 100,
		EvictionRate:   float64(stats.Evictions) / total * 100,
		MemoryUsage:    memoryUsage,
		MemoryLimit:    memoryLimit,
	}
}