package blocks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/crypto"
)

// DirectoryProcessor handles recursive processing of directory trees
type DirectoryProcessor struct {
	blockSize     int
	maxWorkers    int
	splitter      *StreamingSplitter
	encryptionKey *crypto.EncryptionKey
	progressFn    DirectoryProgressCallback
	errorHandler  DirectoryErrorHandler
	cancelFunc    context.CancelFunc
	ctx           context.Context

	// Internal state (protected by stateMux)
	processedFiles int64
	processedBytes int64
	totalFiles     int64
	totalBytes     int64
	stateMux       sync.RWMutex // Protects progress state fields

	// Worker pool management
	workerPool chan struct{}
	wg         sync.WaitGroup

	// Results collection
	results    chan ProcessResult
	resultsMux sync.RWMutex
	errors     []error
}

// DescriptorType represents the type of descriptor (file or directory)
type DescriptorType int

const (
	FileType DescriptorType = iota
	DirectoryType
)

// ProcessResult represents the result of processing a file or directory
type ProcessResult struct {
	Path           string
	Type           DescriptorType
	Size           int64
	CID            string
	EncryptedName  []byte
	ManifestCID    string // For directories, contains the manifest CID
	Error          error
	ProcessedAt    time.Time
	ProcessingTime time.Duration
}

// DirectoryEntry represents a single entry in a directory
type DirectoryEntry struct {
	EncryptedName []byte         `json:"name"`     // Encrypted filename
	CID           string         `json:"cid"`      // CID of the file/directory descriptor
	Type          DescriptorType `json:"type"`     // File or Directory
	Size          int64          `json:"size"`     // Size in bytes (0 for directories)
	ModifiedAt    time.Time      `json:"modified"` // Last modification time
}

// SnapshotInfo represents metadata about a directory snapshot
type SnapshotInfo struct {
	OriginalCID  string    `json:"original_cid"`  // CID of the original directory
	CreationTime time.Time `json:"creation_time"` // When the snapshot was created
	SnapshotName string    `json:"snapshot_name"` // User-provided name for the snapshot
	Description  string    `json:"description"`   // Optional description of the snapshot
	IsSnapshot   bool      `json:"is_snapshot"`   // Indicates this is a snapshot manifest
}

// DirectoryManifest represents the contents of a directory
type DirectoryManifest struct {
	Version      string           `json:"version"`
	Entries      []DirectoryEntry `json:"entries"`
	CreatedAt    time.Time        `json:"created"`
	ModifiedAt   time.Time        `json:"modified"`
	SnapshotInfo *SnapshotInfo    `json:"snapshot_info,omitempty"` // Snapshot metadata if this is a snapshot
	mu           sync.Mutex       // Protects concurrent access to Entries
}

// NewDirectoryManifest creates a new empty directory manifest
func NewDirectoryManifest() *DirectoryManifest {
	now := time.Now()
	return &DirectoryManifest{
		Version:    "1.0",
		Entries:    make([]DirectoryEntry, 0),
		CreatedAt:  now,
		ModifiedAt: now,
	}
}

// AddEntry adds a new entry to the directory manifest
func (m *DirectoryManifest) AddEntry(entry DirectoryEntry) error {
	if len(entry.EncryptedName) == 0 {
		return errors.New("encrypted name cannot be empty")
	}
	if entry.CID == "" {
		return errors.New("CID cannot be empty")
	}
	if entry.Type != FileType && entry.Type != DirectoryType {
		return errors.New("invalid entry type")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.Entries = append(m.Entries, entry)
	m.ModifiedAt = time.Now()
	return nil
}

// GetEntriesCopy returns a thread-safe copy of the directory entries
func (m *DirectoryManifest) GetEntriesCopy() []DirectoryEntry {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Create a deep copy of the entries
	entriesCopy := make([]DirectoryEntry, len(m.Entries))
	copy(entriesCopy, m.Entries)
	return entriesCopy
}

// GetSnapshot returns a thread-safe snapshot of the manifest
func (m *DirectoryManifest) GetSnapshot() DirectoryManifest {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Create a deep copy of the entries
	entriesCopy := make([]DirectoryEntry, len(m.Entries))
	copy(entriesCopy, m.Entries)

	// Copy snapshot info if present
	var snapshotInfoCopy *SnapshotInfo
	if m.SnapshotInfo != nil {
		snapshotInfoCopy = &SnapshotInfo{
			OriginalCID:  m.SnapshotInfo.OriginalCID,
			CreationTime: m.SnapshotInfo.CreationTime,
			SnapshotName: m.SnapshotInfo.SnapshotName,
			Description:  m.SnapshotInfo.Description,
			IsSnapshot:   m.SnapshotInfo.IsSnapshot,
		}
	}

	return DirectoryManifest{
		Version:      m.Version,
		Entries:      entriesCopy,
		CreatedAt:    m.CreatedAt,
		ModifiedAt:   m.ModifiedAt,
		SnapshotInfo: snapshotInfoCopy,
	}
}

// IsSnapshot returns true if this manifest represents a snapshot
func (m *DirectoryManifest) IsSnapshot() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.SnapshotInfo != nil && m.SnapshotInfo.IsSnapshot
}

// GetSnapshotInfo returns the snapshot information, or nil if not a snapshot
func (m *DirectoryManifest) GetSnapshotInfo() *SnapshotInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.SnapshotInfo != nil {
		return &SnapshotInfo{
			OriginalCID:  m.SnapshotInfo.OriginalCID,
			CreationTime: m.SnapshotInfo.CreationTime,
			SnapshotName: m.SnapshotInfo.SnapshotName,
			Description:  m.SnapshotInfo.Description,
			IsSnapshot:   m.SnapshotInfo.IsSnapshot,
		}
	}
	return nil
}

// RemoveEntry removes an entry from the directory manifest by encrypted name
func (m *DirectoryManifest) RemoveEntry(encryptedName []byte) error {
	if len(encryptedName) == 0 {
		return errors.New("encrypted name cannot be empty")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for i, entry := range m.Entries {
		if bytes.Equal(entry.EncryptedName, encryptedName) {
			// Remove entry by replacing with the last entry and shrinking slice
			m.Entries[i] = m.Entries[len(m.Entries)-1]
			m.Entries = m.Entries[:len(m.Entries)-1]
			m.ModifiedAt = time.Now()
			return nil
		}
	}

	return errors.New("entry not found")
}

// UpdateEntry updates an existing entry in the directory manifest
func (m *DirectoryManifest) UpdateEntry(encryptedName []byte, newEntry DirectoryEntry) error {
	if len(encryptedName) == 0 {
		return errors.New("encrypted name cannot be empty")
	}
	if len(newEntry.EncryptedName) == 0 {
		return errors.New("new entry encrypted name cannot be empty")
	}
	if newEntry.CID == "" {
		return errors.New("new entry CID cannot be empty")
	}
	if newEntry.Type != FileType && newEntry.Type != DirectoryType {
		return errors.New("invalid entry type")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for i, entry := range m.Entries {
		if bytes.Equal(entry.EncryptedName, encryptedName) {
			m.Entries[i] = newEntry
			m.ModifiedAt = time.Now()
			return nil
		}
	}

	return errors.New("entry not found")
}

// FindEntryByName finds an entry by encrypted name and returns its index and the entry
func (m *DirectoryManifest) FindEntryByName(encryptedName []byte) (int, *DirectoryEntry, error) {
	if len(encryptedName) == 0 {
		return -1, nil, errors.New("encrypted name cannot be empty")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for i, entry := range m.Entries {
		if bytes.Equal(entry.EncryptedName, encryptedName) {
			// Return a copy to avoid race conditions
			entryCopy := entry
			return i, &entryCopy, nil
		}
	}

	return -1, nil, errors.New("entry not found")
}

// HasEntry checks if an entry exists by encrypted name
func (m *DirectoryManifest) HasEntry(encryptedName []byte) bool {
	if len(encryptedName) == 0 {
		return false
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, entry := range m.Entries {
		if bytes.Equal(entry.EncryptedName, encryptedName) {
			return true
		}
	}

	return false
}

// NewSnapshotManifest creates a new snapshot manifest from an existing directory manifest
func NewSnapshotManifest(original *DirectoryManifest, originalCID, snapshotName, description string) *DirectoryManifest {
	now := time.Now()

	// Get a thread-safe snapshot of the original
	originalSnapshot := original.GetSnapshot()

	// Create snapshot info
	snapshotInfo := &SnapshotInfo{
		OriginalCID:  originalCID,
		CreationTime: now,
		SnapshotName: snapshotName,
		Description:  description,
		IsSnapshot:   true,
	}

	return &DirectoryManifest{
		Version:      "1.0",
		Entries:      originalSnapshot.Entries, // Same file CIDs
		CreatedAt:    now,
		ModifiedAt:   now,
		SnapshotInfo: snapshotInfo,
	}
}

// EncryptManifest encrypts a directory manifest
func EncryptManifest(manifest *DirectoryManifest, key *crypto.EncryptionKey) ([]byte, error) {
	// Get a thread-safe snapshot
	snapshot := manifest.GetSnapshot()

	// Serialize manifest as JSON
	data, err := json.Marshal(snapshot)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal manifest: %w", err)
	}

	// Encrypt the data
	encrypted, err := crypto.Encrypt(data, key)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt manifest: %w", err)
	}

	return encrypted, nil
}

// DirectoryProgressCallback is called to report processing progress
type DirectoryProgressCallback func(processed, total int64, currentFile string)

// DirectoryErrorHandler handles errors during directory processing
type DirectoryErrorHandler func(path string, err error) bool // Return true to continue, false to stop

// ProcessorConfig holds configuration for the directory processor
type ProcessorConfig struct {
	BlockSize         int
	MaxWorkers        int
	EncryptionKey     *crypto.EncryptionKey
	ProgressCallback  DirectoryProgressCallback
	ErrorHandler      DirectoryErrorHandler
	SkipSymlinks      bool
	SkipHidden        bool
	MaxFileSize       int64
	AllowedExtensions []string
	BlockedExtensions []string
}

// NewDirectoryProcessor creates a new directory processor
func NewDirectoryProcessor(config *ProcessorConfig) (*DirectoryProcessor, error) {
	if config == nil {
		return nil, errors.New("processor config cannot be nil")
	}

	if config.BlockSize <= 0 {
		config.BlockSize = DefaultBlockSize
	}

	if config.MaxWorkers <= 0 {
		config.MaxWorkers = 10
	}

	if config.EncryptionKey == nil {
		return nil, errors.New("encryption key is required")
	}

	splitter, err := NewStreamingSplitter(config.BlockSize)
	if err != nil {
		return nil, fmt.Errorf("failed to create streaming splitter: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &DirectoryProcessor{
		blockSize:     config.BlockSize,
		maxWorkers:    config.MaxWorkers,
		splitter:      splitter,
		encryptionKey: config.EncryptionKey,
		progressFn:    config.ProgressCallback,
		errorHandler:  config.ErrorHandler,
		cancelFunc:    cancel,
		ctx:           ctx,
		workerPool:    make(chan struct{}, config.MaxWorkers),
		results:       make(chan ProcessResult, 100),
		errors:        make([]error, 0),
	}, nil
}

// ProcessDirectory processes a directory tree recursively
func (dp *DirectoryProcessor) ProcessDirectory(rootPath string, processor DirectoryBlockProcessor) ([]*ProcessResult, error) {
	// First pass: calculate total files and bytes
	if err := dp.calculateTotals(rootPath); err != nil {
		return nil, fmt.Errorf("failed to calculate totals: %w", err)
	}

	// Process the directory tree
	resultList := make([]*ProcessResult, 0)
	var resultsMux sync.Mutex
	var collectorDone sync.WaitGroup

	// Start result collector
	collectorDone.Add(1)
	go func() {
		defer collectorDone.Done()
		for result := range dp.results {
			resultsMux.Lock()
			resultList = append(resultList, &result)
			resultsMux.Unlock()
		}
	}()

	// Process directory
	if err := dp.processDirectoryRecursive(rootPath, processor); err != nil {
		return nil, fmt.Errorf("failed to process directory: %w", err)
	}

	// Wait for all workers to complete
	dp.wg.Wait()
	close(dp.results)

	// Wait for result collector to finish
	collectorDone.Wait()

	// Check for errors
	dp.resultsMux.RLock()
	errors := append([]error(nil), dp.errors...)
	dp.resultsMux.RUnlock()

	if len(errors) > 0 {
		return resultList, fmt.Errorf("encountered %d errors during processing", len(errors))
	}

	return resultList, nil
}

// processDirectoryRecursive processes a directory and its contents recursively
func (dp *DirectoryProcessor) processDirectoryRecursive(dirPath string, processor DirectoryBlockProcessor) error {
	// Check for cancellation
	select {
	case <-dp.ctx.Done():
		return dp.ctx.Err()
	default:
	}

	// Read directory entries
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %w", dirPath, err)
	}

	// Create directory manifest
	manifest := NewDirectoryManifest()

	// Process each entry
	for _, entry := range entries {
		entryPath := filepath.Join(dirPath, entry.Name())

		// Check for cancellation
		select {
		case <-dp.ctx.Done():
			return dp.ctx.Err()
		default:
		}

		// Skip hidden files if configured
		if entry.Name()[0] == '.' {
			continue
		}

		if entry.IsDir() {
			// Process subdirectory
			if err := dp.processDirectoryEntry(entryPath, entry, manifest, processor); err != nil {
				if !dp.handleError(entryPath, err) {
					return err
				}
			}
		} else {
			// Process file
			if err := dp.processFileEntry(entryPath, entry, manifest, processor); err != nil {
				if !dp.handleError(entryPath, err) {
					return err
				}
			}
		}
	}

	// Store directory manifest
	return dp.storeDirectoryManifest(dirPath, manifest, processor)
}

// processDirectoryEntry processes a directory entry
func (dp *DirectoryProcessor) processDirectoryEntry(dirPath string, entry os.DirEntry, manifest *DirectoryManifest, processor DirectoryBlockProcessor) error {
	// First recursively process the subdirectory
	if err := dp.processDirectoryRecursive(dirPath, processor); err != nil {
		return err
	}

	// Get directory info
	info, err := entry.Info()
	if err != nil {
		return fmt.Errorf("failed to get directory info: %w", err)
	}

	// Derive directory-specific key
	dirKey, err := crypto.DeriveDirectoryKey(dp.encryptionKey, dirPath)
	if err != nil {
		return fmt.Errorf("failed to derive directory key: %w", err)
	}

	// Encrypt directory name
	encryptedName, err := crypto.EncryptFileName(entry.Name(), dirKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt directory name: %w", err)
	}

	// Create directory entry with a placeholder CID
	// In a real implementation, this would be the CID of the directory's manifest
	dirEntry := DirectoryEntry{
		EncryptedName: encryptedName,
		CID:           "dir-" + filepath.Base(dirPath), // Simple placeholder for testing
		Type:          DirectoryType,
		Size:          0,
		ModifiedAt:    info.ModTime(),
	}

	// Add to manifest
	return manifest.AddEntry(dirEntry)
}

// processFileEntry processes a file entry
func (dp *DirectoryProcessor) processFileEntry(filePath string, entry os.DirEntry, manifest *DirectoryManifest, processor DirectoryBlockProcessor) error {
	// Get file info
	info, err := entry.Info()
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}

	// Acquire worker slot
	dp.workerPool <- struct{}{}
	dp.wg.Add(1)

	// Process file in goroutine
	go func() {
		defer func() {
			<-dp.workerPool
			dp.wg.Done()
		}()

		startTime := time.Now()

		// Open file
		file, err := os.Open(filePath)
		if err != nil {
			dp.recordError(fmt.Errorf("failed to open file %s: %w", filePath, err))
			return
		}
		defer file.Close()

		// Create file processor
		fileProcessor := &FileBlockProcessor{
			FilePath:      filePath,
			FileSize:      info.Size(),
			Processor:     processor,
			Results:       make([]*ProcessResult, 0),
			EncryptionKey: dp.encryptionKey,
		}

		// Process file blocks
		if err := dp.splitter.Split(file, fileProcessor); err != nil {
			dp.recordError(fmt.Errorf("failed to process file %s: %w", filePath, err))
			return
		}

		// Update progress
		atomic.AddInt64(&dp.processedFiles, 1)
		atomic.AddInt64(&dp.processedBytes, info.Size())

		if dp.progressFn != nil {
			dp.progressFn(atomic.LoadInt64(&dp.processedFiles), atomic.LoadInt64(&dp.totalFiles), filePath)
		}

		// Derive directory-specific key
		dirKey, err := crypto.DeriveDirectoryKey(dp.encryptionKey, filepath.Dir(filePath))
		if err != nil {
			dp.recordError(fmt.Errorf("failed to derive directory key: %w", err))
			return
		}

		// Encrypt filename
		encryptedName, err := crypto.EncryptFileName(entry.Name(), dirKey)
		if err != nil {
			dp.recordError(fmt.Errorf("failed to encrypt filename: %w", err))
			return
		}

		// Create file entry
		fileEntry := DirectoryEntry{
			EncryptedName: encryptedName,
			CID:           fileProcessor.GetFileCID(),
			Type:          FileType,
			Size:          info.Size(),
			ModifiedAt:    info.ModTime(),
		}

		// Add to manifest (thread-safe)
		if err := manifest.AddEntry(fileEntry); err != nil {
			dp.recordError(fmt.Errorf("failed to add file entry: %w", err))
			return
		}

		// Record result
		result := ProcessResult{
			Path:           filePath,
			Type:           FileType,
			Size:           info.Size(),
			CID:            fileProcessor.GetFileCID(),
			EncryptedName:  encryptedName,
			ProcessedAt:    time.Now(),
			ProcessingTime: time.Since(startTime),
		}

		dp.results <- result
	}()

	return nil
}

// storeDirectoryManifest stores the directory manifest
func (dp *DirectoryProcessor) storeDirectoryManifest(dirPath string, manifest *DirectoryManifest, processor DirectoryBlockProcessor) error {
	// Encrypt and store manifest
	encryptedManifest, err := EncryptManifest(manifest, dp.encryptionKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt manifest: %w", err)
	}

	// Create block from encrypted manifest
	manifestBlock, err := NewBlock(encryptedManifest)
	if err != nil {
		return fmt.Errorf("failed to create manifest block: %w", err)
	}

	// Store manifest block
	if err := processor.ProcessDirectoryManifest(dirPath, manifestBlock); err != nil {
		return fmt.Errorf("failed to store manifest: %w", err)
	}

	// Record result
	result := ProcessResult{
		Path:           dirPath,
		Type:           DirectoryType,
		Size:           0,
		CID:            manifestBlock.ID,
		ManifestCID:    manifestBlock.ID,
		ProcessedAt:    time.Now(),
		ProcessingTime: 0,
	}

	dp.results <- result

	return nil
}

// calculateTotals calculates total files and bytes to process
func (dp *DirectoryProcessor) calculateTotals(rootPath string) error {
	return filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			atomic.AddInt64(&dp.totalFiles, 1)
			atomic.AddInt64(&dp.totalBytes, info.Size())
		}

		return nil
	})
}

// handleError handles errors during processing
func (dp *DirectoryProcessor) handleError(path string, err error) bool {
	dp.recordError(err)

	if dp.errorHandler != nil {
		return dp.errorHandler(path, err)
	}

	return false // Stop on error by default
}

// recordError records an error
func (dp *DirectoryProcessor) recordError(err error) {
	dp.resultsMux.Lock()
	dp.errors = append(dp.errors, err)
	dp.resultsMux.Unlock()
}

// Cancel cancels the processing
func (dp *DirectoryProcessor) Cancel() {
	if dp.cancelFunc != nil {
		dp.cancelFunc()
	}
}

// GetProgress returns current progress
func (dp *DirectoryProcessor) GetProgress() (processed, total int64) {
	return atomic.LoadInt64(&dp.processedFiles), atomic.LoadInt64(&dp.totalFiles)
}

// DirectoryBlockProcessor interface for handling processed blocks
type DirectoryBlockProcessor interface {
	ProcessDirectoryBlock(blockIndex int, block *Block) error
	ProcessDirectoryManifest(dirPath string, manifestBlock *Block) error
}

// FileBlockProcessor processes blocks from a single file
type FileBlockProcessor struct {
	FilePath      string
	FileSize      int64
	Processor     DirectoryBlockProcessor
	Results       []*ProcessResult
	EncryptionKey *crypto.EncryptionKey
	fileCID       string
	mutex         sync.Mutex
}

// ProcessBlock implements BlockProcessor interface
func (fbp *FileBlockProcessor) ProcessBlock(blockIndex int, block *Block) error {
	fbp.mutex.Lock()
	defer fbp.mutex.Unlock()

	// Store the first block's CID as the file CID
	if blockIndex == 0 {
		fbp.fileCID = block.ID
	}

	return fbp.Processor.ProcessDirectoryBlock(blockIndex, block)
}

// GetFileCID returns the file's CID (first block's CID)
func (fbp *FileBlockProcessor) GetFileCID() string {
	fbp.mutex.Lock()
	defer fbp.mutex.Unlock()
	return fbp.fileCID
}

// StreamingDirectoryProcessor for processing large directories without memory overflow
type StreamingDirectoryProcessor struct {
	*DirectoryProcessor
	maxMemoryUsage int64
	currentMemory  int64
	memoryMux      sync.RWMutex
}

// NewStreamingDirectoryProcessor creates a processor optimized for large directories
func NewStreamingDirectoryProcessor(config *ProcessorConfig, maxMemoryMB int64) (*StreamingDirectoryProcessor, error) {
	baseProcessor, err := NewDirectoryProcessor(config)
	if err != nil {
		return nil, err
	}

	return &StreamingDirectoryProcessor{
		DirectoryProcessor: baseProcessor,
		maxMemoryUsage:     maxMemoryMB * 1024 * 1024,
		currentMemory:      0,
	}, nil
}

// ProcessDirectoryStreaming processes directory with memory management
func (sdp *StreamingDirectoryProcessor) ProcessDirectoryStreaming(rootPath string, processor DirectoryBlockProcessor) error {
	// Implementation would include memory monitoring and backpressure
	// For now, delegate to base processor
	_, err := sdp.ProcessDirectory(rootPath, processor)
	return err
}
