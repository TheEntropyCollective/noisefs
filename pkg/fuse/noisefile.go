// +build fuse

package fuse

import (
	"bytes"
	"fmt"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/core/descriptors"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	noisefs "github.com/TheEntropyCollective/noisefs/pkg/core/client"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/fuse/nodefs"
)

// NoiseFile implements nodefs.File for NoiseFS files
type NoiseFile struct {
	nodefs.File
	
	// NoiseFS components
	client         *noisefs.Client
	storageManager *storage.Manager
	descriptorCID  string
	descriptor     *descriptors.Descriptor
	
	// File metadata
	path     string
	readOnly bool
	
	// Content caching
	mu      sync.RWMutex
	content []byte
	loaded  bool
	
	// Write support
	writeBuffer []byte
	dirty       bool
	
	// Index management
	index *FileIndex
	
	// File locking
	lockType int32
	lockOwner uint64
}

// NewNoiseFile creates a new NoiseFS file handle
func NewNoiseFile(client *noisefs.Client, storageManager *storage.Manager, descriptorCID string, path string, readOnly bool, index *FileIndex) *NoiseFile {
	return &NoiseFile{
		File:           nodefs.NewDefaultFile(),
		client:         client,
		storageManager: storageManager,
		descriptorCID:  descriptorCID,
		path:           path,
		readOnly:       readOnly,
		index:          index,
	}
}

// loadDescriptor loads the file descriptor from storage
func (f *NoiseFile) loadDescriptor() error {
	if f.descriptor != nil {
		return nil
	}
	
	store, err := descriptors.NewStore(f.storageManager)
	if err != nil {
		return fmt.Errorf("failed to create descriptor store: %w", err)
	}
	
	descriptor, err := store.Load(f.descriptorCID)
	if err != nil {
		return fmt.Errorf("failed to load descriptor: %w", err)
	}
	
	f.descriptor = descriptor
	return nil
}

// downloadContent downloads and decrypts the file content
func (f *NoiseFile) downloadContent() ([]byte, error) {
	if err := f.loadDescriptor(); err != nil {
		return nil, err
	}
	
	// Retrieve all blocks
	dataBlocks := make([]*blocks.Block, len(f.descriptor.Blocks))
	randomizer1Blocks := make([]*blocks.Block, len(f.descriptor.Blocks))
	randomizer2Blocks := make([]*blocks.Block, len(f.descriptor.Blocks))
	
	for i, blockPair := range f.descriptor.Blocks {
		// Get data block
		dataBlock, err := f.storageManager.RetrieveBlock(blockPair.DataCID)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve data block %d: %w", i, err)
		}
		dataBlocks[i] = dataBlock
		
		// Get first randomizer block
		randomizer1Block, err := f.storageManager.RetrieveBlock(blockPair.RandomizerCID1)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve randomizer1 block %d: %w", i, err)
		}
		randomizer1Blocks[i] = randomizer1Block
		
		// Get second randomizer block (3-tuple format)
		randomizer2Block, err := f.storageManager.RetrieveBlock(blockPair.RandomizerCID2)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve randomizer2 block %d: %w", i, err)
		}
		randomizer2Blocks[i] = randomizer2Block
	}
	
	// XOR to reconstruct original blocks
	originalBlocks := make([]*blocks.Block, len(dataBlocks))
	for i := range dataBlocks {
		// Use 3-tuple XOR
		originalBlock, err := dataBlocks[i].XOR(randomizer1Blocks[i], randomizer2Blocks[i])
		if err != nil {
			return nil, fmt.Errorf("failed to XOR blocks: %w", err)
		}
		originalBlocks[i] = originalBlock
	}
	
	// Assemble file
	assembler := blocks.NewAssembler()
	data, err := assembler.Assemble(originalBlocks)
	if err != nil {
		return nil, fmt.Errorf("failed to assemble file: %w", err)
	}
	
	// Record download
	f.client.RecordDownload()
	
	return data, nil
}

// uploadFile uploads the write buffer to NoiseFS
func (f *NoiseFile) uploadFile() error {
	if f.writeBuffer == nil {
		return fmt.Errorf("no write buffer to upload")
	}
	
	// Create a reader from the write buffer
	reader := bytes.NewReader(f.writeBuffer)
	
	// Create splitter with default block size
	splitter, err := blocks.NewSplitter(blocks.DefaultBlockSize)
	if err != nil {
		return fmt.Errorf("failed to create splitter: %w", err)
	}
	
	// Split file into blocks
	fileBlocks, err := splitter.Split(reader)
	if err != nil {
		return fmt.Errorf("failed to split file: %w", err)
	}
	
	// Create descriptor
	descriptor := descriptors.NewDescriptor(
		f.path,
		int64(len(f.writeBuffer)),
		int64(len(f.writeBuffer)),
		blocks.DefaultBlockSize,
	)
	
	// Generate or select randomizer blocks (using 3-tuple format)
	randomizer1Blocks := make([]*blocks.Block, len(fileBlocks))
	randomizer1CIDs := make([]string, len(fileBlocks))
	randomizer2Blocks := make([]*blocks.Block, len(fileBlocks))
	randomizer2CIDs := make([]string, len(fileBlocks))
	
	for i := range fileBlocks {
		randBlock1, cid1, randBlock2, cid2, err := f.client.SelectRandomizers(fileBlocks[i].Size())
		if err != nil {
			return fmt.Errorf("failed to select randomizer blocks: %w", err)
		}
		randomizer1Blocks[i] = randBlock1
		randomizer1CIDs[i] = cid1
		randomizer2Blocks[i] = randBlock2
		randomizer2CIDs[i] = cid2
	}
	
	// XOR blocks with randomizers (3-tuple: data XOR randomizer1 XOR randomizer2)
	anonymizedBlocks := make([]*blocks.Block, len(fileBlocks))
	for i := range fileBlocks {
		xorBlock, err := fileBlocks[i].XOR(randomizer1Blocks[i], randomizer2Blocks[i])
		if err != nil {
			return fmt.Errorf("failed to XOR blocks: %w", err)
		}
		anonymizedBlocks[i] = xorBlock
	}
	
	// Store anonymized blocks in IPFS with caching
	dataCIDs := make([]string, len(anonymizedBlocks))
	for i, block := range anonymizedBlocks {
		cid, err := f.client.StoreBlockWithCache(block)
		if err != nil {
			return fmt.Errorf("failed to store data block %d: %w", i, err)
		}
		dataCIDs[i] = cid
	}
	
	// Add block triples to descriptor (3-tuple format)
	for i := range dataCIDs {
		if err := descriptor.AddBlockTriple(dataCIDs[i], randomizer1CIDs[i], randomizer2CIDs[i]); err != nil {
			return fmt.Errorf("failed to add block triple to descriptor: %w", err)
		}
	}
	
	// Store descriptor in storage
	store, err := descriptors.NewStore(f.storageManager)
	if err != nil {
		return fmt.Errorf("failed to create descriptor store: %w", err)
	}
	
	descriptorCID, err := store.Save(descriptor)
	if err != nil {
		return fmt.Errorf("failed to store descriptor: %w", err)
	}
	
	// Update descriptor CID and cache
	f.descriptorCID = descriptorCID
	f.descriptor = descriptor
	f.content = make([]byte, len(f.writeBuffer))
	copy(f.content, f.writeBuffer)
	
	// Record upload metrics
	totalStoredBytes := int64(0)
	for _, block := range anonymizedBlocks {
		totalStoredBytes += int64(len(block.Data))
	}
	f.client.RecordUpload(int64(len(f.writeBuffer)), totalStoredBytes*3)
	
	// Update index if available
	if f.index != nil {
		f.index.AddFile(f.path, descriptorCID, int64(len(f.writeBuffer)))
		f.index.SaveIndex()
	}
	
	return nil
}

// Read implements nodefs.File
func (f *NoiseFile) Read(buf []byte, off int64) (fuse.ReadResult, fuse.Status) {
	f.mu.Lock()
	defer f.mu.Unlock()
	
	// Load content if not already loaded
	if !f.loaded {
		if f.descriptorCID != "" {
			content, err := f.downloadContent()
			if err != nil {
				return nil, fuse.EIO
			}
			f.content = content
		} else {
			f.content = make([]byte, 0)
		}
		f.loaded = true
	}
	
	// Use write buffer if file has been modified
	var readFrom []byte
	if f.dirty && f.writeBuffer != nil {
		readFrom = f.writeBuffer
	} else {
		readFrom = f.content
	}
	
	// Handle offset beyond file size
	if off >= int64(len(readFrom)) {
		return fuse.ReadResultData([]byte{}), fuse.OK
	}
	
	// Calculate read range
	end := int(off) + len(buf)
	if end > len(readFrom) {
		end = len(readFrom)
	}
	
	// Return requested portion
	return fuse.ReadResultData(readFrom[off:end]), fuse.OK
}

// GetAttr implements nodefs.File
func (f *NoiseFile) GetAttr(out *fuse.Attr) fuse.Status {
	f.mu.RLock()
	defer f.mu.RUnlock()
	
	out.Mode = fuse.S_IFREG | 0644
	
	// Use write buffer size if dirty, otherwise use descriptor size
	if f.dirty && f.writeBuffer != nil {
		out.Size = uint64(len(f.writeBuffer))
	} else if f.descriptor != nil {
		out.Size = uint64(f.descriptor.FileSize)
	} else if f.descriptorCID != "" {
		// Load descriptor to get file size
		if err := f.loadDescriptor(); err != nil {
			return fuse.EIO
		}
		out.Size = uint64(f.descriptor.FileSize)
	} else {
		// New file
		out.Size = 0
	}
	
	// Set timestamps
	if f.descriptor != nil {
		out.Mtime = uint64(f.descriptor.CreatedAt.Unix())
		out.Atime = out.Mtime
		out.Ctime = out.Mtime
	} else {
		// Use current time for new files
		now := uint64(time.Now().Unix())
		out.Mtime = now
		out.Atime = now
		out.Ctime = now
	}
	
	return fuse.OK
}

// Write implements nodefs.File
func (f *NoiseFile) Write(data []byte, off int64) (written uint32, code fuse.Status) {
	if f.readOnly {
		return 0, fuse.EROFS
	}
	
	f.mu.Lock()
	defer f.mu.Unlock()
	
	// Ensure we have content loaded or initialize empty buffer
	if !f.loaded {
		if f.descriptorCID != "" {
			// Load existing file content first
			if f.content == nil {
				content, err := f.downloadContent()
				if err != nil {
					return 0, fuse.EIO
				}
				f.content = content
			}
			f.writeBuffer = make([]byte, len(f.content))
			copy(f.writeBuffer, f.content)
		} else {
			// New file - initialize empty buffer
			f.writeBuffer = make([]byte, 0)
		}
		f.loaded = true
	}
	
	// Calculate required buffer size
	requiredSize := int(off) + len(data)
	if requiredSize > len(f.writeBuffer) {
		// Expand buffer
		newBuffer := make([]byte, requiredSize)
		copy(newBuffer, f.writeBuffer)
		f.writeBuffer = newBuffer
	}
	
	// Write data to buffer
	copy(f.writeBuffer[off:], data)
	f.dirty = true
	
	return uint32(len(data)), fuse.OK
}

// Flush implements nodefs.File
func (f *NoiseFile) Flush() fuse.Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	
	// If not dirty, nothing to flush
	if !f.dirty || f.writeBuffer == nil {
		return fuse.OK
	}
	
	// Upload file to NoiseFS
	if err := f.uploadFile(); err != nil {
		return fuse.EIO
	}
	
	// Clear dirty flag
	f.dirty = false
	
	return fuse.OK
}

// Release implements nodefs.File
func (f *NoiseFile) Release() {
	f.mu.Lock()
	defer f.mu.Unlock()
	
	// Auto-flush dirty files on close
	if f.dirty && f.writeBuffer != nil {
		f.uploadFile()
		f.dirty = false
	}
	
	// Clear cached content to free memory
	f.content = nil
	f.writeBuffer = nil
	f.loaded = false
	f.lockType = 0
	f.lockOwner = 0
}

// Flock implements nodefs.File for file locking
func (f *NoiseFile) Flock(flags int) fuse.Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	
	// Simple locking implementation
	switch flags {
	case 1: // LOCK_SH (shared lock)
		if f.lockType == 2 { // Already exclusively locked
			return fuse.EAGAIN
		}
		f.lockType = 1
		return fuse.OK
	case 2: // LOCK_EX (exclusive lock)
		if f.lockType != 0 { // Already locked
			return fuse.EAGAIN
		}
		f.lockType = 2
		return fuse.OK
	case 8: // LOCK_UN (unlock)
		f.lockType = 0
		f.lockOwner = 0
		return fuse.OK
	default:
		return fuse.EINVAL
	}
}

// GetLk implements nodefs.File for lock testing
func (f *NoiseFile) GetLk(owner uint64, lk *fuse.FileLock, flags uint32, out *fuse.FileLock) (fuse.Status) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	
	// Check if there's a conflicting lock
	if f.lockType != 0 && f.lockOwner != owner {
		out.Typ = uint32(f.lockType)
		out.Pid = uint32(f.lockOwner)
		out.Start = lk.Start
		out.End = lk.End
		return fuse.OK
	}
	
	// No conflicting lock
	out.Typ = 3 // F_UNLCK
	return fuse.OK
}

// SetLk implements nodefs.File for setting locks
func (f *NoiseFile) SetLk(owner uint64, lk *fuse.FileLock, flags uint32) (fuse.Status) {
	f.mu.Lock()
	defer f.mu.Unlock()
	
	switch lk.Typ {
	case 0: // F_RDLCK (shared lock)
		if f.lockType == 2 && f.lockOwner != owner {
			return fuse.EAGAIN
		}
		f.lockType = 1
		f.lockOwner = owner
		return fuse.OK
	case 1: // F_WRLCK (exclusive lock)
		if f.lockType != 0 && f.lockOwner != owner {
			return fuse.EAGAIN
		}
		f.lockType = 2
		f.lockOwner = owner
		return fuse.OK
	case 2: // F_UNLCK (unlock)
		if f.lockOwner == owner {
			f.lockType = 0
			f.lockOwner = 0
		}
		return fuse.OK
	default:
		return fuse.EINVAL
	}
}