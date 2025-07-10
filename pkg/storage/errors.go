package storage

import (
	"fmt"
	"net"
	"context"
	"strings"
	"time"
)

// ErrorClassifier helps categorize and understand storage errors
type ErrorClassifier struct {
	backendType string
}

// NewErrorClassifier creates a new error classifier for a backend type
func NewErrorClassifier(backendType string) *ErrorClassifier {
	return &ErrorClassifier{backendType: backendType}
}

// ClassifyError analyzes an error and returns a standardized StorageError
func (ec *ErrorClassifier) ClassifyError(err error, operation string, address *BlockAddress) *StorageError {
	if err == nil {
		return nil
	}
	
	// Check if it's already a StorageError
	if storageErr, ok := err.(*StorageError); ok {
		return storageErr
	}
	
	// Classify common error types
	switch {
	case isNotFoundError(err):
		return &StorageError{
			Code:        ErrCodeNotFound,
			Message:     fmt.Sprintf("%s: block not found", operation),
			BackendType: ec.backendType,
			Address:     address,
			Cause:       err,
			Metadata:    map[string]interface{}{"operation": operation},
		}
		
	case isConnectionError(err):
		return &StorageError{
			Code:        ErrCodeConnectionFailed,
			Message:     fmt.Sprintf("%s: connection failed", operation),
			BackendType: ec.backendType,
			Address:     address,
			Cause:       err,
			Metadata:    map[string]interface{}{"operation": operation},
		}
		
	case isTimeoutError(err):
		return &StorageError{
			Code:        ErrCodeTimeout,
			Message:     fmt.Sprintf("%s: operation timed out", operation),
			BackendType: ec.backendType,
			Address:     address,
			Cause:       err,
			Metadata:    map[string]interface{}{"operation": operation},
		}
		
	case isQuotaError(err):
		return &StorageError{
			Code:        ErrCodeQuotaExceeded,
			Message:     fmt.Sprintf("%s: storage quota exceeded", operation),
			BackendType: ec.backendType,
			Address:     address,
			Cause:       err,
			Metadata:    map[string]interface{}{"operation": operation},
		}
		
	case isAuthError(err):
		return &StorageError{
			Code:        ErrCodeUnauthorized,
			Message:     fmt.Sprintf("%s: authentication failed", operation),
			BackendType: ec.backendType,
			Address:     address,
			Cause:       err,
			Metadata:    map[string]interface{}{"operation": operation},
		}
		
	case isIntegrityError(err):
		return &StorageError{
			Code:        ErrCodeIntegrityFailure,
			Message:     fmt.Sprintf("%s: data integrity check failed", operation),
			BackendType: ec.backendType,
			Address:     address,
			Cause:       err,
			Metadata:    map[string]interface{}{"operation": operation},
		}
		
	default:
		// Generic error
		return &StorageError{
			Code:        "UNKNOWN_ERROR",
			Message:     fmt.Sprintf("%s: %s", operation, err.Error()),
			BackendType: ec.backendType,
			Address:     address,
			Cause:       err,
			Metadata:    map[string]interface{}{"operation": operation},
		}
	}
}

// Error detection helpers
func isNotFoundError(err error) bool {
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "not found") ||
		   strings.Contains(errStr, "no such") ||
		   strings.Contains(errStr, "does not exist") ||
		   strings.Contains(errStr, "404")
}

func isConnectionError(err error) bool {
	// Check for network errors
	if _, ok := err.(net.Error); ok {
		return true
	}
	
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "connection") ||
		   strings.Contains(errStr, "dial") ||
		   strings.Contains(errStr, "connect") ||
		   strings.Contains(errStr, "network") ||
		   strings.Contains(errStr, "unreachable")
}

func isTimeoutError(err error) bool {
	// Check for timeout errors
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return true
	}
	
	// Check for context deadline exceeded
	if err == context.DeadlineExceeded {
		return true
	}
	
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "timeout") ||
		   strings.Contains(errStr, "deadline") ||
		   strings.Contains(errStr, "context deadline exceeded")
}

func isQuotaError(err error) bool {
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "quota") ||
		   strings.Contains(errStr, "limit") ||
		   strings.Contains(errStr, "space") ||
		   strings.Contains(errStr, "storage full") ||
		   strings.Contains(errStr, "insufficient")
}

func isAuthError(err error) bool {
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "unauthorized") ||
		   strings.Contains(errStr, "authentication") ||
		   strings.Contains(errStr, "permission") ||
		   strings.Contains(errStr, "forbidden") ||
		   strings.Contains(errStr, "401") ||
		   strings.Contains(errStr, "403")
}

func isIntegrityError(err error) bool {
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "checksum") ||
		   strings.Contains(errStr, "hash") ||
		   strings.Contains(errStr, "integrity") ||
		   strings.Contains(errStr, "corrupt") ||
		   strings.Contains(errStr, "verification")
}

// ErrorAggregator collects and analyzes multiple errors from different backends
type ErrorAggregator struct {
	errors []error
	operation string
}

// NewErrorAggregator creates a new error aggregator
func NewErrorAggregator(operation string) *ErrorAggregator {
	return &ErrorAggregator{
		errors: make([]error, 0),
		operation: operation,
	}
}

// Add adds an error to the aggregator
func (ea *ErrorAggregator) Add(err error) {
	if err != nil {
		ea.errors = append(ea.errors, err)
	}
}

// HasErrors returns true if any errors were collected
func (ea *ErrorAggregator) HasErrors() bool {
	return len(ea.errors) > 0
}

// GetPrimaryError returns the most significant error
func (ea *ErrorAggregator) GetPrimaryError() error {
	if len(ea.errors) == 0 {
		return nil
	}
	
	// Prioritize certain error types
	for _, err := range ea.errors {
		if storageErr, ok := err.(*StorageError); ok {
			switch storageErr.Code {
			case ErrCodeConnectionFailed, ErrCodeBackendOffline:
				return err // High priority
			}
		}
	}
	
	// Return the first error if no high-priority errors found
	return ea.errors[0]
}

// GetAllErrors returns all collected errors
func (ea *ErrorAggregator) GetAllErrors() []error {
	return ea.errors
}

// CreateAggregateError creates a single error that represents all collected errors
func (ea *ErrorAggregator) CreateAggregateError() error {
	if len(ea.errors) == 0 {
		return nil
	}
	
	if len(ea.errors) == 1 {
		return ea.errors[0]
	}
	
	// Create an aggregate error
	var backendTypes []string
	var messages []string
	
	for _, err := range ea.errors {
		if storageErr, ok := err.(*StorageError); ok {
			backendTypes = append(backendTypes, storageErr.BackendType)
			messages = append(messages, storageErr.Message)
		} else {
			messages = append(messages, err.Error())
		}
	}
	
	return &StorageError{
		Code:        "AGGREGATE_ERROR",
		Message:     fmt.Sprintf("%s failed on multiple backends: %s", ea.operation, strings.Join(messages, "; ")),
		BackendType: strings.Join(backendTypes, ","),
		Cause:       ea.errors[0],
		Metadata: map[string]interface{}{
			"operation": ea.operation,
			"error_count": len(ea.errors),
			"backend_types": backendTypes,
		},
	}
}

// RetryableError wraps an error with retry information
type RetryableError struct {
	*StorageError
	RetryAfter    time.Duration
	MaxRetries    int
	CurrentRetry  int
}

// IsRetryable returns true if the error should be retried
func (re *RetryableError) IsRetryable() bool {
	return re.CurrentRetry < re.MaxRetries
}

// NextRetryDelay returns the delay before the next retry
func (re *RetryableError) NextRetryDelay() time.Duration {
	return re.RetryAfter
}

// CreateRetryableError creates a retryable error based on the error type
func CreateRetryableError(err *StorageError, config *RetryConfig) *RetryableError {
	if !isRetryableErrorCode(err.Code) {
		return &RetryableError{
			StorageError: err,
			MaxRetries:   0, // Not retryable
		}
	}
	
	retryAfter := config.BaseDelay
	if err.Code == ErrCodeTimeout {
		retryAfter = config.BaseDelay * 2 // Longer delay for timeouts
	}
	
	return &RetryableError{
		StorageError: err,
		RetryAfter:   retryAfter,
		MaxRetries:   config.MaxAttempts,
		CurrentRetry: 0,
	}
}

// isRetryableErrorCode returns true if the error code represents a retryable error
func isRetryableErrorCode(code string) bool {
	switch code {
	case ErrCodeTimeout, ErrCodeConnectionFailed, ErrCodeBackendOffline:
		return true
	default:
		return false
	}
}

// ErrorMetrics tracks error statistics for monitoring
type ErrorMetrics struct {
	TotalErrors        int64                    `json:"total_errors"`
	ErrorsByCode       map[string]int64         `json:"errors_by_code"`
	ErrorsByBackend    map[string]int64         `json:"errors_by_backend"`
	ErrorsByOperation  map[string]int64         `json:"errors_by_operation"`
	LastError          *StorageError            `json:"last_error,omitempty"`
	LastErrorTime      time.Time                `json:"last_error_time"`
	ErrorRate          float64                  `json:"error_rate"` // errors per operation
	RecentErrors       []*StorageError          `json:"recent_errors"`
}

// NewErrorMetrics creates a new error metrics tracker
func NewErrorMetrics() *ErrorMetrics {
	return &ErrorMetrics{
		ErrorsByCode:      make(map[string]int64),
		ErrorsByBackend:   make(map[string]int64),
		ErrorsByOperation: make(map[string]int64),
		RecentErrors:      make([]*StorageError, 0),
	}
}

// RecordError records an error in the metrics
func (em *ErrorMetrics) RecordError(err *StorageError) {
	em.TotalErrors++
	em.ErrorsByCode[err.Code]++
	em.ErrorsByBackend[err.BackendType]++
	
	if operation, ok := err.Metadata["operation"].(string); ok {
		em.ErrorsByOperation[operation]++
	}
	
	em.LastError = err
	em.LastErrorTime = time.Now()
	
	// Keep recent errors (limit to last 100)
	em.RecentErrors = append(em.RecentErrors, err)
	if len(em.RecentErrors) > 100 {
		em.RecentErrors = em.RecentErrors[1:]
	}
}

// CalculateErrorRate calculates the error rate based on total operations
func (em *ErrorMetrics) CalculateErrorRate(totalOperations int64) {
	if totalOperations > 0 {
		em.ErrorRate = float64(em.TotalErrors) / float64(totalOperations)
	}
}

// GetTopErrorCodes returns the most common error codes
func (em *ErrorMetrics) GetTopErrorCodes(limit int) []ErrorCodeStat {
	stats := make([]ErrorCodeStat, 0, len(em.ErrorsByCode))
	
	for code, count := range em.ErrorsByCode {
		stats = append(stats, ErrorCodeStat{
			Code:  code,
			Count: count,
		})
	}
	
	// Sort by count (descending)
	for i := 0; i < len(stats)-1; i++ {
		for j := i + 1; j < len(stats); j++ {
			if stats[i].Count < stats[j].Count {
				stats[i], stats[j] = stats[j], stats[i]
			}
		}
	}
	
	if len(stats) > limit {
		stats = stats[:limit]
	}
	
	return stats
}

// ErrorCodeStat represents statistics for an error code
type ErrorCodeStat struct {
	Code  string `json:"code"`
	Count int64  `json:"count"`
}

// ErrorReporter provides error reporting capabilities
type ErrorReporter interface {
	ReportError(err *StorageError)
	ReportAggregateError(errors []error, operation string)
	GetErrorMetrics() *ErrorMetrics
}

// DefaultErrorReporter provides basic error reporting
type DefaultErrorReporter struct {
	metrics *ErrorMetrics
}

// NewDefaultErrorReporter creates a new default error reporter
func NewDefaultErrorReporter() *DefaultErrorReporter {
	return &DefaultErrorReporter{
		metrics: NewErrorMetrics(),
	}
}

// ReportError reports a single error
func (der *DefaultErrorReporter) ReportError(err *StorageError) {
	der.metrics.RecordError(err)
}

// ReportAggregateError reports multiple errors from an operation
func (der *DefaultErrorReporter) ReportAggregateError(errors []error, operation string) {
	for _, err := range errors {
		if storageErr, ok := err.(*StorageError); ok {
			der.ReportError(storageErr)
		}
	}
}

// GetErrorMetrics returns the current error metrics
func (der *DefaultErrorReporter) GetErrorMetrics() *ErrorMetrics {
	return der.metrics
}