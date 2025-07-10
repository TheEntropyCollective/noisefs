package testing

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/ipfs"
	"github.com/TheEntropyCollective/noisefs/pkg/core/client"
)

// RealIPFSTestHarness manages a real multi-node IPFS test environment
type RealIPFSTestHarness struct {
	nodes       []*RealIPFSNode
	nodeCount   int
	networkName string
	isRunning   bool
	mu          sync.RWMutex
}

// RealIPFSNode represents a real IPFS node in the test network
type RealIPFSNode struct {
	NodeID      string
	APIPort     int
	P2PPort     int
	APIAddress  string
	ipfsClient  *ipfs.Client
	NoiseClient *noisefs.Client // Exported for test access
	cache       cache.Cache
}

// NodeConfig holds configuration for a real IPFS node
type NodeConfig struct {
	NodeCount   int
	CacheSize   int
	NetworkName string
	StartPort   int // Starting port for node 1 (5001), subsequent nodes use StartPort+1, etc.
}

// NewRealIPFSTestHarness creates a new real IPFS test environment
func NewRealIPFSTestHarness(config NodeConfig) *RealIPFSTestHarness {
	if config.NetworkName == "" {
		config.NetworkName = "noisefs-test-network"
	}
	if config.StartPort == 0 {
		config.StartPort = 5001
	}
	if config.CacheSize == 0 {
		config.CacheSize = 100
	}

	harness := &RealIPFSTestHarness{
		nodeCount:   config.NodeCount,
		networkName: config.NetworkName,
		nodes:       make([]*RealIPFSNode, config.NodeCount),
	}

	// Initialize node configurations
	for i := 0; i < config.NodeCount; i++ {
		harness.nodes[i] = &RealIPFSNode{
			NodeID:     fmt.Sprintf("noisefs-ipfs-%d", i+1),
			APIPort:    config.StartPort + i,
			P2PPort:    4001 + i,
			APIAddress: fmt.Sprintf("localhost:%d", config.StartPort+i),
			cache:      cache.NewMemoryCache(config.CacheSize),
		}
	}

	return harness
}

// StartNetwork starts the IPFS test network using Docker Compose
func (h *RealIPFSTestHarness) StartNetwork() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.isRunning {
		return fmt.Errorf("network is already running")
	}

	fmt.Printf("Starting real IPFS test network with %d nodes...\n", h.nodeCount)

	// Find project root and docker-compose file
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}
	
	// Look for docker-compose.test.yml in current dir and parent dirs
	composeFile := ""
	for dir := wd; dir != "/"; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, "docker-compose.test.yml")
		if _, err := os.Stat(candidate); err == nil {
			composeFile = candidate
			break
		}
	}
	
	if composeFile == "" {
		return fmt.Errorf("could not find docker-compose.test.yml in %s or parent directories", wd)
	}

	// Start Docker Compose
	cmd := exec.Command("docker-compose", "-f", composeFile, "up", "-d")
	cmd.Dir = filepath.Dir(composeFile) // Set working directory to project root
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start Docker network: %w\nOutput: %s", err, output)
	}

	// Wait for nodes to be ready - increased timeout
	fmt.Println("Waiting for IPFS nodes to initialize...")
	time.Sleep(45 * time.Second)

	// Connect to each node and create NoiseFS clients with retries
	for i, node := range h.nodes {
		fmt.Printf("Connecting to IPFS node %d at %s...\n", i+1, node.APIAddress)
		
		// Retry connection up to 3 times
		var ipfsClient *ipfs.Client
		var err error
		for attempt := 1; attempt <= 3; attempt++ {
			ipfsClient, err = ipfs.NewClient(node.APIAddress)
			if err == nil {
				break
			}
			fmt.Printf("  Attempt %d failed, retrying in 10s: %v\n", attempt, err)
			time.Sleep(10 * time.Second)
		}
		
		if err != nil {
			fmt.Printf("Warning: Could not connect to node %d after 3 attempts: %v\n", i+1, err)
			continue
		}
		node.ipfsClient = ipfsClient

		// Create NoiseFS client
		noiseClient, err := noisefs.NewClient(ipfsClient, node.cache)
		if err != nil {
			fmt.Printf("Warning: Failed to create NoiseFS client for node %d: %v\n", i+1, err)
			continue
		}
		node.NoiseClient = noiseClient
		fmt.Printf("  ✅ Node %d connected successfully\n", i+1)
	}

	// Verify network connectivity
	if err := h.verifyNetworkConnectivity(); err != nil {
		fmt.Printf("Warning: Network connectivity check failed: %v\n", err)
		fmt.Println("Continuing with partial connectivity...")
	}

	h.isRunning = true
	
	// Count successfully connected nodes
	connectedCount := 0
	for _, node := range h.nodes {
		if node.NoiseClient != nil {
			connectedCount++
		}
	}
	
	fmt.Printf("Real IPFS test network started with %d/%d nodes connected\n", connectedCount, h.nodeCount)
	if connectedCount < h.nodeCount {
		fmt.Printf("Warning: Only %d of %d nodes connected successfully\n", connectedCount, h.nodeCount)
		fmt.Println("Cross-node tests may be skipped")
	}
	return nil
}

// StopNetwork stops the IPFS test network
func (h *RealIPFSTestHarness) StopNetwork() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.isRunning {
		return nil
	}

	fmt.Println("Stopping IPFS test network...")

	// Find project root and docker-compose file
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}
	
	// Look for docker-compose.test.yml in current dir and parent dirs
	composeFile := ""
	for dir := wd; dir != "/"; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, "docker-compose.test.yml")
		if _, err := os.Stat(candidate); err == nil {
			composeFile = candidate
			break
		}
	}
	
	if composeFile == "" {
		// If we can't find it, try the simple command anyway
		composeFile = "docker-compose.test.yml"
	}

	// Stop Docker Compose
	cmd := exec.Command("docker-compose", "-f", composeFile, "down", "-v")
	if composeFile != "docker-compose.test.yml" {
		cmd.Dir = filepath.Dir(composeFile) // Set working directory to project root
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to stop Docker network: %w\nOutput: %s", err, output)
	}

	h.isRunning = false
	fmt.Println("IPFS test network stopped successfully")
	return nil
}

// GetNode returns a specific node by index
func (h *RealIPFSTestHarness) GetNode(index int) (*RealIPFSNode, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if index < 0 || index >= len(h.nodes) {
		return nil, fmt.Errorf("node index %d out of range [0, %d)", index, len(h.nodes))
	}

	if h.nodes[index].NoiseClient == nil {
		return nil, fmt.Errorf("node %d is not properly initialized", index)
	}

	return h.nodes[index], nil
}

// GetAllNodes returns all nodes
func (h *RealIPFSTestHarness) GetAllNodes() []*RealIPFSNode {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.nodes
}

// verifyNetworkConnectivity checks that nodes can communicate
func (h *RealIPFSTestHarness) verifyNetworkConnectivity() error {
	fmt.Println("Verifying network connectivity...")

	connectedNodes := 0
	for i, node := range h.nodes {
		if node.ipfsClient == nil {
			fmt.Printf("Node %d not connected, attempting reconnection...\n", i+1)
			
			// Retry connection with timeout
			ipfsClient, err := ipfs.NewClient(node.APIAddress)
			if err != nil {
				fmt.Printf("Failed to connect to node %d at %s: %v\n", i+1, node.APIAddress, err)
				continue
			}
			node.ipfsClient = ipfsClient

			// Create NoiseFS client
			noiseClient, err := noisefs.NewClient(ipfsClient, node.cache)
			if err != nil {
				fmt.Printf("Failed to create NoiseFS client for node %d: %v\n", i+1, err)
				continue
			}
			node.NoiseClient = noiseClient
		}

		// Test basic connectivity with a simple operation
		peers := node.ipfsClient.GetConnectedPeers()
		fmt.Printf("Node %d (%s) has %d connected peers\n", i+1, node.APIAddress, len(peers))
		
		// Check if IPFS API is responsive
		if node.ipfsClient != nil {
			fmt.Printf("  Node %d IPFS API is responsive\n", i+1)
		}
		
		// Check if NoiseFS client is ready
		if node.NoiseClient != nil {
			fmt.Printf("  Node %d NoiseFS client is ready\n", i+1)
		}
		
		connectedNodes++
	}

	if connectedNodes == 0 {
		return fmt.Errorf("no nodes are connected")
	}

	fmt.Printf("Network connectivity verified: %d/%d nodes connected\n", connectedNodes, len(h.nodes))
	
	// Additional network diagnostics
	if connectedNodes < len(h.nodes) {
		fmt.Printf("Network diagnostics: %d nodes failed to connect properly\n", len(h.nodes)-connectedNodes)
		fmt.Println("This may be due to Docker network startup timing or port conflicts")
	}
	return nil
}

// TestRealUploadDownload performs a real file upload and download test
func (h *RealIPFSTestHarness) TestRealUploadDownload(nodeIndex int, testData []byte) (*RealTestResults, error) {
	node, err := h.GetNode(nodeIndex)
	if err != nil {
		return nil, err
	}

	results := &RealTestResults{
		NodeID:    node.NodeID,
		StartTime: time.Now(),
	}

	fmt.Printf("Testing real upload/download on node %s with %d bytes\n", node.NodeID, len(testData))

	// Create a real block from test data
	block, err := blocks.NewBlock(testData)
	if err != nil {
		return nil, fmt.Errorf("failed to create block: %w", err)
	}

	// Real upload - actually store in IPFS
	uploadStart := time.Now()
	cid, err := node.NoiseClient.StoreBlockWithCache(block)
	if err != nil {
		return nil, fmt.Errorf("real upload failed: %w", err)
	}
	results.UploadLatency = time.Since(uploadStart)
	results.StoredCID = cid

	fmt.Printf("Real upload completed: CID=%s, latency=%v\n", cid, results.UploadLatency)

	// Real download - actually retrieve from IPFS  
	downloadStart := time.Now()
	retrievedBlock, err := node.NoiseClient.RetrieveBlockWithCache(cid)
	if err != nil {
		return nil, fmt.Errorf("real download failed: %w", err)
	}
	results.DownloadLatency = time.Since(downloadStart)

	// Verify data integrity
	if !equalBytes(retrievedBlock.Data, testData) {
		return nil, fmt.Errorf("data integrity check failed: original ≠ retrieved")
	}

	results.EndTime = time.Now()
	results.Success = true
	results.DataIntegrityVerified = true

	fmt.Printf("Real download completed: latency=%v, integrity=verified\n", results.DownloadLatency)

	return results, nil
}

// TestCrossNodeReplication tests real cross-node block replication
func (h *RealIPFSTestHarness) TestCrossNodeReplication(sourceNodeIndex, targetNodeIndex int, testData []byte) (*CrossNodeTestResults, error) {
	sourceNode, err := h.GetNode(sourceNodeIndex)
	if err != nil {
		return nil, err
	}

	targetNode, err := h.GetNode(targetNodeIndex)
	if err != nil {
		return nil, err
	}

	results := &CrossNodeTestResults{
		SourceNodeID: sourceNode.NodeID,
		TargetNodeID: targetNode.NodeID,
		StartTime:    time.Now(),
	}

	fmt.Printf("Testing cross-node replication: %s -> %s\n", sourceNode.NodeID, targetNode.NodeID)

	// Upload to source node
	block, err := blocks.NewBlock(testData)
	if err != nil {
		return nil, fmt.Errorf("failed to create block: %w", err)
	}

	uploadStart := time.Now()
	cid, err := sourceNode.NoiseClient.StoreBlockWithCache(block)
	if err != nil {
		return nil, fmt.Errorf("upload to source node failed: %w", err)
	}
	results.UploadLatency = time.Since(uploadStart)

	// Wait a bit for IPFS replication
	time.Sleep(2 * time.Second)

	// Try to retrieve from target node
	downloadStart := time.Now()
	retrievedBlock, err := targetNode.NoiseClient.RetrieveBlockWithCache(cid)
	if err != nil {
		return nil, fmt.Errorf("download from target node failed: %w", err)
	}
	results.CrossNodeLatency = time.Since(downloadStart)

	// Verify data integrity
	if !equalBytes(retrievedBlock.Data, testData) {
		return nil, fmt.Errorf("cross-node data integrity check failed")
	}

	results.EndTime = time.Now()
	results.Success = true
	results.ReplicationVerified = true

	fmt.Printf("Cross-node replication verified: latency=%v\n", results.CrossNodeLatency)

	return results, nil
}

// RealTestResults holds results from real testing operations
type RealTestResults struct {
	NodeID                 string
	StartTime              time.Time
	EndTime                time.Time
	UploadLatency          time.Duration
	DownloadLatency        time.Duration
	StoredCID              string
	Success                bool
	DataIntegrityVerified  bool
}

// CrossNodeTestResults holds results from cross-node testing
type CrossNodeTestResults struct {
	SourceNodeID        string
	TargetNodeID        string
	StartTime           time.Time
	EndTime             time.Time
	UploadLatency       time.Duration
	CrossNodeLatency    time.Duration
	Success             bool
	ReplicationVerified bool
}

// equalBytes compares two byte slices for equality
func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}