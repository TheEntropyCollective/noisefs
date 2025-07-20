//go:build fuse

package testutil

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// MockNoisefsClient provides a mock NoiseFS client for testing
type MockNoisefsClient struct {
	mu              sync.RWMutex
	uploadCalls     int
	downloadCalls   int
	blocks          map[string][]byte
	randomizers     map[string]*MockBlock
	shouldFailStore bool
	storeError      error
}

// NewMockNoisefsClient creates a new mock NoiseFS client
func NewMockNoisefsClient() *MockNoisefsClient {
	return &MockNoisefsClient{
		blocks:      make(map[string][]byte),
		randomizers: make(map[string]*MockBlock),
	}
}

// SetStoreError sets an error to be returned by store operations
func (m *MockNoisefsClient) SetStoreError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.storeError = err
	m.shouldFailStore = err != nil
}

// SelectRandomizers returns mock randomizers for testing
func (m *MockNoisefsClient) SelectRandomizers(size int) (*MockBlock, string, *MockBlock, string, int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return two simple randomizers
	data1 := make([]byte, size)
	data2 := make([]byte, size)
	for i := range data1 {
		data1[i] = 0xFF // Simple pattern for first randomizer
		data2[i] = 0xAA // Different pattern for second randomizer
	}
	
	block1 := &MockBlock{data: data1}
	block2 := &MockBlock{data: data2}
	
	// Store for later retrieval
	cid1 := fmt.Sprintf("mock_randomizer1_%d", len(m.randomizers))
	cid2 := fmt.Sprintf("mock_randomizer2_%d", len(m.randomizers))
	m.randomizers[cid1] = block1
	m.randomizers[cid2] = block2
	
	return block1, cid1, block2, cid2, 0, nil
}

// StoreBlockWithCache stores a block in the mock client
func (m *MockNoisefsClient) StoreBlockWithCache(block blockLike) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldFailStore && m.storeError != nil {
		return "", m.storeError
	}

	m.uploadCalls++
	cid := fmt.Sprintf("mock_cid_%d", m.uploadCalls)
	m.blocks[cid] = block.getData()
	return cid, nil
}

// RetrieveBlockWithCache retrieves a block from the mock client
func (m *MockNoisefsClient) RetrieveBlockWithCache(cid string) (*MockBlock, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.downloadCalls++
	
	// Check randomizers first
	if block, exists := m.randomizers[cid]; exists {
		return block, nil
	}
	
	// Check regular blocks
	data, exists := m.blocks[cid]
	if !exists {
		return nil, errors.New("block not found")
	}
	return &MockBlock{data: data}, nil
}

// RecordUpload records upload metrics (no-op for testing)
func (m *MockNoisefsClient) RecordUpload(originalBytes, storedBytes int64) {
	// No-op for testing
}

// RecordDownload records download metrics (no-op for testing)
func (m *MockNoisefsClient) RecordDownload() {
	// No-op for testing
}

// GetMetrics returns empty metrics for testing
func (m *MockNoisefsClient) GetMetrics() interface{} {
	return struct{}{} // Empty metrics for testing
}

// GetUploadCalls returns the number of upload calls made
func (m *MockNoisefsClient) GetUploadCalls() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.uploadCalls
}

// GetDownloadCalls returns the number of download calls made
func (m *MockNoisefsClient) GetDownloadCalls() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.downloadCalls
}

// GetStoredBlocks returns all stored block CIDs
func (m *MockNoisefsClient) GetStoredBlocks() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	cids := make([]string, 0, len(m.blocks))
	for cid := range m.blocks {
		cids = append(cids, cid)
	}
	return cids
}

// MockBlock implements the block interface for testing
type MockBlock struct {
	data []byte
}

// XOR performs XOR operation with another block
func (b *MockBlock) XOR(other *MockBlock) (*MockBlock, error) {
	if len(b.data) != len(other.data) {
		return nil, errors.New("block size mismatch")
	}
	
	result := make([]byte, len(b.data))
	for i := range b.data {
		result[i] = b.data[i] ^ other.data[i]
	}
	
	return &MockBlock{data: result}, nil
}

// getData returns the block data
func (b *MockBlock) getData() []byte {
	return b.data
}

// Data returns the block data (for compatibility)
func (b *MockBlock) Data() []byte {
	return b.data
}

// Size returns the size of the block
func (b *MockBlock) Size() int {
	return len(b.data)
}

// blockLike interface for testing compatibility
type blockLike interface {
	getData() []byte
}

// MockStorageBackend provides a mock storage backend for testing
type MockStorageBackend struct {
	mu       sync.RWMutex
	blocks   map[string][]byte
	fails    bool
	failErr  error
	latency  bool
}

// NewMockStorageBackend creates a new mock storage backend
func NewMockStorageBackend() *MockStorageBackend {
	return &MockStorageBackend{
		blocks: make(map[string][]byte),
	}
}

// SetFailMode sets the backend to fail operations
func (m *MockStorageBackend) SetFailMode(fail bool, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fails = fail
	m.failErr = err
}

// SetLatency enables latency simulation
func (m *MockStorageBackend) SetLatency(enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.latency = enabled
}

// Put stores data in the mock backend
func (m *MockStorageBackend) Put(ctx context.Context, data []byte) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.fails && m.failErr != nil {
		return "", m.failErr
	}

	cid := fmt.Sprintf("mock_backend_cid_%d", len(m.blocks))
	m.blocks[cid] = make([]byte, len(data))
	copy(m.blocks[cid], data)
	return cid, nil
}

// Get retrieves data from the mock backend
func (m *MockStorageBackend) Get(ctx context.Context, cid string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.fails && m.failErr != nil {
		return nil, m.failErr
	}

	data, exists := m.blocks[cid]
	if !exists {
		return nil, errors.New("block not found")
	}
	
	result := make([]byte, len(data))
	copy(result, data)
	return result, nil
}

// Has checks if data exists in the mock backend
func (m *MockStorageBackend) Has(ctx context.Context, cid string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.fails && m.failErr != nil {
		return false, m.failErr
	}

	_, exists := m.blocks[cid]
	return exists, nil
}

// Delete removes data from the mock backend
func (m *MockStorageBackend) Delete(ctx context.Context, cid string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.fails && m.failErr != nil {
		return m.failErr
	}

	delete(m.blocks, cid)
	return nil
}

// GetStoredCIDs returns all stored CIDs
func (m *MockStorageBackend) GetStoredCIDs() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	cids := make([]string, 0, len(m.blocks))
	for cid := range m.blocks {
		cids = append(cids, cid)
	}
	return cids
}

// MockEncryptionUtil provides mock encryption utilities for testing
type MockEncryptionUtil struct {
	shouldFailEncrypt bool
	shouldFailDecrypt bool
	encryptError      error
	decryptError      error
}

// NewMockEncryptionUtil creates a new mock encryption utility
func NewMockEncryptionUtil() *MockEncryptionUtil {
	return &MockEncryptionUtil{}
}

// SetEncryptError sets an error to be returned by encrypt operations
func (m *MockEncryptionUtil) SetEncryptError(err error) {
	m.encryptError = err
	m.shouldFailEncrypt = err != nil
}

// SetDecryptError sets an error to be returned by decrypt operations
func (m *MockEncryptionUtil) SetDecryptError(err error) {
	m.decryptError = err
	m.shouldFailDecrypt = err != nil
}

// Encrypt performs mock encryption (simple XOR for testing)
func (m *MockEncryptionUtil) Encrypt(data []byte, key []byte) ([]byte, error) {
	if m.shouldFailEncrypt {
		return nil, m.encryptError
	}

	// Simple XOR encryption for testing
	result := make([]byte, len(data))
	for i := range data {
		result[i] = data[i] ^ key[i%len(key)]
	}
	return result, nil
}

// Decrypt performs mock decryption (simple XOR for testing)
func (m *MockEncryptionUtil) Decrypt(data []byte, key []byte) ([]byte, error) {
	if m.shouldFailDecrypt {
		return nil, m.decryptError
	}

	// Simple XOR decryption for testing (same as encrypt)
	result := make([]byte, len(data))
	for i := range data {
		result[i] = data[i] ^ key[i%len(key)]
	}
	return result, nil
}

// GenerateKey generates a mock encryption key
func (m *MockEncryptionUtil) GenerateKey() []byte {
	// Return a simple test key
	return []byte("test_encryption_key_32_bytes___")
}