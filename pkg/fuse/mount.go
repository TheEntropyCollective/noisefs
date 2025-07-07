// +build fuse

package fuse

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/TheEntropyCollective/noisefs/pkg/ipfs"
	"github.com/TheEntropyCollective/noisefs/pkg/noisefs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/fuse/nodefs"
	"github.com/hanwen/go-fuse/v2/fuse/pathfs"
)

// MountOptions contains options for mounting the filesystem
type MountOptions struct {
	MountPath   string
	VolumeName  string
	ReadOnly    bool
	AllowOther  bool
	Debug       bool
}

// MountInfo contains information about mounted filesystems
type MountInfo struct {
	MountPath  string
	VolumeName string
	ReadOnly   bool
	PID        int
}

// Mount mounts the NoiseFS FUSE filesystem using go-fuse
func Mount(client *noisefs.Client, ipfsClient *ipfs.Client, opts MountOptions) error {
	return MountWithIndex(client, ipfsClient, opts, "")
}

// MountWithIndex mounts the NoiseFS FUSE filesystem with a custom index path
func MountWithIndex(client *noisefs.Client, ipfsClient *ipfs.Client, opts MountOptions, indexPath string) error {
	// Ensure mount point exists
	if err := os.MkdirAll(opts.MountPath, 0755); err != nil {
		return fmt.Errorf("failed to create mount point: %w", err)
	}

	// Determine index path
	if indexPath == "" {
		var err error
		indexPath, err = GetDefaultIndexPath()
		if err != nil {
			return fmt.Errorf("failed to get default index path: %w", err)
		}
	}

	// Create and load file index
	index := NewFileIndex(indexPath)
	if err := index.LoadIndex(); err != nil {
		return fmt.Errorf("failed to load file index: %w", err)
	}

	// Create NoiseFS filesystem
	nfs := &NoiseFS{
		FileSystem: pathfs.NewDefaultFileSystem(),
		client:     client,
		ipfsClient: ipfsClient,
		mountPath:  opts.MountPath,
		readOnly:   opts.ReadOnly,
		index:      index,
	}

	// Create path filesystem
	pathFs := pathfs.NewPathNodeFs(nfs, nil)

	// Create FUSE mount options
	fuseOpts := &fuse.MountOptions{
		Name:       "noisefs",
		FsName:     opts.VolumeName,
		AllowOther: opts.AllowOther,
		Debug:      opts.Debug,
	}
	
	// Create raw filesystem
	conn := nodefs.NewFileSystemConnector(pathFs.Root(), &nodefs.Options{
		Debug: opts.Debug,
	})
	
	// Create and mount the server
	server, err := fuse.NewServer(conn.RawFS(), opts.MountPath, fuseOpts)
	if err != nil {
		return fmt.Errorf("mount failed: %w", err)
	}

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	fmt.Printf("NoiseFS mounted at: %s\n", opts.MountPath)
	fmt.Printf("Volume name: %s\n", opts.VolumeName)
	fmt.Println("Press Ctrl+C to unmount")

	// Start serving in background
	go server.Serve()

	// Wait for shutdown signal
	<-sigChan
	fmt.Println("\nShutting down...")

	// Save index before unmounting
	if err := nfs.index.SaveIndex(); err != nil {
		fmt.Printf("Warning: Failed to save index: %v\n", err)
	}

	// Unmount
	err = server.Unmount()
	if err != nil {
		return fmt.Errorf("unmount failed: %w", err)
	}

	return nil
}

// Unmount unmounts the filesystem at the given path
func Unmount(mountPath string) error {
	// For go-fuse, we need to use system unmount
	err := os.RemoveAll(mountPath + "/.control")
	if err != nil {
		return fmt.Errorf("failed to remove control file: %w", err)
	}
	
	// Try to unmount using system command
	return syscall.Unmount(mountPath, 0)
}

// NoiseFS implements pathfs.FileSystem
type NoiseFS struct {
	pathfs.FileSystem
	client     *noisefs.Client
	ipfsClient *ipfs.Client
	mountPath  string
	readOnly   bool
	
	// Persistent file index
	index *FileIndex
}

// GetAttr implements pathfs.FileSystem
func (fs *NoiseFS) GetAttr(name string, context *fuse.Context) (*fuse.Attr, fuse.Status) {
	if name == "" {
		// Root directory
		return &fuse.Attr{
			Mode: fuse.S_IFDIR | 0755,
		}, fuse.OK
	}

	// Check if it's a directory
	if name == "files" || strings.HasPrefix(name, "files/") {
		// Check if it's the files directory itself
		if name == "files" {
			return &fuse.Attr{
				Mode: fuse.S_IFDIR | 0755,
			}, fuse.OK
		}
		
		// Check if it's a known file
		relativePath := strings.TrimPrefix(name, "files/")
		entry, exists := fs.index.GetFile(relativePath)
		
		if exists {
			// Return file attributes from index
			return &fuse.Attr{
				Mode:  fuse.S_IFREG | 0644,
				Size:  uint64(entry.FileSize),
				Mtime: uint64(entry.ModifiedAt.Unix()),
				Atime: uint64(entry.ModifiedAt.Unix()),
				Ctime: uint64(entry.CreatedAt.Unix()),
			}, fuse.OK
		}
	}

	return nil, fuse.ENOENT
}

// OpenDir implements pathfs.FileSystem
func (fs *NoiseFS) OpenDir(name string, context *fuse.Context) ([]fuse.DirEntry, fuse.Status) {
	if name == "" {
		// Root directory
		return []fuse.DirEntry{
			{Name: "files", Mode: fuse.S_IFDIR},
		}, fuse.OK
	}

	if name == "files" {
		// List all files in the index
		files := fs.index.GetFilesInDirectory("")
		entries := make([]fuse.DirEntry, 0, len(files))
		
		for _, entry := range files {
			entries = append(entries, fuse.DirEntry{
				Name: entry.Filename,
				Mode: fuse.S_IFREG,
			})
		}
		
		return entries, fuse.OK
	}

	return nil, fuse.ENOENT
}

// Open implements pathfs.FileSystem
func (fs *NoiseFS) Open(name string, flags uint32, context *fuse.Context) (nodefs.File, fuse.Status) {
	// Only handle files under the files directory
	if !strings.HasPrefix(name, "files/") {
		return nil, fuse.EINVAL
	}
	
	// Get relative path
	relativePath := strings.TrimPrefix(name, "files/")
	
	// Look up descriptor CID
	entry, exists := fs.index.GetFile(relativePath)
	if !exists {
		return nil, fuse.ENOENT
	}
	
	// Create NoiseFS file handle
	readOnly := (flags & fuse.O_ANYWRITE) == 0
	file := NewNoiseFile(fs.client, fs.ipfsClient, entry.DescriptorCID, relativePath, readOnly, fs.index)
	
	return file, fuse.OK
}

// Create implements pathfs.FileSystem
func (fs *NoiseFS) Create(name string, flags uint32, mode uint32, context *fuse.Context) (nodefs.File, fuse.Status) {
	if fs.readOnly {
		return nil, fuse.EROFS
	}

	// Only handle files under the files directory
	if !strings.HasPrefix(name, "files/") {
		return nil, fuse.EINVAL
	}
	
	// Get relative path
	relativePath := strings.TrimPrefix(name, "files/")
	
	// Create new NoiseFS file handle with empty descriptor CID (new file)
	file := NewNoiseFile(fs.client, fs.ipfsClient, "", relativePath, false, fs.index)
	
	return file, fuse.OK
}

// Mkdir implements pathfs.FileSystem
func (fs *NoiseFS) Mkdir(name string, mode uint32, context *fuse.Context) fuse.Status {
	if fs.readOnly {
		return fuse.EROFS
	}
	return fuse.OK
}

// Unlink implements pathfs.FileSystem
func (fs *NoiseFS) Unlink(name string, context *fuse.Context) fuse.Status {
	if fs.readOnly {
		return fuse.EROFS
	}
	return fuse.OK
}

// AddFile adds a file to the index and saves it
func (fs *NoiseFS) AddFile(filename, descriptorCID string, fileSize int64) error {
	fs.index.AddFile(filename, descriptorCID, fileSize)
	return fs.index.SaveIndex()
}

// RemoveFile removes a file from the index and saves it
func (fs *NoiseFS) RemoveFile(filename string) error {
	if fs.index.RemoveFile(filename) {
		return fs.index.SaveIndex()
	}
	return nil
}

// ListFiles returns all files in the index
func (fs *NoiseFS) ListFiles() map[string]*IndexEntry {
	return fs.index.ListFiles()
}

// GetIndex returns the file index for direct access
func (fs *NoiseFS) GetIndex() *FileIndex {
	return fs.index
}

// Daemon runs the FUSE filesystem as a background daemon
func Daemon(client *noisefs.Client, ipfsClient *ipfs.Client, opts MountOptions, pidFile string) error {
	return DaemonWithIndex(client, ipfsClient, opts, pidFile, "")
}

// DaemonWithIndex runs the FUSE filesystem as a background daemon with a custom index
func DaemonWithIndex(client *noisefs.Client, ipfsClient *ipfs.Client, opts MountOptions, pidFile, indexPath string) error {
	if pidFile != "" {
		if err := writePIDFile(pidFile); err != nil {
			return fmt.Errorf("failed to write PID file: %w", err)
		}
		defer os.Remove(pidFile)
	}
	
	return MountWithIndex(client, ipfsClient, opts, indexPath)
}

func writePIDFile(pidFile string) error {
	file, err := os.Create(pidFile)
	if err != nil {
		return err
	}
	defer file.Close()
	
	_, err = fmt.Fprintf(file, "%d\n", os.Getpid())
	return err
}

func StopDaemon(pidFile string) error {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return fmt.Errorf("failed to read PID file: %w", err)
	}
	
	var pid int
	if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil {
		return fmt.Errorf("invalid PID file format: %w", err)
	}
	
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

// ListMounts returns information about mounted NoiseFS filesystems
func ListMounts() ([]MountInfo, error) {
	// This would typically parse /proc/mounts or use system calls
	// For now, return empty list
	return []MountInfo{}, nil
}