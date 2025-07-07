// +build fuse

package fuse

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/TheEntropyCollective/noisefs/pkg/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/descriptors"
	"github.com/TheEntropyCollective/noisefs/pkg/noisefs"
)

// FileManager handles the integration between FUSE operations and NoiseFS
type FileManager struct {
	client *noisefs.Client
	
	// Descriptor cache for filesystem metadata
	mu          sync.RWMutex
	descriptors map[string]*descriptors.Descriptor // filename -> descriptor
	filePaths   map[string]string                   // full path -> filename
	
	// Upload queue for background processing
	uploadQueue chan *File
	uploadCtx   context.Context
	uploadStop  context.CancelFunc
}

// NewFileManager creates a new file manager for FUSE integration
func NewFileManager(client *noisefs.Client) *FileManager {
	ctx, cancel := context.WithCancel(context.Background())
	
	fm := &FileManager{
		client:      client,
		descriptors: make(map[string]*descriptors.Descriptor),
		filePaths:   make(map[string]string),
		uploadQueue: make(chan *File, 100), // Buffer up to 100 pending uploads
		uploadCtx:   ctx,
		uploadStop:  cancel,
	}
	
	// Start background upload worker
	go fm.uploadWorker()
	
	return fm
}

// Close shuts down the file manager
func (fm *FileManager) Close() {
	fm.uploadStop()
	close(fm.uploadQueue)
}

// uploadWorker processes files in the background for upload to NoiseFS
func (fm *FileManager) uploadWorker() {
	for {
		select {
		case file := <-fm.uploadQueue:
			if file != nil {
				fm.processUpload(file)
			}
		case <-fm.uploadCtx.Done():
			return
		}
	}
}

// processUpload handles the actual upload of a file to NoiseFS
func (fm *FileManager) processUpload(file *File) {
	file.mu.RLock()
	data := make([]byte, len(file.data))
	copy(data, file.data)
	filename := file.name
	file.mu.RUnlock()
	
	if len(data) == 0 {
		return // Nothing to upload
	}
	
	// Create descriptor for the file
	blockSize := 128 * 1024 // 128KB blocks
	desc := descriptors.NewDescriptor(filename, int64(len(data)), blockSize)
	
	// Split file into blocks and upload each one
	offset := 0
	for offset < len(data) {
		end := offset + blockSize
		if end > len(data) {
			end = len(data)
		}
		
		blockData := data[offset:end]
		
		// Upload block using NoiseFS client
		err := fm.uploadBlock(blockData, desc)
		if err != nil {
			// TODO: Add proper logging
			fmt.Printf("Failed to upload block for %s: %v\n", filename, err)
			return
		}
		
		offset = end
	}
	
	// Store descriptor
	fm.mu.Lock()
	fm.descriptors[filename] = desc
	fm.mu.Unlock()
	
	// Mark file as uploaded
	file.mu.Lock()
	file.uploaded = true
	file.mu.Unlock()
	
	// Record upload metrics
	fm.client.RecordUpload(int64(len(data)), int64(len(data)*2)) // Estimate 2x storage overhead
}

// uploadBlock uploads a single block to NoiseFS with anonymization
func (fm *FileManager) uploadBlock(blockData []byte, desc *descriptors.Descriptor) error {
	// Create data block
	dataBlock, err := blocks.NewBlock(blockData)
	if err != nil {
		return fmt.Errorf("failed to create data block: %w", err)
	}
	
	// Get randomizer block
	randBlock, randCID, err := fm.client.SelectRandomizer(len(blockData))
	if err != nil {
		return fmt.Errorf("failed to select randomizer: %w", err)
	}
	
	// XOR with randomizer to anonymize
	anonymizedBlock, err := dataBlock.XOR(randBlock)
	if err != nil {
		return fmt.Errorf("failed to anonymize block: %w", err)
	}
	
	// Store anonymized block
	dataCID, err := fm.client.StoreBlockWithCache(anonymizedBlock)
	if err != nil {
		return fmt.Errorf("failed to store anonymized block: %w", err)
	}
	
	// Add block pair to descriptor
	err = desc.AddBlockPair(dataCID, randCID)
	if err != nil {
		return fmt.Errorf("failed to add block pair to descriptor: %w", err)
	}
	
	return nil
}

// LoadFile loads a file from NoiseFS using its descriptor
func (fm *FileManager) LoadFile(filename string) (*File, error) {
	fm.mu.RLock()
	desc, exists := fm.descriptors[filename]
	fm.mu.RUnlock()
	
	if !exists {
		// Try to find descriptor file
		desc, err := fm.loadDescriptorFromFS(filename)
		if err != nil {
			return nil, fmt.Errorf("file not found: %s", filename)
		}
		
		fm.mu.Lock()
		fm.descriptors[filename] = desc
		fm.mu.Unlock()
	}
	
	// Download and reconstruct file
	data, err := fm.downloadFile(desc)
	if err != nil {
		return nil, fmt.Errorf("failed to download file: %w", err)
	}
	
	// Create file object
	file := &File{
		name:          filename,
		data:          data,
		size:          uint64(len(data)),
		uploaded:      true,
		descriptorCID: "", // Will be set when we have descriptor storage
		openHandles:   make(map[uint64]*FileHandle),
	}
	
	// Record download metrics
	fm.client.RecordDownload()
	
	return file, nil
}

// downloadFile downloads and reconstructs a file from NoiseFS
func (fm *FileManager) downloadFile(desc *descriptors.Descriptor) ([]byte, error) {
	var result bytes.Buffer
	
	for _, blockPair := range desc.Blocks {
		// Retrieve anonymized data block
		anonymizedBlock, err := fm.client.RetrieveBlockWithCache(blockPair.DataCID)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve data block %s: %w", blockPair.DataCID, err)
		}
		
		// Retrieve randomizer block
		randBlock, err := fm.client.RetrieveBlockWithCache(blockPair.RandomizerCID)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve randomizer block %s: %w", blockPair.RandomizerCID, err)
		}
		
		// XOR to recover original data
		originalBlock, err := anonymizedBlock.XOR(randBlock)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt block: %w", err)
		}
		
		// Append to result
		result.Write(originalBlock.Data)
	}
	
	return result.Bytes(), nil
}

// loadDescriptorFromFS attempts to load a descriptor from the filesystem
func (fm *FileManager) loadDescriptorFromFS(filename string) (*descriptors.Descriptor, error) {
	// This would typically load from .nfs files in the descriptors/ directory
	// For now, return an error since we haven't implemented descriptor persistence
	return nil, fmt.Errorf("descriptor file system not implemented yet")
}

// QueueUpload adds a file to the upload queue for background processing
func (fm *FileManager) QueueUpload(file *File) {
	select {
	case fm.uploadQueue <- file:
		// Queued successfully
	default:
		// Queue full, upload synchronously
		fm.processUpload(file)
	}
}

// ListFiles returns a list of files available in NoiseFS
func (fm *FileManager) ListFiles() []string {
	fm.mu.RLock()
	defer fm.mu.RUnlock()
	
	files := make([]string, 0, len(fm.descriptors))
	for filename := range fm.descriptors {
		files = append(files, filename)
	}
	
	return files
}

// GetFileInfo returns information about a file
func (fm *FileManager) GetFileInfo(filename string) (*FileInfo, error) {
	fm.mu.RLock()
	desc, exists := fm.descriptors[filename]
	fm.mu.RUnlock()
	
	if !exists {
		return nil, fmt.Errorf("file not found: %s", filename)
	}
	
	return &FileInfo{
		Name:      desc.Filename,
		Size:      desc.FileSize,
		BlockSize: desc.BlockSize,
		Blocks:    len(desc.Blocks),
		CreatedAt: desc.CreatedAt,
	}, nil
}

// DeleteFile removes a file from NoiseFS
func (fm *FileManager) DeleteFile(filename string) error {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	
	delete(fm.descriptors, filename)
	
	// TODO: Implement cleanup of blocks from IPFS
	// This is complex because blocks may be shared between files
	
	return nil
}

// RegisterFilePath maps a full filesystem path to a filename
func (fm *FileManager) RegisterFilePath(fullPath, filename string) {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	
	cleanPath := filepath.Clean(fullPath)
	fm.filePaths[cleanPath] = filename
}

// GetFilenameFromPath returns the NoiseFS filename for a filesystem path
func (fm *FileManager) GetFilenameFromPath(fullPath string) (string, bool) {
	fm.mu.RLock()
	defer fm.mu.RUnlock()
	
	cleanPath := filepath.Clean(fullPath)
	filename, exists := fm.filePaths[cleanPath]
	return filename, exists
}

// FileInfo contains information about a file in NoiseFS
type FileInfo struct {
	Name      string
	Size      int64
	BlockSize int
	Blocks    int
	CreatedAt interface{}
}

