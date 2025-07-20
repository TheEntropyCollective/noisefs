package resilience

import (
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"
	"time"
)

// ErrorType represents the classification of errors for resilience handling
type ErrorType int

const (
	UnknownError ErrorType = iota
	NetworkError
	StorageError
	TransientError
	PermanentError
	TimeoutError
	AuthenticationError
	RateLimitError
)

// String returns the string representation of ErrorType
func (et ErrorType) String() string {
	switch et {
	case NetworkError:
		return "NetworkError"
	case StorageError:
		return "StorageError"
	case TransientError:
		return "TransientError"
	case PermanentError:
		return "PermanentError"
	case TimeoutError:
		return "TimeoutError"
	case AuthenticationError:
		return "AuthenticationError"
	case RateLimitError:
		return "RateLimitError"
	default:
		return "UnknownError"
	}
}

// ClassifiedError wraps an error with its classification for resilience handling
type ClassifiedError struct {
	Err       error
	Type      ErrorType
	Retryable bool
	Component string
	Timestamp time.Time
}

// Error implements the error interface
func (ce *ClassifiedError) Error() string {
	return fmt.Sprintf("[%s:%s] %v", ce.Component, ce.Type.String(), ce.Err)
}

// Unwrap returns the underlying error
func (ce *ClassifiedError) Unwrap() error {
	return ce.Err
}

// IsRetryable returns whether this error should trigger retry logic
func (ce *ClassifiedError) IsRetryable() bool {
	return ce.Retryable
}

// ClassifyError analyzes an error and returns its classification
func ClassifyError(err error, component string) *ClassifiedError {
	if err == nil {
		return nil
	}

	classified := &ClassifiedError{
		Err:       err,
		Component: component,
		Timestamp: time.Now(),
	}

	// Classify based on error type and content
	switch {
	case isNetworkError(err):
		classified.Type = NetworkError
		classified.Retryable = true
	case isTimeoutError(err):
		classified.Type = TimeoutError
		classified.Retryable = true
	case isStorageError(err):
		classified.Type = StorageError
		classified.Retryable = true
	case isRateLimitError(err):
		classified.Type = RateLimitError
		classified.Retryable = true
	case isAuthenticationError(err):
		classified.Type = AuthenticationError
		classified.Retryable = false
	case isTransientError(err):
		classified.Type = TransientError
		classified.Retryable = true
	case isPermanentError(err):
		classified.Type = PermanentError
		classified.Retryable = false
	default:
		classified.Type = UnknownError
		classified.Retryable = true // Default to retryable for unknown errors
	}

	return classified
}

// isNetworkError checks if the error is network-related
func isNetworkError(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}

	// Check for common network error patterns
	errStr := err.Error()
	networkPatterns := []string{
		"connection refused",
		"connection reset",
		"network is unreachable",
		"no route to host",
		"host is down",
		"connection timed out",
		"network timeout",
		"dial tcp",
		"dial udp",
	}

	for _, pattern := range networkPatterns {
		if contains(errStr, pattern) {
			return true
		}
	}

	return false
}

// isTimeoutError checks if the error is timeout-related
func isTimeoutError(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	// Check for timeout patterns in error message
	errStr := err.Error()
	timeoutPatterns := []string{
		"timeout",
		"deadline exceeded",
		"context deadline exceeded",
		"request timeout",
		"operation timed out",
	}

	for _, pattern := range timeoutPatterns {
		if contains(errStr, pattern) {
			return true
		}
	}

	return false
}

// isStorageError checks if the error is storage-related
func isStorageError(err error) bool {
	// Check for filesystem errors
	if errors.Is(err, os.ErrNotExist) ||
		errors.Is(err, os.ErrPermission) ||
		errors.Is(err, os.ErrExist) {
		return true
	}

	// Check for syscall errors
	var errno syscall.Errno
	if errors.As(err, &errno) {
		switch errno {
		case syscall.ENOSPC, syscall.EIO, syscall.EROFS, syscall.EMFILE, syscall.ENFILE:
			return true
		}
	}

	// Check for storage error patterns
	errStr := err.Error()
	storagePatterns := []string{
		"no space left",
		"disk full",
		"storage quota exceeded",
		"read-only file system",
		"file system error",
		"block device",
		"storage backend",
		"ipfs",
		"storage manager",
	}

	for _, pattern := range storagePatterns {
		if contains(errStr, pattern) {
			return true
		}
	}

	return false
}

// isRateLimitError checks if the error is rate limiting related
func isRateLimitError(err error) bool {
	errStr := err.Error()
	rateLimitPatterns := []string{
		"rate limit",
		"too many requests",
		"quota exceeded",
		"throttled",
		"429",
		"rate exceeded",
	}

	for _, pattern := range rateLimitPatterns {
		if contains(errStr, pattern) {
			return true
		}
	}

	return false
}

// isAuthenticationError checks if the error is authentication-related
func isAuthenticationError(err error) bool {
	errStr := err.Error()
	authPatterns := []string{
		"unauthorized",
		"authentication failed",
		"invalid credentials",
		"access denied",
		"permission denied",
		"401",
		"403",
		"forbidden",
	}

	for _, pattern := range authPatterns {
		if contains(errStr, pattern) {
			return true
		}
	}

	return false
}

// isTransientError checks if the error is likely transient
func isTransientError(err error) bool {
	errStr := err.Error()
	transientPatterns := []string{
		"temporary failure",
		"service unavailable",
		"server overloaded",
		"try again",
		"502",
		"503",
		"504",
		"internal server error",
		"bad gateway",
		"gateway timeout",
	}

	for _, pattern := range transientPatterns {
		if contains(errStr, pattern) {
			return true
		}
	}

	return false
}

// isPermanentError checks if the error is permanent and should not be retried
func isPermanentError(err error) bool {
	errStr := err.Error()
	permanentPatterns := []string{
		"not found",
		"does not exist",
		"invalid format",
		"malformed",
		"bad request",
		"400",
		"404",
		"410",
		"unprocessable entity",
		"validation failed",
	}

	for _, pattern := range permanentPatterns {
		if contains(errStr, pattern) {
			return true
		}
	}

	return false
}

// contains is a case-insensitive string contains check
func contains(s, substr string) bool {
	return len(s) >= len(substr) && 
		   (s == substr || 
		    len(s) > len(substr) && 
		    (s[:len(substr)] == substr || 
		     s[len(s)-len(substr):] == substr ||
		     containsSubstring(s, substr)))
}

// containsSubstring checks if substr exists anywhere in s
func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Common error constructors for resilience package

// NewNetworkError creates a classified network error
func NewNetworkError(err error, component string) *ClassifiedError {
	return &ClassifiedError{
		Err:       err,
		Type:      NetworkError,
		Retryable: true,
		Component: component,
		Timestamp: time.Now(),
	}
}

// NewStorageError creates a classified storage error
func NewStorageError(err error, component string) *ClassifiedError {
	return &ClassifiedError{
		Err:       err,
		Type:      StorageError,
		Retryable: true,
		Component: component,
		Timestamp: time.Now(),
	}
}

// NewTimeoutError creates a classified timeout error
func NewTimeoutError(err error, component string) *ClassifiedError {
	return &ClassifiedError{
		Err:       err,
		Type:      TimeoutError,
		Retryable: true,
		Component: component,
		Timestamp: time.Now(),
	}
}

// NewPermanentError creates a classified permanent error
func NewPermanentError(err error, component string) *ClassifiedError {
	return &ClassifiedError{
		Err:       err,
		Type:      PermanentError,
		Retryable: false,
		Component: component,
		Timestamp: time.Now(),
	}
}