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

// generateInitialFiles creates initial files following realistic content patterns
func (sim *NetworkSimulation) generateInitialFiles() {
	fmt.Printf("Generating %d initial files with realistic content patterns...\n", sim.config.NumFiles)
	
	// Create common content patterns that will be shared across files
	commonPatterns := sim.generateCommonContentPatterns()
	
	for i := 0; i < sim.config.NumFiles; i++ {
		// Generate file with random size within range
		size := sim.config.FileSizeRange[0] + rand.Intn(sim.config.FileSizeRange[1]-sim.config.FileSizeRange[0])
		
		// Assign popularity using Zipf distribution
		popularity := sim.calculatePopularity(i)
		
		// Determine content type based on realistic distribution
		contentType := sim.selectContentType(i)
		
		file := &SimulatedFile{
			id:           fmt.Sprintf("file-%d", i),
			originalSize: int64(size),
			popularity:   popularity,
			uploadTime:   time.Now(),
		}
		
		// Generate blocks for file using realistic content patterns
		sim.generateRealisticBlocksForFile(file, contentType, commonPatterns)
		
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
		
		// Simulate block reuse probability based on file popularity (3-tuple)
		// With 3-tuple, we need 2 randomizers per block, so reuse is more likely
		reuseProb := file.popularity * 0.3 // Higher reuse probability for 3-tuple
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

// calculateStoredSize calculates total storage needed for a file (3-tuple implementation)
func (sim *NetworkSimulation) calculateStoredSize(file *SimulatedFile) int64 {
	var totalSize int64
	for _, block := range file.blocks {
		// 3-tuple storage calculation:
		// - Each block needs 2 randomizers
		// - With reuse, we approach the theoretical 1.5x overhead
		
		if block.isReused {
			// Reused blocks: randomizers are likely to be found in cache
			// Optimal case approaches 1.5x overhead as per OFFSystem theory
			totalSize += block.size * 15 / 10 // 1.5x
		} else {
			// New blocks: need to store new randomizers initially
			// But this will improve as the network grows and cache fills
			totalSize += block.size * 2 // Conservative 2x for new blocks
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

// ContentType represents different types of content
type ContentType int

const (
	TextDocument ContentType = iota
	MediaFile
	ArchiveFile
	CodeRepository
	ConfigFile
)

// CommonContentPattern represents shared content patterns
type CommonContentPattern struct {
	id      string
	content []byte
	usage   int // How many times this pattern has been used
}

// generateCommonContentPatterns creates realistic shared content
func (sim *NetworkSimulation) generateCommonContentPatterns() []*CommonContentPattern {
	patterns := []*CommonContentPattern{
		// Common file headers
		{id: "pdf-header", content: []byte("%PDF-1.4\n%âãÏÓ"), usage: 0},
		{id: "zip-header", content: []byte("PK\x03\x04"), usage: 0},
		{id: "jpeg-header", content: []byte("\xFF\xD8\xFF\xE0"), usage: 0},
		{id: "png-header", content: []byte("\x89PNG\r\n\x1a\n"), usage: 0},
		
		// Common text patterns
		{id: "lorem-ipsum", content: []byte("Lorem ipsum dolor sit amet, consectetur adipiscing elit. "), usage: 0},
		{id: "copyright", content: []byte("Copyright (c) 2024 All rights reserved. "), usage: 0},
		{id: "config-header", content: []byte("# Configuration File\n# Generated automatically\n"), usage: 0},
		
		// Common code patterns
		{id: "import-common", content: []byte("import (\n\t\"fmt\"\n\t\"log\"\n\t\"os\"\n)"), usage: 0},
		{id: "struct-common", content: []byte("type Config struct {\n\tHost string\n\tPort int\n}"), usage: 0},
		
		// Common data patterns
		{id: "json-template", content: []byte("{\"version\":\"1.0\",\"timestamp\":"), usage: 0},
		{id: "xml-header", content: []byte("<?xml version=\"1.0\" encoding=\"UTF-8\"?>"), usage: 0},
		
		// Padding patterns (very common in files)
		{id: "zeros", content: make([]byte, 64), usage: 0}, // 64 bytes of zeros
		{id: "spaces", content: []byte("                                                                "), usage: 0}, // 64 spaces
	}
	
	return patterns
}

// selectContentType determines content type based on realistic distribution
func (sim *NetworkSimulation) selectContentType(fileIndex int) ContentType {
	// Realistic distribution of file types
	switch fileIndex % 10 {
	case 0, 1, 2: // 30% text documents
		return TextDocument
	case 3, 4: // 20% media files
		return MediaFile
	case 5, 6: // 20% archives
		return ArchiveFile
	case 7, 8: // 20% code repositories
		return CodeRepository
	default: // 10% config files
		return ConfigFile
	}
}

// generateRealisticBlocksForFile creates blocks with realistic content patterns
func (sim *NetworkSimulation) generateRealisticBlocksForFile(file *SimulatedFile, contentType ContentType, patterns []*CommonContentPattern) {
	blockSize := int64(128 * 1024) // 128KB blocks
	numBlocks := (file.originalSize + blockSize - 1) / blockSize
	
	file.blocks = make([]SimulatedBlock, numBlocks)
	
	for i := int64(0); i < numBlocks; i++ {
		size := blockSize
		if i == numBlocks-1 {
			size = file.originalSize - (i * blockSize)
		}
		
		// Determine if this block should reuse existing content
		var isReused bool
		var blockId string
		
		// First block often contains headers/metadata (high reuse probability)
		if i == 0 {
			headerPattern := sim.selectHeaderPattern(contentType, patterns)
			if headerPattern != nil {
				blockId = fmt.Sprintf("shared-%s", headerPattern.id)
				isReused = true
				headerPattern.usage++
			}
		}
		
		// Common content blocks have higher reuse probability
		if !isReused {
			reuseProb := sim.calculateBlockReuseProb(file, contentType, i, numBlocks)
			if rand.Float64() < reuseProb {
				// Select a common pattern to reuse
				pattern := sim.selectCommonPattern(patterns, contentType)
				if pattern != nil {
					blockId = fmt.Sprintf("shared-%s-%d", pattern.id, pattern.usage/5) // Group by usage frequency
					isReused = true
					pattern.usage++
				}
			}
		}
		
		// If not reused, generate unique block ID
		if !isReused {
			blockId = fmt.Sprintf("unique-%s-%d", file.id, i)
		}
		
		file.blocks[i] = SimulatedBlock{
			id:       blockId,
			size:     size,
			isReused: isReused,
		}
	}
}

// selectHeaderPattern selects appropriate header pattern for content type
func (sim *NetworkSimulation) selectHeaderPattern(contentType ContentType, patterns []*CommonContentPattern) *CommonContentPattern {
	switch contentType {
	case MediaFile:
		// Randomly select image/media header
		options := []string{"pdf-header", "jpeg-header", "png-header"}
		selected := options[rand.Intn(len(options))]
		for _, pattern := range patterns {
			if pattern.id == selected {
				return pattern
			}
		}
	case ArchiveFile:
		for _, pattern := range patterns {
			if pattern.id == "zip-header" {
				return pattern
			}
		}
	case CodeRepository:
		for _, pattern := range patterns {
			if pattern.id == "import-common" {
				return pattern
			}
		}
	case ConfigFile:
		for _, pattern := range patterns {
			if pattern.id == "config-header" {
				return pattern
			}
		}
	}
	return nil
}

// selectCommonPattern selects a common content pattern for reuse
func (sim *NetworkSimulation) selectCommonPattern(patterns []*CommonContentPattern, contentType ContentType) *CommonContentPattern {
	// Filter patterns relevant to content type
	var candidates []*CommonContentPattern
	
	switch contentType {
	case TextDocument:
		for _, pattern := range patterns {
			if pattern.id == "lorem-ipsum" || pattern.id == "copyright" || pattern.id == "spaces" {
				candidates = append(candidates, pattern)
			}
		}
	case CodeRepository:
		for _, pattern := range patterns {
			if pattern.id == "struct-common" || pattern.id == "import-common" || pattern.id == "spaces" {
				candidates = append(candidates, pattern)
			}
		}
	case ConfigFile:
		for _, pattern := range patterns {
			if pattern.id == "json-template" || pattern.id == "xml-header" || pattern.id == "config-header" {
				candidates = append(candidates, pattern)
			}
		}
	default:
		// For other types, use padding patterns
		for _, pattern := range patterns {
			if pattern.id == "zeros" || pattern.id == "spaces" {
				candidates = append(candidates, pattern)
			}
		}
	}
	
	if len(candidates) == 0 {
		return nil
	}
	
	return candidates[rand.Intn(len(candidates))]
}

// calculateBlockReuseProb calculates probability of block reuse based on realistic factors
func (sim *NetworkSimulation) calculateBlockReuseProb(file *SimulatedFile, contentType ContentType, blockIndex, totalBlocks int64) float64 {
	baseProb := 0.1 // Base 10% chance
	
	// File popularity increases reuse probability
	popularityBonus := file.popularity * 0.4
	
	// Content type affects reuse probability
	var contentBonus float64
	switch contentType {
	case TextDocument:
		contentBonus = 0.3 // Text files often have repeated content
	case CodeRepository:
		contentBonus = 0.25 // Code has common patterns
	case ConfigFile:
		contentBonus = 0.4 // Config files are very repetitive
	case ArchiveFile:
		contentBonus = 0.1 // Archives are typically unique
	case MediaFile:
		contentBonus = 0.05 // Media files are mostly unique
	}
	
	// Position-based probability (middle blocks more likely to be padding/common content)
	var positionBonus float64
	if blockIndex > 0 && blockIndex < totalBlocks-1 {
		positionBonus = 0.2 // Middle blocks
	} else {
		positionBonus = 0.1 // First/last blocks
	}
	
	// 3-tuple implementation: higher reuse due to randomizer requirements
	tupleBonus := 0.15
	
	totalProb := baseProb + popularityBonus + contentBonus + positionBonus + tupleBonus
	
	// Cap at reasonable maximum
	if totalProb > 0.8 {
		totalProb = 0.8
	}
	
	return totalProb
}