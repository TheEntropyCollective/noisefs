package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	noisefs "github.com/TheEntropyCollective/noisefs/pkg/core/client"
	"github.com/TheEntropyCollective/noisefs/pkg/core/descriptors"
	"github.com/TheEntropyCollective/noisefs/pkg/fuse"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/config"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/logging"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/ipfs"
)

func main() {
	var (
		sourceDir    = flag.String("source", "", "Source directory to index (required)")
		indexFile    = flag.String("index", "", "Output index file path (default: source_dir.noisefs)")
		configFile   = flag.String("config", "", "Configuration file path")
		ipfsAPI      = flag.String("ipfs", "", "IPFS API endpoint (default: 127.0.0.1:5001)")
		blockSize    = flag.Int("block-size", 0, "Block size in bytes (default: from config)")
		cacheSize    = flag.Int("cache-size", 0, "Cache size in blocks (default: from config)")
		includeExt   = flag.String("include", "", "Include only files with these extensions (comma-separated)")
		excludeExt   = flag.String("exclude", "", "Exclude files with these extensions (comma-separated)")
		maxFileSize  = flag.Int64("max-size", 0, "Maximum file size in bytes (0 = no limit)")
		verbose      = flag.Bool("verbose", false, "Enable verbose output")
		dryRun       = flag.Bool("dry-run", false, "Show what would be uploaded without actually uploading")
		recursive    = flag.Bool("recursive", true, "Process directories recursively")
		showProgress = flag.Bool("progress", true, "Show upload progress")
	)
	flag.Parse()

	if *sourceDir == "" {
		fmt.Fprintf(os.Stderr, "Error: Source directory is required\n")
		flag.Usage()
		os.Exit(1)
	}

	// Validate source directory
	sourceInfo, err := os.Stat(*sourceDir)
	if err != nil {
		log.Fatalf("Error accessing source directory: %v", err)
	}
	if !sourceInfo.IsDir() {
		log.Fatalf("Source path is not a directory: %s", *sourceDir)
	}

	// Set default index file name
	if *indexFile == "" {
		*indexFile = filepath.Base(*sourceDir) + ".noisefs"
	}

	// Load configuration
	cfg, err := loadConfig(*configFile)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Apply command-line overrides
	if *ipfsAPI != "" {
		cfg.IPFS.APIEndpoint = *ipfsAPI
	}
	if *blockSize != 0 {
		cfg.Performance.BlockSize = *blockSize
	}
	if *cacheSize != 0 {
		cfg.Cache.BlockCacheSize = *cacheSize
	}

	// Initialize logging
	logLevel := "info"
	if *verbose {
		logLevel = "debug"
	}
	if err := logging.InitFromConfig(logLevel, "text", "stdout", ""); err != nil {
		log.Fatalf("Failed to initialize logging: %v", err)
	}
	logger := logging.GetGlobalLogger().WithComponent("directory-indexer")

	fmt.Printf("🗂️  NoiseFS Directory Indexer\n")
	fmt.Printf("============================\n")
	fmt.Printf("Source Directory: %s\n", *sourceDir)
	fmt.Printf("Index File: %s\n", *indexFile)
	fmt.Printf("IPFS Endpoint: %s\n", cfg.IPFS.APIEndpoint)
	fmt.Printf("Block Size: %d bytes\n", cfg.Performance.BlockSize)
	fmt.Printf("Dry Run: %t\n", *dryRun)
	fmt.Printf("\n")

	// Collect files to process
	files, err := collectFiles(*sourceDir, *includeExt, *excludeExt, *maxFileSize, *recursive, logger)
	if err != nil {
		log.Fatalf("Failed to collect files: %v", err)
	}

	fmt.Printf("Found %d files to process\n\n", len(files))

	if *dryRun {
		fmt.Printf("📋 Dry run - files that would be uploaded:\n")
		for _, file := range files {
			fmt.Printf("  %s (%d bytes)\n", file.RelPath, file.Size)
		}
		fmt.Printf("\nTotal: %d files, %.2f MB\n", len(files), float64(getTotalSize(files))/(1024*1024))
		return
	}

	// Create IPFS client
	logger.Info("Connecting to IPFS", map[string]interface{}{
		"endpoint": cfg.IPFS.APIEndpoint,
	})
	ipfsClient, err := ipfs.NewClient(cfg.IPFS.APIEndpoint)
	if err != nil {
		log.Fatalf("Failed to connect to IPFS: %v", err)
	}

	// Create cache and client
	blockCache := cache.NewMemoryCache(cfg.Cache.BlockCacheSize)
	noisefsClient, err := noisefs.NewClient(ipfsClient, blockCache)
	if err != nil {
		log.Fatalf("Failed to create NoiseFS client: %v", err)
	}

	// Create index
	index := fuse.NewFileIndex(*indexFile)

	// Upload files and build index
	startTime := time.Now()
	fmt.Printf("🚀 Starting upload process...\n\n")

	for i, file := range files {
		if *showProgress {
			fmt.Printf("[%d/%d] Uploading: %s", i+1, len(files), file.RelPath)
		}

		// Upload file
		descriptorCID, err := uploadFile(noisefsClient, file.FullPath, file.RelPath, cfg.Performance.BlockSize, logger)
		if err != nil {
			fmt.Printf(" ❌ FAILED\n")
			logger.Error("Failed to upload file", map[string]interface{}{
				"file":  file.RelPath,
				"error": err.Error(),
			})
			continue
		}

		// Add to index
		index.AddFile(file.RelPath, descriptorCID, file.Size)

		if *showProgress {
			fmt.Printf(" ✅ %s\n", descriptorCID[:12]+"...")
		}
	}

	// Save index
	fmt.Printf("\n💾 Saving index file...\n")
	if err := index.SaveIndex(); err != nil {
		log.Fatalf("Failed to save index: %v", err)
	}

	// Show summary
	duration := time.Since(startTime)
	metrics := noisefsClient.GetMetrics()

	fmt.Printf("\n🎉 Directory indexing completed!\n")
	fmt.Printf("================================\n")
	fmt.Printf("Index File: %s\n", *indexFile)
	fmt.Printf("Files Processed: %d\n", len(files))
	fmt.Printf("Total Size: %.2f MB\n", float64(getTotalSize(files))/(1024*1024))
	fmt.Printf("Duration: %v\n", duration.Round(time.Second))
	fmt.Printf("Cache Hit Rate: %.1f%%\n", metrics.CacheHitRate)
	fmt.Printf("Blocks Generated: %d\n", metrics.BlocksGenerated)
	fmt.Printf("Blocks Reused: %d\n", metrics.BlocksReused)
	fmt.Printf("\n📤 Share this index file with others to give them access to your directory!\n")
}

type FileInfo struct {
	FullPath string
	RelPath  string
	Size     int64
}

func collectFiles(sourceDir, includeExt, excludeExt string, maxFileSize int64, recursive bool, logger *logging.Logger) ([]FileInfo, error) {
	var files []FileInfo

	includeExtMap := make(map[string]bool)
	excludeExtMap := make(map[string]bool)

	if includeExt != "" {
		for _, ext := range strings.Split(includeExt, ",") {
			includeExtMap[strings.TrimSpace(ext)] = true
		}
	}

	if excludeExt != "" {
		for _, ext := range strings.Split(excludeExt, ",") {
			excludeExtMap[strings.TrimSpace(ext)] = true
		}
	}

	walkFunc := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			logger.Warn("Error accessing file", map[string]interface{}{
				"path":  path,
				"error": err.Error(),
			})
			return nil // Continue walking
		}

		// Skip directories
		if info.IsDir() {
			if !recursive && path != sourceDir {
				return filepath.SkipDir
			}
			return nil
		}

		// Check file size limit
		if maxFileSize > 0 && info.Size() > maxFileSize {
			logger.Debug("Skipping large file", map[string]interface{}{
				"file":  path,
				"size":  info.Size(),
				"limit": maxFileSize,
			})
			return nil
		}

		// Check file extension filters
		ext := strings.ToLower(filepath.Ext(path))

		if len(includeExtMap) > 0 && !includeExtMap[ext] {
			return nil
		}

		if excludeExtMap[ext] {
			return nil
		}

		// Calculate relative path
		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			logger.Warn("Failed to calculate relative path", map[string]interface{}{
				"path":  path,
				"error": err.Error(),
			})
			return nil
		}

		files = append(files, FileInfo{
			FullPath: path,
			RelPath:  relPath,
			Size:     info.Size(),
		})

		return nil
	}

	if err := filepath.Walk(sourceDir, walkFunc); err != nil {
		return nil, err
	}

	return files, nil
}

func getTotalSize(files []FileInfo) int64 {
	var total int64
	for _, file := range files {
		total += file.Size
	}
	return total
}

func uploadFile(client *noisefs.Client, filePath, relativePath string, blockSize int, logger *logging.Logger) (string, error) {
	// Read file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	// Create splitter
	splitter, err := blocks.NewSplitter(blockSize)
	if err != nil {
		return "", fmt.Errorf("failed to create splitter: %w", err)
	}

	// Split file into blocks
	fileBlocks, err := splitter.SplitBytes(content)
	if err != nil {
		return "", fmt.Errorf("failed to split file: %w", err)
	}

	// Create descriptor
	descriptor := descriptors.NewDescriptor(relativePath, int64(len(content)), blockSize)

	// Process each block with 3-tuple anonymization
	for _, dataBlock := range fileBlocks {
		// Select randomizers
		rand1, cid1, rand2, cid2, err := client.SelectTwoRandomizers(dataBlock.Size())
		if err != nil {
			return "", fmt.Errorf("failed to select randomizers: %w", err)
		}

		// Anonymize block (XOR with two randomizers)
		anonymizedBlock, err := dataBlock.XOR3(rand1, rand2)
		if err != nil {
			return "", fmt.Errorf("failed to anonymize block: %w", err)
		}

		// Store anonymized block
		dataCID, err := client.StoreBlockWithCache(anonymizedBlock)
		if err != nil {
			return "", fmt.Errorf("failed to store block: %w", err)
		}

		// Add to descriptor
		if err := descriptor.AddBlockTriple(dataCID, cid1, cid2); err != nil {
			return "", fmt.Errorf("failed to add block to descriptor: %w", err)
		}
	}

	// Store descriptor
	descriptorData, err := descriptor.ToJSON()
	if err != nil {
		return "", fmt.Errorf("failed to serialize descriptor: %w", err)
	}

	descriptorBlock, err := blocks.NewBlock(descriptorData)
	if err != nil {
		return "", fmt.Errorf("failed to create descriptor block: %w", err)
	}

	descriptorCID, err := client.StoreBlockWithCache(descriptorBlock)
	if err != nil {
		return "", fmt.Errorf("failed to store descriptor: %w", err)
	}

	return descriptorCID, nil
}

func loadConfig(configPath string) (*config.Config, error) {
	if configPath == "" {
		defaultPath, err := config.GetDefaultConfigPath()
		if err == nil {
			configPath = defaultPath
		}
	}

	return config.LoadConfig(configPath)
}
