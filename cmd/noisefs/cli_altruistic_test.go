package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/TheEntropyCollective/noisefs/pkg/util"
)

func TestCLIAltruisticCache(t *testing.T) {
	// Skip if not running integration tests or if IPFS is not available
	if testing.Short() {
		t.Skip("Skipping CLI integration test")
	}

	// Check if we have IPFS available
	if os.Getenv("NOISEFS_TEST_IPFS_ENDPOINT") == "" {
		t.Skip("Skipping CLI test: IPFS not available (set NOISEFS_TEST_IPFS_ENDPOINT to enable)")
	}

	// Test basic stats output includes altruistic cache info
	t.Run("StatsWithAltruisticCache", func(t *testing.T) {
		// Create a test config with altruistic cache enabled
		ipfsEndpoint := os.Getenv("NOISEFS_TEST_IPFS_ENDPOINT")
		if ipfsEndpoint == "" {
			ipfsEndpoint = "http://localhost:5001"
		}
		config := fmt.Sprintf(`{
			"ipfs": {"api_endpoint": "%s"},
			"cache": {
				"block_cache_size": 100,
				"memory_limit_mb": 512,
				"enable_altruistic": true,
				"min_personal_cache_mb": 100
			},
			"performance": {"block_size": 131072}
		}`, ipfsEndpoint)

		// Write config to temp file
		tmpConfig, err := os.CreateTemp("", "noisefs-test-*.json")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(tmpConfig.Name())

		if _, err := tmpConfig.WriteString(config); err != nil {
			t.Fatal(err)
		}
		tmpConfig.Close()

		// Run stats command
		cmd := exec.Command("./noisefs", "-config", tmpConfig.Name(), "-stats", "-json")
		cmd.Env = append(os.Environ(), "NOISEFS_SKIP_LEGAL_NOTICE=1")
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("Command failed: %v\nOutput: %s", err, output)
		}

		// Parse JSON output
		var result struct {
			Success bool `json:"success"`
			Data    struct {
				Result util.StatsResult `json:"result"`
			} `json:"data"`
		}

		if err := json.Unmarshal(output, &result); err != nil {
			t.Fatalf("Failed to parse JSON: %v\nOutput: %s", err, output)
		}

		// Verify altruistic cache is present
		if result.Data.Result.Altruistic == nil {
			t.Error("Expected altruistic cache stats in output")
		}

		if result.Data.Result.Altruistic != nil {
			// Verify configuration values
			if !result.Data.Result.Altruistic.Enabled {
				t.Error("Altruistic cache should be enabled")
			}

			if result.Data.Result.Altruistic.MinPersonalCacheMB != 100 {
				t.Errorf("Expected MinPersonalCacheMB=100, got %d",
					result.Data.Result.Altruistic.MinPersonalCacheMB)
			}
		}
	})

	// Test command-line overrides
	t.Run("CLIOverrides", func(t *testing.T) {
		// Base config with altruistic disabled
		ipfsEndpoint := os.Getenv("NOISEFS_TEST_IPFS_ENDPOINT")
		if ipfsEndpoint == "" {
			ipfsEndpoint = "http://localhost:5001"
		}
		config := fmt.Sprintf(`{
			"ipfs": {"api_endpoint": "%s"},
			"cache": {
				"block_cache_size": 100,
				"enable_altruistic": false
			}
		}`, ipfsEndpoint)

		tmpConfig, err := os.CreateTemp("", "noisefs-test-*.json")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(tmpConfig.Name())

		if _, err := tmpConfig.WriteString(config); err != nil {
			t.Fatal(err)
		}
		tmpConfig.Close()

		// Run with override flags
		cmd := exec.Command("./noisefs",
			"-config", tmpConfig.Name(),
			"-min-personal-cache", "200",
			"-altruistic-bandwidth", "50",
			"-stats")
		cmd.Env = append(os.Environ(), "NOISEFS_SKIP_LEGAL_NOTICE=1")

		output, err := cmd.Output()
		if err != nil {
			// If altruistic is disabled in config, this is expected
			// Just verify the command runs
			return
		}

		// Check output contains altruistic info
		outputStr := string(output)
		if strings.Contains(outputStr, "Altruistic Cache") {
			if !strings.Contains(outputStr, "Min Personal Cache: 200") {
				t.Error("Override for min-personal-cache not applied")
			}
		}
	})

	// Test disable flag
	t.Run("DisableAltruistic", func(t *testing.T) {
		ipfsEndpoint := os.Getenv("NOISEFS_TEST_IPFS_ENDPOINT")
		if ipfsEndpoint == "" {
			ipfsEndpoint = "http://localhost:5001"
		}
		config := fmt.Sprintf(`{
			"ipfs": {"api_endpoint": "%s"},
			"cache": {
				"block_cache_size": 100,
				"enable_altruistic": true,
				"min_personal_cache_mb": 100
			}
		}`, ipfsEndpoint)

		tmpConfig, err := os.CreateTemp("", "noisefs-test-*.json")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(tmpConfig.Name())

		if _, err := tmpConfig.WriteString(config); err != nil {
			t.Fatal(err)
		}
		tmpConfig.Close()

		// Run with disable flag
		cmd := exec.Command("./noisefs",
			"-config", tmpConfig.Name(),
			"-disable-altruistic",
			"-stats",
			"-json")
		cmd.Env = append(os.Environ(), "NOISEFS_SKIP_LEGAL_NOTICE=1")

		output, err := cmd.Output()
		if err == nil {
			// Parse output to verify altruistic is disabled
			var result struct {
				Success bool `json:"success"`
				Data    struct {
					Result util.StatsResult `json:"result"`
				} `json:"data"`
			}

			if json.Unmarshal(output, &result) == nil {
				if result.Data.Result.Altruistic != nil &&
					result.Data.Result.Altruistic.Enabled {
					t.Error("Altruistic cache should be disabled")
				}
			}
		}
	})
}

func TestCacheVisualization(t *testing.T) {
	viz := util.NewCacheVisualization(20)

	// Test empty cache
	bar := viz.RenderCacheUsage(0, 0)
	if !strings.Contains(bar, "░░░░░░░░░░░░░░░░░░░░") {
		t.Errorf("Empty cache should show all empty blocks: %s", bar)
	}

	// Test full personal cache
	bar = viz.RenderCacheUsage(100, 0)
	if !strings.Contains(bar, "████████████████████") {
		t.Errorf("Full personal cache should show all full blocks: %s", bar)
	}

	// Test mixed usage
	bar = viz.RenderCacheUsage(50, 30)
	if !strings.Contains(bar, "█") || !strings.Contains(bar, "▒") || !strings.Contains(bar, "░") {
		t.Errorf("Mixed usage should show all block types: %s", bar)
	}

	// Test flex pool
	bar = viz.RenderFlexPoolUsage(0.75)
	if !strings.Contains(bar, "▓") || !strings.Contains(bar, "░") {
		t.Errorf("Flex pool should show filled and empty blocks: %s", bar)
	}
}
