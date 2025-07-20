// Package streaming provides data types and structures for streaming operations.
// This file defines the supporting types, configuration options, metrics,
// and error types used by the streaming interfaces.
package streaming

import (
	"errors"
	"time"
)

// UploadOptions configures streaming upload operations with type-safe parameters.
// Provides comprehensive configuration for upload behavior, performance tuning,
// and operational requirements.
type UploadOptions struct {
	// Filename is the logical name for the uploaded file.
	// Used in the descriptor metadata and for progress reporting.
	// Must be non-empty for valid upload operations.
	Filename string

	// BlockSize specifies the size in bytes for file splitting.
	// Affects memory usage, network efficiency, and cache performance.
	// Must be positive. If 0, uses default block size (128 KiB).
	BlockSize int

	// MaxConcurrency limits the number of concurrent block operations.
	// Controls resource usage and prevents overwhelming storage backends.
	// Must be positive. If 0, uses default concurrency (runtime.NumCPU()).
	MaxConcurrency int

	// ProgressReporter receives real-time progress updates during upload.
	// Enables user interface integration and operation monitoring.
	// May be nil to disable progress reporting.
	ProgressReporter ProgressReporter

	// Timeout specifies the maximum duration for the upload operation.
	// Operation will be cancelled with context.DeadlineExceeded if exceeded.
	// Zero value means no timeout.
	Timeout time.Duration

	// BufferSize specifies the internal buffer size for streaming operations.
	// Affects memory usage and I/O efficiency during data processing.
	// Must be positive. If 0, uses default buffer size (64 KiB).
	BufferSize int

	// EnableEncryption determines whether descriptor encryption is enabled.
	// When true, file descriptors are encrypted before storage for enhanced privacy.
	EnableEncryption bool

	// EncryptionPassword provides the password for descriptor encryption.
	// Required when EnableEncryption is true. Must be non-empty for encrypted uploads.
	EncryptionPassword string

	// RetryPolicy configures retry behavior for failed operations.
	// Specifies retry attempts, backoff strategy, and retry conditions.
	// May be nil to disable retries.
	RetryPolicy *RetryPolicy

	// ValidationLevel specifies the level of data validation to perform.
	// Higher levels provide more integrity checking at the cost of performance.
	ValidationLevel ValidationLevel

	// Tags provides metadata tags for the upload operation.
	// Used for categorization, search, and operational tracking.
	// May be empty if no tags are needed.
	Tags map[string]string
}

// DownloadOptions configures streaming download operations with type-safe parameters.
// Provides comprehensive configuration for download behavior, performance tuning,
// and operational requirements.
type DownloadOptions struct {
	// MaxConcurrency limits the number of concurrent block retrieval operations.
	// Controls resource usage and prevents overwhelming storage backends.
	// Must be positive. If 0, uses default concurrency (runtime.NumCPU()).
	MaxConcurrency int

	// ProgressReporter receives real-time progress updates during download.
	// Enables user interface integration and operation monitoring.
	// May be nil to disable progress reporting.
	ProgressReporter ProgressReporter

	// Timeout specifies the maximum duration for the download operation.
	// Operation will be cancelled with context.DeadlineExceeded if exceeded.
	// Zero value means no timeout.
	Timeout time.Duration

	// BufferSize specifies the internal buffer size for streaming operations.
	// Affects memory usage and I/O efficiency during data processing.
	// Must be positive. If 0, uses default buffer size (64 KiB).
	BufferSize int

	// DecryptionPassword provides the password for descriptor decryption.
	// Required for encrypted descriptors. If descriptor is encrypted but password
	// is empty, download will fail with ErrDecryptionRequired.
	DecryptionPassword string

	// RetryPolicy configures retry behavior for failed block retrieval operations.
	// Specifies retry attempts, backoff strategy, and retry conditions.
	// May be nil to disable retries.
	RetryPolicy *RetryPolicy

	// ValidationLevel specifies the level of data validation to perform.
	// Higher levels provide more integrity checking at the cost of performance.
	ValidationLevel ValidationLevel

	// VerifyIntegrity determines whether to verify block integrity during download.
	// When enabled, performs cryptographic verification of retrieved blocks.
	VerifyIntegrity bool

	// PreferCached specifies whether to prefer cached blocks over fresh retrieval.
	// When true, uses cached blocks when available to improve performance.
	PreferCached bool
}

// ProgressInfo contains comprehensive progress information for streaming operations.
// Provides detailed metrics for user interface updates, monitoring, and observability.
type ProgressInfo struct {
	// Stage describes the current operation stage in human-readable format.
	// Examples: "Initializing", "Processing blocks", "Saving descriptor", "Complete"
	Stage string

	// BytesProcessed indicates the total bytes processed so far.
	// Used for throughput calculation and completion percentage.
	BytesProcessed int64

	// TotalBytes indicates the total expected bytes to process.
	// May be 0 if total size is unknown (streaming from network).
	TotalBytes int64

	// BlocksProcessed indicates the number of blocks completed.
	// Used for progress percentage and block-level monitoring.
	BlocksProcessed int

	// TotalBlocks indicates the total expected blocks to process.
	// May be 0 if total count is unknown.
	TotalBlocks int

	// StartTime records when the operation began.
	// Used for elapsed time calculation and ETA estimation.
	StartTime time.Time

	// CurrentTime records when this progress update was generated.
	// Used for real-time throughput calculation.
	CurrentTime time.Time

	// Throughput indicates the current processing rate in bytes per second.
	// Calculated based on recent processing activity.
	Throughput float64

	// ETA estimates the time remaining to completion.
	// Based on current throughput and remaining work.
	// May be 0 if ETA cannot be calculated.
	ETA time.Duration

	// ErrorCount indicates the number of recoverable errors encountered.
	// Used for error rate monitoring and operation health assessment.
	ErrorCount int

	// CurrentBlockIndex indicates the index of the block currently being processed.
	// Used for detailed progress tracking and debugging.
	CurrentBlockIndex int

	// AdditionalInfo provides operation-specific additional information.
	// Used for detailed status updates and debugging information.
	AdditionalInfo map[string]interface{}
}

// StreamingMetrics provides comprehensive metrics for streaming operations.
// Enables monitoring, performance analysis, and capacity planning.
type StreamingMetrics struct {
	// TotalOperations counts the total number of streaming operations performed.
	TotalOperations int64

	// SuccessfulOperations counts operations that completed successfully.
	SuccessfulOperations int64

	// FailedOperations counts operations that failed with errors.
	FailedOperations int64

	// CancelledOperations counts operations that were cancelled.
	CancelledOperations int64

	// TotalBytesProcessed indicates the cumulative bytes processed across all operations.
	TotalBytesProcessed int64

	// AverageThroughput indicates the average processing rate in bytes per second.
	AverageThroughput float64

	// PeakThroughput indicates the highest recorded throughput.
	PeakThroughput float64

	// AverageOperationDuration indicates the mean time per operation.
	AverageOperationDuration time.Duration

	// PeakMemoryUsage indicates the highest recorded memory usage.
	PeakMemoryUsage int64

	// CurrentConcurrency indicates the number of currently active operations.
	CurrentConcurrency int

	// ErrorRate indicates the percentage of operations that failed.
	ErrorRate float64

	// LastOperationTime records when the most recent operation occurred.
	LastOperationTime time.Time
}

// ProcessorMetrics provides performance metrics for individual block processors.
// Enables monitoring of processor-specific performance and resource utilization.
type ProcessorMetrics struct {
	// ProcessorName identifies the processor these metrics belong to.
	ProcessorName string

	// BlocksProcessed counts the total number of blocks processed.
	BlocksProcessed int64

	// AverageProcessingTime indicates the mean time per block.
	AverageProcessingTime time.Duration

	// PeakProcessingTime indicates the longest block processing time.
	PeakProcessingTime time.Duration

	// ErrorCount counts the number of processing errors encountered.
	ErrorCount int64

	// SuccessRate indicates the percentage of successful block processing operations.
	SuccessRate float64

	// TotalProcessingTime indicates the cumulative time spent processing blocks.
	TotalProcessingTime time.Duration
}

// RetryPolicy configures retry behavior for failed streaming operations.
// Provides flexible retry strategies with backoff and condition controls.
type RetryPolicy struct {
	// MaxAttempts specifies the maximum number of retry attempts.
	// Must be positive. Total attempts = original + MaxAttempts.
	MaxAttempts int

	// InitialDelay specifies the delay before the first retry attempt.
	InitialDelay time.Duration

	// MaxDelay specifies the maximum delay between retry attempts.
	// Prevents exponential backoff from becoming too large.
	MaxDelay time.Duration

	// BackoffMultiplier specifies the multiplier for exponential backoff.
	// Each retry delay = previous delay * BackoffMultiplier.
	// Must be >= 1.0. Common values: 1.5, 2.0.
	BackoffMultiplier float64

	// RetryableErrors specifies which error types should trigger retries.
	// Only errors matching these conditions will be retried.
	// If empty, all non-context errors are considered retryable.
	RetryableErrors []error

	// ShouldRetry provides custom logic for determining retry eligibility.
	// If provided, this function is called to determine if an error should be retried.
	// Takes precedence over RetryableErrors if both are specified.
	ShouldRetry func(error, int) bool
}

// ValidationLevel specifies the level of data validation to perform during streaming operations.
type ValidationLevel int

const (
	// ValidationNone disables data validation for maximum performance.
	// Suitable for trusted environments with high performance requirements.
	ValidationNone ValidationLevel = iota

	// ValidationBasic performs basic size and format validation.
	// Provides minimal validation with low performance impact.
	ValidationBasic

	// ValidationStandard performs comprehensive validation including checksums.
	// Recommended for most production environments.
	ValidationStandard

	// ValidationStrict performs all available validation including cryptographic verification.
	// Provides maximum data integrity at the cost of performance.
	ValidationStrict
)

// String returns a human-readable representation of the validation level.
func (vl ValidationLevel) String() string {
	switch vl {
	case ValidationNone:
		return "none"
	case ValidationBasic:
		return "basic"
	case ValidationStandard:
		return "standard"
	case ValidationStrict:
		return "strict"
	default:
		return "unknown"
	}
}

// Common streaming operation errors with structured error types.
var (
	// ErrInvalidOptions indicates that the provided configuration options are invalid.
	ErrInvalidOptions = errors.New("invalid streaming options")

	// ErrStreamingFailed indicates a general streaming operation failure.
	ErrStreamingFailed = errors.New("streaming operation failed")

	// ErrDescriptorNotFound indicates the requested descriptor was not found in storage.
	ErrDescriptorNotFound = errors.New("descriptor not found")

	// ErrBlockRetrievalFailed indicates failure to retrieve required blocks from storage.
	ErrBlockRetrievalFailed = errors.New("block retrieval failed")

	// ErrDeAnonymizationFailed indicates failure in XOR de-anonymization process.
	ErrDeAnonymizationFailed = errors.New("de-anonymization failed")

	// ErrBlockProcessingFailed indicates failure in block processing operation.
	ErrBlockProcessingFailed = errors.New("block processing failed")

	// ErrInvalidBlock indicates that block data is invalid or corrupted.
	ErrInvalidBlock = errors.New("invalid block data")

	// ErrStreamerClosed indicates that operations were attempted on a closed streamer.
	ErrStreamerClosed = errors.New("streamer is closed")

	// ErrDecryptionRequired indicates that decryption password is required but not provided.
	ErrDecryptionRequired = errors.New("decryption password required")

	// ErrValidationFailed indicates that data validation checks failed.
	ErrValidationFailed = errors.New("validation failed")

	// ErrRetryExhausted indicates that all retry attempts have been exhausted.
	ErrRetryExhausted = errors.New("retry attempts exhausted")
)

// StreamingError provides structured error information for streaming operations.
// Enables detailed error reporting with context and recovery suggestions.
type StreamingError struct {
	// Operation describes the operation that failed (e.g., "upload", "download", "block_processing").
	Operation string

	// Stage describes the stage where the error occurred (e.g., "initialization", "processing", "finalization").
	Stage string

	// Underlying contains the original error that caused this failure.
	Underlying error

	// Context provides additional context about the error occurrence.
	Context map[string]interface{}

	// Retryable indicates whether this error might succeed if retried.
	Retryable bool

	// RecoveryAction suggests possible recovery actions for this error.
	RecoveryAction string
}

// Error implements the error interface for StreamingError.
func (se *StreamingError) Error() string {
	if se.Underlying != nil {
		return se.Operation + " " + se.Stage + " failed: " + se.Underlying.Error()
	}
	return se.Operation + " " + se.Stage + " failed"
}

// Unwrap enables error unwrapping for StreamingError.
func (se *StreamingError) Unwrap() error {
	return se.Underlying
}

// Is enables error comparison for StreamingError.
func (se *StreamingError) Is(target error) bool {
	if se.Underlying != nil {
		return errors.Is(se.Underlying, target)
	}
	return false
}