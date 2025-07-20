package descriptors

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	_ "github.com/TheEntropyCollective/noisefs/pkg/storage/backends" // Import mock backend
)

// newTestPasswordProvider creates a password provider for testing
func newTestPasswordProvider(password string) PasswordProvider {
	return func() (string, error) {
		return password, nil
	}
}

// newErrorPasswordProvider creates a password provider that returns an error
func newErrorPasswordProvider() PasswordProvider {
	return func() (string, error) {
		return "", errors.New("password provider error")
	}
}

// createTestDescriptor creates a test descriptor with known content
func createTestDescriptor() *Descriptor {
	descriptor := NewDescriptor("test-file.txt", 1024, 1024, 128*1024)
	descriptor.AddBlockTriple("block1", "rand1", "rand2")
	descriptor.AddBlockTriple("block2", "rand3", "rand4")
	return descriptor
}

// testMockBackend provides a simple mock storage backend for testing
type testMockBackend struct {
	mu     sync.RWMutex
	blocks map[string]*blocks.Block
}

func newTestMockBackend() *testMockBackend {
	return &testMockBackend{
		blocks: make(map[string]*blocks.Block),
	}
}

func (m *testMockBackend) Put(ctx context.Context, block *blocks.Block) (*storage.BlockAddress, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Generate a simple CID based on block data hash
	hash := sha256.Sum256(block.Data)
	cid := hex.EncodeToString(hash[:16]) // Use first 16 bytes for shorter CID
	
	m.blocks[cid] = block
	return &storage.BlockAddress{ID: cid, BackendType: "test"}, nil
}

func (m *testMockBackend) Get(ctx context.Context, address *storage.BlockAddress) (*blocks.Block, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	block, exists := m.blocks[address.ID]
	if !exists {
		return nil, fmt.Errorf("block not found: %s", address.ID)
	}
	
	return block, nil
}

func (m *testMockBackend) Has(ctx context.Context, address *storage.BlockAddress) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	_, exists := m.blocks[address.ID]
	return exists, nil
}

func (m *testMockBackend) Delete(ctx context.Context, address *storage.BlockAddress) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	delete(m.blocks, address.ID)
	return nil
}

func (m *testMockBackend) Start(ctx context.Context) error { return nil }
func (m *testMockBackend) Stop(ctx context.Context) error  { return nil }
func (m *testMockBackend) Type() string                    { return "test" }
func (m *testMockBackend) HealthCheck(ctx context.Context) error { return nil }

// createEncryptedTestStorageManager creates an in-memory storage manager for testing
func createEncryptedTestStorageManager() *storage.Manager {
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
		panic(fmt.Sprintf("Failed to create storage manager: %v", err))
	}
	
	// Start the manager
	ctx := context.Background()
	err = manager.Start(ctx)
	if err != nil {
		panic(fmt.Sprintf("Failed to start storage manager: %v", err))
	}
	
	return manager
}

func TestNewEncryptedStore(t *testing.T) {
	tests := []struct {
		name            string
		storageManager  *storage.Manager
		passwordProvider PasswordProvider
		expectError     bool
		errorContains   string
	}{
		{
			name:            "Valid parameters",
			storageManager:  createEncryptedTestStorageManager(),
			passwordProvider: newTestPasswordProvider("test-password"),
			expectError:     false,
		},
		{
			name:            "Nil storage manager",
			storageManager:  nil,
			passwordProvider: newTestPasswordProvider("test-password"),
			expectError:     true,
			errorContains:   "storage manager is required",
		},
		{
			name:            "Nil password provider should work",
			storageManager:  createEncryptedTestStorageManager(),
			passwordProvider: nil,
			expectError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, err := NewEncryptedStore(tt.storageManager, tt.passwordProvider)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
					return
				}
				if !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error to contain '%s', got: %v", tt.errorContains, err)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if store == nil {
				t.Error("Expected non-nil store")
				return
			}

			if store.storageManager != tt.storageManager {
				t.Error("Storage manager not set correctly")
			}
		})
	}
}

func TestNewEncryptedStoreWithPassword(t *testing.T) {
	storageManager := createEncryptedTestStorageManager()

	tests := []struct {
		name        string
		password    string
		expectError bool
	}{
		{
			name:        "Valid password",
			password:    "secure-password-123",
			expectError: false,
		},
		{
			name:        "Empty password",
			password:    "",
			expectError: false, // Should work - creates provider that returns empty string
		},
		{
			name:        "Long password",
			password:    strings.Repeat("a", 1000),
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, err := NewEncryptedStoreWithPassword(storageManager, tt.password)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if store == nil {
				t.Error("Expected non-nil store")
				return
			}

			// Test that the password provider returns the expected password
			if store.passwordProvider != nil {
				password, err := store.passwordProvider()
				if err != nil {
					t.Errorf("Password provider returned error: %v", err)
				}
				if password != tt.password {
					t.Errorf("Expected password '%s', got '%s'", tt.password, password)
				}
			}
		})
	}
}

func TestEncryptedStore_SaveAndLoad_BasicEncryption(t *testing.T) {

	storageManager := createEncryptedTestStorageManager()
	password := "test-encryption-password"
	store, err := NewEncryptedStoreWithPassword(storageManager, password)
	if err != nil {
		t.Fatalf("Failed to create encrypted store: %v", err)
	}

	descriptor := createTestDescriptor()

	// Test save
	cid, err := store.Save(descriptor)
	if err != nil {
		t.Fatalf("Failed to save descriptor: %v", err)
	}

	if cid == "" {
		t.Error("Expected non-empty CID")
	}

	// Test load
	loadedDescriptor, err := store.Load(cid)
	if err != nil {
		t.Fatalf("Failed to load descriptor: %v", err)
	}

	// Verify content matches
	if loadedDescriptor.Filename != descriptor.Filename {
		t.Errorf("Expected filename '%s', got '%s'", descriptor.Filename, loadedDescriptor.Filename)
	}

	if loadedDescriptor.FileSize != descriptor.FileSize {
		t.Errorf("Expected file size %d, got %d", descriptor.FileSize, loadedDescriptor.FileSize)
	}

	if len(loadedDescriptor.Blocks) != len(descriptor.Blocks) {
		t.Errorf("Expected %d blocks, got %d", len(descriptor.Blocks), len(loadedDescriptor.Blocks))
	}
}

func TestEncryptedStore_SaveUnencrypted(t *testing.T) {

	storageManager := createEncryptedTestStorageManager()
	password := "test-password"
	store, err := NewEncryptedStoreWithPassword(storageManager, password)
	if err != nil {
		t.Fatalf("Failed to create encrypted store: %v", err)
	}

	descriptor := createTestDescriptor()

	// Test save unencrypted
	cid, err := store.SaveUnencrypted(descriptor)
	if err != nil {
		t.Fatalf("Failed to save unencrypted descriptor: %v", err)
	}

	if cid == "" {
		t.Error("Expected non-empty CID")
	}

	// Test that we can load it
	loadedDescriptor, err := store.Load(cid)
	if err != nil {
		t.Fatalf("Failed to load unencrypted descriptor: %v", err)
	}

	// Verify content matches
	if loadedDescriptor.Filename != descriptor.Filename {
		t.Errorf("Expected filename '%s', got '%s'", descriptor.Filename, loadedDescriptor.Filename)
	}
}

func TestEncryptedStore_PasswordProvider_Error(t *testing.T) {
	storageManager := createEncryptedTestStorageManager()
	store, err := NewEncryptedStore(storageManager, newErrorPasswordProvider())
	if err != nil {
		t.Fatalf("Failed to create encrypted store: %v", err)
	}

	descriptor := createTestDescriptor()

	// Test save with password provider error
	_, err = store.Save(descriptor)
	if err == nil {
		t.Error("Expected error when password provider fails")
	}

	if !strings.Contains(err.Error(), "failed to get password") {
		t.Errorf("Expected error to mention password failure, got: %v", err)
	}
}

func TestEncryptedStore_NilDescriptor(t *testing.T) {
	storageManager := createEncryptedTestStorageManager()
	store, err := NewEncryptedStoreWithPassword(storageManager, "test-password")
	if err != nil {
		t.Fatalf("Failed to create encrypted store: %v", err)
	}

	// Test save with nil descriptor
	_, err = store.Save(nil)
	if err == nil {
		t.Error("Expected error when saving nil descriptor")
	}

	if !strings.Contains(err.Error(), "descriptor cannot be nil") {
		t.Errorf("Expected error to mention nil descriptor, got: %v", err)
	}
}

func TestEncryptedStore_EmptyCID(t *testing.T) {
	storageManager := createEncryptedTestStorageManager()
	store, err := NewEncryptedStoreWithPassword(storageManager, "test-password")
	if err != nil {
		t.Fatalf("Failed to create encrypted store: %v", err)
	}

	// Test load with empty CID
	_, err = store.Load("")
	if err == nil {
		t.Error("Expected error when loading with empty CID")
	}

	if !strings.Contains(err.Error(), "CID cannot be empty") {
		t.Errorf("Expected error to mention empty CID, got: %v", err)
	}
}

func TestEncryptedStore_IsEncrypted(t *testing.T) {

	storageManager := createEncryptedTestStorageManager()
	store, err := NewEncryptedStoreWithPassword(storageManager, "test-password")
	if err != nil {
		t.Fatalf("Failed to create encrypted store: %v", err)
	}

	// Test IsEncrypted with empty CID
	_, err = store.IsEncrypted("")
	if err == nil {
		t.Error("Expected error when checking encryption status with empty CID")
	}
}

// TestEncryptDecryptRoundTrip tests the internal encryption/decryption methods
func TestEncryptDecryptRoundTrip(t *testing.T) {
	storageManager := createEncryptedTestStorageManager()
	password := "test-round-trip-password"
	store, err := NewEncryptedStoreWithPassword(storageManager, password)
	if err != nil {
		t.Fatalf("Failed to create encrypted store: %v", err)
	}

	descriptor := createTestDescriptor()

	// Test encryption
	encryptedData, err := store.encryptDescriptor(descriptor, password)
	if err != nil {
		t.Fatalf("Failed to encrypt descriptor: %v", err)
	}

	if len(encryptedData) == 0 {
		t.Error("Expected non-empty encrypted data")
	}

	// Parse the encrypted data structure
	var encDesc EncryptedDescriptor
	if err := json.Unmarshal(encryptedData, &encDesc); err != nil {
		t.Fatalf("Failed to parse encrypted descriptor: %v", err)
	}

	// Verify encryption metadata
	if encDesc.Version != "3.0" {
		t.Errorf("Expected version '3.0', got '%s'", encDesc.Version)
	}

	if !encDesc.IsEncrypted {
		t.Error("Expected IsEncrypted to be true")
	}

	if len(encDesc.Salt) == 0 {
		t.Error("Expected non-empty salt")
	}

	if len(encDesc.Ciphertext) == 0 {
		t.Error("Expected non-empty ciphertext")
	}

	// Test decryption
	decryptedDescriptor, err := store.decryptDescriptor(&encDesc)
	if err != nil {
		t.Fatalf("Failed to decrypt descriptor: %v", err)
	}

	// Verify decrypted content matches original
	if decryptedDescriptor.Filename != descriptor.Filename {
		t.Errorf("Expected filename '%s', got '%s'", descriptor.Filename, decryptedDescriptor.Filename)
	}

	if decryptedDescriptor.FileSize != descriptor.FileSize {
		t.Errorf("Expected file size %d, got %d", descriptor.FileSize, decryptedDescriptor.FileSize)
	}

	if len(decryptedDescriptor.Blocks) != len(descriptor.Blocks) {
		t.Errorf("Expected %d blocks, got %d", len(descriptor.Blocks), len(decryptedDescriptor.Blocks))
	}

	// Verify block content
	for i, block := range descriptor.Blocks {
		if i >= len(decryptedDescriptor.Blocks) {
			break
		}
		decryptedBlock := decryptedDescriptor.Blocks[i]
		if block.DataCID != decryptedBlock.DataCID {
			t.Errorf("Block %d: expected DataCID '%s', got '%s'", i, block.DataCID, decryptedBlock.DataCID)
		}
	}
}

func TestEncryptedStore_WrongPassword(t *testing.T) {
	storageManager := createEncryptedTestStorageManager()
	correctPassword := "correct-password"
	wrongPassword := "wrong-password"

	// Create store with correct password
	store, err := NewEncryptedStoreWithPassword(storageManager, correctPassword)
	if err != nil {
		t.Fatalf("Failed to create encrypted store: %v", err)
	}

	descriptor := createTestDescriptor()

	// Encrypt with correct password
	encryptedData, err := store.encryptDescriptor(descriptor, correctPassword)
	if err != nil {
		t.Fatalf("Failed to encrypt descriptor: %v", err)
	}

	var encDesc EncryptedDescriptor
	if err := json.Unmarshal(encryptedData, &encDesc); err != nil {
		t.Fatalf("Failed to parse encrypted descriptor: %v", err)
	}

	// Try to decrypt with wrong password
	wrongStore, err := NewEncryptedStoreWithPassword(storageManager, wrongPassword)
	if err != nil {
		t.Fatalf("Failed to create store with wrong password: %v", err)
	}

	_, err = wrongStore.decryptDescriptor(&encDesc)
	if err == nil {
		t.Error("Expected error when decrypting with wrong password")
	}

	// The error should indicate decryption failure
	if !strings.Contains(err.Error(), "failed to decrypt") && !strings.Contains(err.Error(), "authentication") {
		t.Errorf("Expected decryption error, got: %v", err)
	}
}

// Additional tests for edge cases and security scenarios

func TestEncryptedStore_EmptyPassword(t *testing.T) {
	storageManager := createEncryptedTestStorageManager()
	store, err := NewEncryptedStoreWithPassword(storageManager, "")
	if err != nil {
		t.Fatalf("Failed to create encrypted store: %v", err)
	}

	descriptor := createTestDescriptor()

	// Save with empty password should use unencrypted format
	encryptedData, err := store.encryptDescriptor(descriptor, "")
	if err != nil {
		t.Fatalf("Failed to encrypt descriptor with empty password: %v", err)
	}

	// Should still produce valid data (unencrypted format)
	if len(encryptedData) == 0 {
		t.Error("Expected non-empty data even with empty password")
	}
}

func TestEncryptedStore_LargeDescriptor(t *testing.T) {
	storageManager := createEncryptedTestStorageManager()
	password := "test-large-descriptor"
	store, err := NewEncryptedStoreWithPassword(storageManager, password)
	if err != nil {
		t.Fatalf("Failed to create encrypted store: %v", err)
	}

	// Create a large descriptor with many blocks
	descriptor := NewDescriptor("large-file.bin", 1024*1024*100, 1024*1024*100, 128*1024) // 100MB file
	for i := 0; i < 1000; i++ { // 1000 blocks
		descriptor.AddBlockTriple(
			fmt.Sprintf("block%d", i),
			fmt.Sprintf("rand1_%d", i),
			fmt.Sprintf("rand2_%d", i),
		)
	}

	// Test encryption of large descriptor
	start := time.Now()
	encryptedData, err := store.encryptDescriptor(descriptor, password)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("Failed to encrypt large descriptor: %v", err)
	}

	if len(encryptedData) == 0 {
		t.Error("Expected non-empty encrypted data for large descriptor")
	}

	// Performance check - encryption should complete reasonably quickly
	if duration > 5*time.Second {
		t.Errorf("Encryption took too long: %v (expected < 5s)", duration)
	}

	t.Logf("Large descriptor encryption completed in %v", duration)
}