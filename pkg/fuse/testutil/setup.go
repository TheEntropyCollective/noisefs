//go:build fuse

package testutil

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/core/crypto"
	"github.com/TheEntropyCollective/noisefs/pkg/core/descriptors"
	"github.com/TheEntropyCollective/noisefs/pkg/fuse"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
	storagetesting "github.com/TheEntropyCollective/noisefs/pkg/storage/testing"
	noisefs "github.com/TheEntropyCollective/noisefs/pkg/core/client"
)

// TestEnvironment holds all components needed for FUSE testing
type TestEnvironment struct {
	TempDir        string
	MountDir       string
	IndexFile      string
	StorageManager *storage.Manager
	Client         *noisefs.Client
	Cache          cache.Cache
}

// SetupTestEnvironment creates a complete test environment for FUSE tests
func SetupTestEnvironment(t *testing.T) *TestEnvironment {
	t.Helper()

	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "noisefs_fuse_test_")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Create mount directory
	mountDir := filepath.Join(tempDir, "mount")
	if err := os.MkdirAll(mountDir, 0755); err != nil {
		t.Fatalf("Failed to create mount dir: %v", err)
	}

	// Create index file path
	indexFile := filepath.Join(tempDir, "test_index.json")

	// Setup storage manager
	storageManager, err := storagetesting.CreateRealTestStorageManager()
	if err != nil {
		os.RemoveAll(tempDir)
		t.Skipf("Skipping test - storage manager setup failed: %v", err)
	}

	// Create cache and NoiseFS client
	blockCache := cache.NewMemoryCache(1000)
	client, err := noisefs.NewClient(storageManager, blockCache)
	if err != nil {
		storageManager.Stop(context.Background())
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to create NoiseFS client: %v", err)
	}

	return &TestEnvironment{
		TempDir:        tempDir,
		MountDir:       mountDir,
		IndexFile:      indexFile,
		StorageManager: storageManager,
		Client:         client,
		Cache:          blockCache,
	}
}

// Cleanup cleans up the test environment
func (env *TestEnvironment) Cleanup() {
	if env.StorageManager != nil {
		env.StorageManager.Stop(context.Background())
	}
	if env.TempDir != "" {
		os.RemoveAll(env.TempDir)
	}
}

// CreateMountOptions creates standard mount options for testing
func (env *TestEnvironment) CreateMountOptions() fuse.MountOptions {
	return fuse.MountOptions{
		MountPath:  env.MountDir,
		VolumeName: "test_noisefs",
		ReadOnly:   false,
		AllowOther: false,
		Debug:      false,
	}
}

// CreateDirectoryMountOptions creates mount options for directory mounting
func (env *TestEnvironment) CreateDirectoryMountOptions(dirCID, encKey string) fuse.MountOptions {
	opts := env.CreateMountOptions()
	opts.DirectoryDescriptor = dirCID
	opts.DirectoryKey = encKey
	return opts
}

// CreateMultiDirectoryMountOptions creates mount options for multiple directory mounting
func (env *TestEnvironment) CreateMultiDirectoryMountOptions(dirs []fuse.DirectoryMount) fuse.MountOptions {
	opts := env.CreateMountOptions()
	opts.MultiDirs = dirs
	return opts
}

// StartMountInBackground starts a FUSE mount in background and returns error channel
func (env *TestEnvironment) StartMountInBackground(opts fuse.MountOptions) chan error {
	mountErr := make(chan error, 1)
	go func() {
		err := fuse.MountWithIndex(env.Client, env.StorageManager, opts, env.IndexFile)
		mountErr <- err
	}()
	
	// Wait for mount to be ready
	time.Sleep(2 * time.Second)
	return mountErr
}

// WaitForMount waits for mount to complete or fail
func (env *TestEnvironment) WaitForMount(mountErr chan error, timeout time.Duration) error {
	select {
	case err := <-mountErr:
		return err
	case <-time.After(timeout):
		return nil // Mount is still running
	}
}

// CheckFUSEAvailable checks if FUSE is available for testing
func CheckFUSEAvailable(t *testing.T) {
	t.Helper()
	if _, err := os.Stat("/dev/fuse"); err != nil {
		t.Skip("Skipping FUSE test: /dev/fuse not available")
	}
}

// SkipInShortMode skips test if running in short mode
func SkipInShortMode(t *testing.T, reason string) {
	t.Helper()
	if testing.Short() {
		t.Skipf("Skipping test in short mode: %s", reason)
	}
}

// CreateTestDirectoryStructure creates a test directory structure with manifest
func CreateTestDirectoryStructure(t *testing.T, env *TestEnvironment) (string, string) {
	t.Helper()

	// Create encryption key
	key, err := crypto.GenerateKey("test-password")
	if err != nil {
		t.Fatalf("Failed to generate encryption key: %v", err)
	}
	encodedKey := base64.StdEncoding.EncodeToString(key.Key)

	// Create directory manifest
	manifest := descriptors.NewDirectoryManifest()
	
	// Add test entries
	file1Name, _ := crypto.EncryptFileName("file1.txt", key)
	file2Name, _ := crypto.EncryptFileName("file2.txt", key)
	subdirName, _ := crypto.EncryptFileName("subdir", key)
	
	manifest.Entries = []descriptors.DirectoryEntry{
		{
			EncryptedName: file1Name,
			Type:          descriptors.FileType,
			CID:           "QmFile1Test",
			Size:          100,
		},
		{
			EncryptedName: file2Name,
			Type:          descriptors.FileType,
			CID:           "QmFile2Test",
			Size:          200,
		},
		{
			EncryptedName: subdirName,
			Type:          descriptors.DirectoryType,
			CID:           "QmSubdirTest",
			Size:          0,
		},
	}

	// Upload manifest
	dirCID := uploadManifest(t, env.StorageManager, manifest, key)
	
	return dirCID, encodedKey
}

// CreateLargeTestDirectory creates a large directory with many files for testing
func CreateLargeTestDirectory(t *testing.T, env *TestEnvironment, fileCount int) (string, string) {
	t.Helper()

	// Create directory manifest with many files
	manifest := descriptors.NewDirectoryManifest()
	manifest.Entries = make([]descriptors.DirectoryEntry, fileCount)

	for i := 0; i < fileCount; i++ {
		filename := generateTestFileName(i)
		encryptedName := []byte(filename) // For testing, use unencrypted names
		
		manifest.Entries[i] = descriptors.DirectoryEntry{
			EncryptedName: encryptedName,
			Type:          descriptors.FileType,
			CID:           generateTestCID(i),
			Size:          int64(i * 100),
		}
	}

	// Upload manifest
	dirCID := uploadManifest(t, env.StorageManager, manifest, nil)
	
	return dirCID, ""
}

// CreateNestedDirectoryStructure creates a nested directory structure for testing
func CreateNestedDirectoryStructure(t *testing.T, env *TestEnvironment, depth int) string {
	t.Helper()
	return createNestedDirectoryStructure(t, env.StorageManager, depth)
}

// uploadManifest uploads a directory manifest to storage
func uploadManifest(t *testing.T, storageManager *storage.Manager, manifest *descriptors.DirectoryManifest, key *crypto.EncryptionKey) string {
	t.Helper()

	var data []byte
	var err error

	// Serialize manifest (with or without encryption)
	if key != nil {
		data, err = descriptors.EncryptManifest(manifest, key)
	} else {
		data, err = manifest.Marshal()
	}
	if err != nil {
		t.Fatalf("Failed to serialize manifest: %v", err)
	}

	// Create a block from the data
	block, err := blocks.NewBlock(data)
	if err != nil {
		t.Fatalf("Failed to create block: %v", err)
	}

	// Store in backend
	addr, err := storageManager.Put(context.Background(), block)
	if err != nil {
		t.Fatalf("Failed to store manifest: %v", err)
	}

	return addr.ID
}

// createNestedDirectoryStructure recursively creates nested directory structure
func createNestedDirectoryStructure(t *testing.T, storageManager *storage.Manager, depth int) string {
	t.Helper()

	if depth == 0 {
		// Leaf directory
		manifest := descriptors.NewDirectoryManifest()
		manifest.Entries = []descriptors.DirectoryEntry{
			{
				EncryptedName: []byte("leaf_file.txt"),
				Type:          descriptors.FileType,
				CID:           "QmLeafFile",
				Size:          42,
			},
		}
		return uploadManifest(t, storageManager, manifest, nil)
	}

	// Create child directory first
	childCID := createNestedDirectoryStructure(t, storageManager, depth-1)

	// Create parent directory
	manifest := descriptors.NewDirectoryManifest()
	manifest.Entries = []descriptors.DirectoryEntry{
		{
			EncryptedName: []byte(generateTestFileName(depth)),
			Type:          descriptors.FileType,
			CID:           generateTestCID(depth),
			Size:          int64(depth * 100),
		},
		{
			EncryptedName: []byte(generateTestDirName(depth)),
			Type:          descriptors.DirectoryType,
			CID:           childCID,
			Size:          0,
		},
	}

	return uploadManifest(t, storageManager, manifest, nil)
}