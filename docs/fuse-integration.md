# NoiseFS FUSE Integration

## Overview

The FUSE (Filesystem in Userspace) integration enables NoiseFS to be mounted as a regular filesystem, providing transparent access to anonymized storage through standard file operations. This allows any application to use NoiseFS without modification, making privacy-preserving storage accessible to all users.

## Architecture Overview

### FUSE Bridge Design

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
│         (Filesystem operations implementation)               │
├─────────────────────────────────────────────────────────────┤
│                     NoiseFS Core API                         │
│          (Block management, anonymization)                   │
├─────────────────────────────────────────────────────────────┤
│                    IPFS Storage Backend                      │
│               (Distributed storage layer)                    │
└─────────────────────────────────────────────────────────────┘
```

### Core Components

```go
type NoiseFS struct {
    client      *noisefs.Client
    index       *FileIndex
    cache       *FileCache
    writeBuffer *WriteBuffer
    config      *FUSEConfig
    stats       *FilesystemStats
}

type FileIndex struct {
    root        *IndexNode
    nodes       map[uint64]*IndexNode
    encryption  *IndexEncryption
    persistence *IndexPersistence
    lock        sync.RWMutex
}

type IndexNode struct {
    Inode       uint64
    Name        string
    Type        FileType
    Descriptor  *descriptors.Descriptor
    Children    map[string]uint64
    Attributes  *FileAttributes
    Modified    time.Time
}
```

## Virtual Filesystem Design

### Inode Management

NoiseFS implements a virtual inode system for file identification:

```go
type InodeAllocator struct {
    nextInode   uint64
    recycled    []uint64
    reserved    map[uint64]bool
    mutex       sync.Mutex
}

func (a *InodeAllocator) Allocate() uint64 {
    a.mutex.Lock()
    defer a.mutex.Unlock()
    
    // Try to reuse recycled inodes first
    if len(a.recycled) > 0 {
        inode := a.recycled[len(a.recycled)-1]
        a.recycled = a.recycled[:len(a.recycled)-1]
        return inode
    }
    
    // Allocate new inode
    inode := a.nextInode
    a.nextInode++
    
    // Skip reserved inodes
    for a.reserved[inode] {
        inode = a.nextInode
        a.nextInode++
    }
    
    return inode
}

const (
    RootInode = 1  // Root directory always inode 1
    // Reserved inodes for special files
    ConfigInode = 2
    StatsInode  = 3
)
```

### Directory Structure

Efficient directory operations with encrypted metadata:

```go
type Directory struct {
    inode    uint64
    entries  map[string]*DirEntry
    version  uint64  // For detecting concurrent modifications
    lock     sync.RWMutex
}

type DirEntry struct {
    Name       string
    Inode      uint64
    Type       fuse.DirentType
    Descriptor string  // CID of file descriptor
}

func (d *Directory) Lookup(name string) (*DirEntry, error) {
    d.lock.RLock()
    defer d.lock.RUnlock()
    
    entry, exists := d.entries[name]
    if !exists {
        return nil, fuse.ENOENT
    }
    
    return entry, nil
}

func (d *Directory) AddEntry(name string, inode uint64, dtype fuse.DirentType) error {
    d.lock.Lock()
    defer d.lock.Unlock()
    
    if _, exists := d.entries[name]; exists {
        return fuse.EEXIST
    }
    
    d.entries[name] = &DirEntry{
        Name:  name,
        Inode: inode,
        Type:  dtype,
    }
    
    d.version++
    return nil
}
```

## Index Management and Encryption

### Encrypted Index Storage

The filesystem index is encrypted and stored in IPFS:

```go
type IndexEncryption struct {
    masterKey   []byte
    cipher      cipher.AEAD
    keyDeriver  *KeyDeriver
}

type EncryptedIndex struct {
    Version     uint32
    Nonce       []byte
    Ciphertext  []byte
    MAC         []byte
    Salt        []byte
}

func (e *IndexEncryption) EncryptIndex(index *FileIndex) (*EncryptedIndex, error) {
    // Serialize index
    plaintext, err := index.Serialize()
    if err != nil {
        return nil, err
    }
    
    // Generate nonce
    nonce := make([]byte, e.cipher.NonceSize())
    if _, err := rand.Read(nonce); err != nil {
        return nil, err
    }
    
    // Encrypt with AEAD
    ciphertext := e.cipher.Seal(nil, nonce, plaintext, nil)
    
    return &EncryptedIndex{
        Version:    CurrentIndexVersion,
        Nonce:      nonce,
        Ciphertext: ciphertext,
    }, nil
}
```

### Index Persistence

Automatic index snapshots and recovery:

```go
type IndexPersistence struct {
    storage     StorageBackend
    snapshots   *SnapshotManager
    journal     *WriteJournal
}

func (p *IndexPersistence) SaveIndex(index *FileIndex) error {
    // Create snapshot
    snapshot := &IndexSnapshot{
        Timestamp: time.Now(),
        Version:   index.GetVersion(),
        Data:      index.Clone(),
    }
    
    // Encrypt snapshot
    encrypted, err := p.encryptSnapshot(snapshot)
    if err != nil {
        return err
    }
    
    // Store in IPFS
    cid, err := p.storage.Store(encrypted)
    if err != nil {
        return err
    }
    
    // Update snapshot chain
    p.snapshots.AddSnapshot(cid, snapshot.Timestamp)
    
    // Clear journal after successful snapshot
    p.journal.Clear()
    
    return nil
}

func (p *IndexPersistence) RecoverIndex() (*FileIndex, error) {
    // Get latest snapshot
    snapshotCID := p.snapshots.GetLatest()
    if snapshotCID == "" {
        return NewFileIndex(), nil
    }
    
    // Retrieve and decrypt
    encrypted, err := p.storage.Retrieve(snapshotCID)
    if err != nil {
        return nil, err
    }
    
    snapshot, err := p.decryptSnapshot(encrypted)
    if err != nil {
        return nil, err
    }
    
    // Apply journal entries
    index := snapshot.Data
    if err := p.journal.ApplyTo(index); err != nil {
        return nil, err
    }
    
    return index, nil
}
```

## Write Operations

### Write-Through vs Write-Back

NoiseFS supports both caching strategies:

```go
type WriteStrategy interface {
    Write(path string, data []byte, offset int64) (int, error)
    Flush(path string) error
    Sync(path string) error
}

type WriteThroughStrategy struct {
    client *noisefs.Client
}

func (w *WriteThroughStrategy) Write(
    path string, 
    data []byte, 
    offset int64,
) (int, error) {
    // Immediately write to backend
    descriptor, err := w.client.Upload(data)
    if err != nil {
        return 0, err
    }
    
    // Update index
    w.updateFileDescriptor(path, descriptor)
    
    return len(data), nil
}

type WriteBackStrategy struct {
    buffer      *WriteBuffer
    flushDelay  time.Duration
    client      *noisefs.Client
}

func (w *WriteBackStrategy) Write(
    path string, 
    data []byte, 
    offset int64,
) (int, error) {
    // Buffer the write
    w.buffer.AddWrite(path, offset, data)
    
    // Schedule flush
    w.scheduleFlush(path)
    
    return len(data), nil
}

func (w *WriteBackStrategy) Flush(path string) error {
    writes := w.buffer.GetWrites(path)
    if len(writes) == 0 {
        return nil
    }
    
    // Coalesce writes
    data := w.coalesceWrites(writes)
    
    // Upload to backend
    descriptor, err := w.client.Upload(data)
    if err != nil {
        return err
    }
    
    // Update index and clear buffer
    w.updateFileDescriptor(path, descriptor)
    w.buffer.Clear(path)
    
    return nil
}
```

### Atomic Operations

Ensuring consistency during concurrent access:

```go
type AtomicFileOps struct {
    locks   *LockManager
    journal *OperationJournal
}

func (a *AtomicFileOps) Rename(oldPath, newPath string) error {
    // Acquire locks in consistent order to prevent deadlock
    locks := a.locks.AcquireInOrder([]string{oldPath, newPath})
    defer locks.Release()
    
    // Record operation in journal
    op := &RenameOp{
        OldPath: oldPath,
        NewPath: newPath,
        Time:    time.Now(),
    }
    a.journal.Record(op)
    
    // Perform rename
    if err := a.performRename(oldPath, newPath); err != nil {
        a.journal.Rollback(op)
        return err
    }
    
    // Commit journal entry
    a.journal.Commit(op)
    
    return nil
}
```

## POSIX Compatibility

### File Attributes

Full POSIX attribute support:

```go
type FileAttributes struct {
    Inode       uint64
    Size        uint64
    Blocks      uint64
    Mode        os.FileMode
    Nlink       uint32
    UID         uint32
    GID         uint32
    Rdev        uint32
    Atime       time.Time
    Mtime       time.Time
    Ctime       time.Time
    Crtime      time.Time  // Creation time
    BlockSize   uint32
}

func (fs *NoiseFS) GetAttr(
    ctx context.Context, 
    req *fuse.GetattrRequest,
) (*fuse.GetattrResponse, error) {
    node := fs.index.GetNode(req.Inode)
    if node == nil {
        return nil, fuse.ENOENT
    }
    
    resp := &fuse.GetattrResponse{
        Attr: fuse.Attr{
            Inode:  node.Attributes.Inode,
            Size:   node.Attributes.Size,
            Blocks: node.Attributes.Blocks,
            Mode:   node.Attributes.Mode,
            Nlink:  node.Attributes.Nlink,
            Uid:    node.Attributes.UID,
            Gid:    node.Attributes.GID,
            Atime:  node.Attributes.Atime,
            Mtime:  node.Attributes.Mtime,
            Ctime:  node.Attributes.Ctime,
            Crtime: node.Attributes.Crtime,
        },
    }
    
    return resp, nil
}
```

### Extended Attributes

Support for xattr operations:

```go
type ExtendedAttributes struct {
    attrs map[string][]byte
    lock  sync.RWMutex
}

func (x *ExtendedAttributes) Get(name string) ([]byte, error) {
    x.lock.RLock()
    defer x.lock.RUnlock()
    
    value, exists := x.attrs[name]
    if !exists {
        return nil, fuse.ErrNoXattr
    }
    
    return value, nil
}

func (x *ExtendedAttributes) Set(name string, value []byte) error {
    x.lock.Lock()
    defer x.lock.Unlock()
    
    // Validate xattr name
    if !isValidXattrName(name) {
        return fuse.EINVAL
    }
    
    // Check size limits
    if len(value) > MaxXattrSize {
        return fuse.E2BIG
    }
    
    x.attrs[name] = value
    return nil
}
```

## Performance Optimizations

### Read-Ahead Caching

Predictive caching for sequential reads:

```go
type ReadAheadCache struct {
    cache       *BlockCache
    predictor   *SequentialPredictor
    prefetcher  *Prefetcher
}

func (r *ReadAheadCache) Read(
    ctx context.Context,
    inode uint64,
    offset int64,
    size int,
) ([]byte, error) {
    // Serve current read
    data, err := r.readBlocks(inode, offset, size)
    if err != nil {
        return nil, err
    }
    
    // Predict and prefetch next blocks
    if r.predictor.IsSequential(inode, offset) {
        nextOffset := offset + int64(size)
        nextSize := r.predictor.PredictReadSize(inode)
        
        go r.prefetcher.Prefetch(inode, nextOffset, nextSize)
    }
    
    return data, nil
}
```

### Directory Entry Caching

Efficient directory listing:

```go
type DirCache struct {
    entries     map[uint64][]*fuse.Dirent
    versions    map[uint64]uint64
    ttl         time.Duration
    lastAccess  map[uint64]time.Time
}

func (d *DirCache) ReadDir(
    ctx context.Context,
    inode uint64,
) ([]*fuse.Dirent, error) {
    // Check cache
    if entries, ok := d.getCached(inode); ok {
        return entries, nil
    }
    
    // Load from index
    node := d.index.GetNode(inode)
    if node == nil || node.Type != DirectoryType {
        return nil, fuse.ENOTDIR
    }
    
    entries := make([]*fuse.Dirent, 0, len(node.Children))
    
    for name, childInode := range node.Children {
        child := d.index.GetNode(childInode)
        if child == nil {
            continue
        }
        
        entries = append(entries, &fuse.Dirent{
            Inode: childInode,
            Type:  child.GetDirentType(),
            Name:  name,
        })
    }
    
    // Cache results
    d.cache(inode, entries)
    
    return entries, nil
}
```

### Parallel Operations

Concurrent handling of independent operations:

```go
type ParallelHandler struct {
    workers     int
    queue       chan Operation
    results     chan Result
}

func (h *ParallelHandler) HandleOperations(ops []Operation) []Result {
    // Start workers
    for i := 0; i < h.workers; i++ {
        go h.worker()
    }
    
    // Submit operations
    go func() {
        for _, op := range ops {
            h.queue <- op
        }
        close(h.queue)
    }()
    
    // Collect results
    results := make([]Result, 0, len(ops))
    for range ops {
        result := <-h.results
        results = append(results, result)
    }
    
    return results
}
```

## Security Considerations

### Access Control

File permission enforcement:

```go
type AccessController struct {
    defaultUID  uint32
    defaultGID  uint32
    umask       uint32
}

func (a *AccessController) CheckAccess(
    ctx context.Context,
    node *IndexNode,
    mask uint32,
) error {
    // Get request credentials
    creds := fuse.CredentialsFromContext(ctx)
    
    // Root always has access
    if creds.Uid == 0 {
        return nil
    }
    
    // Check owner
    if creds.Uid == node.Attributes.UID {
        if a.checkOwnerAccess(node.Attributes.Mode, mask) {
            return nil
        }
    }
    
    // Check group
    if creds.Gid == node.Attributes.GID {
        if a.checkGroupAccess(node.Attributes.Mode, mask) {
            return nil
        }
    }
    
    // Check others
    if a.checkOtherAccess(node.Attributes.Mode, mask) {
        return nil
    }
    
    return fuse.EACCES
}
```

### Secure Mount Options

```go
type MountOptions struct {
    // Security options
    AllowOther    bool   // Allow access by other users
    AllowRoot     bool   // Allow access by root
    DefaultPerms  bool   // Enable permission checking
    Umask         uint32 // Default umask for new files
    
    // Performance options
    AsyncRead     bool   // Enable async reads
    MaxBackground int    // Max background requests
    CongestionThreshold int
    
    // Privacy options
    NoAccessTime  bool   // Don't update access times
    NoDevices     bool   // Don't allow device files
}
```

## Integration Examples

### Basic Mount

```go
func MountNoiseFS(mountpoint string, config *Config) error {
    // Initialize NoiseFS client
    client, err := noisefs.NewClient(config.IPFSEndpoint)
    if err != nil {
        return err
    }
    
    // Create FUSE filesystem
    fs := &NoiseFS{
        client: client,
        index:  NewFileIndex(),
        cache:  NewFileCache(config.CacheSize),
        config: config.FUSE,
    }
    
    // Mount options
    options := []fuse.MountOption{
        fuse.FSName("noisefs"),
        fuse.Subtype("noisefs"),
        fuse.LocalVolume(),
        fuse.VolumeName("NoiseFS"),
        fuse.AllowOther(),
        fuse.DefaultPermissions(),
    }
    
    // Mount filesystem
    conn, err := fuse.Mount(mountpoint, options...)
    if err != nil {
        return err
    }
    defer conn.Close()
    
    // Serve requests
    if err := fs.Serve(conn); err != nil {
        return err
    }
    
    return nil
}
```

### Advanced Configuration

```go
func ConfigureAdvancedMount(config *AdvancedConfig) *NoiseFS {
    fs := &NoiseFS{
        client: config.Client,
        index:  config.Index,
    }
    
    // Configure caching
    fs.cache = &FileCache{
        BlockCache:    NewAdaptiveCache(config.CacheSize),
        WriteStrategy: config.WriteStrategy,
        ReadAhead:     config.EnableReadAhead,
    }
    
    // Configure security
    fs.access = &AccessController{
        DefaultUID: config.DefaultUID,
        DefaultGID: config.DefaultGID,
        Umask:      config.Umask,
    }
    
    // Configure performance
    fs.parallel = &ParallelHandler{
        Workers: config.Workers,
    }
    
    return fs
}
```

## Future Improvements

### Planned Features

1. **NFS/SMB Export**: Network filesystem protocols
2. **Quota Management**: User and group quotas
3. **Snapshot Support**: Point-in-time snapshots
4. **Compression**: Transparent compression
5. **Deduplication**: Block-level dedup

### Research Directions

1. **Distributed Locking**: Cluster-wide file locking
2. **Transactional Semantics**: ACID file operations
3. **Query Interface**: File search and indexing
4. **Version Control**: Git-like file versioning

## Conclusion

The FUSE integration makes NoiseFS accessible as a standard filesystem while maintaining all privacy and anonymity guarantees. Through careful implementation of POSIX semantics, efficient caching strategies, and robust index management, it provides a seamless experience for users and applications. The encrypted index ensures metadata privacy, while optimizations like read-ahead caching and parallel operations deliver excellent performance for real-world workloads.