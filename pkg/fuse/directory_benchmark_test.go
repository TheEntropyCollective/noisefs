// +build fuse

package fuse

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/descriptors"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
	storagetesting "github.com/TheEntropyCollective/noisefs/pkg/storage/testing"
	noisefs "github.com/TheEntropyCollective/noisefs/pkg/core/client"
)

// BenchmarkDirectoryOperations benchmarks various directory operations
func BenchmarkDirectoryOperations(b *testing.B) {
	// Setup once
	mountDir, storageManager, client := setupBenchmarkEnvironment(b)
	defer os.RemoveAll(mountDir)
	defer storageManager.Stop(context.Background())

	// Create test directories of various sizes
	smallDirCID := createBenchmarkDirectory(b, storageManager, 10)      // 10 files
	mediumDirCID := createBenchmarkDirectory(b, storageManager, 100)    // 100 files
	largeDirCID := createBenchmarkDirectory(b, storageManager, 1000)    // 1000 files
	xlargeDirCID := createBenchmarkDirectory(b, storageManager, 10000)  // 10000 files

	b.Run("ListSmallDirectory", func(b *testing.B) {
		benchmarkDirectoryListing(b, mountDir, storageManager, client, smallDirCID, 10)
	})

	b.Run("ListMediumDirectory", func(b *testing.B) {
		benchmarkDirectoryListing(b, mountDir, storageManager, client, mediumDirCID, 100)
	})

	b.Run("ListLargeDirectory", func(b *testing.B) {
		benchmarkDirectoryListing(b, mountDir, storageManager, client, largeDirCID, 1000)
	})

	b.Run("ListXLargeDirectory", func(b *testing.B) {
		benchmarkDirectoryListing(b, mountDir, storageManager, client, xlargeDirCID, 10000)
	})

	b.Run("RandomFileAccess", func(b *testing.B) {
		benchmarkRandomFileAccess(b, mountDir, storageManager, client, largeDirCID, 1000)
	})

	b.Run("SequentialFileAccess", func(b *testing.B) {
		benchmarkSequentialFileAccess(b, mountDir, storageManager, client, mediumDirCID, 100)
	})

	b.Run("ConcurrentDirectoryAccess", func(b *testing.B) {
		benchmarkConcurrentAccess(b, mountDir, storageManager, client, mediumDirCID)
	})

	b.Run("DirectoryTraversal", func(b *testing.B) {
		benchmarkDirectoryTraversal(b, mountDir, storageManager, client)
	})
}

// BenchmarkDirectoryCache benchmarks the directory manifest cache
func BenchmarkDirectoryCache(b *testing.B) {
	storageManager, err := storagetesting.CreateRealTestStorageManager()
	if err != nil {
		b.Skipf("Skipping cache benchmark - storage setup failed: %v", err)
	}
	defer storageManager.Stop(context.Background())

	// Create caches of different sizes
	configs := []struct {
		name string
		size int
	}{
		{"SmallCache", 10},
		{"MediumCache", 100},
		{"LargeCache", 1000},
	}

	for _, config := range configs {
		b.Run(config.name+"_Put", func(b *testing.B) {
			benchmarkCachePut(b, storageManager, config.size)
		})

		b.Run(config.name+"_Get", func(b *testing.B) {
			benchmarkCacheGet(b, storageManager, config.size)
		})

		b.Run(config.name+"_Eviction", func(b *testing.B) {
			benchmarkCacheEviction(b, storageManager, config.size)
		})
	}

	b.Run("CacheConcurrency", func(b *testing.B) {
		benchmarkCacheConcurrency(b, storageManager)
	})
}

// BenchmarkMountPerformance benchmarks mounting operations
func BenchmarkMountPerformance(b *testing.B) {
	storageManager, err := storagetesting.CreateRealTestStorageManager()
	if err != nil {
		b.Skipf("Skipping mount benchmark - storage setup failed: %v", err)
	}
	defer storageManager.Stop(context.Background())

	b.Run("SingleDirectoryMount", func(b *testing.B) {
		benchmarkSingleMount(b, storageManager)
	})

	b.Run("MultipleDirectoryMount", func(b *testing.B) {
		benchmarkMultipleMount(b, storageManager)
	})

	b.Run("LargeDirectoryMount", func(b *testing.B) {
		benchmarkLargeDirectoryMount(b, storageManager)
	})
}

// Benchmark implementations

func benchmarkDirectoryListing(b *testing.B, mountDir string, storageManager *storage.Manager, client *noisefs.Client, dirCID string, expectedFiles int) {
	// Mount directory
	subMount := filepath.Join(mountDir, fmt.Sprintf("bench_%d", expectedFiles))
	os.MkdirAll(subMount, 0755)

	opts := MountOptions{
		MountPath:           subMount,
		VolumeName:          fmt.Sprintf("bench_%d", expectedFiles),
		DirectoryDescriptor: dirCID,
	}

	go func() {
		MountWithIndex(client, storageManager, opts, "")
	}()
	time.Sleep(2 * time.Second)

	dirPath := filepath.Join(subMount, "mounted-dir")

	// Warm up
	os.ReadDir(dirPath)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		entries, err := os.ReadDir(dirPath)
		if err != nil {
			b.Fatalf("Failed to read directory: %v", err)
		}
		if len(entries) != expectedFiles {
			b.Fatalf("Expected %d files, got %d", expectedFiles, len(entries))
		}
	}

	b.ReportMetric(float64(expectedFiles), "files/op")
}

func benchmarkRandomFileAccess(b *testing.B, mountDir string, storageManager *storage.Manager, client *noisefs.Client, dirCID string, fileCount int) {
	// Mount directory
	subMount := filepath.Join(mountDir, "bench_random")
	os.MkdirAll(subMount, 0755)

	opts := MountOptions{
		MountPath:           subMount,
		VolumeName:          "bench_random",
		DirectoryDescriptor: dirCID,
	}

	go func() {
		MountWithIndex(client, storageManager, opts, "")
	}()
	time.Sleep(2 * time.Second)

	dirPath := filepath.Join(subMount, "mounted-dir")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Access random file
		fileNum := i % fileCount
		filename := fmt.Sprintf("file_%04d.txt", fileNum)
		path := filepath.Join(dirPath, filename)
		
		info, err := os.Stat(path)
		if err != nil {
			b.Fatalf("Failed to stat %s: %v", filename, err)
		}
		if info.Size() == 0 {
			b.Errorf("File %s has zero size", filename)
		}
	}
}

func benchmarkSequentialFileAccess(b *testing.B, mountDir string, storageManager *storage.Manager, client *noisefs.Client, dirCID string, fileCount int) {
	// Mount directory
	subMount := filepath.Join(mountDir, "bench_sequential")
	os.MkdirAll(subMount, 0755)

	opts := MountOptions{
		MountPath:           subMount,
		VolumeName:          "bench_sequential",
		DirectoryDescriptor: dirCID,
	}

	go func() {
		MountWithIndex(client, storageManager, opts, "")
	}()
	time.Sleep(2 * time.Second)

	dirPath := filepath.Join(subMount, "mounted-dir")

	b.ResetTimer()
	b.ReportAllocs()

	fileIndex := 0
	for i := 0; i < b.N; i++ {
		// Access files sequentially
		filename := fmt.Sprintf("file_%04d.txt", fileIndex)
		path := filepath.Join(dirPath, filename)
		
		_, err := os.Stat(path)
		if err != nil {
			b.Fatalf("Failed to stat %s: %v", filename, err)
		}

		fileIndex = (fileIndex + 1) % fileCount
	}
}

func benchmarkConcurrentAccess(b *testing.B, mountDir string, storageManager *storage.Manager, client *noisefs.Client, dirCID string) {
	// Mount directory
	subMount := filepath.Join(mountDir, "bench_concurrent")
	os.MkdirAll(subMount, 0755)

	opts := MountOptions{
		MountPath:           subMount,
		VolumeName:          "bench_concurrent",
		DirectoryDescriptor: dirCID,
	}

	go func() {
		MountWithIndex(client, storageManager, opts, "")
	}()
	time.Sleep(2 * time.Second)

	dirPath := filepath.Join(subMount, "mounted-dir")

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// Alternate between listing and file access
			if time.Now().UnixNano()%2 == 0 {
				os.ReadDir(dirPath)
			} else {
				os.Stat(filepath.Join(dirPath, "file_0000.txt"))
			}
		}
	})
}

func benchmarkDirectoryTraversal(b *testing.B, mountDir string, storageManager *storage.Manager, client *noisefs.Client) {
	// Create nested directory structure
	rootCID := createNestedBenchmarkDirectory(b, storageManager, 5, 10)

	// Mount directory
	subMount := filepath.Join(mountDir, "bench_traverse")
	os.MkdirAll(subMount, 0755)

	opts := MountOptions{
		MountPath:           subMount,
		VolumeName:          "bench_traverse",
		DirectoryDescriptor: rootCID,
	}

	go func() {
		MountWithIndex(client, storageManager, opts, "")
	}()
	time.Sleep(2 * time.Second)

	rootPath := filepath.Join(subMount, "mounted-dir")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		fileCount := 0
		dirCount := 0
		
		filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				dirCount++
			} else {
				fileCount++
			}
			return nil
		})

		if fileCount == 0 || dirCount == 0 {
			b.Error("Traversal found no files or directories")
		}
	}
}

func benchmarkCachePut(b *testing.B, storageManager *storage.Manager, cacheSize int) {
	config := DirectoryCacheConfig{
		MaxSize: cacheSize,
		TTL:     time.Hour,
	}

	cache, err := NewDirectoryCache(config, storageManager)
	if err != nil {
		b.Fatalf("Failed to create cache: %v", err)
	}

	// Create test manifests
	manifests := make([]*descriptors.DirectoryManifest, b.N)
	for i := 0; i < b.N; i++ {
		manifests[i] = createTestManifest(i%100 + 1)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		cid := fmt.Sprintf("QmBench%d", i)
		cache.Put(cid, manifests[i])
	}
}

func benchmarkCacheGet(b *testing.B, storageManager *storage.Manager, cacheSize int) {
	config := DirectoryCacheConfig{
		MaxSize: cacheSize,
		TTL:     time.Hour,
	}

	cache, err := NewDirectoryCache(config, storageManager)
	if err != nil {
		b.Fatalf("Failed to create cache: %v", err)
	}

	// Pre-populate cache
	for i := 0; i < cacheSize; i++ {
		cid := fmt.Sprintf("QmBench%d", i)
		cache.Put(cid, createTestManifest(10))
	}

	b.ResetTimer()
	b.ReportAllocs()

	hits := 0
	for i := 0; i < b.N; i++ {
		cid := fmt.Sprintf("QmBench%d", i%cacheSize)
		if manifest, found := cache.Get(cid); found && manifest != nil {
			hits++
		}
	}

	b.ReportMetric(float64(hits)/float64(b.N)*100, "hit%")
}

func benchmarkCacheEviction(b *testing.B, storageManager *storage.Manager, cacheSize int) {
	config := DirectoryCacheConfig{
		MaxSize: cacheSize,
		TTL:     time.Hour,
	}

	cache, err := NewDirectoryCache(config, storageManager)
	if err != nil {
		b.Fatalf("Failed to create cache: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Keep adding beyond cache size to trigger eviction
		cid := fmt.Sprintf("QmEvict%d", i)
		cache.Put(cid, createTestManifest(10))
	}
}

func benchmarkCacheConcurrency(b *testing.B, storageManager *storage.Manager) {
	config := DirectoryCacheConfig{
		MaxSize: 1000,
		TTL:     time.Hour,
	}

	cache, err := NewDirectoryCache(config, storageManager)
	if err != nil {
		b.Fatalf("Failed to create cache: %v", err)
	}

	// Pre-populate
	for i := 0; i < 100; i++ {
		cid := fmt.Sprintf("QmConcurrent%d", i)
		cache.Put(cid, createTestManifest(10))
	}

	b.ResetTimer()
	b.ReportAllocs()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < b.N/10; j++ {
				if j%3 == 0 {
					// Put operation
					cid := fmt.Sprintf("QmConcurrent%d", j)
					cache.Put(cid, createTestManifest(5))
				} else {
					// Get operation
					cid := fmt.Sprintf("QmConcurrent%d", j%100)
					cache.Get(cid)
				}
			}
		}(i)
	}
	wg.Wait()
}

func benchmarkSingleMount(b *testing.B, storageManager *storage.Manager) {
	client, _ := createBenchmarkClient(storageManager)
	dirCID := createBenchmarkDirectory(b, storageManager, 100)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		mountDir := filepath.Join(os.TempDir(), fmt.Sprintf("bench_mount_%d", i))
		os.MkdirAll(mountDir, 0755)
		defer os.RemoveAll(mountDir)

		opts := MountOptions{
			MountPath:           mountDir,
			VolumeName:          "bench",
			DirectoryDescriptor: dirCID,
		}

		// Time just the mount operation
		start := time.Now()
		err := MountWithIndex(client, storageManager, opts, "")
		duration := time.Since(start)
		
		if err != nil {
			b.Fatalf("Mount failed: %v", err)
		}

		b.ReportMetric(duration.Seconds()*1000, "ms/mount")
		
		// Cleanup
		Unmount(mountDir)
	}
}

func benchmarkMultipleMount(b *testing.B, storageManager *storage.Manager) {
	client, _ := createBenchmarkClient(storageManager)
	
	// Create multiple directories
	dirCIDs := make([]string, 5)
	for i := 0; i < 5; i++ {
		dirCIDs[i] = createBenchmarkDirectory(b, storageManager, 20)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		mountDir := filepath.Join(os.TempDir(), fmt.Sprintf("bench_multi_%d", i))
		os.MkdirAll(mountDir, 0755)
		defer os.RemoveAll(mountDir)

		mounts := make([]DirectoryMount, 5)
		for j := 0; j < 5; j++ {
			mounts[j] = DirectoryMount{
				Name:          fmt.Sprintf("dir%d", j),
				DescriptorCID: dirCIDs[j],
			}
		}

		opts := MountOptions{
			MountPath:  mountDir,
			VolumeName: "bench_multi",
			MultiDirs:  mounts,
		}

		start := time.Now()
		err := MountWithIndex(client, storageManager, opts, "")
		duration := time.Since(start)
		
		if err != nil {
			b.Fatalf("Mount failed: %v", err)
		}

		b.ReportMetric(duration.Seconds()*1000, "ms/mount")
		b.ReportMetric(float64(len(mounts)), "dirs/mount")
		
		Unmount(mountDir)
	}
}

func benchmarkLargeDirectoryMount(b *testing.B, storageManager *storage.Manager) {
	client, _ := createBenchmarkClient(storageManager)
	
	// Create a very large directory
	largeDirCID := createBenchmarkDirectory(b, storageManager, 5000)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		mountDir := filepath.Join(os.TempDir(), fmt.Sprintf("bench_large_%d", i))
		os.MkdirAll(mountDir, 0755)
		defer os.RemoveAll(mountDir)

		opts := MountOptions{
			MountPath:           mountDir,
			VolumeName:          "bench_large",
			DirectoryDescriptor: largeDirCID,
		}

		start := time.Now()
		err := MountWithIndex(client, storageManager, opts, "")
		duration := time.Since(start)
		
		if err != nil {
			b.Fatalf("Mount failed: %v", err)
		}

		b.ReportMetric(duration.Seconds()*1000, "ms/mount")
		b.ReportMetric(5000, "files/dir")
		
		Unmount(mountDir)
	}
}

// Helper functions

func setupBenchmarkEnvironment(b *testing.B) (string, *storage.Manager, *noisefs.Client) {
	// Create mount directory
	mountDir, err := os.MkdirTemp("", "noisefs_bench_")
	if err != nil {
		b.Fatalf("Failed to create mount dir: %v", err)
	}

	// Setup storage
	storageManager, err := storagetesting.CreateRealTestStorageManager()
	if err != nil {
		os.RemoveAll(mountDir)
		b.Skipf("Skipping benchmark - storage setup failed: %v", err)
	}

	// Create client
	client, err := createBenchmarkClient(storageManager)
	if err != nil {
		storageManager.Stop(context.Background())
		os.RemoveAll(mountDir)
		b.Fatalf("Failed to create client: %v", err)
	}

	return mountDir, storageManager, client
}

func createBenchmarkClient(storageManager *storage.Manager) (*noisefs.Client, error) {
	blockCache := cache.NewMemoryCache(10000)
	return noisefs.NewClient(storageManager, blockCache)
}

func createBenchmarkDirectory(b *testing.B, storageManager *storage.Manager, fileCount int) string {
	manifest := &descriptors.DirectoryManifest{
		Version: 1,
		Entries: make([]descriptors.DirectoryEntry, fileCount),
	}

	for i := 0; i < fileCount; i++ {
		manifest.Entries[i] = descriptors.DirectoryEntry{
			Name:          fmt.Sprintf("file_%04d.txt", i),
			Type:          descriptors.FileType,
			DescriptorCID: fmt.Sprintf("QmFile%d", i),
			Size:          int64(i * 100),
		}
	}

	data, err := manifest.Serialize(nil)
	if err != nil {
		b.Fatalf("Failed to serialize manifest: %v", err)
	}

	addr, err := storageManager.Put(context.Background(), data)
	if err != nil {
		b.Fatalf("Failed to store manifest: %v", err)
	}

	return addr.CID
}

func createNestedBenchmarkDirectory(b *testing.B, storageManager *storage.Manager, depth, filesPerLevel int) string {
	if depth == 0 {
		return createBenchmarkDirectory(b, storageManager, filesPerLevel)
	}

	// Create child directories
	childCIDs := make([]string, 3)
	for i := 0; i < 3; i++ {
		childCIDs[i] = createNestedBenchmarkDirectory(b, storageManager, depth-1, filesPerLevel)
	}

	// Create parent with files and subdirs
	manifest := &descriptors.DirectoryManifest{
		Version: 1,
		Entries: make([]descriptors.DirectoryEntry, filesPerLevel+3),
	}

	// Add files
	for i := 0; i < filesPerLevel; i++ {
		manifest.Entries[i] = descriptors.DirectoryEntry{
			Name:          fmt.Sprintf("file_%d_%04d.txt", depth, i),
			Type:          descriptors.FileType,
			DescriptorCID: fmt.Sprintf("QmFile%d%d", depth, i),
			Size:          int64(i * 100),
		}
	}

	// Add subdirectories
	for i := 0; i < 3; i++ {
		manifest.Entries[filesPerLevel+i] = descriptors.DirectoryEntry{
			Name:                   fmt.Sprintf("subdir_%d", i),
			Type:                   descriptors.DirectoryType,
			DirectoryDescriptorCID: childCIDs[i],
			Size:                   0,
		}
	}

	data, err := manifest.Serialize(nil)
	if err != nil {
		b.Fatalf("Failed to serialize manifest: %v", err)
	}

	addr, err := storageManager.Put(context.Background(), data)
	if err != nil {
		b.Fatalf("Failed to store manifest: %v", err)
	}

	return addr.CID
}

func createTestManifest(fileCount int) *descriptors.DirectoryManifest {
	manifest := &descriptors.DirectoryManifest{
		Version: 1,
		Entries: make([]descriptors.DirectoryEntry, fileCount),
	}

	for i := 0; i < fileCount; i++ {
		manifest.Entries[i] = descriptors.DirectoryEntry{
			Name:          fmt.Sprintf("test_%d.txt", i),
			Type:          descriptors.FileType,
			DescriptorCID: fmt.Sprintf("QmTest%d", i),
			Size:          int64(i * 1024),
		}
	}

	return manifest
}