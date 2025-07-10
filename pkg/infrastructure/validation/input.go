package validation

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// Validator provides input validation utilities
type Validator struct {
	maxFileSize    int64
	maxFilenameLen int
	allowedExts    map[string]bool
	cidPattern     *regexp.Regexp
}

// NewValidator creates a new input validator
func NewValidator() *Validator {
	// IPFS CID pattern (simplified for basic validation)
	cidPattern := regexp.MustCompile(`^[Qm][1-9A-HJ-NP-Za-km-z]{44,58}$|^[a-z0-9]{59}$|^[A-Za-z0-9]{46}$`)
	
	return &Validator{
		maxFileSize:    100 * 1024 * 1024, // 100MB default
		maxFilenameLen: 255,
		allowedExts:    make(map[string]bool),
		cidPattern:     cidPattern,
	}
}

// SetMaxFileSize sets the maximum allowed file size
func (v *Validator) SetMaxFileSize(size int64) {
	v.maxFileSize = size
}

// SetAllowedExtensions sets allowed file extensions (empty map = all allowed)
func (v *Validator) SetAllowedExtensions(extensions []string) {
	v.allowedExts = make(map[string]bool)
	for _, ext := range extensions {
		v.allowedExts[strings.ToLower(ext)] = true
	}
}

// ValidationError represents a validation error
type ValidationError struct {
	Field   string
	Message string
	Value   interface{}
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("validation error for field '%s': %s", e.Field, e.Message)
}

// ValidateFilename validates a filename for security issues
func (v *Validator) ValidateFilename(filename string) error {
	if filename == "" {
		return ValidationError{
			Field:   "filename",
			Message: "filename cannot be empty",
			Value:   filename,
		}
	}
	
	if len(filename) > v.maxFilenameLen {
		return ValidationError{
			Field:   "filename",
			Message: fmt.Sprintf("filename too long (max %d characters)", v.maxFilenameLen),
			Value:   filename,
		}
	}
	
	// Check for path traversal
	if strings.Contains(filename, "..") {
		return ValidationError{
			Field:   "filename",
			Message: "filename contains path traversal sequences",
			Value:   filename,
		}
	}
	
	// Check for directory separators
	if strings.ContainsAny(filename, "/\\") {
		return ValidationError{
			Field:   "filename",
			Message: "filename contains directory separators",
			Value:   filename,
		}
	}
	
	// Check for control characters
	for _, r := range filename {
		if unicode.IsControl(r) {
			return ValidationError{
				Field:   "filename",
				Message: "filename contains control characters",
				Value:   filename,
			}
		}
	}
	
	// Check for reserved names (Windows)
	reserved := []string{"CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9"}
	baseName := strings.ToUpper(strings.TrimSuffix(filename, filepath.Ext(filename)))
	for _, res := range reserved {
		if baseName == res {
			return ValidationError{
				Field:   "filename",
				Message: "filename is a reserved system name",
				Value:   filename,
			}
		}
	}
	
	// Check file extension if restrictions are set
	if len(v.allowedExts) > 0 {
		ext := strings.ToLower(filepath.Ext(filename))
		if !v.allowedExts[ext] {
			return ValidationError{
				Field:   "filename",
				Message: "file extension not allowed",
				Value:   filename,
			}
		}
	}
	
	return nil
}

// ValidateFileSize validates file size
func (v *Validator) ValidateFileSize(size int64) error {
	if size < 0 {
		return ValidationError{
			Field:   "file_size",
			Message: "file size cannot be negative",
			Value:   size,
		}
	}
	
	if size > v.maxFileSize {
		return ValidationError{
			Field:   "file_size",
			Message: fmt.Sprintf("file size exceeds maximum (%d bytes)", v.maxFileSize),
			Value:   size,
		}
	}
	
	return nil
}

// ValidateCID validates an IPFS Content Identifier
func (v *Validator) ValidateCID(cid string) error {
	if cid == "" {
		return ValidationError{
			Field:   "cid",
			Message: "CID cannot be empty",
			Value:   cid,
		}
	}
	
	// Basic length check
	if len(cid) < 10 || len(cid) > 100 {
		return ValidationError{
			Field:   "cid",
			Message: "CID length invalid",
			Value:   cid,
		}
	}
	
	// Pattern validation
	if !v.cidPattern.MatchString(cid) {
		return ValidationError{
			Field:   "cid",
			Message: "CID format invalid",
			Value:   cid,
		}
	}
	
	return nil
}

// ValidatePassword validates password strength
func (v *Validator) ValidatePassword(password string) error {
	if len(password) < 8 {
		return ValidationError{
			Field:   "password",
			Message: "password must be at least 8 characters long",
			Value:   "[redacted]",
		}
	}
	
	if len(password) > 128 {
		return ValidationError{
			Field:   "password",
			Message: "password too long (max 128 characters)",
			Value:   "[redacted]",
		}
	}
	
	// Check for null bytes
	if strings.Contains(password, "\x00") {
		return ValidationError{
			Field:   "password",
			Message: "password contains null bytes",
			Value:   "[redacted]",
		}
	}
	
	return nil
}

// ValidateBlockSize validates block size parameter
func (v *Validator) ValidateBlockSize(size int) error {
	validSizes := []int{64 * 1024, 128 * 1024, 256 * 1024, 512 * 1024, 1024 * 1024}
	
	for _, validSize := range validSizes {
		if size == validSize {
			return nil
		}
	}
	
	return ValidationError{
		Field:   "block_size",
		Message: "invalid block size (must be 64KB, 128KB, 256KB, 512KB, or 1MB)",
		Value:   size,
	}
}

// SanitizeInput sanitizes user input for safe processing
func (v *Validator) SanitizeInput(input string) string {
	// Remove null bytes
	input = strings.ReplaceAll(input, "\x00", "")
	
	// Trim whitespace
	input = strings.TrimSpace(input)
	
	// Remove control characters except newlines and tabs
	var result strings.Builder
	for _, r := range input {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			continue
		}
		result.WriteRune(r)
	}
	
	return result.String()
}

// ValidateHTTPHeader validates HTTP header values
func (v *Validator) ValidateHTTPHeader(name, value string) error {
	// Check header name
	if name == "" {
		return ValidationError{
			Field:   "header_name",
			Message: "header name cannot be empty",
			Value:   name,
		}
	}
	
	// Check for invalid characters in header name
	for _, r := range name {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return ValidationError{
				Field:   "header_name",
				Message: "header name contains invalid characters",
				Value:   name,
			}
		}
	}
	
	// Check header value
	if len(value) > 8192 {
		return ValidationError{
			Field:   "header_value",
			Message: "header value too long",
			Value:   value,
		}
	}
	
	// Check for header injection
	if strings.ContainsAny(value, "\r\n") {
		return ValidationError{
			Field:   "header_value",
			Message: "header value contains newline characters",
			Value:   value,
		}
	}
	
	return nil
}

// ValidateIPAddress validates IP address format
func (v *Validator) ValidateIPAddress(ip string) error {
	if ip == "" {
		return ValidationError{
			Field:   "ip_address",
			Message: "IP address cannot be empty",
			Value:   ip,
		}
	}
	
	// Basic IPv4 pattern
	ipv4Pattern := regexp.MustCompile(`^(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$`)
	
	// Basic IPv6 pattern (simplified)
	ipv6Pattern := regexp.MustCompile(`^([0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}$`)
	
	if !ipv4Pattern.MatchString(ip) && !ipv6Pattern.MatchString(ip) && ip != "localhost" {
		return ValidationError{
			Field:   "ip_address",
			Message: "invalid IP address format",
			Value:   ip,
		}
	}
	
	return nil
}

// ValidatePort validates port number
func (v *Validator) ValidatePort(port int) error {
	if port < 1 || port > 65535 {
		return ValidationError{
			Field:   "port",
			Message: "port must be between 1 and 65535",
			Value:   port,
		}
	}
	
	return nil
}

// ValidateUploadRequest validates a complete upload request
func (v *Validator) ValidateUploadRequest(filename string, fileSize int64, blockSize int) []ValidationError {
	var errors []ValidationError
	
	if err := v.ValidateFilename(filename); err != nil {
		if ve, ok := err.(ValidationError); ok {
			errors = append(errors, ve)
		}
	}
	
	if err := v.ValidateFileSize(fileSize); err != nil {
		if ve, ok := err.(ValidationError); ok {
			errors = append(errors, ve)
		}
	}
	
	if err := v.ValidateBlockSize(blockSize); err != nil {
		if ve, ok := err.(ValidationError); ok {
			errors = append(errors, ve)
		}
	}
	
	return errors
}

// ValidateDownloadRequest validates a download request
func (v *Validator) ValidateDownloadRequest(cid, password string) []ValidationError {
	var errors []ValidationError
	
	if err := v.ValidateCID(cid); err != nil {
		if ve, ok := err.(ValidationError); ok {
			errors = append(errors, ve)
		}
	}
	
	if password != "" {
		if err := v.ValidatePassword(password); err != nil {
			if ve, ok := err.(ValidationError); ok {
				errors = append(errors, ve)
			}
		}
	}
	
	return errors
}