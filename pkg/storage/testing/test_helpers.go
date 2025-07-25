package testing

import (
	"context"
	cryptorand "crypto/rand"
	"fmt"
	"math/rand"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	noisefs "github.com/TheEntropyCollective/noisefs/pkg/core/client"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
)

// CreateTestStorageManager creates a pre-configured mock storage manager for testing
func CreateTestStorageManager() *MockStorageManager {
	manager := NewMockStorageManager()

	// Add a default mock backend
	backend := NewMockBackend("ipfs")
	manager.backends[manager.defaultBackend] = backend

	return manager
}

// CreateRealTestStorageManager creates a real storage.Manager for testing
func CreateRealTestStorageManager() (*storage.Manager, error) {
	// Register a mock backend using the "mock" type which is already supported
	storage.RegisterBackend("mock", func(cfg *storage.BackendConfig) (storage.Backend, error) {
		// Create a mock backend for testing
		return NewMockBackend("mock"), nil
	})

	config := storage.DefaultConfig()
	config.Backends = make(map[string]*storage.BackendConfig)

	// Configure to use the mock backend type (which is in the supported list)
	config.Backends["mock"] = &storage.BackendConfig{
		Type:    "mock",
		Enabled: true,
		Connection: &storage.ConnectionConfig{
			Endpoint: "memory://test",
		},
	}
	config.DefaultBackend = "mock"

	manager, err := storage.NewManager(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create test storage manager: %w", err)
	}

	// Start the manager
	err = manager.Start(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to start test storage manager: %w", err)
	}

	return manager, nil
}

// CreateTestStorageManagerWithBackends creates a mock storage manager with multiple backends
func CreateTestStorageManagerWithBackends() *MockStorageManager {
	manager := NewMockStorageManager()

	// Add multiple backends
	manager.backends["ipfs"] = NewMockBackend("ipfs")
	manager.backends["mock"] = NewMockBackend("mock")

	manager.defaultBackend = "ipfs"
	return manager
}

// CreateTestStorageManagerWithData creates a mock storage manager pre-populated with test data
func CreateTestStorageManagerWithData(blockCount int) (*MockStorageManager, map[string]*blocks.Block, error) {
	manager := CreateTestStorageManager()
	testBlocks := make(map[string]*blocks.Block)

	for i := 0; i < blockCount; i++ {
		// Create test data
		data := make([]byte, 1024) // 1KB blocks
		cryptorand.Read(data)

		block, err := blocks.NewBlock(data)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create test block %d: %w", i, err)
		}

		// Store the block
		cid, err := manager.Store(context.Background(), block)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to store test block %d: %w", i, err)
		}

		testBlocks[cid] = block
	}

	return manager, testBlocks, nil
}

// CreateTestNoiseClient creates a NoiseFS client with mock storage for testing
func CreateTestNoiseClient() (*noisefs.Client, *storage.Manager, error) {
	// Create a real storage manager with mock backend for compatibility
	config := storage.DefaultConfig()
	config.Backends = make(map[string]*storage.BackendConfig)

	// Configure to use mock storage which works well for testing
	config.Backends["mock"] = &storage.BackendConfig{
		Type: storage.BackendTypeMock,
		Connection: &storage.ConnectionConfig{
			Endpoint: "memory://test",
		},
	}
	config.DefaultBackend = "mock"

	manager, err := storage.NewManager(config)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create test storage manager: %w", err)
	}

	// Start the manager
	err = manager.Start(context.Background())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to start test storage manager: %w", err)
	}

	cache := cache.NewMemoryCache(100)
	client, err := noisefs.NewClient(manager, cache)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create test NoiseFS client: %w", err)
	}

	return client, manager, nil
}

// CreateTestNoiseClientWithData creates a NoiseFS client with pre-populated test data
func CreateTestNoiseClientWithData(blockCount int) (*noisefs.Client, *storage.Manager, map[string]*blocks.Block, error) {
	manager, err := CreateRealTestStorageManager()
	if err != nil {
		return nil, nil, nil, err
	}

	cache := cache.NewMemoryCache(100)
	client, err := noisefs.NewClient(manager, cache)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create test NoiseFS client: %w", err)
	}

	// Pre-populate with test data
	testBlocks := make(map[string]*blocks.Block)
	for i := 0; i < blockCount; i++ {
		data := make([]byte, 1024)
		cryptorand.Read(data)

		block, err := blocks.NewBlock(data)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to create test block %d: %w", i, err)
		}

		cid, err := client.StoreBlockWithCache(context.Background(), block)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to store test block %d: %w", i, err)
		}

		testBlocks[cid] = block
	}

	return client, manager, testBlocks, nil
}

// CorruptedMockStorageManager simulates a storage manager with corruption issues
type CorruptedMockStorageManager struct {
	*MockStorageManager
	corruptionRate float64 // 0.0 to 1.0
}

// NewCorruptedMockStorageManager creates a storage manager that simulates data corruption
func NewCorruptedMockStorageManager(corruptionRate float64) *CorruptedMockStorageManager {
	return &CorruptedMockStorageManager{
		MockStorageManager: CreateTestStorageManager(),
		corruptionRate:     corruptionRate,
	}
}

// Retrieve overrides the base method to simulate corruption
func (c *CorruptedMockStorageManager) Retrieve(ctx context.Context, cid string) (*blocks.Block, error) {
	block, err := c.MockStorageManager.Retrieve(ctx, cid)
	if err != nil {
		return nil, err
	}

	// Simulate corruption based on corruption rate
	if rand.Float64() < c.corruptionRate {
		// Corrupt the data by flipping a random bit
		if len(block.Data) > 0 {
			corruptedData := make([]byte, len(block.Data))
			copy(corruptedData, block.Data)

			// Flip a random bit
			byteIndex := rand.Intn(len(corruptedData))
			bitIndex := rand.Intn(8)
			corruptedData[byteIndex] ^= (1 << bitIndex)

			corruptedBlock, err := blocks.NewBlock(corruptedData)
			if err != nil {
				return nil, err
			}
			return corruptedBlock, nil
		}
	}

	return block, nil
}

// SlowMockStorageManager simulates a slow storage manager for performance testing
type SlowMockStorageManager struct {
	*MockStorageManager
}

// NewSlowMockStorageManager creates a storage manager with simulated latency
func NewSlowMockStorageManager() *SlowMockStorageManager {
	manager := CreateTestStorageManager()
	// Set 100ms latency simulation
	manager.SetLatencySimulation(100000000) // 100ms in nanoseconds

	return &SlowMockStorageManager{
		MockStorageManager: manager,
	}
}

// TestBlock creates a test block with specified data
func CreateTestBlock(data []byte) (*blocks.Block, error) {
	if data == nil {
		// Create random test data
		data = make([]byte, 1024)
		cryptorand.Read(data)
	}
	return blocks.NewBlock(data)
}

// CreateTestBlocks creates multiple test blocks
func CreateTestBlocks(count int, size int) ([]*blocks.Block, error) {
	testBlocks := make([]*blocks.Block, count)

	for i := 0; i < count; i++ {
		data := make([]byte, size)
		cryptorand.Read(data)

		block, err := blocks.NewBlock(data)
		if err != nil {
			return nil, fmt.Errorf("failed to create test block %d: %w", i, err)
		}

		testBlocks[i] = block
	}

	return testBlocks, nil
}

// SetupTestEnvironment sets up a complete test environment with storage manager and client
func SetupTestEnvironment() (*noisefs.Client, *storage.Manager, cache.Cache, error) {
	manager, err := CreateRealTestStorageManager()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create test storage manager: %w", err)
	}

	blockCache := cache.NewMemoryCache(100)

	client, err := noisefs.NewClient(manager, blockCache)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create test environment: %w", err)
	}

	return client, manager, blockCache, nil
}
