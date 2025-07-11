package util

import (
	"fmt"
	"strings"
)

// ErrorWithSuggestion wraps an error with a helpful suggestion
type ErrorWithSuggestion struct {
	Err        error
	Suggestion string
}

func (e *ErrorWithSuggestion) Error() string {
	return fmt.Sprintf("%v\nSuggestion: %s", e.Err, e.Suggestion)
}

// WrapErrorWithSuggestion creates an error with a helpful suggestion
func WrapErrorWithSuggestion(err error, suggestion string) error {
	if err == nil {
		return nil
	}
	return &ErrorWithSuggestion{
		Err:        err,
		Suggestion: suggestion,
	}
}

// GetErrorSuggestion returns helpful suggestions based on common error patterns
func GetErrorSuggestion(err error) string {
	if err == nil {
		return ""
	}
	
	errStr := err.Error()
	
	// IPFS connection errors
	if strings.Contains(errStr, "connection refused") || strings.Contains(errStr, "no route to host") {
		return "Make sure IPFS daemon is running with 'ipfs daemon'"
	}
	
	if strings.Contains(errStr, "cannot connect to IPFS") {
		return "Check if IPFS is installed and the daemon is running. You can specify a custom endpoint with --api"
	}
	
	// File errors
	if strings.Contains(errStr, "no such file or directory") {
		return "Check the file path and ensure the file exists"
	}
	
	if strings.Contains(errStr, "permission denied") {
		return "Check file permissions or try running with appropriate privileges"
	}
	
	if strings.Contains(errStr, "file too large") {
		return "The file may be too large for processing. Consider splitting it into smaller parts"
	}
	
	// Block errors
	if strings.Contains(errStr, "failed to store block") {
		return "This may be due to IPFS storage issues. Check available disk space and IPFS repo size"
	}
	
	if strings.Contains(errStr, "failed to retrieve block") {
		return "The block may not be available in the network. Ensure the source node is online"
	}
	
	// Descriptor errors
	if strings.Contains(errStr, "failed to load descriptor") {
		return "The descriptor CID may be invalid or the descriptor is not available in IPFS"
	}
	
	// Network errors
	if strings.Contains(errStr, "timeout") {
		return "The operation timed out. Check your network connection and try again"
	}
	
	if strings.Contains(errStr, "context deadline exceeded") {
		return "The operation took too long. This may be due to network issues or large file sizes"
	}
	
	// Configuration errors
	if strings.Contains(errStr, "failed to load configuration") {
		return "Check if the configuration file exists and is valid JSON. Use --config to specify a custom path"
	}
	
	// Default suggestion
	return "Check the error message above and ensure all requirements are met"
}

// FormatError formats an error with suggestions for better user experience
func FormatError(err error) string {
	if err == nil {
		return ""
	}
	
	// Check if it already has a suggestion
	if _, ok := err.(*ErrorWithSuggestion); ok {
		return err.Error()
	}
	
	// Get automatic suggestion
	suggestion := GetErrorSuggestion(err)
	if suggestion != "" {
		return fmt.Sprintf("Error: %v\n💡 Suggestion: %s", err, suggestion)
	}
	
	return fmt.Sprintf("Error: %v", err)
}