// +build fuse

package fuse

import (
	"fmt"
	"sync"

	"github.com/TheEntropyCollective/noisefs/pkg/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/descriptors"
	"github.com/TheEntropyCollective/noisefs/pkg/ipfs"
	"github.com/TheEntropyCollective/noisefs/pkg/noisefs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/fuse/nodefs"
)

// NoiseFile implements nodefs.File for NoiseFS files
type NoiseFile struct {
	nodefs.File
	
	// NoiseFS components
	client        *noisefs.Client
	ipfsClient    *ipfs.Client
	descriptorCID string
	descriptor    *descriptors.Descriptor
	
	// File metadata
	path     string
	readOnly bool
	
	// Content caching
	mu      sync.RWMutex
	content []byte
	loaded  bool
}

// NewNoiseFile creates a new NoiseFS file handle
func NewNoiseFile(client *noisefs.Client, ipfsClient *ipfs.Client, descriptorCID string, path string, readOnly bool) *NoiseFile {
	return &NoiseFile{
		File:          nodefs.NewDefaultFile(),
		client:        client,
		ipfsClient:    ipfsClient,
		descriptorCID: descriptorCID,
		path:          path,
		readOnly:      readOnly,
	}
}

// loadDescriptor loads the file descriptor from IPFS
func (f *NoiseFile) loadDescriptor() error {
	if f.descriptor != nil {
		return nil
	}
	
	store, err := descriptors.NewStore(f.ipfsClient)
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
		dataBlock, err := f.ipfsClient.RetrieveBlock(blockPair.DataCID)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve data block %d: %w", i, err)
		}
		dataBlocks[i] = dataBlock
		
		// Get first randomizer block
		randomizer1Block, err := f.ipfsClient.RetrieveBlock(blockPair.RandomizerCID1)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve randomizer1 block %d: %w", i, err)
		}
		randomizer1Blocks[i] = randomizer1Block
		
		// Get second randomizer block if using 3-tuple format
		if f.descriptor.IsThreeTuple() && blockPair.RandomizerCID2 != "" {
			randomizer2Block, err := f.ipfsClient.RetrieveBlock(blockPair.RandomizerCID2)
			if err != nil {
				return nil, fmt.Errorf("failed to retrieve randomizer2 block %d: %w", i, err)
			}
			randomizer2Blocks[i] = randomizer2Block
		}
	}
	
	// XOR to reconstruct original blocks
	originalBlocks := make([]*blocks.Block, len(dataBlocks))
	for i := range dataBlocks {
		var originalBlock *blocks.Block
		var err error
		
		if f.descriptor.IsThreeTuple() && randomizer2Blocks[i] != nil {
			// Use 3-tuple XOR
			originalBlock, err = dataBlocks[i].XOR3(randomizer1Blocks[i], randomizer2Blocks[i])
		} else {
			// Use 2-tuple XOR for legacy format
			originalBlock, err = dataBlocks[i].XOR(randomizer1Blocks[i])
		}
		
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

// Read implements nodefs.File
func (f *NoiseFile) Read(buf []byte, off int64) (fuse.ReadResult, fuse.Status) {
	f.mu.Lock()
	defer f.mu.Unlock()
	
	// Load content if not already loaded
	if !f.loaded {
		content, err := f.downloadContent()
		if err != nil {
			return nil, fuse.EIO
		}
		f.content = content
		f.loaded = true
	}
	
	// Handle offset beyond file size
	if off >= int64(len(f.content)) {
		return fuse.ReadResultData([]byte{}), fuse.OK
	}
	
	// Calculate read range
	end := int(off) + len(buf)
	if end > len(f.content) {
		end = len(f.content)
	}
	
	// Return requested portion
	return fuse.ReadResultData(f.content[off:end]), fuse.OK
}

// GetAttr implements nodefs.File
func (f *NoiseFile) GetAttr(out *fuse.Attr) fuse.Status {
	f.mu.RLock()
	defer f.mu.RUnlock()
	
	// Load descriptor to get file size
	if err := f.loadDescriptor(); err != nil {
		return fuse.EIO
	}
	
	out.Mode = fuse.S_IFREG | 0644
	out.Size = uint64(f.descriptor.FileSize)
	out.Mtime = uint64(f.descriptor.CreatedAt.Unix())
	out.Atime = out.Mtime
	out.Ctime = out.Mtime
	
	return fuse.OK
}

// Write implements nodefs.File (returns error for now)
func (f *NoiseFile) Write(data []byte, off int64) (written uint32, code fuse.Status) {
	if f.readOnly {
		return 0, fuse.EROFS
	}
	// TODO: Implement write support
	return 0, fuse.ENOSYS
}

// Flush implements nodefs.File
func (f *NoiseFile) Flush() fuse.Status {
	// TODO: Implement flush for write support
	return fuse.OK
}

// Release implements nodefs.File
func (f *NoiseFile) Release() {
	f.mu.Lock()
	defer f.mu.Unlock()
	
	// Clear cached content to free memory
	f.content = nil
	f.loaded = false
}