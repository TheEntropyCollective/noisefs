// Package streaming provides concrete implementations of streaming interfaces.
// This file implements the core streaming functionality using existing NoiseFS components.
package streaming

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/core/descriptors"
)

// streamerImpl provides a concrete implementation of the Streamer interface.
// It bridges the new streaming interfaces with existing NoiseFS functionality.
type streamerImpl struct {
	storage      StreamingStorage
	randomizer   RandomizerProvider
	assembler    BlockAssembler
	config       StreamingConfig
	metrics      *streamingMetrics
	closed       bool
	mu           sync.RWMutex
}

// streamingMetrics implements comprehensive metrics tracking.
type streamingMetrics struct {
	totalOperations      int64
	successfulOps        int64
	failedOps           int64
	cancelledOps        int64
	totalBytesProcessed int64
	avgThroughput       float64
	peakThroughput      float64
	avgOpDuration       time.Duration
	peakMemoryUsage     int64
	currentConcurrency  int
	errorRate           float64
	lastOpTime          time.Time
	mu                  sync.RWMutex
}

// NewStreamer creates a new streaming implementation with the provided components.
func NewStreamer(storage StreamingStorage, randomizer RandomizerProvider, assembler BlockAssembler, config StreamingConfig) (Streamer, error) {
	if storage == nil {
		return nil, fmt.Errorf("%w: storage is required", ErrInvalidOptions)
	}
	if randomizer == nil {
		return nil, fmt.Errorf("%w: randomizer provider is required", ErrInvalidOptions)
	}
	if assembler == nil {
		return nil, fmt.Errorf("%w: block assembler is required", ErrInvalidOptions)
	}
	if config == nil {
		return nil, fmt.Errorf("%w: configuration is required", ErrInvalidOptions)
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("%w: configuration validation failed: %v", ErrInvalidOptions, err)
	}

	return &streamerImpl{
		storage:    storage,
		randomizer: randomizer,
		assembler:  assembler,
		config:     config,
		metrics:    &streamingMetrics{},
	}, nil
}

// StreamUpload implements the Streamer interface for memory-efficient file uploads.
func (s *streamerImpl) StreamUpload(ctx context.Context, reader io.Reader, opts UploadOptions) (string, error) {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return "", ErrStreamerClosed
	}
	s.mu.RUnlock()

	// Update metrics
	s.updateMetrics(func(m *streamingMetrics) {
		m.totalOperations++
		m.currentConcurrency++
		m.lastOpTime = time.Now()
	})

	defer s.updateMetrics(func(m *streamingMetrics) {
		m.currentConcurrency--
	})

	startTime := time.Now()

	// Apply configuration defaults
	blockSize := opts.BlockSize
	if blockSize <= 0 {
		blockSize = s.config.GetBlockSize()
	}

	maxConcurrency := opts.MaxConcurrency
	if maxConcurrency <= 0 {
		maxConcurrency = s.config.GetMaxConcurrency()
	}

	// Apply timeout if specified
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	// Initialize progress reporter
	if opts.ProgressReporter != nil {
		opts.ProgressReporter.ReportProgress(ProgressInfo{
			Stage:         "Initializing upload",
			StartTime:     startTime,
			CurrentTime:   time.Now(),
		})
	}

	// Create streaming splitter
	splitter, err := blocks.NewStreamingSplitter(blockSize)
	if err != nil {
		s.updateMetrics(func(m *streamingMetrics) { m.failedOps++ })
		return "", &StreamingError{
			Operation:      "upload",
			Stage:          "initialization",
			Underlying:     err,
			Retryable:      false,
			RecoveryAction: "Check block size configuration",
		}
	}

	// Track upload state
	var (
		blockPairs     []descriptors.BlockPair
		totalBytes     int64
		blocksUpload   int
		mu             sync.Mutex
	)

	// Block processor for upload
	processor := &uploadProcessor{
		ctx:        ctx,
		storage:    s.storage,
		randomizer: s.randomizer,
		progress:   opts.ProgressReporter,
		blockPairs: &blockPairs,
		totalBytes: &totalBytes,
		blockCount: &blocksUpload,
		mu:         &mu,
		startTime:  startTime,
	}

	// Process file with streaming splitter
	err = splitter.SplitWithProgressAndContext(ctx, reader, processor, func(bytesProcessed int64, blocksProcessed int) {
		// Update total bytes with actual data processed (not block size)
		mu.Lock()
		totalBytes = bytesProcessed
		mu.Unlock()
		
		if opts.ProgressReporter != nil {
			opts.ProgressReporter.ReportProgress(ProgressInfo{
				Stage:           "Processing blocks",
				BytesProcessed:  bytesProcessed,
				BlocksProcessed: blocksProcessed,
				StartTime:       startTime,
				CurrentTime:     time.Now(),
				Throughput:      calculateThroughput(bytesProcessed, startTime),
			})
		}
	})

	if err != nil {
		s.updateMetrics(func(m *streamingMetrics) { 
			if ctx.Err() != nil {
				m.cancelledOps++
			} else {
				m.failedOps++
			}
		})
		return "", &StreamingError{
			Operation:      "upload",
			Stage:          "block_processing",
			Underlying:     err,
			Retryable:      true,
			RecoveryAction: "Retry upload with same parameters",
		}
	}

	// Create file descriptor
	descriptor := &descriptors.Descriptor{
		Version:        "4.0",
		Type:           descriptors.FileType,
		Filename:       opts.Filename,
		FileSize:       totalBytes,
		PaddedFileSize: int64(len(blockPairs) * blockSize),
		BlockSize:      blockSize,
		Blocks:         blockPairs,
		CreatedAt:      time.Now(),
	}

	// Store descriptor
	descriptorCID, err := s.storage.StoreDescriptor(ctx, descriptor, opts.EnableEncryption, opts.EncryptionPassword)
	if err != nil {
		s.updateMetrics(func(m *streamingMetrics) { m.failedOps++ })
		return "", &StreamingError{
			Operation:      "upload",
			Stage:          "descriptor_storage",
			Underlying:     err,
			Retryable:      true,
			RecoveryAction: "Retry descriptor storage",
		}
	}

	// Success metrics
	duration := time.Since(startTime)
	s.updateMetrics(func(m *streamingMetrics) {
		m.successfulOps++
		m.totalBytesProcessed += totalBytes
		m.avgOpDuration = updateAverage(m.avgOpDuration, duration, m.successfulOps)
		
		throughput := float64(totalBytes) / duration.Seconds()
		m.avgThroughput = updateThroughputAverage(m.avgThroughput, throughput, m.successfulOps)
		if throughput > m.peakThroughput {
			m.peakThroughput = throughput
		}
	})

	// Final progress report
	if opts.ProgressReporter != nil {
		opts.ProgressReporter.Complete(ProgressInfo{
			Stage:           "Upload complete",
			BytesProcessed:  totalBytes,
			BlocksProcessed: blocksUpload,
			TotalBytes:      totalBytes,
			TotalBlocks:     blocksUpload,
			StartTime:       startTime,
			CurrentTime:     time.Now(),
			Throughput:      float64(totalBytes) / duration.Seconds(),
		})
	}

	return descriptorCID, nil
}

// StreamDownload implements the Streamer interface for memory-efficient file downloads.
func (s *streamerImpl) StreamDownload(ctx context.Context, descriptorCID string, writer io.Writer, opts DownloadOptions) error {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return ErrStreamerClosed
	}
	s.mu.RUnlock()

	// Update metrics
	s.updateMetrics(func(m *streamingMetrics) {
		m.totalOperations++
		m.currentConcurrency++
		m.lastOpTime = time.Now()
	})

	defer s.updateMetrics(func(m *streamingMetrics) {
		m.currentConcurrency--
	})

	startTime := time.Now()

	// Apply timeout if specified
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	// Initialize progress reporter
	if opts.ProgressReporter != nil {
		opts.ProgressReporter.ReportProgress(ProgressInfo{
			Stage:       "Initializing download",
			StartTime:   startTime,
			CurrentTime: time.Now(),
		})
	}

	// Retrieve descriptor
	descriptor, err := s.storage.RetrieveDescriptor(ctx, descriptorCID, opts.DecryptionPassword)
	if err != nil {
		s.updateMetrics(func(m *streamingMetrics) { m.failedOps++ })
		return &StreamingError{
			Operation:      "download",
			Stage:          "descriptor_retrieval",
			Underlying:     err,
			Retryable:      true,
			RecoveryAction: "Verify descriptor CID and retry",
		}
	}

	// Initialize assembler
	err = s.assembler.Initialize(ctx, descriptor, writer)
	if err != nil {
		s.updateMetrics(func(m *streamingMetrics) { m.failedOps++ })
		return &StreamingError{
			Operation:      "download",
			Stage:          "assembler_initialization",
			Underlying:     err,
			Retryable:      false,
			RecoveryAction: "Check output writer and descriptor validity",
		}
	}

	// Set total for progress reporting
	if opts.ProgressReporter != nil {
		opts.ProgressReporter.SetTotal(descriptor.FileSize, len(descriptor.Blocks))
	}

	// Download and process blocks
	for i, blockPair := range descriptor.Blocks {
		select {
		case <-ctx.Done():
			s.updateMetrics(func(m *streamingMetrics) { m.cancelledOps++ })
			return ctx.Err()
		default:
		}

		// Retrieve anonymized block
		anonymizedBlock, err := s.storage.RetrieveBlock(ctx, blockPair.DataCID, "data")
		if err != nil {
			s.updateMetrics(func(m *streamingMetrics) { m.failedOps++ })
			return &StreamingError{
				Operation:      "download",
				Stage:          "block_retrieval",
				Underlying:     err,
				Retryable:      true,
				RecoveryAction: "Retry block retrieval",
				Context: map[string]interface{}{
					"block_index": i,
					"block_cid":   blockPair.DataCID,
				},
			}
		}

		// Retrieve randomizer blocks
		randomizer1, err := s.storage.RetrieveBlock(ctx, blockPair.RandomizerCID1, "randomizer")
		if err != nil {
			s.updateMetrics(func(m *streamingMetrics) { m.failedOps++ })
			return &StreamingError{
				Operation:      "download",
				Stage:          "randomizer_retrieval",
				Underlying:     err,
				Retryable:      true,
				RecoveryAction: "Retry randomizer retrieval",
			}
		}

		randomizer2, err := s.storage.RetrieveBlock(ctx, blockPair.RandomizerCID2, "randomizer")
		if err != nil {
			s.updateMetrics(func(m *streamingMetrics) { m.failedOps++ })
			return &StreamingError{
				Operation:      "download",
				Stage:          "randomizer_retrieval",
				Underlying:     err,
				Retryable:      true,
				RecoveryAction: "Retry randomizer retrieval",
			}
		}

		// Add block to assembler
		complete, err := s.assembler.AddBlock(ctx, i, anonymizedBlock, randomizer1, randomizer2)
		if err != nil {
			s.updateMetrics(func(m *streamingMetrics) { m.failedOps++ })
			return &StreamingError{
				Operation:      "download",
				Stage:          "block_assembly",
				Underlying:     err,
				Retryable:      false,
				RecoveryAction: "Check assembler state and restart download",
			}
		}

		// Report progress
		if opts.ProgressReporter != nil {
			progress := s.assembler.GetProgress()
			opts.ProgressReporter.ReportProgress(ProgressInfo{
				Stage:             "Downloading blocks",
				BytesProcessed:    progress.ProcessedBytes,
				TotalBytes:        descriptor.FileSize,
				BlocksProcessed:   progress.ProcessedBlocks,
				TotalBlocks:       len(descriptor.Blocks),
				CurrentBlockIndex: i,
				StartTime:         startTime,
				CurrentTime:       time.Now(),
				Throughput:        progress.Throughput,
			})
		}

		if complete {
			break
		}
	}

	// Success metrics
	duration := time.Since(startTime)
	s.updateMetrics(func(m *streamingMetrics) {
		m.successfulOps++
		m.totalBytesProcessed += descriptor.FileSize
		m.avgOpDuration = updateAverage(m.avgOpDuration, duration, m.successfulOps)
		
		throughput := float64(descriptor.FileSize) / duration.Seconds()
		m.avgThroughput = updateThroughputAverage(m.avgThroughput, throughput, m.successfulOps)
		if throughput > m.peakThroughput {
			m.peakThroughput = throughput
		}
	})

	// Final progress report
	if opts.ProgressReporter != nil {
		opts.ProgressReporter.Complete(ProgressInfo{
			Stage:           "Download complete",
			BytesProcessed:  descriptor.FileSize,
			BlocksProcessed: len(descriptor.Blocks),
			TotalBytes:      descriptor.FileSize,
			TotalBlocks:     len(descriptor.Blocks),
			StartTime:       startTime,
			CurrentTime:     time.Now(),
			Throughput:      float64(descriptor.FileSize) / duration.Seconds(),
		})
	}

	return nil
}

// GetMetrics returns current streaming operation metrics.
func (s *streamerImpl) GetMetrics() StreamingMetrics {
	s.metrics.mu.RLock()
	defer s.metrics.mu.RUnlock()

	// Update error rate
	if s.metrics.totalOperations > 0 {
		s.metrics.errorRate = float64(s.metrics.failedOps) / float64(s.metrics.totalOperations)
	}

	return StreamingMetrics{
		TotalOperations:          s.metrics.totalOperations,
		SuccessfulOperations:     s.metrics.successfulOps,
		FailedOperations:         s.metrics.failedOps,
		CancelledOperations:      s.metrics.cancelledOps,
		TotalBytesProcessed:      s.metrics.totalBytesProcessed,
		AverageThroughput:        s.metrics.avgThroughput,
		PeakThroughput:          s.metrics.peakThroughput,
		AverageOperationDuration: s.metrics.avgOpDuration,
		PeakMemoryUsage:         s.metrics.peakMemoryUsage,
		CurrentConcurrency:      s.metrics.currentConcurrency,
		ErrorRate:               s.metrics.errorRate,
		LastOperationTime:       s.metrics.lastOpTime,
	}
}

// Close gracefully shuts down the streamer and releases resources.
func (s *streamerImpl) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true

	// Close components
	var errs []error
	if err := s.storage.Close(); err != nil {
		errs = append(errs, fmt.Errorf("storage close error: %w", err))
	}
	if err := s.assembler.Close(); err != nil {
		errs = append(errs, fmt.Errorf("assembler close error: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}

	return nil
}

// uploadProcessor implements blocks.BlockProcessor for upload operations.
type uploadProcessor struct {
	ctx        context.Context
	storage    StreamingStorage
	randomizer RandomizerProvider
	progress   ProgressReporter
	blockPairs *[]descriptors.BlockPair
	totalBytes *int64
	blockCount *int
	mu         *sync.Mutex
	startTime  time.Time
}

// ProcessBlock implements the blocks.BlockProcessor interface.
func (p *uploadProcessor) ProcessBlock(blockIndex int, block *blocks.Block) error {
	// Get randomizers
	randomizer1, rand1CID, randomizer2, rand2CID, overhead, err := p.randomizer.SelectRandomizers(p.ctx, len(block.Data), "upload")
	if err != nil {
		return fmt.Errorf("randomizer selection failed: %w", err)
	}

	// Apply XOR anonymization
	anonymizedBlock, err := block.XOR(randomizer1, randomizer2)
	if err != nil {
		return fmt.Errorf("XOR anonymization failed: %w", err)
	}

	// Store anonymized block
	blockCID, err := p.storage.StoreBlock(p.ctx, anonymizedBlock, "data")
	if err != nil {
		return fmt.Errorf("block storage failed: %w", err)
	}

	// Update block pairs
	p.mu.Lock()
	*p.blockPairs = append(*p.blockPairs, descriptors.BlockPair{
		DataCID:        blockCID,
		RandomizerCID1: rand1CID,
		RandomizerCID2: rand2CID,
	})
	*p.blockCount++
	p.mu.Unlock()

	// Unused overhead tracking for now
	_ = overhead

	return nil
}

// Helper functions for metrics calculations.

func (s *streamerImpl) updateMetrics(update func(*streamingMetrics)) {
	s.metrics.mu.Lock()
	defer s.metrics.mu.Unlock()
	update(s.metrics)
}

func updateAverage(currentAvg time.Duration, newValue time.Duration, count int64) time.Duration {
	if count == 1 {
		return newValue
	}
	total := currentAvg*time.Duration(count-1) + newValue
	return total / time.Duration(count)
}

func updateThroughputAverage(currentAvg, newValue float64, count int64) float64 {
	if count == 1 {
		return newValue
	}
	return (currentAvg*float64(count-1) + newValue) / float64(count)
}

func calculateThroughput(bytes int64, startTime time.Time) float64 {
	duration := time.Since(startTime).Seconds()
	if duration <= 0 {
		return 0
	}
	return float64(bytes) / duration
}