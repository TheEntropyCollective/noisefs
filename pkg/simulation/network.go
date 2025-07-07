package simulation

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/cache"
	"github.com/TheEntropyCollective/noisefs/pkg/noisefs"
)

// NetworkSimulation represents a simulated NoiseFS network
type NetworkSimulation struct {
	nodes        []*SimulatedNode
	globalMetrics *GlobalMetrics
	config       *SimulationConfig
	mu           sync.RWMutex
}

// SimulationConfig defines parameters for network simulation
type SimulationConfig struct {
	NumNodes           int           // Number of nodes in the network
	CacheSize          int           // Cache size per node (number of blocks)
	NumFiles           int           // Number of files to simulate
	FileSizeRange      [2]int        // Min/max file sizes in bytes
	PopularityFactor   float64       // Zipf distribution parameter for file popularity
	SimulationDuration time.Duration // How long to run the simulation
	UploadRate         float64       // Files uploaded per second per node
	DownloadRate       float64       // Files downloaded per second per node
}

// SimulatedNode represents a single node in the NoiseFS network
type SimulatedNode struct {
	id      string
	cache   *cache.MemoryCache
	metrics *noisefs.Metrics
	files   map[string]*SimulatedFile // Files stored on this node
}

// SimulatedFile represents a file in the simulation
type SimulatedFile struct {
	id            string
	originalSize  int64
	blocks        []SimulatedBlock
	popularity    float64 // Higher values = more popular
	uploadTime    time.Time
	downloadCount int64
}

// SimulatedBlock represents a block in the simulation
type SimulatedBlock struct {
	id       string
	size     int64
	isReused bool // Whether this block was reused from cache
}

// GlobalMetrics tracks network-wide statistics
type GlobalMetrics struct {
	mu                    sync.RWMutex
	TotalNodes            int
	TotalFiles            int
	TotalBlocks           int64
	UniqueBlocks          int64
	TotalUploads          int64
	TotalDownloads        int64
	TotalBytesOriginal    int64
	TotalBytesStored      int64
	NetworkBlockReuseRate float64
	AverageLatency        time.Duration
	PeakMemoryUsage       int64
	NodeMetrics           map[string]*noisefs.MetricsSnapshot
}

// NewNetworkSimulation creates a new network simulation
func NewNetworkSimulation(config *SimulationConfig) *NetworkSimulation {
	sim := &NetworkSimulation{
		nodes:         make([]*SimulatedNode, config.NumNodes),
		globalMetrics: &GlobalMetrics{NodeMetrics: make(map[string]*noisefs.MetricsSnapshot)},
		config:        config,
	}

	// Initialize nodes
	for i := 0; i < config.NumNodes; i++ {
		sim.nodes[i] = &SimulatedNode{
			id:      fmt.Sprintf("node-%d", i),
			cache:   cache.NewMemoryCache(config.CacheSize),
			metrics: noisefs.NewMetrics(),
			files:   make(map[string]*SimulatedFile),
		}
	}

	return sim
}

// Run executes the network simulation
func (sim *NetworkSimulation) Run() error {
	fmt.Printf("Starting network simulation with %d nodes...\n", sim.config.NumNodes)
	
	// Generate initial file distribution
	sim.generateInitialFiles()
	
	// Run simulation
	start := time.Now()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sim.simulateActivity()
			
			if time.Since(start) >= sim.config.SimulationDuration {
				fmt.Println("Simulation completed")
				return nil
			}
		}
	}
}

// generateInitialFiles creates initial files following a popularity distribution
func (sim *NetworkSimulation) generateInitialFiles() {
	fmt.Printf("Generating %d initial files...\n", sim.config.NumFiles)
	
	for i := 0; i < sim.config.NumFiles; i++ {
		// Generate file with random size within range
		size := sim.config.FileSizeRange[0] + rand.Intn(sim.config.FileSizeRange[1]-sim.config.FileSizeRange[0])
		
		// Assign popularity using Zipf distribution
		popularity := sim.calculatePopularity(i)
		
		file := &SimulatedFile{
			id:           fmt.Sprintf("file-%d", i),
			originalSize: int64(size),
			popularity:   popularity,
			uploadTime:   time.Now(),
		}
		
		// Generate blocks for file
		sim.generateBlocksForFile(file)
		
		// Assign file to a random node
		nodeIndex := rand.Intn(len(sim.nodes))
		sim.nodes[nodeIndex].files[file.id] = file
		
		// Record upload
		sim.nodes[nodeIndex].metrics.RecordUpload(file.originalSize, sim.calculateStoredSize(file))
		
		// Add blocks to cache with some probability
		sim.addBlocksToCache(file, nodeIndex)
	}
}

// generateBlocksForFile creates blocks for a file with realistic reuse patterns
func (sim *NetworkSimulation) generateBlocksForFile(file *SimulatedFile) {
	blockSize := int64(128 * 1024) // 128KB blocks
	numBlocks := (file.originalSize + blockSize - 1) / blockSize
	
	file.blocks = make([]SimulatedBlock, numBlocks)
	
	for i := int64(0); i < numBlocks; i++ {
		size := blockSize
		if i == numBlocks-1 {
			size = file.originalSize - (i * blockSize)
		}
		
		// Simulate block reuse probability based on file popularity
		reuseProb := file.popularity * 0.1 // More popular files have higher reuse
		isReused := rand.Float64() < reuseProb
		
		file.blocks[i] = SimulatedBlock{
			id:       fmt.Sprintf("block-%s-%d", file.id, i),
			size:     size,
			isReused: isReused,
		}
	}
}

// calculatePopularity generates popularity using Zipf distribution
func (sim *NetworkSimulation) calculatePopularity(rank int) float64 {
	// Zipf distribution: popularity decreases with rank
	return 1.0 / (float64(rank+1) * sim.config.PopularityFactor)
}

// calculateStoredSize calculates total storage needed for a file
func (sim *NetworkSimulation) calculateStoredSize(file *SimulatedFile) int64 {
	var totalSize int64
	for _, block := range file.blocks {
		if !block.isReused {
			totalSize += block.size * 2 // Original block + randomizer
		} else {
			totalSize += block.size // Only original block (randomizer reused)
		}
	}
	return totalSize
}

// addBlocksToCache adds file blocks to various node caches
func (sim *NetworkSimulation) addBlocksToCache(file *SimulatedFile, originNode int) {
	for _, block := range file.blocks {
		// Create a simulated block
		simulatedBlock, err := blocks.NewBlock(make([]byte, block.size))
		if err != nil {
			continue
		}
		
		// Add to origin node cache
		sim.nodes[originNode].cache.Store(block.id, simulatedBlock)
		
		// Randomly distribute to other nodes based on popularity
		numCopies := int(file.popularity * 10) // More popular files get more copies
		for i := 0; i < numCopies && i < len(sim.nodes); i++ {
			nodeIndex := rand.Intn(len(sim.nodes))
			if nodeIndex != originNode {
				sim.nodes[nodeIndex].cache.Store(block.id, simulatedBlock)
			}
		}
	}
}

// simulateActivity performs one round of network activity
func (sim *NetworkSimulation) simulateActivity() {
	// Simulate uploads and downloads across nodes
	for _, node := range sim.nodes {
		// Simulate upload activity
		if rand.Float64() < sim.config.UploadRate/10.0 {
			sim.simulateUpload(node)
		}
		
		// Simulate download activity
		if rand.Float64() < sim.config.DownloadRate/10.0 {
			sim.simulateDownload(node)
		}
	}
}

// simulateUpload simulates a file upload by a node
func (sim *NetworkSimulation) simulateUpload(node *SimulatedNode) {
	// Generate a new file
	size := sim.config.FileSizeRange[0] + rand.Intn(sim.config.FileSizeRange[1]-sim.config.FileSizeRange[0])
	
	file := &SimulatedFile{
		id:           fmt.Sprintf("file-%s-%d", node.id, time.Now().UnixNano()),
		originalSize: int64(size),
		popularity:   rand.Float64(),
		uploadTime:   time.Now(),
	}
	
	sim.generateBlocksForFile(file)
	node.files[file.id] = file
	
	// Record metrics
	node.metrics.RecordUpload(file.originalSize, sim.calculateStoredSize(file))
	
	// Add to cache
	sim.addBlocksToCache(file, sim.getNodeIndex(node))
}

// simulateDownload simulates a file download by a node
func (sim *NetworkSimulation) simulateDownload(node *SimulatedNode) {
	// Select a random file to download
	allFiles := sim.getAllFiles()
	if len(allFiles) == 0 {
		return
	}
	
	// Select based on popularity
	file := sim.selectFileByPopularity(allFiles)
	file.downloadCount++
	
	// Simulate cache hits/misses
	for _, block := range file.blocks {
		if _, err := node.cache.Get(block.id); err == nil {
			node.metrics.RecordCacheHit()
		} else {
			node.metrics.RecordCacheMiss()
		}
	}
	
	node.metrics.RecordDownload()
}

// Helper functions

func (sim *NetworkSimulation) getNodeIndex(targetNode *SimulatedNode) int {
	for i, node := range sim.nodes {
		if node.id == targetNode.id {
			return i
		}
	}
	return 0
}

func (sim *NetworkSimulation) getAllFiles() []*SimulatedFile {
	var files []*SimulatedFile
	for _, node := range sim.nodes {
		for _, file := range node.files {
			files = append(files, file)
		}
	}
	return files
}

func (sim *NetworkSimulation) selectFileByPopularity(files []*SimulatedFile) *SimulatedFile {
	if len(files) == 0 {
		return nil
	}
	
	// Simple popularity-based selection
	totalPopularity := 0.0
	for _, file := range files {
		totalPopularity += file.popularity
	}
	
	target := rand.Float64() * totalPopularity
	current := 0.0
	
	for _, file := range files {
		current += file.popularity
		if current >= target {
			return file
		}
	}
	
	return files[0]
}

// GetGlobalMetrics calculates and returns network-wide metrics
func (sim *NetworkSimulation) GetGlobalMetrics() *GlobalMetrics {
	sim.mu.Lock()
	defer sim.mu.Unlock()
	
	metrics := &GlobalMetrics{
		TotalNodes:  len(sim.nodes),
		NodeMetrics: make(map[string]*noisefs.MetricsSnapshot),
	}
	
	var totalBlocks, uniqueBlocks int64
	var totalUploads, totalDownloads int64
	var totalBytesOriginal, totalBytesStored int64
	
	blockSet := make(map[string]bool)
	
	// Aggregate metrics from all nodes
	for _, node := range sim.nodes {
		nodeSnapshot := node.metrics.GetStats()
		metrics.NodeMetrics[node.id] = &nodeSnapshot
		
		totalUploads += nodeSnapshot.TotalUploads
		totalDownloads += nodeSnapshot.TotalDownloads
		totalBytesOriginal += nodeSnapshot.BytesUploadedOriginal
		totalBytesStored += nodeSnapshot.BytesStoredIPFS
		
		// Count unique blocks
		for _, file := range node.files {
			metrics.TotalFiles++
			for _, block := range file.blocks {
				totalBlocks++
				if !blockSet[block.id] {
					blockSet[block.id] = true
					uniqueBlocks++
				}
			}
		}
	}
	
	metrics.TotalBlocks = totalBlocks
	metrics.UniqueBlocks = uniqueBlocks
	metrics.TotalUploads = totalUploads
	metrics.TotalDownloads = totalDownloads
	metrics.TotalBytesOriginal = totalBytesOriginal
	metrics.TotalBytesStored = totalBytesStored
	
	// Calculate network-wide block reuse rate
	if totalBlocks > 0 {
		metrics.NetworkBlockReuseRate = float64(totalBlocks-uniqueBlocks) / float64(totalBlocks) * 100.0
	}
	
	return metrics
}