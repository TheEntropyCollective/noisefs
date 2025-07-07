package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/TheEntropyCollective/noisefs/pkg/cache"
	"github.com/TheEntropyCollective/noisefs/pkg/config"
	"github.com/TheEntropyCollective/noisefs/pkg/fuse"
	"github.com/TheEntropyCollective/noisefs/pkg/ipfs"
	"github.com/TheEntropyCollective/noisefs/pkg/noisefs"
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
		log.Fatal("Mount path is required")
	}

	// Mount filesystem
	mountFS(cfg.FUSE.MountPath, cfg.FUSE.VolumeName, cfg.IPFS.APIEndpoint, cfg.Cache.BlockCacheSize, 
		cfg.FUSE.ReadOnly, cfg.FUSE.AllowOther, cfg.FUSE.Debug, *daemon, *pidFile, cfg.FUSE.IndexPath)
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

func mountFS(mountPath, volumeName, ipfsAPI string, cacheSize int, readOnly, allowOther, debug, daemon bool, pidFile, indexFile string) {
	// Clean mount path
	mountPath = filepath.Clean(mountPath)

	// Create IPFS client
	ipfsClient, err := ipfs.NewClient(ipfsAPI)
	if err != nil {
		log.Fatalf("Failed to create IPFS client: %v", err)
	}

	// Create cache
	blockCache := cache.NewMemoryCache(cacheSize)

	// Create NoiseFS client
	client, err := noisefs.NewClient(ipfsClient, blockCache)
	if err != nil {
		log.Fatalf("Failed to create NoiseFS client: %v", err)
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
		err = fuse.DaemonWithIndex(client, ipfsClient, opts, pidFile, indexFile)
	} else {
		err = fuse.MountWithIndex(client, ipfsClient, opts, indexFile)
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