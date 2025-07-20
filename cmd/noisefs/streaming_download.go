package main

import (
	"context"
	"fmt"
	"io"
	"os"
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

// StreamingDownloadProcessor handles streaming download with bounded memory
type StreamingDownloadProcessor struct {
	client         *noisefs.Client
	storageManager *storage.Manager
	writer         io.Writer
	descriptor     *descriptors.Descriptor
	pool           *workers.Pool
	memoryLimit    int64
	bufferSize     int
	currentMemory  int64
	mu             sync.Mutex
	logger         *logging.Logger
	memMonitor     *MemoryMonitor
	
	// Channels for pipelined processing
	fetchChannel     chan *fetchRequest
	xorChannel       chan *xorRequest
	writeChannel     chan *writeRequest
	errors           chan error
	done             chan bool
	
	// Progress tracking
	blocksProcessed  int
	bytesWritten     int64
	totalBlocks      int
}

type fetchRequest struct {
	index          int
	dataCID        string
	randomizer1CID string
	randomizer2CID string
}

type xorRequest struct {
	index           int
	dataBlock       *blocks.Block
	randomizer1     *blocks.Block
	randomizer2     *blocks.Block
}

type writeRequest struct {
	index int
	block *blocks.Block
}

// NewStreamingDownloadProcessor creates a new streaming download processor
func NewStreamingDownloadProcessor(client *noisefs.Client, storageManager *storage.Manager, writer io.Writer, descriptor *descriptors.Descriptor, cfg *config.Config, logger *logging.Logger) *StreamingDownloadProcessor {
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
	
	return &StreamingDownloadProcessor{
		client:         client,
		storageManager: storageManager,
		writer:         writer,
		descriptor:     descriptor,
		pool:           pool,
		memoryLimit:    int64(memoryLimit) * 1024 * 1024,
		bufferSize:     bufferSize,
		logger:         logger,
		memMonitor:     NewMemoryMonitor(cfg.Performance.EnableMemoryMonitoring),
		fetchChannel:   make(chan *fetchRequest, bufferSize),
		xorChannel:     make(chan *xorRequest, bufferSize),
		writeChannel:   make(chan *writeRequest, bufferSize),
		errors:         make(chan error, 1),
		done:           make(chan bool),
		totalBlocks:    len(descriptor.Blocks),
	}
}

// Start starts the download pipeline
func (p *StreamingDownloadProcessor) Start() error {
	if err := p.pool.Start(); err != nil {
		return fmt.Errorf("failed to start worker pool: %w", err)
	}
	
	// Start pipeline stages
	go p.fetchStage()
	go p.xorStage()
	go p.writeStage()
	
	// Start memory monitoring
	if p.memMonitor.enabled {
		go p.monitorMemory()
	}
	
	// Queue all blocks for fetching
	go p.queueBlocks()
	
	return nil
}

// queueBlocks queues all blocks for fetching
func (p *StreamingDownloadProcessor) queueBlocks() {
	for i, block := range p.descriptor.Blocks {
		p.fetchChannel <- &fetchRequest{
			index:          i,
			dataCID:        block.DataCID,
			randomizer1CID: block.RandomizerCID1,
			randomizer2CID: block.RandomizerCID2,
		}
	}
	close(p.fetchChannel)
}

// fetchStage fetches blocks from storage
func (p *StreamingDownloadProcessor) fetchStage() {
	// Use worker pool for parallel fetching
	var wg sync.WaitGroup
	
	for req := range p.fetchChannel {
		// Check memory limit before fetching
		estimatedSize := int64(p.descriptor.BlockSize * 3) // data + 2 randomizers
		
		p.mu.Lock()
		for p.currentMemory+estimatedSize > p.memoryLimit {
			p.mu.Unlock()
			// Wait for memory to be freed
			p.logger.Debug("Waiting for memory to fetch blocks", map[string]interface{}{
				"current_memory_mb": p.currentMemory / (1024 * 1024),
				"estimated_size_kb": estimatedSize / 1024,
				"limit_mb":          p.memoryLimit / (1024 * 1024),
			})
			time.Sleep(100 * time.Millisecond)
			p.mu.Lock()
		}
		p.currentMemory += estimatedSize
		p.mu.Unlock()
		
		wg.Add(1)
		req := req // Capture loop variable
		go func() {
			defer wg.Done()
			
			// Fetch blocks in parallel
			var dataBlock, randomizer1, randomizer2 *blocks.Block
			var err error
			
			// Create error channel for parallel fetches
			fetchErrors := make(chan error, 3)
			
			// Fetch data block
			go func() {
				dataBlock, err = p.client.RetrieveBlockWithCache(context.Background(), req.dataCID)
				fetchErrors <- err
			}()
			
			// Fetch randomizer 1
			go func() {
				randomizer1, err = p.client.RetrieveBlockWithCache(context.Background(), req.randomizer1CID)
				fetchErrors <- err
			}()
			
			// Fetch randomizer 2
			go func() {
				randomizer2, err = p.client.RetrieveBlockWithCache(context.Background(), req.randomizer2CID)
				fetchErrors <- err
			}()
			
			// Wait for all fetches
			for i := 0; i < 3; i++ {
				if err := <-fetchErrors; err != nil {
					p.errors <- fmt.Errorf("failed to fetch block %d: %w", req.index, err)
					return
				}
			}
			
			// Update actual memory usage
			actualSize := int64(dataBlock.Size() + randomizer1.Size() + randomizer2.Size())
			p.mu.Lock()
			p.currentMemory = p.currentMemory - estimatedSize + actualSize
			p.mu.Unlock()
			
			// Send to XOR stage
			p.xorChannel <- &xorRequest{
				index:       req.index,
				dataBlock:   dataBlock,
				randomizer1: randomizer1,
				randomizer2: randomizer2,
			}
			
			// Update memory monitor
			p.memMonitor.Update()
		}()
	}
	
	wg.Wait()
	close(p.xorChannel)
}

// xorStage performs XOR reconstruction
func (p *StreamingDownloadProcessor) xorStage() {
	for req := range p.xorChannel {
		// Reconstruct original block
		originalBlock, err := req.dataBlock.XOR(req.randomizer1, req.randomizer2)
		if err != nil {
			p.errors <- fmt.Errorf("failed to XOR block %d: %w", req.index, err)
			return
		}
		
		// Free memory from anonymized blocks
		blockMemory := int64(req.dataBlock.Size() + req.randomizer1.Size() + req.randomizer2.Size())
		p.mu.Lock()
		p.currentMemory -= blockMemory
		p.mu.Unlock()
		
		// Send to write stage
		p.writeChannel <- &writeRequest{
			index: req.index,
			block: originalBlock,
		}
		
		// Update memory monitor
		p.memMonitor.Update()
	}
	close(p.writeChannel)
}

// writeStage writes blocks to output in order
func (p *StreamingDownloadProcessor) writeStage() {
	// Use streaming assembler for out-of-order block handling
	assembler, err := blocks.NewStreamingAssembler(p.writer)
	if err != nil {
		p.errors <- fmt.Errorf("failed to create assembler: %w", err)
		return
	}
	
	assembler.SetTotalBlocks(p.totalBlocks)
	
	for req := range p.writeChannel {
		// Add block to assembler
		if err := assembler.AddBlock(req.index, req.block); err != nil {
			p.errors <- fmt.Errorf("failed to write block %d: %w", req.index, err)
			return
		}
		
		// Free block memory after writing
		p.mu.Lock()
		p.currentMemory -= int64(req.block.Size())
		p.blocksProcessed++
		p.bytesWritten += int64(req.block.Size())
		p.mu.Unlock()
		
		// Update memory monitor
		p.memMonitor.Update()
	}
	
	// Finalize assembly
	if err := assembler.Finalize(); err != nil {
		p.errors <- fmt.Errorf("failed to finalize assembly: %w", err)
		return
	}
	
	p.done <- true
}

// monitorMemory periodically monitors memory usage
func (p *StreamingDownloadProcessor) monitorMemory() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	
	for range ticker.C {
		select {
		case <-p.done:
			return
		default:
			p.memMonitor.Update()
			current, peak, start := p.memMonitor.GetStats()
			
			p.logger.Debug("Download memory usage", map[string]interface{}{
				"current_mb":        current / (1024 * 1024),
				"peak_mb":           peak / (1024 * 1024),
				"start_mb":          start / (1024 * 1024),
				"increase_mb":       (current - start) / (1024 * 1024),
				"blocks_processed":  p.blocksProcessed,
				"pipeline_memory_mb": p.currentMemory / (1024 * 1024),
				"progress_percent":  float64(p.blocksProcessed) / float64(p.totalBlocks) * 100,
			})
		}
	}
}

// Wait waits for download to complete
func (p *StreamingDownloadProcessor) Wait() error {
	select {
	case <-p.done:
		p.pool.Shutdown()
		return nil
	case err := <-p.errors:
		p.pool.Shutdown()
		return err
	}
}

// GetStats returns download statistics
func (p *StreamingDownloadProcessor) GetStats() (blocksProcessed int, bytesWritten int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.blocksProcessed, p.bytesWritten
}

// streamingDownloadFile downloads a file using streaming with bounded memory
func streamingDownloadFile(storageManager *storage.Manager, client *noisefs.Client, descriptorCID string, outputPath string, quiet bool, jsonOutput bool, cfg *config.Config, logger *logging.Logger) error {
	// Track download start time
	downloadStartTime := time.Now()
	
	// Create descriptor store
	store, err := descriptors.NewStoreWithManager(storageManager)
	if err != nil {
		return fmt.Errorf("failed to create descriptor store: %w", err)
	}
	
	// Load descriptor
	if !quiet {
		fmt.Printf("Loading descriptor from CID: %s\n", descriptorCID)
	}
	descriptor, err := store.Load(descriptorCID)
	if err != nil {
		return fmt.Errorf("failed to load descriptor: %w", err)
	}
	
	if !quiet {
		fmt.Printf("Downloading file: %s (%d bytes)\n", descriptor.Filename, descriptor.FileSize)
		fmt.Printf("Blocks to retrieve: %d\n", len(descriptor.Blocks))
	}
	
	// Create output file
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outputFile.Close()
	
	// Create streaming processor
	processor := NewStreamingDownloadProcessor(client, storageManager, outputFile, descriptor, cfg, logger)
	
	// Start processing pipeline
	if err := processor.Start(); err != nil {
		return fmt.Errorf("failed to start processor: %w", err)
	}
	
	// Progress tracking
	var progress *util.ProgressBar
	if !quiet {
		progress = util.NewProgressBar(descriptor.FileSize, "Streaming download", os.Stdout)
		
		// Update progress periodically
		go func() {
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			
			for range ticker.C {
				_, bytesWritten := processor.GetStats()
				progress.SetCurrent(bytesWritten)
				
				if bytesWritten >= descriptor.FileSize {
					break
				}
			}
		}()
	}
	
	logger.Info("Starting streaming download", map[string]interface{}{
		"file_size_mb":       descriptor.FileSize / (1024 * 1024),
		"block_count":        len(descriptor.Blocks),
		"block_size_kb":      descriptor.BlockSize / 1024,
		"memory_limit_mb":    cfg.Performance.MemoryLimit,
		"buffer_size":        cfg.Performance.StreamBufferSize,
		"concurrent_workers": cfg.Performance.MaxConcurrentOps,
	})
	
	// Wait for download to complete
	if err := processor.Wait(); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	
	if progress != nil {
		progress.Finish()
	}
	
	// Get final statistics
	blocksProcessed, bytesWritten := processor.GetStats()
	
	// Calculate total download time
	totalDownloadDuration := time.Since(downloadStartTime)
	
	// Get memory statistics
	if processor.memMonitor.enabled {
		current, peak, start := processor.memMonitor.GetStats()
		logger.Info("Download memory usage summary", map[string]interface{}{
			"start_mb":    start / (1024 * 1024),
			"peak_mb":     peak / (1024 * 1024),
			"current_mb":  current / (1024 * 1024),
			"increase_mb": (peak - start) / (1024 * 1024),
		})
	}
	
	// Log download completion
	logger.Info("Streaming download completed successfully", map[string]interface{}{
		"descriptor_cid":      descriptorCID,
		"output_file":         outputPath,
		"file_size":           descriptor.FileSize,
		"bytes_written":       bytesWritten,
		"blocks_processed":    blocksProcessed,
		"total_duration_ms":   totalDownloadDuration.Milliseconds(),
		"throughput_mb_per_s": float64(descriptor.FileSize) / (1024 * 1024) / totalDownloadDuration.Seconds(),
	})
	
	// Display results
	if jsonOutput {
		result := map[string]interface{}{
			"success":       true,
			"descriptorCID": descriptorCID,
			"outputFile":    outputPath,
			"fileSize":      descriptor.FileSize,
			"blocksCount":   blocksProcessed,
			"duration":      totalDownloadDuration.Seconds(),
		}
		util.PrintJSON(result)
	} else if !quiet {
		fmt.Println("\nStreaming download complete!")
		fmt.Printf("Output file: %s\n", outputPath)
		fmt.Printf("Performance: %.2f MB/s (total time: %.2fs)\n",
			float64(descriptor.FileSize)/(1024*1024)/totalDownloadDuration.Seconds(),
			totalDownloadDuration.Seconds())
		
		if processor.memMonitor.enabled {
			_, peak, start := processor.memMonitor.GetStats()
			fmt.Printf("Memory usage: Peak %.2f MB (increase: %.2f MB)\n",
				float64(peak)/(1024*1024),
				float64(peak-start)/(1024*1024))
		}
	}
	
	// Record download metrics
	client.RecordDownload()
	
	return nil
}