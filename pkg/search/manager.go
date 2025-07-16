package search

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/analysis/analyzer/standard"
	"github.com/blevesearch/bleve/v2/mapping"
)

// IndexRequest represents a request to index a file or directory
type IndexRequest struct {
	Operation string                 // "add", "update", "delete"
	Path      string                 // File path
	CID       string                 // Content identifier
	Metadata  map[string]interface{} // Additional metadata
	Priority  int                    // Request priority (higher = more urgent)
	Timestamp time.Time              // Request timestamp
}

// SearchManager manages the search indexing and querying functionality
type SearchManager struct {
	// Core components
	config      SearchConfig
	bleveIndex  bleve.Index
	fileIndex   FileIndexInterface
	storage     *storage.Manager
	
	// Async processing
	indexQueue  chan IndexRequest
	workers     sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc
	
	// State management
	mutex       sync.RWMutex
	started     bool
	indexPath   string
	
	// Metrics
	metrics     SearchMetrics
}

// SearchMetrics tracks search performance metrics
type SearchMetrics struct {
	mutex           sync.RWMutex
	IndexedFiles    int64
	QueueSize       int
	IndexingErrors  int64
	SearchQueries   int64
	AvgSearchTimeMs float64
	LastIndexTime   time.Time
}

// NewSearchManager creates a new search manager instance
func NewSearchManager(config SearchConfig, fileIndex FileIndexInterface, storage *storage.Manager) (*SearchManager, error) {
	// Validate configuration
	if config.IndexPath == "" {
		config = DefaultSearchConfig()
	}
	
	// Expand home directory
	indexPath := config.IndexPath
	if indexPath[:2] == "~/" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		indexPath = filepath.Join(homeDir, indexPath[2:])
	}
	
	// Create search directory if it doesn't exist
	searchDir := filepath.Dir(indexPath)
	if err := os.MkdirAll(searchDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create search directory: %w", err)
	}
	
	// Create context for lifecycle management
	ctx, cancel := context.WithCancel(context.Background())
	
	sm := &SearchManager{
		config:     config,
		fileIndex:  fileIndex,
		storage:    storage,
		indexQueue: make(chan IndexRequest, config.BatchSize*2),
		ctx:        ctx,
		cancel:     cancel,
		indexPath:  indexPath,
		metrics:    SearchMetrics{},
	}
	
	return sm, nil
}

// Start initializes the search manager and starts background workers
func (sm *SearchManager) Start() error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	
	if sm.started {
		return fmt.Errorf("search manager already started")
	}
	
	// Open or create Bleve index
	var err error
	sm.bleveIndex, err = sm.openOrCreateIndex()
	if err != nil {
		return fmt.Errorf("failed to open search index: %w", err)
	}
	
	// Start background workers
	for i := 0; i < sm.config.Workers; i++ {
		sm.workers.Add(1)
		go sm.indexingWorker(i)
	}
	
	// Start maintenance tasks
	if sm.config.OptimizeInterval > 0 {
		go sm.maintenanceLoop()
	}
	
	sm.started = true
	return nil
}

// Stop gracefully shuts down the search manager
func (sm *SearchManager) Stop() error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	
	if !sm.started {
		return nil
	}
	
	// Cancel context to signal workers to stop
	sm.cancel()
	
	// Close the queue
	close(sm.indexQueue)
	
	// Wait for workers to finish
	sm.workers.Wait()
	
	// Close Bleve index
	if sm.bleveIndex != nil {
		if err := sm.bleveIndex.Close(); err != nil {
			return fmt.Errorf("failed to close search index: %w", err)
		}
	}
	
	sm.started = false
	return nil
}

// openOrCreateIndex opens an existing Bleve index or creates a new one
func (sm *SearchManager) openOrCreateIndex() (bleve.Index, error) {
	// Try to open existing index
	index, err := bleve.Open(sm.indexPath)
	if err == nil {
		return index, nil
	}
	
	// Create new index if it doesn't exist
	if err == bleve.ErrorIndexPathDoesNotExist {
		mapping := sm.createIndexMapping()
		index, err = bleve.New(sm.indexPath, mapping)
		if err != nil {
			return nil, fmt.Errorf("failed to create new index: %w", err)
		}
		return index, nil
	}
	
	return nil, fmt.Errorf("failed to open index: %w", err)
}

// createIndexMapping creates the Bleve index mapping for NoiseFS files
func (sm *SearchManager) createIndexMapping() mapping.IndexMapping {
	// Create a new index mapping
	indexMapping := bleve.NewIndexMapping()
	
	// Create document mapping for files
	fileMapping := bleve.NewDocumentMapping()
	
	// Path field - searchable, stored
	pathField := bleve.NewTextFieldMapping()
	pathField.Store = true
	pathField.Index = true
	pathField.Analyzer = "keyword"
	fileMapping.AddFieldMappingsAt("path", pathField)
	
	// Filename field - searchable, stored
	filenameField := bleve.NewTextFieldMapping()
	filenameField.Store = true
	filenameField.Index = true
	filenameField.Analyzer = standard.Name
	fileMapping.AddFieldMappingsAt("filename", filenameField)
	
	// Content field - searchable, not stored (for preview)
	contentField := bleve.NewTextFieldMapping()
	contentField.Store = false
	contentField.Index = true
	contentField.Analyzer = standard.Name
	fileMapping.AddFieldMappingsAt("content", contentField)
	
	// Content preview field - searchable, stored
	previewField := bleve.NewTextFieldMapping()
	previewField.Store = true
	previewField.Index = true
	previewField.Analyzer = standard.Name
	fileMapping.AddFieldMappingsAt("preview", previewField)
	
	// Size field - numeric, facetable
	sizeField := bleve.NewNumericFieldMapping()
	sizeField.Store = true
	sizeField.Index = true
	fileMapping.AddFieldMappingsAt("size", sizeField)
	
	// Modified date field - datetime, sortable
	modifiedField := bleve.NewDateTimeFieldMapping()
	modifiedField.Store = true
	modifiedField.Index = true
	fileMapping.AddFieldMappingsAt("modified", modifiedField)
	
	// MIME type field - keyword, facetable
	mimeField := bleve.NewTextFieldMapping()
	mimeField.Store = true
	mimeField.Index = true
	mimeField.Analyzer = "keyword"
	fileMapping.AddFieldMappingsAt("mime_type", mimeField)
	
	// Descriptor CID field - keyword, stored
	cidField := bleve.NewTextFieldMapping()
	cidField.Store = true
	cidField.Index = true
	cidField.Analyzer = "keyword"
	fileMapping.AddFieldMappingsAt("descriptor_cid", cidField)
	
	// Directory field - keyword, facetable
	dirField := bleve.NewTextFieldMapping()
	dirField.Store = true
	dirField.Index = true
	dirField.Analyzer = "keyword"
	fileMapping.AddFieldMappingsAt("directory", dirField)
	
	// Add the file mapping to the index
	indexMapping.AddDocumentMapping("file", fileMapping)
	
	// Set the default type
	indexMapping.DefaultType = "file"
	
	return indexMapping
}

// indexingWorker processes indexing requests from the queue
func (sm *SearchManager) indexingWorker(id int) {
	defer sm.workers.Done()
	
	for {
		select {
		case req, ok := <-sm.indexQueue:
			if !ok {
				// Queue closed, worker should exit
				return
			}
			
			// Process the indexing request
			err := sm.processIndexRequest(req)
			if err != nil {
				sm.incrementErrorCount()
			}
			
		case <-sm.ctx.Done():
			// Context cancelled, worker should exit
			return
		}
	}
}

// processIndexRequest handles a single indexing request
func (sm *SearchManager) processIndexRequest(req IndexRequest) error {
	switch req.Operation {
	case "add", "update":
		return sm.indexDocument(req)
	case "delete":
		return sm.deleteDocument(req.Path)
	default:
		return fmt.Errorf("unknown operation: %s", req.Operation)
	}
}

// indexDocument indexes a single document
func (sm *SearchManager) indexDocument(req IndexRequest) error {
	// Get file metadata from FileIndex
	fileEntry, exists := sm.fileIndex.GetFile(req.Path)
	if !exists {
		// Check if it's a directory
		dirEntries, dirExists := sm.fileIndex.GetDirectory(req.Path)
		if !dirExists {
			return fmt.Errorf("file not found in index: %s", req.Path)
		}
		// For directories, just create a basic entry
		doc := map[string]interface{}{
			"path":           req.Path,
			"filename":       filepath.Base(req.Path),
			"is_directory":   true,
			"children_count": len(dirEntries),
		}
		// Add metadata if provided
		for k, v := range req.Metadata {
			doc[k] = v
		}
		// Index the document
		if err := sm.bleveIndex.Index(req.Path, doc); err != nil {
			return fmt.Errorf("failed to index directory: %w", err)
		}
		return nil
	}
	
	// Extract fields from the interface{}
	entryMap, ok := fileEntry.(map[string]interface{})
	if !ok {
		return fmt.Errorf("unexpected file entry type: %T", fileEntry)
	}
	
	// Create document for indexing
	doc := map[string]interface{}{
		"path":           req.Path,
		"filename":       filepath.Base(req.Path),
		"size":           entryMap["FileSize"],
		"modified":       entryMap["ModifiedAt"],
		"descriptor_cid": entryMap["DescriptorCID"],
		"directory":      filepath.Dir(req.Path),
		"is_directory":   false,
	}
	
	// Add metadata if provided
	for k, v := range req.Metadata {
		doc[k] = v
	}
	
	// Index the document
	if err := sm.bleveIndex.Index(req.Path, doc); err != nil {
		return fmt.Errorf("failed to index document: %w", err)
	}
	
	// Update metrics
	sm.incrementIndexedCount()
	sm.updateLastIndexTime()
	
	return nil
}

// deleteDocument removes a document from the index
func (sm *SearchManager) deleteDocument(path string) error {
	if err := sm.bleveIndex.Delete(path); err != nil {
		return fmt.Errorf("failed to delete document: %w", err)
	}
	return nil
}

// maintenanceLoop performs periodic index maintenance
func (sm *SearchManager) maintenanceLoop() {
	optimizeTicker := time.NewTicker(sm.config.OptimizeInterval)
	defer optimizeTicker.Stop()
	
	for {
		select {
		case <-optimizeTicker.C:
			// Optimize the index
			sm.OptimizeIndex()
			
		case <-sm.ctx.Done():
			return
		}
	}
}

// Metric update methods
func (sm *SearchManager) incrementIndexedCount() {
	sm.metrics.mutex.Lock()
	defer sm.metrics.mutex.Unlock()
	sm.metrics.IndexedFiles++
}

func (sm *SearchManager) incrementErrorCount() {
	sm.metrics.mutex.Lock()
	defer sm.metrics.mutex.Unlock()
	sm.metrics.IndexingErrors++
}

func (sm *SearchManager) updateLastIndexTime() {
	sm.metrics.mutex.Lock()
	defer sm.metrics.mutex.Unlock()
	sm.metrics.LastIndexTime = time.Now()
}