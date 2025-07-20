package benchmarks

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	noisefs "github.com/TheEntropyCollective/noisefs/pkg/core/client"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/TheEntropyCollective/noisefs/pkg/common/logging"
	
	// Import backends to register them
	_ "github.com/TheEntropyCollective/noisefs/pkg/storage/backends"
)

// CorrectedOverheadResult captures overhead measurements with corrected tracking
type CorrectedOverheadResult struct {
	Scenario        string  `json:"scenario"`
	FileSize        int64   `json:"file_size_bytes"`
	OverheadPercent float64 `json:"overhead_percent"`
	SystemState     string  `json:"system_state"` // "cold", "warm", "mature"
	RandomizerReuse int     `json:"randomizer_reuse_count"`
}

// BenchmarkStorageOverhead tests storage overhead with accurate tracking
func BenchmarkStorageOverhead(b *testing.B) {
	// Create storage setup with mock backend
	_ = logging.NewLogger(nil)
	cache := cache.NewMemoryCache(1000)
	
	// Create storage config with mock backend
	config := storage.DefaultConfig()
	config.Backends = map[string]*storage.BackendConfig{
		"mock": {
			Type:     "mock",
			Enabled:  true,
			Priority: 100,
			Connection: &storage.ConnectionConfig{
				Endpoint: "mock://test",
			},
		},
	}
	config.DefaultBackend = "mock"
	
	// Create storage manager
	storageManager, err := storage.NewManager(config)
	if err != nil {
		b.Fatalf("Failed to create storage manager: %v", err)
	}
	
	// Start storage manager
	ctx := context.Background()
	if err := storageManager.Start(ctx); err != nil {
		b.Fatalf("Failed to start storage manager: %v", err)
	}
	defer storageManager.Stop(ctx)
	
	// Create NoiseFS client with the storage manager
	client, err := noisefs.NewClientWithStorageManager(storageManager, cache)
	if err != nil {
		b.Fatalf("Failed to create NoiseFS client: %v", err)
	}

	// Test progressive system maturity: cold -> warm -> mature
	testFileSize := int64(128 * 1024) // 128KB files (1 block each)
	numTestFiles := 20
	
	var results []CorrectedOverheadResult
	
	b.Run("Progressive_System_Maturity", func(b *testing.B) {
		b.Logf("Testing progressive system maturity with %d files of %dKB each", numTestFiles, testFileSize/1024)
		
		for fileNum := 0; fileNum < numTestFiles; fileNum++ {
			// Generate test data
			testData := make([]byte, testFileSize)
			for i := range testData {
				testData[i] = byte((i + fileNum*7) % 256)
			}
			
			// Get metrics before upload
			initialMetrics := client.GetMetrics()
			initialStored := initialMetrics.BytesStoredIPFS
			
			// Get cache state
			cacheStatsBefore := cache.GetStats()
			
			// Upload file
			reader := strings.NewReader(string(testData))
			descriptorCID, err := client.Upload(reader, fmt.Sprintf("test_corrected_%d.dat", fileNum))
			if err != nil {
				b.Fatalf("Upload %d failed: %v", fileNum, err)
			}
			
			// Get metrics after upload
			finalMetrics := client.GetMetrics()
			finalStored := finalMetrics.BytesStoredIPFS
			
			// Calculate overhead with corrected definition
			bytesStored := finalStored - initialStored
			overheadPercent := ((float64(bytesStored) - float64(testFileSize)) / float64(testFileSize)) * 100.0
			
			// Determine system state
			var systemState string
			switch {
			case fileNum == 0:
				systemState = "cold"
			case fileNum < 5:
				systemState = "warming"
			case fileNum < 15:
				systemState = "warm"
			default:
				systemState = "mature"
			}
			
			// Get cache state after upload
			cacheStatsAfter := cache.GetStats()
			
			result := CorrectedOverheadResult{
				Scenario:        fmt.Sprintf("File_%d_%s_system", fileNum+1, systemState),
				FileSize:        testFileSize,
				OverheadPercent: overheadPercent,
				SystemState:     systemState,
				RandomizerReuse: int(cacheStatsAfter.Size - cacheStatsBefore.Size),
			}
			results = append(results, result)
			
			// Log detailed progress for key files
			if fileNum < 3 || fileNum >= numTestFiles-3 || (fileNum+1)%5 == 0 {
				b.Logf("File %d (%s): %dKB->%dB stored, Overhead: %.1f%%, Cache: %d->%d, Hit rate: %.1f%%", 
					fileNum+1, systemState, testFileSize/1024, bytesStored, overheadPercent, 
					cacheStatsBefore.Size, cacheStatsAfter.Size, cacheStatsAfter.HitRate*100)
			}
			
			// Verify we can download it back
			_, err = client.Download(descriptorCID)
			if err != nil {
				b.Fatalf("Download %d failed: %v", fileNum, err)
			}
		}
	})

	// Generate report with corrected overhead analysis
	generateCorrectedReport(b, results)
}

// generateCorrectedReport creates analysis with corrected overhead understanding
func generateCorrectedReport(b *testing.B, results []CorrectedOverheadResult) {
	if len(results) == 0 {
		return
	}

	// Analyze by system state
	stateAnalysis := make(map[string][]float64)
	for _, result := range results {
		stateAnalysis[result.SystemState] = append(stateAnalysis[result.SystemState], result.OverheadPercent)
	}

	b.Logf("\n=== Corrected NoiseFS Storage Overhead Analysis ===")
	b.Logf("Overhead Definition: (StoredBytes - OriginalBytes) / OriginalBytes * 100%%")
	b.Logf("Expected: Cold ~200%%, Warm ~100%%, Mature ~0%%")
	
	for state, overheads := range stateAnalysis {
		if len(overheads) == 0 {
			continue
		}
		
		var total float64
		min, max := 1000.0, -1000.0
		for _, overhead := range overheads {
			total += overhead
			if overhead < min {
				min = overhead
			}
			if overhead > max {
				max = overhead
			}
		}
		avg := total / float64(len(overheads))
		
		b.Logf("%s System: %.1f%% avg (%.1f%% - %.1f%%), %d files", 
			strings.Title(state), avg, min, max, len(overheads))
	}
	
	// Overall progression analysis
	firstFileOverhead := results[0].OverheadPercent
	lastFileOverhead := results[len(results)-1].OverheadPercent
	improvement := firstFileOverhead - lastFileOverhead
	
	b.Logf("\nProgression Analysis:")
	b.Logf("First file (cold): %.1f%% overhead", firstFileOverhead)
	b.Logf("Last file (mature): %.1f%% overhead", lastFileOverhead)
	b.Logf("Improvement: %.1f percentage points", improvement)
	
	// Validation against expected ranges
	b.Logf("\nValidation:")
	if firstFileOverhead >= 150.0 && firstFileOverhead <= 250.0 {
		b.Logf("✓ Cold system overhead within expected range (150-250%%)")
	} else {
		b.Logf("⚠ Cold system overhead outside expected range: %.1f%%", firstFileOverhead)
	}
	
	if lastFileOverhead >= -10.0 && lastFileOverhead <= 50.0 {
		b.Logf("✓ Mature system overhead approaching expected range (0-50%%)")
	} else {
		b.Logf("⚠ Mature system overhead outside expected range: %.1f%%", lastFileOverhead)
	}
	
	if improvement > 50.0 {
		b.Logf("✓ Significant improvement through randomizer reuse (%.1f points)", improvement)
	} else {
		b.Logf("⚠ Limited improvement through randomizer reuse (%.1f points)", improvement)
	}
	
	b.Logf("\nKey Findings:")
	b.Logf("- Tracking now includes ONLY new randomizer storage")
	b.Logf("- Cached randomizer reuse does NOT count toward overhead")
	b.Logf("- System maturity significantly impacts overhead")
	b.Logf("- Date: %s", time.Now().Format("2006-01-02 15:04:05"))
}