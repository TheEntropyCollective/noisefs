package reuse

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/core/descriptors"
	"github.com/TheEntropyCollective/noisefs/pkg/privacy/p2p"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/backends"
	"github.com/libp2p/go-libp2p/core/peer"
)

// Global initialization to ensure mock backend is registered only once
var mockBackendRegistered sync.Once

func init() {
	// Register mock backend globally to prevent race conditions
	mockBackendRegistered.Do(func() {
		storage.RegisterBackend("mock", func(cfg *storage.BackendConfig) (storage.Backend, error) {
			return backends.NewMockBackend("mock", cfg)
		})
	})
}

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
		// Create a test storage manager first
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
		
		// Get the default backend to use with the pool
		defaultBackend, err := storageManager.GetDefaultBackend()
		if err != nil {
			t.Fatalf("Failed to get default backend from storage manager: %v", err)
		}
		
		// Create pool with the same backend
		pool := NewUniversalBlockPool(DefaultPoolConfig(), defaultBackend)
		
		// Initialize the pool
		err = pool.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize pool: %v", err)
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
	t.Run("Client Creation", func(t *testing.T) {
		// Note: ReuseAwareClient requires a real ipfs.Client, not a mock
		// For unit testing, we'll skip this test as it requires real IPFS integration
		t.Skip("ReuseAwareClient requires *ipfs.Client type, not compatible with mocks")
	})

	t.Run("Upload with Reuse Enforcement", func(t *testing.T) {
		t.Skip("ReuseAwareClient requires *ipfs.Client type, not compatible with mocks")
	})

	t.Run("Statistics and Documentation", func(t *testing.T) {
		t.Skip("ReuseAwareClient requires *ipfs.Client type, not compatible with mocks")
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
		t.Skip("ReuseAwareClient requires *ipfs.Client type, not compatible with mocks")
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