package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	noisefs "github.com/TheEntropyCollective/noisefs/pkg/core/client"
	"github.com/TheEntropyCollective/noisefs/pkg/core/descriptors"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/config"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/logging"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/workers"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/TheEntropyCollective/noisefs/pkg/util"
)

// MemoryMonitor tracks memory usage during streaming operations
type MemoryMonitor struct {
	mu           sync.RWMutex
	startMemory  runtime.MemStats
	peakMemory   uint64
	currentAlloc uint64
	enabled      bool
}

// NewMemoryMonitor creates a new memory monitor
func NewMemoryMonitor(enabled bool) *MemoryMonitor {
	m := &MemoryMonitor{enabled: enabled}
	if enabled {
		runtime.ReadMemStats(&m.startMemory)
		m.currentAlloc = m.startMemory.Alloc
		m.peakMemory = m.startMemory.Alloc
	}
	return m
}

// Update updates memory statistics
func (m *MemoryMonitor) Update() {
	if !m.enabled {
		return
	}
	
	m.mu.Lock()
	defer m.mu.Unlock()
	
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	m.currentAlloc = memStats.Alloc
	
	if memStats.Alloc > m.peakMemory {
		m.peakMemory = memStats.Alloc
	}
}

// GetStats returns current memory statistics
func (m *MemoryMonitor) GetStats() (current, peak, start uint64) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentAlloc, m.peakMemory, m.startMemory.Alloc
}

// StreamingBlockProcessor handles streaming block processing with bounded memory
type StreamingBlockProcessor struct {
	client          *noisefs.Client
	descriptor      *descriptors.Descriptor
	storageManager  *storage.Manager
	pool            *workers.Pool
	memoryLimit     int64
	bufferSize      int
	currentMemory   int64
	mu              sync.Mutex
	logger          *logging.Logger
	memMonitor      *MemoryMonitor
	
	// Channels for pipelined processing
	blockChannel    chan *blockData
	xorChannel      chan *xorData
	storeChannel    chan *storeData
	errors          chan error
	done            chan bool
	
	// Progress tracking
	blocksProcessed int
	bytesProcessed  int64
}

type blockData struct {
	index int
	block *blocks.Block
}

type xorData struct {
	index           int
	anonymizedBlock *blocks.Block
	randomizer1CID  string
	randomizer2CID  string
}

type storeData struct {
	index          int
	dataCID        string
	randomizer1CID string
	randomizer2CID string
}

// NewStreamingBlockProcessor creates a new streaming processor
func NewStreamingBlockProcessor(client *noisefs.Client, storageManager *storage.Manager, descriptor *descriptors.Descriptor, cfg *config.Config, logger *logging.Logger) *StreamingBlockProcessor {
	workerCount := cfg.Performance.MaxConcurrentOps
	if workerCount <= 0 {
		workerCount = runtime.NumCPU()
	}
	
	pool := workers.NewPool(workers.Config{
		WorkerCount:     workerCount,
		BufferSize:      workerCount * 2,
		ShutdownTimeout: 30 * time.Second,
	})
	
	memoryLimit := cfg.Performance.MemoryLimit
	if memoryLimit <= 0 {
		memoryLimit = 512 // Default 512MB
	}
	
	bufferSize := cfg.Performance.StreamBufferSize
	if bufferSize <= 0 {
		bufferSize = 10 // Default buffer size
	}
	
	return &StreamingBlockProcessor{
		client:         client,
		descriptor:     descriptor,
		storageManager: storageManager,
		pool:           pool,
		memoryLimit:    int64(memoryLimit) * 1024 * 1024,
		bufferSize:     bufferSize,
		logger:         logger,
		memMonitor:     NewMemoryMonitor(cfg.Performance.EnableMemoryMonitoring),
		blockChannel:   make(chan *blockData, bufferSize),
		xorChannel:     make(chan *xorData, bufferSize),
		storeChannel:   make(chan *storeData, bufferSize),
		errors:         make(chan error, 1),
		done:           make(chan bool),
	}
}

// ProcessBlock implements blocks.BlockProcessor interface
func (p *StreamingBlockProcessor) ProcessBlock(blockIndex int, block *blocks.Block) error {
	// Check memory limit
	blockSize := int64(block.Size())
	p.mu.Lock()
	if p.currentMemory+blockSize > p.memoryLimit {
		p.mu.Unlock()
		// Wait for memory to be freed
		p.logger.Debug("Waiting for memory to be freed", map[string]interface{}{
			"current_memory_mb": p.currentMemory / (1024 * 1024),
			"block_size_kb":     blockSize / 1024,
			"limit_mb":          p.memoryLimit / (1024 * 1024),
		})
		
		// Apply backpressure - wait for pipeline to process blocks
		time.Sleep(100 * time.Millisecond)
		
		// Retry
		p.mu.Lock()
	}
	p.currentMemory += blockSize
	p.mu.Unlock()
	
	// Send block for processing
	select {
	case p.blockChannel <- &blockData{index: blockIndex, block: block}:
		return nil
	case err := <-p.errors:
		return err
	}
}

// Start starts the processing pipeline
func (p *StreamingBlockProcessor) Start() error {
	if err := p.pool.Start(); err != nil {
		return fmt.Errorf("failed to start worker pool: %w", err)
	}
	
	// Start pipeline stages
	go p.xorStage()
	go p.storeStage()
	go p.descriptorStage()
	
	// Start memory monitoring
	if p.memMonitor.enabled {
		go p.monitorMemory()
	}
	
	return nil
}

// xorStage performs XOR operations on blocks
func (p *StreamingBlockProcessor) xorStage() {
	for blockData := range p.blockChannel {
		// Select randomizers
		randBlock1, cid1, randBlock2, cid2, _, err := p.client.SelectRandomizers(context.Background(), blockData.block.Size())
		if err != nil {
			p.errors <- fmt.Errorf("failed to select randomizers for block %d: %w", blockData.index, err)
			return
		}
		
		// Perform XOR
		xorBlock, err := blockData.block.XOR(randBlock1, randBlock2)
		if err != nil {
			p.errors <- fmt.Errorf("failed to XOR block %d: %w", blockData.index, err)
			return
		}
		
		// Free original block memory
		p.mu.Lock()
		p.currentMemory -= int64(blockData.block.Size())
		p.mu.Unlock()
		
		// Send to next stage
		p.xorChannel <- &xorData{
			index:           blockData.index,
			anonymizedBlock: xorBlock,
			randomizer1CID:  cid1,
			randomizer2CID:  cid2,
		}
		
		// Update memory monitor
		p.memMonitor.Update()
	}
	close(p.xorChannel)
}

// storeStage stores anonymized blocks
func (p *StreamingBlockProcessor) storeStage() {
	for xorData := range p.xorChannel {
		// Store anonymized block
		dataCID, err := p.client.StoreBlockWithCache(context.Background(), xorData.anonymizedBlock)
		if err != nil {
			p.errors <- fmt.Errorf("failed to store block %d: %w", xorData.index, err)
			return
		}
		
		// Free XOR block memory
		p.mu.Lock()
		p.currentMemory -= int64(xorData.anonymizedBlock.Size())
		p.mu.Unlock()
		
		// Send to descriptor stage
		p.storeChannel <- &storeData{
			index:          xorData.index,
			dataCID:        dataCID,
			randomizer1CID: xorData.randomizer1CID,
			randomizer2CID: xorData.randomizer2CID,
		}
		
		// Update progress
		p.mu.Lock()
		p.blocksProcessed++
		p.bytesProcessed += int64(xorData.anonymizedBlock.Size())
		p.mu.Unlock()
		
		// Update memory monitor
		p.memMonitor.Update()
	}
	close(p.storeChannel)
}

// descriptorStage adds blocks to descriptor in order
func (p *StreamingBlockProcessor) descriptorStage() {
	// Buffer for out-of-order blocks
	blockBuffer := make(map[int]*storeData)
	nextIndex := 0
	
	for storeData := range p.storeChannel {
		blockBuffer[storeData.index] = storeData
		
		// Process sequential blocks
		for {
			if data, exists := blockBuffer[nextIndex]; exists {
				if err := p.descriptor.AddBlockTriple(data.dataCID, data.randomizer1CID, data.randomizer2CID); err != nil {
					p.errors <- fmt.Errorf("failed to add block triple %d: %w", data.index, err)
					return
				}
				delete(blockBuffer, nextIndex)
				nextIndex++
			} else {
				break
			}
		}
	}
	
	// Process any remaining blocks
	for i := nextIndex; i < nextIndex+len(blockBuffer); i++ {
		if data, exists := blockBuffer[i]; exists {
			if err := p.descriptor.AddBlockTriple(data.dataCID, data.randomizer1CID, data.randomizer2CID); err != nil {
				p.errors <- fmt.Errorf("failed to add final block triple %d: %w", data.index, err)
				return
			}
		}
	}
	
	p.done <- true
}

// monitorMemory periodically monitors memory usage
func (p *StreamingBlockProcessor) monitorMemory() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	
	for range ticker.C {
		select {
		case <-p.done:
			return
		default:
			p.memMonitor.Update()
			current, peak, start := p.memMonitor.GetStats()
			
			p.logger.Debug("Memory usage", map[string]interface{}{
				"current_mb":        current / (1024 * 1024),
				"peak_mb":           peak / (1024 * 1024),
				"start_mb":          start / (1024 * 1024),
				"increase_mb":       (current - start) / (1024 * 1024),
				"blocks_processed":  p.blocksProcessed,
				"pipeline_memory_mb": p.currentMemory / (1024 * 1024),
			})
		}
	}
}

// Wait waits for processing to complete
func (p *StreamingBlockProcessor) Wait() error {
	close(p.blockChannel)
	
	select {
	case <-p.done:
		p.pool.Shutdown()
		return nil
	case err := <-p.errors:
		p.pool.Shutdown()
		return err
	}
}

// GetStats returns processing statistics
func (p *StreamingBlockProcessor) GetStats() (blocksProcessed int, bytesProcessed int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.blocksProcessed, p.bytesProcessed
}

// streamingUploadFile uploads a file using streaming with bounded memory
func streamingUploadFile(storageManager *storage.Manager, client *noisefs.Client, filePath string, blockSize int, quiet bool, jsonOutput bool, cfg *config.Config, logger *logging.Logger) error {
	// Track overall upload time
	uploadStartTime := time.Now()
	
	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()
	
	// Get file info
	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}
	
	// Create descriptor
	descriptor := descriptors.NewDescriptor(
		filepath.Base(filePath),
		fileInfo.Size(),
		fileInfo.Size(),
		blockSize,
	)
	
	// Create streaming processor
	processor := NewStreamingBlockProcessor(client, storageManager, descriptor, cfg, logger)
	
	// Start processing pipeline
	if err := processor.Start(); err != nil {
		return fmt.Errorf("failed to start processor: %w", err)
	}
	
	// Create streaming splitter
	splitter, err := blocks.NewStreamingSplitter(blockSize)
	if err != nil {
		return fmt.Errorf("failed to create streaming splitter: %w", err)
	}
	
	// Progress tracking
	var progress *util.ProgressBar
	if !quiet {
		progress = util.NewProgressBar(fileInfo.Size(), "Streaming upload", os.Stdout)
	}
	
	// Split and process file with progress
	progressCallback := func(bytesProcessed int64, blocksProcessed int) {
		if progress != nil {
			progress.SetCurrent(bytesProcessed)
		}
	}
	
	logger.Info("Starting streaming upload", map[string]interface{}{
		"file_size_mb":       fileInfo.Size() / (1024 * 1024),
		"block_size_kb":      blockSize / 1024,
		"memory_limit_mb":    cfg.Performance.MemoryLimit,
		"buffer_size":        cfg.Performance.StreamBufferSize,
		"concurrent_workers": cfg.Performance.MaxConcurrentOps,
	})
	
	// Process file in streaming fashion
	if err := splitter.SplitWithProgress(file, processor, progressCallback); err != nil {
		return fmt.Errorf("failed to process file: %w", err)
	}
	
	// Wait for processing to complete
	if err := processor.Wait(); err != nil {
		return fmt.Errorf("processing failed: %w", err)
	}
	
	if progress != nil {
		progress.Finish()
	}
	
	// Update descriptor with final size (should match file size)
	blocksProcessed, bytesProcessed := processor.GetStats()
	logger.Info("Streaming processing complete", map[string]interface{}{
		"blocks_processed": blocksProcessed,
		"bytes_processed":  bytesProcessed,
	})
	
	// Store descriptor
	store, err := descriptors.NewStoreWithManager(storageManager)
	if err != nil {
		return fmt.Errorf("failed to create descriptor store: %w", err)
	}
	
	descriptorCID, err := store.Save(descriptor)
	if err != nil {
		return fmt.Errorf("failed to store descriptor: %w", err)
	}
	
	// Calculate total upload time
	totalUploadDuration := time.Since(uploadStartTime)
	
	// Get memory statistics
	if processor.memMonitor.enabled {
		current, peak, start := processor.memMonitor.GetStats()
		logger.Info("Memory usage summary", map[string]interface{}{
			"start_mb":    start / (1024 * 1024),
			"peak_mb":     peak / (1024 * 1024),
			"current_mb":  current / (1024 * 1024),
			"increase_mb": (peak - start) / (1024 * 1024),
		})
	}
	
	// Log upload completion with performance metrics
	logger.Info("Streaming upload completed successfully", map[string]interface{}{
		"descriptor_cid":      descriptorCID,
		"file_name":           filepath.Base(filePath),
		"file_size":           fileInfo.Size(),
		"block_count":         blocksProcessed,
		"total_duration_ms":   totalUploadDuration.Milliseconds(),
		"throughput_mb_per_s": float64(fileInfo.Size()) / (1024 * 1024) / totalUploadDuration.Seconds(),
	})
	
	// Display results
	if jsonOutput {
		result := util.UploadResult{
			DescriptorCID: descriptorCID,
			Filename:      filepath.Base(filePath),
			FileSize:      fileInfo.Size(),
			BlockCount:    blocksProcessed,
			BlockSize:     blockSize,
		}
		util.PrintJSONSuccess(result)
	} else if quiet {
		fmt.Println(descriptorCID)
	} else {
		fmt.Println("\nStreaming upload complete!")
		fmt.Printf("Descriptor CID: %s\n", descriptorCID)
		fmt.Printf("Performance: %.2f MB/s (total time: %.2fs)\n",
			float64(fileInfo.Size())/(1024*1024)/totalUploadDuration.Seconds(),
			totalUploadDuration.Seconds())
		
		if processor.memMonitor.enabled {
			_, peak, start := processor.memMonitor.GetStats()
			fmt.Printf("Memory usage: Peak %.2f MB (increase: %.2f MB)\n",
				float64(peak)/(1024*1024),
				float64(peak-start)/(1024*1024))
		}
	}
	
	// Record upload metrics
	client.RecordUpload(fileInfo.Size(), bytesProcessed*3) // *3 for data + 2 randomizer blocks
	
	return nil
}