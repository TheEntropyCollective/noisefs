package descriptors

import (
	"context"
	"testing"

	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	_ "github.com/TheEntropyCollective/noisefs/pkg/storage/backends" // Import mock backend
)

// createTestStorageManager creates a storage manager with mock backend for testing
func createTestStorageManager(t *testing.T) *storage.Manager {
	// Start with default config and modify for testing
	config := storage.DefaultConfig()
	
	// Replace with mock backend
	config.DefaultBackend = "mock"
	config.Backends = map[string]*storage.BackendConfig{
		"mock": {
			Type:     "mock",
			Enabled:  true,
			Priority: 100,
			Connection: &storage.ConnectionConfig{
				Endpoint: "mock://test",
			},
		},
	}
	
	// Disable health checking for faster tests
	config.HealthCheck.Enabled = false
	
	manager, err := storage.NewManager(config)
	if err != nil {
		t.Fatalf("Failed to create storage manager: %v", err)
	}
	
	// Start the manager
	ctx := context.Background()
	err = manager.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start storage manager: %v", err)
	}
	
	return manager
}

func TestNewStore(t *testing.T) {
	manager := createTestStorageManager(t)
	
	// Test successful creation
	store, err := NewStore(manager)
	if err != nil {
		t.Errorf("NewStore() error = %v, want nil", err)
	}
	
	if store == nil {
		t.Fatal("NewStore() returned nil store")
	}
	
	// Test with nil storage manager
	_, err = NewStore(nil)
	if err == nil {
		t.Error("NewStore() with nil storage manager should return error")
	}
	
	expectedErrMsg := "storage manager is required"
	if err.Error() != expectedErrMsg {
		t.Errorf("NewStore() error = %v, want %v", err.Error(), expectedErrMsg)
	}
}

func TestNewStoreWithManager(t *testing.T) {
	manager := createTestStorageManager(t)
	
	// Test successful creation
	store, err := NewStoreWithManager(manager)
	if err != nil {
		t.Errorf("NewStoreWithManager() error = %v, want nil", err)
	}
	
	if store == nil {
		t.Fatal("NewStoreWithManager() returned nil store")
	}
	
	// Test with nil storage manager
	_, err = NewStoreWithManager(nil)
	if err == nil {
		t.Error("NewStoreWithManager() with nil storage manager should return error")
	}
	
	expectedErrMsg := "storage manager is required"
	if err.Error() != expectedErrMsg {
		t.Errorf("NewStoreWithManager() error = %v, want %v", err.Error(), expectedErrMsg)
	}
}

func TestStoreSave(t *testing.T) {
	manager := createTestStorageManager(t)
	store, err := NewStore(manager)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	
	// Create a test descriptor
	desc := NewDescriptor("test-save.txt", 1024, 1024, 128)
	err = desc.AddBlockTriple("data1", "rand1", "rand2")
	if err != nil {
		t.Fatalf("Failed to add block triple: %v", err)
	}
	
	// Test successful save
	cid, err := store.Save(desc)
	if err != nil {
		t.Errorf("Save() error = %v, want nil", err)
	}
	
	if cid == "" {
		t.Error("Save() returned empty CID")
	}
	
	// Test save with nil descriptor
	_, err = store.Save(nil)
	if err == nil {
		t.Error("Save() with nil descriptor should return error")
	}
	
	expectedErrMsg := "descriptor cannot be nil"
	if err.Error() != expectedErrMsg {
		t.Errorf("Save() error = %v, want %v", err.Error(), expectedErrMsg)
	}
	
	// Test save with invalid descriptor
	invalidDesc := &Descriptor{} // Missing required fields
	_, err = store.Save(invalidDesc)
	if err == nil {
		t.Error("Save() with invalid descriptor should return error")
	}
}

func TestStoreLoad(t *testing.T) {
	manager := createTestStorageManager(t)
	store, err := NewStore(manager)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	
	// Create and save a test descriptor first
	desc := NewDescriptor("test-load.txt", 2048, 2048, 256)
	err = desc.AddBlockTriple("data1", "rand1", "rand2")
	if err != nil {
		t.Fatalf("Failed to add block triple: %v", err)
	}
	
	cid, err := store.Save(desc)
	if err != nil {
		t.Fatalf("Failed to save descriptor: %v", err)
	}
	
	// Test successful load
	loadedDesc, err := store.Load(cid)
	if err != nil {
		t.Errorf("Load() error = %v, want nil", err)
	}
	
	if loadedDesc == nil {
		t.Fatal("Load() returned nil descriptor")
	}
	
	// Verify loaded descriptor matches original
	if loadedDesc.Filename != desc.Filename {
		t.Errorf("Load() filename = %v, want %v", loadedDesc.Filename, desc.Filename)
	}
	
	if loadedDesc.FileSize != desc.FileSize {
		t.Errorf("Load() file size = %v, want %v", loadedDesc.FileSize, desc.FileSize)
	}
	
	if len(loadedDesc.Blocks) != len(desc.Blocks) {
		t.Errorf("Load() blocks length = %v, want %v", len(loadedDesc.Blocks), len(desc.Blocks))
	}
	
	// Test load with empty CID
	_, err = store.Load("")
	if err == nil {
		t.Error("Load() with empty CID should return error")
	}
	
	expectedErrMsg := "CID cannot be empty"
	if err.Error() != expectedErrMsg {
		t.Errorf("Load() error = %v, want %v", err.Error(), expectedErrMsg)
	}
	
	// Test load with non-existent CID
	_, err = store.Load("QmNonExistent123456")
	if err == nil {
		t.Error("Load() with non-existent CID should return error")
	}
}

func TestStoreRoundTrip(t *testing.T) {
	manager := createTestStorageManager(t)
	store, err := NewStore(manager)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	
	// Create a complex descriptor with multiple blocks
	desc := NewDescriptor("roundtrip-complex.txt", 4096, 4096, 128)
	
	// Add multiple block triples
	blocks := []struct {
		data, rand1, rand2 string
	}{
		{"QmData1", "QmRand1A", "QmRand1B"},
		{"QmData2", "QmRand2A", "QmRand2B"},
		{"QmData3", "QmRand3A", "QmRand3B"},
	}
	
	for _, block := range blocks {
		err = desc.AddBlockTriple(block.data, block.rand1, block.rand2)
		if err != nil {
			t.Fatalf("Failed to add block triple: %v", err)
		}
	}
	
	// Save the descriptor
	cid, err := store.Save(desc)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	
	// Load the descriptor
	loadedDesc, err := store.Load(cid)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	
	// Verify all blocks were preserved
	if len(loadedDesc.Blocks) != len(desc.Blocks) {
		t.Errorf("Round-trip blocks count = %v, want %v", len(loadedDesc.Blocks), len(desc.Blocks))
	}
	
	for i, block := range loadedDesc.Blocks {
		if i >= len(desc.Blocks) {
			break
		}
		
		origBlock := desc.Blocks[i]
		if block.DataCID != origBlock.DataCID {
			t.Errorf("Round-trip Block[%d] DataCID = %v, want %v", i, block.DataCID, origBlock.DataCID)
		}
		if block.RandomizerCID1 != origBlock.RandomizerCID1 {
			t.Errorf("Round-trip Block[%d] RandomizerCID1 = %v, want %v", i, block.RandomizerCID1, origBlock.RandomizerCID1)
		}
		if block.RandomizerCID2 != origBlock.RandomizerCID2 {
			t.Errorf("Round-trip Block[%d] RandomizerCID2 = %v, want %v", i, block.RandomizerCID2, origBlock.RandomizerCID2)
		}
	}
	
	// Verify the descriptor is valid after round-trip
	err = loadedDesc.Validate()
	if err != nil {
		t.Errorf("Round-trip descriptor validation failed: %v", err)
	}
}

func TestStoreWithDirectoryDescriptor(t *testing.T) {
	manager := createTestStorageManager(t)
	store, err := NewStore(manager)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	
	// Create a directory descriptor
	dirDesc := NewDirectoryDescriptor("test-directory", "QmManifestCID123")
	
	// Save the directory descriptor
	cid, err := store.Save(dirDesc)
	if err != nil {
		t.Errorf("Save() directory descriptor error = %v, want nil", err)
	}
	
	if cid == "" {
		t.Error("Save() directory descriptor returned empty CID")
	}
	
	// Load the directory descriptor
	loadedDesc, err := store.Load(cid)
	if err != nil {
		t.Errorf("Load() directory descriptor error = %v, want nil", err)
	}
	
	if loadedDesc == nil {
		t.Fatal("Load() returned nil directory descriptor")
	}
	
	// Verify it's still a directory descriptor
	if !loadedDesc.IsDirectory() {
		t.Error("Loaded descriptor should be a directory")
	}
	
	if loadedDesc.IsFile() {
		t.Error("Loaded descriptor should not be a file")
	}
	
	if loadedDesc.ManifestCID != dirDesc.ManifestCID {
		t.Errorf("Directory ManifestCID = %v, want %v", loadedDesc.ManifestCID, dirDesc.ManifestCID)
	}
}

func TestStoreErrorHandling(t *testing.T) {
	manager := createTestStorageManager(t)
	store, err := NewStore(manager)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	
	// Test save with descriptor missing blocks - should fail validation
	invalidDesc := NewDescriptor("invalid.txt", 1024, 1024, 128)
	// Don't add any blocks - this should cause validation to fail
	
	_, err = store.Save(invalidDesc)
	if err == nil {
		t.Error("Save() with descriptor missing blocks should return error")
	}
}