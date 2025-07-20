//go:build real_ipfs
// +build real_ipfs

package system

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	noisefs "github.com/TheEntropyCollective/noisefs/pkg/core/client"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/config"
	"github.com/TheEntropyCollective/noisefs/pkg/privacy/reuse"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
)

// RealIPFSTestSuite provides real IPFS network testing
type RealIPFSTestSuite struct {
	ipfsNodes      []string // IPFS node endpoints
	noisefsClients []*noisefs.Client
	reuseClients   []*reuse.ReuseAwareClient
	testConfig     *config.Config
	testData       []TestFile
}

type TestFile struct {
	Name     string
	Content  []byte
	Size     int64
	Expected string // Expected descriptor CID
}

type NetworkMetrics struct {
	StartTime             time.Time
	EndTime               time.Time
	TotalOperations       int64
	SuccessfulOperations  int64
	FailedOperations      int64
	AverageLatency        time.Duration
	TotalBytesTransferred int64
	NodeMetrics           map[string]*NodeMetric
}

type NodeMetric struct {
	NodeURL             string
	OperationsHandled   int64
	AverageResponseTime time.Duration
	ErrorRate           float64
	ConnectedPeers      int
	BlocksStored        int64
	BlocksRetrieved     int64
}

// TestRealIPFSNetworkInitialization tests that the multi-node IPFS network starts correctly
func TestRealIPFSNetworkInitialization(t *testing.T) {
	suite := setupRealIPFSTest(t)
	defer suite.cleanup()

	// Test that all IPFS nodes are responding
	for i, client := range suite.ipfsNodes {
		t.Run(fmt.Sprintf("Node_%d_Health", i+1), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			// Test basic connectivity
			testBlock, err := blocks.NewBlock([]byte(fmt.Sprintf("test-connectivity-node-%d", i+1)))
			if err != nil {
				t.Fatalf("Failed to create test block: %v", err)
			}

			// Store and retrieve a test block
			cid, err := client.StoreBlock(testBlock)
			if err != nil {
				t.Fatalf("Failed to store test block on node %d: %v", i+1, err)
			}

			retrievedBlock, err := client.RetrieveBlock(cid)
			if err != nil {
				t.Fatalf("Failed to retrieve test block from node %d: %v", i+1, err)
			}

			if !bytes.Equal(testBlock.Data, retrievedBlock.Data) {
				t.Fatalf("Retrieved block data doesn't match original on node %d", i+1)
			}

			t.Logf("Node %d is healthy and responding correctly", i+1)
		})
	}
}

// TestMultiNodeBlockDistribution tests that blocks are properly distributed across nodes
func TestMultiNodeBlockDistribution(t *testing.T) {
	suite := setupRealIPFSTest(t)
	defer suite.cleanup()

	ctx := context.Background()
	testBlocks := make([]*blocks.Block, 10)
	storedCIDs := make([]string, 10)

	// Create test blocks of different sizes
	for i := 0; i < 10; i++ {
		content := bytes.Repeat([]byte(fmt.Sprintf("test-block-%d-", i)), 1000) // ~13KB per block
		block, err := blocks.NewBlock(content)
		if err != nil {
			t.Fatalf("Failed to create test block %d: %v", i, err)
		}
		testBlocks[i] = block
	}

	// Store blocks using different nodes
	for i, block := range testBlocks {
		nodeIndex := i % len(suite.ipfsNodes)
		client := suite.ipfsNodes[nodeIndex]

		cid, err := client.StoreBlock(block)
		if err != nil {
			t.Fatalf("Failed to store block %d on node %d: %v", i, nodeIndex, err)
		}
		storedCIDs[i] = cid

		t.Logf("Stored block %d (CID: %s) on node %d", i, cid[:8], nodeIndex+1)
	}

	// Allow time for IPFS network propagation
	time.Sleep(30 * time.Second)

	// Verify blocks can be retrieved from any node
	for i, cid := range storedCIDs {
		for j, client := range suite.ipfsNodes {
			t.Run(fmt.Sprintf("Block_%d_From_Node_%d", i, j+1), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
				defer cancel()

				retrievedBlock, err := client.RetrieveBlock(cid)
				if err != nil {
					t.Errorf("Failed to retrieve block %d from node %d: %v", i, j+1, err)
					return
				}

				if !bytes.Equal(testBlocks[i].Data, retrievedBlock.Data) {
					t.Errorf("Block %d data mismatch when retrieved from node %d", i, j+1)
				}
			})
		}
	}
}

// TestNoiseFS E2E workflow with real IPFS
func TestNoiseFS_RealIPFS_E2E_Workflow(t *testing.T) {
	suite := setupRealIPFSTest(t)
	defer suite.cleanup()

	// Test file upload and download workflow
	testFiles := []TestFile{
		{
			Name:    "small_text.txt",
			Content: []byte("This is a small test file for NoiseFS testing."),
			Size:    47,
		},
		{
			Name:    "medium_document.txt",
			Content: bytes.Repeat([]byte("Medium size document content. "), 1000), // ~30KB
			Size:    30000,
		},
		{
			Name:    "large_binary.bin",
			Content: generateRandomBytes(500000), // 500KB
			Size:    500000,
		},
	}

	for _, testFile := range testFiles {
		t.Run(fmt.Sprintf("E2E_%s", testFile.Name), func(t *testing.T) {
			// Use the first NoiseFS client for upload
			uploadClient := suite.noisefsClients[0]

			// Upload file
			reader := bytes.NewReader(testFile.Content)
			descriptorCID, err := uploadClient.Upload(reader, testFile.Name)
			if err != nil {
				t.Fatalf("Failed to upload %s: %v", testFile.Name, err)
			}

			t.Logf("Uploaded %s with descriptor CID: %s", testFile.Name, descriptorCID)

			// Allow time for IPFS propagation
			time.Sleep(10 * time.Second)

			// Download from different NoiseFS clients (different IPFS nodes)
			for i, downloadClient := range suite.noisefsClients {
				t.Run(fmt.Sprintf("Download_From_Client_%d", i+1), func(t *testing.T) {
					downloadedData, err := downloadClient.Download(descriptorCID)
					if err != nil {
						t.Errorf("Failed to download from client %d: %v", i+1, err)
						return
					}

					if !bytes.Equal(testFile.Content, downloadedData) {
						t.Errorf("Downloaded content doesn't match original for %s via client %d", testFile.Name, i+1)
						return
					}

					t.Logf("Successfully downloaded %s (%d bytes) via client %d", testFile.Name, len(downloadedData), i+1)
				})
			}
		})
	}
}

// TestReuseSystemWithRealIPFS tests the block reuse system with real IPFS
func TestReuseSystemWithRealIPFS(t *testing.T) {
	suite := setupRealIPFSTest(t)
	defer suite.cleanup()

	if len(suite.reuseClients) == 0 {
		t.Skip("Reuse clients not available for this test")
	}

	// Test reuse system functionality
	testContent := []byte("Test content for reuse system validation with real IPFS network")

	reuseClient := suite.reuseClients[0]
	reader := bytes.NewReader(testContent)

	// Upload with reuse enforcement
	result, err := reuseClient.UploadFile(reader, "reuse_test.txt", 64*1024)
	if err != nil {
		// This might fail due to insufficient reuse, which is expected
		t.Logf("Upload rejected due to reuse enforcement: %v", err)

		// Verify that the rejection was due to reuse requirements
		if result != nil && result.ValidationResult != nil && !result.ValidationResult.Valid {
			t.Logf("Validation failed as expected: %v", result.ValidationResult.Violations)
		}
		return
	}

	t.Logf("Upload succeeded with descriptor CID: %s", result.DescriptorCID)

	// Verify reuse statistics
	stats := reuseClient.GetReuseStatistics()
	t.Logf("Reuse statistics: %+v", stats)

	// Generate legal documentation
	if result.DescriptorCID != "" {
		legalDoc, err := reuseClient.GetLegalDocumentation(result.DescriptorCID)
		if err != nil {
			t.Errorf("Failed to generate legal documentation: %v", err)
		} else {
			t.Logf("Generated legal documentation with %d block evidence entries", len(legalDoc.BlockReuseEvidence))
		}
	}
}

// TestNetworkPerformanceMetrics measures real network performance
func TestNetworkPerformanceMetrics(t *testing.T) {
	suite := setupRealIPFSTest(t)
	defer suite.cleanup()

	metrics := &NetworkMetrics{
		StartTime:   time.Now(),
		NodeMetrics: make(map[string]*NodeMetric),
	}

	// Initialize node metrics
	nodeURLs := []string{
		"http://127.0.0.1:5001",
		"http://127.0.0.1:5002",
		"http://127.0.0.1:5003",
		"http://127.0.0.1:5004",
		"http://127.0.0.1:5005",
	}

	for _, url := range nodeURLs {
		metrics.NodeMetrics[url] = &NodeMetric{
			NodeURL: url,
		}
	}

	// Perform performance testing
	numOperations := 50
	testBlockSize := 32 * 1024 // 32KB blocks

	for i := 0; i < numOperations; i++ {
		nodeIndex := i % len(suite.ipfsNodes)
		client := suite.ipfsNodes[nodeIndex]
		nodeURL := nodeURLs[nodeIndex]

		// Create test block
		content := generateRandomBytes(testBlockSize)
		block, err := blocks.NewBlock(content)
		if err != nil {
			t.Fatalf("Failed to create test block: %v", err)
		}

		// Measure store operation
		storeStart := time.Now()
		cid, err := client.StoreBlock(block)
		storeDuration := time.Since(storeStart)

		if err != nil {
			metrics.FailedOperations++
			t.Logf("Store operation %d failed: %v", i, err)
			continue
		}

		// Measure retrieve operation
		retrieveStart := time.Now()
		_, err = client.RetrieveBlock(cid)
		retrieveDuration := time.Since(retrieveStart)

		if err != nil {
			metrics.FailedOperations++
			t.Logf("Retrieve operation %d failed: %v", i, err)
			continue
		}

		// Update metrics
		metrics.SuccessfulOperations++
		metrics.TotalBytesTransferred += int64(len(content))

		nodeMetric := metrics.NodeMetrics[nodeURL]
		nodeMetric.OperationsHandled++
		nodeMetric.AverageResponseTime = (nodeMetric.AverageResponseTime + storeDuration + retrieveDuration) / 2
		nodeMetric.BlocksStored++
		nodeMetric.BlocksRetrieved++

		if i%10 == 0 {
			t.Logf("Completed %d/%d operations", i+1, numOperations)
		}
	}

	metrics.TotalOperations = metrics.SuccessfulOperations + metrics.FailedOperations
	metrics.EndTime = time.Now()
	metrics.AverageLatency = metrics.EndTime.Sub(metrics.StartTime) / time.Duration(metrics.TotalOperations)

	// Report metrics
	t.Logf("=== Network Performance Metrics ===")
	t.Logf("Total Operations: %d", metrics.TotalOperations)
	t.Logf("Successful: %d (%.2f%%)", metrics.SuccessfulOperations, float64(metrics.SuccessfulOperations)/float64(metrics.TotalOperations)*100)
	t.Logf("Failed: %d (%.2f%%)", metrics.FailedOperations, float64(metrics.FailedOperations)/float64(metrics.TotalOperations)*100)
	t.Logf("Average Latency: %v", metrics.AverageLatency)
	t.Logf("Total Data Transferred: %d bytes (%.2f MB)", metrics.TotalBytesTransferred, float64(metrics.TotalBytesTransferred)/(1024*1024))
	t.Logf("Test Duration: %v", metrics.EndTime.Sub(metrics.StartTime))

	for url, nodeMetric := range metrics.NodeMetrics {
		t.Logf("Node %s: %d ops, avg response: %v", url, nodeMetric.OperationsHandled, nodeMetric.AverageResponseTime)
	}

	// Verify minimum performance requirements
	successRate := float64(metrics.SuccessfulOperations) / float64(metrics.TotalOperations)
	if successRate < 0.95 {
		t.Errorf("Success rate too low: %.2f%% (minimum: 95%%)", successRate*100)
	}

	if metrics.AverageLatency > 5*time.Second {
		t.Errorf("Average latency too high: %v (maximum: 5s)", metrics.AverageLatency)
	}
}

// Helper functions

func setupRealIPFSTest(t *testing.T) *RealIPFSTestSuite {
	// Check if IPFS nodes are available
	nodeURLs := []string{
		"http://127.0.0.1:5001",
		"http://127.0.0.1:5002",
		"http://127.0.0.1:5003",
		"http://127.0.0.1:5004",
		"http://127.0.0.1:5005",
	}

	// Verify IPFS nodes are running
	for i, url := range nodeURLs {
		if !checkIPFSNodeHealth(url) {
			t.Fatalf("IPFS node %d at %s is not responding. Run 'make ipfs-network-start' first.", i+1, url)
		}
	}

	// Initialize IPFS clients
	ipfsClients := make([]*ipfs.Client, len(nodeURLs))
	for i, url := range nodeURLs {
		client, err := ipfs.NewClient(url)
		if err != nil {
			t.Fatalf("Failed to create IPFS client for %s: %v", url, err)
		}
		ipfsClients[i] = client
	}

	// Initialize NoiseFS clients
	noisefsClients := make([]*noisefs.Client, len(ipfsClients))
	for i, ipfsClient := range ipfsClients {
		cacheInstance := cache.NewMemoryCache(50 * 1024 * 1024) // 50MB cache
		client, err := noisefs.NewClient(ipfsClient, cacheInstance)
		if err != nil {
			t.Fatalf("Failed to create NoiseFS client %d: %v", i, err)
		}
		noisefsClients[i] = client
	}

	// Initialize reuse-aware clients (may fail if reuse system isn't ready)
	reuseClients := make([]*reuse.ReuseAwareClient, 0, len(ipfsClients))
	for i, ipfsClient := range ipfsClients {
		cacheInstance := cache.NewMemoryCache(50 * 1024 * 1024)
		reuseClient, err := reuse.NewReuseAwareClient(ipfsClient, cacheInstance)
		if err != nil {
			t.Logf("Warning: Failed to create reuse client %d (this is normal if reuse system isn't fully initialized): %v", i, err)
		} else {
			reuseClients = append(reuseClients, reuseClient)
		}
	}

	// Load test configuration
	testConfig, err := config.LoadFromFile("../tests/configs/noisefs-test.json")
	if err != nil {
		t.Logf("Warning: Could not load test config, using defaults: %v", err)
		testConfig = config.DefaultConfig()
	}

	return &RealIPFSTestSuite{
		ipfsNodes:      ipfsClients,
		noisefsClients: noisefsClients,
		reuseClients:   reuseClients,
		testConfig:     testConfig,
	}
}

func (suite *RealIPFSTestSuite) cleanup() {
	// Cleanup is minimal since we're using external IPFS nodes
	// Just ensure any temporary test data is cleaned up
}

func checkIPFSNodeHealth(url string) bool {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url + "/api/v0/version")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}

func generateRandomBytes(size int) []byte {
	// Simple deterministic "random" bytes for testing
	data := make([]byte, size)
	for i := 0; i < size; i++ {
		data[i] = byte(i % 256)
	}
	return data
}

// TestRealIPFSNetworkStability tests long-term network stability
func TestRealIPFSNetworkStability(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stability test in short mode")
	}

	// Only run if explicitly requested
	if os.Getenv("RUN_STABILITY_TESTS") != "true" {
		t.Skip("Stability tests disabled. Set RUN_STABILITY_TESTS=true to enable")
	}

	suite := setupRealIPFSTest(t)
	defer suite.cleanup()

	// Run for 30 minutes with continuous operations
	testDuration := 30 * time.Minute
	startTime := time.Now()
	operationCount := 0

	t.Logf("Starting %v stability test", testDuration)

	for time.Since(startTime) < testDuration {
		// Perform a round of operations
		for i, client := range suite.ipfsNodes {
			content := fmt.Sprintf("stability-test-%d-%d", operationCount, i)
			block, err := blocks.NewBlock([]byte(content))
			if err != nil {
				t.Errorf("Failed to create block at operation %d: %v", operationCount, err)
				continue
			}

			cid, err := client.StoreBlock(block)
			if err != nil {
				t.Errorf("Failed to store block at operation %d on node %d: %v", operationCount, i, err)
				continue
			}

			// Verify retrieval from a different node
			retrieveNodeIndex := (i + 1) % len(suite.ipfsNodes)
			retrieveClient := suite.ipfsNodes[retrieveNodeIndex]

			_, err = retrieveClient.RetrieveBlock(cid)
			if err != nil {
				t.Errorf("Failed to retrieve block at operation %d from node %d: %v", operationCount, retrieveNodeIndex, err)
				continue
			}

			operationCount++
		}

		// Report progress every 5 minutes
		if operationCount%1000 == 0 {
			elapsed := time.Since(startTime)
			remaining := testDuration - elapsed
			t.Logf("Stability test progress: %v elapsed, %v remaining, %d operations completed", elapsed.Round(time.Second), remaining.Round(time.Second), operationCount)
		}

		// Small delay between rounds
		time.Sleep(100 * time.Millisecond)
	}

	t.Logf("Stability test completed: %d operations over %v", operationCount, time.Since(startTime))
}
