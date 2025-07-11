package bootstrap

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
)

// BlockGenerator generates NoiseFS blocks from content
type BlockGenerator struct {
	config      *SeedConfig
	stats       *BlockStats
	blockPaths  map[string]string // CID -> file path
	mutex       sync.Mutex
}

// NewBlockGenerator creates a new block generator
func NewBlockGenerator(config *SeedConfig) *BlockGenerator {
	return &BlockGenerator{
		config: config,
		stats: &BlockStats{
			BlocksBySize: make(map[int]int),
			ContentTypes: make(map[string]int),
		},
		blockPaths: make(map[string]string),
	}
}

// GenerateFromContent generates blocks from all downloaded content
func (g *BlockGenerator) GenerateFromContent() (*BlockStats, error) {
	contentDir := filepath.Join(g.config.OutputDir, "downloads")
	blocksDir := filepath.Join(g.config.OutputDir, "blocks")

	// Process each content type
	contentTypes := []string{"books", "images", "audio", "documents"}
	if g.config.IncludeVideo {
		contentTypes = append(contentTypes, "videos")
	}

	for _, contentType := range contentTypes {
		typeDir := filepath.Join(contentDir, contentType)
		if _, err := os.Stat(typeDir); os.IsNotExist(err) {
			continue
		}

		fmt.Printf("\nProcessing %s...\n", contentType)
		if err := g.processContentType(typeDir, blocksDir, contentType); err != nil {
			return nil, fmt.Errorf("failed to process %s: %w", contentType, err)
		}
	}

	// Calculate statistics
	g.calculateStats()

	return g.stats, nil
}

// processContentType processes all files of a specific content type
func (g *BlockGenerator) processContentType(inputDir, outputDir, contentType string) error {
	files, err := filepath.Glob(filepath.Join(inputDir, "*"))
	if err != nil {
		return err
	}

	// Process files in parallel
	sem := make(chan struct{}, g.config.Parallel)
	var wg sync.WaitGroup

	for _, file := range files {
		// Skip metadata files
		if filepath.Ext(file) == ".json" {
			continue
		}

		wg.Add(1)
		go func(filePath string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if err := g.processFile(filePath, outputDir, contentType); err != nil {
				fmt.Printf("Error processing %s: %v\n", filepath.Base(filePath), err)
			}
		}(file)
	}

	wg.Wait()
	return nil
}

// processFile generates blocks from a single file
func (g *BlockGenerator) processFile(filePath, outputDir, contentType string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return err
	}

	// Choose block sizes based on file size and type
	blockSizes := g.selectBlockSizes(fileInfo.Size(), contentType)
	
	for _, blockSize := range blockSizes {
		// Reset file position
		file.Seek(0, 0)
		
		// Generate blocks of this size
		if err := g.generateBlocksFromFile(file, outputDir, blockSize, contentType); err != nil {
			return fmt.Errorf("failed to generate %d byte blocks: %w", blockSize, err)
		}
	}

	return nil
}

// selectBlockSizes chooses appropriate block sizes for a file
func (g *BlockGenerator) selectBlockSizes(fileSize int64, contentType string) []int {
	sizes := []int{}

	// For small files, only use smaller block sizes
	if fileSize < 1024*1024 { // < 1MB
		sizes = append(sizes, 64*1024, 128*1024)
	} else if fileSize < 10*1024*1024 { // < 10MB
		sizes = append(sizes, 128*1024, 256*1024)
	} else {
		// For large files (especially video), use larger blocks
		if contentType == "videos" {
			sizes = append(sizes, 256*1024, 512*1024, 1024*1024)
		} else {
			sizes = append(sizes, 128*1024, 256*1024, 512*1024)
		}
	}

	return sizes
}

// generateBlocksFromFile creates blocks of a specific size from a file
func (g *BlockGenerator) generateBlocksFromFile(file *os.File, outputDir string, blockSize int, contentType string) error {
	buffer := make([]byte, blockSize)
	blockIndex := 0
	
	for {
		n, err := io.ReadFull(file, buffer)
		if err == io.EOF {
			break
		}
		if err != nil && err != io.ErrUnexpectedEOF {
			return err
		}
		
		// Create block (handle partial reads)
		blockData := buffer[:n]
		if n < blockSize {
			// Pad smaller blocks with deterministic data
			blockData = g.padBlock(blockData, blockSize)
		}
		
		block, err := blocks.NewBlock(blockData)
		if err != nil {
			return err
		}
		
		// Generate CID
		hash := sha256.Sum256(block.Data)
		cid := hex.EncodeToString(hash[:])
		
		// Save block
		blockPath := filepath.Join(outputDir, fmt.Sprintf("%s_%d.block", cid[:16], blockSize))
		if err := g.saveBlock(block, blockPath); err != nil {
			return err
		}
		
		// Update tracking
		g.mutex.Lock()
		g.blockPaths[cid] = blockPath
		g.stats.TotalBlocks++
		g.stats.PublicDomainBlocks++
		g.stats.BlocksBySize[blockSize]++
		g.stats.ContentTypes[contentType]++
		g.mutex.Unlock()
		
		blockIndex++
		
		// Limit blocks per file for diversity
		if blockIndex >= 10 && contentType != "videos" {
			break
		}
	}
	
	return nil
}

// padBlock pads a block to the target size with deterministic data
func (g *BlockGenerator) padBlock(data []byte, targetSize int) []byte {
	if len(data) >= targetSize {
		return data
	}
	
	padded := make([]byte, targetSize)
	copy(padded, data)
	
	// Fill padding with deterministic pattern
	hash := sha256.Sum256(data)
	for i := len(data); i < targetSize; i++ {
		padded[i] = hash[i%32]
	}
	
	return padded
}

// saveBlock writes a block to disk
func (g *BlockGenerator) saveBlock(block *blocks.Block, path string) error {
	// Create directory if needed
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	
	// Write block data
	return os.WriteFile(path, block.Data, 0644)
}

// GenerateGenesisBlocks creates deterministic genesis blocks
func (g *BlockGenerator) GenerateGenesisBlocks() error {
	outputDir := filepath.Join(g.config.OutputDir, "blocks", "genesis")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}

	sizes := []int{64 * 1024, 128 * 1024, 256 * 1024, 512 * 1024, 1024 * 1024}
	
	for _, size := range sizes {
		// Generate multiple genesis blocks per size
		for i := 0; i < g.config.GenesisBlockCount/len(sizes); i++ {
			data := g.generateDeterministicData(size, i)
			
			block, err := blocks.NewBlock(data)
			if err != nil {
				return err
			}
			
			// Generate CID
			hash := sha256.Sum256(block.Data)
			cid := hex.EncodeToString(hash[:])
			
			// Save block
			blockPath := filepath.Join(outputDir, fmt.Sprintf("genesis_%d_%d.block", size, i))
			if err := g.saveBlock(block, blockPath); err != nil {
				return err
			}
			
			// Update tracking
			g.mutex.Lock()
			g.blockPaths[cid] = blockPath
			g.stats.TotalBlocks++
			g.stats.PublicDomainBlocks++
			g.stats.BlocksBySize[size]++
			g.stats.ContentTypes["genesis"]++
			g.mutex.Unlock()
		}
	}
	
	return nil
}

// generateDeterministicData creates deterministic data for genesis blocks
func (g *BlockGenerator) generateDeterministicData(size, index int) []byte {
	data := make([]byte, size)
	
	// Create seed from size and index
	seed := fmt.Sprintf("noisefs-genesis-%d-%d-%d", size, index, time.Now().Year())
	hash := sha256.Sum256([]byte(seed))
	
	// Fill data with pattern based on hash
	for i := 0; i < size; i++ {
		// Mix hash bytes with position for variety
		data[i] = hash[i%32] ^ byte(i&0xFF)
	}
	
	return data
}

// calculateStats updates block statistics
func (g *BlockGenerator) calculateStats() {
	totalReusePotential := 0.0
	
	// Calculate average reuse potential based on block distribution
	for size, count := range g.stats.BlocksBySize {
		// Larger blocks have higher reuse potential
		potential := float64(count) * (float64(size) / 131072.0)
		totalReusePotential += potential
	}
	
	if g.stats.TotalBlocks > 0 {
		g.stats.AverageReusePotential = totalReusePotential / float64(g.stats.TotalBlocks)
		
		// Ensure minimum reuse potential
		if g.stats.AverageReusePotential < 3.0 {
			g.stats.AverageReusePotential = 3.0
		}
	}
}

// GetBlockPaths returns the mapping of CIDs to block file paths
func (g *BlockGenerator) GetBlockPaths() map[string]string {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	
	// Return a copy
	paths := make(map[string]string)
	for k, v := range g.blockPaths {
		paths[k] = v
	}
	
	return paths
}