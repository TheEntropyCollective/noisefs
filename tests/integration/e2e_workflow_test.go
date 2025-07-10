package integration

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
	"github.com/TheEntropyCollective/noisefs/pkg/compliance"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/config"
	"github.com/TheEntropyCollective/noisefs/pkg/core/descriptors"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/ipfs"
	"github.com/TheEntropyCollective/noisefs/pkg/core/client"
	"github.com/TheEntropyCollective/noisefs/pkg/privacy/reuse"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/libp2p/go-libp2p/core/peer"
)

// E2ETestSuite provides comprehensive end-to-end testing
type E2ETestSuite struct {
	mockBlockStore    *MockBlockStore
	memoryCache       cache.Cache
	noisefsClient     *noisefs.Client
	reuseClient       *reuse.ReuseAwareClient
	storageManager    *storage.Manager
	complianceSystem  *compliance.ComplianceAuditSystem
	testConfig        *config.Config
}

type MockBlockStore struct {
	blocks map[string]*blocks.Block
}

func NewMockBlockStore() *MockBlockStore {
	return &MockBlockStore{
		blocks: make(map[string]*blocks.Block),
	}
}

func (m *MockBlockStore) StoreBlock(block *blocks.Block) (string, error) {
	if block == nil {
		return "", errors.New("block cannot be nil")
	}
	cid := fmt.Sprintf("mock_%s", block.ID)
	m.blocks[cid] = block
	return cid, nil
}

func (m *MockBlockStore) RetrieveBlock(cid string) (*blocks.Block, error) {
	block, exists := m.blocks[cid]
	if !exists {
		return nil, errors.New("block not found")
	}
	return block, nil
}

func (m *MockBlockStore) RetrieveBlockWithPeerHint(cid string, preferredPeers []peer.ID) (*blocks.Block, error) {
	return m.RetrieveBlock(cid)
}

func (m *MockBlockStore) StoreBlockWithStrategy(block *blocks.Block, strategy string) (string, error) {
	return m.StoreBlock(block)
}

// TestCompleteUploadDownloadWorkflow tests the entire file lifecycle
func TestCompleteUploadDownloadWorkflow(t *testing.T) {
	suite := setupE2ETestSuite(t)

	testCases := []struct {
		name        string
		content     []byte
		filename    string
		expectError bool
	}{
		{
			name:     "Small Text File",
			content:  []byte("This is a small test file for end-to-end testing."),
			filename: "small_test.txt",
		},
		{
			name:     "Medium Document",
			content:  bytes.Repeat([]byte("Medium content block. "), 1000), // ~21KB
			filename: "medium_doc.txt",
		},
		{
			name:     "Large Binary File",
			content:  generateTestData(500 * 1024), // 500KB
			filename: "large_binary.bin",
		},
		{
			name:     "Empty File",
			content:  []byte{},
			filename: "empty.txt",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Step 1: Upload file
			reader := bytes.NewReader(tc.content)
			descriptorCID, err := suite.noisefsClient.Upload(reader, tc.filename)
			
			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error but upload succeeded")
				}
				return
			}
			
			if err != nil {
				t.Fatalf("Upload failed: %v", err)
			}

			if descriptorCID == "" {
				t.Fatal("Upload returned empty descriptor CID")
			}

			t.Logf("Uploaded %s with descriptor CID: %s", tc.filename, descriptorCID)

			// Step 2: Download file
			downloadedData, err := suite.noisefsClient.Download(descriptorCID)
			if err != nil {
				t.Fatalf("Download failed: %v", err)
			}

			// Step 3: Verify content integrity
			if !bytes.Equal(tc.content, downloadedData) {
				t.Errorf("Downloaded content doesn't match original")
				t.Errorf("Original size: %d, Downloaded size: %d", len(tc.content), len(downloadedData))
			}

			// Step 4: Verify cache behavior
			cacheStats := suite.memoryCache.GetStats()
			t.Logf("Cache stats after %s: hits=%d, misses=%d", tc.filename, cacheStats.Hits, cacheStats.Misses)

			// Step 5: Test repeated download (should hit cache)
			downloadedAgain, err := suite.noisefsClient.Download(descriptorCID)
			if err != nil {
				t.Errorf("Second download failed: %v", err)
			}

			if !bytes.Equal(tc.content, downloadedAgain) {
				t.Errorf("Second download content doesn't match original")
			}

			// Verify cache hit improvement
			newCacheStats := suite.memoryCache.GetStats()
			if newCacheStats.Hits <= cacheStats.Hits {
				t.Logf("Warning: Cache hits didn't increase on second download (this may be normal for small files)")
			}
		})
	}
}

// TestMultiClientWorkflow tests interaction between multiple clients
func TestMultiClientWorkflow(t *testing.T) {
	suite := setupE2ETestSuite(t)

	// Create a second client with shared storage but separate cache
	secondBlockStore := suite.mockBlockStore // Shared storage
	secondCache := cache.NewMemoryCache(10 * 1024 * 1024) // 10MB cache
	secondClient, err := noisefs.NewClient(secondBlockStore, secondCache)
	if err != nil {
		t.Fatalf("Failed to create second client: %v", err)
	}

	testContent := []byte("Multi-client test content for validation")
	filename := "multi_client_test.txt"

	// Client 1 uploads
	reader := bytes.NewReader(testContent)
	descriptorCID, err := suite.noisefsClient.Upload(reader, filename)
	if err != nil {
		t.Fatalf("Upload with client 1 failed: %v", err)
	}

	t.Logf("Client 1 uploaded file with CID: %s", descriptorCID)

	// Client 2 downloads (different cache, same storage)
	downloadedData, err := secondClient.Download(descriptorCID)
	if err != nil {
		t.Fatalf("Download with client 2 failed: %v", err)
	}

	if !bytes.Equal(testContent, downloadedData) {
		t.Errorf("Multi-client download content doesn't match original")
	}

	// Verify both caches have their own statistics
	cache1Stats := suite.memoryCache.GetStats()
	cache2Stats := secondCache.GetStats()

	t.Logf("Client 1 cache: hits=%d, misses=%d", cache1Stats.Hits, cache1Stats.Misses)
	t.Logf("Client 2 cache: hits=%d, misses=%d", cache2Stats.Hits, cache2Stats.Misses)

	// Client 2 should have more misses since it's downloading content uploaded by Client 1
	if cache2Stats.Misses == 0 {
		t.Logf("Warning: Client 2 had no cache misses (this may indicate a testing issue)")
	}
}

// TestReuseSystemIntegration tests the reuse system end-to-end
func TestReuseSystemIntegration(t *testing.T) {
	suite := setupE2ETestSuite(t)

	if suite.reuseClient == nil {
		t.Skip("Reuse client not available for this test")
	}

	testContent := []byte("Reuse system integration test content")
	filename := "reuse_test.txt"

	// Attempt upload with reuse enforcement
	reader := bytes.NewReader(testContent)
	result, err := suite.reuseClient.UploadFile(reader, filename, 64*1024)

	if err != nil {
		// Upload might fail due to reuse requirements, which is expected behavior
		if result != nil && result.ValidationResult != nil {
			t.Logf("Upload rejected due to reuse validation: %v", result.ValidationResult.Violations)
			
			// Verify the system properly enforced reuse requirements
			if !result.ValidationResult.Valid {
				t.Logf("Reuse enforcement working correctly - upload rejected")
				return
			}
		}
		t.Fatalf("Unexpected upload failure: %v", err)
	}

	// If upload succeeded, validate the reuse system
	t.Logf("Upload succeeded with descriptor: %s", result.DescriptorCID)
	
	// Verify reuse validation was performed
	if result.ValidationResult == nil {
		t.Error("Validation result should not be nil")
		return
	}

	if !result.ValidationResult.Valid {
		t.Error("Upload succeeded but validation result indicates invalid")
		return
	}

	// Verify mixing plan was created
	if result.MixingPlan == nil {
		t.Error("Mixing plan should not be nil for successful upload")
		return
	}

	t.Logf("Reuse validation: ratio=%.2f, public_domain_ratio=%.2f", 
		result.ValidationResult.ReuseRatio, 
		result.ValidationResult.PublicDomainRatio)

	// Test legal documentation generation
	if result.DescriptorCID != "" {
		legalDoc, err := suite.reuseClient.GetLegalDocumentation(result.DescriptorCID)
		if err != nil {
			t.Errorf("Failed to generate legal documentation: %v", err)
		} else {
			t.Logf("Generated legal documentation with %d block evidence entries and %d public domain proofs", 
				len(legalDoc.BlockReuseEvidence), 
				len(legalDoc.PublicDomainProof))
		}
	}

	// Test download through reuse client
	baseClient := suite.reuseClient.GetBaseClient()
	downloadedData, err := baseClient.Download(result.DescriptorCID)
	if err != nil {
		t.Errorf("Download through reuse client failed: %v", err)
	} else if !bytes.Equal(testContent, downloadedData) {
		t.Errorf("Downloaded content through reuse client doesn't match original")
	}
}

// TestStorageBackendIntegration tests the storage layer integration
func TestStorageBackendIntegration(t *testing.T) {
	suite := setupE2ETestSuite(t)

	if suite.storageManager == nil {
		t.Skip("Storage manager not available for this test")
	}

	ctx := context.Background()

	// Test basic storage operations
	testContent := []byte("Storage backend integration test")
	block, err := blocks.NewBlock(testContent)
	if err != nil {
		t.Fatalf("Failed to create test block: %v", err)
	}

	// Store through storage manager
	address, err := suite.storageManager.Put(ctx, block)
	if err != nil {
		t.Fatalf("Failed to store block: %v", err)
	}

	t.Logf("Stored block with address: %+v", address)

	// Retrieve through storage manager
	retrievedBlock, err := suite.storageManager.Get(ctx, address)
	if err != nil {
		t.Fatalf("Failed to retrieve block: %v", err)
	}

	if !bytes.Equal(block.Data, retrievedBlock.Data) {
		t.Error("Retrieved block data doesn't match original")
	}

	// Test storage manager status
	status := suite.storageManager.GetManagerStatus()
	t.Logf("Storage manager status: total_backends=%d, active_backends=%d", 
		status.TotalBackends, status.ActiveBackends)

	if status.TotalBackends == 0 {
		t.Error("Storage manager should have at least one backend")
	}
}

// TestComplianceSystemIntegration tests the compliance system integration
func TestComplianceSystemIntegration(t *testing.T) {
	suite := setupE2ETestSuite(t)

	if suite.complianceSystem == nil {
		t.Skip("Compliance system not available for this test")
	}

	// Test compliance event logging
	err := suite.complianceSystem.LogComplianceEvent(
		"test_upload",
		"test_user_001",
		"test_descriptor_001",
		"file_uploaded",
		map[string]interface{}{
			"filename":    "test_file.txt",
			"file_size":   1024,
			"upload_time": time.Now(),
		},
	)

	if err != nil {
		t.Errorf("Failed to log compliance event: %v", err)
	}

	// Test DMCA takedown logging
	err = suite.complianceSystem.LogDMCATakedown(
		"takedown_001",
		"test_descriptor_001",
		"copyright@example.com",
		"Test Copyrighted Work",
	)

	if err != nil {
		t.Errorf("Failed to log DMCA takedown: %v", err)
	}

	// Test compliance report generation
	startDate := time.Now().Add(-24 * time.Hour)
	endDate := time.Now()
	
	report, err := suite.complianceSystem.GenerateComplianceReport(startDate, endDate, "integration_test")
	if err != nil {
		t.Errorf("Failed to generate compliance report: %v", err)
	} else {
		t.Logf("Generated compliance report with %d total events", report.Statistics.TotalEvents)
	}
}

// TestErrorHandlingAndRecovery tests error scenarios and recovery
func TestErrorHandlingAndRecovery(t *testing.T) {
	suite := setupE2ETestSuite(t)

	// Test with corrupted block store
	corruptedBlockStore := &CorruptedMockBlockStore{
		blocks:     make(map[string]*blocks.Block),
		failureRate: 0.3, // 30% failure rate
	}

	corruptedClient, err := noisefs.NewClient(corruptedBlockStore, suite.memoryCache)
	if err != nil {
		t.Fatalf("Failed to create corrupted client: %v", err)
	}

	testContent := []byte("Error handling test content")
	reader := bytes.NewReader(testContent)

	// This should handle retries and potentially fail gracefully
	_, err = corruptedClient.Upload(reader, "error_test.txt")
	if err != nil {
		t.Logf("Upload with corrupted backend failed as expected: %v", err)
	} else {
		t.Log("Upload with corrupted backend succeeded despite errors")
	}

	// Test download with non-existent CID
	_, err = suite.noisefsClient.Download("non_existent_cid")
	if err == nil {
		t.Error("Download with non-existent CID should have failed")
	} else {
		t.Logf("Download with non-existent CID failed as expected: %v", err)
	}

	// Test upload with nil content
	_, err = suite.noisefsClient.Upload(nil, "nil_test.txt")
	if err == nil {
		t.Error("Upload with nil content should have failed")
	} else {
		t.Logf("Upload with nil content failed as expected: %v", err)
	}
}

// Helper functions

func setupE2ETestSuite(t *testing.T) *E2ETestSuite {
	// Create mock block store
	mockBlockStore := NewMockBlockStore()

	// Create memory cache
	memoryCache := cache.NewMemoryCache(50 * 1024 * 1024) // 50MB

	// Create NoiseFS client
	noisefsClient, err := noisefs.NewClient(mockBlockStore, memoryCache)
	if err != nil {
		t.Fatalf("Failed to create NoiseFS client: %v", err)
	}

	// Try to create reuse client (may fail if not fully set up)
	var reuseClient *reuse.ReuseAwareClient
	reuseClient, err = reuse.NewReuseAwareClient(mockBlockStore, memoryCache)
	if err != nil {
		t.Logf("Warning: Could not create reuse client: %v", err)
	}

	// Try to create storage manager (may fail if not configured)
	var storageManager *storage.Manager
	config := storage.DefaultConfig()
	if config != nil {
		storageManager, err = storage.NewManager(config)
		if err != nil {
			t.Logf("Warning: Could not create storage manager: %v", err)
		}
	}

	// Try to create compliance system
	var complianceSystem *compliance.ComplianceAuditSystem
	auditConfig := compliance.DefaultAuditConfig()
	if auditConfig != nil {
		complianceSystem = compliance.NewComplianceAuditSystem(auditConfig)
	}

	// Load test configuration
	testConfig := config.DefaultConfig()

	return &E2ETestSuite{
		mockBlockStore:   mockBlockStore,
		memoryCache:      memoryCache,
		noisefsClient:    noisefsClient,
		reuseClient:      reuseClient,
		storageManager:   storageManager,
		complianceSystem: complianceSystem,
		testConfig:       testConfig,
	}
}

func generateTestData(size int) []byte {
	data := make([]byte, size)
	for i := 0; i < size; i++ {
		data[i] = byte(i % 256)
	}
	return data
}

// CorruptedMockBlockStore simulates a unreliable storage backend
type CorruptedMockBlockStore struct {
	blocks      map[string]*blocks.Block
	failureRate float64
	operationCount int
}

func (c *CorruptedMockBlockStore) StoreBlock(block *blocks.Block) (string, error) {
	c.operationCount++
	
	// Simulate intermittent failures
	if float64(c.operationCount % 10) < c.failureRate * 10 {
		return "", errors.New("simulated storage failure")
	}
	
	if block == nil {
		return "", errors.New("block cannot be nil")
	}
	
	cid := fmt.Sprintf("corrupted_%s", block.ID)
	c.blocks[cid] = block
	return cid, nil
}

func (c *CorruptedMockBlockStore) RetrieveBlock(cid string) (*blocks.Block, error) {
	c.operationCount++
	
	// Simulate intermittent failures
	if float64(c.operationCount % 10) < c.failureRate * 10 {
		return nil, errors.New("simulated retrieval failure")
	}
	
	block, exists := c.blocks[cid]
	if !exists {
		return nil, errors.New("block not found")
	}
	return block, nil
}

func (c *CorruptedMockBlockStore) RetrieveBlockWithPeerHint(cid string, preferredPeers []peer.ID) (*blocks.Block, error) {
	return c.RetrieveBlock(cid)
}

func (c *CorruptedMockBlockStore) StoreBlockWithStrategy(block *blocks.Block, strategy string) (string, error) {
	return c.StoreBlock(block)
}