package validation

import (
	"fmt"
	"math"
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

// ValidatePassword validates password strength with comprehensive checks
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
	
	// Check complexity requirements
	var hasUpper, hasLower, hasNumber, hasSpecial bool
	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsNumber(char):
			hasNumber = true
		case !unicode.IsLetter(char) && !unicode.IsNumber(char):
			hasSpecial = true
		}
	}
	
	if !hasUpper {
		return ValidationError{
			Field:   "password",
			Message: "password must contain at least one uppercase letter",
			Value:   "[redacted]",
		}
	}
	
	if !hasLower {
		return ValidationError{
			Field:   "password",
			Message: "password must contain at least one lowercase letter",
			Value:   "[redacted]",
		}
	}
	
	if !hasNumber {
		return ValidationError{
			Field:   "password",
			Message: "password must contain at least one number",
			Value:   "[redacted]",
		}
	}
	
	if !hasSpecial {
		return ValidationError{
			Field:   "password",
			Message: "password must contain at least one special character",
			Value:   "[redacted]",
		}
	}
	
	// Check for common passwords
	if v.isCommonPassword(strings.ToLower(password)) {
		return ValidationError{
			Field:   "password",
			Message: "password is too common, please choose a more unique password",
			Value:   "[redacted]",
		}
	}
	
	// Check entropy (minimum 40 bits recommended)
	entropy := v.calculatePasswordEntropy(password)
	if entropy < 40 {
		return ValidationError{
			Field:   "password",
			Message: "password is too predictable, please use a more complex password",
			Value:   "[redacted]",
		}
	}
	
	// Check for repeated characters
	if v.hasExcessiveRepeatedChars(password) {
		return ValidationError{
			Field:   "password",
			Message: "password contains too many repeated characters",
			Value:   "[redacted]",
		}
	}
	
	// Check for sequential characters
	if v.hasSequentialChars(password) {
		return ValidationError{
			Field:   "password",
			Message: "password contains sequential characters (e.g., 123, abc)",
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

// PasswordStrength represents the strength level of a password
type PasswordStrength int

const (
	PasswordStrengthVeryWeak PasswordStrength = iota
	PasswordStrengthWeak
	PasswordStrengthFair
	PasswordStrengthStrong
	PasswordStrengthVeryStrong
)

// String returns the string representation of password strength
func (ps PasswordStrength) String() string {
	switch ps {
	case PasswordStrengthVeryWeak:
		return "Very Weak"
	case PasswordStrengthWeak:
		return "Weak"
	case PasswordStrengthFair:
		return "Fair"
	case PasswordStrengthStrong:
		return "Strong"
	case PasswordStrengthVeryStrong:
		return "Very Strong"
	default:
		return "Unknown"
	}
}

// GetPasswordStrength analyzes password and returns its strength level
func (v *Validator) GetPasswordStrength(password string) (PasswordStrength, float64) {
	if password == "" {
		return PasswordStrengthVeryWeak, 0
	}
	
	entropy := v.calculatePasswordEntropy(password)
	
	// Determine strength based on entropy bits
	switch {
	case entropy < 20:
		return PasswordStrengthVeryWeak, entropy
	case entropy < 35:
		return PasswordStrengthWeak, entropy
	case entropy < 50:
		return PasswordStrengthFair, entropy
	case entropy < 65:
		return PasswordStrengthStrong, entropy
	default:
		return PasswordStrengthVeryStrong, entropy
	}
}

// calculatePasswordEntropy calculates the entropy of a password in bits
func (v *Validator) calculatePasswordEntropy(password string) float64 {
	if password == "" {
		return 0
	}
	
	// Character set size
	var charSetSize int
	var hasLower, hasUpper, hasDigit, hasSpecial bool
	
	for _, char := range password {
		switch {
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsDigit(char):
			hasDigit = true
		case !unicode.IsLetter(char) && !unicode.IsNumber(char):
			hasSpecial = true
		}
	}
	
	if hasLower {
		charSetSize += 26
	}
	if hasUpper {
		charSetSize += 26
	}
	if hasDigit {
		charSetSize += 10
	}
	if hasSpecial {
		charSetSize += 32 // Common special characters
	}
	
	if charSetSize == 0 {
		return 0
	}
	
	// Calculate entropy: length * log2(charset_size)
	return float64(len(password)) * math.Log2(float64(charSetSize))
}

// isCommonPassword checks if password is in the common passwords list
func (v *Validator) isCommonPassword(password string) bool {
	// Top 100 most common passwords (lowercase)
	commonPasswords := map[string]bool{
		"password": true, "123456": true, "password123": true, "12345678": true,
		"qwerty": true, "abc123": true, "123456789": true, "111111": true,
		"1234567": true, "iloveyou": true, "adobe123": true, "welcome": true,
		"admin": true, "letmein": true, "monkey": true, "1234567890": true,
		"photoshop": true, "1234": true, "sunshine": true, "12345": true,
		"password1": true, "princess": true, "azerty": true, "trustno1": true,
		"000000": true, "access": true, "baseball": true, "batman": true,
		"dragon": true, "football": true, "freedom": true, "hello": true,
		"login": true, "master": true, "michael": true, "mustang": true,
		"ninja": true, "passw0rd": true, "password2": true, "qazwsx": true,
		"qwertyuiop": true, "shadow": true, "superman": true, "welcome123": true,
		"zaq1zaq1": true, "1q2w3e4r": true, "1qaz2wsx": true, "aa123456": true,
		"donald": true, "hottie": true, "loveme": true, "whatever": true,
		"666666": true, "7777777": true, "888888": true, "987654321": true,
		"jordan": true, "michelle": true, "nicole": true, "hunter": true,
		"test": true, "test123": true, "testing": true, "changeme": true,
		"summer": true, "winter": true, "spring": true, "autumn": true,
		"secret": true, "god": true, "love": true, "hello123": true,
		"123": true, "1111": true, "12341234": true, "123123": true,
		"guest": true, "default": true, "user": true, "demo": true,
		"oracle": true, "root": true, "toor": true, "pass": true,
		"mysql": true, "web": true, "cisco": true, "internet": true,
		"administrator": true, "adminadmin": true, "system": true, "server": true,
		"computer": true, "test1234": true, "database": true, "security": true,
		"finance": true, "sales": true, "support": true, "development": true,
	}
	
	return commonPasswords[password]
}

// hasExcessiveRepeatedChars checks for excessive character repetition
func (v *Validator) hasExcessiveRepeatedChars(password string) bool {
	if len(password) < 3 {
		return false
	}
	
	// Check for 3 or more consecutive identical characters
	count := 1
	for i := 1; i < len(password); i++ {
		if password[i] == password[i-1] {
			count++
			if count >= 3 {
				return true
			}
		} else {
			count = 1
		}
	}
	
	return false
}

// hasSequentialChars checks for sequential characters
func (v *Validator) hasSequentialChars(password string) bool {
	if len(password) < 3 {
		return false
	}
	
	lowerPass := strings.ToLower(password)
	
	// Common sequences to check
	sequences := []string{
		"123", "234", "345", "456", "567", "678", "789", "890",
		"098", "987", "876", "765", "654", "543", "432", "321", "210",
		"abc", "bcd", "cde", "def", "efg", "fgh", "ghi", "hij", "ijk",
		"jkl", "klm", "lmn", "mno", "nop", "opq", "pqr", "qrs", "rst",
		"stu", "tuv", "uvw", "vwx", "wxy", "xyz",
		"zyx", "yxw", "xwv", "wvu", "vut", "uts", "tsr", "srq", "rqp",
		"qpo", "pon", "onm", "nml", "mlk", "lkj", "kji", "jih", "ihg",
		"hgf", "gfe", "fed", "edc", "dcb", "cba",
		"qwerty", "asdf", "zxcv", "qazwsx", "qwertyuiop",
	}
	
	for _, seq := range sequences {
		if strings.Contains(lowerPass, seq) {
			return true
		}
	}
	
	return false
}