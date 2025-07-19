package main

import (
	"encoding/json"
	"fmt"
	"runtime"
	"time"

	noisefs "github.com/TheEntropyCollective/noisefs/pkg/core/client"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/logging"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
)

// showMetrics displays NoiseFS client metrics
func showMetrics(client *noisefs.Client, logger *logging.Logger) {
	metrics := client.GetMetrics()

	fmt.Println("=== NoiseFS Metrics ===")
	fmt.Printf("Files uploaded: %d\n", metrics.FilesUploaded)
	fmt.Printf("Files downloaded: %d\n", metrics.FilesDownloaded)
	fmt.Printf("Blocks generated: %d\n", metrics.BlocksGenerated)
	fmt.Printf("Blocks stored: %d\n", metrics.BlocksStored)
	fmt.Printf("Blocks retrieved: %d\n", metrics.BlocksRetrieved)
	fmt.Printf("Bytes uploaded: %s\n", formatBytes(metrics.BytesUploaded))
	fmt.Printf("Bytes downloaded: %s\n", formatBytes(metrics.BytesDownloaded))
	fmt.Printf("Bytes stored in IPFS: %s\n", formatBytes(metrics.BytesStoredIPFS))
	
	// Calculate efficiency metrics
	if metrics.BytesUploaded > 0 {
		efficiency := float64(metrics.BytesStoredIPFS) / float64(metrics.BytesUploaded)
		fmt.Printf("Storage efficiency: %.2fx overhead\n", efficiency)
	}
	
	if metrics.BlocksGenerated > 0 && metrics.BlocksStored > 0 {
		deduplicationRate := 1.0 - (float64(metrics.BlocksStored) / float64(metrics.BlocksGenerated))
		fmt.Printf("Block deduplication rate: %.1f%%\n", deduplicationRate*100)
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
	
	// Get storage manager status
	backends := storageManager.GetBackendStatus()
	
	if jsonOutput {
		stats := map[string]interface{}{
			"timestamp": time.Now().Format(time.RFC3339),
			"noisefs": map[string]interface{}{
				"files_uploaded":      noisefsMetrics.FilesUploaded,
				"files_downloaded":    noisefsMetrics.FilesDownloaded,
				"blocks_generated":    noisefsMetrics.BlocksGenerated,
				"blocks_stored":       noisefsMetrics.BlocksStored,
				"blocks_retrieved":    noisefsMetrics.BlocksRetrieved,
				"bytes_uploaded":      noisefsMetrics.BytesUploaded,
				"bytes_downloaded":    noisefsMetrics.BytesDownloaded,
				"bytes_stored_ipfs":   noisefsMetrics.BytesStoredIPFS,
			},
			"cache": map[string]interface{}{
				"hits":           cacheStats.Hits,
				"misses":         cacheStats.Misses,
				"hit_rate":       float64(cacheStats.Hits) / float64(cacheStats.Hits + cacheStats.Misses),
				"entries":        cacheStats.Entries,
				"memory_usage":   cacheStats.MemoryUsage,
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
		if noisefsMetrics.BytesUploaded > 0 {
			stats["noisefs"].(map[string]interface{})["storage_efficiency"] = 
				float64(noisefsMetrics.BytesStoredIPFS) / float64(noisefsMetrics.BytesUploaded)
		}
		
		if noisefsMetrics.BlocksGenerated > 0 && noisefsMetrics.BlocksStored > 0 {
			stats["noisefs"].(map[string]interface{})["deduplication_rate"] = 
				1.0 - (float64(noisefsMetrics.BlocksStored) / float64(noisefsMetrics.BlocksGenerated))
		}
		
		jsonData, err := json.MarshalIndent(stats, "", "  ")
		if err != nil {
			logger.Error("Failed to marshal stats JSON", "error", err)
			return
		}
		fmt.Println(string(jsonData))
	} else {
		fmt.Println("=== NoiseFS System Statistics ===")
		fmt.Printf("Timestamp: %s\n\n", time.Now().Format("2006-01-02 15:04:05"))
		
		// NoiseFS metrics
		fmt.Println("NoiseFS Operations:")
		fmt.Printf("  Files uploaded: %d\n", noisefsMetrics.FilesUploaded)
		fmt.Printf("  Files downloaded: %d\n", noisefsMetrics.FilesDownloaded)
		fmt.Printf("  Blocks generated: %d\n", noisefsMetrics.BlocksGenerated)
		fmt.Printf("  Blocks stored: %d\n", noisefsMetrics.BlocksStored)
		fmt.Printf("  Blocks retrieved: %d\n", noisefsMetrics.BlocksRetrieved)
		fmt.Printf("  Bytes uploaded: %s\n", formatBytes(noisefsMetrics.BytesUploaded))
		fmt.Printf("  Bytes downloaded: %s\n", formatBytes(noisefsMetrics.BytesDownloaded))
		fmt.Printf("  Bytes stored in IPFS: %s\n", formatBytes(noisefsMetrics.BytesStoredIPFS))
		
		// Efficiency metrics
		if noisefsMetrics.BytesUploaded > 0 {
			efficiency := float64(noisefsMetrics.BytesStoredIPFS) / float64(noisefsMetrics.BytesUploaded)
			fmt.Printf("  Storage efficiency: %.2fx overhead\n", efficiency)
		}
		
		if noisefsMetrics.BlocksGenerated > 0 && noisefsMetrics.BlocksStored > 0 {
			deduplicationRate := 1.0 - (float64(noisefsMetrics.BlocksStored) / float64(noisefsMetrics.BlocksGenerated))
			fmt.Printf("  Block deduplication rate: %.1f%%\n", deduplicationRate*100)
		}
		
		// Cache statistics
		fmt.Println("\nCache Performance:")
		fmt.Printf("  Cache hits: %d\n", cacheStats.Hits)
		fmt.Printf("  Cache misses: %d\n", cacheStats.Misses)
		if cacheStats.Hits + cacheStats.Misses > 0 {
			hitRate := float64(cacheStats.Hits) / float64(cacheStats.Hits + cacheStats.Misses)
			fmt.Printf("  Hit rate: %.2f%%\n", hitRate*100)
		}
		fmt.Printf("  Cache entries: %d\n", cacheStats.Entries)
		fmt.Printf("  Memory usage: %s\n", formatBytes(cacheStats.MemoryUsage))
		
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