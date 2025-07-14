package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/tools/bootstrap"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/config"
	"github.com/TheEntropyCollective/noisefs/pkg/core/descriptors"
	"github.com/TheEntropyCollective/noisefs/pkg/fuse"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/logging"
	"github.com/TheEntropyCollective/noisefs/pkg/core/client"
)

func main() {
	var (
		configFile   = flag.String("config", "", "Configuration file path")
		mountPath    = flag.String("mount", "", "Mount point for the filesystem (overrides config)")
		volumeName   = flag.String("volume", "", "Volume name (overrides config)")
		ipfsAPI      = flag.String("ipfs", "", "IPFS API endpoint (overrides config)")
		cacheSize    = flag.Int("cache", 0, "Cache size (number of blocks, overrides config)")
		readOnly     = flag.Bool("readonly", false, "Mount as read-only (overrides config)")
		allowOther   = flag.Bool("allow-other", false, "Allow other users to access (overrides config)")
		debug        = flag.Bool("debug", false, "Enable debug output (overrides config)")
		daemon       = flag.Bool("daemon", false, "Run as daemon")
		pidFile      = flag.String("pidfile", "", "PID file for daemon mode")
		unmount      = flag.Bool("unmount", false, "Unmount filesystem")
		list         = flag.Bool("list", false, "List mounted filesystems")
		help         = flag.Bool("help", false, "Show help message")
		
		// Index management flags
		indexFile    = flag.String("index", "", "Custom index file path (overrides config)")
		addFile      = flag.String("add-file", "", "Add file to index: filename:descriptor_cid:size")
		removeFile   = flag.String("remove-file", "", "Remove file from index")
		listFiles    = flag.Bool("list-files", false, "List files in index")
		showIndex    = flag.Bool("show-index", false, "Show index file path and stats")
		
		// Bootstrap flags
		bootstrapFlag   = flag.Bool("bootstrap", false, "Bootstrap filesystem with sample data")
		bootstrapData   = flag.String("bootstrap-data", "mixed", "Bootstrap dataset (mixed, books, images, documents, code)")
		bootstrapSize   = flag.Int64("bootstrap-size", 100*1024*1024, "Maximum bootstrap data size in bytes")
		bootstrapDir    = flag.String("bootstrap-dir", "", "Directory to store bootstrap data before upload")
		listBootstrap   = flag.Bool("list-bootstrap", false, "List available bootstrap datasets")
	)
	flag.Parse()

	if *help {
		showHelp()
		return
	}

	if *list {
		listMounts()
		return
	}

	if *listBootstrap {
		bootstrap.ListAvailableDatasets()
		return
	}

	if *unmount {
		if *mountPath == "" {
			log.Fatal("Mount path required for unmount operation")
		}
		unmountFS(*mountPath)
		return
	}

	// Handle index management operations
	if *showIndex || *listFiles || *addFile != "" || *removeFile != "" {
		handleIndexOperations(*indexFile, *showIndex, *listFiles, *addFile, *removeFile)
		return
	}

	// Load configuration
	cfg, err := loadConfig(*configFile)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize logging
	if err := logging.InitFromConfig(cfg.Logging.Level, cfg.Logging.Format, cfg.Logging.Output, cfg.Logging.File); err != nil {
		log.Fatalf("Failed to initialize logging: %v", err)
	}
	
	logger := logging.GetGlobalLogger().WithComponent("noisefs-mount")

	// Apply command-line overrides
	if *mountPath != "" {
		cfg.FUSE.MountPath = *mountPath
	}
	if *volumeName != "" {
		cfg.FUSE.VolumeName = *volumeName
	}
	if *ipfsAPI != "" {
		cfg.IPFS.APIEndpoint = *ipfsAPI
	}
	if *cacheSize != 0 {
		cfg.Cache.BlockCacheSize = *cacheSize
	}
	if *indexFile != "" {
		cfg.FUSE.IndexPath = *indexFile
	}
	// Apply boolean overrides (they override config regardless of value)
	cfg.FUSE.ReadOnly = *readOnly
	cfg.FUSE.AllowOther = *allowOther
	cfg.FUSE.Debug = *debug

	if cfg.FUSE.MountPath == "" {
		logger.Error("Mount path is required", nil)
		os.Exit(1)
	}

	logger.Info("Starting NoiseFS mount", map[string]interface{}{
		"mount_path":  cfg.FUSE.MountPath,
		"volume_name": cfg.FUSE.VolumeName,
		"ipfs_api":    cfg.IPFS.APIEndpoint,
		"read_only":   cfg.FUSE.ReadOnly,
		"debug":       cfg.FUSE.Debug,
		"daemon":      *daemon,
		"bootstrap":   *bootstrapFlag,
	})

	// Handle bootstrap BEFORE mounting
	if *bootstrapFlag {
		handleBootstrap(cfg, *bootstrapData, *bootstrapSize, *bootstrapDir, logger)
	}

	// Mount filesystem
	mountFS(cfg.FUSE.MountPath, cfg.FUSE.VolumeName, cfg.IPFS.APIEndpoint, cfg.Cache.BlockCacheSize, 
		cfg.FUSE.ReadOnly, cfg.FUSE.AllowOther, cfg.FUSE.Debug, *daemon, *pidFile, cfg.FUSE.IndexPath, logger)
}

func showHelp() {
	fmt.Println("NoiseFS FUSE Mount Tool")
	fmt.Println("=======================")
	fmt.Println()
	fmt.Println("Mount NoiseFS as a FUSE filesystem for transparent file operations.")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  noisefs-mount [options]")
	fmt.Println()
	fmt.Println("Options:")
	flag.PrintDefaults()
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  # Mount NoiseFS at /mnt/noisefs")
	fmt.Println("  noisefs-mount -mount /mnt/noisefs")
	fmt.Println()
	fmt.Println("  # Mount with custom IPFS endpoint")
	fmt.Println("  noisefs-mount -mount /mnt/noisefs -ipfs 192.168.1.100:5001")
	fmt.Println()
	fmt.Println("  # Mount as daemon with PID file")
	fmt.Println("  noisefs-mount -mount /mnt/noisefs -daemon -pidfile /var/run/noisefs.pid")
	fmt.Println()
	fmt.Println("  # Unmount filesystem")
	fmt.Println("  noisefs-mount -unmount -mount /mnt/noisefs")
	fmt.Println()
	fmt.Println("  # List mounted filesystems")
	fmt.Println("  noisefs-mount -list")
	fmt.Println()
	fmt.Println("Index Management:")
	fmt.Println("  # Show index info")
	fmt.Println("  noisefs-mount -show-index")
	fmt.Println()
	fmt.Println("  # List files in index")
	fmt.Println("  noisefs-mount -list-files")
	fmt.Println()
	fmt.Println("  # Add file to index")
	fmt.Println("  noisefs-mount -add-file filename.txt:QmXXX...:1024")
	fmt.Println()
	fmt.Println("  # Remove file from index")
	fmt.Println("  noisefs-mount -remove-file filename.txt")
	fmt.Println()
	fmt.Println("Bootstrap Operations:")
	fmt.Println("  # List available bootstrap datasets")
	fmt.Println("  noisefs-mount -list-bootstrap")
	fmt.Println()
	fmt.Println("  # Mount with bootstrap data")
	fmt.Println("  noisefs-mount -mount /mnt/noisefs -bootstrap")
	fmt.Println()
	fmt.Println("  # Mount with specific bootstrap dataset")
	fmt.Println("  noisefs-mount -mount /mnt/noisefs -bootstrap -bootstrap-data books")
	fmt.Println()
	fmt.Println("  # Mount with limited bootstrap size")
	fmt.Println("  noisefs-mount -mount /mnt/noisefs -bootstrap -bootstrap-size 50000000")
	fmt.Println()
	fmt.Println("Requirements:")
	fmt.Println("  - IPFS daemon running at specified endpoint")
	fmt.Println("  - macFUSE or FUSE installed (macOS/Linux)")
	fmt.Println("  - Mount point directory exists and is accessible")
	fmt.Println()
	fmt.Println("Once mounted, you can use standard file operations:")
	fmt.Println("  cp file.txt /mnt/noisefs/files/")
	fmt.Println("  ls /mnt/noisefs/files/")
	fmt.Println("  cat /mnt/noisefs/files/file.txt")
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

func mountFS(mountPath, volumeName, ipfsAPI string, cacheSize int, readOnly, allowOther, debug, daemon bool, pidFile, indexFile string, logger *logging.Logger) {
	// Clean mount path
	mountPath = filepath.Clean(mountPath)

	// Create storage manager
	logger.Info("Connecting to storage for mount", map[string]interface{}{
		"ipfs_api": ipfsAPI,
	})
	storageConfig := storage.DefaultConfig()
	if ipfsBackend, exists := storageConfig.Backends["ipfs"]; exists {
		ipfsBackend.Connection.Endpoint = ipfsAPI
	}
	
	storageManager, err := storage.NewManager(storageConfig)
	if err != nil {
		logger.Error("Failed to create storage manager", map[string]interface{}{
			"ipfs_api": ipfsAPI,
			"error":    err.Error(),
		})
		os.Exit(1)
	}
	
	err = storageManager.Start(context.Background())
	if err != nil {
		logger.Error("Failed to start storage manager", map[string]interface{}{
			"error": err.Error(),
		})
		os.Exit(1)
	}
	defer storageManager.Stop(context.Background())

	// Create cache
	logger.Debug("Initializing cache for mount", map[string]interface{}{
		"cache_size": cacheSize,
	})
	blockCache := cache.NewMemoryCache(cacheSize)

	// Create NoiseFS client
	client, err := noisefs.NewClient(storageManager, blockCache)
	if err != nil {
		logger.Error("Failed to create NoiseFS client", map[string]interface{}{
			"error": err.Error(),
		})
		os.Exit(1)
	}

	// Mount options
	opts := fuse.MountOptions{
		MountPath:  mountPath,
		VolumeName: volumeName,
		ReadOnly:   readOnly,
		AllowOther: allowOther,
		Debug:      debug,
	}

	fmt.Printf("Mounting NoiseFS at: %s\n", mountPath)
	fmt.Printf("IPFS endpoint: %s\n", ipfsAPI)
	fmt.Printf("Cache size: %d blocks\n", cacheSize)
	fmt.Printf("Volume name: %s\n", volumeName)

	if daemon {
		fmt.Println("Running in daemon mode...")
		if pidFile != "" {
			fmt.Printf("PID file: %s\n", pidFile)
		}
		err = fuse.DaemonWithIndex(client, storageManager, opts, pidFile, indexFile)
	} else {
		err = fuse.MountWithIndex(client, storageManager, opts, indexFile)
	}

	if err != nil {
		log.Fatalf("Mount failed: %v", err)
	}
}

func unmountFS(mountPath string) {
	mountPath = filepath.Clean(mountPath)
	
	fmt.Printf("Unmounting filesystem at: %s\n", mountPath)
	
	err := fuse.Unmount(mountPath)
	if err != nil {
		log.Fatalf("Unmount failed: %v", err)
	}
	
	fmt.Println("Filesystem unmounted successfully")
}

func listMounts() {
	fmt.Println("Mounted NoiseFS filesystems:")
	fmt.Println("============================")
	
	mounts, err := fuse.ListMounts()
	if err != nil {
		log.Fatalf("Failed to list mounts: %v", err)
	}
	
	if len(mounts) == 0 {
		fmt.Println("No NoiseFS filesystems currently mounted")
		return
	}
	
	for _, mount := range mounts {
		fmt.Printf("Mount Path: %s\n", mount.MountPath)
		fmt.Printf("Volume:     %s\n", mount.VolumeName)
		fmt.Printf("Read-Only:  %v\n", mount.ReadOnly)
		fmt.Printf("PID:        %d\n", mount.PID)
		fmt.Println()
	}
}

func handleIndexOperations(indexFile string, showIndex, listFiles bool, addFile, removeFile string) {
	// Get index path
	var indexPath string
	var err error
	
	if indexFile != "" {
		indexPath = indexFile
	} else {
		indexPath, err = fuse.GetDefaultIndexPath()
		if err != nil {
			log.Fatalf("Failed to get index path: %v", err)
		}
	}
	
	// Create and load index
	index := fuse.NewFileIndex(indexPath)
	if err := index.LoadIndex(); err != nil {
		log.Fatalf("Failed to load index: %v", err)
	}
	
	// Handle operations
	if showIndex {
		fmt.Printf("Index file: %s\n", indexPath)
		fmt.Printf("Files: %d\n", index.GetSize())
		if index.IsDirty() {
			fmt.Println("Status: Has unsaved changes")
		} else {
			fmt.Println("Status: Saved")
		}
	}
	
	if listFiles {
		files := index.ListFiles()
		if len(files) == 0 {
			fmt.Println("No files in index")
		} else {
			fmt.Printf("Files in index (%d):\n", len(files))
			for path, entry := range files {
				fmt.Printf("  %s -> %s (%d bytes, %s)\n", 
					path, entry.DescriptorCID, entry.FileSize, entry.CreatedAt.Format("2006-01-02 15:04:05"))
			}
		}
	}
	
	if addFile != "" {
		parts := strings.Split(addFile, ":")
		if len(parts) != 3 {
			log.Fatal("add-file format: filename:descriptor_cid:size")
		}
		
		filename := parts[0]
		descriptorCID := parts[1]
		fileSize, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			log.Fatalf("Invalid file size: %v", err)
		}
		
		index.AddFile(filename, descriptorCID, fileSize)
		if err := index.SaveIndex(); err != nil {
			log.Fatalf("Failed to save index: %v", err)
		}
		fmt.Printf("Added file: %s\n", filename)
	}
	
	if removeFile != "" {
		if index.RemoveFile(removeFile) {
			if err := index.SaveIndex(); err != nil {
				log.Fatalf("Failed to save index: %v", err)
			}
			fmt.Printf("Removed file: %s\n", removeFile)
		} else {
			fmt.Printf("File not found: %s\n", removeFile)
		}
	}
}

func init() {
	// Set up logging
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	
	// Check if running as root (may be required for mounting)
	if os.Geteuid() == 0 {
		fmt.Println("Warning: Running as root")
	}
}

// handleBootstrap downloads and uploads bootstrap data directly to NoiseFS
func handleBootstrap(cfg *config.Config, dataset string, maxSize int64, bootstrapDir string, logger *logging.Logger) {
	logger.Info("Starting bootstrap process", map[string]interface{}{
		"dataset": dataset,
		"max_size": maxSize,
		"bootstrap_dir": bootstrapDir,
	})
	
	// Set default bootstrap directory if not provided
	if bootstrapDir == "" {
		bootstrapDir = filepath.Join(os.TempDir(), "noisefs_bootstrap")
	}
	
	// Create bootstrap configuration
	bootstrapConfig := &bootstrap.Config{
		OutputDir:         bootstrapDir,
		Dataset:           dataset,
		MaxSize:           maxSize,
		Verbose:           cfg.FUSE.Debug,
		ParallelDownloads: 4,
	}
	
	// Download bootstrap data
	fmt.Printf("Downloading bootstrap dataset '%s'...\n", dataset)
	generator := bootstrap.NewDatasetGenerator(bootstrapConfig)
	if err := generator.GenerateDataset(); err != nil {
		logger.Error("Failed to generate bootstrap dataset", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}
	
	// Get summary
	summary, err := generator.GetSummary()
	if err != nil {
		logger.Error("Failed to get bootstrap summary", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}
	
	fmt.Printf("Downloaded %d files (%.2f MB)\n", summary.TotalFiles, float64(summary.TotalSize)/(1024*1024))
	
	// Create IPFS client and NoiseFS client for direct upload
	fmt.Printf("Uploading bootstrap data to NoiseFS...\n")
	if err := uploadBootstrapDataDirectly(bootstrapDir, cfg, logger); err != nil {
		logger.Error("Failed to upload bootstrap data", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}
	
	fmt.Println("Bootstrap process completed successfully!")
	fmt.Printf("Files will be available in the mounted filesystem at: %s\n", cfg.FUSE.MountPath)
	
	// Clean up bootstrap directory
	if err := os.RemoveAll(bootstrapDir); err != nil {
		logger.Warn("Failed to clean up bootstrap directory", map[string]interface{}{
			"directory": bootstrapDir,
			"error": err.Error(),
		})
	}
}

// uploadBootstrapDataDirectly uploads files directly to NoiseFS before mounting
func uploadBootstrapDataDirectly(srcDir string, cfg *config.Config, logger *logging.Logger) error {
	// Create storage manager
	storageConfig := storage.DefaultConfig()
	if ipfsBackend, exists := storageConfig.Backends["ipfs"]; exists {
		ipfsBackend.Connection.Endpoint = cfg.IPFS.APIEndpoint
	}
	
	storageManager, err := storage.NewManager(storageConfig)
	if err != nil {
		return fmt.Errorf("failed to create storage manager: %w", err)
	}
	
	err = storageManager.Start(context.Background())
	if err != nil {
		return fmt.Errorf("failed to start storage manager: %w", err)
	}
	defer storageManager.Stop(context.Background())
	
	// Create cache
	blockCache := cache.NewMemoryCache(cfg.Cache.BlockCacheSize)
	
	// Create NoiseFS client
	client, err := noisefs.NewClient(storageManager, blockCache)
	if err != nil {
		return fmt.Errorf("failed to create NoiseFS client: %w", err)
	}
	
	// Create or load index
	indexPath := cfg.FUSE.IndexPath
	if indexPath == "" {
		indexPath, err = fuse.GetDefaultIndexPath()
		if err != nil {
			return fmt.Errorf("failed to get default index path: %w", err)
		}
	}
	
	index := fuse.NewFileIndex(indexPath)
	if err := index.LoadIndex(); err != nil {
		logger.Debug("Creating new index file", map[string]interface{}{
			"path": indexPath,
		})
	}
	
	// Walk through bootstrap directory and upload files
	fileCount := 0
	err = filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		// Skip metadata files and directories
		if info.IsDir() || strings.HasSuffix(path, ".meta") {
			return nil
		}
		
		// Calculate relative path
		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		
		// Read file content
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", path, err)
		}
		
		// Upload to NoiseFS using block operations
		logger.Debug("Uploading bootstrap file", map[string]interface{}{
			"file": relPath,
			"size": len(content),
		})
		
		descriptorCID, err := storeFileInNoiseFS(client, relPath, content)
		if err != nil {
			return fmt.Errorf("failed to store file %s: %w", relPath, err)
		}
		
		// Add to index
		index.AddFile(relPath, descriptorCID, int64(len(content)))
		fileCount++
		
		if fileCount%10 == 0 {
			fmt.Printf("Uploaded %d files...\n", fileCount)
		}
		
		return nil
	})
	
	if err != nil {
		return err
	}
	
	// Save the index
	if err := index.SaveIndex(); err != nil {
		return fmt.Errorf("failed to save index: %w", err)
	}
	
	fmt.Printf("Successfully uploaded %d files to NoiseFS\n", fileCount)
	return nil
}

// storeFileInNoiseFS stores a file in NoiseFS using the block-based system
func storeFileInNoiseFS(client *noisefs.Client, filename string, content []byte) (string, error) {
	// Create splitter
	splitter, err := blocks.NewSplitter(blocks.DefaultBlockSize)
	if err != nil {
		return "", fmt.Errorf("failed to create splitter: %w", err)
	}
	
	// Split file into blocks
	fileBlocks, err := splitter.SplitBytes(content)
	if err != nil {
		return "", fmt.Errorf("failed to split file: %w", err)
	}
	
	// Create descriptor
	descriptor := descriptors.NewDescriptor(filename, int64(len(content)), blocks.DefaultBlockSize)
	
	// Process each block
	for _, dataBlock := range fileBlocks {
		// Select randomizers for 3-tuple anonymization
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
		descriptor.AddBlockTriple(dataCID, cid1, cid2)
	}
	
	// Store descriptor
	descriptorData, err := descriptor.ToJSON()
	if err != nil {
		return "", fmt.Errorf("failed to serialize descriptor: %w", err)
	}
	
	// Create block from descriptor data
	descriptorBlock, err := blocks.NewBlock(descriptorData)
	if err != nil {
		return "", fmt.Errorf("failed to create descriptor block: %w", err)
	}
	
	// Store descriptor in IPFS
	descriptorCID, err := client.StoreBlockWithCache(descriptorBlock)
	if err != nil {
		return "", fmt.Errorf("failed to store descriptor: %w", err)
	}
	
	return descriptorCID, nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()
	
	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()
	
	_, err = io.Copy(dstFile, srcFile)
	return err
}