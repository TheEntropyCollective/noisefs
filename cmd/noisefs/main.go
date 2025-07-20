package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	noisefs "github.com/TheEntropyCollective/noisefs/pkg/core/client"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/config"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/logging"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	_ "github.com/TheEntropyCollective/noisefs/pkg/storage/backends" // Import to register backends
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
	"github.com/TheEntropyCollective/noisefs/pkg/util"
)

func main() {
	// Show legal disclaimer on first run or if not accepted recently
	// Skip if running in test mode
	if os.Getenv("NOISEFS_SKIP_LEGAL_NOTICE") != "1" && !checkLegalDisclaimerAccepted() {
		showLegalDisclaimer()
	}

	// Parse command line flags
	config := parseFlags()

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
	cfg, err := loadConfig(config.ConfigFile)
	if err != nil {
		if config.JSONOutput {
			util.PrintJSONError(err)
		} else {
			fmt.Fprintf(os.Stderr, "%s\n", util.FormatError(err))
		}
		os.Exit(1)
	}

	// Initialize logging
	if err := logging.InitFromConfig(cfg.Logging.Level, cfg.Logging.Format, cfg.Logging.Output, cfg.Logging.File); err != nil {
		if config.JSONOutput {
			util.PrintJSONError(err)
		} else {
			fmt.Fprintf(os.Stderr, "%s\n", util.FormatError(err))
		}
		os.Exit(1)
	}

	logger := logging.GetGlobalLogger().WithComponent("noisefs")

	// Apply command-line overrides to configuration
	applyConfigOverrides(cfg, config)

	// Initialize storage manager
	storageManager, err := initializeStorageManager(cfg)
	if err != nil {
		if config.JSONOutput {
			util.PrintJSONError(err)
		} else {
			fmt.Fprintf(os.Stderr, "%s\n", util.FormatError(err))
		}
		os.Exit(1)
	}
	defer storageManager.Stop(context.Background())

	// Create cache
	blockCache := cache.NewMemoryCache(cfg.Cache.BlockCacheSize)

	// Create NoiseFS client
	noisefsClient, err := noisefs.NewClient(storageManager, blockCache)
	if err != nil {
		if config.JSONOutput {
			util.PrintJSONError(err)
		} else {
			fmt.Fprintf(os.Stderr, "%s\n", util.FormatError(err))
		}
		os.Exit(1)
	}

	// Execute the requested operation
	err = executeOperation(config, cfg, storageManager, noisefsClient, blockCache, logger)
	if err != nil {
		if config.JSONOutput {
			util.PrintJSONError(err)
		} else {
			fmt.Fprintf(os.Stderr, "%s\n", util.FormatError(err))
		}
		os.Exit(1)
	}
}

// CommandConfig holds all parsed command line flags
type CommandConfig struct {
	ConfigFile            string
	IPFSAPI              string
	Upload               string
	Download             string
	Output               string
	Recursive            bool
	Exclude              string
	Stats                bool
	Quiet                bool
	JSONOutput           bool
	BlockSize            int
	CacheSize            int
	Workers              int
	MinPersonalCacheMB   int
	DisableAltruistic    bool
	AltruisticBandwidthMB int
	Streaming            bool
	MemoryLimitMB        int
	StreamBufferSize     int
	EnableMemMonitoring  bool
	// Encryption flags
	Encrypt              bool
	Password             string
}

// parseFlags parses command line flags and returns a configuration
func parseFlags() *CommandConfig {
	config := &CommandConfig{}

	flag.StringVar(&config.ConfigFile, "config", "", "Configuration file path")
	flag.StringVar(&config.IPFSAPI, "api", "", "IPFS API endpoint (overrides config)")
	flag.StringVar(&config.Upload, "upload", "", "File or directory to upload to NoiseFS (uses parallel processing)")
	flag.StringVar(&config.Download, "download", "", "Descriptor CID to download from NoiseFS")
	flag.StringVar(&config.Output, "output", "", "Output file path for download")
	flag.BoolVar(&config.Recursive, "r", false, "Recursively upload/download directories")
	flag.StringVar(&config.Exclude, "exclude", "", "Comma-separated list of file patterns to exclude from directory upload")
	flag.BoolVar(&config.Stats, "stats", false, "Show NoiseFS statistics")
	flag.BoolVar(&config.Quiet, "quiet", false, "Minimal output (only show errors and results)")
	flag.BoolVar(&config.JSONOutput, "json", false, "Output results in JSON format")
	flag.IntVar(&config.BlockSize, "block-size", 0, "Block size in bytes (overrides config)")
	flag.IntVar(&config.CacheSize, "cache-size", 0, "Number of blocks to cache in memory (overrides config)")
	flag.IntVar(&config.Workers, "workers", 0, "Number of parallel workers for upload/download (overrides config)")
	
	// Altruistic cache flags
	flag.IntVar(&config.MinPersonalCacheMB, "min-personal-cache", 0, "Minimum personal cache size in MB (overrides config)")
	flag.BoolVar(&config.DisableAltruistic, "disable-altruistic", false, "Disable altruistic caching")
	flag.IntVar(&config.AltruisticBandwidthMB, "altruistic-bandwidth", 0, "Bandwidth limit for altruistic operations in MB/s")
	
	// Streaming flags
	flag.BoolVar(&config.Streaming, "streaming", false, "Use streaming mode for upload/download with bounded memory")
	flag.IntVar(&config.MemoryLimitMB, "memory-limit", 0, "Memory limit for streaming operations in MB (overrides config)")
	flag.IntVar(&config.StreamBufferSize, "stream-buffer", 0, "Buffer size for streaming pipeline (overrides config)")
	flag.BoolVar(&config.EnableMemMonitoring, "monitor-memory", false, "Enable memory monitoring during streaming operations")
	
	// Encryption flags
	flag.BoolVar(&config.Encrypt, "e", false, "Encrypt descriptor metadata for privacy")
	flag.BoolVar(&config.Encrypt, "encrypt", false, "Encrypt descriptor metadata for privacy")
	flag.StringVar(&config.Password, "p", "", "Password for descriptor encryption")
	flag.StringVar(&config.Password, "password", "", "Password for descriptor encryption")

	return config
}

// applyConfigOverrides applies command-line flag overrides to the configuration
func applyConfigOverrides(cfg *config.Config, cmdConfig *CommandConfig) {
	if cmdConfig.IPFSAPI != "" {
		cfg.IPFS.APIEndpoint = cmdConfig.IPFSAPI
	}
	if cmdConfig.BlockSize != 0 {
		cfg.Performance.BlockSize = cmdConfig.BlockSize
	}
	if cmdConfig.CacheSize != 0 {
		cfg.Cache.BlockCacheSize = cmdConfig.CacheSize
	}
	if cmdConfig.Workers != 0 {
		// TODO: Add Workers field to PerformanceConfig
		// cfg.Performance.Workers = cmdConfig.Workers
		_ = cmdConfig.Workers // Suppress unused variable warning
	}
	if cmdConfig.MinPersonalCacheMB != 0 {
		cfg.Cache.MinPersonalCacheMB = cmdConfig.MinPersonalCacheMB
	}
	if cmdConfig.DisableAltruistic {
		cfg.Cache.EnableAltruistic = false
	}
	if cmdConfig.AltruisticBandwidthMB != 0 {
		cfg.Cache.AltruisticBandwidthMB = cmdConfig.AltruisticBandwidthMB
	}
	if cmdConfig.MemoryLimitMB != 0 {
		cfg.Performance.MemoryLimit = cmdConfig.MemoryLimitMB
	}
	if cmdConfig.StreamBufferSize != 0 {
		cfg.Performance.StreamBufferSize = cmdConfig.StreamBufferSize
	}
}

// executeOperation executes the main operation based on command line flags
func executeOperation(cmdConfig *CommandConfig, cfg *config.Config, storageManager *storage.Manager, client *noisefs.Client, blockCache cache.Cache, logger *logging.Logger) error {
	// Show statistics if requested
	if cmdConfig.Stats {
		showSystemStats(storageManager, client, blockCache, cmdConfig.JSONOutput, logger)
		return nil
	}

	// Handle upload operation
	if cmdConfig.Upload != "" {
		return handleUpload(cmdConfig, cfg, storageManager, client, logger)
	}

	// Handle download operation
	if cmdConfig.Download != "" {
		return handleDownload(cmdConfig, cfg, storageManager, client, logger)
	}

	// No operation specified, show usage
	showUsage()
	return nil
}

// handleUpload handles file or directory upload operations
func handleUpload(cmdConfig *CommandConfig, cfg *config.Config, storageManager *storage.Manager, client *noisefs.Client, logger *logging.Logger) error {
	uploadPath := cmdConfig.Upload

	// Handle password from environment variable if not provided via flag
	password := cmdConfig.Password
	if password == "" && cmdConfig.Encrypt {
		password = os.Getenv("NOISEFS_PASSWORD")
	}

	// Check if path exists
	info, err := os.Stat(uploadPath)
	if err != nil {
		return fmt.Errorf("upload path does not exist: %w", err)
	}

	// Handle directory upload
	if info.IsDir() {
		if cmdConfig.Streaming {
			return streamingUploadDirectory(storageManager, client, uploadPath, cmdConfig.BlockSize, cmdConfig.Exclude, cmdConfig.Quiet, cmdConfig.JSONOutput, cfg, logger, cmdConfig.Encrypt, password)
		} else {
			return uploadDirectory(storageManager, client, uploadPath, cmdConfig.BlockSize, cmdConfig.Exclude, cmdConfig.Quiet, cmdConfig.JSONOutput, cfg, logger, cmdConfig.Encrypt, password)
		}
	}

	// Handle file upload
	return uploadFile(storageManager, client, uploadPath, cmdConfig.BlockSize, cmdConfig.Quiet, cmdConfig.JSONOutput, cfg, logger, cmdConfig.Encrypt, password)
}

// handleDownload handles file or directory download operations
func handleDownload(cmdConfig *CommandConfig, cfg *config.Config, storageManager *storage.Manager, client *noisefs.Client, logger *logging.Logger) error {
	descriptorCID := cmdConfig.Download

	// Check if this is a directory descriptor
	isDirectory, err := detectDirectoryDescriptor(storageManager, descriptorCID)
	if err != nil {
		// If we can't detect, try as file first
		isDirectory = false
	}

	if isDirectory {
		if cmdConfig.Streaming {
			return streamingDownloadDirectory(storageManager, client, descriptorCID, cmdConfig.Output, cmdConfig.Quiet, cmdConfig.JSONOutput, cfg, logger)
		} else {
			return downloadDirectory(storageManager, client, descriptorCID, cmdConfig.Output, cmdConfig.Quiet, cmdConfig.JSONOutput, cfg, logger)
		}
	}

	// Handle as file download
	return downloadFile(storageManager, client, descriptorCID, cmdConfig.Output, cmdConfig.Quiet, cmdConfig.JSONOutput, logger)
}

// showUsage displays usage information
func showUsage() {
	fmt.Println("NoiseFS - Privacy-Preserving Distributed Storage")
	fmt.Println("===============================================")
	fmt.Println()
	fmt.Println("USAGE:")
	fmt.Println("  noisefs [OPTIONS] COMMAND")
	fmt.Println()
	fmt.Println("COMMANDS:")
	fmt.Println("  -upload <path>        Upload file or directory")
	fmt.Println("  -download <cid>       Download file or directory")
	fmt.Println("  -stats                Show system statistics")
	fmt.Println()
	fmt.Println("SUBCOMMANDS:")
	fmt.Println("  ls <dir-cid>          List directory contents")
	fmt.Println("  announce              Publish file announcements")
	fmt.Println("  subscribe             Subscribe to announcements")
	fmt.Println("  discover              Discover available content")
	fmt.Println("  search                Search for content")
	fmt.Println("  sync                  Synchronize directories")
	fmt.Println("  share-directory       Create shareable directory snapshot")
	fmt.Println("  receive-directory     Receive shared directory")
	fmt.Println("  list-snapshots        List available snapshots")
	fmt.Println()
	fmt.Println("OPTIONS:")
	fmt.Println("  -config <file>        Configuration file path")
	fmt.Println("  -api <endpoint>       IPFS API endpoint")
	fmt.Println("  -output <path>        Output path for downloads")
	fmt.Println("  -r                    Recursive directory operations")
	fmt.Println("  -exclude <patterns>   Exclude file patterns (comma-separated)")
	fmt.Println("  -quiet                Minimal output")
	fmt.Println("  -json                 JSON output format")
	fmt.Println("  -block-size <bytes>   Block size override")
	fmt.Println("  -cache-size <count>   Cache size override")
	fmt.Println("  -workers <count>      Parallel workers override")
	fmt.Println("  -streaming            Enable streaming mode")
	fmt.Println("  -memory-limit <mb>    Memory limit for streaming")
	fmt.Println("  -monitor-memory       Enable memory monitoring")
	fmt.Println("  -e, --encrypt         Encrypt descriptor metadata for privacy")
	fmt.Println("  -p, --password <pwd>  Password for descriptor encryption")
	fmt.Println()
	fmt.Println("EXAMPLES:")
	fmt.Println("  noisefs -upload myfile.txt")
	fmt.Println("  noisefs -upload -r mydirectory/")
	fmt.Println("  noisefs -download QmXxXxXx... -output downloaded.txt")
	fmt.Println("  noisefs -stats -json")
	fmt.Println("  noisefs ls QmYyYyYy...")
	fmt.Println("  noisefs -upload -streaming -memory-limit 512 largedir/")
	fmt.Println()
	fmt.Println("ENCRYPTION EXAMPLES:")
	fmt.Println("  noisefs -upload -e -p mypassword secret.txt")
	fmt.Println("  noisefs -upload -e secret.txt                    # Interactive password prompt")
	fmt.Println("  NOISEFS_PASSWORD=secret noisefs -upload -e file.txt")
	fmt.Println()
	fmt.Println("PRIVACY NOTE:")
	fmt.Println("  Without -e: File content is encrypted, but metadata (filename, size) is public")
	fmt.Println("  With -e:    Both file content and metadata are encrypted for complete privacy")
	fmt.Println()
	fmt.Println("For more information, see the documentation in docs/")
}