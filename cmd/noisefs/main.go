package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/config"
	"github.com/TheEntropyCollective/noisefs/pkg/core/descriptors"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/ipfs"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/logging"
	"github.com/TheEntropyCollective/noisefs/pkg/core/client"
	"github.com/TheEntropyCollective/noisefs/pkg/util"
	shell "github.com/ipfs/go-ipfs-api"
)

func main() {
	var (
		configFile = flag.String("config", "", "Configuration file path")
		ipfsAPI    = flag.String("api", "", "IPFS API endpoint (overrides config)")
		upload     = flag.String("upload", "", "File to upload to NoiseFS")
		download   = flag.String("download", "", "Descriptor CID to download from NoiseFS")
		output     = flag.String("output", "", "Output file path for download")
		stats      = flag.Bool("stats", false, "Show NoiseFS statistics")
		quiet      = flag.Bool("quiet", false, "Minimal output (only show errors and results)")
		jsonOutput = flag.Bool("json", false, "Output results in JSON format")
		blockSize  = flag.Int("block-size", 0, "Block size in bytes (overrides config)")
		cacheSize  = flag.Int("cache-size", 0, "Number of blocks to cache in memory (overrides config)")
		// Altruistic cache flags
		minPersonalCacheMB = flag.Int("min-personal-cache", 0, "Minimum personal cache size in MB (overrides config)")
		disableAltruistic = flag.Bool("disable-altruistic", false, "Disable altruistic caching")
		altruisticBandwidthMB = flag.Int("altruistic-bandwidth", 0, "Bandwidth limit for altruistic operations in MB/s")
	)
	
	// Check for subcommands first
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "announce", "subscribe", "discover":
			handleSubcommand(os.Args[1], os.Args[2:])
			return
		}
	}
	
	flag.Parse()
	
	// Load configuration
	cfg, err := loadConfig(*configFile)
	if err != nil {
		if *jsonOutput {
			util.PrintJSONError(err)
		} else {
			fmt.Fprintf(os.Stderr, "%s\n", util.FormatError(err))
		}
		os.Exit(1)
	}

	// Initialize logging
	if err := logging.InitFromConfig(cfg.Logging.Level, cfg.Logging.Format, cfg.Logging.Output, cfg.Logging.File); err != nil {
		if *jsonOutput {
			util.PrintJSONError(err)
		} else {
			fmt.Fprintf(os.Stderr, "%s\n", util.FormatError(err))
		}
		os.Exit(1)
	}
	
	logger := logging.GetGlobalLogger().WithComponent("noisefs")
	
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
	// Apply altruistic cache overrides
	if *minPersonalCacheMB > 0 {
		cfg.Cache.MinPersonalCacheMB = *minPersonalCacheMB
	}
	if *disableAltruistic {
		cfg.Cache.EnableAltruistic = false
	}
	if *altruisticBandwidthMB > 0 {
		cfg.Cache.AltruisticBandwidthMB = *altruisticBandwidthMB
	}
	
	// Create IPFS client
	logger.Info("Connecting to IPFS", map[string]interface{}{
		"endpoint": cfg.IPFS.APIEndpoint,
	})
	ipfsClient, err := ipfs.NewClient(cfg.IPFS.APIEndpoint)
	if err != nil {
		logger.Error("Failed to connect to IPFS", map[string]interface{}{
			"endpoint": cfg.IPFS.APIEndpoint,
			"error":    err.Error(),
		})
		if *jsonOutput {
			util.PrintJSONError(err)
		} else {
			fmt.Fprintf(os.Stderr, "%s\n", util.FormatError(err))
		}
		os.Exit(1)
	}
	
	// Create cache
	logger.Debug("Initializing block cache", map[string]interface{}{
		"cache_size": cfg.Cache.BlockCacheSize,
		"altruistic_enabled": cfg.Cache.EnableAltruistic,
	})
	
	var blockCache cache.Cache
	baseCache := cache.NewMemoryCache(cfg.Cache.BlockCacheSize)
	
	// Wrap with altruistic cache if enabled
	if cfg.Cache.EnableAltruistic && cfg.Cache.MinPersonalCacheMB > 0 {
		altruisticConfig := &cache.AltruisticCacheConfig{
			MinPersonalCache:      int64(cfg.Cache.MinPersonalCacheMB) * 1024 * 1024,
			EnableAltruistic:      true,
			AltruisticBandwidthMB: cfg.Cache.AltruisticBandwidthMB,
		}
		
		// Calculate total capacity based on memory limit or default
		totalCapacity := int64(cfg.Cache.MemoryLimit) * 1024 * 1024
		if totalCapacity == 0 {
			totalCapacity = int64(cfg.Cache.BlockCacheSize) * 128 * 1024 // Assume 128KB blocks
		}
		
		blockCache = cache.NewAltruisticCache(baseCache, altruisticConfig, totalCapacity)
		logger.Info("Altruistic cache enabled", map[string]interface{}{
			"min_personal_mb": cfg.Cache.MinPersonalCacheMB,
			"total_capacity_mb": totalCapacity / (1024 * 1024),
			"bandwidth_limit_mb": cfg.Cache.AltruisticBandwidthMB,
		})
	} else {
		blockCache = baseCache
	}
	
	// Create NoiseFS client
	client, err := noisefs.NewClient(ipfsClient, blockCache)
	if err != nil {
		logger.Error("Failed to create NoiseFS client", map[string]interface{}{
			"error": err.Error(),
		})
		if *jsonOutput {
			util.PrintJSONError(err)
		} else {
			fmt.Fprintf(os.Stderr, "%s\n", util.FormatError(err))
		}
		os.Exit(1)
	}
	
	if *upload != "" {
		logger.Info("Starting file upload", map[string]interface{}{
			"file":       *upload,
			"block_size": cfg.Performance.BlockSize,
		})
		if err := uploadFile(ipfsClient, client, *upload, cfg.Performance.BlockSize, *quiet, *jsonOutput, logger); err != nil {
			logger.Error("Upload failed", map[string]interface{}{
				"file":  *upload,
				"error": err.Error(),
			})
			if *jsonOutput {
				util.PrintJSONError(err)
			}
			os.Exit(1)
		}
		if !*quiet {
			showMetrics(client, logger)
		}
	} else if *download != "" {
		if *output == "" {
			err := fmt.Errorf("output file path required for download")
			logger.Error("Output file path required for download", nil)
			if *jsonOutput {
				util.PrintJSONError(err)
			} else {
				fmt.Fprintf(os.Stderr, "Error: %s\n💡 Suggestion: Use -output to specify where to save the downloaded file\n", err)
			}
			os.Exit(1)
		}
		logger.Info("Starting file download", map[string]interface{}{
			"descriptor_cid": *download,
			"output_file":    *output,
		})
		if err := downloadFile(ipfsClient, client, *download, *output, *quiet, *jsonOutput, logger); err != nil {
			logger.Error("Download failed", map[string]interface{}{
				"descriptor_cid": *download,
				"output_file":    *output,
				"error":          err.Error(),
			})
			if *jsonOutput {
				util.PrintJSONError(err)
			}
			os.Exit(1)
		}
		if !*quiet {
			showMetrics(client, logger)
		}
	} else if *stats {
		// Show statistics
		showSystemStats(ipfsClient, client, blockCache, *jsonOutput, logger)
	} else {
		flag.Usage()
	}
}

// loadConfig loads configuration from file or uses defaults
func loadConfig(configPath string) (*config.Config, error) {
	if configPath == "" {
		// Try default config path
		defaultPath, err := config.GetDefaultConfigPath()
		if err == nil {
			configPath = defaultPath
		}
	}
	
	return config.LoadConfig(configPath)
}

func uploadFile(ipfsClient *ipfs.Client, client *noisefs.Client, filePath string, blockSize int, quiet bool, jsonOutput bool, logger *logging.Logger) error {
	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()
	
	// Get file info
	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}
	
	// Create splitter
	splitter, err := blocks.NewSplitter(blockSize)
	if err != nil {
		return fmt.Errorf("failed to create splitter: %w", err)
	}
	
	// Split file into blocks
	logger.Info("Splitting file into blocks", map[string]interface{}{
		"block_size": blockSize,
	})
	
	var splitProgress *util.ProgressBar
	if !quiet {
		splitProgress = util.NewProgressBar(fileInfo.Size(), "Splitting file", os.Stdout)
	}
	
	fileBlocks, err := splitter.Split(file)
	if err != nil {
		return fmt.Errorf("failed to split file: %w", err)
	}
	
	if splitProgress != nil {
		splitProgress.Finish()
	}
	
	logger.Info("File split into blocks", map[string]interface{}{
		"block_count": len(fileBlocks),
	})
	
	// Create descriptor
	descriptor := descriptors.NewDescriptor(
		filepath.Base(filePath),
		fileInfo.Size(),
		blockSize,
	)
	
	// Generate or select randomizer blocks (using 3-tuple format)
	randomizer1Blocks := make([]*blocks.Block, len(fileBlocks))
	randomizer1CIDs := make([]string, len(fileBlocks))
	randomizer2Blocks := make([]*blocks.Block, len(fileBlocks))
	randomizer2CIDs := make([]string, len(fileBlocks))
	
	for i := range fileBlocks {
		randBlock1, cid1, randBlock2, cid2, err := client.SelectTwoRandomizers(fileBlocks[i].Size())
		if err != nil {
			return fmt.Errorf("failed to select randomizer blocks: %w", err)
		}
		randomizer1Blocks[i] = randBlock1
		randomizer1CIDs[i] = cid1
		randomizer2Blocks[i] = randBlock2
		randomizer2CIDs[i] = cid2
	}
	
	// XOR blocks with randomizers (3-tuple: data XOR randomizer1 XOR randomizer2)
	anonymizedBlocks := make([]*blocks.Block, len(fileBlocks))
	for i := range fileBlocks {
		xorBlock, err := fileBlocks[i].XOR3(randomizer1Blocks[i], randomizer2Blocks[i])
		if err != nil {
			return fmt.Errorf("failed to XOR blocks: %w", err)
		}
		anonymizedBlocks[i] = xorBlock
	}
	
	// Store anonymized blocks in IPFS with caching
	var uploadProgress *util.ProgressBar
	if !quiet {
		uploadProgress = util.NewProgressBar(int64(len(anonymizedBlocks)), "Uploading blocks", os.Stdout)
	}
	
	dataCIDs := make([]string, len(anonymizedBlocks))
	for i, block := range anonymizedBlocks {
		cid, err := client.StoreBlockWithCache(block)
		if err != nil {
			return fmt.Errorf("failed to store data block %d: %w", i, err)
		}
		dataCIDs[i] = cid
		if uploadProgress != nil {
			uploadProgress.Add(1)
		}
	}
	
	if uploadProgress != nil {
		uploadProgress.Finish()
	}
	
	// Add block triples to descriptor (3-tuple format)
	for i := range dataCIDs {
		if err := descriptor.AddBlockTriple(dataCIDs[i], randomizer1CIDs[i], randomizer2CIDs[i]); err != nil {
			return fmt.Errorf("failed to add block triple to descriptor: %w", err)
		}
	}
	
	// Store descriptor in IPFS
	store, err := descriptors.NewStore(ipfsClient)
	if err != nil {
		return fmt.Errorf("failed to create descriptor store: %w", err)
	}
	
	descriptorCID, err := store.Save(descriptor)
	if err != nil {
		return fmt.Errorf("failed to store descriptor: %w", err)
	}
	
	// Log upload completion
	logger.Info("Upload completed successfully", map[string]interface{}{
		"descriptor_cid": descriptorCID,
		"file_name":      filepath.Base(filePath),
		"file_size":      fileInfo.Size(),
		"block_count":    len(fileBlocks),
	})
	
	// Display results for user
	if jsonOutput {
		result := util.UploadResult{
			DescriptorCID: descriptorCID,
			Filename:      filepath.Base(filePath),
			FileSize:      fileInfo.Size(),
			BlockCount:    len(fileBlocks),
			BlockSize:     blockSize,
		}
		util.PrintJSONSuccess(result)
	} else if quiet {
		fmt.Println(descriptorCID)
	} else {
		fmt.Println("\nUpload complete!")
		fmt.Printf("Descriptor CID: %s\n", descriptorCID)
	}
	
	// Record upload metrics
	totalStoredBytes := int64(0)
	for _, block := range anonymizedBlocks {
		totalStoredBytes += int64(len(block.Data))
	}
	// Add randomizer blocks size (they're stored but already exist)
	// For 3-tuple: data + randomizer1 + randomizer2 = 3x the data size
	client.RecordUpload(fileInfo.Size(), totalStoredBytes*3) // *3 for data + 2 randomizer blocks
	
	return nil
}

func downloadFile(ipfsClient *ipfs.Client, client *noisefs.Client, descriptorCID string, outputPath string, quiet bool, jsonOutput bool, logger *logging.Logger) error {
	// Create descriptor store
	store, err := descriptors.NewStore(ipfsClient)
	if err != nil {
		return fmt.Errorf("failed to create descriptor store: %w", err)
	}
	
	// Load descriptor from IPFS
	if !quiet {
		fmt.Printf("Loading descriptor from CID: %s\n", descriptorCID)
	}
	descriptor, err := store.Load(descriptorCID)
	if err != nil {
		return fmt.Errorf("failed to load descriptor: %w", err)
	}
	
	if !quiet {
		fmt.Printf("Downloading file: %s (%d bytes)\n", descriptor.Filename, descriptor.FileSize)
		fmt.Printf("Blocks to retrieve: %d\n", len(descriptor.Blocks))
	}
	
	// Retrieve all data blocks
	dataCIDs := make([]string, len(descriptor.Blocks))
	randomizer1CIDs := make([]string, len(descriptor.Blocks))
	randomizer2CIDs := make([]string, len(descriptor.Blocks))
	
	for i, block := range descriptor.Blocks {
		dataCIDs[i] = block.DataCID
		randomizer1CIDs[i] = block.RandomizerCID1
		if descriptor.IsThreeTuple() {
			randomizer2CIDs[i] = block.RandomizerCID2
		}
	}
	
	// Retrieve anonymized data blocks
	var downloadProgress *util.ProgressBar
	if !quiet {
		downloadProgress = util.NewProgressBar(int64(len(dataCIDs)), "Downloading blocks", os.Stdout)
	}
	
	dataBlocks := make([]*blocks.Block, len(dataCIDs))
	for i, cid := range dataCIDs {
		block, err := ipfsClient.RetrieveBlock(cid)
		if err != nil {
			return fmt.Errorf("failed to retrieve data block %d: %w", i, err)
		}
		dataBlocks[i] = block
		if downloadProgress != nil {
			downloadProgress.Add(1)
		}
	}
	if downloadProgress != nil {
		downloadProgress.Finish()
	}
	
	// Retrieve first randomizer blocks
	var randomizerProgress *util.ProgressBar
	if !quiet {
		randomizerProgress = util.NewProgressBar(int64(len(randomizer1CIDs)), "Retrieving randomizers", os.Stdout)
	}
	
	randomizer1Blocks := make([]*blocks.Block, len(randomizer1CIDs))
	for i, cid := range randomizer1CIDs {
		block, err := ipfsClient.RetrieveBlock(cid)
		if err != nil {
			return fmt.Errorf("failed to retrieve randomizer1 block %d: %w", i, err)
		}
		randomizer1Blocks[i] = block
		if randomizerProgress != nil {
			randomizerProgress.Add(1)
		}
	}
	if randomizerProgress != nil {
		randomizerProgress.Finish()
	}
	
	// Retrieve second randomizer blocks if using 3-tuple format
	var randomizer2Blocks []*blocks.Block
	if descriptor.IsThreeTuple() {
		var randomizer2Progress *util.ProgressBar
		if !quiet {
			randomizer2Progress = util.NewProgressBar(int64(len(randomizer2CIDs)), "Retrieving randomizers 2", os.Stdout)
		}
		
		randomizer2Blocks = make([]*blocks.Block, len(randomizer2CIDs))
		for i, cid := range randomizer2CIDs {
			block, err := ipfsClient.RetrieveBlock(cid)
			if err != nil {
				return fmt.Errorf("failed to retrieve randomizer2 block %d: %w", i, err)
			}
			randomizer2Blocks[i] = block
			if randomizer2Progress != nil {
				randomizer2Progress.Add(1)
			}
		}
		if randomizer2Progress != nil {
			randomizer2Progress.Finish()
		}
	}
	
	// XOR blocks to reconstruct original data
	originalBlocks := make([]*blocks.Block, len(dataBlocks))
	for i := range dataBlocks {
		var origBlock *blocks.Block
		var err error
		
		if descriptor.IsThreeTuple() && randomizer2Blocks != nil {
			// Use 3-tuple XOR for version 2.0
			origBlock, err = dataBlocks[i].XOR3(randomizer1Blocks[i], randomizer2Blocks[i])
		} else {
			// Use 2-tuple XOR for legacy format
			origBlock, err = dataBlocks[i].XOR(randomizer1Blocks[i])
		}
		
		if err != nil {
			return fmt.Errorf("failed to XOR blocks: %w", err)
		}
		originalBlocks[i] = origBlock
	}
	
	// Create output file
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outputFile.Close()
	
	// Assemble file
	assembler := blocks.NewAssembler()
	if err := assembler.AssembleToWriter(originalBlocks, outputFile); err != nil {
		return fmt.Errorf("failed to assemble file: %w", err)
	}
	
	if jsonOutput {
		result := util.DownloadResult{
			OutputPath: outputPath,
			Filename:   descriptor.Filename,
			FileSize:   descriptor.FileSize,
			BlockCount: len(descriptor.Blocks),
		}
		util.PrintJSONSuccess(result)
	} else if !quiet {
		fmt.Printf("\nDownload complete! File saved to: %s\n", outputPath)
	}
	
	// Record download
	client.RecordDownload()
	
	return nil
}

// showMetrics displays current NoiseFS metrics
func showMetrics(client *noisefs.Client, logger *logging.Logger) {
	metrics := client.GetMetrics()
	
	// Log performance metrics for analysis
	logger.Info("Performance metrics", map[string]interface{}{
		"block_reuse_rate":        metrics.BlockReuseRate,
		"blocks_reused":           metrics.BlocksReused,
		"blocks_generated":        metrics.BlocksGenerated,
		"cache_hit_rate":          metrics.CacheHitRate,
		"cache_hits":              metrics.CacheHits,
		"cache_misses":            metrics.CacheMisses,
		"storage_efficiency":      metrics.StorageEfficiency,
		"total_uploads":           metrics.TotalUploads,
		"total_downloads":         metrics.TotalDownloads,
		"bytes_uploaded_original": metrics.BytesUploadedOriginal,
		"bytes_stored_ipfs":       metrics.BytesStoredIPFS,
	})
	
	// Keep console output for user visibility
	fmt.Println("\n--- NoiseFS Metrics ---")
	fmt.Printf("Block Reuse Rate: %.1f%% (%d reused, %d generated)\n", 
		metrics.BlockReuseRate, metrics.BlocksReused, metrics.BlocksGenerated)
	fmt.Printf("Cache Hit Rate: %.1f%% (%d hits, %d misses)\n", 
		metrics.CacheHitRate, metrics.CacheHits, metrics.CacheMisses)
	fmt.Printf("Storage Efficiency: %.1f%% overhead\n", metrics.StorageEfficiency)
	fmt.Printf("Total Operations: %d uploads, %d downloads\n", 
		metrics.TotalUploads, metrics.TotalDownloads)
	
	if metrics.BytesUploadedOriginal > 0 {
		fmt.Printf("Data: %d bytes original → %d bytes stored\n", 
			metrics.BytesUploadedOriginal, metrics.BytesStoredIPFS)
	}
}

// showSystemStats displays comprehensive system statistics
func showSystemStats(ipfsClient *ipfs.Client, client *noisefs.Client, blockCache cache.Cache, jsonOutput bool, logger *logging.Logger) {
	// Gather all statistics
	var ipfsConnected bool
	var peerCount int
	
	// Test IPFS connection
	testBlock, _ := blocks.NewBlock([]byte("test"))
	if _, err := ipfsClient.StoreBlock(testBlock); err == nil {
		ipfsConnected = true
		peerCount = len(ipfsClient.GetConnectedPeers())
	}
	
	// Get cache stats
	cacheStats := blockCache.GetStats()
	var cacheHitRate float64
	if total := cacheStats.Hits + cacheStats.Misses; total > 0 {
		cacheHitRate = float64(cacheStats.Hits) / float64(total) * 100
	}
	
	// Get NoiseFS metrics
	metrics := client.GetMetrics()
	
	if jsonOutput {
		// Output as JSON
		result := util.StatsResult{
			IPFS: util.IPFSStats{
				Connected: ipfsConnected,
				Peers:     peerCount,
			},
			Cache: util.CacheStats{
				Size:      cacheStats.Size,
				Hits:      cacheStats.Hits,
				Misses:    cacheStats.Misses,
				Evictions: cacheStats.Evictions,
				HitRate:   cacheHitRate,
			},
			Blocks: util.BlockStats{
				Reused:    metrics.BlocksReused,
				Generated: metrics.BlocksGenerated,
				ReuseRate: metrics.BlockReuseRate,
			},
			Storage: util.StorageStats{
				OriginalBytes: metrics.BytesUploadedOriginal,
				StoredBytes:   metrics.BytesStoredIPFS,
				Overhead:      metrics.StorageEfficiency,
			},
			Activity: util.ActivityStats{
				Uploads:   metrics.TotalUploads,
				Downloads: metrics.TotalDownloads,
			},
		}
		
		// Add altruistic cache stats if available
		if altruisticStats := client.GetAltruisticCacheStats(); altruisticStats != nil {
			personalHitRate := 0.0
			if altruisticStats.PersonalHits+altruisticStats.PersonalMisses > 0 {
				personalHitRate = float64(altruisticStats.PersonalHits) / 
					float64(altruisticStats.PersonalHits+altruisticStats.PersonalMisses) * 100
			}
			
			altruisticHitRate := 0.0
			if altruisticStats.AltruisticHits+altruisticStats.AltruisticMisses > 0 {
				altruisticHitRate = float64(altruisticStats.AltruisticHits) / 
					float64(altruisticStats.AltruisticHits+altruisticStats.AltruisticMisses) * 100
			}
			
			personalPercent := 0.0
			altruisticPercent := 0.0
			usedPercent := 0.0
			if altruisticStats.TotalCapacity > 0 {
				personalPercent = float64(altruisticStats.PersonalSize) / 
					float64(altruisticStats.TotalCapacity) * 100
				altruisticPercent = float64(altruisticStats.AltruisticSize) / 
					float64(altruisticStats.TotalCapacity) * 100
				usedPercent = float64(altruisticStats.PersonalSize+altruisticStats.AltruisticSize) / 
					float64(altruisticStats.TotalCapacity) * 100
			}
			
			minPersonalCacheMB := 0
			if cacheConfig := client.GetCacheConfig(); cacheConfig != nil {
				minPersonalCacheMB = int(cacheConfig.MinPersonalCache / (1024 * 1024))
			}
			
			result.Altruistic = &util.AltruisticStats{
				Enabled:            client.IsAltruisticCacheEnabled(),
				PersonalBlocks:     altruisticStats.PersonalBlocks,
				AltruisticBlocks:   altruisticStats.AltruisticBlocks,
				PersonalSize:       altruisticStats.PersonalSize,
				AltruisticSize:     altruisticStats.AltruisticSize,
				TotalCapacity:      altruisticStats.TotalCapacity,
				PersonalPercent:    personalPercent,
				AltruisticPercent:  altruisticPercent,
				UsedPercent:        usedPercent,
				PersonalHitRate:    personalHitRate,
				AltruisticHitRate:  altruisticHitRate,
				FlexPoolUsage:      altruisticStats.FlexPoolUsage * 100,
				MinPersonalCacheMB: minPersonalCacheMB,
			}
		}
		util.PrintJSONSuccess(result)
		return
	}
	
	// Regular text output
	fmt.Println("=== NoiseFS System Statistics ===\n")
	
	// IPFS Connection Status
	fmt.Println("--- IPFS Connection ---")
	if ipfsConnected {
		fmt.Println("IPFS Status: Connected")
		if peerCount > 0 {
			fmt.Printf("Connected Peers: %d\n", peerCount)
		}
	} else {
		fmt.Println("IPFS Status: Disconnected")
	}
	
	// Cache Statistics
	fmt.Println("\n--- Cache Statistics ---")
	fmt.Printf("Cache Size: %d blocks\n", cacheStats.Size)
	fmt.Printf("Cache Hits: %d\n", cacheStats.Hits)
	fmt.Printf("Cache Misses: %d\n", cacheStats.Misses)
	fmt.Printf("Cache Evictions: %d\n", cacheStats.Evictions)
	if total := cacheStats.Hits + cacheStats.Misses; total > 0 {
		hitRate := float64(cacheStats.Hits) / float64(total) * 100
		fmt.Printf("Cache Hit Rate: %.1f%%\n", hitRate)
	}
	
	// Altruistic Cache Statistics (if enabled)
	if altruisticStats := client.GetAltruisticCacheStats(); altruisticStats != nil {
		fmt.Println("\n--- Altruistic Cache ---")
		fmt.Printf("Personal Blocks: %d (%s)\n", 
			altruisticStats.PersonalBlocks, 
			formatBytes(altruisticStats.PersonalSize))
		fmt.Printf("Altruistic Blocks: %d (%s)\n", 
			altruisticStats.AltruisticBlocks, 
			formatBytes(altruisticStats.AltruisticSize))
		
		// Show usage percentages
		totalUsed := altruisticStats.PersonalSize + altruisticStats.AltruisticSize
		if altruisticStats.TotalCapacity > 0 {
			personalPercent := float64(altruisticStats.PersonalSize) / float64(altruisticStats.TotalCapacity) * 100
			altruisticPercent := float64(altruisticStats.AltruisticSize) / float64(altruisticStats.TotalCapacity) * 100
			usedPercent := float64(totalUsed) / float64(altruisticStats.TotalCapacity) * 100
			
			fmt.Printf("Total Capacity: %s (%.1f%% used)\n", 
				formatBytes(altruisticStats.TotalCapacity), usedPercent)
			fmt.Printf("  Personal: %.1f%% | Altruistic: %.1f%%\n", 
				personalPercent, altruisticPercent)
		}
		
		// Show hit rates
		if altruisticStats.PersonalHits+altruisticStats.PersonalMisses > 0 {
			personalHitRate := float64(altruisticStats.PersonalHits) / 
				float64(altruisticStats.PersonalHits+altruisticStats.PersonalMisses) * 100
			fmt.Printf("Personal Hit Rate: %.1f%%\n", personalHitRate)
		}
		if altruisticStats.AltruisticHits+altruisticStats.AltruisticMisses > 0 {
			altruisticHitRate := float64(altruisticStats.AltruisticHits) / 
				float64(altruisticStats.AltruisticHits+altruisticStats.AltruisticMisses) * 100
			fmt.Printf("Altruistic Hit Rate: %.1f%%\n", altruisticHitRate)
		}
		
		// Show flex pool usage
		fmt.Printf("Flex Pool Usage: %.1f%%\n", altruisticStats.FlexPoolUsage * 100)
		
		// Show MinPersonalCache setting
		if cacheConfig := client.GetCacheConfig(); cacheConfig != nil {
			fmt.Printf("Min Personal Cache: %s\n", 
				formatBytes(cacheConfig.MinPersonalCache))
		}
		
		// Visual representation
		fmt.Println("\n--- Cache Visualization ---")
		viz := util.NewCacheVisualization(50)
		fmt.Print(viz.RenderCacheSummary(
			altruisticStats.PersonalSize,
			altruisticStats.AltruisticSize,
			altruisticStats.TotalCapacity,
			altruisticStats.FlexPoolUsage,
			client.GetCacheConfig().MinPersonalCache,
		))
	}
	
	fmt.Println("\n--- Block Management ---")
	fmt.Printf("Blocks Reused: %d\n", metrics.BlocksReused)
	fmt.Printf("Blocks Generated: %d\n", metrics.BlocksGenerated)
	if total := metrics.BlocksReused + metrics.BlocksGenerated; total > 0 {
		reuseRate := float64(metrics.BlocksReused) / float64(total) * 100
		fmt.Printf("Block Reuse Rate: %.1f%%\n", reuseRate)
	}
	
	fmt.Println("\n--- Storage Efficiency ---")
	if metrics.BytesUploadedOriginal > 0 {
		fmt.Printf("Original Data: %s\n", formatBytes(metrics.BytesUploadedOriginal))
		fmt.Printf("Stored Data: %s\n", formatBytes(metrics.BytesStoredIPFS))
		overhead := float64(metrics.BytesStoredIPFS)/float64(metrics.BytesUploadedOriginal)*100 - 100
		fmt.Printf("Storage Overhead: %.1f%%\n", overhead)
	} else {
		fmt.Println("No data uploaded yet")
	}
	
	fmt.Println("\n--- Operation History ---")
	fmt.Printf("Total Uploads: %d\n", metrics.TotalUploads)
	fmt.Printf("Total Downloads: %d\n", metrics.TotalDownloads)
	
	// Log the stats for debugging
	logger.Info("System statistics displayed", map[string]interface{}{
		"cache_size":      cacheStats.Size,
		"cache_hit_rate":  float64(cacheStats.Hits) / float64(cacheStats.Hits+cacheStats.Misses) * 100,
		"block_reuse_rate": metrics.BlockReuseRate,
		"total_operations": metrics.TotalUploads + metrics.TotalDownloads,
	})
}

// formatBytes converts bytes to human-readable format
func formatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	
	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d bytes", bytes)
	}
}

// handleSubcommand handles announcement-related subcommands
func handleSubcommand(cmd string, args []string) {
	// Parse global flags that might be before the subcommand
	var (
		configFile = ""
		ipfsAPI    = ""
		quiet      = false
		jsonOutput = false
	)
	
	// Look for global flags in args
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-config":
			if i+1 < len(args) {
				configFile = args[i+1]
				i++
			}
		case "-api":
			if i+1 < len(args) {
				ipfsAPI = args[i+1]
				i++
			}
		case "-quiet":
			quiet = true
		case "-json":
			jsonOutput = true
		}
	}
	
	// Special case for discover - doesn't need IPFS connection
	if cmd == "discover" {
		if err := discoverCommand(args, quiet, jsonOutput); err != nil {
			if jsonOutput {
				util.PrintJSONError(err)
			} else {
				fmt.Fprintf(os.Stderr, "Error: %s\n", err)
			}
			os.Exit(1)
		}
		return
	}
	
	// Load configuration for commands that need IPFS
	cfg, err := loadConfig(configFile)
	if err != nil {
		if jsonOutput {
			util.PrintJSONError(err)
		} else {
			fmt.Fprintf(os.Stderr, "Error loading config: %s\n", err)
		}
		os.Exit(1)
	}
	
	// Apply command-line override
	if ipfsAPI != "" {
		cfg.IPFS.APIEndpoint = ipfsAPI
	}
	
	// Create IPFS client
	ipfsClient, err := ipfs.NewClient(cfg.IPFS.APIEndpoint)
	if err != nil {
		if jsonOutput {
			util.PrintJSONError(err)
		} else {
			fmt.Fprintf(os.Stderr, "Error connecting to IPFS: %s\n", err)
		}
		os.Exit(1)
	}
	
	// Create shell for PubSub
	ipfsShell := shell.NewShell(cfg.IPFS.APIEndpoint)
	
	// Handle subcommands
	switch cmd {
	case "announce":
		err = announceCommand(args, ipfsClient, ipfsShell, quiet, jsonOutput)
	case "subscribe":
		err = subscribeCommand(args, ipfsClient, ipfsShell, quiet, jsonOutput)
	default:
		err = fmt.Errorf("unknown command: %s", cmd)
	}
	
	if err != nil {
		if jsonOutput {
			util.PrintJSONError(err)
		} else {
			fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		}
		os.Exit(1)
	}
}