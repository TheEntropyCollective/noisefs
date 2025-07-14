// +build fuse

package fuse

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/TheEntropyCollective/noisefs/pkg/core/client"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/security"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/fuse/nodefs"
	"github.com/hanwen/go-fuse/v2/fuse/pathfs"
)

// MountOptions contains options for mounting the filesystem
type MountOptions struct {
	MountPath      string
	VolumeName     string
	ReadOnly       bool
	AllowOther     bool
	Debug          bool
	Security       *security.SecurityManager
	IndexPassword  string
}

// MountInfo contains information about mounted filesystems
type MountInfo struct {
	MountPath  string
	VolumeName string
	ReadOnly   bool
	PID        int
}

// Mount mounts the NoiseFS FUSE filesystem using go-fuse
func Mount(client *noisefs.Client, storageManager *storage.Manager, opts MountOptions) error {
	return MountWithIndex(client, storageManager, opts, "")
}

// MountWithIndex mounts the NoiseFS FUSE filesystem with a custom index path
func MountWithIndex(client *noisefs.Client, storageManager *storage.Manager, opts MountOptions, indexPath string) error {
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

	// Create and load file index (encrypted if password provided)
	var index *FileIndex
	if opts.IndexPassword != "" {
		encIndex, err := NewEncryptedFileIndex(indexPath, opts.IndexPassword)
		if err != nil {
			return fmt.Errorf("failed to create encrypted index: %w", err)
		}
		defer encIndex.Cleanup()
		
		if err := encIndex.LoadIndex(); err != nil {
			return fmt.Errorf("failed to load encrypted index: %w", err)
		}
		
		// Lock memory if security manager is available
		if opts.Security != nil && opts.Security.MemoryProtection != nil {
			encIndex.LockMemory()
		}
		
		index = encIndex.FileIndex
	} else {
		// Use standard unencrypted index
		index = NewFileIndex(indexPath)
		if err := index.LoadIndex(); err != nil {
			return fmt.Errorf("failed to load file index: %w", err)
		}
	}

	// Create NoiseFS filesystem
	nfs := &NoiseFS{
		FileSystem: pathfs.NewDefaultFileSystem(),
		client:     client,
		storageManager: storageManager,
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
	client         *noisefs.Client
	storageManager *storage.Manager
	mountPath      string
	readOnly       bool
	
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
		
		// Get relative path
		relativePath := strings.TrimPrefix(name, "files/")
		
		// Check if it's a known file
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
		
		// Check if it's a directory by looking for files in subdirectories
		if fs.index.IsDirectory(relativePath) {
			return &fuse.Attr{
				Mode: fuse.S_IFDIR | 0755,
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

	if strings.HasPrefix(name, "files") {
		// Get relative directory path
		var dirPath string
		if name == "files" {
			dirPath = ""
		} else {
			dirPath = strings.TrimPrefix(name, "files/")
		}
		
		// Get files in this directory
		files := fs.index.GetFilesInDirectory(dirPath)
		
		// Track subdirectories we've seen
		subdirs := make(map[string]bool)
		
		// Find subdirectories by examining file paths
		for _, entry := range fs.index.ListFiles() {
			if strings.HasPrefix(entry.Directory, dirPath) {
				// Calculate relative path from current directory
				relDir := entry.Directory
				if dirPath != "" {
					if !strings.HasPrefix(relDir, dirPath+"/") {
						continue
					}
					relDir = strings.TrimPrefix(relDir, dirPath+"/")
				}
				
				// Get the first component of the relative directory
				if relDir != "" {
					parts := strings.Split(relDir, "/")
					if len(parts) > 0 && parts[0] != "" {
						subdirs[parts[0]] = true
					}
				}
			}
		}
		
		// Build directory entries
		entries := make([]fuse.DirEntry, 0, len(files)+len(subdirs))
		
		// Add subdirectories
		for subdir := range subdirs {
			entries = append(entries, fuse.DirEntry{
				Name: subdir,
				Mode: fuse.S_IFDIR,
			})
		}
		
		// Add files
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
	file := NewNoiseFile(fs.client, fs.storageManager, entry.DescriptorCID, relativePath, readOnly, fs.index)
	
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
	file := NewNoiseFile(fs.client, fs.storageManager, "", relativePath, false, fs.index)
	
	return file, fuse.OK
}

// Mkdir implements pathfs.FileSystem
func (fs *NoiseFS) Mkdir(name string, mode uint32, context *fuse.Context) fuse.Status {
	if fs.readOnly {
		return fuse.EROFS
	}
	
	// Only allow directories under files/
	if !strings.HasPrefix(name, "files/") {
		return fuse.EINVAL
	}
	
	// Check if directory already exists
	relativePath := strings.TrimPrefix(name, "files/")
	if fs.index.IsDirectory(relativePath) {
		return fuse.Status(17) // EEXIST
	}
	
	// For now, directories are created implicitly when files are added to them
	// No explicit directory creation needed in the index
	return fuse.OK
}

// Unlink implements pathfs.FileSystem
func (fs *NoiseFS) Unlink(name string, context *fuse.Context) fuse.Status {
	if fs.readOnly {
		return fuse.EROFS
	}
	
	// Only handle files under files/
	if !strings.HasPrefix(name, "files/") {
		return fuse.EINVAL
	}
	
	// Get relative path
	relativePath := strings.TrimPrefix(name, "files/")
	
	// Remove file from index
	if !fs.index.RemoveFile(relativePath) {
		return fuse.ENOENT
	}
	
	// Save index
	if err := fs.index.SaveIndex(); err != nil {
		return fuse.EIO
	}
	
	return fuse.OK
}

// Rmdir implements pathfs.FileSystem
func (fs *NoiseFS) Rmdir(name string, context *fuse.Context) fuse.Status {
	if fs.readOnly {
		return fuse.EROFS
	}
	
	// Only handle directories under files/
	if !strings.HasPrefix(name, "files/") || name == "files" {
		return fuse.EINVAL
	}
	
	// Get relative path
	relativePath := strings.TrimPrefix(name, "files/")
	
	// Check if directory exists
	if !fs.index.IsDirectory(relativePath) {
		return fuse.ENOENT
	}
	
	// Check if directory is empty
	files := fs.index.GetFilesInDirectory(relativePath)
	if len(files) > 0 {
		return fuse.Status(39) // ENOTEMPTY
	}
	
	// Check for subdirectories
	for _, entry := range fs.index.ListFiles() {
		if strings.HasPrefix(entry.Directory, relativePath+"/") {
			return fuse.Status(39) // ENOTEMPTY
		}
	}
	
	// Directory is empty, removal is implicit since we don't store empty directories
	return fuse.OK
}

// Rename implements pathfs.FileSystem
func (fs *NoiseFS) Rename(oldName string, newName string, context *fuse.Context) fuse.Status {
	if fs.readOnly {
		return fuse.EROFS
	}
	
	// Both paths must be under files/
	if !strings.HasPrefix(oldName, "files/") || !strings.HasPrefix(newName, "files/") {
		return fuse.EINVAL
	}
	
	// Get relative paths
	oldPath := strings.TrimPrefix(oldName, "files/")
	newPath := strings.TrimPrefix(newName, "files/")
	
	// Check if source exists
	entry, exists := fs.index.GetFile(oldPath)
	if !exists {
		return fuse.ENOENT
	}
	
	// Check if destination already exists
	if _, exists := fs.index.GetFile(newPath); exists {
		return fuse.Status(17) // EEXIST
	}
	
	// Remove old entry
	fs.index.RemoveFile(oldPath)
	
	// Add new entry
	fs.index.AddFile(newPath, entry.DescriptorCID, entry.FileSize)
	
	// Save index
	if err := fs.index.SaveIndex(); err != nil {
		return fuse.EIO
	}
	
	return fuse.OK
}

// GetXAttr implements pathfs.FileSystem for extended attributes
func (fs *NoiseFS) GetXAttr(name string, attribute string, context *fuse.Context) ([]byte, fuse.Status) {
	// Only handle files under files/
	if !strings.HasPrefix(name, "files/") {
		return nil, fuse.ENODATA
	}
	
	relativePath := strings.TrimPrefix(name, "files/")
	entry, exists := fs.index.GetFile(relativePath)
	if !exists {
		return nil, fuse.ENOENT
	}
	
	// Handle standard attributes
	switch attribute {
	case "user.noisefs.descriptor_cid":
		return []byte(entry.DescriptorCID), fuse.OK
	case "user.noisefs.created_at":
		return []byte(entry.CreatedAt.Format("2006-01-02T15:04:05Z07:00")), fuse.OK
	case "user.noisefs.modified_at":
		return []byte(entry.ModifiedAt.Format("2006-01-02T15:04:05Z07:00")), fuse.OK
	case "user.noisefs.file_size":
		return []byte(fmt.Sprintf("%d", entry.FileSize)), fuse.OK
	case "user.noisefs.directory":
		return []byte(entry.Directory), fuse.OK
	default:
		return nil, fuse.ENODATA
	}
}

// ListXAttr implements pathfs.FileSystem for listing extended attributes
func (fs *NoiseFS) ListXAttr(name string, context *fuse.Context) ([]string, fuse.Status) {
	// Only handle files under files/
	if !strings.HasPrefix(name, "files/") {
		return nil, fuse.ENODATA
	}
	
	relativePath := strings.TrimPrefix(name, "files/")
	_, exists := fs.index.GetFile(relativePath)
	if !exists {
		return nil, fuse.ENOENT
	}
	
	// Return list of available extended attributes
	attrs := []string{
		"user.noisefs.descriptor_cid",
		"user.noisefs.created_at",
		"user.noisefs.modified_at",
		"user.noisefs.file_size",
		"user.noisefs.directory",
	}
	
	return attrs, fuse.OK
}

// SetXAttr implements pathfs.FileSystem for setting extended attributes
func (fs *NoiseFS) SetXAttr(name string, attribute string, data []byte, flags int, context *fuse.Context) fuse.Status {
	if fs.readOnly {
		return fuse.EROFS
	}
	
	// Extended attributes are read-only for NoiseFS metadata
	// Only allow setting user-defined attributes that don't conflict with system ones
	if strings.HasPrefix(attribute, "user.noisefs.") {
		return fuse.EPERM
	}
	
	// For now, don't support arbitrary extended attributes
	return fuse.ENOTSUP
}

// RemoveXAttr implements pathfs.FileSystem for removing extended attributes
func (fs *NoiseFS) RemoveXAttr(name string, attribute string, context *fuse.Context) fuse.Status {
	if fs.readOnly {
		return fuse.EROFS
	}
	
	// System attributes cannot be removed
	if strings.HasPrefix(attribute, "user.noisefs.") {
		return fuse.EPERM
	}
	
	// For now, don't support arbitrary extended attributes
	return fuse.ENOTSUP
}

// Symlink implements pathfs.FileSystem for creating symbolic links
func (fs *NoiseFS) Symlink(value string, linkName string, context *fuse.Context) fuse.Status {
	if fs.readOnly {
		return fuse.EROFS
	}
	
	// Only allow symlinks under files/
	if !strings.HasPrefix(linkName, "files/") {
		return fuse.EINVAL
	}
	
	// For now, don't support symbolic links in NoiseFS
	// Symbolic links would require storing link targets in the index
	// and special handling during directory listing
	return fuse.ENOTSUP
}

// Readlink implements pathfs.FileSystem for reading symbolic links
func (fs *NoiseFS) Readlink(name string, context *fuse.Context) (string, fuse.Status) {
	// Only handle links under files/
	if !strings.HasPrefix(name, "files/") {
		return "", fuse.EINVAL
	}
	
	// For now, don't support symbolic links
	return "", fuse.ENOTSUP
}

// Link implements pathfs.FileSystem for creating hard links
func (fs *NoiseFS) Link(oldName string, newName string, context *fuse.Context) fuse.Status {
	if fs.readOnly {
		return fuse.EROFS
	}
	
	// Both paths must be under files/
	if !strings.HasPrefix(oldName, "files/") || !strings.HasPrefix(newName, "files/") {
		return fuse.EINVAL
	}
	
	// Get relative paths
	oldPath := strings.TrimPrefix(oldName, "files/")
	newPath := strings.TrimPrefix(newName, "files/")
	
	// Check if source exists
	entry, exists := fs.index.GetFile(oldPath)
	if !exists {
		return fuse.ENOENT
	}
	
	// Check if destination already exists
	if _, exists := fs.index.GetFile(newPath); exists {
		return fuse.Status(17) // EEXIST
	}
	
	// Create hard link by adding another index entry with same descriptor CID
	fs.index.AddFile(newPath, entry.DescriptorCID, entry.FileSize)
	
	// Save index
	if err := fs.index.SaveIndex(); err != nil {
		return fuse.EIO
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
func Daemon(client *noisefs.Client, storageManager *storage.Manager, opts MountOptions, pidFile string) error {
	return DaemonWithIndex(client, storageManager, opts, pidFile, "")
}

// DaemonWithIndex runs the FUSE filesystem as a background daemon with a custom index
func DaemonWithIndex(client *noisefs.Client, storageManager *storage.Manager, opts MountOptions, pidFile, indexPath string) error {
	if pidFile != "" {
		if err := writePIDFile(pidFile); err != nil {
			return fmt.Errorf("failed to write PID file: %w", err)
		}
		defer os.Remove(pidFile)
	}
	
	return MountWithIndex(client, storageManager, opts, indexPath)
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