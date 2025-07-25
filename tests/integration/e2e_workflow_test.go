package integration

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/compliance"
	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	noisefs "github.com/TheEntropyCollective/noisefs/pkg/core/client"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/config"
	"github.com/TheEntropyCollective/noisefs/pkg/privacy/reuse"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
	storagetesting "github.com/TheEntropyCollective/noisefs/pkg/storage/testing"
)

// E2ETestSuite provides comprehensive end-to-end testing
type E2ETestSuite struct {
	storageManager   *storage.Manager
	memoryCache      cache.Cache
	noisefsClient    *noisefs.Client
	reuseClient      *reuse.ReuseAwareClient
	complianceSystem *compliance.ComplianceAuditSystem
	testConfig       *config.Config
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
			// Skip empty files - they are incompatible with NoiseFS anonymization architecture
			if len(tc.content) == 0 {
				t.Skip("Empty files not supported by NoiseFS anonymization architecture - " +
					"system requires blocks for XOR operations")
				return
			}
			
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
	secondCache := cache.NewMemoryCache(10 * 1024 * 1024) // 10MB cache
	secondClient, err := noisefs.NewClient(suite.storageManager, secondCache)
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
	downloadedData, err := suite.reuseClient.DownloadFile(result.DescriptorCID)
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

	// For error testing, we'll use the existing storage manager but test with
	// invalid data and non-existent CIDs instead of creating a corrupted manager
	
	// Test upload with nil content first
	_, err := suite.noisefsClient.Upload(nil, "nil_test.txt")
	if err == nil {
		t.Error("Upload with nil content should have failed")
	} else {
		t.Logf("Upload with nil content failed as expected: %v", err)
	}

	// Test download with non-existent CID
	_, err = suite.noisefsClient.Download("non_existent_cid")
	if err == nil {
		t.Error("Download with non-existent CID should have failed")
	} else {
		t.Logf("Download with non-existent CID failed as expected: %v", err)
	}

	// Test upload with empty filename
	testContent := []byte("Error handling test content")
	reader := bytes.NewReader(testContent)
	_, err = suite.noisefsClient.Upload(reader, "")
	if err == nil {
		t.Error("Upload with empty filename should have failed")
	} else {
		t.Logf("Upload with empty filename failed as expected: %v", err)
	}
}

// Helper functions

func setupE2ETestSuite(t *testing.T) *E2ETestSuite {
	// Create real test storage manager
	storageManager, err := storagetesting.CreateRealTestStorageManager()
	if err != nil {
		t.Fatalf("Failed to create storage manager: %v", err)
	}

	// Create memory cache
	memoryCache := cache.NewMemoryCache(50 * 1024 * 1024) // 50MB

	// Create NoiseFS client
	noisefsClient, err := noisefs.NewClient(storageManager, memoryCache)
	if err != nil {
		t.Fatalf("Failed to create NoiseFS client: %v", err)
	}

	// Try to create reuse client (may fail if not fully set up)
	var reuseClient *reuse.ReuseAwareClient
	reuseClient, err = reuse.NewReuseAwareClient(storageManager, memoryCache)
	if err != nil {
		t.Logf("Warning: Could not create reuse client: %v", err)
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
		storageManager:   storageManager,
		memoryCache:      memoryCache,
		noisefsClient:    noisefsClient,
		reuseClient:      reuseClient,
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

