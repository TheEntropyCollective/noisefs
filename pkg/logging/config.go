package logging

import (
	"fmt"
	"io"
	"os"
)

// ConfigureFromSettings configures a logger from settings
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

// InitFromConfig initializes the global logger from configuration settings
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