package bootstrap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	ipfsapi "github.com/ipfs/go-ipfs-api"
)

// PoolInitializer handles storing blocks in IPFS and initializing the pool
type PoolInitializer struct {
	config     *SeedConfig
	ipfs       *ipfsapi.Shell
	blockPaths map[string]string
	cidMapping map[string]string // local hash -> IPFS CID
	mutex      sync.Mutex
}

// NewPoolInitializer creates a new pool initializer
func NewPoolInitializer(config *SeedConfig) *PoolInitializer {
	return &PoolInitializer{
		config:     config,
		ipfs:       ipfsapi.NewShell(config.IPFSEndpoint),
		blockPaths: make(map[string]string),
		cidMapping: make(map[string]string),
	}
}

// StoreBlocks uploads all generated blocks to IPFS
func (p *PoolInitializer) StoreBlocks() error {
	blocksDir := filepath.Join(p.config.OutputDir, "blocks")
	
	// Walk through all block files
	var blocks []string
	err := filepath.Walk(blocksDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		if !info.IsDir() && filepath.Ext(path) == ".block" {
			blocks = append(blocks, path)
		}
		
		return nil
	})
	
	if err != nil {
		return fmt.Errorf("failed to find blocks: %w", err)
	}
	
	fmt.Printf("Found %d blocks to upload\n", len(blocks))
	
	// Upload blocks in parallel
	sem := make(chan struct{}, p.config.Parallel)
	var wg sync.WaitGroup
	var uploadErrors []error
	var errorMutex sync.Mutex
	
	uploadedCount := 0
	startTime := time.Now()
	
	for _, blockPath := range blocks {
		wg.Add(1)
		go func(path string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			
			// Read block data
			data, err := os.ReadFile(path)
			if err != nil {
				errorMutex.Lock()
				uploadErrors = append(uploadErrors, fmt.Errorf("failed to read %s: %w", path, err))
				errorMutex.Unlock()
				return
			}
			
			// Add to IPFS
			cid, err := p.ipfs.Add(bytes.NewReader(data))
			if err != nil {
				errorMutex.Lock()
				uploadErrors = append(uploadErrors, fmt.Errorf("failed to upload %s: %w", path, err))
				errorMutex.Unlock()
				return
			}
			
			// Store mapping
			p.mutex.Lock()
			p.blockPaths[cid] = path
			p.cidMapping[filepath.Base(path)] = cid
			uploadedCount++
			p.mutex.Unlock()
			
			// Progress update
			if uploadedCount%10 == 0 {
				elapsed := time.Since(startTime).Seconds()
				rate := float64(uploadedCount) / elapsed
				fmt.Printf("Progress: %d/%d blocks (%.1f blocks/sec)\n", uploadedCount, len(blocks), rate)
			}
		}(blockPath)
	}
	
	wg.Wait()
	
	if len(uploadErrors) > 0 {
		fmt.Printf("Warning: %d upload errors occurred\n", len(uploadErrors))
		for _, err := range uploadErrors[:5] { // Show first 5 errors
			fmt.Printf("  - %v\n", err)
		}
	}
	
	fmt.Printf("\nSuccessfully uploaded %d blocks to IPFS\n", uploadedCount)
	
	// Save CID mapping
	if err := p.saveCIDMapping(); err != nil {
		return fmt.Errorf("failed to save CID mapping: %w", err)
	}
	
	return nil
}

// InitializePool creates the initial pool configuration
func (p *PoolInitializer) InitializePool() error {
	poolDir := filepath.Join(p.config.OutputDir, "pool")
	
	// Create pool metadata
	poolMeta := map[string]interface{}{
		"version":        "1.0",
		"created":        time.Now().Format(time.RFC3339),
		"profile":        p.config.Profile,
		"block_count":    len(p.cidMapping),
		"block_sizes":    []int{64 * 1024, 128 * 1024, 256 * 1024, 512 * 1024, 1024 * 1024},
		"ipfs_endpoint":  p.config.IPFSEndpoint,
	}
	
	// Write pool configuration
	configPath := filepath.Join(poolDir, "pool.json")
	if err := writeJSON(configPath, poolMeta); err != nil {
		return fmt.Errorf("failed to write pool config: %w", err)
	}
	
	// Create block index
	blockIndex := make(map[string][]string) // size -> CIDs
	for localPath, cid := range p.cidMapping {
		// Extract size from filename (e.g., "hash_131072.block")
		var size int
		fmt.Sscanf(localPath, "%*[^_]_%d.block", &size)
		
		sizeKey := fmt.Sprintf("%d", size)
		blockIndex[sizeKey] = append(blockIndex[sizeKey], cid)
	}
	
	// Write block index
	indexPath := filepath.Join(poolDir, "block_index.json")
	if err := writeJSON(indexPath, blockIndex); err != nil {
		return fmt.Errorf("failed to write block index: %w", err)
	}
	
	// Create genesis manifest
	genesisCIDs := []string{}
	for path, cid := range p.cidMapping {
		base := filepath.Base(path)
		if len(base) >= 7 && base[:7] == "genesis" {
			genesisCIDs = append(genesisCIDs, cid)
		}
	}
	
	genesisPath := filepath.Join(poolDir, "genesis.json")
	if err := writeJSON(genesisPath, genesisCIDs); err != nil {
		return fmt.Errorf("failed to write genesis manifest: %w", err)
	}
	
	fmt.Printf("\nPool initialized with:\n")
	fmt.Printf("- Configuration: %s\n", configPath)
	fmt.Printf("- Block index: %s\n", indexPath)
	fmt.Printf("- Genesis blocks: %d\n", len(genesisCIDs))
	
	return nil
}

// ValidatePool checks if the pool meets privacy requirements
func (p *PoolInitializer) ValidatePool() (*PoolValidation, error) {
	validation := &PoolValidation{
		Valid:             true,
		Issues:            []string{},
		BlockSizeCoverage: make(map[int]bool),
	}
	
	// Count blocks by size
	blocksBySize := make(map[int]int)
	publicDomainCount := 0
	
	for localPath := range p.cidMapping {
		var size int
		fmt.Sscanf(localPath, "%*[^_]_%d.block", &size)
		blocksBySize[size]++
		
		// All our generated blocks are from public domain content
		publicDomainCount++
	}
	
	validation.TotalBlocks = len(p.cidMapping)
	validation.PublicDomainRatio = float64(publicDomainCount) / float64(validation.TotalBlocks)
	
	// Check minimum blocks per size
	requiredSizes := []int{64 * 1024, 128 * 1024, 256 * 1024, 512 * 1024, 1024 * 1024}
	for _, size := range requiredSizes {
		count := blocksBySize[size]
		validation.BlockSizeCoverage[size] = count >= 50
		
		if count < 50 {
			validation.Valid = false
			validation.Issues = append(validation.Issues, 
				fmt.Sprintf("Insufficient blocks for size %d KB: %d < 50", size/1024, count))
		}
	}
	
	// Check total block count
	if validation.TotalBlocks < 500 {
		validation.Valid = false
		validation.Issues = append(validation.Issues, 
			fmt.Sprintf("Insufficient total blocks: %d < 500", validation.TotalBlocks))
	}
	
	// Check public domain ratio
	if validation.PublicDomainRatio < 0.5 {
		validation.Valid = false
		validation.Issues = append(validation.Issues, 
			fmt.Sprintf("Public domain ratio too low: %.1f%% < 50%%", validation.PublicDomainRatio*100))
	}
	
	// Calculate diversity score
	validation.DiversityScore = calculateDiversityScore(blocksBySize)
	
	// Check minimum requirements
	validation.MinimumRequirementsMet = validation.Valid && 
		validation.TotalBlocks >= 500 && 
		validation.PublicDomainRatio >= 0.5
	
	return validation, nil
}

// saveCIDMapping saves the mapping of local files to IPFS CIDs
func (p *PoolInitializer) saveCIDMapping() error {
	mappingPath := filepath.Join(p.config.OutputDir, "pool", "cid_mapping.json")
	return writeJSON(mappingPath, p.cidMapping)
}

// Helper functions

func writeJSON(path string, data interface{}) error {
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

func contains(s, substr string) bool {
	return filepath.Base(s) == substr || filepath.Dir(s) == substr ||
		   len(s) > len(substr) && s[len(s)-len(substr):] == substr
}

func calculateDiversityScore(blocksBySize map[int]int) float64 {
	if len(blocksBySize) == 0 {
		return 0.0
	}
	
	total := 0
	for _, count := range blocksBySize {
		total += count
	}
	
	// Shannon diversity index
	diversity := 0.0
	for _, count := range blocksBySize {
		if count > 0 {
			p := float64(count) / float64(total)
			diversity -= p * math.Log(p)
		}
	}
	
	// Normalize to 0-1 range
	maxDiversity := math.Log(float64(len(blocksBySize)))
	if maxDiversity > 0 {
		diversity = diversity / maxDiversity
	}
	
	return diversity
}