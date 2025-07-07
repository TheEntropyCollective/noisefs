package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/TheEntropyCollective/noisefs/pkg/cache"
	"github.com/TheEntropyCollective/noisefs/pkg/fuse"
	"github.com/TheEntropyCollective/noisefs/pkg/ipfs"
	"github.com/TheEntropyCollective/noisefs/pkg/noisefs"
)

func main() {
	var (
		mountPath    = flag.String("mount", "", "Mount point for the filesystem (required)")
		volumeName   = flag.String("volume", "NoiseFS", "Volume name")
		ipfsAPI      = flag.String("ipfs", "localhost:5001", "IPFS API endpoint")
		cacheSize    = flag.Int("cache", 100, "Cache size (number of blocks)")
		readOnly     = flag.Bool("readonly", false, "Mount as read-only")
		allowOther   = flag.Bool("allow-other", false, "Allow other users to access")
		debug        = flag.Bool("debug", false, "Enable debug output")
		daemon       = flag.Bool("daemon", false, "Run as daemon")
		pidFile      = flag.String("pidfile", "", "PID file for daemon mode")
		unmount      = flag.Bool("unmount", false, "Unmount filesystem")
		list         = flag.Bool("list", false, "List mounted filesystems")
		help         = flag.Bool("help", false, "Show help message")
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

	if *mountPath == "" {
		log.Fatal("Mount path is required")
	}

	// Mount filesystem
	mountFS(*mountPath, *volumeName, *ipfsAPI, *cacheSize, *readOnly, *allowOther, *debug, *daemon, *pidFile)
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

func mountFS(mountPath, volumeName, ipfsAPI string, cacheSize int, readOnly, allowOther, debug, daemon bool, pidFile string) {
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
		err = fuse.Daemon(client, opts, pidFile)
	} else {
		err = fuse.Mount(client, opts)
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

func init() {
	// Set up logging
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	
	// Check if running as root (may be required for mounting)
	if os.Geteuid() == 0 {
		fmt.Println("Warning: Running as root")
	}
}