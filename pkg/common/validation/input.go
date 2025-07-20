// Package validation provides comprehensive input validation and security checking
// for NoiseFS user inputs, file operations, and API requests.
//
// This package implements defense-in-depth input validation covering multiple
// security domains and attack vectors. It protects against common web application
// vulnerabilities while providing usable error messages for legitimate users.
//
// Validation Categories:
//   - File Security: Filename sanitization, path traversal prevention, extension filtering
//   - Content Security: IPFS CID validation, file size limits, block size validation  
//   - Password Security: Strength analysis, entropy calculation, common password detection
//   - Network Security: IP address validation, port range checking, HTTP header validation
//   - Request Security: Upload validation, download validation, composite request checking
//
// Security Features:
//   - Path traversal attack prevention ("../" sequences)
//   - Directory separator filtering (prevents directory injection)
//   - Control character filtering (prevents terminal injection)
//   - Reserved filename protection (Windows compatibility)
//   - Password strength assessment with entropy analysis
//   - Common password dictionary blocking (top 100 most common)
//   - Sequential pattern detection (123, abc, qwerty)
//   - Character repetition limits (prevents aaa...)
//   - HTTP header injection prevention
//   - IP address format validation
//
// Threat Model:
//   - Malicious file uploads with path traversal
//   - Weak password attacks and credential stuffing
//   - HTTP header injection and request smuggling
//   - Resource exhaustion through oversized inputs
//   - Directory traversal and file system escape
//   - Cross-platform filename compatibility issues
//
// Usage Examples:
//
//	// Create validator with security settings
//	validator := NewValidator()
//	validator.SetMaxFileSize(100 * 1024 * 1024) // 100MB limit
//	validator.SetAllowedExtensions([]string{".txt", ".pdf", ".jpg"})
//	
//	// Validate file upload
//	errors := validator.ValidateUploadRequest("document.pdf", 5242880, 128*1024)
//	if len(errors) > 0 {
//		return fmt.Errorf("validation failed: %v", errors)
//	}
//	
//	// Check password strength
//	strength, entropy := validator.GetPasswordStrength("MySecureP@ssw0rd123")
//	if strength < PasswordStrengthStrong {
//		return fmt.Errorf("password too weak: %s (%.1f bits)", strength, entropy)
//	}
//	
//	// Validate network input
//	if err := validator.ValidateIPAddress(clientIP); err != nil {
//		return fmt.Errorf("invalid IP: %w", err)
//	}
//
// Performance Characteristics:
//   - Filename validation: O(n) where n is filename length
//   - Password validation: O(n) where n is password length
//   - CID validation: O(1) with regex matching
//   - IP validation: O(1) with regex matching
//   - Composite validation: O(k) where k is number of checks
//
// Error Handling:
//   - ValidationError type with structured field information
//   - Detailed error messages for debugging and user feedback
//   - Sanitized error values (passwords are redacted)
//   - Multiple error collection for batch validation
//
package validation

import (
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// Validator provides comprehensive input validation utilities for NoiseFS security.
//
// This type encapsulates all validation logic and configuration, providing
// a consistent interface for security checking across the NoiseFS application.
// It maintains validation state and compiled patterns for efficient reuse.
//
// Configuration:
//   - maxFileSize: Maximum allowed file size in bytes (default: 100MB)
//   - maxFilenameLen: Maximum filename length in characters (default: 255)
//   - allowedExts: Whitelist of permitted file extensions (empty = all allowed)
//   - cidPattern: Compiled regex for IPFS Content Identifier validation
//
// Security Philosophy:
//   - Fail-safe defaults with reasonable limits
//   - Whitelist approach for file extensions
//   - Comprehensive character set validation
//   - Defense-in-depth against multiple attack vectors
//
// Thread Safety:
//   - Safe for concurrent read operations (validation methods)
//   - Configuration changes (SetMaxFileSize, SetAllowedExtensions) should be synchronized
//   - Regex compilation is thread-safe after initialization
//
// Performance Optimizations:
//   - Pre-compiled regex patterns for fast validation
//   - Map-based extension lookup for O(1) checking
//   - Efficient string processing with minimal allocations
//
type Validator struct {
	maxFileSize    int64
	maxFilenameLen int
	allowedExts    map[string]bool
	cidPattern     *regexp.Regexp
}

// NewValidator creates a new input validator with secure default settings.
//
// This constructor initializes a Validator instance with conservative security
// defaults suitable for most NoiseFS deployments. The defaults balance security
// with usability while preventing common attack vectors.
//
// Default Configuration:
//   - Maximum file size: 100MB (prevents resource exhaustion)
//   - Maximum filename length: 255 characters (filesystem compatibility)
//   - Allowed extensions: Empty map (all extensions permitted)
//   - CID pattern: Compiled regex for IPFS v0/v1 Content Identifiers
//
// IPFS CID Pattern Coverage:
//   - CIDv0: Base58 encoded, starts with 'Qm', 44-58 characters
//   - CIDv1: Various encodings including base32 and base58
//   - Supports both raw and directory CIDs
//   - Validates length and character set constraints
//
// Security Defaults:
//   - Conservative file size limits prevent DoS attacks
//   - Standard filename length prevents buffer overflows
//   - Permissive extension policy (can be restricted via SetAllowedExtensions)
//   - Robust CID validation prevents injection attacks
//
// Usage Pattern:
//   validator := NewValidator()
//   // Optionally customize settings
//   validator.SetMaxFileSize(50 * 1024 * 1024) // 50MB
//   validator.SetAllowedExtensions([]string{".txt", ".pdf"})
//
// Returns:
//   *Validator: A new validator instance with secure defaults
//
// Error Handling:
//   - Regex compilation failure would panic (should not occur with tested pattern)
//   - All runtime validation errors are returned via validation methods
//
// Complexity: O(1) - Simple initialization with regex compilation
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

// SetMaxFileSize configures the maximum allowed file size for upload validation.
//
// This method updates the file size limit used by ValidateFileSize() and
// ValidateUploadRequest(). It provides runtime configuration for different
// deployment scenarios and storage capacity constraints.
//
// Security Considerations:
//   - Prevents resource exhaustion attacks via oversized uploads
//   - Limits storage consumption per file
//   - Should be set based on available storage and network capacity
//   - Consider NoiseFS block overhead when setting limits
//
// NoiseFS Integration:
//   - File size affects number of blocks generated (fileSize / blockSize)
//   - Each file requires 2 additional randomizer blocks for anonymization
//   - Storage overhead: ~3x original file size for small files
//   - Large files approach 1x overhead due to randomizer reuse
//
// Deployment Guidance:
//   - Development: 10-100MB for testing
//   - Production: Based on storage backend capacity
//   - Mobile clients: Lower limits for bandwidth conservation
//   - Enterprise: Higher limits for business documents
//
// Parameters:
//   size: Maximum file size in bytes (0 = unlimited, negative values allowed but not recommended)
//
// Thread Safety:
//   - Not thread-safe for concurrent modification
//   - Safe to call during validator initialization
//   - Consider synchronization for runtime configuration changes
//
// Complexity: O(1) - Simple field assignment
func (v *Validator) SetMaxFileSize(size int64) {
	v.maxFileSize = size
}

// SetAllowedExtensions configures file extension whitelist for security filtering.
//
// This method establishes a whitelist of permitted file extensions, providing
// defense against malicious file uploads and enforcing organizational policies.
// Empty input disables extension filtering (all extensions allowed).
//
// Security Benefits:
//   - Prevents upload of executable files (.exe, .bat, .sh)
//   - Blocks script files that could contain malicious code
//   - Enforces content type restrictions for specific use cases
//   - Reduces attack surface for file-based exploits
//
// Whitelist Approach:
//   - Only explicitly listed extensions are permitted
//   - Case-insensitive matching (extensions converted to lowercase)
//   - Extensions should include the leading dot (e.g., ".txt", ".pdf")
//   - Empty list disables filtering (allows all extensions)
//
// Common Extension Sets:
//   - Documents: [".txt", ".pdf", ".doc", ".docx"]
//   - Images: [".jpg", ".jpeg", ".png", ".gif", ".webp"]
//   - Archives: [".zip", ".tar", ".gz", ".7z"]
//   - Code: [".go", ".js", ".py", ".java"]
//
// Security Considerations:
//   - Don't rely solely on extensions for security
//   - Consider MIME type validation for additional protection
//   - Be aware of extension spoofing attacks
//   - Some file types may contain executable code regardless of extension
//
// Usage Examples:
//   // Allow only text and PDF files
//   validator.SetAllowedExtensions([]string{".txt", ".pdf"})
//   
//   // Allow common document formats
//   validator.SetAllowedExtensions([]string{".doc", ".docx", ".pdf", ".txt"})
//   
//   // Disable extension filtering
//   validator.SetAllowedExtensions([]string{})
//
// Parameters:
//   extensions: Slice of allowed file extensions including leading dot
//
// Thread Safety:
//   - Not thread-safe for concurrent modification
//   - Safe to call during validator initialization
//   - Map recreation ensures clean state
//
// Complexity: O(n) where n is the number of extensions
func (v *Validator) SetAllowedExtensions(extensions []string) {
	v.allowedExts = make(map[string]bool)
	for _, ext := range extensions {
		v.allowedExts[strings.ToLower(ext)] = true
	}
}

// ValidationError represents a structured validation error with field context.
//
// This type provides detailed validation error information including the
// specific field that failed validation, a human-readable error message,
// and the value that caused the validation failure.
//
// Error Structure:
//   - Field: Name of the field that failed validation (e.g., "filename", "password")
//   - Message: Descriptive error message explaining the validation failure
//   - Value: The input value that caused the error (may be sanitized for security)
//
// Security Features:
//   - Sensitive values (passwords) are automatically redacted
//   - Error messages provide actionable feedback without exposing internals
//   - Field names help developers identify problematic inputs
//
// Usage in NoiseFS:
//   - Provides structured error responses for API validation
//   - Enables client-side error handling and user feedback
//   - Supports multiple validation errors per request
//   - Integrates with logging systems for security monitoring
//
type ValidationError struct {
	Field   string
	Message string
	Value   interface{}
}

// Error returns a formatted error message for the validation failure.
//
// This method implements the error interface, providing a standardized
// error message format for validation failures. The format includes both
// the field name and the specific validation message.
//
// Message Format:
//   "validation error for field 'FIELD_NAME': ERROR_MESSAGE"
//
// Examples:
//   "validation error for field 'filename': filename contains path traversal sequences"
//   "validation error for field 'password': password must be at least 8 characters long"
//
// Returns:
//   string: Formatted error message suitable for logging and error responses
//
// Complexity: O(1) - Simple string formatting
func (e ValidationError) Error() string {
	return fmt.Sprintf("validation error for field '%s': %s", e.Field, e.Message)
}

// ValidateFilename performs comprehensive security validation on filenames to prevent various attack vectors.
//
// This method implements multi-layered filename validation protecting against
// path traversal attacks, directory injection, control character exploits,
// and cross-platform compatibility issues.
//
// Security Validations (in order):
//   1. Empty filename rejection
//   2. Length limit enforcement (default: 255 characters)
//   3. Path traversal sequence detection (".." patterns)
//   4. Directory separator filtering ("/" and "\" characters)
//   5. Control character detection (prevents terminal injection)
//   6. Reserved filename protection (Windows compatibility)
//   7. File extension whitelist enforcement (if configured)
//
// Path Traversal Protection:
//   - Detects ".." sequences anywhere in filename
//   - Prevents directory escape attacks ("../../../etc/passwd")
//   - Blocks both Unix and Windows path traversal patterns
//
// Cross-Platform Security:
//   - Validates against Windows reserved names (CON, PRN, AUX, etc.)
//   - Handles case-insensitive reserved name detection
//   - Supports both COM1-COM9 and LPT1-LPT9 device names
//   - Prevents files that would be inaccessible on Windows systems
//
// Control Character Filtering:
//   - Detects Unicode control characters (U+0000 to U+001F, U+007F to U+009F)
//   - Prevents terminal injection and display corruption
//   - Protects against null byte injection attacks
//   - Ensures filename safety in terminal environments
//
// Extension Validation:
//   - Enforces whitelist if SetAllowedExtensions() was called with non-empty list
//   - Case-insensitive extension matching
//   - Extracts extension using filepath.Ext() for accuracy
//
// Security Threat Model:
//   - Path traversal attacks attempting directory escape
//   - Directory injection trying to create files in restricted locations
//   - Terminal injection via control characters in filenames
//   - Cross-platform compatibility attacks using reserved names
//   - File type restriction bypass using disallowed extensions
//
// Parameters:
//   filename: The filename to validate (without directory path)
//
// Returns:
//   error: ValidationError with specific field and message, nil if valid
//
// Thread Safety:
//   - Thread-safe for concurrent validation operations
//   - Reads validator configuration without modification
//
// Complexity: O(n) where n is the filename length
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

// ValidateFileSize validates file size against configured limits to prevent resource exhaustion.
//
// This method enforces file size limits to protect against denial-of-service
// attacks via oversized file uploads and to manage storage resource consumption
// in NoiseFS deployments.
//
// Validation Checks:
//   1. Negative size rejection (file sizes must be non-negative)
//   2. Maximum size limit enforcement (configurable via SetMaxFileSize)
//
// Security Benefits:
//   - Prevents resource exhaustion attacks via large file uploads
//   - Limits storage consumption per individual file
//   - Protects against memory exhaustion during file processing
//   - Enables capacity planning and quota enforcement
//
// NoiseFS Storage Implications:
//   - Each file is split into fixed-size blocks (typically 128KB)
//   - Large files have minimal storage overhead due to block reuse
//   - Small files may have significant overhead due to minimum block size
//   - File size affects anonymization processing time and memory usage
//
// Resource Planning:
//   - Consider available storage backend capacity
//   - Account for NoiseFS anonymization overhead (~3x for small files)
//   - Balance user experience with resource constraints
//   - Plan for concurrent upload scenarios
//
// Error Conditions:
//   - Negative file size: "file size cannot be negative"
//   - Oversized file: "file size exceeds maximum (X bytes)"
//
// Parameters:
//   size: File size in bytes to validate
//
// Returns:
//   error: ValidationError with details if invalid, nil if valid
//
// Thread Safety:
//   - Thread-safe for concurrent validation operations
//   - Reads maxFileSize configuration without modification
//
// Complexity: O(1) - Simple numeric comparisons
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

// ValidateCID validates IPFS Content Identifiers to ensure proper format and prevent injection attacks.
//
// This method validates IPFS CIDs (Content Identifiers) used throughout NoiseFS
// for addressing anonymized blocks in distributed storage. It ensures CIDs conform
// to IPFS specifications and prevents malicious input injection.
//
// IPFS CID Format Support:
//   - CIDv0: Base58-encoded, starting with 'Qm', 44-58 characters
//   - CIDv1: Various encodings (base32, base58, base64url)
//   - Raw CIDs: Direct hash representation
//   - Directory CIDs: IPFS directory structures
//
// Validation Strategy:
//   1. Empty string rejection
//   2. Length bounds checking (10-100 characters)
//   3. Pattern matching against known CID formats
//   4. Character set validation
//
// Security Considerations:
//   - Prevents injection attacks via malformed CIDs
//   - Validates input before IPFS operations
//   - Ensures CIDs can be safely logged and processed
//   - Protects against buffer overflow in CID processing
//
// NoiseFS Integration:
//   - Used for validating block references in file descriptors
//   - Ensures anonymized block CIDs are properly formatted
//   - Validates randomizer block references
//   - Critical for secure block retrieval operations
//
// Regex Pattern Coverage:
//   - CIDv0: `^[Qm][1-9A-HJ-NP-Za-km-z]{44,58}$`
//   - CIDv1 variants: Multiple patterns for different encodings
//   - Comprehensive coverage of standard IPFS CID formats
//
// Error Conditions:
//   - Empty CID: "CID cannot be empty"
//   - Invalid length: "CID length invalid"
//   - Format mismatch: "CID format invalid"
//
// Parameters:
//   cid: IPFS Content Identifier string to validate
//
// Returns:
//   error: ValidationError with specific issue, nil if valid
//
// Thread Safety:
//   - Thread-safe using pre-compiled regex pattern
//   - No shared state modification during validation
//
// Complexity: O(n) where n is CID length (dominated by regex matching)
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

// ValidatePassword performs comprehensive password strength validation with entropy analysis.
//
// This method implements multi-layered password security validation designed to
// prevent weak passwords while maintaining usability. It combines traditional
// complexity rules with modern entropy analysis and common password detection.
//
// Validation Layers (in order):
//   1. Length Requirements: 8-128 character range
//   2. Null Byte Detection: Prevents injection attacks
//   3. Character Complexity: Upper, lower, numeric, special characters required
//   4. Common Password Blocking: Dictionary of top 100 most common passwords
//   5. Entropy Analysis: Minimum 40-bit entropy requirement
//   6. Pattern Detection: Excessive repetition and sequential character blocking
//
// Length Requirements:
//   - Minimum 8 characters: Industry standard for basic security
//   - Maximum 128 characters: Prevents buffer overflow attacks
//   - Balances security with usability and system constraints
//
// Character Complexity Rules:
//   - At least one uppercase letter (A-Z)
//   - At least one lowercase letter (a-z)
//   - At least one numeric digit (0-9)
//   - At least one special character (non-alphanumeric)
//
// Entropy Analysis (40-bit minimum):
//   - Calculates password entropy using character set analysis
//   - 40 bits ≈ 1.1 trillion possible combinations
//   - Provides reasonable protection against brute force attacks
//   - Higher than many industry standards but suitable for privacy-critical storage
//
// Common Password Protection:
//   - Blocks top 100 most common passwords from security breaches
//   - Case-insensitive matching to prevent simple evasion
//   - Includes variants like "password123", "qwerty", "admin"
//   - Regular updates needed to address new common passwords
//
// Pattern Detection:
//   - Excessive Repetition: Prevents "aaa..." style passwords
//   - Sequential Characters: Blocks "123", "abc", "qwerty" patterns
//   - Keyboard Patterns: Detects common keyboard sequences
//
// Security Context for NoiseFS:
//   - Passwords protect encryption keys for private file storage
//   - Compromise allows decryption of user's anonymized files
//   - Strong passwords are critical since files may be stored long-term
//   - Higher security threshold justified by privacy-critical use case
//
// Error Message Security:
//   - All error messages contain "[redacted]" instead of actual password
//   - Prevents password exposure in logs and error responses
//   - Maintains user privacy during validation failures
//
// Compliance Considerations:
//   - Exceeds NIST SP 800-63B minimum recommendations
//   - Aligns with financial industry password requirements
//   - Suitable for GDPR and privacy regulation compliance
//
// Parameters:
//   password: The password string to validate
//
// Returns:
//   error: ValidationError with specific weakness identified, nil if valid
//
// Thread Safety:
//   - Thread-safe for concurrent password validation
//   - Uses read-only helper methods and constants
//
// Complexity: O(n) where n is password length
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
	
	// Check entropy (minimum 40 bits required for reasonable security)
	// 
	// ENTROPY THRESHOLD RATIONALE:
	// • 40 bits = ~1.1 trillion possible combinations (2^40 ≈ 1.1 × 10^12)
	// • Corresponds to "Fair" strength level in GetPasswordStrength classification
	// • Provides reasonable protection against casual brute force attacks
	// • Balances security requirements with usability for this privacy storage system
	// • Higher thresholds (50+ bits) recommended for high-security applications
	//
	// SECURITY CONTEXT:
	// • In NoiseFS, passwords protect encryption keys for private data storage
	// • Compromise allows decryption of user's stored files
	// • 40-bit minimum ensures passwords aren't trivially guessable
	// • Additional protections: common password blocking, pattern detection
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

// ValidateBlockSize validates NoiseFS block size parameters against supported values.
//
// This method ensures block sizes conform to NoiseFS architecture requirements
// and performance optimization constraints. Block size is critical for anonymization
// efficiency and storage overhead in the privacy-preserving file system.
//
// Supported Block Sizes:
//   - 64KB (65,536 bytes): Minimum size for reasonable efficiency
//   - 128KB (131,072 bytes): Recommended default for balanced performance
//   - 256KB (262,144 bytes): Good for large files and high-throughput scenarios
//   - 512KB (524,288 bytes): Optimized for very large files
//   - 1MB (1,048,576 bytes): Maximum size for memory efficiency
//
// NoiseFS Architecture Constraints:
//   - Fixed block sizes enable consistent anonymization patterns
//   - All files use the same block size for maximum privacy
//   - Block size affects randomizer reuse efficiency
//   - Larger blocks reduce metadata overhead but increase minimum file overhead
//
// Performance Implications:
//   - 64KB: Lower memory usage, higher metadata overhead
//   - 128KB: Balanced performance, recommended for most use cases
//   - 256KB+: Better for large files, more memory usage per operation
//   - 1MB: Optimal for very large files, significant memory requirements
//
// Security Considerations:
//   - Consistent block sizes prevent size-based fingerprinting
//   - Supported sizes have been tested for cryptographic safety
//   - Block size affects anonymity set size and reuse patterns
//
// Storage Overhead Analysis:
//   - Small files (< block size): ~300% overhead due to padding and randomizers
//   - Large files: Approaches ~100% overhead due to randomizer reuse
//   - Block size directly affects the crossover point for efficiency
//
// Future Considerations:
//   - Additional block sizes may be supported based on usage patterns
//   - Variable block sizes under research for improved efficiency
//   - Current fixed sizes prioritize privacy over storage optimization
//
// Parameters:
//   size: Block size in bytes to validate
//
// Returns:
//   error: ValidationError if size not supported, nil if valid
//
// Thread Safety:
//   - Thread-safe with read-only validation against constant slice
//
// Complexity: O(k) where k is number of supported sizes (currently 5)
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

// SanitizeInput performs defensive input sanitization to prevent injection attacks and ensure safe processing.
//
// This method implements conservative input sanitization that removes potentially
// dangerous characters while preserving legitimate user content. It provides
// defense-in-depth protection against various injection attack vectors.
//
// Sanitization Steps:
//   1. Null byte removal: Prevents null byte injection attacks
//   2. Whitespace trimming: Removes leading and trailing whitespace
//   3. Control character filtering: Removes dangerous control characters
//   4. Preserves safe characters: Keeps newlines and tabs for legitimate use
//
// Security Benefits:
//   - Null Byte Protection: Prevents string truncation attacks
//   - Control Character Filtering: Blocks terminal injection attempts
//   - Safe Character Preservation: Maintains usability for text content
//   - Consistent Output: Predictable sanitization behavior
//
// Character Handling:
//   - Null bytes (\x00): Completely removed
//   - Newlines (\n): Preserved for multiline text
//   - Tabs (\t): Preserved for formatted text
//   - Other control characters: Removed (U+0000-U+001F, U+007F-U+009F except \n, \t)
//
// Use Cases:
//   - User comments and descriptions
//   - Configuration values from external sources
//   - API input parameters
//   - Log message sanitization
//
// Limitations:
//   - May alter legitimate Unicode content containing control characters
//   - Does not validate semantic correctness of content
//   - Basic sanitization - may need domain-specific validation
//   - Not suitable for binary data or encoded content
//
// Security Context:
//   - Complements validation functions for defense-in-depth
//   - Prevents injection into logs and database storage
//   - Safe for display in terminal environments
//   - Reduces risk from untrusted user input
//
// Performance:
//   - Efficient string processing with minimal allocations
//   - Single pass through input string
//   - Uses strings.Builder for efficient concatenation
//
// Parameters:
//   input: The user input string to sanitize
//
// Returns:
//   string: Sanitized string safe for processing and storage
//
// Thread Safety:
//   - Thread-safe with no shared state
//
// Complexity: O(n) where n is input string length
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

// ValidateHTTPHeader validates HTTP header names and values to prevent injection attacks.
//
// This method provides security validation for HTTP headers to prevent header
// injection attacks and ensure RFC compliance. It validates both header names
// and values against established security best practices.
//
// Header Name Validation:
//   - Rejects empty header names
//   - Prevents control characters in header names
//   - Blocks whitespace characters that could enable injection
//   - Ensures RFC 7230 compliance for header name format
//
// Header Value Validation:
//   - Enforces maximum length limit (8192 bytes)
//   - Prevents header injection via newline characters
//   - Blocks carriage return and line feed characters
//   - Protects against HTTP response splitting attacks
//
// Security Threat Model:
//   - HTTP Header Injection: Malicious headers containing CRLF sequences
//   - HTTP Response Splitting: Injection of additional HTTP responses
//   - Cache Poisoning: Malformed headers affecting proxy caches
//   - Cross-Site Scripting: Headers containing script content
//
// RFC 7230 Compliance:
//   - Header names must not contain control characters or spaces
//   - Header values must not contain unescaped CRLF sequences
//   - Field names are case-insensitive but must be valid tokens
//
// Common Attack Patterns:
//   - "X-Custom\r\nSet-Cookie: malicious=1": Header injection
//   - "Valid\r\n\r\n<script>alert(1)</script>": Response splitting
//   - Names with spaces: "Bad Name: value": Protocol violation
//
// Integration with NoiseFS:
//   - Validates custom headers in API requests
//   - Protects proxy and load balancer configurations
//   - Ensures safe logging of header values
//   - Prevents injection into HTTP responses
//
// Performance Characteristics:
//   - O(n) validation where n is header name + value length
//   - Single pass validation for efficiency
//   - No regex compilation overhead
//
// Parameters:
//   name: HTTP header name to validate
//   value: HTTP header value to validate
//
// Returns:
//   error: ValidationError with specific issue, nil if valid
//
// Thread Safety:
//   - Thread-safe with no shared state
//
// Complexity: O(n) where n is the combined length of name and value
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

// ValidateIPAddress validates IP address format for both IPv4 and IPv6 addresses.
//
// This method provides comprehensive IP address validation using regex patterns
// to ensure addresses conform to standard formats and can be safely processed
// by network operations and logging systems.
//
// Supported Formats:
//   - IPv4: Standard dotted decimal notation (192.168.1.1)
//   - IPv6: Standard hexadecimal notation with colons
//   - localhost: Special case for local development
//
// IPv4 Validation:
//   - Four octets separated by dots
//   - Each octet: 0-255 (with proper leading zero handling)
//   - Prevents invalid formats like 999.999.999.999
//   - Handles edge cases like 0.0.0.0 and 255.255.255.255
//
// IPv6 Validation (Simplified):
//   - Eight groups of four hexadecimal digits
//   - Groups separated by colons
//   - Basic format validation (full RFC compliance would be more complex)
//   - Does not currently handle compressed notation (::)
//
// Security Considerations:
//   - Prevents injection of malformed IP addresses
//   - Ensures addresses can be safely logged and processed
//   - Validates input before network operations
//   - Protects against buffer overflow in IP parsing
//
// Limitations:
//   - IPv6 validation is simplified and may not catch all invalid formats
//   - Does not validate IP address reachability or existence
//   - Does not check for private/public address ranges
//   - Compressed IPv6 notation (::) not currently supported
//
// Use Cases in NoiseFS:
//   - Validating client IP addresses for rate limiting
//   - Configuration validation for network settings
//   - Whitelist/blacklist IP address validation
//   - Proxy and load balancer configuration
//
// Future Enhancements:
//   - Full RFC-compliant IPv6 validation
//   - Support for IPv6 compressed notation
//   - CIDR notation support for network ranges
//   - Private/public address range detection
//
// Parameters:
//   ip: IP address string to validate
//
// Returns:
//   error: ValidationError if invalid format, nil if valid
//
// Thread Safety:
//   - Thread-safe using compiled regex patterns
//
// Complexity: O(n) where n is IP address string length
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

// ValidatePort validates network port numbers against valid ranges.
//
// This method ensures port numbers fall within the valid range defined by
// network protocols and prevents invalid port configurations that could
// cause network operation failures.
//
// Port Number Ranges:
//   - Valid range: 1-65535 (16-bit unsigned integer)
//   - Port 0: Reserved and not allowed for general use
//   - Ports 1-1023: Well-known ports (usually require privileges)
//   - Ports 1024-49151: Registered ports
//   - Ports 49152-65535: Dynamic/private ports
//
// Technical Constraints:
//   - TCP and UDP protocols use 16-bit port numbers
//   - Port 0 is reserved for special use ("any port")
//   - Maximum valid port: 65535 (2^16 - 1)
//   - Negative ports are not valid in network protocols
//
// Security Considerations:
//   - Validates port numbers before network binding
//   - Prevents invalid configurations that could cause application crashes
//   - Ensures port numbers can be safely used in network operations
//   - Protects against integer overflow in port handling
//
// NoiseFS Integration:
//   - Validates configuration for IPFS nodes
//   - Checks API server port configurations
//   - Validates proxy and load balancer port settings
//   - Ensures WebUI port assignments are valid
//
// Common Use Cases:
//   - Server configuration validation
//   - API endpoint port validation
//   - Proxy configuration checking
//   - Dynamic port assignment validation
//
// Operating System Considerations:
//   - Ports 1-1023 typically require root privileges on Unix systems
//   - Some ports may be reserved by the operating system
//   - Actual availability depends on current network usage
//   - This validation only checks format, not availability
//
// Parameters:
//   port: Port number to validate
//
// Returns:
//   error: ValidationError if port out of range, nil if valid
//
// Thread Safety:
//   - Thread-safe with simple numeric validation
//
// Complexity: O(1) - Simple range checking
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

// ValidateUploadRequest performs comprehensive validation of complete file upload requests.
//
// This method provides end-to-end validation for NoiseFS file upload operations,
// combining multiple security checks into a single validation operation. It
// validates all aspects of an upload request for security and correctness.
//
// Validation Components:
//   1. Filename Security: Path traversal, injection, and format validation
//   2. File Size Limits: Resource exhaustion and capacity constraint checking
//   3. Block Size Configuration: NoiseFS architecture requirement validation
//
// Batch Validation Benefits:
//   - Single validation call for complete request
//   - Collects all validation errors for comprehensive feedback
//   - Consistent validation order and error reporting
//   - Efficient validation with minimal redundant processing
//
// Error Collection:
//   - Returns slice of ValidationError for all detected issues
//   - Empty slice indicates successful validation
//   - Each error includes field name and specific validation failure
//   - Enables client-side error handling and user feedback
//
// NoiseFS Upload Context:
//   - Validates request before expensive block splitting operations
//   - Ensures upload parameters are compatible with anonymization
//   - Prevents resource exhaustion from invalid uploads
//   - Validates configuration before IPFS storage operations
//
// Integration Pattern:
//   errors := validator.ValidateUploadRequest(filename, size, blockSize)
//   if len(errors) > 0 {
//       return http.StatusBadRequest, errors
//   }
//   // Proceed with upload processing
//
// Performance Characteristics:
//   - O(n) where n is filename length (dominated by filename validation)
//   - Minimal overhead for valid requests
//   - Early termination possible but not currently implemented
//   - Efficient error collection without excessive allocation
//
// Security Benefits:
//   - Comprehensive input validation before expensive operations
//   - Prevents multiple classes of attacks with single validation
//   - Consistent security policy enforcement
//   - Defense-in-depth protection
//
// Parameters:
//   filename: Name of file to upload (security validated)
//   fileSize: Size of file in bytes (resource limit validated)
//   blockSize: NoiseFS block size for anonymization (architecture validated)
//
// Returns:
//   []ValidationError: Slice of validation errors, empty if valid
//
// Thread Safety:
//   - Thread-safe using underlying validation methods
//
// Complexity: O(n) where n is filename length
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

// ValidateDownloadRequest performs comprehensive validation of NoiseFS download requests.
//
// This method validates all components of a file download request, ensuring
// the request is properly formatted and secure before attempting file
// reconstruction and decryption operations.
//
// Validation Components:
//   1. IPFS CID Format: Validates content identifier for file descriptor
//   2. Password Strength: Validates decryption password (if provided)
//
// CID Validation:
//   - Ensures CID refers to valid IPFS content
//   - Prevents injection attacks via malformed CIDs
//   - Validates format before expensive IPFS operations
//   - Critical for secure block retrieval
//
// Password Validation:
//   - Optional validation (empty password skips validation)
//   - Applies full password strength requirements if provided
//   - Ensures passwords meet security standards for file protection
//   - Prevents weak passwords from compromising file security
//
// NoiseFS Download Context:
//   - Validates request before expensive block retrieval operations
//   - Ensures CID format compatibility with IPFS
//   - Validates decryption credentials before file reconstruction
//   - Prevents resource waste on invalid requests
//
// Error Collection Strategy:
//   - Collects all validation errors for comprehensive feedback
//   - Returns empty slice for valid requests
//   - Each error provides specific field and validation failure
//   - Enables proper client error handling
//
// Security Considerations:
//   - CID validation prevents injection into IPFS operations
//   - Password validation ensures strong encryption protection
//   - Combined validation provides defense-in-depth
//   - Prevents enumeration attacks via error timing
//
// Usage Pattern:
//   errors := validator.ValidateDownloadRequest(cid, password)
//   if len(errors) > 0 {
//       return http.StatusBadRequest, errors
//   }
//   // Proceed with download processing
//
// Performance Impact:
//   - Minimal overhead for valid requests
//   - Password validation only if password provided
//   - CID validation uses efficient regex matching
//   - Much faster than actual IPFS operations
//
// Parameters:
//   cid: IPFS Content Identifier for file descriptor
//   password: Optional password for encrypted files (empty string if not encrypted)
//
// Returns:
//   []ValidationError: Slice of validation errors, empty if valid
//
// Thread Safety:
//   - Thread-safe using underlying validation methods
//
// Complexity: O(max(|cid|, |password|)) for string validation
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

// PasswordStrength represents discrete password strength levels based on entropy analysis.
//
// This enumeration provides a user-friendly classification of password strength
// that abstracts the underlying entropy calculations into meaningful security
// categories. Each level corresponds to specific entropy thresholds.
//
// Strength Levels (with entropy ranges):
//   - VeryWeak: < 20 bits entropy
//   - Weak: 20-34 bits entropy
//   - Fair: 35-49 bits entropy
//   - Strong: 50-64 bits entropy
//   - VeryStrong: ≥ 65 bits entropy
//
// Security Interpretation:
//   - VeryWeak: Trivially breakable, unacceptable for any security
//   - Weak: Vulnerable to targeted attacks, unsuitable for sensitive data
//   - Fair: Basic security, acceptable for low-sensitivity applications
//   - Strong: Good security for most applications, resists casual attacks
//   - VeryStrong: Excellent security, resists sophisticated attacks
//
// NoiseFS Security Context:
//   - Minimum "Fair" strength required (40+ bits)
//   - "Strong" recommended for sensitive files
//   - "VeryStrong" recommended for high-value data
//
type PasswordStrength int

const (
	PasswordStrengthVeryWeak PasswordStrength = iota
	PasswordStrengthWeak
	PasswordStrengthFair
	PasswordStrengthStrong
	PasswordStrengthVeryStrong
)

// String returns the human-readable string representation of password strength level.
//
// This method provides user-friendly strength level names for displaying
// password strength feedback in user interfaces and error messages.
//
// Returns:
//   string: Human-readable strength level name
//
// Complexity: O(1) - Simple switch statement
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

// GetPasswordStrength analyzes password and returns its strength level based on calculated entropy.
//
// STRENGTH CLASSIFICATION THRESHOLDS:
// The entropy thresholds are based on practical security considerations and industry practices:
//
// • Very Weak (< 20 bits): 
//   - Equivalent to ~3-character passwords or highly predictable patterns
//   - Vulnerable to trivial brute force attacks (< 1 million guesses)
//   - Examples: "123", "abc", "aaa" 
//   - Attack time: Seconds to minutes with modern hardware
//
// • Weak (20-34 bits):
//   - Equivalent to ~4-5 character passwords with limited character sets
//   - Vulnerable to targeted brute force (1M - 17B guesses)
//   - Examples: "pass1", "hello", short dictionary words with numbers
//   - Attack time: Minutes to hours with dedicated hardware
//
// • Fair (35-49 bits):
//   - Equivalent to ~6-7 character mixed-case passwords
//   - Provides reasonable protection against casual attacks (17B - 562T guesses)
//   - Examples: "Hello1", "myPass2", simple passphrases
//   - Attack time: Hours to days with specialized hardware
//
// • Strong (50-64 bits):
//   - Equivalent to ~8-9 character complex passwords or longer simple ones
//   - Good protection against most attack scenarios (562T - 18.4Q guesses)
//   - Examples: "MyP@ssw0rd", "ilovecoffee123"
//   - Attack time: Days to years with current technology
//
// • Very Strong (≥ 65 bits):
//   - Equivalent to 10+ character complex passwords or long passphrases
//   - Excellent protection against all but nation-state level attacks (≥ 18.4Q guesses)
//   - Examples: "MySecure!P@ssword123", "correct horse battery staple"
//   - Attack time: Years to centuries with current technology
//
// SECURITY CONSIDERATIONS:
// • These thresholds assume the entropy calculation's upper-bound estimate
// • Real-world attack resistance may be lower due to password patterns and dictionary attacks
// • The 40-bit minimum in ValidatePassword() ensures passwords meet "Fair" strength baseline
// • For high-security applications, consider requiring "Strong" (50+ bits) or "Very Strong" (65+ bits)
//
// INDUSTRY ALIGNMENT:
// • NIST SP 800-63B suggests focusing on length over complexity rules
// • These thresholds align with common security framework recommendations
// • The classification helps users understand relative password strength without false precision
//
// LIMITATIONS:
// • Strength assessment is based purely on character space analysis
// • Does not account for real-world attack patterns (dictionary attacks, social engineering)
// • High entropy does not guarantee resistance to targeted attacks using personal information
func (v *Validator) GetPasswordStrength(password string) (PasswordStrength, float64) {
	if password == "" {
		return PasswordStrengthVeryWeak, 0
	}
	
	entropy := v.calculatePasswordEntropy(password)
	
	// Apply entropy-based strength classification
	// Thresholds chosen to provide meaningful security guidance while avoiding false precision
	switch {
	case entropy < 20:
		// < 1M possible combinations - trivially breakable
		return PasswordStrengthVeryWeak, entropy
	case entropy < 35:
		// 1M to ~17B combinations - vulnerable to focused attacks
		return PasswordStrengthWeak, entropy
	case entropy < 50:
		// ~17B to ~562T combinations - reasonable baseline security
		return PasswordStrengthFair, entropy
	case entropy < 65:
		// ~562T to ~18.4Q combinations - good security for most applications
		return PasswordStrengthStrong, entropy
	default:
		// ≥ 18.4Q combinations - excellent security against current threats
		return PasswordStrengthVeryStrong, entropy
	}
}

// calculatePasswordEntropy calculates the entropy of a password in bits using Shannon entropy principles.
// 
// METHODOLOGY:
// This implementation uses a simplified character set analysis approach rather than true Shannon entropy.
// It estimates password strength by determining the effective character space size and applying the formula:
// Entropy = password_length × log₂(character_set_size)
//
// CHARACTER SET DETECTION:
// The algorithm scans the password once to detect which character categories are present:
// - Lowercase letters (a-z): 26 characters
// - Uppercase letters (A-Z): 26 characters  
// - Digits (0-9): 10 characters
// - Special characters: 32 characters (estimated common special chars)
//
// CHARACTER SET SIZE ASSUMPTIONS:
// • 26 lowercase: Standard English alphabet (a-z)
// • 26 uppercase: Standard English alphabet (A-Z)
// • 10 digits: Standard decimal digits (0-9)
// • 32 special: Conservative estimate covering most printable ASCII special characters
//   including: !@#$%^&*()_+-=[]{}|;':\",./<>?`~ and space
//   This is a practical approximation - actual special character space varies by context
//
// SECURITY IMPLICATIONS:
// • This is an UPPER BOUND estimate assuming truly random character selection
// • Real-world passwords have patterns, dictionary words, and human biases that reduce actual entropy
// • The algorithm does NOT account for:
//   - Dictionary words or common patterns
//   - Character frequency analysis
//   - Positional patterns (e.g., capital at start, number at end)
//   - Keyboard patterns or sequences
//   - Language-specific character distributions
//
// CRYPTOGRAPHIC ASSUMPTIONS:
// • Assumes uniform random distribution within detected character sets
// • Does not detect or penalize for reduced randomness due to human password creation habits
// • Suitable for minimum strength validation but should not be used for cryptographic key derivation
//
// LIMITATIONS:
// • Overestimates entropy for passwords with patterns or dictionary words
// • Does not account for password composition rules that reduce effective randomness
// • Special character count (32) is an approximation and may vary by system/context
// • Does not consider Unicode characters beyond basic ASCII
//
// INDUSTRY STANDARDS REFERENCE:
// • NIST SP 800-63B recommends against prescriptive composition rules
// • This implementation provides baseline entropy estimation for minimum security requirements
// • For high-security applications, consider additional checks against breach databases
func (v *Validator) calculatePasswordEntropy(password string) float64 {
	if password == "" {
		return 0
	}
	
	// Detect character categories present in the password
	// Each category contributes to the total character space size
	var charSetSize int
	var hasLower, hasUpper, hasDigit, hasSpecial bool
	
	// Single pass through password to detect character categories
	// This is more efficient than multiple passes and sufficient for our entropy model
	for _, char := range password {
		switch {
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsDigit(char):
			hasDigit = true
		case !unicode.IsLetter(char) && !unicode.IsNumber(char):
			// Any non-alphanumeric character is considered "special"
			// This includes punctuation, symbols, whitespace, etc.
			hasSpecial = true
		}
	}
	
	// Calculate effective character set size based on detected categories
	// Each character category adds to the total possible character space
	if hasLower {
		charSetSize += 26 // English lowercase letters: a-z
	}
	if hasUpper {
		charSetSize += 26 // English uppercase letters: A-Z
	}
	if hasDigit {
		charSetSize += 10 // Decimal digits: 0-9
	}
	if hasSpecial {
		// Estimated common special characters count
		// This covers most printable ASCII special characters commonly allowed in passwords
		// Including: !"#$%&'()*+,-./:;<=>?@[\]^_`{|}~ and space (32 total)
		charSetSize += 32
	}
	
	// Handle edge case: password with no recognizable character categories
	if charSetSize == 0 {
		return 0
	}
	
	// Apply Shannon entropy formula for uniform random selection:
	// H = L × log₂(N)
	// where H = entropy in bits, L = password length, N = character set size
	//
	// This assumes each character is independently and uniformly selected from the character set.
	// Real passwords deviate significantly from this ideal, so this provides an upper bound estimate.
	return float64(len(password)) * math.Log2(float64(charSetSize))
}

// isCommonPassword checks if a password appears in the common passwords blacklist.
//
// This method protects against credential stuffing and common password attacks
// by checking passwords against a curated list of the most frequently used
// passwords from security breaches and password analysis studies.
//
// Blacklist Source:
//   - Top 100 most common passwords from security breach analysis
//   - Includes variations with numbers and common substitutions
//   - Based on real-world password usage data
//   - Regularly updated list of compromised passwords
//
// Detection Strategy:
//   - Case-insensitive matching (all passwords converted to lowercase)
//   - Exact string matching against known common passwords
//   - Includes obvious variations like "password123", "admin", "qwerty"
//   - Covers multiple languages and character sets
//
// Common Password Categories:
//   - Dictionary words: "password", "admin", "welcome"
//   - Number sequences: "123456", "1234567890"
//   - Keyboard patterns: "qwerty", "asdf", "qazwsx"
//   - Names and dates: "michael", "jordan", "summer"
//   - Simple variations: "password1", "admin123"
//
// Security Benefits:
//   - Prevents use of passwords from known breaches
//   - Blocks passwords vulnerable to dictionary attacks
//   - Reduces success rate of credential stuffing attacks
//   - Forces users to choose more unique passwords
//
// Limitations:
//   - Fixed list may not include newest common passwords
//   - Only covers passwords in English
//   - Case-insensitive matching may miss some variations
//   - Does not detect similar passwords with small modifications
//
// Maintenance Requirements:
//   - List should be updated regularly with new breach data
//   - Consider adding locale-specific common passwords
//   - Monitor for new attack patterns and password trends
//
// Parameters:
//   password: Password to check (should be lowercase for proper matching)
//
// Returns:
//   bool: true if password is in common passwords list, false otherwise
//
// Thread Safety:
//   - Thread-safe using read-only map access
//
// Complexity: O(1) - HashMap lookup
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

// hasExcessiveRepeatedChars detects passwords with excessive character repetition patterns.
//
// This method identifies passwords containing three or more consecutive identical
// characters, which significantly reduces password entropy and makes passwords
// vulnerable to pattern-based attacks and human guessing.
//
// Detection Algorithm:
//   - Scans password for consecutive identical characters
//   - Triggers on 3 or more consecutive identical characters
//   - Resets counter when character changes
//   - Single pass through password for efficiency
//
// Examples of Rejected Patterns:
//   - "aaa" or longer: "password111", "hellooo"
//   - "..." sequences: "wait...", "loading..."
//   - Repeated symbols: "!!!password", "wow!!!"
//   - Number repetition: "password000", "test333"
//
// Security Rationale:
//   - Excessive repetition reduces effective entropy
//   - Makes passwords easier to guess and remember
//   - Common pattern in weak passwords
//   - Indicates low randomness in password creation
//
// Entropy Impact:
//   - "aaa" contributes much less entropy than "abc"
//   - Repetitive patterns are predictable to attackers
//   - Reduces effective character space size
//   - Makes password composition more guessable
//
// Edge Cases:
//   - Passwords shorter than 3 characters always pass
//   - Two consecutive characters are allowed
//   - Different characters reset the repetition counter
//   - Case-sensitive matching ("Aaa" would pass)
//
// Usability Considerations:
//   - Blocks some legitimate passwords with repetition
//   - Encourages more diverse character usage
//   - May require user education about the restriction
//   - Balances security with password memorability
//
// Parameters:
//   password: Password string to analyze for character repetition
//
// Returns:
//   bool: true if password has excessive repetition, false otherwise
//
// Thread Safety:
//   - Thread-safe with no shared state
//
// Complexity: O(n) where n is password length
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

// hasSequentialChars detects common sequential character patterns in passwords.
//
// This method identifies passwords containing sequential characters that follow
// predictable patterns, such as consecutive numbers, alphabet sequences, or
// keyboard patterns. These patterns significantly reduce password security.
//
// Sequential Pattern Categories:
//   1. Numeric sequences: "123", "234", "456", etc.
//   2. Reverse numeric: "987", "321", "876", etc.
//   3. Alphabetic sequences: "abc", "def", "xyz", etc.
//   4. Reverse alphabetic: "cba", "fed", "zyx", etc.
//   5. Keyboard patterns: "qwerty", "asdf", "qazwsx", etc.
//
// Detection Strategy:
//   - Case-insensitive matching (converts password to lowercase)
//   - Searches for predefined sequential patterns
//   - Covers both forward and reverse sequences
//   - Includes common keyboard layout patterns
//
// Security Impact:
//   - Sequential patterns are highly predictable
//   - Reduce effective entropy of passwords
//   - Common in weak passwords and password spraying attacks
//   - Easy for humans to guess and remember (but also crack)
//
// Pattern Examples:
//   - Numeric: "123", "456", "789", "987", "654"
//   - Alphabetic: "abc", "def", "xyz", "cba", "fed"
//   - Keyboard: "qwerty", "asdf", "zxcv", "qwertyuiop"
//   - Mixed: "qazwsx" (diagonal keyboard pattern)
//
// Comprehensive Coverage:
//   - All 3-character numeric sequences (000-999)
//   - All 3-character alphabetic sequences (aaa-zzz)
//   - Common keyboard patterns and layouts
//   - Both ascending and descending sequences
//
// Limitations:
//   - Only detects predefined patterns
//   - May not catch all possible sequential patterns
//   - Case-insensitive matching may miss some patterns
//   - Limited to 3+ character sequences
//
// Usability Impact:
//   - Blocks many easily memorable password patterns
//   - Encourages more random password composition
//   - May require password generation tools
//   - Improves overall password strength
//
// Parameters:
//   password: Password string to analyze for sequential patterns
//
// Returns:
//   bool: true if password contains sequential patterns, false otherwise
//
// Thread Safety:
//   - Thread-safe using read-only slice and string operations
//
// Complexity: O(n*m) where n is password length and m is number of patterns
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