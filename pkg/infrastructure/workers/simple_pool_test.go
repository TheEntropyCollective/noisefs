package workers

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
)

// Mock storage manager for testing
type mockStorageManager struct {
	stored   map[string]*blocks.Block
	latency  time.Duration
	failRate float32
	callCount int
	mutex    sync.Mutex
}

func newMockStorageManager() *mockStorageManager {
	return &mockStorageManager{
		stored: make(map[string]*blocks.Block),
	}
}

func (m *mockStorageManager) Put(ctx context.Context, block *blocks.Block) (*storage.BlockAddress, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	m.callCount++
	if m.latency > 0 {
		time.Sleep(m.latency)
	}
	
	if m.failRate > 0 && float32(m.callCount)/(float32(m.callCount)+1) < m.failRate {
		return nil, fmt.Errorf("mock storage failure")
	}
	
	address := &storage.BlockAddress{ID: block.ID}
	m.stored[block.ID] = block
	return address, nil
}

func (m *mockStorageManager) Get(ctx context.Context, address *storage.BlockAddress) (*blocks.Block, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	m.callCount++
	if m.latency > 0 {
		time.Sleep(m.latency)
	}
	
	if m.failRate > 0 && float32(m.callCount)/(float32(m.callCount)+1) < m.failRate {
		return nil, fmt.Errorf("mock retrieval failure")
	}
	
	block, exists := m.stored[address.ID]
	if !exists {
		return nil, fmt.Errorf("block not found: %s", address.ID)
	}
	return block, nil
}

// Mock client for testing
type mockClient struct {
	stored   map[string]*blocks.Block
	latency  time.Duration
	failRate float32
	callCount int
	mutex    sync.Mutex
}

func newMockClient() *mockClient {
	return &mockClient{
		stored: make(map[string]*blocks.Block),
	}
}

func (m *mockClient) StoreBlockWithCache(block *blocks.Block) (string, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	m.callCount++
	if m.latency > 0 {
		time.Sleep(m.latency)
	}
	
	if m.failRate > 0 && float32(m.callCount)/(float32(m.callCount)+1) < m.failRate {
		return "", fmt.Errorf("mock client storage failure")
	}
	
	cid := fmt.Sprintf("cid-%s", block.ID)
	m.stored[cid] = block
	return cid, nil
}

func TestSimpleWorkerPoolParallelXOR(t *testing.T) {
	pool := NewSimpleWorkerPool(runtime.NumCPU())
	
	// Create test blocks
	blockCount := 10
	blockSize := 1024
	
	dataBlocks := make([]*blocks.Block, blockCount)
	randomizer1Blocks := make([]*blocks.Block, blockCount)
	randomizer2Blocks := make([]*blocks.Block, blockCount)
	
	for i := 0; i < blockCount; i++ {
		data := make([]byte, blockSize)
		for j := range data {
			data[j] = byte(i + j) // Some test pattern
		}
		
		var err error
		dataBlocks[i], err = blocks.NewBlock(data)
		if err != nil {
			t.Fatalf("Failed to create data block %d: %v", i, err)
		}
		
		randomizer1Blocks[i], err = blocks.NewRandomBlock(blockSize)
		if err != nil {
			t.Fatalf("Failed to create randomizer1 block %d: %v", i, err)
		}
		
		randomizer2Blocks[i], err = blocks.NewRandomBlock(blockSize)
		if err != nil {
			t.Fatalf("Failed to create randomizer2 block %d: %v", i, err)
		}
	}
	
	// Perform parallel XOR
	start := time.Now()
	ctx := context.Background()
	xorBlocks, err := pool.ParallelXOR(ctx, dataBlocks, randomizer1Blocks, randomizer2Blocks)
	duration := time.Since(start)
	
	if err != nil {
		t.Fatalf("Parallel XOR failed: %v", err)
	}
	
	// Verify results
	if len(xorBlocks) != blockCount {
		t.Fatalf("Expected %d XOR blocks, got %d", blockCount, len(xorBlocks))
	}
	
	// Verify XOR correctness by reversing
	for i, xorBlock := range xorBlocks {
		reverse, err := xorBlock.XOR(randomizer1Blocks[i], randomizer2Blocks[i])
		if err != nil {
			t.Fatalf("Failed to reverse XOR for block %d: %v", i, err)
		}
		
		if string(reverse.Data) != string(dataBlocks[i].Data) {
			t.Fatalf("XOR reversal failed for block %d", i)
		}
	}
	
	t.Logf("Parallel XOR of %d blocks completed in %v", blockCount, duration)
}

func TestSimpleWorkerPoolParallelStorage(t *testing.T) {
	pool := NewSimpleWorkerPool(runtime.NumCPU())
	client := newMockClient()
	client.latency = 5 * time.Millisecond // Simulate some latency
	
	// Create test blocks
	blockCount := 10
	testBlocks := make([]*blocks.Block, blockCount)
	
	for i := 0; i < blockCount; i++ {
		data := []byte(fmt.Sprintf("test block %d data", i))
		block, err := blocks.NewBlock(data)
		if err != nil {
			t.Fatalf("Failed to create block %d: %v", i, err)
		}
		testBlocks[i] = block
	}
	
	// Perform parallel storage
	start := time.Now()
	ctx := context.Background()
	cids, err := pool.ParallelStorage(ctx, testBlocks, client)
	duration := time.Since(start)
	
	if err != nil {
		t.Fatalf("Parallel storage failed: %v", err)
	}
	
	// Verify results
	if len(cids) != blockCount {
		t.Fatalf("Expected %d CIDs, got %d", blockCount, len(cids))
	}
	
	// Verify all blocks were stored
	for i, cid := range cids {
		if _, exists := client.stored[cid]; !exists {
			t.Fatalf("Block %d was not stored (CID: %s)", i, cid)
		}
	}
	
	// Check that parallel execution was faster than sequential would be
	expectedSequentialDuration := time.Duration(blockCount) * client.latency
	if duration >= expectedSequentialDuration {
		t.Logf("Warning: Parallel execution (%v) not significantly faster than sequential (%v)", 
			duration, expectedSequentialDuration)
	}
	
	t.Logf("Parallel storage of %d blocks completed in %v", blockCount, duration)
}

func TestSimpleWorkerPoolParallelRetrieval(t *testing.T) {
	pool := NewSimpleWorkerPool(runtime.NumCPU())
	mockStorage := newMockStorageManager()
	mockStorage.latency = 3 * time.Millisecond // Simulate some latency
	
	// Store test blocks first
	blockCount := 10
	addresses := make([]*storage.BlockAddress, blockCount)
	originalBlocks := make([]*blocks.Block, blockCount)
	
	for i := 0; i < blockCount; i++ {
		data := []byte(fmt.Sprintf("retrieval test block %d", i))
		block, err := blocks.NewBlock(data)
		if err != nil {
			t.Fatalf("Failed to create block %d: %v", i, err)
		}
		
		originalBlocks[i] = block
		addresses[i] = &storage.BlockAddress{ID: block.ID}
		mockStorage.stored[block.ID] = block
	}
	
	// Perform parallel retrieval
	start := time.Now()
	ctx := context.Background()
	retrievedBlocks, err := pool.ParallelRetrieval(ctx, addresses, mockStorage)
	duration := time.Since(start)
	
	if err != nil {
		t.Fatalf("Parallel retrieval failed: %v", err)
	}
	
	// Verify results
	if len(retrievedBlocks) != blockCount {
		t.Fatalf("Expected %d blocks, got %d", blockCount, len(retrievedBlocks))
	}
	
	// Verify block data integrity
	for i, block := range retrievedBlocks {
		if block.ID != originalBlocks[i].ID {
			t.Fatalf("Block %d ID mismatch: expected %s, got %s", 
				i, originalBlocks[i].ID, block.ID)
		}
		
		if string(block.Data) != string(originalBlocks[i].Data) {
			t.Fatalf("Block %d data mismatch", i)
		}
	}
	
	t.Logf("Parallel retrieval of %d blocks completed in %v", blockCount, duration)
}

func TestSimpleWorkerPoolParallelRandomizerGeneration(t *testing.T) {
	pool := NewSimpleWorkerPool(runtime.NumCPU())
	
	blockCount := 50
	blockSize := 1024
	
	// Generate randomizer blocks in parallel
	start := time.Now()
	ctx := context.Background()
	randomizers, err := pool.ParallelRandomizerGeneration(ctx, blockCount, blockSize)
	duration := time.Since(start)
	
	if err != nil {
		t.Fatalf("Parallel randomizer generation failed: %v", err)
	}
	
	// Verify results
	if len(randomizers) != blockCount {
		t.Fatalf("Expected %d randomizer blocks, got %d", blockCount, len(randomizers))
	}
	
	// Verify all blocks are valid and correct size
	for i, block := range randomizers {
		if len(block.Data) != blockSize {
			t.Fatalf("Block %d size mismatch: expected %d, got %d", i, blockSize, len(block.Data))
		}
		
		if !block.VerifyIntegrity() {
			t.Fatalf("Block %d failed integrity check", i)
		}
	}
	
	t.Logf("Parallel generation of %d randomizer blocks completed in %v", blockCount, duration)
}

func BenchmarkSimpleWorkerPoolXORParallelism(b *testing.B) {
	blockCount := 100
	blockSize := 64 * 1024 // 64KB blocks
	
	// Prepare test data
	dataBlocks := make([]*blocks.Block, blockCount)
	randomizer1Blocks := make([]*blocks.Block, blockCount)
	randomizer2Blocks := make([]*blocks.Block, blockCount)
	
	for i := 0; i < blockCount; i++ {
		var err error
		dataBlocks[i], err = blocks.NewRandomBlock(blockSize)
		if err != nil {
			b.Fatalf("Failed to create data block: %v", err)
		}
		randomizer1Blocks[i], err = blocks.NewRandomBlock(blockSize)
		if err != nil {
			b.Fatalf("Failed to create randomizer1 block: %v", err)
		}
		randomizer2Blocks[i], err = blocks.NewRandomBlock(blockSize)
		if err != nil {
			b.Fatalf("Failed to create randomizer2 block: %v", err)
		}
	}
	
	b.Run("Sequential", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			for j := 0; j < blockCount; j++ {
				_, err := dataBlocks[j].XOR(randomizer1Blocks[j], randomizer2Blocks[j])
				if err != nil {
					b.Fatalf("XOR failed: %v", err)
				}
			}
		}
	})
	
	b.Run("Parallel", func(b *testing.B) {
		pool := NewSimpleWorkerPool(runtime.NumCPU())
		ctx := context.Background()
		
		b.ResetTimer()
		
		for i := 0; i < b.N; i++ {
			_, err := pool.ParallelXOR(ctx, dataBlocks, randomizer1Blocks, randomizer2Blocks)
			if err != nil {
				b.Fatalf("Parallel XOR failed: %v", err)
			}
		}
	})
}

func BenchmarkSimpleWorkerPoolConcurrencyLevels(b *testing.B) {
	blockCount := 50
	blockSize := 32 * 1024
	
	// Prepare test data
	dataBlocks := make([]*blocks.Block, blockCount)
	randomizer1Blocks := make([]*blocks.Block, blockCount)
	randomizer2Blocks := make([]*blocks.Block, blockCount)
	
	for i := 0; i < blockCount; i++ {
		var err error
		dataBlocks[i], err = blocks.NewRandomBlock(blockSize)
		if err != nil {
			b.Fatalf("Failed to create data block: %v", err)
		}
		randomizer1Blocks[i], err = blocks.NewRandomBlock(blockSize)
		if err != nil {
			b.Fatalf("Failed to create randomizer1 block: %v", err)
		}
		randomizer2Blocks[i], err = blocks.NewRandomBlock(blockSize)
		if err != nil {
			b.Fatalf("Failed to create randomizer2 block: %v", err)
		}
	}
	
	workerCounts := []int{1, 2, 4, 8, runtime.NumCPU()}
	
	for _, workerCount := range workerCounts {
		b.Run(fmt.Sprintf("Workers-%d", workerCount), func(b *testing.B) {
			pool := NewSimpleWorkerPool(workerCount)
			ctx := context.Background()
			
			b.ResetTimer()
			
			for i := 0; i < b.N; i++ {
				_, err := pool.ParallelXOR(ctx, dataBlocks, randomizer1Blocks, randomizer2Blocks)
				if err != nil {
					b.Fatalf("Parallel XOR failed: %v", err)
				}
			}
		})
	}
}