# NoiseFS FUSE Integration

## Overview

The FUSE (Filesystem in Userspace) integration enables NoiseFS to be mounted as a regular filesystem, providing transparent access to anonymized storage through standard file operations. This allows any application to use NoiseFS without modification, making privacy-preserving storage accessible to all users.

## Current Implementation

### Architecture Overview

The FUSE integration uses the go-fuse library to bridge between the kernel VFS layer and NoiseFS:

```
┌─────────────────────────────────────────────────────────────┐
│                    User Applications                         │
│              (Regular file operations)                       │
├─────────────────────────────────────────────────────────────┤
│                     Kernel VFS Layer                         │
│              (Virtual Filesystem Switch)                     │
├─────────────────────────────────────────────────────────────┤
│                      FUSE Kernel Module                      │
│                 (Kernel ↔ Userspace bridge)                  │
├─────────────────────────────────────────────────────────────┤
│                    NoiseFS FUSE Daemon                       │
│         (go-fuse pathfs.FileSystem implementation)           │
├─────────────────────────────────────────────────────────────┤
│                     NoiseFS Core API                         │
│          (Block management, anonymization)                   │
├─────────────────────────────────────────────────────────────┤
│                    IPFS Storage Backend                      │
│               (Distributed storage layer)                    │
└─────────────────────────────────────────────────────────────┘
```

### Core Components

The FUSE implementation consists of several key components:

```go
type NoiseFS struct {
    pathfs.FileSystem              // go-fuse base implementation
    client     *noisefs.Client     // NoiseFS client for operations
    ipfsClient *ipfs.Client        // IPFS client for storage
    mountPath  string              // Mount point path
    readOnly   bool                // Read-only mount flag
    index      *FileIndex          // Persistent file metadata
}
```

## File Index System

### Index Structure

NoiseFS maintains a persistent index of files and their descriptors:

```go
type FileIndex struct {
    Version  int                      // Index format version
    Files    map[string]*IndexEntry   // Filename -> metadata mapping
    filePath string                   // Path to index file
    mutex    sync.RWMutex            // Thread-safe access
}

type IndexEntry struct {
    Filename      string    // Full path relative to mount
    Directory     string    // Parent directory path
    DescriptorCID string    // IPFS CID of file descriptor
    FileSize      int64     // Size in bytes
    CreatedAt     time.Time // Creation timestamp
    ModifiedAt    time.Time // Last modification time
}
```

### Index Persistence

The index is stored as JSON and loaded/saved automatically:

```go
func (idx *FileIndex) SaveIndex() error {
    idx.mutex.RLock()
    defer idx.mutex.RUnlock()
    
    data, err := json.MarshalIndent(idx, "", "  ")
    if err != nil {
        return err
    }
    
    return os.WriteFile(idx.filePath, data, 0600)
}
```

## Filesystem Operations

### Directory Structure

NoiseFS presents a simple directory structure:
```
/mount-point/
└── files/
    ├── document.txt
    ├── images/
    │   ├── photo1.jpg
    │   └── photo2.png
    └── videos/
        └── movie.mp4
```

All user files are stored under the `files/` directory.

### Supported Operations

#### File Operations

```go
// Open - Open existing file for reading
func (fs *NoiseFS) Open(name string, flags uint32, context *fuse.Context) (nodefs.File, fuse.Status)

// Create - Create new file
func (fs *NoiseFS) Create(name string, flags uint32, mode uint32, context *fuse.Context) (nodefs.File, fuse.Status)

// Unlink - Delete file
func (fs *NoiseFS) Unlink(name string, context *fuse.Context) fuse.Status

// Rename - Rename/move file
func (fs *NoiseFS) Rename(oldName string, newName string, context *fuse.Context) fuse.Status
```

#### Directory Operations

```go
// OpenDir - List directory contents
func (fs *NoiseFS) OpenDir(name string, context *fuse.Context) ([]fuse.DirEntry, fuse.Status)

// Mkdir - Create directory
func (fs *NoiseFS) Mkdir(name string, mode uint32, context *fuse.Context) fuse.Status

// Rmdir - Remove empty directory
func (fs *NoiseFS) Rmdir(name string, context *fuse.Context) fuse.Status
```

#### Metadata Operations

```go
// GetAttr - Get file/directory attributes
func (fs *NoiseFS) GetAttr(name string, context *fuse.Context) (*fuse.Attr, fuse.Status)

// GetXAttr - Get extended attributes
func (fs *NoiseFS) GetXAttr(name string, attribute string, context *fuse.Context) ([]byte, fuse.Status)
```

### File Handles

Individual files are managed through NoiseFile handles:

```go
type NoiseFile struct {
    nodefs.File
    client        *noisefs.Client
    ipfsClient    *ipfs.Client
    descriptorCID string
    filename      string
    readOnly      bool
    content       []byte        // Cached file content
    isDirty       bool          // Modified flag
    index         *FileIndex    // Reference to global index
}
```

## Mount Options

### Basic Mount Options

```go
type MountOptions struct {
    MountPath      string   // Where to mount the filesystem
    VolumeName     string   // Display name for the volume
    ReadOnly       bool     // Mount as read-only
    AllowOther     bool     // Allow other users to access
    Debug          bool     // Enable debug logging
    Security       *security.SecurityManager
    IndexPassword  string   // Password for encrypted index
}
```

### Mounting Process

```go
// Basic mount
err := Mount(client, ipfsClient, MountOptions{
    MountPath:  "/mnt/noisefs",
    VolumeName: "NoiseFS",
})

// Mount with encrypted index
err := Mount(client, ipfsClient, MountOptions{
    MountPath:     "/mnt/noisefs",
    VolumeName:    "NoiseFS-Secure",
    IndexPassword: "my-secret-password",
})
```

## Extended Attributes

NoiseFS exposes metadata through extended attributes:

```bash
# View descriptor CID
xattr -p user.noisefs.descriptor_cid myfile.txt

# List all NoiseFS attributes
xattr -l myfile.txt
```

Available attributes:
- `user.noisefs.descriptor_cid` - File descriptor CID
- `user.noisefs.created_at` - Creation timestamp
- `user.noisefs.modified_at` - Modification timestamp
- `user.noisefs.file_size` - File size in bytes
- `user.noisefs.directory` - Parent directory path

## Security Features

### Encrypted Index

The file index can be encrypted for additional security:

```go
type EncryptedFileIndex struct {
    *FileIndex
    encryptionKey []byte
    salt          []byte
    locked        bool
}
```

Features:
- Password-based encryption using AES-256
- Secure key derivation with PBKDF2
- Memory locking to prevent swap
- Automatic cleanup on unmount

### Read-Only Mounts

Files can be mounted read-only to prevent accidental modifications:

```go
mount -o ro /mnt/noisefs
```

## Performance Optimizations

### Caching

NoiseFile implements content caching:
- Full file content cached on first read
- Write operations buffered until flush
- Dirty tracking for efficient writes

### Lazy Loading

- Directory entries loaded on demand
- File content fetched only when accessed
- Metadata cached in the index

## Limitations

Current implementation limitations:

1. **No Symbolic Links**: Symlinks not supported
2. **Limited Extended Attributes**: Only NoiseFS metadata exposed
3. **No Special Files**: No support for devices, sockets, or FIFOs
4. **Single Writer**: No concurrent write support
5. **Full File Operations**: No partial block updates

## Usage Examples

### Mounting

```bash
# Mount NoiseFS
noisefs-mount /mnt/noisefs

# Mount with options
noisefs-mount --read-only --volume-name "Secure Storage" /mnt/noisefs

# Mount with encrypted index
noisefs-mount --index-password /mnt/noisefs
```

### File Operations

```bash
# Copy file to NoiseFS
cp document.pdf /mnt/noisefs/files/

# Create directory structure
mkdir -p /mnt/noisefs/files/projects/2024

# List files
ls -la /mnt/noisefs/files/

# Read file
cat /mnt/noisefs/files/document.pdf > local-copy.pdf
```

### Unmounting

```bash
# Unmount (saves index automatically)
umount /mnt/noisefs

# Force unmount
umount -f /mnt/noisefs
```

## Future Enhancements

The following features are planned but not yet implemented:

### Performance Improvements
- Partial file updates (block-level writes)
- Parallel block retrieval
- Write-back caching with periodic sync
- Prefetching based on access patterns

### Feature Additions
- Symbolic link support
- Extended attribute storage
- File locking and concurrent access
- Quota management
- Snapshot support

### Integration Improvements
- Native OS integration (Finder/Explorer)
- Automatic mounting on startup
- System tray management UI
- Performance monitoring

## Testing

The FUSE implementation includes tests for:
- Basic file operations (create, read, write, delete)
- Directory operations
- Extended attributes
- Index persistence
- Encrypted index functionality
- Error handling

## Troubleshooting

Common issues and solutions:

1. **Mount fails**: Check FUSE kernel module is loaded
2. **Permission denied**: Use `--allow-other` for multi-user access
3. **Index corruption**: Delete index file to rebuild from descriptors
4. **Slow performance**: Ensure IPFS daemon is running locally
5. **Lost password**: No recovery for encrypted index

## Conclusion

The NoiseFS FUSE integration successfully provides transparent filesystem access to anonymized storage. While some advanced filesystem features are not yet implemented, the current system offers reliable file storage and retrieval with strong privacy guarantees. The persistent index ensures fast directory listings and metadata access, while the go-fuse library provides a stable foundation for filesystem operations.