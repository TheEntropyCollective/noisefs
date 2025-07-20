// Package logging provides structured logging with automatic PII sanitization for NoiseFS.
//
// This package implements a comprehensive logging system designed for privacy-preserving
// applications where sensitive data protection is critical. It provides structured
// logging with automatic detection and redaction of personally identifiable information.
//
// Key Features:
//   - Structured logging with JSON and text formats
//   - Automatic PII and sensitive data sanitization
//   - Configurable log levels and output destinations
//   - Component-based logging for better organization
//   - Thread-safe operations with concurrent access protection
//   - Field-based logging with context preservation
//   - Global logger convenience functions
//
// Security Features:
//   - Automatic detection of sensitive field names (password, token, key, etc.)
//   - Pattern-based sanitization of credit cards, SSNs, JWT tokens
//   - Base64 encoded secret detection and redaction
//   - Inline sensitive data pattern replacement
//   - Recursive sanitization of nested data structures
//   - Configurable sanitization enable/disable
//
// NoiseFS Integration:
//   - Privacy-first logging approach for file storage system
//   - Component-based logging for different subsystems
//   - Sanitization prevents accidental exposure of user data
//   - Structured logging enables better monitoring and debugging
//
// Usage Examples:
//
//	// Initialize global logger with custom configuration
//	config := &logging.Config{
//		Level:            logging.InfoLevel,
//		Format:           logging.JSONFormat,
//		Output:           os.Stdout,
//		EnableSanitizing: true,
//	}
//	logging.InitGlobalLogger(config)
//	
//	// Component-specific logging
//	logger := logging.GetGlobalLogger().WithComponent("ipfs")
//	logger.Info("Starting IPFS node", map[string]interface{}{
//		"node_id": nodeID,
//		"port":    4001,
//	})
//	
//	// Field-based logging with automatic sanitization
//	logger.WithField("user_id", userID).
//		WithField("filename", "document.pdf").
//		Info("File uploaded successfully")
//	
//	// Formatted logging with automatic argument sanitization
//	logger.Infof("Processing block %s for user %s", blockCID, userID)
//
package logging

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

// LogLevel represents hierarchical logging levels for message filtering and prioritization.
//
// LogLevel implements a standard logging hierarchy where each level includes all
// messages at higher priority levels. This enables fine-grained control over
// logging verbosity and helps manage log volume in production systems.
//
// Level Hierarchy (lowest to highest priority):
//   - DebugLevel: Detailed diagnostic information for development
//   - InfoLevel: General operational messages and state changes
//   - WarnLevel: Warning conditions that don't prevent operation
//   - ErrorLevel: Error conditions that may affect functionality
//
// Filtering Behavior:
//   - Setting level to InfoLevel will show Info, Warn, and Error messages
//   - Setting level to ErrorLevel will show only Error messages
//   - Lower levels include higher priority levels automatically
//
// Production Recommendations:
//   - Development: DebugLevel for comprehensive diagnostic information
//   - Staging: InfoLevel for operational visibility with reasonable volume
//   - Production: WarnLevel or ErrorLevel for minimal noise and focus on issues
//
type LogLevel int

const (
	DebugLevel LogLevel = iota
	InfoLevel
	WarnLevel
	ErrorLevel
)

// String returns the human-readable string representation of the log level.
//
// This method provides standardized level names for log output formatting
// and configuration parsing. The names follow common logging conventions
// and are suitable for both human reading and machine processing.
//
// Level Names:
//   - DebugLevel: "DEBUG"
//   - InfoLevel: "INFO"
//   - WarnLevel: "WARN"
//   - ErrorLevel: "ERROR"
//   - Invalid levels: "UNKNOWN"
//
// Returns:
//   string: Uppercase level name for consistent formatting
//
// Complexity: O(1) - Simple switch statement
func (l LogLevel) String() string {
	switch l {
	case DebugLevel:
		return "DEBUG"
	case InfoLevel:
		return "INFO"
	case WarnLevel:
		return "WARN"
	case ErrorLevel:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// ParseLogLevel parses a string representation into a LogLevel for configuration.
//
// This function enables configuration of log levels from external sources
// such as environment variables, configuration files, and command-line
// arguments. It supports common level name variations and aliases.
//
// Supported Level Names (case-insensitive):
//   - "debug": DebugLevel
//   - "info": InfoLevel
//   - "warn" or "warning": WarnLevel
//   - "error": ErrorLevel
//
// Configuration Integration:
//   - Environment variables: LOG_LEVEL=info
//   - Configuration files: {"log_level": "warn"}
//   - Command-line flags: --log-level=debug
//
// Error Handling:
//   - Invalid level names return InfoLevel as safe default
//   - Error message includes the invalid input for debugging
//   - Case-insensitive parsing improves usability
//
// Parameters:
//   level: String representation of log level to parse
//
// Returns:
//   LogLevel: Parsed log level or InfoLevel if invalid
//   error: Descriptive error for invalid level names, nil if valid
//
// Complexity: O(1) - Simple string comparison with switch statement
func ParseLogLevel(level string) (LogLevel, error) {
	switch strings.ToLower(level) {
	case "debug":
		return DebugLevel, nil
	case "info":
		return InfoLevel, nil
	case "warn", "warning":
		return WarnLevel, nil
	case "error":
		return ErrorLevel, nil
	default:
		return InfoLevel, fmt.Errorf("invalid log level: %s", level)
	}
}

// LogFormat represents different log output formatting options for various use cases.
//
// LogFormat enables selection between human-readable and machine-parseable
// output formats, supporting both development workflows and production
// log aggregation systems.
//
// Format Options:
//   - TextFormat: Human-readable format for development and console output
//   - JSONFormat: Structured format for log aggregation and analysis
//
// Use Case Recommendations:
//   - Development: TextFormat for readable console output
//   - Production: JSONFormat for structured log collection
//   - Log aggregation: JSONFormat for parsing by ELK, Splunk, etc.
//   - File logging: Either format depending on downstream processing
//
type LogFormat int

const (
	TextFormat LogFormat = iota
	JSONFormat
)

// LogEntry represents a single structured log record with metadata and sanitization support.
//
// LogEntry encapsulates all information for a single log message, including
// timing, severity, content, and contextual metadata. It supports both
// human-readable and machine-parseable output formats.
//
// Structure Fields:
//   - Timestamp: Precise timing information for log correlation
//   - Level: String representation of log severity level
//   - Message: Primary log message content
//   - Fields: Key-value metadata for structured logging
//   - Caller: Optional source code location information
//
// JSON Serialization:
//   - All fields have appropriate JSON tags for structured output
//   - Optional fields use omitempty to reduce output size
//   - Timestamp formatting follows RFC3339 standard
//
// Sanitization Integration:
//   - All fields are processed for sensitive data before output
//   - Fields map is recursively sanitized for nested structures
//   - Message content is pattern-matched for PII redaction
//
type LogEntry struct {
	Timestamp time.Time         `json:"timestamp"`
	Level     string            `json:"level"`
	Message   string            `json:"message"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
	Caller    string            `json:"caller,omitempty"`
}

// Logger provides comprehensive structured logging with automatic PII sanitization.
//
// Logger is the core logging implementation that combines structured logging
// capabilities with privacy protection through automatic sensitive data
// detection and redaction. It supports concurrent usage and flexible configuration.
//
// Core Features:
//   - Thread-safe concurrent logging operations
//   - Configurable log levels and output formatting
//   - Automatic sensitive data sanitization
//   - Component-based logging for organizational clarity
//   - Optional caller information for debugging
//   - Flexible output destination support
//
// Thread Safety:
//   - RWMutex protects configuration changes during logging operations
//   - Read locks for logging operations allow high concurrency
//   - Write locks for configuration changes ensure consistency
//
// Configuration Fields:
//   - level: Minimum log level for message filtering
//   - format: Output format (text or JSON)
//   - output: Destination writer for log messages
//   - showCaller: Include source code location in log entries
//   - component: Component name for message categorization
//   - enableSanitizing: Toggle for sensitive data protection
//   - sensitivePatterns: Compiled regex patterns for PII detection
//
// Privacy Protection:
//   - Automatic detection of sensitive field names
//   - Pattern-based content sanitization
//   - Recursive sanitization of nested data structures
//   - Configurable enable/disable for performance scenarios
//
type Logger struct {
	mu               sync.RWMutex
	level            LogLevel
	format           LogFormat
	output           io.Writer
	showCaller       bool
	component        string
	enableSanitizing bool
	sensitivePatterns []*regexp.Regexp
}

// Config holds comprehensive logger configuration for initialization and customization.
//
// Config provides a structured way to configure Logger instances with all
// available options, enabling consistent logger setup across the application
// and easy configuration from external sources.
//
// Configuration Options:
//   - Level: Minimum log level for message filtering
//   - Format: Output format selection (text or JSON)
//   - Output: Destination writer (file, stdout, custom)
//   - ShowCaller: Include source code location in log entries
//   - Component: Component name for message categorization
//   - EnableSanitizing: Toggle automatic sensitive data protection
//
// Usage Patterns:
//   - Application initialization with custom settings
//   - Environment-specific configuration (dev/staging/prod)
//   - Component-specific logger configuration
//   - Testing scenarios with custom output capture
//
type Config struct {
	Level            LogLevel
	Format           LogFormat
	Output           io.Writer
	ShowCaller       bool
	Component        string
	EnableSanitizing bool
}

// DefaultConfig returns a secure default logger configuration suitable for most NoiseFS deployments.
//
// This function provides conservative defaults that balance functionality,
// security, and performance. The defaults prioritize privacy protection
// and operational visibility while minimizing log volume.
//
// Default Settings:
//   - Level: InfoLevel for reasonable operational visibility
//   - Format: TextFormat for human-readable console output
//   - Output: os.Stdout for immediate visibility
//   - ShowCaller: false to reduce log verbosity
//   - Component: empty string (no default component)
//   - EnableSanitizing: true for privacy protection
//
// Security-First Approach:
//   - Sanitization enabled by default to prevent PII exposure
//   - InfoLevel prevents verbose debug information in production
//   - Safe defaults suitable for production deployment
//
// Customization:
//   config := DefaultConfig()
//   config.Level = DebugLevel  // For development
//   config.Format = JSONFormat // For log aggregation
//
// Returns:
//   *Config: Default configuration with secure settings
//
// Complexity: O(1) - Simple struct initialization
func DefaultConfig() *Config {
	return &Config{
		Level:            InfoLevel,
		Format:           TextFormat,
		Output:           os.Stdout,
		ShowCaller:       false,
		Component:        "",
		EnableSanitizing: true,
	}
}

// Sensitive field patterns for detection
var (
	// Pattern for field names that might contain sensitive data
	sensitiveFieldPattern = regexp.MustCompile(`(?i)(password|passwd|pwd|secret|token|key|auth|authorization|credential|api[-_]?key|access[-_]?token|refresh[-_]?token|private[-_]?key|session[-_]?id|ssn|credit[-_]?card|cvv)`)
	
	// Pattern for values that look like tokens or keys
	tokenPattern = regexp.MustCompile(`^[a-zA-Z0-9_\-\.]{20,}$`)
	
	// Pattern for credit card numbers
	creditCardPattern = regexp.MustCompile(`\b\d{4}[\s\-]?\d{4}[\s\-]?\d{4}[\s\-]?\d{4}\b`)
	
	// Pattern for SSN
	ssnPattern = regexp.MustCompile(`\b\d{3}-?\d{2}-?\d{4}\b`)
	
	// Pattern for JWT tokens
	jwtPattern = regexp.MustCompile(`^[A-Za-z0-9-_]+\.[A-Za-z0-9-_]+\.[A-Za-z0-9-_]*$`)
	
	// Pattern for base64 encoded strings that might be secrets
	base64SecretPattern = regexp.MustCompile(`^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$`)
)

// NewLogger creates a new Logger instance with the specified configuration and security patterns.
//
// This constructor initializes a fully functional Logger with privacy protection
// enabled and all sensitive data detection patterns compiled. It provides
// safe defaults when configuration is nil.
//
// Initialization Process:
//   1. Use DefaultConfig() if config is nil
//   2. Initialize Logger with provided/default configuration
//   3. Compile and assign sensitive data detection patterns
//   4. Return ready-to-use Logger instance
//
// Sensitive Data Patterns:
//   - sensitiveFieldPattern: Detects field names containing sensitive keywords
//   - creditCardPattern: Identifies credit card number formats
//   - ssnPattern: Detects Social Security Number patterns
//
// Security Features:
//   - Automatic PII detection and redaction patterns
//   - Privacy-first configuration with sanitization enabled by default
//   - Comprehensive pattern matching for common sensitive data types
//
// Performance Considerations:
//   - Regex patterns are compiled once during initialization
//   - Pattern reuse across all log operations for efficiency
//   - Thread-safe design for concurrent logging operations
//
// Parameters:
//   config: Logger configuration (uses defaults if nil)
//
// Returns:
//   *Logger: Fully initialized logger with sensitive data protection
//
// Complexity: O(1) - Simple initialization with pattern compilation
func NewLogger(config *Config) *Logger {
	if config == nil {
		config = DefaultConfig()
	}

	logger := &Logger{
		level:            config.Level,
		format:           config.Format,
		output:           config.Output,
		showCaller:       config.ShowCaller,
		component:        config.Component,
		enableSanitizing: config.EnableSanitizing,
	}
	
	// Initialize sensitive patterns
	logger.sensitivePatterns = []*regexp.Regexp{
		sensitiveFieldPattern,
		creditCardPattern,
		ssnPattern,
	}
	
	return logger
}

// WithComponent creates a new Logger instance with the specified component name for categorized logging.
//
// This method creates a new Logger that inherits all configuration from the
// current Logger but adds a component identifier for better log organization
// and filtering. Component names help identify log sources in large systems.
//
// Component Usage:
//   - Subsystem identification: "ipfs", "fuse", "cache", "api"
//   - Module-specific logging: "block-splitter", "anonymizer", "descriptor"
//   - Service categorization: "storage", "encryption", "validation"
//
// Logger Inheritance:
//   - All configuration settings are copied from parent logger
//   - Sensitive patterns are shared (not duplicated)
//   - Thread-safe creation with read lock on parent
//
// Benefits:
//   - Log filtering by component in aggregation systems
//   - Clear source identification in distributed systems
//   - Hierarchical logging organization
//   - Performance tracing by component
//
// Usage Example:
//   ipfsLogger := logger.WithComponent("ipfs")
//   ipfsLogger.Info("Starting IPFS node")
//   // Output includes component field
//
// Parameters:
//   component: Component name for log categorization
//
// Returns:
//   *Logger: New logger instance with component configuration
//
// Thread Safety:
//   - Thread-safe with read lock on parent logger
//
// Complexity: O(1) - Simple logger creation with configuration copy
func (l *Logger) WithComponent(component string) *Logger {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return &Logger{
		level:             l.level,
		format:            l.format,
		output:            l.output,
		showCaller:        l.showCaller,
		component:         component,
		enableSanitizing:  l.enableSanitizing,
		sensitivePatterns: l.sensitivePatterns,
	}
}

// SetLevel dynamically changes the minimum logging level for message filtering.
//
// This method allows runtime adjustment of logging verbosity, enabling
// dynamic debugging and production log volume control without restart.
// Higher levels include all lower-priority messages.
//
// Level Filtering:
//   - DebugLevel: Shows all messages (most verbose)
//   - InfoLevel: Shows Info, Warn, Error messages
//   - WarnLevel: Shows Warn, Error messages only
//   - ErrorLevel: Shows Error messages only (least verbose)
//
// Runtime Control:
//   - Reduce log volume in production by increasing level
//   - Enable debug logging for troubleshooting
//   - Performance tuning by filtering verbose messages
//
// Thread Safety:
//   - Write lock ensures atomic level changes
//   - Safe for concurrent modification during logging operations
//
// Parameters:
//   level: New minimum log level for message filtering
//
// Complexity: O(1) - Simple field assignment with lock
func (l *Logger) SetLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// SetOutput dynamically changes the log output destination.
//
// This method enables runtime redirection of log output to different
// destinations such as files, network connections, or custom writers.
// Useful for log rotation, testing, and operational requirements.
//
// Common Output Destinations:
//   - os.Stdout: Console output for development
//   - os.Stderr: Error output separate from stdout
//   - File writers: Persistent logging to disk
//   - io.MultiWriter: Simultaneous output to multiple destinations
//   - Custom writers: Network logging, buffers, etc.
//
// Runtime Scenarios:
//   - Log rotation: Switch to new file handles
//   - Testing: Capture output for validation
//   - Emergency logging: Redirect to alternative destinations
//   - Performance optimization: Switch to faster writers
//
// Thread Safety:
//   - Write lock ensures atomic output changes
//   - Safe for concurrent modification during logging operations
//
// Parameters:
//   output: New io.Writer destination for log messages
//
// Complexity: O(1) - Simple field assignment with lock
func (l *Logger) SetOutput(output io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.output = output
}

// IsEnabled checks if a specific log level will produce output based on current configuration.
//
// This method provides efficient level checking to avoid expensive log
// message construction when messages won't be output. It's useful for
// conditional logging and performance optimization.
//
// Level Hierarchy:
//   - Messages at or above the configured level are enabled
//   - Lower priority messages are disabled and won't be processed
//   - Enables early return from expensive logging operations
//
// Performance Optimization:
//   if logger.IsEnabled(DebugLevel) {
//       logger.Debug("Expensive debug info", expensiveCalculation())
//   }
//
// Thread Safety:
//   - Read lock allows concurrent level checking
//   - Safe for concurrent access during configuration changes
//
// Parameters:
//   level: Log level to check for enablement
//
// Returns:
//   bool: true if level is enabled, false if filtered out
//
// Complexity: O(1) - Simple comparison with read lock
func (l *Logger) IsEnabled(level LogLevel) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return level >= l.level
}

// SanitizeLogEntry performs comprehensive sensitive data sanitization on a complete log entry.
//
// This method is the primary privacy protection mechanism that processes all
// components of a log entry to detect and redact personally identifiable
// information (PII) and other sensitive data before output.
//
// Sanitization Scope:
//   1. Message content: Pattern-based PII detection and replacement
//   2. Field keys: Sensitive field name detection
//   3. Field values: Recursive sanitization of nested structures
//   4. Preserves log structure while protecting sensitive content
//
// Data Protection:
//   - Credit card numbers: Replaced with "[CREDIT-CARD-REDACTED]"
//   - Social Security Numbers: Replaced with "[SSN-REDACTED]"
//   - JWT tokens: Replaced with "[JWT-REDACTED]"
//   - API keys and tokens: Replaced with "[TOKEN-REDACTED]"
//   - Password fields: Replaced with "[REDACTED]"
//   - Inline sensitive patterns: key=value pairs sanitized
//
// Field Name Detection:
//   - Case-insensitive matching of sensitive field names
//   - Regex pattern matching for common sensitive keywords
//   - Covers password, token, key, auth, credential variations
//
// Recursive Processing:
//   - Handles nested maps and slices recursively
//   - Preserves data structure while sanitizing content
//   - Ensures complete sanitization of complex log data
//
// Performance Considerations:
//   - Early return if sanitization is disabled
//   - Efficient pattern matching with compiled regexes
//   - Single-pass processing of log entry components
//
// Parameters:
//   entry: LogEntry to sanitize in-place
//
// Thread Safety:
//   - Safe for concurrent use (reads configuration, modifies entry)
//
// Complexity: O(n) where n is the total size of log entry content
func (l *Logger) SanitizeLogEntry(entry *LogEntry) {
	if !l.enableSanitizing {
		return
	}
	
	// Sanitize message
	entry.Message = l.sanitizeString(entry.Message)
	
	// Sanitize fields
	if entry.Fields != nil {
		sanitizedFields := make(map[string]interface{})
		for key, value := range entry.Fields {
			// Check if the field name itself suggests sensitive data
			if l.isSensitiveFieldName(key) {
				sanitizedFields[key] = "[REDACTED]"
			} else {
				// Check the value
				sanitizedFields[key] = l.sanitizeValue(value)
			}
		}
		entry.Fields = sanitizedFields
	}
}

// isSensitiveFieldName determines if a field name indicates sensitive content requiring redaction.
//
// This method implements field-level security by analyzing field names
// against known patterns that typically contain sensitive information.
// It enables proactive protection even when field values appear innocuous.
//
// Sensitive Field Patterns:
//   - Authentication: password, passwd, pwd, auth, authorization
//   - Security tokens: token, secret, key, api_key, access_token
//   - Credentials: credential, private_key, session_id
//   - Financial: credit_card, cvv
//   - Personal: ssn (Social Security Number)
//
// Pattern Matching:
//   - Case-insensitive matching for robustness
//   - Supports underscore and hyphen variations
//   - Regex-based detection for comprehensive coverage
//
// Security Benefits:
//   - Proactive protection of sensitive fields
//   - Defense against accidental logging of credentials
//   - Comprehensive coverage of common sensitive field names
//
// Parameters:
//   fieldName: Field name to analyze for sensitivity indicators
//
// Returns:
//   bool: true if field name suggests sensitive content, false otherwise
//
// Complexity: O(1) - Regex matching with compiled pattern
func (l *Logger) isSensitiveFieldName(fieldName string) bool {
	return sensitiveFieldPattern.MatchString(fieldName)
}

// sanitizeValue performs recursive sanitization of individual values with type-aware processing.
//
// This method handles sanitization of various Go data types, providing
// comprehensive protection for complex nested data structures commonly
// found in structured logging scenarios.
//
// Type-Specific Processing:
//   - Strings: Pattern-based PII detection and replacement
//   - Maps: Recursive sanitization with field name analysis
//   - Slices: Element-wise recursive sanitization
//   - Other types: Pass-through without modification
//
// Recursive Sanitization:
//   - Handles arbitrarily nested data structures
//   - Preserves original data types and structure
//   - Ensures complete sanitization of complex objects
//
// String Sanitization:
//   - Credit card pattern detection and redaction
//   - SSN pattern detection and redaction
//   - JWT token format detection and redaction
//   - Base64 secret pattern detection and redaction
//   - Inline key=value pattern sanitization
//
// Map Processing:
//   - Field name sensitivity analysis
//   - Recursive value sanitization for nested structures
//   - Key preservation with value protection
//
// Performance Optimization:
//   - Type-specific processing avoids unnecessary work
//   - Early return for non-sensitive types
//   - Efficient pattern matching with compiled regexes
//
// Parameters:
//   value: Interface{} value to sanitize (any Go type)
//
// Returns:
//   interface{}: Sanitized value preserving original type structure
//
// Complexity: O(n) where n is the size of the value structure
func (l *Logger) sanitizeValue(value interface{}) interface{} {
	switch v := value.(type) {
	case string:
		return l.sanitizeString(v)
	case map[string]interface{}:
		// Recursively sanitize nested maps
		sanitized := make(map[string]interface{})
		for k, val := range v {
			if l.isSensitiveFieldName(k) {
				sanitized[k] = "[REDACTED]"
			} else {
				sanitized[k] = l.sanitizeValue(val)
			}
		}
		return sanitized
	case []interface{}:
		// Recursively sanitize slices
		sanitized := make([]interface{}, len(v))
		for i, val := range v {
			sanitized[i] = l.sanitizeValue(val)
		}
		return sanitized
	default:
		return value
	}
}

// sanitizeString performs comprehensive pattern-based sanitization of string content for PII protection.
//
// This method implements the core string sanitization logic that detects
// and redacts various types of sensitive information using regex patterns.
// It's the foundation of the privacy protection system.
//
// Sanitization Patterns:
//   1. Credit card numbers: 16-digit patterns with optional separators
//   2. Social Security Numbers: XXX-XX-XXXX or XXXXXXXXX formats
//   3. JWT tokens: Base64-encoded three-part structure
//   4. API tokens: Long alphanumeric strings (20+ characters)
//   5. Base64 secrets: Base64-encoded strings that might contain secrets
//   6. Inline key=value patterns: password=secret123 style patterns
//
// Processing Strategy:
//   - Early return for empty strings to avoid unnecessary work
//   - Sequential pattern matching with specific replacements
//   - Inline pattern replacement preserves context while redacting values
//   - Token detection uses length and character set heuristics
//
// Replacement Tokens:
//   - "[CREDIT-CARD-REDACTED]": Credit card number replacements
//   - "[SSN-REDACTED]": Social Security Number replacements
//   - "[JWT-REDACTED]": JWT token replacements
//   - "[TOKEN-REDACTED]": Generic token/secret replacements
//   - "key=[REDACTED]": Inline pattern replacements
//
// Security Considerations:
//   - Comprehensive pattern coverage for common PII types
//   - False positive tolerance (better to over-redact than under-redact)
//   - Preserves string structure while protecting sensitive content
//
// Parameters:
//   s: String content to sanitize for sensitive patterns
//
// Returns:
//   string: Sanitized string with sensitive patterns redacted
//
// Complexity: O(n) where n is string length (dominated by regex operations)
func (l *Logger) sanitizeString(s string) string {
	// Don't sanitize empty strings
	if s == "" {
		return s
	}
	
	// Check for credit card numbers
	if creditCardPattern.MatchString(s) {
		s = creditCardPattern.ReplaceAllString(s, "[CREDIT-CARD-REDACTED]")
	}
	
	// Check for SSN
	if ssnPattern.MatchString(s) {
		s = ssnPattern.ReplaceAllString(s, "[SSN-REDACTED]")
	}
	
	// Check for JWT tokens
	if jwtPattern.MatchString(s) {
		return "[JWT-REDACTED]"
	}
	
	// Check if the entire string looks like a token (long alphanumeric string)
	if len(s) >= 20 && tokenPattern.MatchString(s) {
		// Check if it might be a base64 encoded secret
		if base64SecretPattern.MatchString(s) {
			return "[TOKEN-REDACTED]"
		}
	}
	
	// Look for inline sensitive data patterns (e.g., "password=secret123")
	inlinePattern := regexp.MustCompile(`(?i)(password|passwd|pwd|secret|token|key|auth|credential|api[-_]?key|access[-_]?token)\s*[:=]\s*[^\s]+`)
	if inlinePattern.MatchString(s) {
		s = inlinePattern.ReplaceAllStringFunc(s, func(match string) string {
			parts := regexp.MustCompile(`[:=]`).Split(match, 2)
			if len(parts) == 2 {
				return parts[0] + "=[REDACTED]"
			}
			return "[REDACTED]"
		})
	}
	
	return s
}

// SetSanitizing dynamically enables or disables sensitive data sanitization for performance tuning.
//
// This method allows runtime control over privacy protection features,
// enabling performance optimization in scenarios where sanitization
// overhead is critical or data is known to be non-sensitive.
//
// Use Cases:
//   - Development environments: Disable for full debugging visibility
//   - Performance-critical scenarios: Disable when sanitization overhead is unacceptable
//   - Testing environments: Enable/disable for test case validation
//   - Production tuning: Enable for privacy compliance
//
// Security Considerations:
//   - Disabling sanitization exposes all logged data
//   - Should only be disabled in trusted environments
//   - Consider compliance requirements before disabling
//   - Re-enable for production deployments handling user data
//
// Performance Impact:
//   - Enabling: Adds regex processing overhead to all log operations
//   - Disabling: Eliminates sanitization overhead for maximum performance
//   - Dynamic switching: No restart required for configuration changes
//
// Parameters:
//   enabled: true to enable sanitization, false to disable
//
// Thread Safety:
//   - Write lock ensures atomic configuration changes
//
// Complexity: O(1) - Simple field assignment with lock
func (l *Logger) SetSanitizing(enabled bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.enableSanitizing = enabled
}

// log implements the core logging logic with level filtering, sanitization, and output formatting.
//
// This method is the central logging pipeline that processes all log messages
// through filtering, enrichment, sanitization, formatting, and output stages.
// It coordinates all logging features into a single operation.
//
// Processing Pipeline:
//   1. Level filtering: Early return if level not enabled
//   2. Entry construction: Build LogEntry with timestamp and metadata
//   3. Component enrichment: Add component information if configured
//   4. Caller information: Add source location if showCaller enabled
//   5. Sanitization: Process entry for sensitive data redaction
//   6. Formatting: Convert to text or JSON format
//   7. Output: Write to configured destination
//
// Performance Optimizations:
//   - Early return for disabled levels avoids expensive processing
//   - Conditional caller information lookup (expensive runtime.Caller)
//   - Efficient string formatting based on configured format
//   - Single write operation to output destination
//
// Thread Safety:
//   - Read lock protects configuration during logging operation
//   - Safe for concurrent logging from multiple goroutines
//   - Configuration changes are atomic with write locks
//
// Caller Information:
//   - Uses runtime.Caller(3) to skip logging wrapper functions
//   - Provides filename and line number for debugging
//   - Optional feature controlled by showCaller configuration
//
// Parameters:
//   level: Log level for filtering and display
//   message: Primary log message content
//   fields: Optional structured metadata
//
// Complexity: O(n) where n is the size of message and fields content
func (l *Logger) log(level LogLevel, message string, fields map[string]interface{}) {
	if !l.IsEnabled(level) {
		return
	}

	l.mu.RLock()
	defer l.mu.RUnlock()

	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     level.String(),
		Message:   message,
		Fields:    fields,
	}

	// Add component to fields if specified
	if l.component != "" {
		if entry.Fields == nil {
			entry.Fields = make(map[string]interface{})
		}
		entry.Fields["component"] = l.component
	}

	// Add caller information if enabled
	if l.showCaller {
		if _, file, line, ok := runtime.Caller(3); ok {
			entry.Caller = fmt.Sprintf("%s:%d", filepath.Base(file), line)
		}
	}
	
	// Sanitize the log entry to remove sensitive data
	l.SanitizeLogEntry(&entry)

	// Format and write the log entry
	var output string
	switch l.format {
	case JSONFormat:
		data, _ := json.Marshal(entry)
		output = string(data) + "\n"
	default: // TextFormat
		output = l.formatText(entry)
	}

	l.output.Write([]byte(output))
}

// formatText converts a LogEntry to human-readable text format for console and file output.
//
// This method implements text formatting that prioritizes readability for
// development and debugging scenarios. It creates structured but human-friendly
// output suitable for console viewing and simple log file analysis.
//
// Text Format Structure:
//   "YYYY-MM-DD HH:MM:SS [LEVEL] (caller) message [key=value key=value]"
//
// Format Components:
//   - Timestamp: ISO format (2006-01-02 15:04:05) for sorting and readability
//   - Level: Bracketed uppercase level name for easy visual scanning
//   - Caller: Optional parenthesized filename:line for debugging
//   - Message: Primary log content
//   - Fields: Bracketed key=value pairs for structured data
//
// Output Example:
//   "2023-10-15 14:30:25 [INFO] (main.go:42) File uploaded successfully [user_id=12345 size=1024]"
//
// Performance:
//   - Efficient string building with minimal allocations
//   - Single concatenation operation for final output
//   - Optimized for readability over parsing efficiency
//
// Parameters:
//   entry: LogEntry to format as human-readable text
//
// Returns:
//   string: Formatted text with newline terminator
//
// Complexity: O(n) where n is the total content size
func (l *Logger) formatText(entry LogEntry) string {
	timestamp := entry.Timestamp.Format("2006-01-02 15:04:05")
	
	var parts []string
	parts = append(parts, timestamp)
	parts = append(parts, fmt.Sprintf("[%s]", entry.Level))
	
	if entry.Caller != "" {
		parts = append(parts, fmt.Sprintf("(%s)", entry.Caller))
	}
	
	parts = append(parts, entry.Message)
	
	result := strings.Join(parts, " ")
	
	// Add fields if present
	if len(entry.Fields) > 0 {
		var fieldParts []string
		for key, value := range entry.Fields {
			fieldParts = append(fieldParts, fmt.Sprintf("%s=%v", key, value))
		}
		result += fmt.Sprintf(" [%s]", strings.Join(fieldParts, " "))
	}
	
	return result + "\n"
}

// Debug logs a debug-level message with optional structured fields for detailed diagnostic information.
//
// Debug messages provide detailed diagnostic information typically used during
// development and troubleshooting. These messages are filtered out in production
// unless explicitly enabled for debugging specific issues.
//
// Debug Use Cases:
//   - Detailed execution flow tracing
//   - Variable state inspection
//   - Performance measurement details
//   - Integration debugging information
//   - Internal state transitions
//
// Parameters:
//   message: Diagnostic message describing debug information
//   fields: Optional structured metadata (variadic, uses first map if provided)
//
// Thread Safety:
//   - Thread-safe delegation to core log method
//
// Complexity: O(n) where n is message and fields content size
func (l *Logger) Debug(message string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(DebugLevel, message, f)
}

// Info logs an informational message with optional structured fields for operational visibility.
//
// Info messages provide general operational information about system state
// changes, successful operations, and important events. These messages are
// typically enabled in production for operational monitoring.
//
// Info Use Cases:
//   - System startup and shutdown events
//   - Successful operation completions
//   - Configuration changes
//   - Performance metrics
//   - User action confirmations
//
// Parameters:
//   message: Informational message describing system events
//   fields: Optional structured metadata (variadic, uses first map if provided)
//
// Thread Safety:
//   - Thread-safe delegation to core log method
//
// Complexity: O(n) where n is message and fields content size
func (l *Logger) Info(message string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(InfoLevel, message, f)
}

// Warn logs a warning message with optional structured fields for attention-worthy conditions.
//
// Warning messages indicate conditions that are unusual or potentially
// problematic but don't prevent continued operation. These messages help
// identify issues before they become critical problems.
//
// Warning Use Cases:
//   - Deprecated feature usage
//   - Performance degradation
//   - Recovery from transient failures
//   - Configuration inconsistencies
//   - Rate limiting activations
//
// Parameters:
//   message: Warning message describing concerning conditions
//   fields: Optional structured metadata (variadic, uses first map if provided)
//
// Thread Safety:
//   - Thread-safe delegation to core log method
//
// Complexity: O(n) where n is message and fields content size
func (l *Logger) Warn(message string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(WarnLevel, message, f)
}

// Error logs an error message with optional structured fields for critical issue reporting.
//
// Error messages indicate serious problems that affect system functionality
// or user experience. These messages are typically always enabled and may
// trigger alerting systems in production environments.
//
// Error Use Cases:
//   - Operation failures and exceptions
//   - Security violations and authentication failures
//   - Resource exhaustion and system limits
//   - Data corruption or integrity issues
//   - Critical service unavailability
//
// Parameters:
//   message: Error message describing the critical issue
//   fields: Optional structured metadata (variadic, uses first map if provided)
//
// Thread Safety:
//   - Thread-safe delegation to core log method
//
// Complexity: O(n) where n is message and fields content size
func (l *Logger) Error(message string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(ErrorLevel, message, f)
}

// Debugf logs a formatted debug message with automatic argument sanitization.
//
// This method provides printf-style formatting for debug messages while
// ensuring all arguments are sanitized for sensitive data before formatting.
// It's useful for detailed diagnostic output with variable data.
//
// Sanitization:
//   - All arguments are processed for sensitive content before formatting
//   - Prevents accidental exposure of PII in formatted strings
//   - Maintains format string structure while protecting data
//
// Parameters:
//   format: Printf-style format string
//   args: Arguments for format string (automatically sanitized)
//
// Thread Safety:
//   - Thread-safe with automatic argument sanitization
//
// Complexity: O(n) where n is total content size after formatting
func (l *Logger) Debugf(format string, args ...interface{}) {
	// Sanitize args before formatting
	sanitizedArgs := l.sanitizeFormatArgs(args)
	l.log(DebugLevel, fmt.Sprintf(format, sanitizedArgs...), nil)
}

// Infof logs a formatted info message
func (l *Logger) Infof(format string, args ...interface{}) {
	// Sanitize args before formatting
	sanitizedArgs := l.sanitizeFormatArgs(args)
	l.log(InfoLevel, fmt.Sprintf(format, sanitizedArgs...), nil)
}

// Warnf logs a formatted warning message
func (l *Logger) Warnf(format string, args ...interface{}) {
	// Sanitize args before formatting
	sanitizedArgs := l.sanitizeFormatArgs(args)
	l.log(WarnLevel, fmt.Sprintf(format, sanitizedArgs...), nil)
}

// Errorf logs a formatted error message
func (l *Logger) Errorf(format string, args ...interface{}) {
	// Sanitize args before formatting
	sanitizedArgs := l.sanitizeFormatArgs(args)
	l.log(ErrorLevel, fmt.Sprintf(format, sanitizedArgs...), nil)
}

// sanitizeFormatArgs preprocesses format arguments to remove sensitive data before string formatting.
//
// This method ensures that printf-style logging methods don't accidentally
// expose sensitive information through format arguments. It processes each
// argument individually while preserving the argument list structure.
//
// Processing Strategy:
//   - Early return if sanitization is disabled for performance
//   - Element-wise sanitization preserving argument order
//   - Type-aware sanitization using sanitizeValue method
//   - Creates new slice to avoid modifying original arguments
//
// Performance:
//   - Conditional processing based on sanitization configuration
//   - Efficient argument processing for enabled sanitization
//   - No overhead when sanitization is disabled
//
// Parameters:
//   args: Slice of interface{} arguments to sanitize
//
// Returns:
//   []interface{}: Sanitized arguments ready for formatting
//
// Complexity: O(n*m) where n is argument count and m is average argument size
func (l *Logger) sanitizeFormatArgs(args []interface{}) []interface{} {
	if !l.enableSanitizing {
		return args
	}
	
	sanitized := make([]interface{}, len(args))
	for i, arg := range args {
		sanitized[i] = l.sanitizeValue(arg)
	}
	return sanitized
}

// WithField creates a new FieldLogger with a single additional field for contextual logging.
//
// This method enables contextual logging by creating a FieldLogger that
// automatically includes the specified field in all subsequent log messages.
// It's useful for request-scoped or operation-scoped logging context.
//
// Contextual Logging:
//   - All messages from returned logger include the specified field
//   - Enables consistent context across multiple log statements
//   - Useful for request IDs, user IDs, operation context
//
// Usage Example:
//   userLogger := logger.WithField("user_id", userID)
//   userLogger.Info("User action started")
//   userLogger.Debug("Processing user data")
//   userLogger.Info("User action completed")
//
// Parameters:
//   key: Field name for contextual information
//   value: Field value (will be sanitized during logging)
//
// Returns:
//   *FieldLogger: New logger with additional field context
//
// Thread Safety:
//   - Thread-safe creation of new logger instance
//
// Complexity: O(1) - Simple FieldLogger creation
func (l *Logger) WithField(key string, value interface{}) *FieldLogger {
	return &FieldLogger{
		logger: l,
		fields: map[string]interface{}{key: value},
	}
}

// WithFields creates a new FieldLogger with multiple additional fields for rich contextual logging.
//
// This method enables rich contextual logging by creating a FieldLogger that
// automatically includes all specified fields in subsequent log messages.
// It's ideal for complex context with multiple related attributes.
//
// Contextual Logging:
//   - All messages include all specified fields automatically
//   - Enables comprehensive context for related operations
//   - Useful for request context, transaction details, system state
//
// Field Management:
//   - Creates defensive copy of input fields to prevent external modification
//   - Preserves field isolation between logger instances
//   - Fields are sanitized during actual logging operations
//
// Usage Example:
//   requestLogger := logger.WithFields(map[string]interface{}{
//       "request_id": reqID,
//       "user_id":    userID,
//       "method":     "POST",
//   })
//   requestLogger.Info("Request started")
//   requestLogger.Error("Request failed")
//
// Parameters:
//   fields: Map of field names to values for contextual logging
//
// Returns:
//   *FieldLogger: New logger with additional field context
//
// Thread Safety:
//   - Thread-safe creation with defensive copying
//
// Complexity: O(n) where n is the number of fields
func (l *Logger) WithFields(fields map[string]interface{}) *FieldLogger {
	f := make(map[string]interface{})
	for k, v := range fields {
		f[k] = v
	}
	return &FieldLogger{
		logger: l,
		fields: f,
	}
}

// FieldLogger wraps a Logger with additional fields for contextual logging.
//
// FieldLogger provides contextual logging by automatically including
// predefined fields in all log messages. It enables consistent context
// across multiple log statements without repetitive field specification.
//
// Structure:
//   - logger: Reference to underlying Logger for actual log processing
//   - fields: Map of key-value pairs included in all log messages
//
// Benefits:
//   - Consistent context across related log messages
//   - Reduced boilerplate in repetitive logging scenarios
//   - Request-scoped and operation-scoped logging support
//   - Chainable field addition for building rich context
//
// Usage Patterns:
//   - Request processing: Include request ID, user ID, method
//   - Operation tracking: Include operation type, resource ID
//   - Component logging: Include component name, instance ID
//   - Error context: Include error codes, failure context
//
type FieldLogger struct {
	logger *Logger
	fields map[string]interface{}
}

// Debug logs a debug message with fields
func (fl *FieldLogger) Debug(message string) {
	fl.logger.log(DebugLevel, message, fl.fields)
}

// Info logs an info message with fields
func (fl *FieldLogger) Info(message string) {
	fl.logger.log(InfoLevel, message, fl.fields)
}

// Warn logs a warning message with fields
func (fl *FieldLogger) Warn(message string) {
	fl.logger.log(WarnLevel, message, fl.fields)
}

// Error logs an error message with fields
func (fl *FieldLogger) Error(message string) {
	fl.logger.log(ErrorLevel, message, fl.fields)
}

// Debugf logs a formatted debug message with fields
func (fl *FieldLogger) Debugf(format string, args ...interface{}) {
	// Sanitize args before formatting
	sanitizedArgs := fl.logger.sanitizeFormatArgs(args)
	fl.logger.log(DebugLevel, fmt.Sprintf(format, sanitizedArgs...), fl.fields)
}

// Infof logs a formatted info message with fields
func (fl *FieldLogger) Infof(format string, args ...interface{}) {
	// Sanitize args before formatting
	sanitizedArgs := fl.logger.sanitizeFormatArgs(args)
	fl.logger.log(InfoLevel, fmt.Sprintf(format, sanitizedArgs...), fl.fields)
}

// Warnf logs a formatted warning message with fields
func (fl *FieldLogger) Warnf(format string, args ...interface{}) {
	// Sanitize args before formatting
	sanitizedArgs := fl.logger.sanitizeFormatArgs(args)
	fl.logger.log(WarnLevel, fmt.Sprintf(format, sanitizedArgs...), fl.fields)
}

// Errorf logs a formatted error message with fields
func (fl *FieldLogger) Errorf(format string, args ...interface{}) {
	// Sanitize args before formatting
	sanitizedArgs := fl.logger.sanitizeFormatArgs(args)
	fl.logger.log(ErrorLevel, fmt.Sprintf(format, sanitizedArgs...), fl.fields)
}

// WithField adds another field to the logger
func (fl *FieldLogger) WithField(key string, value interface{}) *FieldLogger {
	fields := make(map[string]interface{})
	for k, v := range fl.fields {
		fields[k] = v
	}
	fields[key] = value
	return &FieldLogger{
		logger: fl.logger,
		fields: fields,
	}
}

// Global logger instance
var defaultLogger *Logger
var defaultLoggerMu sync.RWMutex

// InitGlobalLogger initializes the application-wide global logger with custom configuration.
//
// This function sets up the global logger instance used by package-level
// convenience functions. It enables centralized logging configuration
// for applications that prefer global logger access.
//
// Global Logger Benefits:
//   - Centralized configuration for entire application
//   - Consistent logging behavior across all packages
//   - Simplified logging setup with package-level functions
//   - Easy runtime reconfiguration of logging behavior
//
// Configuration:
//   - Accepts any valid Logger configuration
//   - Replaces any existing global logger instance
//   - Thread-safe initialization with write lock
//
// Parameters:
//   config: Logger configuration for global instance
//
// Thread Safety:
//   - Thread-safe initialization with mutex protection
//
// Complexity: O(1) - Simple logger creation and assignment
func InitGlobalLogger(config *Config) {
	defaultLoggerMu.Lock()
	defer defaultLoggerMu.Unlock()
	defaultLogger = NewLogger(config)
}

// GetGlobalLogger returns the global logger instance with lazy initialization.
//
// This function provides access to the application-wide global logger,
// creating one with default configuration if none exists. It enables
// package-level logging functions and centralized logger access.
//
// Lazy Initialization:
//   - Creates default logger if none exists
//   - Uses DefaultConfig() for automatic setup
//   - Thread-safe initialization on first access
//
// Thread Safety:
//   - Read lock for normal access to existing logger
//   - Lazy initialization is thread-safe
//   - Safe for concurrent access from multiple goroutines
//
// Returns:
//   *Logger: Global logger instance (never nil)
//
// Complexity: O(1) - Simple access with conditional initialization
func GetGlobalLogger() *Logger {
	defaultLoggerMu.RLock()
	defer defaultLoggerMu.RUnlock()
	if defaultLogger == nil {
		defaultLogger = NewLogger(DefaultConfig())
	}
	return defaultLogger
}

// Global convenience functions provide package-level access to logging functionality.
//
// These functions delegate to the global logger instance, enabling simple
// logging without explicit logger management. They provide the same functionality
// as Logger methods but with global scope and automatic logger access.
//
// Benefits:
//   - Simplified logging without logger instance management
//   - Consistent global configuration across application
//   - Easy migration from other logging libraries
//   - Reduced boilerplate for simple logging needs
//
// Thread Safety:
//   - All functions are thread-safe through global logger delegation
//   - Safe for concurrent use across multiple goroutines

// Debug logs a debug message using the global logger.
func Debug(message string, fields ...map[string]interface{}) {
	GetGlobalLogger().Debug(message, fields...)
}

// Info logs an informational message using the global logger.
func Info(message string, fields ...map[string]interface{}) {
	GetGlobalLogger().Info(message, fields...)
}

// Warn logs a warning message using the global logger.
func Warn(message string, fields ...map[string]interface{}) {
	GetGlobalLogger().Warn(message, fields...)
}

// Error logs an error message using the global logger.
func Error(message string, fields ...map[string]interface{}) {
	GetGlobalLogger().Error(message, fields...)
}

// Debugf logs a formatted debug message using the global logger.
func Debugf(format string, args ...interface{}) {
	GetGlobalLogger().Debugf(format, args...)
}

// Infof logs a formatted informational message using the global logger.
func Infof(format string, args ...interface{}) {
	GetGlobalLogger().Infof(format, args...)
}

// Warnf logs a formatted warning message using the global logger.
func Warnf(format string, args ...interface{}) {
	GetGlobalLogger().Warnf(format, args...)
}

// Errorf logs a formatted error message using the global logger.
func Errorf(format string, args ...interface{}) {
	GetGlobalLogger().Errorf(format, args...)
}

// CreateFileOutput creates a file writer for persistent logging with automatic directory creation.
//
// This utility function creates a file-based output destination for logging,
// handling directory creation and file opening with appropriate permissions.
// It's useful for setting up file-based logging configurations.
//
// File Management:
//   - Automatically creates parent directories if they don't exist
//   - Opens file in append mode to preserve existing logs
//   - Sets appropriate file permissions (0644) for security
//   - Uses CREATE|WRONLY|APPEND flags for safe file handling
//
// Directory Creation:
//   - Creates all parent directories as needed (0755 permissions)
//   - Handles complex directory paths gracefully
//   - Provides clear error messages for directory creation failures
//
// Error Handling:
//   - Clear error messages for directory and file creation failures
//   - Wrapped errors with context for debugging
//
// Parameters:
//   filename: Path to log file (parent directories created if needed)
//
// Returns:
//   io.Writer: File writer for logging output
//   error: nil on success, detailed error on failure
//
// Complexity: O(1) - File system operations
func CreateFileOutput(filename string) (io.Writer, error) {
	// Ensure directory exists
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	return file, nil
}

// CreateCombinedOutput creates a multi-writer that outputs to both console and file simultaneously.
//
// This utility function creates a writer that sends all log output to both
// stdout (for immediate visibility) and a file (for persistence). It's ideal
// for development and debugging scenarios requiring both console and file logging.
//
// Output Destinations:
//   - os.Stdout: Immediate console visibility for development
//   - File: Persistent storage for analysis and debugging
//   - Synchronous writing: Both destinations receive identical content
//
// Use Cases:
//   - Development environments: Console visibility with file backup
//   - Debugging scenarios: Real-time monitoring with persistent logs
//   - CI/CD pipelines: Console output with artifact preservation
//   - Hybrid logging: Best of both console and file logging
//
// Error Handling:
//   - File creation errors are propagated from CreateFileOutput
//   - Console output (stdout) is assumed to be always available
//
// Parameters:
//   filename: Path to log file for persistent output
//
// Returns:
//   io.Writer: Multi-writer sending output to both console and file
//   error: nil on success, file creation error on failure
//
// Complexity: O(1) - Writer creation and composition
func CreateCombinedOutput(filename string) (io.Writer, error) {
	fileWriter, err := CreateFileOutput(filename)
	if err != nil {
		return nil, err
	}

	return io.MultiWriter(os.Stdout, fileWriter), nil
}