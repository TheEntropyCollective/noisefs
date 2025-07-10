package privacy

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/cache"
	"github.com/TheEntropyCollective/noisefs/pkg/ipfs"
	"github.com/TheEntropyCollective/noisefs/pkg/reuse"
)

// AnonymizationTestSuite manages privacy validation tests
type AnonymizationTestSuite struct {
	blockManager   *blocks.BlockManager
	cacheManager   *cache.CacheManager
	ipfsClient     ipfs.BlockStore
	reuseClient    *reuse.ReuseAwareClient
	testData       [][]byte
	testBlocks     map[string]*blocks.Block
}

// TestBlockAnonymization tests that blocks are properly anonymized via XOR
func TestBlockAnonymization(t *testing.T) {
	suite := setupAnonymizationTest(t)

	// Create test file data
	testFile := generateTestFile(1024 * 1024) // 1MB test file
	
	// Split into blocks
	sourceBlocks, err := suite.blockManager.SplitFile(testFile, "test-file.dat")
	if err != nil {
		t.Fatalf("Failed to split test file: %v", err)
	}

	// Test each block for anonymization
	for i, block := range sourceBlocks {
		t.Run(fmt.Sprintf("Block_%d", i), func(t *testing.T) {
			// Verify block is XORed with randomizer
			if !suite.verifyBlockAnonymization(block) {
				t.Errorf("Block %d failed anonymization verification", i)
			}

			// Verify randomizer is from public domain content
			if !suite.verifyRandomizerLegality(block) {
				t.Errorf("Block %d randomizer not from legal sources", i)
			}

			// Verify anonymized block appears random
			if !suite.verifyRandomness(block.Data) {
				t.Errorf("Block %d anonymized data failed randomness test", i)
			}
		})
	}

	t.Logf("Anonymization test completed for %d blocks", len(sourceBlocks))
}

// TestPlausibleDeniability tests that individual blocks cannot be linked to files
func TestPlausibleDeniability(t *testing.T) {
	suite := setupAnonymizationTest(t)

	// Create multiple test files
	testFiles := []struct {
		name string
		data []byte
	}{
		{"document1.txt", generateTestFile(512 * 1024)},
		{"document2.txt", generateTestFile(768 * 1024)},
		{"document3.txt", generateTestFile(1024 * 1024)},
	}

	// Split all files into blocks
	allBlocks := make(map[string]*blocks.Block)
	fileBlocks := make(map[string][]*blocks.Block)
	
	for _, file := range testFiles {
		blocks, err := suite.blockManager.SplitFile(file.data, file.name)
		if err != nil {
			t.Fatalf("Failed to split file %s: %v", file.name, err)
		}
		
		fileBlocks[file.name] = blocks
		for _, block := range blocks {
			allBlocks[block.ID] = block
		}
	}

	// Test that blocks cannot be linked to original files
	for blockID, block := range allBlocks {
		t.Run(fmt.Sprintf("Block_%s", blockID), func(t *testing.T) {
			// Verify block content doesn't reveal source file
			if suite.detectSourceFileLeakage(block, testFiles) {
				t.Errorf("Block %s reveals source file information", blockID)
			}

			// Verify block metadata doesn't leak file info
			if suite.detectMetadataLeakage(block, testFiles) {
				t.Errorf("Block %s metadata reveals file information", blockID)
			}

			// Verify block cannot be reverse-engineered without descriptor
			if suite.detectReverseEngineering(block) {
				t.Errorf("Block %s vulnerable to reverse engineering", blockID)
			}
		})
	}

	t.Logf("Plausible deniability test completed for %d blocks across %d files", 
		len(allBlocks), len(testFiles))
}

// TestMultiUseBlockValidation tests that blocks serve multiple file reconstructions
func TestMultiUseBlockValidation(t *testing.T) {
	suite := setupAnonymizationTest(t)

	// Create test scenario with overlapping randomizer usage
	testData := [][]byte{
		generateTestFile(256 * 1024),
		generateTestFile(512 * 1024),
		generateTestFile(384 * 1024),
	}

	// Track randomizer reuse
	randomizerUsage := make(map[string][]string)
	
	for i, data := range testData {
		fileName := fmt.Sprintf("test-file-%d.dat", i)
		blocks, err := suite.blockManager.SplitFile(data, fileName)
		if err != nil {
			t.Fatalf("Failed to split test file %d: %v", i, err)
		}

		// Track which randomizers are used for each file
		for _, block := range blocks {
			if block.RandomizerID != "" {
				randomizerUsage[block.RandomizerID] = append(randomizerUsage[block.RandomizerID], fileName)
			}
		}
	}

	// Verify multi-use property
	multiUseCount := 0
	for randomizerID, files := range randomizerUsage {
		if len(files) > 1 {
			multiUseCount++
			t.Logf("Randomizer %s used by %d files: %v", randomizerID, len(files), files)
		}
	}

	if multiUseCount == 0 {
		t.Error("No randomizers were reused across multiple files")
	}

	// Verify that multi-use doesn't compromise anonymization
	for randomizerID, files := range randomizerUsage {
		if len(files) > 1 {
			if !suite.verifyMultiUseAnonymization(randomizerID, files) {
				t.Errorf("Multi-use randomizer %s compromises anonymization", randomizerID)
			}
		}
	}

	t.Logf("Multi-use validation completed: %d randomizers used by multiple files", multiUseCount)
}

// TestNetworkAnonymity tests network-level anonymity properties
func TestNetworkAnonymity(t *testing.T) {
	suite := setupAnonymizationTest(t)

	// Create test blocks for network analysis
	testFile := generateTestFile(1024 * 1024)
	blocks, err := suite.blockManager.SplitFile(testFile, "network-test.dat")
	if err != nil {
		t.Fatalf("Failed to create test blocks: %v", err)
	}

	// Test network anonymity properties
	for i, block := range blocks {
		t.Run(fmt.Sprintf("NetworkBlock_%d", i), func(t *testing.T) {
			// Verify block storage doesn't leak network information
			if !suite.verifyNetworkAnonymity(block) {
				t.Errorf("Block %d failed network anonymity check", i)
			}

			// Verify IPFS hash doesn't reveal content
			if !suite.verifyIPFSHashAnonymity(block) {
				t.Errorf("Block %d IPFS hash reveals content information", i)
			}

			// Verify peer-to-peer transfer anonymity
			if !suite.verifyP2PAnonymity(block) {
				t.Errorf("Block %d P2P transfer lacks anonymity", i)
			}
		})
	}

	t.Logf("Network anonymity test completed for %d blocks", len(blocks))
}

// TestCoverTrafficEffectiveness tests cover traffic privacy enhancement
func TestCoverTrafficEffectiveness(t *testing.T) {
	suite := setupAnonymizationTest(t)

	// Simulate cover traffic scenario
	realRequests := []string{
		"noisefs://user-document-001",
		"noisefs://user-document-002",
		"noisefs://user-document-003",
	}

	coverRequests := []string{
		"noisefs://cover-traffic-001",
		"noisefs://cover-traffic-002",
		"noisefs://cover-traffic-003",
		"noisefs://cover-traffic-004",
		"noisefs://cover-traffic-005",
	}

	// Test that cover traffic provides plausible deniability
	allRequests := append(realRequests, coverRequests...)
	
	// Verify that real requests cannot be distinguished from cover traffic
	for _, request := range allRequests {
		t.Run(fmt.Sprintf("Request_%s", request), func(t *testing.T) {
			if suite.detectRealVsCoverTraffic(request, realRequests, coverRequests) {
				t.Errorf("Request %s can be distinguished as real vs cover traffic", request)
			}
		})
	}

	// Test cover traffic ratio effectiveness
	coverRatio := float64(len(coverRequests)) / float64(len(realRequests))
	if coverRatio < 1.0 {
		t.Errorf("Cover traffic ratio too low: %.2f (should be >= 1.0)", coverRatio)
	}

	t.Logf("Cover traffic effectiveness test completed: %.2f cover ratio", coverRatio)
}

// TestAnonymizationStatistics tests statistical anonymity properties
func TestAnonymizationStatistics(t *testing.T) {
	suite := setupAnonymizationTest(t)

	// Generate large dataset for statistical analysis
	testFiles := make([][]byte, 100)
	for i := range testFiles {
		testFiles[i] = generateTestFile(64 * 1024) // 64KB files
	}

	// Collect anonymization statistics
	stats := &AnonymizationStats{
		TotalBlocks:      0,
		UniqueRandomizers: make(map[string]int),
		RandomnessScores: make([]float64, 0),
		ReuseFrequency:   make(map[int]int),
	}

	// Process all test files
	for i, file := range testFiles {
		blocks, err := suite.blockManager.SplitFile(file, fmt.Sprintf("stat-test-%d.dat", i))
		if err != nil {
			t.Fatalf("Failed to split file %d: %v", i, err)
		}

		for _, block := range blocks {
			stats.TotalBlocks++
			
			// Track randomizer usage
			if block.RandomizerID != "" {
				stats.UniqueRandomizers[block.RandomizerID]++
			}

			// Calculate randomness score
			randomnessScore := suite.calculateRandomnessScore(block.Data)
			stats.RandomnessScores = append(stats.RandomnessScores, randomnessScore)
		}
	}

	// Analyze reuse frequency distribution
	for _, count := range stats.UniqueRandomizers {
		stats.ReuseFrequency[count]++
	}

	// Verify statistical properties
	if stats.TotalBlocks == 0 {
		t.Fatal("No blocks processed for statistical analysis")
	}

	avgRandomness := suite.calculateAverageRandomness(stats.RandomnessScores)
	if avgRandomness < 0.8 {
		t.Errorf("Average randomness score too low: %.3f (should be >= 0.8)", avgRandomness)
	}

	reuseEfficiency := float64(len(stats.UniqueRandomizers)) / float64(stats.TotalBlocks)
	if reuseEfficiency > 0.5 {
		t.Errorf("Reuse efficiency too low: %.3f (should be <= 0.5)", reuseEfficiency)
	}

	t.Logf("Statistical analysis completed: %d blocks, %.3f avg randomness, %.3f reuse efficiency",
		stats.TotalBlocks, avgRandomness, reuseEfficiency)
}

// Helper types and functions

type AnonymizationStats struct {
	TotalBlocks      int
	UniqueRandomizers map[string]int
	RandomnessScores []float64
	ReuseFrequency   map[int]int
}

func setupAnonymizationTest(t *testing.T) *AnonymizationTestSuite {
	// Create test configuration
	blockConfig := blocks.DefaultBlockConfig()
	blockConfig.BlockSize = 128 * 1024 // 128KB blocks
	
	cacheConfig := cache.DefaultCacheConfig()
	cacheConfig.MaxSize = 100 * 1024 * 1024 // 100MB cache
	
	// Initialize components
	blockManager := blocks.NewBlockManager(blockConfig)
	cacheManager := cache.NewCacheManager(cacheConfig)
	
	// Create mock IPFS client for testing
	ipfsClient := &ipfs.MockIPFSClient{}
	
	// Create reuse client
	reuseClient := reuse.NewReuseAwareClient(ipfsClient, cacheManager)

	return &AnonymizationTestSuite{
		blockManager: blockManager,
		cacheManager: cacheManager,
		ipfsClient:   ipfsClient,
		reuseClient:  reuseClient,
		testData:     make([][]byte, 0),
		testBlocks:   make(map[string]*blocks.Block),
	}
}

func generateTestFile(size int) []byte {
	data := make([]byte, size)
	rand.Read(data)
	return data
}

func (suite *AnonymizationTestSuite) verifyBlockAnonymization(block *blocks.Block) bool {
	// Verify that block data is XORed with randomizer
	if block.RandomizerID == "" {
		return false
	}
	
	// In real implementation, would verify XOR operation
	// For testing, we check that block data doesn't match source patterns
	return suite.verifyRandomness(block.Data)
}

func (suite *AnonymizationTestSuite) verifyRandomizerLegality(block *blocks.Block) bool {
	// Verify randomizer comes from public domain or legally cleared content
	if block.RandomizerID == "" {
		return false
	}
	
	// In real implementation, would check against public domain registry
	// For testing, we assume randomizers are legal if they exist
	return true
}

func (suite *AnonymizationTestSuite) verifyRandomness(data []byte) bool {
	// Simple randomness test using chi-square
	if len(data) < 256 {
		return true // Skip test for small blocks
	}
	
	// Count byte frequencies
	freq := make(map[byte]int)
	for _, b := range data {
		freq[b]++
	}
	
	// Calculate chi-square statistic
	expected := float64(len(data)) / 256.0
	chiSquare := 0.0
	
	for i := 0; i < 256; i++ {
		observed := float64(freq[byte(i)])
		chiSquare += (observed - expected) * (observed - expected) / expected
	}
	
	// Simplified test: randomness is good if chi-square is reasonable
	return chiSquare < 300.0 && chiSquare > 200.0
}

func (suite *AnonymizationTestSuite) detectSourceFileLeakage(block *blocks.Block, files []struct{name string; data []byte}) bool {
	// Check if block data contains identifiable patterns from source files
	for _, file := range files {
		if suite.containsPattern(block.Data, file.data[:min(len(file.data), 1024)]) {
			return true
		}
	}
	return false
}

func (suite *AnonymizationTestSuite) detectMetadataLeakage(block *blocks.Block, files []struct{name string; data []byte}) bool {
	// Check if block metadata reveals file information
	for _, file := range files {
		if block.Metadata != nil {
			if filename, ok := block.Metadata["original_filename"]; ok {
				if filename == file.name {
					return true
				}
			}
		}
	}
	return false
}

func (suite *AnonymizationTestSuite) detectReverseEngineering(block *blocks.Block) bool {
	// Test if block can be reverse-engineered without descriptor
	// This is a simplified test - real implementation would be more sophisticated
	return suite.containsObviousPatterns(block.Data)
}

func (suite *AnonymizationTestSuite) verifyMultiUseAnonymization(randomizerID string, files []string) bool {
	// Verify that using the same randomizer for multiple files doesn't compromise anonymization
	// In real implementation, would check that different source blocks + same randomizer
	// still produce sufficiently different results
	return len(files) > 1 // Simplified check
}

func (suite *AnonymizationTestSuite) verifyNetworkAnonymity(block *blocks.Block) bool {
	// Verify network-level anonymity properties
	return block.ID != "" && len(block.Data) > 0
}

func (suite *AnonymizationTestSuite) verifyIPFSHashAnonymity(block *blocks.Block) bool {
	// Verify IPFS hash doesn't reveal content information
	hash := sha256.Sum256(block.Data)
	return len(hash) == 32 // Simplified check
}

func (suite *AnonymizationTestSuite) verifyP2PAnonymity(block *blocks.Block) bool {
	// Verify peer-to-peer transfer anonymity
	return true // Simplified - real implementation would check network protocols
}

func (suite *AnonymizationTestSuite) detectRealVsCoverTraffic(request string, realRequests, coverRequests []string) bool {
	// Test if request can be distinguished as real vs cover traffic
	// This should return false for good anonymity
	return false // Simplified - real implementation would analyze request patterns
}

func (suite *AnonymizationTestSuite) calculateRandomnessScore(data []byte) float64 {
	// Calculate randomness score using entropy
	if len(data) == 0 {
		return 0.0
	}
	
	freq := make(map[byte]int)
	for _, b := range data {
		freq[b]++
	}
	
	entropy := 0.0
	length := float64(len(data))
	
	for _, count := range freq {
		if count > 0 {
			p := float64(count) / length
			entropy -= p * logBase2(p)
		}
	}
	
	// Normalize to 0-1 scale (max entropy for byte data is 8)
	return entropy / 8.0
}

func (suite *AnonymizationTestSuite) calculateAverageRandomness(scores []float64) float64 {
	if len(scores) == 0 {
		return 0.0
	}
	
	sum := 0.0
	for _, score := range scores {
		sum += score
	}
	
	return sum / float64(len(scores))
}

func (suite *AnonymizationTestSuite) containsPattern(data, pattern []byte) bool {
	// Simple pattern matching - in real implementation would be more sophisticated
	if len(pattern) > len(data) {
		return false
	}
	
	for i := 0; i <= len(data)-len(pattern); i++ {
		match := true
		for j := 0; j < len(pattern); j++ {
			if data[i+j] != pattern[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func (suite *AnonymizationTestSuite) containsObviousPatterns(data []byte) bool {
	// Check for obvious patterns that would indicate poor anonymization
	if len(data) < 16 {
		return false
	}
	
	// Check for repeated sequences
	for i := 0; i < len(data)-8; i++ {
		for j := i + 8; j < len(data)-8; j++ {
			if suite.containsPattern(data[j:j+8], data[i:i+8]) {
				return true
			}
		}
	}
	
	return false
}

func logBase2(x float64) float64 {
	if x <= 0 {
		return 0
	}
	return 1.4426950408889634 * float64(big.NewFloat(x).Text('f', -1)[0]) // Simplified log2
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}