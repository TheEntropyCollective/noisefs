// Package logging configuration utilities provide high-level logger setup from string parameters.
//
// This module complements the core logging package by providing convenient
// configuration functions that parse string-based settings into fully
// configured Logger instances. It's designed for easy integration with
// configuration files, environment variables, and command-line arguments.
//
// Key Features:
//   - String-based configuration parsing for external integration
//   - Support for console, file, and combined output destinations
//   - Automatic file creation and directory handling
//   - Global logger initialization from configuration
//   - Error handling with descriptive messages
//
// Configuration Sources:
//   - Environment variables: LOG_LEVEL, LOG_FORMAT, LOG_OUTPUT
//   - Configuration files: JSON, YAML, TOML configuration
//   - Command-line flags: --log-level, --log-format, --log-output
//   - Application settings: Programmatic configuration
//
// Supported Options:
//   - Level: "debug", "info", "warn", "error"
//   - Format: "text", "json"
//   - Output: "console", "file", "both"
//   - Filename: Required for "file" and "both" outputs
//
package logging

import (
	"fmt"
	"io"
	"os"
)

// ConfigureFromSettings creates a fully configured Logger from string-based parameters.
//
// This function provides a high-level interface for creating Logger instances
// from string configuration values, making it easy to integrate logging
// configuration with external configuration systems and user interfaces.
//
// Configuration Parameters:
//   - level: "debug", "info", "warn", "error" (case-insensitive)
//   - format: "text" (human-readable) or "json" (structured)
//   - output: "console" (stdout), "file" (filename), "both" (console + file)
//   - filename: Required for "file" and "both" outputs, ignored for "console"
//
// Output Destination Behavior:
//   - "console": Writes to os.Stdout for immediate visibility
//   - "file": Writes to specified file with automatic directory creation
//   - "both": Writes to both console and file simultaneously
//
// Error Conditions:
//   - Invalid log level strings return parsing errors
//   - Invalid format strings return validation errors
//   - Invalid output types return validation errors
//   - Missing filename for file outputs returns configuration errors
//   - File creation failures return filesystem errors
//
// Default Configuration:
//   - ShowCaller: false (disabled for cleaner output)
//   - Component: "" (no default component)
//   - EnableSanitizing: true (inherited from DefaultConfig)
//
// Integration Examples:
//   // Environment variable configuration
//   logger, err := ConfigureFromSettings(
//       os.Getenv("LOG_LEVEL"),
//       os.Getenv("LOG_FORMAT"),
//       os.Getenv("LOG_OUTPUT"),
//       os.Getenv("LOG_FILE"),
//   )
//   
//   // Configuration file integration
//   logger, err := ConfigureFromSettings(
//       config.Logging.Level,
//       config.Logging.Format,
//       config.Logging.Output,
//       config.Logging.Filename,
//   )
//
// Parameters:
//   level: Log level as string ("debug", "info", "warn", "error")
//   format: Output format ("text", "json")
//   output: Output destination ("console", "file", "both")
//   filename: File path for file-based outputs (required for "file"/"both")
//
// Returns:
//   *Logger: Configured logger instance ready for use
//   error: Configuration or file creation errors, nil on success
//
// Complexity: O(1) - Simple parsing and logger creation
func ConfigureFromSettings(level, format, output, filename string) (*Logger, error) {
	// Parse log level
	logLevel, err := ParseLogLevel(level)
	if err != nil {
		return nil, fmt.Errorf("invalid log level: %w", err)
	}

	// Parse log format
	var logFormat LogFormat
	switch format {
	case "json":
		logFormat = JSONFormat
	case "text":
		logFormat = TextFormat
	default:
		return nil, fmt.Errorf("invalid log format: %s", format)
	}

	// Configure output
	var writer io.Writer
	switch output {
	case "console":
		writer = os.Stdout
	case "file":
		if filename == "" {
			return nil, fmt.Errorf("log file path required when output is 'file'")
		}
		fileWriter, err := CreateFileOutput(filename)
		if err != nil {
			return nil, fmt.Errorf("failed to create file output: %w", err)
		}
		writer = fileWriter
	case "both":
		if filename == "" {
			return nil, fmt.Errorf("log file path required when output is 'both'")
		}
		combinedWriter, err := CreateCombinedOutput(filename)
		if err != nil {
			return nil, fmt.Errorf("failed to create combined output: %w", err)
		}
		writer = combinedWriter
	default:
		return nil, fmt.Errorf("invalid log output: %s", output)
	}

	config := &Config{
		Level:      logLevel,
		Format:     logFormat,
		Output:     writer,
		ShowCaller: false,
		Component:  "",
	}

	return NewLogger(config), nil
}

// InitFromConfig initializes the global logger from string-based configuration parameters.
//
// This function provides a convenient way to configure the application-wide
// global logger from external configuration sources. It combines the
// configuration parsing of ConfigureFromSettings with global logger initialization.
//
// Global Logger Benefits:
//   - Centralized logging configuration for entire application
//   - Package-level logging functions use configured settings
//   - Consistent logging behavior across all application components
//   - Single point of configuration for application logging
//
// Configuration Flow:
//   1. Parse configuration parameters using ConfigureFromSettings
//   2. Extract configuration from created logger instance
//   3. Initialize global logger with extracted configuration
//   4. Enable package-level logging functions (Debug, Info, Warn, Error)
//
// Error Handling:
//   - All configuration errors from ConfigureFromSettings are propagated
//   - File creation errors are propagated with context
//   - Validation errors include specific parameter information
//
// Usage Scenarios:
//   - Application startup: Configure logging from environment/config files
//   - Runtime reconfiguration: Update global logging behavior
//   - Testing: Set up test-specific logging configuration
//   - CLI applications: Configure logging from command-line flags
//
// Integration Examples:
//   // Application main function
//   if err := InitFromConfig("info", "json", "both", "/var/log/app.log"); err != nil {
//       log.Fatal("Failed to configure logging:", err)
//   }
//   
//   // Environment-based configuration
//   err := InitFromConfig(
//       getEnvWithDefault("LOG_LEVEL", "info"),
//       getEnvWithDefault("LOG_FORMAT", "text"),
//       getEnvWithDefault("LOG_OUTPUT", "console"),
//       os.Getenv("LOG_FILE"),
//   )
//
// Parameters:
//   level: Log level string ("debug", "info", "warn", "error")
//   format: Output format ("text", "json")
//   output: Output destination ("console", "file", "both")
//   filename: File path for file outputs (required for "file"/"both")
//
// Returns:
//   error: Configuration errors or nil on successful initialization
//
// Side Effects:
//   - Replaces any existing global logger configuration
//   - Affects all subsequent package-level logging function calls
//
// Complexity: O(1) - Delegation to ConfigureFromSettings plus initialization
func InitFromConfig(level, format, output, filename string) error {
	logger, err := ConfigureFromSettings(level, format, output, filename)
	if err != nil {
		return err
	}

	InitGlobalLogger(&Config{
		Level:      logger.level,
		Format:     logger.format,
		Output:     logger.output,
		ShowCaller: logger.showCaller,
		Component:  logger.component,
	})

	return nil
}