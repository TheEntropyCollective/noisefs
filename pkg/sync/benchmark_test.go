package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func BenchmarkCalculateFileChecksum(b *testing.B) {
	// Create test files of different sizes
	tmpDir := b.TempDir()

	sizes := []int{1024, 10240, 102400, 1048576} // 1KB, 10KB, 100KB, 1MB

	for _, size := range sizes {
		b.Run(fmt.Sprintf("size_%dB", size), func(b *testing.B) {
			// Create test file
			testFile := filepath.Join(tmpDir, fmt.Sprintf("test_%d.bin", size))
			data := make([]byte, size)
			for i := range data {
				data[i] = byte(i % 256)
			}
			if err := os.WriteFile(testFile, data, 0644); err != nil {
				b.Fatalf("Failed to create test file: %v", err)
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := CalculateFileChecksum(testFile)
				if err != nil {
					b.Fatalf("Failed to calculate checksum: %v", err)
				}
			}
		})
	}
}

func BenchmarkGatherDirectoryMetadata(b *testing.B) {
	// Create test directory with multiple files
	tmpDir := b.TempDir()

	// Create different numbers of files
	fileCounts := []int{10, 50, 100, 500}

	for _, count := range fileCounts {
		b.Run(fmt.Sprintf("files_%d", count), func(b *testing.B) {
			// Create test files
			testDir := filepath.Join(tmpDir, fmt.Sprintf("test_%d", count))
			if err := os.MkdirAll(testDir, 0755); err != nil {
				b.Fatalf("Failed to create test dir: %v", err)
			}

			for i := 0; i < count; i++ {
				testFile := filepath.Join(testDir, fmt.Sprintf("file_%d.txt", i))
				content := fmt.Sprintf("Test content for file %d", i)
				if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
					b.Fatalf("Failed to create test file: %v", err)
				}
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := GatherDirectoryMetadata(testDir, true)
				if err != nil {
					b.Fatalf("Failed to gather metadata: %v", err)
				}
			}
		})
	}
}

func BenchmarkStateComparator(b *testing.B) {
	comparator := NewStateComparator()

	// Create snapshots with different numbers of files
	fileCounts := []int{100, 500, 1000, 5000}

	for _, count := range fileCounts {
		b.Run(fmt.Sprintf("files_%d", count), func(b *testing.B) {
			// Create old snapshot
			oldFiles := make(map[string]FileMetadata)
			for i := 0; i < count; i++ {
				path := fmt.Sprintf("file_%d.txt", i)
				oldFiles[path] = FileMetadata{
					Path:     path,
					Size:     int64(100 + i),
					ModTime:  time.Now().Add(-time.Hour),
					Checksum: fmt.Sprintf("checksum_%d", i),
				}
			}

			// Create new snapshot with some modifications
			newFiles := make(map[string]FileMetadata)
			for i := 0; i < count; i++ {
				path := fmt.Sprintf("file_%d.txt", i)

				// Modify 10% of files
				if i%10 == 0 {
					newFiles[path] = FileMetadata{
						Path:     path,
						Size:     int64(200 + i),
						ModTime:  time.Now(),
						Checksum: fmt.Sprintf("modified_checksum_%d", i),
					}
				} else {
					newFiles[path] = FileMetadata{
						Path:     path,
						Size:     int64(100 + i),
						ModTime:  time.Now().Add(-time.Hour),
						Checksum: fmt.Sprintf("checksum_%d", i),
					}
				}
			}

			oldSnapshot := &StateSnapshot{
				LocalFiles:  oldFiles,
				RemoteFiles: make(map[string]RemoteMetadata),
				Timestamp:   time.Now().Add(-time.Hour),
			}

			newSnapshot := &StateSnapshot{
				LocalFiles:  newFiles,
				RemoteFiles: make(map[string]RemoteMetadata),
				Timestamp:   time.Now(),
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := comparator.CompareStates(oldSnapshot, newSnapshot)
				if err != nil {
					b.Fatalf("Failed to compare states: %v", err)
				}
			}
		})
	}
}

func BenchmarkMoveDetection(b *testing.B) {
	detector := NewMoveDetector()

	// Create scenarios with different numbers of moved files
	moveCounts := []int{10, 50, 100, 500}

	for _, count := range moveCounts {
		b.Run(fmt.Sprintf("moves_%d", count), func(b *testing.B) {
			// Create old snapshot
			oldFiles := make(map[string]FileMetadata)
			for i := 0; i < count; i++ {
				path := fmt.Sprintf("old/file_%d.txt", i)
				oldFiles[path] = FileMetadata{
					Path:     path,
					Size:     int64(100 + i),
					ModTime:  time.Now(),
					Checksum: fmt.Sprintf("checksum_%d", i),
					Inode:    uint64(1000 + i),
					Device:   12345,
				}
			}

			// Create new snapshot with files moved
			newFiles := make(map[string]FileMetadata)
			for i := 0; i < count; i++ {
				path := fmt.Sprintf("new/file_%d.txt", i)
				newFiles[path] = FileMetadata{
					Path:     path,
					Size:     int64(100 + i),
					ModTime:  time.Now(),
					Checksum: fmt.Sprintf("checksum_%d", i),
					Inode:    uint64(1000 + i),
					Device:   12345,
				}
			}

			oldSnapshot := &StateSnapshot{
				LocalFiles:  oldFiles,
				RemoteFiles: make(map[string]RemoteMetadata),
			}

			newSnapshot := &StateSnapshot{
				LocalFiles:  newFiles,
				RemoteFiles: make(map[string]RemoteMetadata),
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = detector.DetectMoves(oldSnapshot, newSnapshot)
			}
		})
	}
}

func BenchmarkTransactionCommit(b *testing.B) {
	tmpDir := b.TempDir()
	stateDir := filepath.Join(tmpDir, "state")

	stateStore, err := NewSyncStateStore(stateDir)
	if err != nil {
		b.Fatalf("Failed to create state store: %v", err)
	}

	syncID := "bench_sync"
	if err := stateStore.CreateInitialState(syncID, "/local", "/remote"); err != nil {
		b.Fatalf("Failed to create initial state: %v", err)
	}

	// Test transaction performance with different operation counts
	opCounts := []int{1, 10, 50, 100}

	for _, count := range opCounts {
		b.Run(fmt.Sprintf("ops_%d", count), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				tx, err := stateStore.txManager.BeginTransaction(syncID)
				if err != nil {
					b.Fatalf("Failed to begin transaction: %v", err)
				}

				// Add operations
				for j := 0; j < count; j++ {
					metadata := FileMetadata{
						Path:     fmt.Sprintf("file_%d_%d.txt", i, j),
						Size:     int64(j * 100),
						Checksum: fmt.Sprintf("checksum_%d_%d", i, j),
					}

					err := stateStore.txManager.AddOperation(tx, TxOpUpdateLocalSnapshot, metadata.Path, nil, metadata)
					if err != nil {
						b.Fatalf("Failed to add operation: %v", err)
					}
				}

				// Commit transaction
				err = stateStore.txManager.CommitTransaction(tx)
				if err != nil {
					b.Fatalf("Failed to commit transaction: %v", err)
				}
			}
		})
	}
}
