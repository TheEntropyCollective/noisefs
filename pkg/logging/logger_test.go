package logging

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogLevels(t *testing.T) {
	buf := &bytes.Buffer{}
	config := &Config{
		Level:  InfoLevel,
		Format: TextFormat,
		Output: buf,
	}
	logger := NewLogger(config)

	// Debug should not appear (below threshold)
	logger.Debug("debug message")
	if buf.Len() > 0 {
		t.Error("Debug message should not appear when level is Info")
	}

	// Info should appear
	logger.Info("info message")
	if buf.Len() == 0 {
		t.Error("Info message should appear when level is Info")
	}

	// Check content
	output := buf.String()
	if !strings.Contains(output, "info message") {
		t.Error("Output should contain the info message")
	}
	if !strings.Contains(output, "[INFO]") {
		t.Error("Output should contain the INFO level")
	}
}

func TestJSONFormat(t *testing.T) {
	buf := &bytes.Buffer{}
	config := &Config{
		Level:  InfoLevel,
		Format: JSONFormat,
		Output: buf,
	}
	logger := NewLogger(config)

	logger.Info("test message", map[string]interface{}{
		"key1": "value1",
		"key2": 42,
	})

	// Parse JSON output
	var entry LogEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("Failed to parse JSON output: %v", err)
	}

	if entry.Level != "INFO" {
		t.Errorf("Expected level INFO, got %s", entry.Level)
	}
	if entry.Message != "test message" {
		t.Errorf("Expected message 'test message', got %s", entry.Message)
	}
	if entry.Fields["key1"] != "value1" {
		t.Errorf("Expected field key1=value1, got %v", entry.Fields["key1"])
	}
	if entry.Fields["key2"] != float64(42) { // JSON numbers are float64
		t.Errorf("Expected field key2=42, got %v", entry.Fields["key2"])
	}
}

func TestWithFields(t *testing.T) {
	buf := &bytes.Buffer{}
	config := &Config{
		Level:  InfoLevel,
		Format: JSONFormat,
		Output: buf,
	}
	logger := NewLogger(config)

	fieldLogger := logger.WithFields(map[string]interface{}{
		"component": "test",
		"version":   "1.0",
	})

	fieldLogger.Info("test message")

	var entry LogEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("Failed to parse JSON output: %v", err)
	}

	if entry.Fields["component"] != "test" {
		t.Errorf("Expected component=test, got %v", entry.Fields["component"])
	}
	if entry.Fields["version"] != "1.0" {
		t.Errorf("Expected version=1.0, got %v", entry.Fields["version"])
	}
}

func TestComponent(t *testing.T) {
	buf := &bytes.Buffer{}
	config := &Config{
		Level:     InfoLevel,
		Format:    JSONFormat,
		Output:    buf,
		Component: "noisefs",
	}
	logger := NewLogger(config)

	logger.Info("test message")

	var entry LogEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("Failed to parse JSON output: %v", err)
	}

	if entry.Fields["component"] != "noisefs" {
		t.Errorf("Expected component=noisefs, got %v", entry.Fields["component"])
	}
}

func TestFormatMethods(t *testing.T) {
	buf := &bytes.Buffer{}
	config := &Config{
		Level:  InfoLevel,
		Format: TextFormat,
		Output: buf,
	}
	logger := NewLogger(config)

	logger.Infof("formatted %s with %d", "message", 42)
	
	output := buf.String()
	if !strings.Contains(output, "formatted message with 42") {
		t.Error("Formatted message not correct")
	}
}

func TestFileOutput(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "logging_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logFile := filepath.Join(tmpDir, "test.log")
	
	fileWriter, err := CreateFileOutput(logFile)
	if err != nil {
		t.Fatalf("Failed to create file output: %v", err)
	}

	config := &Config{
		Level:  InfoLevel,
		Format: TextFormat,
		Output: fileWriter,
	}
	logger := NewLogger(config)

	logger.Info("test message to file")

	// Read file contents
	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if !strings.Contains(string(content), "test message to file") {
		t.Error("Log file should contain the test message")
	}
}

func TestConfigureFromSettings(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "logging_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logFile := filepath.Join(tmpDir, "test.log")

	logger, err := ConfigureFromSettings("debug", "json", "file", logFile)
	if err != nil {
		t.Fatalf("Failed to configure logger: %v", err)
	}

	logger.Debug("debug message")
	logger.Info("info message")

	// Check if messages were written to file
	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if !strings.Contains(string(content), "debug message") {
		t.Error("Log file should contain debug message")
	}
	if !strings.Contains(string(content), "info message") {
		t.Error("Log file should contain info message")
	}
}