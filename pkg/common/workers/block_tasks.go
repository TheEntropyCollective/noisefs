package workers

import (
	"context"
	"fmt"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
)

// XORTask performs XOR operation on blocks (3-tuple anonymization)
type XORTask struct {
	Index         int
	DataBlock     *blocks.Block
	Randomizer1   *blocks.Block
	Randomizer2   *blocks.Block
}

func (t *XORTask) ID() string {
	return fmt.Sprintf("xor-%d", t.Index)
}

func (t *XORTask) Execute(ctx context.Context) (interface{}, error) {
	result, err := t.DataBlock.XOR(t.Randomizer1, t.Randomizer2)
	if err != nil {
		return nil, fmt.Errorf("XOR operation failed for block %d: %w", t.Index, err)
	}
	return result, nil
}

// StorageTask stores a block using the storage manager
type StorageTask struct {
	Index         int
	Block         *blocks.Block
	StorageManager interface {
		Put(ctx context.Context, block *blocks.Block) (*storage.BlockAddress, error)
	}
}

func (t *StorageTask) ID() string {
	return fmt.Sprintf("store-%d", t.Index)
}

func (t *StorageTask) Execute(ctx context.Context) (interface{}, error) {
	address, err := t.StorageManager.Put(ctx, t.Block)
	if err != nil {
		return nil, fmt.Errorf("storage operation failed for block %d: %w", t.Index, err)
	}
	return address, nil
}

// RetrievalTask retrieves a block using the storage manager
type RetrievalTask struct {
	Index          int
	Address        *storage.BlockAddress
	StorageManager interface {
		Get(ctx context.Context, address *storage.BlockAddress) (*blocks.Block, error)
	}
}

func (t *RetrievalTask) ID() string {
	return fmt.Sprintf("retrieve-%d", t.Index)
}

func (t *RetrievalTask) Execute(ctx context.Context) (interface{}, error) {
	block, err := t.StorageManager.Get(ctx, t.Address)
	if err != nil {
		return nil, fmt.Errorf("retrieval operation failed for block %d: %w", t.Index, err)
	}
	return block, nil
}

// RandomizerGenerationTask generates randomizer blocks
type RandomizerGenerationTask struct {
	Index     int
	Size      int
	BlockType string // "randomizer1" or "randomizer2"
}

func (t *RandomizerGenerationTask) ID() string {
	return fmt.Sprintf("gen-%s-%d", t.BlockType, t.Index)
}

func (t *RandomizerGenerationTask) Execute(ctx context.Context) (interface{}, error) {
	block, err := blocks.NewRandomBlock(t.Size)
	if err != nil {
		return nil, fmt.Errorf("randomizer generation failed for %s block %d: %w", t.BlockType, t.Index, err)
	}
	return block, nil
}

// CombinedStorageTask stores block and returns CID string (convenience for CLI)
type CombinedStorageTask struct {
	Index int
	Block *blocks.Block
	Client interface {
		StoreBlockWithCache(block *blocks.Block) (string, error)
	}
}

func (t *CombinedStorageTask) ID() string {
	return fmt.Sprintf("combined-store-%d", t.Index)
}

func (t *CombinedStorageTask) Execute(ctx context.Context) (interface{}, error) {
	cid, err := t.Client.StoreBlockWithCache(t.Block)
	if err != nil {
		return nil, fmt.Errorf("combined storage operation failed for block %d: %w", t.Index, err)
	}
	return cid, nil
}

// BlockOperationBatch provides utilities for batching block operations
type BlockOperationBatch struct {
	pool *Pool
}

// NewBlockOperationBatch creates a new batch processor for block operations
func NewBlockOperationBatch(pool *Pool) *BlockOperationBatch {
	return &BlockOperationBatch{pool: pool}
}

// ParallelXOR performs XOR operations on multiple blocks in parallel
func (b *BlockOperationBatch) ParallelXOR(ctx context.Context, dataBlocks, randomizer1Blocks, randomizer2Blocks []*blocks.Block) ([]*blocks.Block, error) {
	if len(dataBlocks) != len(randomizer1Blocks) || len(dataBlocks) != len(randomizer2Blocks) {
		return nil, fmt.Errorf("block arrays must have the same length")
	}
	
	// Create tasks
	tasks := make([]Task, len(dataBlocks))
	for i := range dataBlocks {
		tasks[i] = &XORTask{
			Index:       i,
			DataBlock:   dataBlocks[i],
			Randomizer1: randomizer1Blocks[i],
			Randomizer2: randomizer2Blocks[i],
		}
	}
	
	// Execute tasks
	results, err := b.pool.ExecuteAll(ctx, tasks)
	if err != nil {
		return nil, fmt.Errorf("parallel XOR execution failed: %w", err)
	}
	
	// Extract results
	xorBlocks := make([]*blocks.Block, len(results))
	for i, result := range results {
		if result.Error != nil {
			return nil, fmt.Errorf("XOR task %d failed: %w", i, result.Error)
		}
		block, ok := result.Value.(*blocks.Block)
		if !ok {
			return nil, fmt.Errorf("unexpected result type for XOR task %d", i)
		}
		xorBlocks[i] = block
	}
	
	return xorBlocks, nil
}

// ParallelStorage stores multiple blocks in parallel
func (b *BlockOperationBatch) ParallelStorage(ctx context.Context, blocks []*blocks.Block, client interface {
	StoreBlockWithCache(block *blocks.Block) (string, error)
}) ([]string, error) {
	// Create tasks
	tasks := make([]Task, len(blocks))
	for i, block := range blocks {
		tasks[i] = &CombinedStorageTask{
			Index:  i,
			Block:  block,
			Client: client,
		}
	}
	
	// Execute tasks
	results, err := b.pool.ExecuteAll(ctx, tasks)
	if err != nil {
		return nil, fmt.Errorf("parallel storage execution failed: %w", err)
	}
	
	// Extract results
	cids := make([]string, len(results))
	for i, result := range results {
		if result.Error != nil {
			return nil, fmt.Errorf("storage task %d failed: %w", i, result.Error)
		}
		cid, ok := result.Value.(string)
		if !ok {
			return nil, fmt.Errorf("unexpected result type for storage task %d", i)
		}
		cids[i] = cid
	}
	
	return cids, nil
}

// ParallelRetrieval retrieves multiple blocks in parallel
func (b *BlockOperationBatch) ParallelRetrieval(ctx context.Context, addresses []*storage.BlockAddress, storageManager interface {
	Get(ctx context.Context, address *storage.BlockAddress) (*blocks.Block, error)
}) ([]*blocks.Block, error) {
	// Create tasks
	tasks := make([]Task, len(addresses))
	for i, address := range addresses {
		tasks[i] = &RetrievalTask{
			Index:          i,
			Address:        address,
			StorageManager: storageManager,
		}
	}
	
	// Execute tasks
	results, err := b.pool.ExecuteAll(ctx, tasks)
	if err != nil {
		return nil, fmt.Errorf("parallel retrieval execution failed: %w", err)
	}
	
	// Extract results
	retrievedBlocks := make([]*blocks.Block, len(results))
	for i, result := range results {
		if result.Error != nil {
			return nil, fmt.Errorf("retrieval task %d failed: %w", i, result.Error)
		}
		block, ok := result.Value.(*blocks.Block)
		if !ok {
			return nil, fmt.Errorf("unexpected result type for retrieval task %d", i)
		}
		retrievedBlocks[i] = block
	}
	
	return retrievedBlocks, nil
}

// ParallelRandomizerGeneration generates randomizer blocks in parallel
func (b *BlockOperationBatch) ParallelRandomizerGeneration(ctx context.Context, count, size int, blockType string) ([]*blocks.Block, error) {
	// Create tasks
	tasks := make([]Task, count)
	for i := 0; i < count; i++ {
		tasks[i] = &RandomizerGenerationTask{
			Index:     i,
			Size:      size,
			BlockType: blockType,
		}
	}
	
	// Execute tasks
	results, err := b.pool.ExecuteAll(ctx, tasks)
	if err != nil {
		return nil, fmt.Errorf("parallel randomizer generation failed: %w", err)
	}
	
	// Extract results
	randomizers := make([]*blocks.Block, len(results))
	for i, result := range results {
		if result.Error != nil {
			return nil, fmt.Errorf("randomizer generation task %d failed: %w", i, result.Error)
		}
		block, ok := result.Value.(*blocks.Block)
		if !ok {
			return nil, fmt.Errorf("unexpected result type for randomizer generation task %d", i)
		}
		randomizers[i] = block
	}
	
	return randomizers, nil
}