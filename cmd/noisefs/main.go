package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	noisefs "github.com/TheEntropyCollective/noisefs/pkg/core/client"
	"github.com/TheEntropyCollective/noisefs/pkg/core/crypto"
	"github.com/TheEntropyCollective/noisefs/pkg/core/descriptors"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/config"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/logging"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/workers"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	_ "github.com/TheEntropyCollective/noisefs/pkg/storage/backends" // Import to register backends
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
	"github.com/TheEntropyCollective/noisefs/pkg/util"
	shell "github.com/ipfs/go-ipfs-api"
)

func main() {
	// Show legal disclaimer on first run or if not accepted recently
	// Skip if running in test mode
	if os.Getenv("NOISEFS_SKIP_LEGAL_NOTICE") != "1" && !checkLegalDisclaimerAccepted() {
		showLegalDisclaimer()
	}

	var (
		configFile = flag.String("config", "", "Configuration file path")
		ipfsAPI    = flag.String("api", "", "IPFS API endpoint (overrides config)")
		upload     = flag.String("upload", "", "File or directory to upload to NoiseFS (uses parallel processing)")
		download   = flag.String("download", "", "Descriptor CID to download from NoiseFS")
		output     = flag.String("output", "", "Output file path for download")
		recursive  = flag.Bool("r", false, "Recursively upload/download directories")
		exclude    = flag.String("exclude", "", "Comma-separated list of file patterns to exclude from directory upload")
		stats      = flag.Bool("stats", false, "Show NoiseFS statistics")
		quiet      = flag.Bool("quiet", false, "Minimal output (only show errors and results)")
		jsonOutput = flag.Bool("json", false, "Output results in JSON format")
		blockSize  = flag.Int("block-size", 0, "Block size in bytes (overrides config)")
		cacheSize  = flag.Int("cache-size", 0, "Number of blocks to cache in memory (overrides config)")
		workers    = flag.Int("workers", 0, "Number of parallel workers for upload/download (overrides config)")
		// Altruistic cache flags
		minPersonalCacheMB    = flag.Int("min-personal-cache", 0, "Minimum personal cache size in MB (overrides config)")
		disableAltruistic     = flag.Bool("disable-altruistic", false, "Disable altruistic caching")
		altruisticBandwidthMB = flag.Int("altruistic-bandwidth", 0, "Bandwidth limit for altruistic operations in MB/s")
		// Streaming flags
		streaming            = flag.Bool("streaming", false, "Use streaming mode for upload/download with bounded memory")
		memoryLimitMB        = flag.Int("memory-limit", 0, "Memory limit for streaming operations in MB (overrides config)")
		streamBufferSize     = flag.Int("stream-buffer", 0, "Buffer size for streaming pipeline (overrides config)")
		enableMemMonitoring  = flag.Bool("monitor-memory", false, "Enable memory monitoring during streaming operations")
	)

	// Check for subcommands first
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "announce", "subscribe", "discover", "ls", "search", "sync", "share-directory", "receive-directory", "list-snapshots":
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
	if *workers > 0 {
		cfg.Performance.MaxConcurrentOps = *workers
	}
	// Apply streaming overrides
	if *memoryLimitMB > 0 {
		cfg.Performance.MemoryLimit = *memoryLimitMB
	}
	if *streamBufferSize > 0 {
		cfg.Performance.StreamBufferSize = *streamBufferSize
	}
	if *enableMemMonitoring {
		cfg.Performance.EnableMemoryMonitoring = true
	}

	// Create storage backend (IPFS with abstraction layer)
	logger.Info("Connecting to storage backend", map[string]interface{}{
		"backend":  "ipfs",
		"endpoint": cfg.IPFS.APIEndpoint,
	})

	// Create storage manager with IPFS backend
	storageConfig := storage.DefaultConfig()
	if ipfsBackend, exists := storageConfig.Backends["ipfs"]; exists {
		ipfsBackend.Connection.Endpoint = cfg.IPFS.APIEndpoint
	}

	storageManager, err := storage.NewManager(storageConfig)
	if err != nil {
		logger.Error("Failed to create storage manager", map[string]interface{}{
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

	// Start the storage manager
	err = storageManager.Start(context.Background())
	if err != nil {
		logger.Error("Failed to start storage manager", map[string]interface{}{
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
	defer storageManager.Stop(context.Background())

	// Create cache
	logger.Debug("Initializing block cache", map[string]interface{}{
		"cache_size":         cfg.Cache.BlockCacheSize,
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
			"min_personal_mb":    cfg.Cache.MinPersonalCacheMB,
			"total_capacity_mb":  totalCapacity / (1024 * 1024),
			"bandwidth_limit_mb": cfg.Cache.AltruisticBandwidthMB,
		})
	} else {
		blockCache = baseCache
	}

	// Create NoiseFS client
	client, err := noisefs.NewClient(storageManager, blockCache)
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
		// Check if the path is a directory
		fileInfo, err := os.Stat(*upload)
		if err != nil {
			logger.Error("Failed to stat upload path", map[string]interface{}{
				"path":  *upload,
				"error": err.Error(),
			})
			if *jsonOutput {
				util.PrintJSONError(err)
			} else {
				fmt.Fprintf(os.Stderr, "Error: %s\n💡 Suggestion: Verify the file or directory path exists\n", err)
			}
			os.Exit(1)
		}

		if fileInfo.IsDir() {
			// Directory upload
			if !*recursive {
				err := fmt.Errorf("path is a directory, use -r flag for recursive upload")
				logger.Error("Directory upload requires recursive flag", map[string]interface{}{
					"path": *upload,
				})
				if *jsonOutput {
					util.PrintJSONError(err)
				} else {
					fmt.Fprintf(os.Stderr, "Error: %s\n💡 Suggestion: Use -r flag to upload directories recursively\n", err)
				}
				os.Exit(1)
			}

			logger.Info("Starting directory upload", map[string]interface{}{
				"directory":  *upload,
				"block_size": cfg.Performance.BlockSize,
				"streaming":  *streaming,
				"recursive":  *recursive,
			})
			
			var err error
			if *streaming {
				err = streamingUploadDirectory(storageManager, client, *upload, cfg.Performance.BlockSize, *exclude, *quiet, *jsonOutput, cfg, logger)
			} else {
				err = uploadDirectory(storageManager, client, *upload, cfg.Performance.BlockSize, *exclude, *quiet, *jsonOutput, cfg, logger)
			}
			if err != nil {
				logger.Error("Directory upload failed", map[string]interface{}{
					"directory": *upload,
					"error":     err.Error(),
				})
				if *jsonOutput {
					util.PrintJSONError(err)
				}
				os.Exit(1)
			}
		} else {
			// File upload
			logger.Info("Starting file upload", map[string]interface{}{
				"file":       *upload,
				"block_size": cfg.Performance.BlockSize,
				"streaming":  *streaming,
			})
			var err error
			if *streaming {
				err = streamingUploadFile(storageManager, client, *upload, cfg.Performance.BlockSize, *quiet, *jsonOutput, cfg, logger)
			} else {
				err = uploadFile(storageManager, client, *upload, cfg.Performance.BlockSize, *quiet, *jsonOutput, cfg, logger)
			}
			if err != nil {
				logger.Error("Upload failed", map[string]interface{}{
					"file":  *upload,
					"error": err.Error(),
				})
				if *jsonOutput {
					util.PrintJSONError(err)
				}
				os.Exit(1)
			}
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
		
		// Try to detect if the CID is a directory descriptor
		isDirectory, err := detectDirectoryDescriptor(storageManager, *download)
		if err != nil {
			logger.Error("Failed to detect descriptor type", map[string]interface{}{
				"descriptor_cid": *download,
				"error":          err.Error(),
			})
			if *jsonOutput {
				util.PrintJSONError(err)
			}
			os.Exit(1)
		}
		
		if isDirectory {
			// Directory download
			logger.Info("Starting directory download", map[string]interface{}{
				"descriptor_cid": *download,
				"output_dir":     *output,
				"streaming":      *streaming,
				"recursive":      *recursive,
			})
			
			var err error
			if *streaming {
				err = streamingDownloadDirectory(storageManager, client, *download, *output, *quiet, *jsonOutput, cfg, logger)
			} else {
				err = downloadDirectory(storageManager, client, *download, *output, *quiet, *jsonOutput, cfg, logger)
			}
			if err != nil {
				logger.Error("Directory download failed", map[string]interface{}{
					"descriptor_cid": *download,
					"output_dir":     *output,
					"error":          err.Error(),
				})
				if *jsonOutput {
					util.PrintJSONError(err)
				}
				os.Exit(1)
			}
		} else {
			// File download
			logger.Info("Starting file download", map[string]interface{}{
				"descriptor_cid": *download,
				"output_file":    *output,
				"streaming":      *streaming,
			})
			var err error
			if *streaming {
				err = streamingDownloadFile(storageManager, client, *download, *output, *quiet, *jsonOutput, cfg, logger)
			} else {
				err = downloadFile(storageManager, client, *download, *output, *quiet, *jsonOutput, logger)
			}
			if err != nil {
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
		}
		
		if !*quiet {
			showMetrics(client, logger)
		}
	} else if *stats {
		// Show statistics
		showSystemStats(storageManager, client, blockCache, *jsonOutput, logger)
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

func uploadFile(storageManager *storage.Manager, client *noisefs.Client, filePath string, blockSize int, quiet bool, jsonOutput bool, cfg *config.Config, logger *logging.Logger) error {
	// Track overall upload time
	uploadStartTime := time.Now()
	
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
		fileInfo.Size(),
		blockSize,
	)

	// Generate or select randomizer blocks (using 3-tuple format)
	randomizer1Blocks := make([]*blocks.Block, len(fileBlocks))
	randomizer1CIDs := make([]string, len(fileBlocks))
	randomizer2Blocks := make([]*blocks.Block, len(fileBlocks))
	randomizer2CIDs := make([]string, len(fileBlocks))

	for i := range fileBlocks {
		randBlock1, cid1, randBlock2, cid2, err := client.SelectRandomizers(fileBlocks[i].Size())
		if err != nil {
			return fmt.Errorf("failed to select randomizer blocks: %w", err)
		}
		randomizer1Blocks[i] = randBlock1
		randomizer1CIDs[i] = cid1
		randomizer2Blocks[i] = randBlock2
		randomizer2CIDs[i] = cid2
	}

	// Start performance timing
	xorStartTime := time.Now()

	// Create worker pool for parallel processing
	workerCount := cfg.Performance.MaxConcurrentOps
	if workerCount <= 0 {
		workerCount = 10 // Default to 10 workers
	}
	pool := workers.NewPool(workers.Config{
		WorkerCount: workerCount,
		BufferSize:  workerCount * 2,
		ShutdownTimeout: 30 * time.Second,
	})
	defer pool.Shutdown()
	
	// Start the worker pool
	if err := pool.Start(); err != nil {
		return fmt.Errorf("failed to start worker pool: %w", err)
	}

	// Create block operation batch processor
	blockOps := workers.NewBlockOperationBatch(pool)

	// Parallel XOR blocks with randomizers (3-tuple: data XOR randomizer1 XOR randomizer2)
	logger.Info("Performing parallel XOR operations", map[string]interface{}{
		"block_count": len(fileBlocks),
		"worker_count": workerCount,
	})

	anonymizedBlocks, err := blockOps.ParallelXOR(context.Background(), fileBlocks, randomizer1Blocks, randomizer2Blocks)
	if err != nil {
		return fmt.Errorf("failed to perform parallel XOR: %w", err)
	}

	xorDuration := time.Since(xorStartTime)
	logger.Info("XOR operations completed", map[string]interface{}{
		"duration_ms": xorDuration.Milliseconds(),
		"blocks_per_second": float64(len(fileBlocks)) / xorDuration.Seconds(),
	})

	// Store anonymized blocks in IPFS with caching (parallel)
	storageStartTime := time.Now()

	var uploadProgress *util.ProgressBar
	if !quiet {
		uploadProgress = util.NewProgressBar(int64(len(anonymizedBlocks)), "Uploading blocks", os.Stdout)
	}

	// Progress will be updated manually after parallel storage completes

	logger.Info("Storing blocks in parallel", map[string]interface{}{
		"block_count": len(anonymizedBlocks),
		"worker_count": workerCount,
	})

	dataCIDs, err := blockOps.ParallelStorage(context.Background(), anonymizedBlocks, client)
	if err != nil {
		return fmt.Errorf("failed to perform parallel storage: %w", err)
	}

	storageDuration := time.Since(storageStartTime)
	logger.Info("Storage operations completed", map[string]interface{}{
		"duration_ms": storageDuration.Milliseconds(),
		"blocks_per_second": float64(len(anonymizedBlocks)) / storageDuration.Seconds(),
	})
	
	// Complete the progress bar
	if uploadProgress != nil {
		for i := 0; i < len(anonymizedBlocks); i++ {
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

	// Store descriptor using storage manager
	store, err := descriptors.NewStoreWithManager(storageManager)
	if err != nil {
		return fmt.Errorf("failed to create descriptor store: %w", err)
	}

	descriptorCID, err := store.Save(descriptor)
	if err != nil {
		return fmt.Errorf("failed to store descriptor: %w", err)
	}

	// Calculate total upload time
	totalUploadDuration := time.Since(uploadStartTime)
	
	// Log upload completion with performance metrics
	logger.Info("Upload completed successfully", map[string]interface{}{
		"descriptor_cid": descriptorCID,
		"file_name":      filepath.Base(filePath),
		"file_size":      fileInfo.Size(),
		"block_count":    len(fileBlocks),
		"total_duration_ms": totalUploadDuration.Milliseconds(),
		"throughput_mb_per_s": float64(fileInfo.Size()) / (1024 * 1024) / totalUploadDuration.Seconds(),
		"xor_duration_ms": xorDuration.Milliseconds(),
		"storage_duration_ms": storageDuration.Milliseconds(),
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
		fmt.Printf("Performance: %.2f MB/s (total time: %.2fs)\n", 
			float64(fileInfo.Size())/(1024*1024)/totalUploadDuration.Seconds(),
			totalUploadDuration.Seconds())
		fmt.Printf("  - XOR operations: %.2fs (%d blocks/s)\n", 
			xorDuration.Seconds(), 
			int(float64(len(fileBlocks))/xorDuration.Seconds()))
		fmt.Printf("  - Storage operations: %.2fs (%d blocks/s)\n", 
			storageDuration.Seconds(),
			int(float64(len(anonymizedBlocks))/storageDuration.Seconds()))
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

// uploadDirectory uploads a directory recursively to NoiseFS
func uploadDirectory(storageManager *storage.Manager, client *noisefs.Client, dirPath string, blockSize int, excludePatterns string, quiet bool, jsonOutput bool, cfg *config.Config, logger *logging.Logger) error {
	uploadStartTime := time.Now()
	
	// Parse exclude patterns
	excludes := make([]string, 0)
	if excludePatterns != "" {
		excludes = strings.Split(excludePatterns, ",")
		for i := range excludes {
			excludes[i] = strings.TrimSpace(excludes[i])
		}
	}
	
	// Create encryption key for directory operations
	encryptionKey, err := crypto.GenerateKey("directory-key")
	if err != nil {
		return fmt.Errorf("failed to generate encryption key: %w", err)
	}
	
	// Create directory processor
	processorConfig := &blocks.ProcessorConfig{
		BlockSize:         blockSize,
		MaxWorkers:        cfg.Performance.MaxConcurrentOps,
		EncryptionKey:     encryptionKey,
		ProgressCallback:  nil, // Will be set below if not quiet
		ErrorHandler:      nil, // Will handle errors at the end
		SkipSymlinks:      true,
		SkipHidden:        true,
		MaxFileSize:       0, // No limit
		AllowedExtensions: nil,
		BlockedExtensions: excludes,
	}
	
	var progressBar *util.ProgressBar
	if !quiet {
		// We'll update this once we know the total
		progressBar = util.NewProgressBar(0, "Processing directory", os.Stdout)
		processorConfig.ProgressCallback = func(processed, total int64, currentFile string) {
			if progressBar != nil {
				progressBar.SetTotal(total)
				progressBar.SetCurrent(processed)
				progressBar.SetDescription(fmt.Sprintf("Processing: %s", filepath.Base(currentFile)))
			}
		}
	}
	
	processor, err := blocks.NewDirectoryProcessor(processorConfig)
	if err != nil {
		return fmt.Errorf("failed to create directory processor: %w", err)
	}
	
	// Create a block processor to handle the blocks
	blockProcessor := &DirectoryBlockProcessor{
		storageManager: storageManager,
		client:         client,
		logger:         logger,
		blocks:         make(map[string]*blocks.Block),
		manifests:      make(map[string]string),
		mutex:          sync.RWMutex{},
	}
	
	// Process the directory
	logger.Info("Starting directory processing", map[string]interface{}{
		"directory":   dirPath,
		"block_size":  blockSize,
		"max_workers": cfg.Performance.MaxConcurrentOps,
	})
	
	results, err := processor.ProcessDirectory(dirPath, blockProcessor)
	if err != nil {
		return fmt.Errorf("failed to process directory: %w", err)
	}
	
	if progressBar != nil {
		progressBar.Finish()
	}
	
	// Find the root directory result
	var rootResult *blocks.ProcessResult
	for _, result := range results {
		if result.Path == dirPath && result.Type == blocks.DirectoryType {
			rootResult = result
			break
		}
	}
	
	if rootResult == nil {
		return fmt.Errorf("failed to find root directory result")
	}
	
	totalDuration := time.Since(uploadStartTime)
	
	// Count files and calculate total size
	var totalFiles int
	var totalSize int64
	for _, result := range results {
		if result.Type == blocks.FileType {
			totalFiles++
			totalSize += result.Size
		}
	}
	
	logger.Info("Directory upload completed", map[string]interface{}{
		"directory_cid":     rootResult.CID,
		"directory_path":    dirPath,
		"total_files":       totalFiles,
		"total_size":        totalSize,
		"total_duration_ms": totalDuration.Milliseconds(),
		"throughput_mb_s":   float64(totalSize) / (1024 * 1024) / totalDuration.Seconds(),
	})
	
	// Display results
	if jsonOutput {
		result := util.DirectoryUploadResult{
			DirectoryCID: rootResult.CID,
			DirectoryPath: filepath.Base(dirPath),
			TotalFiles:   totalFiles,
			TotalSize:    totalSize,
			BlockSize:    blockSize,
		}
		util.PrintJSONSuccess(result)
	} else if quiet {
		fmt.Println(rootResult.CID)
	} else {
		fmt.Printf("\nDirectory upload complete!\n")
		fmt.Printf("Directory CID: %s\n", rootResult.CID)
		fmt.Printf("Files uploaded: %d\n", totalFiles)
		fmt.Printf("Total size: %s\n", formatBytes(totalSize))
		fmt.Printf("Performance: %.2f MB/s (total time: %.2fs)\n", 
			float64(totalSize)/(1024*1024)/totalDuration.Seconds(),
			totalDuration.Seconds())
	}
	
	return nil
}

// streamingUploadDirectory uploads a directory with streaming support
func streamingUploadDirectory(storageManager *storage.Manager, client *noisefs.Client, dirPath string, blockSize int, excludePatterns string, quiet bool, jsonOutput bool, cfg *config.Config, logger *logging.Logger) error {
	// For now, delegate to regular upload - streaming directory upload would need more complex implementation
	return uploadDirectory(storageManager, client, dirPath, blockSize, excludePatterns, quiet, jsonOutput, cfg, logger)
}

// DirectoryBlockProcessor handles blocks produced by directory processing
type DirectoryBlockProcessor struct {
	storageManager *storage.Manager
	client         *noisefs.Client
	logger         *logging.Logger
	blocks         map[string]*blocks.Block
	manifests      map[string]string
	mutex          sync.RWMutex
}

// ProcessDirectoryBlock processes a block from a file in the directory
func (dbp *DirectoryBlockProcessor) ProcessDirectoryBlock(blockIndex int, block *blocks.Block) error {
	dbp.mutex.Lock()
	defer dbp.mutex.Unlock()
	
	// Store the block
	dbp.blocks[block.ID] = block
	
	// Store block in storage backend
	_, err := dbp.storageManager.Put(context.Background(), block)
	if err != nil {
		return fmt.Errorf("failed to store block %s: %w", block.ID, err)
	}
	
	return nil
}

// ProcessDirectoryManifest processes a directory manifest
func (dbp *DirectoryBlockProcessor) ProcessDirectoryManifest(dirPath string, manifestBlock *blocks.Block) error {
	dbp.mutex.Lock()
	defer dbp.mutex.Unlock()
	
	// Store manifest block
	dbp.manifests[dirPath] = manifestBlock.ID
	
	// Store manifest in storage backend
	_, err := dbp.storageManager.Put(context.Background(), manifestBlock)
	if err != nil {
		return fmt.Errorf("failed to store manifest block %s: %w", manifestBlock.ID, err)
	}
	
	return nil
}

func downloadFile(storageManager *storage.Manager, client *noisefs.Client, descriptorCID string, outputPath string, quiet bool, jsonOutput bool, logger *logging.Logger) error {
	// Track download start time
	downloadStartTime := time.Now()
	
	// Create descriptor store
	store, err := descriptors.NewStoreWithManager(storageManager)
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

	// Create worker pool for parallel operations
	poolConfig := workers.Config{
		WorkerCount: runtime.NumCPU() * 2, // Double CPU count for I/O bound operations
		BufferSize:  runtime.NumCPU() * 4,
		ShutdownTimeout: 30 * time.Second,
	}
	
	if !quiet {
		// Add progress reporter
		poolConfig.ProgressReporter = func(completed, total int64) {
			// Progress is tracked via progress bars below
		}
	}
	
	pool := workers.NewPool(poolConfig)
	if err := pool.Start(); err != nil {
		return fmt.Errorf("failed to start worker pool: %w", err)
	}
	defer pool.Shutdown()
	
	// Create batch processor for block operations
	batchProcessor := workers.NewBlockOperationBatch(pool)
	
	// Prepare addresses for parallel retrieval
	dataAddresses := make([]*storage.BlockAddress, len(descriptor.Blocks))
	randomizer1Addresses := make([]*storage.BlockAddress, len(descriptor.Blocks))
	randomizer2Addresses := make([]*storage.BlockAddress, len(descriptor.Blocks))
	
	for i, block := range descriptor.Blocks {
		dataAddresses[i] = &storage.BlockAddress{ID: block.DataCID}
		randomizer1Addresses[i] = &storage.BlockAddress{ID: block.RandomizerCID1}
		randomizer2Addresses[i] = &storage.BlockAddress{ID: block.RandomizerCID2}
	}
	
	ctx := context.Background()
	
	// Track retrieval timings
	retrievalStartTime := time.Now()
	
	// Parallel retrieval of all blocks
	var dataBlocks, randomizer1Blocks, randomizer2Blocks []*blocks.Block
	var retrievalErr error
	
	// Use wait group to retrieve all block types in parallel
	var wg sync.WaitGroup
	wg.Add(3)
	
	// Channel for collecting errors
	errChan := make(chan error, 3)
	
	// Retrieve data blocks
	go func() {
		defer wg.Done()
		if !quiet {
			fmt.Println("Retrieving data blocks in parallel...")
		}
		blocks, err := batchProcessor.ParallelRetrieval(ctx, dataAddresses, storageManager)
		if err != nil {
			errChan <- fmt.Errorf("data blocks retrieval failed: %w", err)
			return
		}
		dataBlocks = blocks
	}()
	
	// Retrieve randomizer1 blocks
	go func() {
		defer wg.Done()
		if !quiet {
			fmt.Println("Retrieving randomizer1 blocks in parallel...")
		}
		blocks, err := batchProcessor.ParallelRetrieval(ctx, randomizer1Addresses, storageManager)
		if err != nil {
			errChan <- fmt.Errorf("randomizer1 blocks retrieval failed: %w", err)
			return
		}
		randomizer1Blocks = blocks
	}()
	
	// Retrieve randomizer2 blocks
	go func() {
		defer wg.Done()
		if !quiet {
			fmt.Println("Retrieving randomizer2 blocks in parallel...")
		}
		blocks, err := batchProcessor.ParallelRetrieval(ctx, randomizer2Addresses, storageManager)
		if err != nil {
			errChan <- fmt.Errorf("randomizer2 blocks retrieval failed: %w", err)
			return
		}
		randomizer2Blocks = blocks
	}()
	
	// Wait for all retrievals to complete
	wg.Wait()
	close(errChan)
	
	// Check for errors
	for err := range errChan {
		if err != nil {
			retrievalErr = err
			break
		}
	}
	
	if retrievalErr != nil {
		return retrievalErr
	}
	
	retrievalDuration := time.Since(retrievalStartTime)
	
	// Validate all blocks were retrieved
	if len(dataBlocks) != len(descriptor.Blocks) || 
	   len(randomizer1Blocks) != len(descriptor.Blocks) || 
	   len(randomizer2Blocks) != len(descriptor.Blocks) {
		return fmt.Errorf("incomplete block retrieval: expected %d blocks, got data=%d, r1=%d, r2=%d",
			len(descriptor.Blocks), len(dataBlocks), len(randomizer1Blocks), len(randomizer2Blocks))
	}
	
	// Parallel XOR reconstruction
	xorStartTime := time.Now()
	if !quiet {
		fmt.Println("Reconstructing original blocks in parallel...")
	}
	
	originalBlocks, err := batchProcessor.ParallelXOR(ctx, dataBlocks, randomizer1Blocks, randomizer2Blocks)
	if err != nil {
		return fmt.Errorf("parallel XOR reconstruction failed: %w", err)
	}
	
	xorDuration := time.Since(xorStartTime)
	
	// Create output file
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outputFile.Close()

	// Assemble file
	assembleStartTime := time.Now()
	assembler := blocks.NewAssembler()
	if err := assembler.AssembleToWriter(originalBlocks, outputFile); err != nil {
		return fmt.Errorf("failed to assemble file: %w", err)
	}
	assembleDuration := time.Since(assembleStartTime)
	
	// Calculate total download duration
	totalDownloadDuration := time.Since(downloadStartTime)
	
	// Log performance metrics
	logger.Info("Download performance metrics", map[string]interface{}{
		"descriptor_cid": descriptorCID,
		"file_size": descriptor.FileSize,
		"block_count": len(descriptor.Blocks),
		"total_duration_ms": totalDownloadDuration.Milliseconds(),
		"retrieval_duration_ms": retrievalDuration.Milliseconds(),
		"xor_duration_ms": xorDuration.Milliseconds(),
		"assembly_duration_ms": assembleDuration.Milliseconds(),
		"throughput_mb_per_s": float64(descriptor.FileSize) / (1024 * 1024) / totalDownloadDuration.Seconds(),
		"blocks_per_second": float64(len(descriptor.Blocks)*3) / retrievalDuration.Seconds(), // *3 for all block types
		"worker_count": poolConfig.WorkerCount,
	})
	
	// Display pool statistics
	stats := pool.Stats()
	logger.Debug("Worker pool statistics", map[string]interface{}{
		"tasks_submitted": stats.Submitted,
		"tasks_completed": stats.Completed,
		"tasks_failed": stats.Failed,
		"worker_count": stats.WorkerCount,
	})

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
		fmt.Printf("Performance: %.2f MB/s (total time: %.2fs)\n", 
			float64(descriptor.FileSize)/(1024*1024)/totalDownloadDuration.Seconds(),
			totalDownloadDuration.Seconds())
		fmt.Printf("  - Block retrieval: %.2fs (%d blocks/s)\n", 
			retrievalDuration.Seconds(), 
			int(float64(len(descriptor.Blocks)*3)/retrievalDuration.Seconds()))
		fmt.Printf("  - XOR reconstruction: %.2fs (%d blocks/s)\n", 
			xorDuration.Seconds(),
			int(float64(len(descriptor.Blocks))/xorDuration.Seconds()))
		fmt.Printf("  - File assembly: %.2fs\n", assembleDuration.Seconds())
		fmt.Printf("  - Parallel workers: %d\n", poolConfig.WorkerCount)
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
func showSystemStats(storageManager *storage.Manager, client *noisefs.Client, blockCache cache.Cache, jsonOutput bool, logger *logging.Logger) {
	// Gather all statistics
	var ipfsConnected bool
	var peerCount int

	// Test storage manager connection
	testBlock, _ := blocks.NewBlock([]byte("test"))
	if _, err := storageManager.Put(context.Background(), testBlock); err == nil {
		ipfsConnected = true
		// TODO: Get peer count from storage manager
		peerCount = 0
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
	fmt.Println("=== NoiseFS System Statistics ===")

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
		fmt.Printf("Flex Pool Usage: %.1f%%\n", altruisticStats.FlexPoolUsage*100)

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
		"cache_size":       cacheStats.Size,
		"cache_hit_rate":   float64(cacheStats.Hits) / float64(cacheStats.Hits+cacheStats.Misses) * 100,
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

	// Create storage manager for subcommands
	storageConfig := storage.DefaultConfig()
	if ipfsBackend, exists := storageConfig.Backends["ipfs"]; exists {
		ipfsBackend.Connection.Endpoint = cfg.IPFS.APIEndpoint
	}

	storageManager, err := storage.NewManager(storageConfig)
	if err != nil {
		if jsonOutput {
			util.PrintJSONError(err)
		} else {
			fmt.Fprintf(os.Stderr, "Error creating storage manager: %s\n", err)
		}
		os.Exit(1)
	}

	err = storageManager.Start(context.Background())
	if err != nil {
		if jsonOutput {
			util.PrintJSONError(err)
		} else {
			fmt.Fprintf(os.Stderr, "Error starting storage manager: %s\n", err)
		}
		os.Exit(1)
	}
	defer storageManager.Stop(context.Background())

	// Create shell for PubSub
	ipfsShell := shell.NewShell(cfg.IPFS.APIEndpoint)

	// Handle subcommands
	switch cmd {
	case "announce":
		err = announceCommand(args, storageManager, ipfsShell, quiet, jsonOutput)
	case "subscribe":
		err = subscribeCommand(args, storageManager, ipfsShell, quiet, jsonOutput)
	case "ls":
		err = lsCommand(args, storageManager, quiet, jsonOutput)
	case "search":
		// Search command handles its own args parsing
		handleSearchCommand(args)
		return
	case "sync":
		err = handleSyncCommand(args, storageManager, quiet, jsonOutput)
	case "share-directory":
		err = shareDirectoryCommand(args, storageManager, quiet, jsonOutput)
	case "receive-directory":
		err = receiveDirectoryCommand(args, storageManager, quiet, jsonOutput)
	case "list-snapshots":
		err = listSnapshotsCommand(args, storageManager, quiet, jsonOutput)
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

// checkLegalDisclaimerAccepted checks if the user has accepted the legal disclaimer
func checkLegalDisclaimerAccepted() bool {
	// Get config directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	configDir := filepath.Join(homeDir, ".noisefs")

	disclaimerFile := filepath.Join(configDir, ".noisefs_legal_accepted")
	info, err := os.Stat(disclaimerFile)
	if err != nil {
		return false
	}

	// Check if disclaimer was accepted within the last 30 days
	return time.Since(info.ModTime()) < 30*24*time.Hour
}

// showLegalDisclaimer displays the legal disclaimer and asks for acceptance
func showLegalDisclaimer() {
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("                          ⚠️  LEGAL NOTICE & TERMS OF USE")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println()
	fmt.Println("IMPORTANT: This software is provided for LEGITIMATE PURPOSES ONLY.")
	fmt.Println()
	fmt.Println("By using NoiseFS, you acknowledge and agree that:")
	fmt.Println()
	fmt.Println("  • You will NOT use this software to share copyrighted material without authorization")
	fmt.Println("  • You will NOT use this software for any illegal activities")
	fmt.Println("  • You are solely responsible for all content you upload, share, or download")
	fmt.Println("  • You understand that all actions may be logged and you may be held accountable")
	fmt.Println("  • The developers are NOT responsible for how you use this software")
	fmt.Println()
	fmt.Println("Legitimate use cases include:")
	fmt.Println("  • Open source software distribution")
	fmt.Println("  • Academic and research data sharing")
	fmt.Println("  • Public domain content distribution")
	fmt.Println("  • Personal backup and file synchronization")
	fmt.Println("  • Creative Commons licensed content")
	fmt.Println()
	fmt.Println("For detailed information, see: docs/LEGITIMATE_USE_CASES.md")
	fmt.Println()
	fmt.Println(strings.Repeat("-", 80))
	fmt.Printf("\nDo you understand and accept these terms? (yes/no): ")

	var response string
	fmt.Scanln(&response)

	if strings.ToLower(response) != "yes" {
		fmt.Println("\nYou must accept the terms to use NoiseFS. Exiting.")
		os.Exit(1)
	}

	// Save acceptance
	homeDir, err := os.UserHomeDir()
	if err == nil {
		configDir := filepath.Join(homeDir, ".noisefs")
		os.MkdirAll(configDir, 0700)
		disclaimerFile := filepath.Join(configDir, ".noisefs_legal_accepted")
		os.WriteFile(disclaimerFile, []byte(time.Now().Format(time.RFC3339)), 0600)
	}

	fmt.Println("\nThank you for accepting the terms. Remember to use NoiseFS responsibly.")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println()
}

// lsCommand implements directory listing functionality
func lsCommand(args []string, storageManager *storage.Manager, quiet bool, jsonOutput bool) error {
	if len(args) == 0 {
		return fmt.Errorf("directory CID required")
	}
	
	directoryCID := args[0]
	
	// Create directory manager
	encryptionKey, err := crypto.GenerateKey("directory-key")
	if err != nil {
		return fmt.Errorf("failed to generate encryption key: %w", err)
	}
	
	directoryManager, err := storage.NewDirectoryManager(storageManager, encryptionKey, nil)
	if err != nil {
		return fmt.Errorf("failed to create directory manager: %w", err)
	}
	
	// Retrieve directory manifest
	manifest, err := directoryManager.RetrieveDirectoryManifest(context.Background(), "", directoryCID)
	if err != nil {
		return fmt.Errorf("failed to retrieve directory manifest: %w", err)
	}
	
	// Process directory entries
	entries := make([]DirectoryListEntry, 0, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		// For now, we'll show encrypted names - in a real implementation,
		// we would need the correct encryption key to decrypt names
		listEntry := DirectoryListEntry{
			Name:       fmt.Sprintf("encrypted_%d", len(entries)),
			CID:        entry.CID,
			Type:       entry.Type,
			Size:       entry.Size,
			ModifiedAt: entry.ModifiedAt,
		}
		entries = append(entries, listEntry)
	}
	
	// Output results
	if jsonOutput {
		result := DirectoryListResult{
			DirectoryCID: directoryCID,
			Entries:      entries,
			TotalEntries: len(entries),
		}
		util.PrintJSONSuccess(result)
	} else if quiet {
		for _, entry := range entries {
			typeStr := "file"
			if entry.Type == blocks.DirectoryType {
				typeStr = "directory"
			}
			fmt.Printf("%s\t%s\t%s\n", entry.CID, typeStr, entry.Name)
		}
	} else {
		fmt.Printf("Directory: %s\n", directoryCID)
		fmt.Printf("Entries: %d\n\n", len(entries))
		
		for _, entry := range entries {
			typeStr := "FILE"
			if entry.Type == blocks.DirectoryType {
				typeStr = "DIR"
			}
			
			fmt.Printf("%-4s  %-8s  %s  %s\n", 
				typeStr, 
				formatBytes(entry.Size), 
				entry.ModifiedAt.Format("2006-01-02 15:04:05"),
				entry.Name)
		}
	}
	
	return nil
}

// DirectoryListEntry represents a directory entry for listing
type DirectoryListEntry struct {
	Name       string                `json:"name"`
	CID        string                `json:"cid"`
	Type       blocks.DescriptorType `json:"type"`
	Size       int64                 `json:"size"`
	ModifiedAt time.Time             `json:"modified_at"`
}

// DirectoryListResult represents the result of directory listing
type DirectoryListResult struct {
	DirectoryCID string               `json:"directory_cid"`
	Entries      []DirectoryListEntry `json:"entries"`
	TotalEntries int                  `json:"total_entries"`
}

// detectDirectoryDescriptor detects if a CID is a directory descriptor
func detectDirectoryDescriptor(storageManager *storage.Manager, cid string) (bool, error) {
	// Try to retrieve the descriptor
	address := &storage.BlockAddress{
		ID:          cid,
		BackendType: storageManager.GetConfig().DefaultBackend,
	}
	
	block, err := storageManager.Get(context.Background(), address)
	if err != nil {
		return false, fmt.Errorf("failed to retrieve descriptor: %w", err)
	}
	
	// Try to unmarshal as a regular file descriptor first
	_, err = descriptors.FromJSON(block.Data)
	if err == nil {
		return false, nil // It's a file descriptor
	}
	
	// Try to unmarshal as directory manifest
	encryptionKey, err := crypto.GenerateKey("directory-key")
	if err != nil {
		return false, fmt.Errorf("failed to generate encryption key: %w", err)
	}
	
	_, err = descriptors.DecryptManifest(block.Data, encryptionKey)
	if err == nil {
		return true, nil // It's a directory descriptor
	}
	
	// If both fail, we can't determine the type
	return false, fmt.Errorf("unable to determine descriptor type")
}

// downloadDirectory downloads a directory recursively
func downloadDirectory(storageManager *storage.Manager, client *noisefs.Client, directoryCID string, outputDir string, quiet bool, jsonOutput bool, cfg *config.Config, logger *logging.Logger) error {
	downloadStartTime := time.Now()
	
	// Create output directory
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}
	
	// Create directory manager
	encryptionKey, err := crypto.GenerateKey("directory-key")
	if err != nil {
		return fmt.Errorf("failed to generate encryption key: %w", err)
	}
	
	directoryManager, err := storage.NewDirectoryManager(storageManager, encryptionKey, nil)
	if err != nil {
		return fmt.Errorf("failed to create directory manager: %w", err)
	}
	
	// Reconstruct directory
	result, err := directoryManager.ReconstructDirectory(context.Background(), directoryCID, outputDir)
	if err != nil {
		return fmt.Errorf("failed to reconstruct directory: %w", err)
	}
	
	// Create progress bar
	var progressBar *util.ProgressBar
	if !quiet {
		progressBar = util.NewProgressBar(int64(result.TotalEntries), "Downloading files", os.Stdout)
	}
	
	// Download each file
	downloadedFiles := 0
	totalSize := int64(0)
	
	for _, entry := range result.Entries {
		if entry.Type == blocks.FileType {
			// Download file
			filePath := filepath.Join(outputDir, entry.DecryptedName)
			
			if err := downloadFile(storageManager, client, entry.CID, filePath, true, false, logger); err != nil {
				logger.Error("Failed to download file", map[string]interface{}{
					"file_cid":  entry.CID,
					"file_path": filePath,
					"error":     err.Error(),
				})
				continue
			}
			
			downloadedFiles++
			totalSize += entry.Size
			
			if progressBar != nil {
				progressBar.Add(1)
			}
		} else if entry.Type == blocks.DirectoryType {
			// Recursively download subdirectory
			subdirPath := filepath.Join(outputDir, entry.DecryptedName)
			if err := downloadDirectory(storageManager, client, entry.CID, subdirPath, true, false, cfg, logger); err != nil {
				logger.Error("Failed to download subdirectory", map[string]interface{}{
					"subdir_cid":  entry.CID,
					"subdir_path": subdirPath,
					"error":       err.Error(),
				})
				continue
			}
			
			if progressBar != nil {
				progressBar.Add(1)
			}
		}
	}
	
	if progressBar != nil {
		progressBar.Finish()
	}
	
	totalDuration := time.Since(downloadStartTime)
	
	logger.Info("Directory download completed", map[string]interface{}{
		"directory_cid":     directoryCID,
		"output_dir":        outputDir,
		"files_downloaded":  downloadedFiles,
		"total_size":        totalSize,
		"total_duration_ms": totalDuration.Milliseconds(),
		"throughput_mb_s":   float64(totalSize) / (1024 * 1024) / totalDuration.Seconds(),
	})
	
	// Display results
	if jsonOutput {
		result := util.DirectoryDownloadResult{
			DirectoryCID:    directoryCID,
			OutputDir:       outputDir,
			FilesDownloaded: downloadedFiles,
			TotalSize:       totalSize,
		}
		util.PrintJSONSuccess(result)
	} else if quiet {
		fmt.Printf("Downloaded %d files to %s\n", downloadedFiles, outputDir)
	} else {
		fmt.Printf("\nDirectory download complete!\n")
		fmt.Printf("Directory: %s\n", outputDir)
		fmt.Printf("Files downloaded: %d\n", downloadedFiles)
		fmt.Printf("Total size: %s\n", formatBytes(totalSize))
		fmt.Printf("Performance: %.2f MB/s (total time: %.2fs)\n", 
			float64(totalSize)/(1024*1024)/totalDuration.Seconds(),
			totalDuration.Seconds())
	}
	
	return nil
}

// streamingDownloadDirectory downloads a directory with streaming support
func streamingDownloadDirectory(storageManager *storage.Manager, client *noisefs.Client, directoryCID string, outputDir string, quiet bool, jsonOutput bool, cfg *config.Config, logger *logging.Logger) error {
	// For now, delegate to regular download - streaming directory download would need more complex implementation
	return downloadDirectory(storageManager, client, directoryCID, outputDir, quiet, jsonOutput, cfg, logger)
}

// shareDirectoryCommand creates an immutable snapshot of a directory for sharing
func shareDirectoryCommand(args []string, storageManager *storage.Manager, quiet bool, jsonOutput bool) error {
	if len(args) < 3 {
		return fmt.Errorf("usage: share-directory <directory-cid> <directory-key> <snapshot-name> [description]")
	}
	
	directoryCID := args[0]
	directoryKeyStr := args[1]
	snapshotName := args[2]
	description := ""
	if len(args) > 3 {
		description = args[3]
	}
	
	// Parse the directory key
	directoryKey, err := crypto.ParseKeyFromString(directoryKeyStr)
	if err != nil {
		return fmt.Errorf("failed to parse directory key: %w", err)
	}
	
	// Create directory manager
	tempKey, err := crypto.GenerateKey("temp-key")
	if err != nil {
		return fmt.Errorf("failed to generate temp key: %w", err)
	}
	
	directoryManager, err := storage.NewDirectoryManager(storageManager, tempKey, nil)
	if err != nil {
		return fmt.Errorf("failed to create directory manager: %w", err)
	}
	
	// Create snapshot
	snapshotCID, snapshotKey, err := directoryManager.CreateDirectorySnapshot(
		context.Background(),
		directoryCID,
		directoryKey,
		snapshotName,
		description,
	)
	if err != nil {
		return fmt.Errorf("failed to create directory snapshot: %w", err)
	}
	
	// Prepare result
	result := ShareDirectoryResult{
		SnapshotCID:  snapshotCID,
		SnapshotKey:  snapshotKey.String(),
		SnapshotName: snapshotName,
		Description:  description,
		OriginalCID:  directoryCID,
		CreatedAt:    time.Now(),
	}
	
	// Output result
	if jsonOutput {
		util.PrintJSONSuccess(result)
	} else if quiet {
		fmt.Printf("%s\t%s\n", snapshotCID, snapshotKey.String())
	} else {
		fmt.Printf("Directory snapshot created successfully!\n")
		fmt.Printf("Snapshot CID: %s\n", snapshotCID)
		fmt.Printf("Snapshot Key: %s\n", snapshotKey.String())
		fmt.Printf("Snapshot Name: %s\n", snapshotName)
		if description != "" {
			fmt.Printf("Description: %s\n", description)
		}
		fmt.Printf("Original CID: %s\n", directoryCID)
		fmt.Printf("\nShare this CID and key with others to give them read-only access to the directory snapshot.\n")
	}
	
	return nil
}

// receiveDirectoryCommand accesses a shared directory snapshot
func receiveDirectoryCommand(args []string, storageManager *storage.Manager, quiet bool, jsonOutput bool) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: receive-directory <snapshot-cid> <snapshot-key>")
	}
	
	snapshotCID := args[0]
	snapshotKeyStr := args[1]
	
	// Parse the snapshot key
	snapshotKey, err := crypto.ParseKeyFromString(snapshotKeyStr)
	if err != nil {
		return fmt.Errorf("failed to parse snapshot key: %w", err)
	}
	
	// Create directory manager
	tempKey, err := crypto.GenerateKey("temp-key")
	if err != nil {
		return fmt.Errorf("failed to generate temp key: %w", err)
	}
	
	directoryManager, err := storage.NewDirectoryManager(storageManager, tempKey, nil)
	if err != nil {
		return fmt.Errorf("failed to create directory manager: %w", err)
	}
	
	// Retrieve snapshot manifest
	manifest, err := directoryManager.RetrieveDirectoryManifestWithKey(
		context.Background(),
		snapshotCID,
		snapshotKey,
	)
	if err != nil {
		return fmt.Errorf("failed to retrieve directory snapshot: %w", err)
	}
	
	// Verify this is a snapshot
	if !manifest.IsSnapshot() {
		return fmt.Errorf("CID does not point to a valid directory snapshot")
	}
	
	// Get snapshot info
	snapshotInfo := manifest.GetSnapshotInfo()
	if snapshotInfo == nil {
		return fmt.Errorf("snapshot missing metadata")
	}
	
	// Process directory entries
	entries := make([]DirectoryListEntry, 0, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		listEntry := DirectoryListEntry{
			Name:       fmt.Sprintf("encrypted_%d", len(entries)),
			CID:        entry.CID,
			Type:       entry.Type,
			Size:       entry.Size,
			ModifiedAt: entry.ModifiedAt,
		}
		entries = append(entries, listEntry)
	}
	
	// Prepare result
	result := ReceiveDirectoryResult{
		SnapshotCID:    snapshotCID,
		SnapshotName:   snapshotInfo.SnapshotName,
		Description:    snapshotInfo.Description,
		OriginalCID:    snapshotInfo.OriginalCID,
		CreatedAt:      snapshotInfo.CreationTime,
		Entries:        entries,
		TotalEntries:   len(entries),
		IsSnapshot:     true,
	}
	
	// Output result
	if jsonOutput {
		util.PrintJSONSuccess(result)
	} else if quiet {
		for _, entry := range entries {
			typeStr := "file"
			if entry.Type == blocks.DirectoryType {
				typeStr = "directory"
			}
			fmt.Printf("%s\t%s\t%s\n", entry.CID, typeStr, entry.Name)
		}
	} else {
		fmt.Printf("Directory Snapshot: %s\n", snapshotCID)
		fmt.Printf("Snapshot Name: %s\n", snapshotInfo.SnapshotName)
		if snapshotInfo.Description != "" {
			fmt.Printf("Description: %s\n", snapshotInfo.Description)
		}
		fmt.Printf("Original CID: %s\n", snapshotInfo.OriginalCID)
		fmt.Printf("Created: %s\n", snapshotInfo.CreationTime.Format("2006-01-02 15:04:05"))
		fmt.Printf("Entries: %d\n\n", len(entries))
		
		for _, entry := range entries {
			typeStr := "FILE"
			if entry.Type == blocks.DirectoryType {
				typeStr = "DIR"
			}
			
			fmt.Printf("%-4s  %-8s  %s  %s\n", 
				typeStr, 
				formatBytes(entry.Size), 
				entry.ModifiedAt.Format("2006-01-02 15:04:05"),
				entry.Name)
		}
	}
	
	return nil
}

// listSnapshotsCommand lists all snapshots associated with a directory
func listSnapshotsCommand(args []string, storageManager *storage.Manager, quiet bool, jsonOutput bool) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: list-snapshots <directory-cid> [directory-key]")
	}
	
	directoryCID := args[0]
	var directoryKey *crypto.EncryptionKey
	
	if len(args) > 1 {
		var err error
		directoryKey, err = crypto.ParseKeyFromString(args[1])
		if err != nil {
			return fmt.Errorf("failed to parse directory key: %w", err)
		}
	}
	
	// Create directory manager
	tempKey, err := crypto.GenerateKey("temp-key")
	if err != nil {
		return fmt.Errorf("failed to generate temp key: %w", err)
	}
	
	directoryManager, err := storage.NewDirectoryManager(storageManager, tempKey, nil)
	if err != nil {
		return fmt.Errorf("failed to create directory manager: %w", err)
	}
	
	// For now, we can only list the original directory info
	// In a full implementation, we would need a snapshot index or registry
	var manifest *blocks.DirectoryManifest
	if directoryKey != nil {
		manifest, err = directoryManager.RetrieveDirectoryManifestWithKey(
			context.Background(),
			directoryCID,
			directoryKey,
		)
	} else {
		// Use default key (this is a limitation - in practice, the key would be required)
		manifest, err = directoryManager.RetrieveDirectoryManifest(
			context.Background(),
			"",
			directoryCID,
		)
	}
	
	if err != nil {
		return fmt.Errorf("failed to retrieve directory manifest: %w", err)
	}
	
	// Prepare result
	snapshots := make([]SnapshotInfo, 0)
	
	// If this is a snapshot, include it in the list
	if manifest.IsSnapshot() {
		snapshotInfo := manifest.GetSnapshotInfo()
		if snapshotInfo != nil {
			snapshots = append(snapshots, SnapshotInfo{
				SnapshotCID:  directoryCID,
				SnapshotName: snapshotInfo.SnapshotName,
				Description:  snapshotInfo.Description,
				OriginalCID:  snapshotInfo.OriginalCID,
				CreatedAt:    snapshotInfo.CreationTime,
				IsSnapshot:   true,
			})
		}
	}
	
	result := ListSnapshotsResult{
		DirectoryCID: directoryCID,
		Snapshots:    snapshots,
		TotalSnapshots: len(snapshots),
	}
	
	// Output result
	if jsonOutput {
		util.PrintJSONSuccess(result)
	} else if quiet {
		for _, snapshot := range snapshots {
			fmt.Printf("%s\t%s\t%s\n", snapshot.SnapshotCID, snapshot.SnapshotName, snapshot.CreatedAt.Format("2006-01-02 15:04:05"))
		}
	} else {
		fmt.Printf("Directory: %s\n", directoryCID)
		fmt.Printf("Snapshots: %d\n\n", len(snapshots))
		
		if len(snapshots) == 0 {
			fmt.Printf("No snapshots found.\n")
			fmt.Printf("Note: This command currently only detects if the provided CID is itself a snapshot.\n")
			fmt.Printf("A full snapshot registry would be needed to list all snapshots of a directory.\n")
		} else {
			for _, snapshot := range snapshots {
				fmt.Printf("Snapshot: %s\n", snapshot.SnapshotCID)
				fmt.Printf("  Name: %s\n", snapshot.SnapshotName)
				if snapshot.Description != "" {
					fmt.Printf("  Description: %s\n", snapshot.Description)
				}
				fmt.Printf("  Original CID: %s\n", snapshot.OriginalCID)
				fmt.Printf("  Created: %s\n", snapshot.CreatedAt.Format("2006-01-02 15:04:05"))
				fmt.Printf("\n")
			}
		}
	}
	
	return nil
}

// Result structures for JSON output
type ShareDirectoryResult struct {
	SnapshotCID  string    `json:"snapshot_cid"`
	SnapshotKey  string    `json:"snapshot_key"`
	SnapshotName string    `json:"snapshot_name"`
	Description  string    `json:"description"`
	OriginalCID  string    `json:"original_cid"`
	CreatedAt    time.Time `json:"created_at"`
}

type ReceiveDirectoryResult struct {
	SnapshotCID    string               `json:"snapshot_cid"`
	SnapshotName   string               `json:"snapshot_name"`
	Description    string               `json:"description"`
	OriginalCID    string               `json:"original_cid"`
	CreatedAt      time.Time            `json:"created_at"`
	Entries        []DirectoryListEntry `json:"entries"`
	TotalEntries   int                  `json:"total_entries"`
	IsSnapshot     bool                 `json:"is_snapshot"`
}

type SnapshotInfo struct {
	SnapshotCID  string    `json:"snapshot_cid"`
	SnapshotName string    `json:"snapshot_name"`
	Description  string    `json:"description"`
	OriginalCID  string    `json:"original_cid"`
	CreatedAt    time.Time `json:"created_at"`
	IsSnapshot   bool      `json:"is_snapshot"`
}

type ListSnapshotsResult struct {
	DirectoryCID   string         `json:"directory_cid"`
	Snapshots      []SnapshotInfo `json:"snapshots"`
	TotalSnapshots int            `json:"total_snapshots"`
}
