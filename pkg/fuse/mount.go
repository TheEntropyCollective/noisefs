// +build fuse

package fuse

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"github.com/TheEntropyCollective/noisefs/pkg/noisefs"
)

// MountOptions contains options for mounting the filesystem
type MountOptions struct {
	MountPath   string
	VolumeName  string
	ReadOnly    bool
	AllowOther  bool
	Debug       bool
}

// Mount mounts the NoiseFS FUSE filesystem
func Mount(client *noisefs.Client, opts MountOptions) error {
	// Ensure mount point exists
	if err := os.MkdirAll(opts.MountPath, 0755); err != nil {
		return fmt.Errorf("failed to create mount point: %w", err)
	}
	
	// Check if already mounted
	if isMounted(opts.MountPath) {
		return fmt.Errorf("filesystem already mounted at %s", opts.MountPath)
	}
	
	// Prepare mount options
	mountOpts := []fuse.MountOption{
		fuse.FSName("noisefs"),
		fuse.Subtype("noisefs"),
		fuse.LocalVolume(),
		fuse.VolumeName(opts.VolumeName),
	}
	
	if opts.ReadOnly {
		mountOpts = append(mountOpts, fuse.ReadOnly())
	}
	
	if opts.AllowOther {
		mountOpts = append(mountOpts, fuse.AllowOther())
	}
	
	if opts.Debug {
		fuse.Debug = func(msg interface{}) {
			fmt.Printf("FUSE DEBUG: %v\n", msg)
		}
	}
	
	// Mount filesystem
	conn, err := fuse.Mount(opts.MountPath, mountOpts...)
	if err != nil {
		return fmt.Errorf("failed to mount filesystem: %w", err)
	}
	defer conn.Close()
	
	// Create filesystem
	fuseFS := NewFS(client, opts.MountPath)
	
	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	
	// Start serving in a goroutine
	errChan := make(chan error, 1)
	go func() {
		err := fs.Serve(conn, fuseFS)
		if err != nil {
			errChan <- fmt.Errorf("filesystem server error: %w", err)
		}
	}()
	
	// Wait for mount to be ready
	<-conn.Ready
	if err := conn.MountError; err != nil {
		return fmt.Errorf("mount failed: %w", err)
	}
	
	fmt.Printf("NoiseFS mounted at: %s\n", opts.MountPath)
	fmt.Printf("Volume name: %s\n", opts.VolumeName)
	fmt.Println("Press Ctrl+C to unmount")
	
	// Wait for shutdown signal or error
	select {
	case <-sigChan:
		fmt.Println("\nShutting down...")
	case err := <-errChan:
		return err
	}
	
	return nil
}

// Unmount unmounts the filesystem at the given path
func Unmount(mountPath string) error {
	if !isMounted(mountPath) {
		return fmt.Errorf("no filesystem mounted at %s", mountPath)
	}
	
	err := fuse.Unmount(mountPath)
	if err != nil {
		return fmt.Errorf("failed to unmount filesystem: %w", err)
	}
	
	fmt.Printf("NoiseFS unmounted from: %s\n", mountPath)
	return nil
}

// isMounted checks if a filesystem is mounted at the given path
func isMounted(mountPath string) bool {
	// Clean the path
	cleanPath := filepath.Clean(mountPath)
	
	// Try to read /proc/mounts on Linux
	if file, err := os.Open("/proc/mounts"); err == nil {
		defer file.Close()
		
		// Read mounts and check if our path is mounted
		// This is a simplified check - in production we'd parse the file properly
		stat, err := os.Stat(cleanPath)
		if err != nil {
			return false
		}
		
		// Check if the directory looks like a mount point
		parent := filepath.Dir(cleanPath)
		parentStat, err := os.Stat(parent)
		if err != nil {
			return false
		}
		
		// Different device means it's likely a mount point
		return stat.Sys() != parentStat.Sys()
	}
	
	// Fallback: try to detect mount on other systems
	// This is a basic check - just see if directory exists
	if _, err := os.Stat(cleanPath); err != nil {
		return false
	}
	
	return false
}

// MountInfo contains information about mounted filesystems
type MountInfo struct {
	MountPath  string
	VolumeName string
	ReadOnly   bool
	PID        int
}

// ListMounts returns information about mounted NoiseFS filesystems
func ListMounts() ([]MountInfo, error) {
	// This would typically parse /proc/mounts or use system calls
	// For now, return empty list
	return []MountInfo{}, nil
}

// Daemon runs the FUSE filesystem as a background daemon
func Daemon(client *noisefs.Client, opts MountOptions, pidFile string) error {
	// Write PID file if specified
	if pidFile != "" {
		if err := writePIDFile(pidFile); err != nil {
			return fmt.Errorf("failed to write PID file: %w", err)
		}
		defer os.Remove(pidFile)
	}
	
	// Mount filesystem
	return Mount(client, opts)
}

// writePIDFile writes the current process ID to a file
func writePIDFile(pidFile string) error {
	file, err := os.Create(pidFile)
	if err != nil {
		return err
	}
	defer file.Close()
	
	_, err = fmt.Fprintf(file, "%d\n", os.Getpid())
	return err
}

// StopDaemon stops a running daemon by reading its PID file
func StopDaemon(pidFile string) error {
	// Read PID from file
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return fmt.Errorf("failed to read PID file: %w", err)
	}
	
	var pid int
	if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil {
		return fmt.Errorf("invalid PID file format: %w", err)
	}
	
	// Find process and send termination signal
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process: %w", err)
	}
	
	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to terminate process: %w", err)
	}
	
	fmt.Printf("Sent termination signal to PID %d\n", pid)
	return nil
}