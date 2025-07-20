# NoiseFS Streaming Interface Integration Guide

## Overview

This document provides comprehensive guidance for integrating the new streaming interfaces into NoiseFS applications and migrating from existing streaming implementations.

## Architecture Overview

The new streaming interface architecture consists of several layers:

```
┌─────────────────────────────────────────────────────┐
│                 Application Layer                    │
├─────────────────────────────────────────────────────┤
│  Streamer Interface (Upload/Download Operations)     │
├─────────────────────────────────────────────────────┤
│  Configuration Layer (StreamingConfig, Builder)      │
├─────────────────────────────────────────────────────┤
│  Processing Layer (BlockProcessor, ProcessorChain)   │
├─────────────────────────────────────────────────────┤
│  Storage Layer (StreamingStorage, RandomizerProvider)│
├─────────────────────────────────────────────────────┤
│  Assembly Layer (BlockAssembler)                     │
└─────────────────────────────────────────────────────┘
```

## Core Interfaces

### 1. Streamer Interface

The main interface for streaming operations:

```go
type Streamer interface {
    StreamUpload(ctx context.Context, reader io.Reader, opts UploadOptions) (string, error)
    StreamDownload(ctx context.Context, descriptorCID string, writer io.Writer, opts DownloadOptions) error
    GetMetrics() StreamingMetrics
    Close() error
}
```

### 2. Configuration Interface

Type-safe configuration with builder pattern:

```go
config := streaming.NewConfigBuilder().
    WithBlockSize(256 * 1024).
    WithMaxConcurrency(8).
    WithTimeout(30 * time.Minute).
    WithProgressReporter(myReporter).
    BuildWithDefaults()
```

### 3. Progress Reporting

Standardized progress reporting across all operations:

```go
type ProgressReporter interface {
    ReportProgress(info ProgressInfo)
    ReportError(err error, context string)
    SetTotal(totalBytes int64, totalBlocks int)
    Complete(finalInfo ProgressInfo)
    Cancel(reason string)
}
```

## Integration Patterns

### Basic Upload Example

```go
package main

import (
    "context"
    "os"
    "time"
    
    "github.com/TheEntropyCollective/noisefs/pkg/core/streaming"
)

func uploadFile(streamer streaming.Streamer, filename string) error {
    // Open file
    file, err := os.Open(filename)
    if err != nil {
        return err
    }
    defer file.Close()

    // Create configuration
    config := streaming.NewConfigBuilder().
        WithBlockSize(128 * 1024).
        WithTimeout(10 * time.Minute).
        WithValidationLevel(streaming.ValidationStandard).
        BuildWithDefaults()

    // Create upload options
    opts := streaming.UploadOptions{
        Filename:         filename,
        BlockSize:        config.GetBlockSize(),
        MaxConcurrency:   config.GetMaxConcurrency(),
        Timeout:          config.GetTimeout(),
        ValidationLevel:  config.GetValidationLevel(),
    }

    // Perform upload
    ctx := context.Background()
    descriptorCID, err := streamer.StreamUpload(ctx, file, opts)
    if err != nil {
        return err
    }

    fmt.Printf("Upload complete: %s\n", descriptorCID)
    return nil
}
```

### Progress Reporting Example

```go
type MyProgressReporter struct {
    filename string
}

func (r *MyProgressReporter) ReportProgress(info streaming.ProgressInfo) {
    percentage := float64(info.BytesProcessed) / float64(info.TotalBytes) * 100
    fmt.Printf("Uploading %s: %.1f%% (%s)\n", 
        r.filename, percentage, info.Stage)
}

func (r *MyProgressReporter) ReportError(err error, context string) {
    fmt.Printf("Error in %s: %v\n", context, err)
}

func (r *MyProgressReporter) SetTotal(totalBytes int64, totalBlocks int) {
    fmt.Printf("Starting upload: %d bytes, %d blocks\n", totalBytes, totalBlocks)
}

func (r *MyProgressReporter) Complete(finalInfo streaming.ProgressInfo) {
    duration := finalInfo.CurrentTime.Sub(finalInfo.StartTime)
    fmt.Printf("Upload complete in %v (%.2f MB/s)\n", 
        duration, finalInfo.Throughput/1024/1024)
}

func (r *MyProgressReporter) Cancel(reason string) {
    fmt.Printf("Upload cancelled: %s\n", reason)
}
```

### Download with Assembly Example

```go
func downloadFile(streamer streaming.Streamer, descriptorCID string, outputPath string) error {
    // Create output file
    output, err := os.Create(outputPath)
    if err != nil {
        return err
    }
    defer output.Close()

    // Create progress reporter
    reporter := &MyProgressReporter{filename: outputPath}

    // Create download options
    opts := streaming.DownloadOptions{
        MaxConcurrency:  runtime.NumCPU(),
        ProgressReporter: reporter,
        Timeout:         15 * time.Minute,
        VerifyIntegrity: true,
        PreferCached:    true,
    }

    // Perform download
    ctx := context.Background()
    err = streamer.StreamDownload(ctx, descriptorCID, output, opts)
    if err != nil {
        return err
    }

    return nil
}
```

### Custom Block Processor Example

```go
type CompressionProcessor struct {
    name string
    metrics streaming.ProcessorMetrics
}

func (p *CompressionProcessor) ProcessBlock(ctx context.Context, blockIndex int, block *blocks.Block) (*blocks.Block, error) {
    start := time.Now()
    defer func() {
        p.metrics.AverageProcessingTime = time.Since(start)
        p.metrics.BlocksProcessed++
    }()

    // Check context cancellation
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }

    // Compress block data
    compressed, err := compress(block.Data())
    if err != nil {
        p.metrics.ErrorCount++
        return nil, fmt.Errorf("compression failed: %w", err)
    }

    // Create new block with compressed data
    compressedBlock, err := blocks.NewBlock(compressed)
    if err != nil {
        p.metrics.ErrorCount++
        return nil, fmt.Errorf("failed to create compressed block: %w", err)
    }

    return compressedBlock, nil
}

func (p *CompressionProcessor) GetName() string {
    return p.name
}

func (p *CompressionProcessor) CanProcess(block *blocks.Block) bool {
    // Can process any block
    return true
}

func (p *CompressionProcessor) GetMetrics() streaming.ProcessorMetrics {
    return p.metrics
}
```

## Migration Guide

### From Current Implementation

The existing streaming implementation in `pkg/core/blocks/streaming.go` can be migrated to the new interfaces gradually:

#### Phase 1: Interface Wrapper

Create a wrapper that implements the new `Streamer` interface using existing code:

```go
type LegacyStreamerWrapper struct {
    client *noisefs.Client
}

func (w *LegacyStreamerWrapper) StreamUpload(ctx context.Context, reader io.Reader, opts streaming.UploadOptions) (string, error) {
    // Convert opts to legacy progress callback
    var progress noisefs.StreamingProgressCallback
    if opts.ProgressReporter != nil {
        progress = func(stage string, bytesProcessed int64, blocksProcessed int) {
            opts.ProgressReporter.ReportProgress(streaming.ProgressInfo{
                Stage:           stage,
                BytesProcessed:  bytesProcessed,
                BlocksProcessed: blocksProcessed,
                CurrentTime:     time.Now(),
            })
        }
    }

    // Call legacy method
    return w.client.StreamingUploadWithContextAndProgress(
        ctx, reader, opts.Filename, opts.BlockSize, progress)
}
```

#### Phase 2: Gradual Replacement

Replace components incrementally:

1. **Configuration First**: Replace manual parameter passing with configuration objects
2. **Progress Reporting**: Standardize progress callbacks
3. **Storage Layer**: Abstract storage operations behind interfaces
4. **Processing Chain**: Implement composable block processing
5. **Assembly**: Add out-of-order block assembly support

#### Phase 3: Full Migration

Remove legacy code and use native interface implementations.

### Breaking Changes

The new interfaces introduce several breaking changes:

1. **Method Signatures**: All methods now require `context.Context` as first parameter
2. **Configuration**: Options are now passed as structured types instead of individual parameters
3. **Progress Reporting**: New standardized progress interface replaces function callbacks
4. **Error Handling**: Structured error types with unwrapping support

### Compatibility Layer

For applications that need gradual migration:

```go
// Compatibility function that wraps new interface with old signature
func LegacyStreamingUpload(client *noisefs.Client, reader io.Reader, filename string) (string, error) {
    streamer := NewStreamerFromClient(client)
    opts := streaming.UploadOptions{
        Filename:  filename,
        BlockSize: blocks.DefaultBlockSize,
    }
    return streamer.StreamUpload(context.Background(), reader, opts)
}
```

## Testing Integration

### Mock Implementations

The interfaces are designed for easy mocking:

```go
type MockStreamer struct {
    uploadFunc   func(context.Context, io.Reader, streaming.UploadOptions) (string, error)
    downloadFunc func(context.Context, string, io.Writer, streaming.DownloadOptions) error
    metrics      streaming.StreamingMetrics
}

func (m *MockStreamer) StreamUpload(ctx context.Context, reader io.Reader, opts streaming.UploadOptions) (string, error) {
    if m.uploadFunc != nil {
        return m.uploadFunc(ctx, reader, opts)
    }
    return "mock-cid", nil
}

func (m *MockStreamer) StreamDownload(ctx context.Context, descriptorCID string, writer io.Writer, opts streaming.DownloadOptions) error {
    if m.downloadFunc != nil {
        return m.downloadFunc(ctx, descriptorCID, writer, opts)
    }
    _, err := writer.Write([]byte("mock data"))
    return err
}

func (m *MockStreamer) GetMetrics() streaming.StreamingMetrics {
    return m.metrics
}

func (m *MockStreamer) Close() error {
    return nil
}
```

### Test Utilities

Convenience functions for testing:

```go
func TestStreamingUpload(t *testing.T) {
    // Create test data
    data := []byte("test file content")
    reader := bytes.NewReader(data)

    // Create mock streamer
    streamer := &MockStreamer{
        uploadFunc: func(ctx context.Context, r io.Reader, opts streaming.UploadOptions) (string, error) {
            // Verify options
            assert.Equal(t, "test.txt", opts.Filename)
            assert.Equal(t, 128*1024, opts.BlockSize)
            
            // Read and verify data
            uploadedData, err := io.ReadAll(r)
            require.NoError(t, err)
            assert.Equal(t, data, uploadedData)
            
            return "test-cid", nil
        },
    }

    // Test upload
    opts := streaming.UploadOptions{
        Filename:  "test.txt",
        BlockSize: 128 * 1024,
    }
    
    cid, err := streamer.StreamUpload(context.Background(), reader, opts)
    require.NoError(t, err)
    assert.Equal(t, "test-cid", cid)
}
```

## Performance Considerations

### Configuration Tuning

For optimal performance, consider these configuration parameters:

```go
// High-throughput configuration
config := streaming.NewConfigBuilder().
    WithBlockSize(512 * 1024).      // Larger blocks for better throughput
    WithMaxConcurrency(16).          // Higher concurrency for I/O bound operations
    WithBufferSize(256 * 1024).      // Larger buffers for better I/O efficiency
    WithValidationLevel(streaming.ValidationBasic). // Minimal validation for speed
    BuildWithDefaults()

// Low-latency configuration
config := streaming.NewConfigBuilder().
    WithBlockSize(64 * 1024).       // Smaller blocks for lower latency
    WithMaxConcurrency(4).           // Moderate concurrency
    WithBufferSize(32 * 1024).       // Smaller buffers for lower memory usage
    WithValidationLevel(streaming.ValidationStrict). // Full validation for safety
    BuildWithDefaults()
```

### Memory Management

The interfaces are designed for constant memory usage:

- Block processing is streaming-based
- Assembly uses bounded buffers
- Configuration is immutable
- Progress reporting is non-blocking

### Monitoring and Observability

Use metrics for performance monitoring:

```go
func monitorStreaming(streamer streaming.Streamer) {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()

    for range ticker.C {
        metrics := streamer.GetMetrics()
        
        fmt.Printf("Streaming Metrics:\n")
        fmt.Printf("  Operations: %d total, %d successful, %d failed\n",
            metrics.TotalOperations, metrics.SuccessfulOperations, metrics.FailedOperations)
        fmt.Printf("  Throughput: %.2f MB/s average, %.2f MB/s peak\n",
            metrics.AverageThroughput/1024/1024, metrics.PeakThroughput/1024/1024)
        fmt.Printf("  Error Rate: %.2f%%\n", metrics.ErrorRate*100)
    }
}
```

## Error Handling

### Structured Errors

The new interfaces use structured error types:

```go
func handleStreamingError(err error) {
    var streamingErr *streaming.StreamingError
    if errors.As(err, &streamingErr) {
        fmt.Printf("Operation: %s, Stage: %s\n", 
            streamingErr.Operation, streamingErr.Stage)
        
        if streamingErr.Retryable {
            fmt.Printf("Error is retryable: %s\n", streamingErr.RecoveryAction)
        }
        
        // Unwrap to get original error
        if originalErr := errors.Unwrap(err); originalErr != nil {
            fmt.Printf("Original error: %v\n", originalErr)
        }
    }
    
    // Check for context errors
    if errors.Is(err, context.Canceled) {
        fmt.Println("Operation was cancelled")
    } else if errors.Is(err, context.DeadlineExceeded) {
        fmt.Println("Operation timed out")
    }
}
```

### Retry Strategies

Implement retry logic using the retry policy:

```go
retryPolicy := &streaming.RetryPolicy{
    MaxAttempts:       3,
    InitialDelay:      time.Second,
    MaxDelay:          30 * time.Second,
    BackoffMultiplier: 2.0,
    ShouldRetry: func(err error, attempt int) bool {
        // Don't retry context errors
        if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
            return false
        }
        
        // Don't retry validation errors
        if errors.Is(err, streaming.ErrValidationFailed) {
            return false
        }
        
        // Retry other errors up to max attempts
        return attempt < 3
    },
}

opts := streaming.UploadOptions{
    Filename:    "test.txt",
    RetryPolicy: retryPolicy,
}
```

## Best Practices

### 1. Always Use Context

```go
// Good: Proper context usage with timeout
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
defer cancel()

cid, err := streamer.StreamUpload(ctx, reader, opts)
```

### 2. Configure Appropriately

```go
// Good: Appropriate configuration for use case
config := streaming.NewConfigBuilder().
    WithBlockSize(getOptimalBlockSize(fileSize)).
    WithMaxConcurrency(getConcurrencyForNetwork()).
    WithTimeout(getTimeoutForFileSize(fileSize)).
    BuildWithDefaults()
```

### 3. Handle Progress Appropriately

```go
// Good: Non-blocking progress reporting
type AsyncProgressReporter struct {
    updates chan streaming.ProgressInfo
}

func (r *AsyncProgressReporter) ReportProgress(info streaming.ProgressInfo) {
    select {
    case r.updates <- info:
    default:
        // Don't block if channel is full
    }
}
```

### 4. Proper Resource Management

```go
// Good: Proper cleanup
streamer := NewStreamer(config)
defer func() {
    if err := streamer.Close(); err != nil {
        log.Printf("Error closing streamer: %v", err)
    }
}()
```

### 5. Error Handling

```go
// Good: Comprehensive error handling
cid, err := streamer.StreamUpload(ctx, reader, opts)
if err != nil {
    handleStreamingError(err)
    
    // Check for retryable errors
    var streamingErr *streaming.StreamingError
    if errors.As(err, &streamingErr) && streamingErr.Retryable {
        // Implement retry logic
        return retryUpload(ctx, reader, opts)
    }
    
    return err
}
```

This integration guide provides comprehensive guidance for adopting the new streaming interfaces while maintaining compatibility with existing NoiseFS applications.