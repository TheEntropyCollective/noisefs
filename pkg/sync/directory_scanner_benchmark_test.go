package sync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkDirectoryScanning(b *testing.B) {
	// Create temporary directory with various file sizes
	tempDir, err := os.MkdirTemp("", "scanner_bench")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test structure for benchmarking
	fileSizes := []int{10, 100, 1000, 10000}
	filesPerSize := []int{10, 50, 100}

	for _, size := range fileSizes {
		for _, count := range filesPerSize {
			b.Run(fmt.Sprintf("size_%db_count_%d", size, count), func(b *testing.B) {
				// Setup test directory
				testDir := filepath.Join(tempDir, fmt.Sprintf("bench_%d_%d", size, count))
				if err := os.MkdirAll(testDir, 0755); err != nil {
					b.Fatalf("Failed to create test dir: %v", err)
				}

				// Create test files
				content := make([]byte, size)
				for i := 0; i < len(content); i++ {
					content[i] = byte('A' + (i % 26))
				}

				for i := 0; i < count; i++ {
					filename := filepath.Join(testDir, fmt.Sprintf("file_%d.txt", i))
					if err := os.WriteFile(filename, content, 0644); err != nil {
						b.Fatalf("Failed to create test file: %v", err)
					}
				}

				// Benchmark the scanning
				scanner := NewDirectoryScanner(nil)
				
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					_, err := scanner.ScanLocalDirectory(testDir)
					if err != nil {
						b.Fatalf("Scan failed: %v", err)
					}
				}
			})
		}
	}
}

func BenchmarkChecksumCalculation(b *testing.B) {
	fileSizes := []int{1024, 10240, 102400, 1024000} // 1KB, 10KB, 100KB, 1MB

	for _, size := range fileSizes {
		b.Run(fmt.Sprintf("size_%dKB", size/1024), func(b *testing.B) {
			// Create temporary file
			tempFile, err := os.CreateTemp("", "checksum_bench")
			if err != nil {
				b.Fatalf("Failed to create temp file: %v", err)
			}
			defer os.Remove(tempFile.Name())

			// Write test data
			content := make([]byte, size)
			for i := range content {
				content[i] = byte(i % 256)
			}
			if _, err := tempFile.Write(content); err != nil {
				b.Fatalf("Failed to write test data: %v", err)
			}
			tempFile.Close()

			// Benchmark checksum calculation
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := CalculateFileChecksum(tempFile.Name())
				if err != nil {
					b.Fatalf("Checksum calculation failed: %v", err)
				}
			}
		})
	}
}

func BenchmarkInitialScanComplete(b *testing.B) {
	// Create a complex directory structure for comprehensive benchmarking
	tempDir, err := os.MkdirTemp("", "full_scan_bench")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a realistic directory structure
	dirs := []string{"docs", "data", "scripts", "config", "logs"}
	fileTypes := map[string][]byte{
		".txt":  []byte("Sample text content for benchmarking purposes"),
		".json": []byte(`{"key": "value", "number": 42, "array": [1, 2, 3]}`),
		".csv":  []byte("name,age,city\nJohn,30,NYC\nJane,25,LA\n"),
		".log":  []byte("2023-01-01 12:00:00 INFO Starting application\n2023-01-01 12:00:01 DEBUG Processing request\n"),
		".sh":   []byte("#!/bin/bash\necho 'Hello World'\ndate\n"),
	}

	// Create files in each directory
	for _, dir := range dirs {
		dirPath := filepath.Join(tempDir, dir)
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			b.Fatalf("Failed to create directory: %v", err)
		}

		// Create 20 files per directory with different extensions
		for i := 0; i < 20; i++ {
			for ext, content := range fileTypes {
				filename := filepath.Join(dirPath, fmt.Sprintf("file_%d%s", i, ext))
				if err := os.WriteFile(filename, content, 0644); err != nil {
					b.Fatalf("Failed to create file: %v", err)
				}
			}
		}
	}

	// Also create some nested directories
	nestedPath := filepath.Join(tempDir, "nested", "deep", "structure")
	if err := os.MkdirAll(nestedPath, 0755); err != nil {
		b.Fatalf("Failed to create nested structure: %v", err)
	}

	// Add files in nested structure
	for i := 0; i < 10; i++ {
		filename := filepath.Join(nestedPath, fmt.Sprintf("nested_file_%d.txt", i))
		content := fmt.Sprintf("Nested content %d", i)
		if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
			b.Fatalf("Failed to create nested file: %v", err)
		}
	}

	scanner := NewDirectoryScanner(nil)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := scanner.PerformInitialScan(ctx, tempDir, "/remote", "", nil)
		if err != nil {
			b.Fatalf("Initial scan failed: %v", err)
		}
	}
}

func BenchmarkOperationGeneration(b *testing.B) {
	scanner := NewDirectoryScanner(nil)

	// Create large number of changes to benchmark operation generation
	changes := make([]DetectedChange, 1000)
	for i := range changes {
		changes[i] = DetectedChange{
			Type:    ChangeTypeCreate,
			Path:    fmt.Sprintf("file_%d.txt", i),
			IsLocal: i%2 == 0, // Alternate between local and remote
			Metadata: FileMetadata{
				Path:  fmt.Sprintf("file_%d.txt", i),
				Size:  int64(i * 100),
				IsDir: i%10 == 0, // Every 10th is a directory
			},
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		operations := scanner.GenerateSyncOperations("bench-session", changes, "/local", "/remote")
		if len(operations) != len(changes) {
			b.Fatalf("Expected %d operations, got %d", len(changes), len(operations))
		}
	}
}

func BenchmarkStateComparison(b *testing.B) {
	// Create large snapshots for benchmarking state comparison
	localSnapshot := make(map[string]FileMetadata)
	remoteSnapshot := make(map[string]RemoteMetadata)

	// Create 1000 files in each snapshot with some overlaps and differences
	for i := 0; i < 1000; i++ {
		path := fmt.Sprintf("file_%d.txt", i)
		
		localSnapshot[path] = FileMetadata{
			Path:     path,
			Size:     int64(i * 100),
			IsDir:    false,
			Checksum: fmt.Sprintf("checksum_%d", i),
		}

		// Only add every other file to remote to create differences
		if i%2 == 0 {
			remoteSnapshot[path] = RemoteMetadata{
				Path:          path,
				DescriptorCID: fmt.Sprintf("Qm%d", i),
				Size:          int64(i * 100),
				IsDir:         false,
			}
		}
	}

	scanner := NewDirectoryScanner(nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		changes := scanner.generateInitialChanges(localSnapshot, remoteSnapshot)
		if len(changes) == 0 {
			b.Fatal("Expected changes to be generated")
		}
	}
}