package search

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/descriptors"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
)

// ContentExtractor handles extracting content from files for indexing
type ContentExtractor struct {
	storage        *storage.Manager
	maxPreviewSize int
	maxFileSize    int64
	supportedTypes []string
}

// NewContentExtractor creates a new content extractor
func NewContentExtractor(storage *storage.Manager, config SearchConfig) *ContentExtractor {
	return &ContentExtractor{
		storage:        storage, // Can be nil for metadata-only operations
		maxPreviewSize: config.ContentPreview,
		maxFileSize:    config.MaxFileSize,
		supportedTypes: config.SupportedTypes,
	}
}

// ExtractContent extracts indexable content from a file
func (ce *ContentExtractor) ExtractContent(ctx context.Context, descriptorCID string) (string, string, error) {
	// If no storage manager, skip content extraction (metadata-only mode)
	if ce.storage == nil {
		return "", "", nil
	}
	
	// Check if we should extract content based on file type
	if !ce.shouldExtractContent(descriptorCID) {
		return "", "", nil
	}

	// Retrieve descriptor
	descriptor, err := ce.retrieveDescriptor(ctx, descriptorCID)
	if err != nil {
		return "", "", fmt.Errorf("failed to retrieve descriptor: %w", err)
	}

	// Check file size
	if descriptor.FileSize > ce.maxFileSize {
		// File too large, only extract preview
		return ce.extractPreview(ctx, descriptor)
	}

	// Extract full content for small files
	content, err := ce.extractFullContent(ctx, descriptor)
	if err != nil {
		return "", "", err
	}

	// Create preview
	preview := ce.createPreview(content)

	return content, preview, nil
}

// shouldExtractContent checks if content should be extracted based on file type
func (ce *ContentExtractor) shouldExtractContent(path string) bool {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))

	for _, supported := range ce.supportedTypes {
		if ext == supported {
			return true
		}
	}

	return false
}

// retrieveDescriptor retrieves and parses a file descriptor
func (ce *ContentExtractor) retrieveDescriptor(ctx context.Context, cid string) (*descriptors.Descriptor, error) {
	// Retrieve descriptor block
	backend, err := ce.storage.GetDefaultBackend()
	if err != nil {
		return nil, fmt.Errorf("failed to get backend: %w", err)
	}
	
	// Create a block address for the CID
	address := &storage.BlockAddress{
		ID: cid,
	}
	
	block, err := backend.Get(ctx, address)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve descriptor: %w", err)
	}
	
	data := block.Data

	// Parse descriptor
	var descriptor descriptors.Descriptor
	if err := json.Unmarshal(data, &descriptor); err != nil {
		return nil, fmt.Errorf("failed to parse descriptor: %w", err)
	}

	return &descriptor, nil
}

// extractPreview extracts a preview from the beginning of a file
func (ce *ContentExtractor) extractPreview(ctx context.Context, descriptor *descriptors.Descriptor) (string, string, error) {
	// Only extract from the first block for preview
	if len(descriptor.Blocks) == 0 {
		return "", "", nil
	}

	// Get the first block
	firstBlockCID := descriptor.Blocks[0].DataCID
	backend, err := ce.storage.GetDefaultBackend()
	if err != nil {
		return "", "", fmt.Errorf("failed to get backend: %w", err)
	}
	
	address := &storage.BlockAddress{
		ID: firstBlockCID,
	}
	
	block, err := backend.Get(ctx, address)
	if err != nil {
		return "", "", fmt.Errorf("failed to retrieve first block: %w", err)
	}
	
	blockData := block.Data

	// For now, skip encryption handling since it's not in the descriptor
	// TODO: Add encryption support when available

	// Extract preview from block data
	preview := string(blockData)
	if len(preview) > ce.maxPreviewSize {
		preview = preview[:ce.maxPreviewSize]
	}

	// Clean up the preview (remove non-printable characters)
	preview = ce.cleanText(preview)

	return "", preview, nil
}

// extractFullContent extracts the complete content from a file
func (ce *ContentExtractor) extractFullContent(ctx context.Context, descriptor *descriptors.Descriptor) (string, error) {
	var contentBuilder strings.Builder
	contentBuilder.Grow(int(descriptor.FileSize))

	backend, err := ce.storage.GetDefaultBackend()
	if err != nil {
		return "", fmt.Errorf("failed to get backend: %w", err)
	}

	// Retrieve all blocks
	for _, blockPair := range descriptor.Blocks {
		address := &storage.BlockAddress{
			ID: blockPair.DataCID,
		}
		
		block, err := backend.Get(ctx, address)
		if err != nil {
			return "", fmt.Errorf("failed to retrieve block %s: %w", blockPair.DataCID, err)
		}
		
		blockData := block.Data

		// For now, skip encryption handling since it's not in the descriptor
		// TODO: Add encryption support when available

		// Append to content
		contentBuilder.Write(blockData)
	}

	content := contentBuilder.String()

	// Clean up the content
	content = ce.cleanText(content)

	return content, nil
}

// createPreview creates a preview from full content
func (ce *ContentExtractor) createPreview(content string) string {
	if len(content) <= ce.maxPreviewSize {
		return content
	}

	// Find a good break point (end of sentence or paragraph)
	preview := content[:ce.maxPreviewSize]

	// Look for sentence end
	lastPeriod := strings.LastIndex(preview, ". ")
	lastNewline := strings.LastIndex(preview, "\n")

	breakPoint := ce.maxPreviewSize
	if lastPeriod > ce.maxPreviewSize*3/4 {
		breakPoint = lastPeriod + 1
	} else if lastNewline > ce.maxPreviewSize*3/4 {
		breakPoint = lastNewline
	}

	if breakPoint < len(preview) {
		preview = preview[:breakPoint]
	}

	return strings.TrimSpace(preview) + "..."
}

// cleanText removes non-printable characters and normalizes whitespace
func (ce *ContentExtractor) cleanText(text string) string {
	// Replace tabs with spaces
	text = strings.ReplaceAll(text, "\t", " ")

	// Replace multiple spaces with single space
	text = strings.Join(strings.Fields(text), " ")

	// Remove non-printable characters
	var cleaned strings.Builder
	for _, r := range text {
		if r == '\n' || r == '\r' || (r >= 32 && r < 127) || r > 127 {
			cleaned.WriteRune(r)
		}
	}

	return cleaned.String()
}

// IndexingPipeline manages the async indexing pipeline
type IndexingPipeline struct {
	manager   *SearchManager
	extractor *ContentExtractor
	batchSize int
}

// NewIndexingPipeline creates a new indexing pipeline
func NewIndexingPipeline(manager *SearchManager, storage *storage.Manager, config SearchConfig) *IndexingPipeline {
	return &IndexingPipeline{
		manager:   manager,
		extractor: NewContentExtractor(storage, config),
		batchSize: config.BatchSize,
	}
}

// ProcessBatch processes a batch of indexing requests
func (ip *IndexingPipeline) ProcessBatch(ctx context.Context, requests []IndexRequest) error {
	// Group requests by operation
	adds := make([]IndexRequest, 0)
	deletes := make([]IndexRequest, 0)

	for _, req := range requests {
		switch req.Operation {
		case "add", "update":
			adds = append(adds, req)
		case "delete":
			deletes = append(deletes, req)
		}
	}

	// Process deletes first (they're usually faster)
	for _, req := range deletes {
		if err := ip.manager.processIndexRequest(req); err != nil {
			// Log error but continue processing
			continue
		}
	}

	// Process adds/updates with content extraction
	for _, req := range adds {
		// Extract content if not provided
		if req.Metadata["content"] == nil && req.Metadata["preview"] == nil {
			content, preview, err := ip.extractor.ExtractContent(ctx, req.CID)
			if err != nil {
				// Log error but continue - we can still index metadata
				req.Metadata["content"] = ""
				req.Metadata["preview"] = ""
			} else {
				req.Metadata["content"] = content
				req.Metadata["preview"] = preview
			}
		}

		// Process the request
		if err := ip.manager.processIndexRequest(req); err != nil {
			// Log error but continue processing
			continue
		}
	}

	return nil
}

// FileWatcher watches for file changes and triggers indexing
type FileWatcher struct {
	manager       *SearchManager
	fileIndex     FileIndexInterface
	lastCheck     time.Time
	checkInterval time.Duration
}

// NewFileWatcher creates a new file watcher
func NewFileWatcher(manager *SearchManager, fileIndex FileIndexInterface) *FileWatcher {
	return &FileWatcher{
		manager:       manager,
		fileIndex:     fileIndex,
		lastCheck:     time.Now(),
		checkInterval: 30 * time.Second,
	}
}

// Watch starts watching for file changes
func (fw *FileWatcher) Watch(ctx context.Context) {
	ticker := time.NewTicker(fw.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			fw.checkForChanges()
		case <-ctx.Done():
			return
		}
	}
}

// checkForChanges checks for new or modified files
func (fw *FileWatcher) checkForChanges() {
	files := fw.fileIndex.ListFiles()

	for path, entryData := range files {
		// Type assert the interface{} to a map
		entryMap, ok := entryData.(map[string]interface{})
		if !ok {
			continue
		}

		// Extract fields from the map
		descriptorCID, _ := entryMap["DescriptorCID"].(string)
		modifiedAt, _ := entryMap["ModifiedAt"].(time.Time)

		// Check if file was modified since last check
		if modifiedAt.After(fw.lastCheck) {
			// Queue for indexing
			req := IndexRequest{
				Operation: "update",
				Path:      path,
				CID:       descriptorCID,
				Priority:  5,
				Timestamp: time.Now(),
			}

			select {
			case fw.manager.indexQueue <- req:
				// Successfully queued
			case <-time.After(10 * time.Millisecond):
				// Skip if queue is full
			}
		}
	}

	fw.lastCheck = time.Now()
}

// QueuePrioritizer manages request prioritization in the indexing queue
type QueuePrioritizer struct {
	queue     chan IndexRequest
	highQueue chan IndexRequest
	lowQueue  chan IndexRequest
}

// NewQueuePrioritizer creates a new queue prioritizer
func NewQueuePrioritizer(size int) *QueuePrioritizer {
	return &QueuePrioritizer{
		queue:     make(chan IndexRequest, size),
		highQueue: make(chan IndexRequest, size/4),
		lowQueue:  make(chan IndexRequest, size/2),
	}
}

// Add adds a request to the appropriate priority queue
func (qp *QueuePrioritizer) Add(req IndexRequest) error {
	var targetQueue chan IndexRequest

	if req.Priority >= 8 {
		targetQueue = qp.highQueue
	} else if req.Priority <= 3 {
		targetQueue = qp.lowQueue
	} else {
		targetQueue = qp.queue
	}

	select {
	case targetQueue <- req:
		return nil
	case <-time.After(100 * time.Millisecond):
		return fmt.Errorf("queue full")
	}
}

// Next returns the next request to process
func (qp *QueuePrioritizer) Next() (IndexRequest, bool) {
	// Check high priority first
	select {
	case req := <-qp.highQueue:
		return req, true
	default:
	}

	// Then normal priority
	select {
	case req := <-qp.queue:
		return req, true
	default:
	}

	// Finally low priority
	select {
	case req := <-qp.lowQueue:
		return req, true
	default:
		return IndexRequest{}, false
	}
}

// Close closes all queues
func (qp *QueuePrioritizer) Close() {
	close(qp.highQueue)
	close(qp.queue)
	close(qp.lowQueue)
}
