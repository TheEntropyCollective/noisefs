# NoiseFS FUSE Integration

## Overview

The FUSE (Filesystem in Userspace) integration enables NoiseFS to be mounted as a regular filesystem, providing transparent access to anonymized storage through standard file operations. This allows any application to use NoiseFS without modification, making privacy-preserving storage accessible to all users.

## Current Implementation

### Architecture Overview

The FUSE integration uses the go-fuse library to bridge between the kernel VFS layer and NoiseFS. The architecture consists of multiple layers:

1. **User Applications**: Standard programs using regular file operations
2. **Kernel VFS Layer**: Linux/macOS virtual filesystem switch that routes operations
3. **FUSE Kernel Module**: Bridges kernel space and userspace filesystem implementations
4. **NoiseFS FUSE Daemon**: Our implementation that translates filesystem calls to NoiseFS operations
5. **NoiseFS Core API**: Handles block management and anonymization
6. **IPFS Storage Backend**: Provides the distributed storage layer

This layered approach allows any application to transparently access NoiseFS-stored files without modification.

### Core Components

The FUSE implementation consists of several key components:

- **FileSystem Base**: Implements the go-fuse pathfs.FileSystem interface
- **NoiseFS Client**: Handles core operations like block management and anonymization
- **IPFS Client**: Manages storage backend communication
- **Mount Configuration**: Tracks mount point and read-only status
- **File Index**: Maintains persistent metadata about stored files

## File Index System

### Index Structure

NoiseFS maintains a persistent index of files and their descriptors. The index tracks:

**Index Components**
- Version number for format compatibility
- Mapping of filenames to their metadata entries
- Path to the index file on disk
- Thread-safe access control

**Per-File Metadata**
- Full path relative to mount point
- Parent directory location
- IPFS CID of the file's descriptor
- File size in bytes
- Creation and modification timestamps

### Index Persistence

The index is stored as JSON and loaded/saved automatically. The persistence mechanism:

- Uses read locks to allow concurrent access during saves
- Formats JSON with indentation for human readability
- Saves with restricted permissions (0600) for security
- Handles errors gracefully to prevent data loss
- Automatically saves on changes and unmount

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

- **Open**: Opens existing files for reading with specified flags
- **Create**: Creates new files with given permissions and flags
- **Unlink**: Deletes files and removes them from the index
- **Rename**: Moves or renames files within the filesystem

#### Directory Operations

- **OpenDir**: Lists directory contents and returns entries
- **Mkdir**: Creates new directories with specified permissions
- **Rmdir**: Removes empty directories from the filesystem

#### Metadata Operations

- **GetAttr**: Retrieves file/directory attributes like size, permissions, timestamps
- **GetXAttr**: Accesses extended attributes for NoiseFS-specific metadata

### File Handles

Individual files are managed through NoiseFile handles that maintain:

- Base file interface implementation
- References to NoiseFS and IPFS clients
- The file's descriptor CID for content retrieval
- Filename and read-only status
- Cached content for performance
- Dirty flag to track modifications
- Reference to the global file index

## Mount Options

### Basic Mount Options

Mount options control filesystem behavior:

- **MountPath**: Directory where the filesystem will be mounted
- **VolumeName**: Display name shown in file managers
- **ReadOnly**: Prevents any write operations when enabled
- **AllowOther**: Allows users other than the mounter to access files
- **Debug**: Enables verbose logging for troubleshooting
- **Security**: Optional security manager for access control
- **IndexPassword**: Password for encrypting the file index

### Mounting Process

NoiseFS can be mounted with various configurations:

**Basic Mount**
- Specify mount path and volume name
- Uses default settings for simplicity
- Index stored unencrypted

**Secure Mount**
- Includes index password for encryption
- Protects file metadata when unmounted
- Requires password to access file list

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

The file index can be encrypted for additional security. The encrypted index includes:

- Standard file index functionality
- Derived encryption key from password
- Random salt for key derivation
- Lock status tracking

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