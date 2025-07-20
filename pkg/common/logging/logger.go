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

// LogLevel represents different logging levels
type LogLevel int

const (
	DebugLevel LogLevel = iota
	InfoLevel
	WarnLevel
	ErrorLevel
)

// String returns the string representation of the log level
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

// ParseLogLevel parses a string into a LogLevel
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

// LogFormat represents different log output formats
type LogFormat int

const (
	TextFormat LogFormat = iota
	JSONFormat
)

// LogEntry represents a single log entry
type LogEntry struct {
	Timestamp time.Time         `json:"timestamp"`
	Level     string            `json:"level"`
	Message   string            `json:"message"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
	Caller    string            `json:"caller,omitempty"`
}

// Logger provides structured logging functionality
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

// Config holds logger configuration
type Config struct {
	Level            LogLevel
	Format           LogFormat
	Output           io.Writer
	ShowCaller       bool
	Component        string
	EnableSanitizing bool
}

// DefaultConfig returns a default logger configuration
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

// NewLogger creates a new logger with the given configuration
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

// WithComponent returns a new logger with the specified component name
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

// SetLevel sets the logging level
func (l *Logger) SetLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// SetOutput sets the output writer
func (l *Logger) SetOutput(output io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.output = output
}

// IsEnabled checks if a log level is enabled
func (l *Logger) IsEnabled(level LogLevel) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return level >= l.level
}

// SanitizeLogEntry sanitizes sensitive data from a log entry
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

// isSensitiveFieldName checks if a field name suggests sensitive data
func (l *Logger) isSensitiveFieldName(fieldName string) bool {
	return sensitiveFieldPattern.MatchString(fieldName)
}

// sanitizeValue sanitizes a single value
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

// sanitizeString sanitizes sensitive patterns in a string
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

// SetSanitizing enables or disables sensitive data sanitizing
func (l *Logger) SetSanitizing(enabled bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.enableSanitizing = enabled
}

// log writes a log entry
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

// formatText formats a log entry as text
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

// Debug logs a debug message
func (l *Logger) Debug(message string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(DebugLevel, message, f)
}

// Info logs an info message
func (l *Logger) Info(message string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(InfoLevel, message, f)
}

// Warn logs a warning message
func (l *Logger) Warn(message string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(WarnLevel, message, f)
}

// Error logs an error message
func (l *Logger) Error(message string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(ErrorLevel, message, f)
}

// Debugf logs a formatted debug message
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

// sanitizeFormatArgs sanitizes format arguments
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

// WithField returns a new logger with the specified field
func (l *Logger) WithField(key string, value interface{}) *FieldLogger {
	return &FieldLogger{
		logger: l,
		fields: map[string]interface{}{key: value},
	}
}

// WithFields returns a new logger with the specified fields
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

// FieldLogger wraps a logger with additional fields
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

// InitGlobalLogger initializes the global logger
func InitGlobalLogger(config *Config) {
	defaultLoggerMu.Lock()
	defer defaultLoggerMu.Unlock()
	defaultLogger = NewLogger(config)
}

// GetGlobalLogger returns the global logger
func GetGlobalLogger() *Logger {
	defaultLoggerMu.RLock()
	defer defaultLoggerMu.RUnlock()
	if defaultLogger == nil {
		defaultLogger = NewLogger(DefaultConfig())
	}
	return defaultLogger
}

// Global convenience functions
func Debug(message string, fields ...map[string]interface{}) {
	GetGlobalLogger().Debug(message, fields...)
}

func Info(message string, fields ...map[string]interface{}) {
	GetGlobalLogger().Info(message, fields...)
}

func Warn(message string, fields ...map[string]interface{}) {
	GetGlobalLogger().Warn(message, fields...)
}

func Error(message string, fields ...map[string]interface{}) {
	GetGlobalLogger().Error(message, fields...)
}

func Debugf(format string, args ...interface{}) {
	GetGlobalLogger().Debugf(format, args...)
}

func Infof(format string, args ...interface{}) {
	GetGlobalLogger().Infof(format, args...)
}

func Warnf(format string, args ...interface{}) {
	GetGlobalLogger().Warnf(format, args...)
}

func Errorf(format string, args ...interface{}) {
	GetGlobalLogger().Errorf(format, args...)
}

// CreateFileOutput creates a file writer for logging
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

// CreateCombinedOutput creates a writer that writes to both console and file
func CreateCombinedOutput(filename string) (io.Writer, error) {
	fileWriter, err := CreateFileOutput(filename)
	if err != nil {
		return nil, err
	}

	return io.MultiWriter(os.Stdout, fileWriter), nil
}