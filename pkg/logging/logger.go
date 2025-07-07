package logging

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	mu         sync.RWMutex
	level      LogLevel
	format     LogFormat
	output     io.Writer
	showCaller bool
	component  string
}

// Config holds logger configuration
type Config struct {
	Level      LogLevel
	Format     LogFormat
	Output     io.Writer
	ShowCaller bool
	Component  string
}

// DefaultConfig returns a default logger configuration
func DefaultConfig() *Config {
	return &Config{
		Level:      InfoLevel,
		Format:     TextFormat,
		Output:     os.Stdout,
		ShowCaller: false,
		Component:  "",
	}
}

// NewLogger creates a new logger with the given configuration
func NewLogger(config *Config) *Logger {
	if config == nil {
		config = DefaultConfig()
	}

	return &Logger{
		level:      config.Level,
		format:     config.Format,
		output:     config.Output,
		showCaller: config.ShowCaller,
		component:  config.Component,
	}
}

// WithComponent returns a new logger with the specified component name
func (l *Logger) WithComponent(component string) *Logger {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return &Logger{
		level:      l.level,
		format:     l.format,
		output:     l.output,
		showCaller: l.showCaller,
		component:  component,
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
	l.log(DebugLevel, fmt.Sprintf(format, args...), nil)
}

// Infof logs a formatted info message
func (l *Logger) Infof(format string, args ...interface{}) {
	l.log(InfoLevel, fmt.Sprintf(format, args...), nil)
}

// Warnf logs a formatted warning message
func (l *Logger) Warnf(format string, args ...interface{}) {
	l.log(WarnLevel, fmt.Sprintf(format, args...), nil)
}

// Errorf logs a formatted error message
func (l *Logger) Errorf(format string, args ...interface{}) {
	l.log(ErrorLevel, fmt.Sprintf(format, args...), nil)
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
	fl.logger.log(DebugLevel, fmt.Sprintf(format, args...), fl.fields)
}

// Infof logs a formatted info message with fields
func (fl *FieldLogger) Infof(format string, args ...interface{}) {
	fl.logger.log(InfoLevel, fmt.Sprintf(format, args...), fl.fields)
}

// Warnf logs a formatted warning message with fields
func (fl *FieldLogger) Warnf(format string, args ...interface{}) {
	fl.logger.log(WarnLevel, fmt.Sprintf(format, args...), fl.fields)
}

// Errorf logs a formatted error message with fields
func (fl *FieldLogger) Errorf(format string, args ...interface{}) {
	fl.logger.log(ErrorLevel, fmt.Sprintf(format, args...), fl.fields)
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