package main

import (
	"encoding/json"
	"fmt"
	"runtime"
	"time"

	noisefs "github.com/TheEntropyCollective/noisefs/pkg/core/client"
	"github.com/TheEntropyCollective/noisefs/pkg/common/logging"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
)

// showMetrics displays NoiseFS client metrics
func showMetrics(client *noisefs.Client, logger *logging.Logger) {
	metrics := client.GetMetrics()

	fmt.Println("=== NoiseFS Metrics ===")
	fmt.Printf("Files uploaded: %d\n", metrics.TotalUploads)
	fmt.Printf("Files downloaded: %d\n", metrics.TotalDownloads)
	fmt.Printf("Blocks generated: %d\n", metrics.BlocksGenerated)
	fmt.Printf("Blocks reused: %d\n", metrics.BlocksReused)
	fmt.Printf("Cache hits: %d\n", metrics.CacheHits)
	fmt.Printf("Bytes uploaded: %s\n", formatBytes(metrics.BytesUploadedOriginal))
	fmt.Printf("Cache misses: %d\n", metrics.CacheMisses)
	fmt.Printf("Bytes stored in IPFS: %s\n", formatBytes(metrics.BytesStoredIPFS))
	
	// Calculate efficiency metrics
	if metrics.BytesUploadedOriginal > 0 {
		efficiency := float64(metrics.BytesStoredIPFS) / float64(metrics.BytesUploadedOriginal)
		fmt.Printf("Storage efficiency: %.2fx overhead\n", efficiency)
	}
	
	if metrics.BlocksGenerated > 0 && metrics.BlocksReused > 0 {
		reuseRate := float64(metrics.BlocksReused) / float64(metrics.BlocksGenerated + metrics.BlocksReused)
		fmt.Printf("Block reuse rate: %.1f%%\n", reuseRate*100)
	}
}

// showSystemStats displays comprehensive system statistics
func showSystemStats(storageManager *storage.Manager, client *noisefs.Client, blockCache cache.Cache, jsonOutput bool, logger *logging.Logger) {
	// Get NoiseFS metrics
	noisefsMetrics := client.GetMetrics()
	
	// Get cache statistics
	cacheStats := blockCache.GetStats()
	
	// Get runtime memory stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	
	// Get storage manager status (placeholder)
	backends := map[string]string{"ipfs": "connected"} // TODO: implement GetBackendStatus
	
	if jsonOutput {
		stats := map[string]interface{}{
			"timestamp": time.Now().Format(time.RFC3339),
			"noisefs": map[string]interface{}{
				"total_uploads":         noisefsMetrics.TotalUploads,
				"total_downloads":       noisefsMetrics.TotalDownloads,
				"blocks_generated":      noisefsMetrics.BlocksGenerated,
				"blocks_reused":         noisefsMetrics.BlocksReused,
				"cache_hits":            noisefsMetrics.CacheHits,
				"cache_misses":          noisefsMetrics.CacheMisses,
				"bytes_uploaded_orig":   noisefsMetrics.BytesUploadedOriginal,
				"bytes_stored_ipfs":     noisefsMetrics.BytesStoredIPFS,
			},
			"cache": map[string]interface{}{
				"hits":           cacheStats.Hits,
				"misses":         cacheStats.Misses,
				"hit_rate":       float64(cacheStats.Hits) / float64(cacheStats.Hits + cacheStats.Misses),
				"size":           cacheStats.Size,
				"evictions":      cacheStats.Evictions,
			},
			"memory": map[string]interface{}{
				"alloc_mb":       float64(memStats.Alloc) / 1024 / 1024,
				"total_alloc_mb": float64(memStats.TotalAlloc) / 1024 / 1024,
				"sys_mb":         float64(memStats.Sys) / 1024 / 1024,
				"num_gc":         memStats.NumGC,
				"goroutines":     runtime.NumGoroutine(),
			},
			"storage_backends": backends,
		}
		
		// Add efficiency calculations
		if noisefsMetrics.BytesUploadedOriginal > 0 {
			stats["noisefs"].(map[string]interface{})["storage_efficiency"] = 
				float64(noisefsMetrics.BytesStoredIPFS) / float64(noisefsMetrics.BytesUploadedOriginal)
		}
		
		if noisefsMetrics.CacheHits + noisefsMetrics.CacheMisses > 0 {
			stats["noisefs"].(map[string]interface{})["cache_hit_rate"] = 
				float64(noisefsMetrics.CacheHits) / float64(noisefsMetrics.CacheHits + noisefsMetrics.CacheMisses)
		}
		
		jsonData, err := json.MarshalIndent(stats, "", "  ")
		if err != nil {
			logger.Error("Failed to marshal stats JSON", map[string]interface{}{
			"error": err.Error(),
		})
			return
		}
		fmt.Println(string(jsonData))
	} else {
		fmt.Println("=== NoiseFS System Statistics ===")
		fmt.Printf("Timestamp: %s\n\n", time.Now().Format("2006-01-02 15:04:05"))
		
		// NoiseFS metrics
		fmt.Println("NoiseFS Operations:")
		fmt.Printf("  Files uploaded: %d\n", noisefsMetrics.TotalUploads)
		fmt.Printf("  Files downloaded: %d\n", noisefsMetrics.TotalDownloads)
		fmt.Printf("  Blocks generated: %d\n", noisefsMetrics.BlocksGenerated)
		fmt.Printf("  Blocks reused: %d\n", noisefsMetrics.BlocksReused)
		fmt.Printf("  Cache hits: %d\n", noisefsMetrics.CacheHits)
		fmt.Printf("  Cache misses: %d\n", noisefsMetrics.CacheMisses)
		fmt.Printf("  Bytes uploaded: %s\n", formatBytes(noisefsMetrics.BytesUploadedOriginal))
		fmt.Printf("  Bytes stored in IPFS: %s\n", formatBytes(noisefsMetrics.BytesStoredIPFS))
		
		// Efficiency metrics
		if noisefsMetrics.BytesUploadedOriginal > 0 {
			efficiency := float64(noisefsMetrics.BytesStoredIPFS) / float64(noisefsMetrics.BytesUploadedOriginal)
			fmt.Printf("  Storage efficiency: %.2fx overhead\n", efficiency)
		}
		
		if noisefsMetrics.CacheHits + noisefsMetrics.CacheMisses > 0 {
			hitRate := float64(noisefsMetrics.CacheHits) / float64(noisefsMetrics.CacheHits + noisefsMetrics.CacheMisses)
			fmt.Printf("  NoiseFS cache hit rate: %.1f%%\n", hitRate*100)
		}
		
		// Cache statistics
		fmt.Println("\nCache Performance:")
		fmt.Printf("  Cache hits: %d\n", cacheStats.Hits)
		fmt.Printf("  Cache misses: %d\n", cacheStats.Misses)
		if cacheStats.Hits + cacheStats.Misses > 0 {
			hitRate := float64(cacheStats.Hits) / float64(cacheStats.Hits + cacheStats.Misses)
			fmt.Printf("  Hit rate: %.2f%%\n", hitRate*100)
		}
		fmt.Printf("  Cache entries: %d\n", cacheStats.Size)
		fmt.Printf("  Cache evictions: %d\n", cacheStats.Evictions)
		
		// Memory statistics
		fmt.Println("\nMemory Usage:")
		fmt.Printf("  Current allocation: %s\n", formatBytes(int64(memStats.Alloc)))
		fmt.Printf("  Total allocations: %s\n", formatBytes(int64(memStats.TotalAlloc)))
		fmt.Printf("  System memory: %s\n", formatBytes(int64(memStats.Sys)))
		fmt.Printf("  GC cycles: %d\n", memStats.NumGC)
		fmt.Printf("  Goroutines: %d\n", runtime.NumGoroutine())
		
		// Storage backend status
		fmt.Println("\nStorage Backends:")
		for name, status := range backends {
			fmt.Printf("  %s: %s\n", name, status)
		}
	}
}

// formatBytes converts bytes to human-readable format
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}