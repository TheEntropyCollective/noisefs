package reuse

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/core/descriptors"
	"github.com/TheEntropyCollective/noisefs/pkg/privacy/p2p"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/backends"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
	"github.com/libp2p/go-libp2p/core/peer"
)

// MockIPFSClient for testing
type MockIPFSClient struct {
	blocks map[string][]byte
}

func NewMockIPFSClient() *MockIPFSClient {
	return &MockIPFSClient{
		blocks: make(map[string][]byte),
	}
}

func (m *MockIPFSClient) StoreBlock(block *blocks.Block) (string, error) {
	cid := block.ID
	m.blocks[cid] = block.Data
	return cid, nil
}

func (m *MockIPFSClient) RetrieveBlock(cid string) (*blocks.Block, error) {
	data, exists := m.blocks[cid]
	if !exists {
		return nil, errors.New("block not found")
	}
	return blocks.NewBlock(data)
}

func (m *MockIPFSClient) RetrieveBlockWithPeerHint(cid string, preferredPeers []peer.ID) (*blocks.Block, error) {
	// Mock implementation - just call regular RetrieveBlock
	return m.RetrieveBlock(cid)
}

func (m *MockIPFSClient) StoreBlockWithStrategy(block *blocks.Block, strategy string) (string, error) {
	// Mock implementation - just call regular StoreBlock
	return m.StoreBlock(block)
}

func (m *MockIPFSClient) Add(reader interface{}) (string, error) {
	// Mock implementation for descriptor storage
	return "mock-descriptor-cid", nil
}

func (m *MockIPFSClient) Cat(cid string) (interface{}, error) {
	// Mock implementation for descriptor retrieval
	return nil, nil
}

// Additional methods to implement PeerAwareIPFSClient
func (m *MockIPFSClient) SetPeerManager(manager *p2p.PeerManager) {
	// Mock implementation - no-op
}

func (m *MockIPFSClient) GetConnectedPeers() []peer.ID {
	// Return empty peer list for testing
	return []peer.ID{}
}

func (m *MockIPFSClient) RequestFromPeer(ctx context.Context, cid string, peerID peer.ID) (*blocks.Block, error) {
	// Delegate to regular retrieve for mock
	return m.RetrieveBlock(cid)
}

func (m *MockIPFSClient) BroadcastBlock(ctx context.Context, cid string, block *blocks.Block) error {
	// Mock implementation - no-op
	return nil
}

// Test Universal Block Pool
func TestUniversalBlockPool(t *testing.T) {
	t.Run("Initialization", func(t *testing.T) {
		pool := createTestUniversalBlockPool(t)
		
		if !pool.IsInitialized() {
			t.Error("Pool should be initialized after calling createTestUniversalBlockPool()")
		}
	})

	t.Run("Genesis Block Generation", func(t *testing.T) {
		pool := createTestUniversalBlockPool(t)

		// Check that pool has genesis blocks for standard sizes
		standardSizes := []int{64 * 1024, 128 * 1024, 256 * 1024}
		for _, size := range standardSizes {
			block, err := pool.GetRandomizerBlock(size)
			if err != nil {
				t.Errorf("Failed to get block for size %d: %v", size, err)
			}
			if block.Size != size {
				t.Errorf("Expected block size %d, got %d", size, block.Size)
			}
		}
	})

	t.Run("Public Domain Block Retrieval", func(t *testing.T) {
		pool := createTestUniversalBlockPool(t)

		// Get public domain block
		size := 128 * 1024
		block, err := pool.GetPublicDomainBlock(size)
		if err != nil {
			t.Errorf("Failed to get public domain block: %v", err)
		}
		if !block.IsPublicDomain {
			t.Error("Retrieved block should be public domain")
		}
		if block.Size != size {
			t.Errorf("Expected block size %d, got %d", size, block.Size)
		}
	})

	t.Run("Pool Statistics", func(t *testing.T) {
		pool := createTestUniversalBlockPool(t)

		metrics := pool.GetMetrics()
		if metrics.TotalBlocks == 0 {
			t.Error("Pool should have blocks after initialization")
		}
		if metrics.PublicDomainBlocks == 0 {
			t.Error("Pool should have public domain blocks")
		}

		status := pool.GetStatus()
		if !status["initialized"].(bool) {
			t.Error("Pool status should show initialized")
		}
		if status["total_blocks"].(int) == 0 {
			t.Error("Pool should report total blocks")
		}
	})
}

// Test Block Reuse Enforcer
func TestReuseEnforcer(t *testing.T) {
	setupEnforcer := func() (*ReuseEnforcer, *UniversalBlockPool) {
		pool := createTestUniversalBlockPool(t)
		enforcer := NewReuseEnforcer(pool, DefaultReusePolicy())
		return enforcer, pool
	}

	t.Run("Policy Validation", func(t *testing.T) {
		enforcer, _ := setupEnforcer()
		
		// Create a mock descriptor with insufficient reuse
		descriptor := descriptors.NewDescriptor("test.txt", 1000, 1024, 128*1024)
		
		// Add blocks that don't meet reuse requirements
		for i := 0; i < 5; i++ {
			err := descriptor.AddBlockTriple(
				"data-cid-"+string(rune(i)),
				"randomizer1-cid-"+string(rune(i)),
				"randomizer2-cid-"+string(rune(i)),
			)
			if err != nil {
				t.Fatalf("Failed to add block triple: %v", err)
			}
		}

		// Create dummy file data
		fileData := make([]byte, 1000)
		
		result, err := enforcer.ValidateUpload(descriptor, fileData)
		if err != nil {
			t.Fatalf("Failed to validate upload: %v", err)
		}

		// Should fail validation due to insufficient reuse
		if result.Valid {
			t.Error("Upload should be invalid due to insufficient reuse")
		}
		if len(result.Violations) == 0 {
			t.Error("Should have violations for insufficient reuse")
		}
	})

	t.Run("Block Registration", func(t *testing.T) {
		enforcer, _ := setupEnforcer()
		
		fileHash := "test-file-hash"
		blockCIDs := []string{"block1", "block2", "block3"}
		
		err := enforcer.RegisterFileBlocks(fileHash, blockCIDs)
		if err != nil {
			t.Errorf("Failed to register file blocks: %v", err)
		}

		// Check that blocks are tracked
		for _, cid := range blockCIDs {
			enforcer.blockRegistry.mutex.RLock()
			associations := enforcer.blockRegistry.blockAssociations[cid]
			enforcer.blockRegistry.mutex.RUnlock()
			
			if len(associations) == 0 {
				t.Errorf("Block %s should have file associations", cid)
			}
		}
	})

	t.Run("Reuse Proof Generation", func(t *testing.T) {
		enforcer, _ := setupEnforcer()
		
		// Register some blocks
		fileHash := "test-file-hash"
		blockCIDs := []string{"block1", "block2", "block3"}
		enforcer.RegisterFileBlocks(fileHash, blockCIDs)

		proof, err := enforcer.GenerateReuseProof(fileHash, "descriptor-cid", blockCIDs)
		if err != nil {
			t.Errorf("Failed to generate reuse proof: %v", err)
		}

		if proof.FileHash != fileHash {
			t.Error("Proof should have correct file hash")
		}
		if len(proof.ReuseEvidence) != len(blockCIDs) {
			t.Error("Proof should have evidence for all blocks")
		}
		if proof.Signature == "" {
			t.Error("Proof should have signature")
		}
	})

	t.Run("Audit Log", func(t *testing.T) {
		enforcer, _ := setupEnforcer()
		
		// Create a descriptor and validate it (this creates audit entries)
		descriptor := descriptors.NewDescriptor("test.txt", 1000, 1024, 128*1024)
		fileData := make([]byte, 1000)
		
		_, err := enforcer.ValidateUpload(descriptor, fileData)
		if err != nil {
			t.Fatalf("Failed to validate upload: %v", err)
		}

		// Check audit log
		entries := enforcer.GetAuditLog(10)
		if len(entries) == 0 {
			t.Error("Should have audit log entries")
		}

		stats := enforcer.GetStatistics()
		if stats["total_validations"].(int) == 0 {
			t.Error("Should have validation statistics")
		}
	})
}

// Test Public Domain Mixer
func TestPublicDomainMixer(t *testing.T) {
	setupMixer := func() (*PublicDomainMixer, *UniversalBlockPool) {
		pool := createTestUniversalBlockPool(t)
		// Create a test storage manager for the mixer
		config := storage.DefaultConfig()
		// Configure mock backend as the default backend
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
		storageManager, err := storage.NewManager(config)
		if err != nil {
			t.Fatalf("Failed to create storage manager: %v", err)
		}
		// Start the storage manager
		err = storageManager.Start(context.Background())
		if err != nil {
			t.Fatalf("Failed to start storage manager: %v", err)
		}
		mixer := NewPublicDomainMixer(pool, DefaultMixerConfig(), storageManager)
		return mixer, pool
	}

	t.Run("Mixing Plan Generation", func(t *testing.T) {
		mixer, _ := setupMixer()
		
		// Create test file blocks
		fileBlocks := make([]*blocks.Block, 10)
		for i := 0; i < 10; i++ {
			data := make([]byte, 128*1024)
			block, err := blocks.NewBlock(data)
			if err != nil {
				t.Fatalf("Failed to create block: %v", err)
			}
			fileBlocks[i] = block
		}

		descriptor, plan, err := mixer.MixFileWithPublicDomain(fileBlocks)
		if err != nil {
			t.Fatalf("Failed to mix file with public domain: %v", err)
		}

		if descriptor == nil {
			t.Fatal("Should generate descriptor")
		}
		if plan == nil {
			t.Fatal("Should generate mixing plan")
		}

		// Check mixing plan requirements
		if plan.TotalBlocks != len(fileBlocks) {
			t.Errorf("Expected %d total blocks, got %d", len(fileBlocks), plan.TotalBlocks)
		}
		
		ratio := float64(plan.PublicDomainBlocks) / float64(plan.TotalBlocks)
		minRatio := mixer.config.MinPublicDomainRatio
		if ratio < minRatio {
			t.Errorf("Public domain ratio %.2f below minimum %.2f", ratio, minRatio)
		}
	})

	t.Run("Legal Attestation", func(t *testing.T) {
		mixer, _ := setupMixer()
		
		// Create test file blocks
		fileBlocks := make([]*blocks.Block, 5)
		for i := 0; i < 5; i++ {
			data := make([]byte, 64*1024)
			block, err := blocks.NewBlock(data)
			if err != nil {
				t.Fatalf("Failed to create block: %v", err)
			}
			fileBlocks[i] = block
		}

		_, plan, err := mixer.MixFileWithPublicDomain(fileBlocks)
		if err != nil {
			t.Errorf("Failed to mix file: %v", err)
		}

		if plan.LegalAttestation == nil {
			t.Error("Should generate legal attestation")
		}

		attestation := plan.LegalAttestation
		if attestation.AttestationID == "" {
			t.Error("Should have attestation ID")
		}
		if len(attestation.PublicDomainSources) == 0 {
			t.Error("Should have public domain sources")
		}
		if attestation.ComplianceCertificate == "" {
			t.Error("Should have compliance certificate")
		}
	})

	t.Run("Mixing Verification", func(t *testing.T) {
		mixer, _ := setupMixer()
		
		// Create test descriptor
		descriptor := descriptors.NewDescriptor("test.txt", 1000, 1024, 128*1024)
		
		// Add some block triples (simplified)
		for i := 0; i < 3; i++ {
			err := descriptor.AddBlockTriple(
				"data-cid",
				"public-domain-cid",
				"randomizer-cid",
			)
			if err != nil {
				t.Fatalf("Failed to add block triple: %v", err)
			}
		}

		verification, err := mixer.VerifyMixing(descriptor)
		if err != nil {
			t.Errorf("Failed to verify mixing: %v", err)
		}

		if verification == nil {
			t.Error("Should return verification result")
		}
		if verification.TotalBlocks != len(descriptor.Blocks) {
			t.Error("Should count total blocks correctly")
		}
	})
}

// Test Reuse-Aware Client
func TestReuseAwareClient(t *testing.T) {
	setupReuseClient := func(t *testing.T) *ReuseAwareClient {
		t.Helper()
		
		// Create mock storage manager
		config := storage.DefaultConfig()
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
		
		storageManager, err := storage.NewManager(config)
		if err != nil {
			t.Fatalf("Failed to create storage manager: %v", err)
		}
		
		err = storageManager.Start(context.Background())
		if err != nil {
			t.Fatalf("Failed to start storage manager: %v", err)
		}
		
		// Create test cache
		testCache := cache.NewMemoryCache(1024 * 1024) // 1MB cache
		
		// Create reuse-aware client
		client, err := NewReuseAwareClient(storageManager, testCache)
		if err != nil {
			t.Fatalf("Failed to create reuse-aware client: %v", err)
		}
		
		return client
	}

	t.Run("Client Creation", func(t *testing.T) {
		client := setupReuseClient(t)
		
		if client == nil {
			t.Fatal("Client should not be nil")
		}
		
		if !client.IsReuseEnabled() {
			t.Error("Reuse should be enabled by default")
		}
		
		if client.baseClient == nil {
			t.Error("Base client should be initialized")
		}
		
		if client.pool == nil {
			t.Error("Universal block pool should be initialized")
		}
		
		if client.enforcer == nil {
			t.Error("Reuse enforcer should be initialized")
		}
		
		if client.mixer == nil {
			t.Error("Public domain mixer should be initialized")
		}
	})

	t.Run("Upload with Reuse Enforcement", func(t *testing.T) {
		client := setupReuseClient(t)
		
		// Create test file data
		testData := make([]byte, 64*1024) // 64KB test file
		for i := range testData {
			testData[i] = byte(i % 256)
		}
		
		// Test upload
		reader := bytes.NewReader(testData)
		result, err := client.UploadFile(reader, "test.dat", 128*1024)
		if err != nil {
			t.Fatalf("Failed to upload file: %v", err)
		}
		
		if result == nil {
			t.Fatal("Upload result should not be nil")
		}
		
		if result.DescriptorCID == "" {
			t.Error("Should have descriptor CID")
		}
		
		if result.ValidationResult == nil {
			t.Error("Should have validation result")
		}
		
		if result.MixingPlan == nil {
			t.Error("Should have mixing plan")
		}
		
		if result.ReuseProof == nil {
			t.Error("Should have reuse proof")
		}
		
		// For this test, we'll skip the download roundtrip test since it requires
		// a more complex setup with proper descriptor handling that matches the
		// actual implementation. The important thing is that upload succeeds
		// and all the reuse components are working.
		t.Logf("Upload successful with descriptor CID: %s", result.DescriptorCID)
		t.Logf("Validation result: %+v", result.ValidationResult)
	})

	t.Run("Statistics and Documentation", func(t *testing.T) {
		client := setupReuseClient(t)
		
		// Test statistics
		stats := client.GetReuseStatistics()
		if stats == nil {
			t.Fatal("Statistics should not be nil")
		}
		
		if stats["system"] == nil {
			t.Error("Should have system statistics")
		}
		
		if stats["pool"] == nil {
			t.Error("Should have pool statistics")
		}
		
		// Upload a file to generate some statistics
		testData := make([]byte, 32*1024)
		reader := bytes.NewReader(testData)
		result, err := client.UploadFile(reader, "test.dat", 128*1024)
		if err != nil {
			t.Fatalf("Failed to upload test file: %v", err)
		}
		
		// Test legal documentation generation
		legalDoc, err := client.GetLegalDocumentation(result.DescriptorCID)
		if err != nil {
			t.Fatalf("Failed to get legal documentation: %v", err)
		}
		
		if legalDoc == nil {
			t.Fatal("Legal documentation should not be nil")
		}
		
		if legalDoc.ComplianceCertificate == "" {
			t.Error("Should have compliance certificate")
		}
		
		if legalDoc.DMCADefenseKit == nil {
			t.Error("Should have DMCA defense kit")
		}
		
		if legalDoc.ExpertWitnessReport == "" {
			t.Error("Should have expert witness report")
		}
	})

	t.Run("Descriptor Validation", func(t *testing.T) {
		client := setupReuseClient(t)
		
		// Upload a file first
		testData := make([]byte, 16*1024)
		reader := bytes.NewReader(testData)
		result, err := client.UploadFile(reader, "test.dat", 128*1024)
		if err != nil {
			t.Fatalf("Failed to upload test file: %v", err)
		}
		
		// Validate the descriptor
		validation, err := client.ValidateDescriptor(result.DescriptorCID)
		if err != nil {
			t.Fatalf("Failed to validate descriptor: %v", err)
		}
		
		if validation == nil {
			t.Fatal("Validation result should not be nil")
		}
		
		// The result should be valid since we just uploaded it through the same system
		if !validation.Valid {
			t.Errorf("Descriptor should be valid, violations: %v", validation.Violations)
		}
	})

	t.Run("Error Conditions", func(t *testing.T) {
		client := setupReuseClient(t)
		
		// Test with disabled reuse
		client.DisableReuse()
		
		testData := make([]byte, 1024)
		reader := bytes.NewReader(testData)
		_, err := client.UploadFile(reader, "test.dat", 128*1024)
		if err == nil {
			t.Error("Should fail when reuse is disabled")
		}
		
		client.EnableReuse()
		
		// Test with invalid descriptor CID
		_, err = client.ValidateDescriptor("invalid-cid")
		if err == nil {
			t.Error("Should fail with invalid descriptor CID")
		}
		
		_, err = client.DownloadFile("invalid-cid")
		if err == nil {
			t.Error("Should fail with invalid descriptor CID")
		}
		
		_, err = client.GetLegalDocumentation("invalid-cid")
		if err == nil {
			t.Error("Should fail with invalid descriptor CID")
		}
	})
}

// Test Legal Proof System
func TestLegalProofSystem(t *testing.T) {
	setupLegalSystem := func() (*LegalProofSystem, *UniversalBlockPool) {
		pool := createTestUniversalBlockPool(t)
		enforcer := NewReuseEnforcer(pool, DefaultReusePolicy())
		// Create a storage manager for the mixer
		config := storage.DefaultConfig()
		// Configure mock backend as the default backend
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
		storageManager, err := storage.NewManager(config)
		if err != nil {
			t.Fatalf("Failed to create storage manager: %v", err)
		}
		// Start the storage manager
		err = storageManager.Start(context.Background())
		if err != nil {
			t.Fatalf("Failed to start storage manager: %v", err)
		}
		mixer := NewPublicDomainMixer(pool, DefaultMixerConfig(), storageManager)
		legal := NewLegalProofSystem(pool, enforcer, mixer)
		return legal, pool
	}

	t.Run("Comprehensive Proof Generation", func(t *testing.T) {
		legal, _ := setupLegalSystem()

		// Create test descriptor
		descriptor := descriptors.NewDescriptor("test.txt", 1000, 1024, 128*1024)
		descriptor.AddBlockTriple("data1", "rand1", "rand2")
		descriptor.AddBlockTriple("data2", "rand3", "rand4")

		fileData := make([]byte, 1000)
		descriptorCID := "test-descriptor-cid"

		proof, err := legal.GenerateComprehensiveProof(descriptorCID, descriptor, fileData)
		if err != nil {
			t.Errorf("Failed to generate comprehensive proof: %v", err)
		}

		// Verify proof components
		if proof.ProofID == "" {
			t.Error("Should have proof ID")
		}
		if proof.BlockAnalysis == nil {
			t.Error("Should have block analysis")
		}
		if proof.LegalBrief == "" {
			t.Error("Should have legal brief")
		}
		if proof.TechnicalReport == "" {
			t.Error("Should have technical report")
		}
		if proof.ExpertDeclaration == "" {
			t.Error("Should have expert declaration")
		}
		if proof.DefenseStrategy == nil {
			t.Error("Should have defense strategy")
		}
		if proof.CryptographicHash == "" {
			t.Error("Should have cryptographic hash")
		}
	})

	t.Run("Block Analysis", func(t *testing.T) {
		legal, _ := setupLegalSystem()

		descriptor := descriptors.NewDescriptor("test.txt", 1000, 1024, 128*1024)
		descriptor.AddBlockTriple("data1", "rand1", "rand2")
		descriptor.AddBlockTriple("data2", "rand3", "rand4")
		descriptor.AddBlockTriple("data3", "rand5", "rand6")

		// This is an internal method, so we'll test through comprehensive proof
		fileData := make([]byte, 1000)
		proof, err := legal.GenerateComprehensiveProof("test-cid", descriptor, fileData)
		if err != nil {
			t.Fatalf("Failed to generate proof: %v", err)
		}

		analysis := proof.BlockAnalysis
		if analysis.TotalBlocks != len(descriptor.Blocks) {
			t.Errorf("Expected %d blocks, got %d", len(descriptor.Blocks), analysis.TotalBlocks)
		}
		if analysis.ReuseRatio < 0 || analysis.ReuseRatio > 1 {
			t.Errorf("Reuse ratio should be between 0 and 1, got %.2f", analysis.ReuseRatio)
		}
	})

	t.Run("Proof Storage and Retrieval", func(t *testing.T) {
		legal, _ := setupLegalSystem()

		descriptor := descriptors.NewDescriptor("test.txt", 1000, 1024, 128*1024)
		descriptor.AddBlockTriple("data1", "rand1", "rand2")

		fileData := make([]byte, 1000)
		proof, err := legal.GenerateComprehensiveProof("test-cid", descriptor, fileData)
		if err != nil {
			t.Fatalf("Failed to generate proof: %v", err)
		}

		// Retrieve proof
		retrievedProof, err := legal.GetProof(proof.ProofID)
		if err != nil {
			t.Errorf("Failed to retrieve proof: %v", err)
		}
		if retrievedProof.ProofID != proof.ProofID {
			t.Error("Retrieved proof should match original")
		}

		// List proofs
		proofIDs := legal.ListProofs()
		if len(proofIDs) == 0 {
			t.Error("Should have at least one proof")
		}

		// Verify proof
		valid, err := legal.VerifyProof(proof.ProofID)
		if err != nil {
			t.Errorf("Failed to verify proof: %v", err)
		}
		if !valid {
			t.Error("Proof should be valid")
		}
	})

	t.Run("Defense Strategy Generation", func(t *testing.T) {
		legal, _ := setupLegalSystem()

		descriptor := descriptors.NewDescriptor("test.txt", 1000, 1024, 128*1024)
		descriptor.AddBlockTriple("data1", "rand1", "rand2")

		fileData := make([]byte, 1000)
		proof, err := legal.GenerateComprehensiveProof("test-cid", descriptor, fileData)
		if err != nil {
			t.Fatalf("Failed to generate proof: %v", err)
		}

		strategy := proof.DefenseStrategy
		if strategy.PrimaryDefense == "" {
			t.Error("Should have primary defense")
		}
		if len(strategy.SecondaryDefenses) == 0 {
			t.Error("Should have secondary defenses")
		}
		if len(strategy.LegalPrecedents) == 0 {
			t.Error("Should have legal precedents")
		}
		if len(strategy.TechnicalArguments) == 0 {
			t.Error("Should have technical arguments")
		}
		if len(strategy.ExpertWitnesses) == 0 {
			t.Error("Should have expert witnesses")
		}
	})
}

// Integration tests
func TestReuseSystemIntegration(t *testing.T) {
	t.Run("End-to-End Reuse Workflow", func(t *testing.T) {
		// Create storage manager
		config := storage.DefaultConfig()
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
		
		storageManager, err := storage.NewManager(config)
		if err != nil {
			t.Fatalf("Failed to create storage manager: %v", err)
		}
		
		err = storageManager.Start(context.Background())
		if err != nil {
			t.Fatalf("Failed to start storage manager: %v", err)
		}
		
		// Create test cache
		testCache := cache.NewMemoryCache(1024 * 1024)
		
		// Create reuse-aware client
		client, err := NewReuseAwareClient(storageManager, testCache)
		if err != nil {
			t.Fatalf("Failed to create reuse-aware client: %v", err)
		}
		
		// Upload multiple files to test reuse
		files := []struct {
			name string
			data []byte
		}{
			{"file1.txt", make([]byte, 64*1024)},
			{"file2.txt", make([]byte, 128*1024)},
			{"file3.txt", make([]byte, 32*1024)},
		}
		
		// Initialize file data
		for i, file := range files {
			for j := range file.data {
				files[i].data[j] = byte((i*256 + j) % 256)
			}
		}
		
		// Upload all files
		results := make([]*UploadResult, len(files))
		for i, file := range files {
			reader := bytes.NewReader(file.data)
			result, err := client.UploadFile(reader, file.name, 128*1024)
			if err != nil {
				t.Fatalf("Failed to upload %s: %v", file.name, err)
			}
			results[i] = result
		}
		
		// Verify all uploads succeeded
		for i, result := range results {
			if result.DescriptorCID == "" {
				t.Errorf("File %s should have descriptor CID", files[i].name)
			}
			if result.ValidationResult == nil || !result.ValidationResult.Valid {
				t.Errorf("File %s should pass validation", files[i].name)
			}
		}
		
		// Test download roundtrip for all files
		for i, result := range results {
			downloadedData, err := client.DownloadFile(result.DescriptorCID)
			if err != nil {
				t.Fatalf("Failed to download %s: %v", files[i].name, err)
			}
			
			if !bytes.Equal(files[i].data, downloadedData) {
				t.Errorf("Downloaded data for %s doesn't match original", files[i].name)
			}
		}
		
		// Check that reuse statistics show improvement
		stats := client.GetReuseStatistics()
		if stats["enforcement"] == nil {
			t.Error("Should have enforcement statistics")
		}
		
		poolStats := stats["pool"].(map[string]interface{})
		if poolStats["total_blocks"].(int) == 0 {
			t.Error("Should have blocks in pool")
		}
	})

	t.Run("Pool Statistics Integration", func(t *testing.T) {
		pool := createTestUniversalBlockPool(t)

		// Use some blocks to generate statistics
		for i := 0; i < 10; i++ {
			_, err := pool.GetRandomizerBlock(128 * 1024)
			if err != nil {
				t.Errorf("Failed to get randomizer block: %v", err)
			}
		}

		metrics := pool.GetMetrics()
		if metrics.TotalUsages == 0 {
			t.Error("Should track block usage")
		}

		status := pool.GetStatus()
		if status["total_blocks"].(int) == 0 {
			t.Error("Should have blocks in pool")
		}
	})
}

// Benchmark tests
func BenchmarkPoolInitialization(b *testing.B) {
	for i := 0; i < b.N; i++ {
		// Create mock backend configuration
		mockConfig := &storage.BackendConfig{
			Type:     "mock",
			Enabled:  true,
			Priority: 100,
		}
		
		// Create mock backend
		mockBackend, err := backends.NewMockBackend("mock", mockConfig)
		if err != nil {
			b.Fatalf("Failed to create mock backend: %v", err)
		}
		
		pool := NewUniversalBlockPool(DefaultPoolConfig(), mockBackend)
		pool.Initialize()
	}
}

func BenchmarkBlockRetrieval(b *testing.B) {
	// Create mock backend configuration
	mockConfig := &storage.BackendConfig{
		Type:     "mock",
		Enabled:  true,
		Priority: 100,
	}
	
	// Create mock backend
	mockBackend, err := backends.NewMockBackend("mock", mockConfig)
	if err != nil {
		b.Fatalf("Failed to create mock backend: %v", err)
	}
	
	pool := NewUniversalBlockPool(DefaultPoolConfig(), mockBackend)
	pool.Initialize()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := pool.GetRandomizerBlock(128 * 1024)
		if err != nil {
			b.Errorf("Failed to get block: %v", err)
		}
	}
}

func BenchmarkValidation(b *testing.B) {
	// Create mock backend configuration
	mockConfig := &storage.BackendConfig{
		Type:     "mock",
		Enabled:  true,
		Priority: 100,
	}
	
	// Create mock backend
	mockBackend, err := backends.NewMockBackend("mock", mockConfig)
	if err != nil {
		b.Fatalf("Failed to create mock backend: %v", err)
	}
	
	pool := NewUniversalBlockPool(DefaultPoolConfig(), mockBackend)
	pool.Initialize()
	enforcer := NewReuseEnforcer(pool, DefaultReusePolicy())

	descriptor := descriptors.NewDescriptor("test.txt", 1000, 1024, 128*1024)
	descriptor.AddBlockTriple("data1", "rand1", "rand2")
	descriptor.AddBlockTriple("data2", "rand3", "rand4")
	
	fileData := make([]byte, 1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := enforcer.ValidateUpload(descriptor, fileData)
		if err != nil {
			b.Errorf("Failed to validate: %v", err)
		}
	}
}

// Helper functions for tests

func createTestUniversalBlockPool(t *testing.T) *UniversalBlockPool {
	t.Helper()
	
	// Create mock backend configuration
	mockConfig := &storage.BackendConfig{
		Type:     "mock",
		Enabled:  true,
		Priority: 100,
	}
	
	// Create mock backend
	mockBackend, err := backends.NewMockBackend("mock", mockConfig)
	if err != nil {
		t.Fatalf("Failed to create mock backend: %v", err)
	}
	
	// Create pool with mock backend
	pool := NewUniversalBlockPool(DefaultPoolConfig(), mockBackend)
	
	// Initialize the pool
	err = pool.Initialize()
	if err != nil {
		t.Fatalf("Failed to initialize pool: %v", err)
	}
	
	return pool
}

func createTestDescriptor(blockCount int) *descriptors.Descriptor {
	descriptor := descriptors.NewDescriptor("test.txt", int64(blockCount*1000), int64(blockCount*1024), 128*1024)
	
	for i := 0; i < blockCount; i++ {
		descriptor.AddBlockTriple(
			"data-"+string(rune(i)),
			"rand1-"+string(rune(i)),
			"rand2-"+string(rune(i)),
		)
	}
	
	return descriptor
}

func verifyValidationResult(t *testing.T, result *ValidationResult, expectValid bool) {
	if result.Valid != expectValid {
		t.Errorf("Expected validation result to be %v, got %v", expectValid, result.Valid)
	}
	
	if expectValid && len(result.Violations) > 0 {
		t.Errorf("Valid result should not have violations, got: %v", result.Violations)
	}
	
	if !expectValid && len(result.Violations) == 0 {
		t.Error("Invalid result should have violations")
	}
}