package workers

import (
	"context"
	"fmt"
	"sync"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
)

// SimpleWorkerPool provides lightweight parallel execution for block operations.
// Uses pure goroutines, trusting Go's excellent scheduler for optimal performance.
type SimpleWorkerPool struct {
	// No internal state needed - pure goroutines handle everything
}

// NewSimpleWorkerPool creates a simple worker pool.
// The workerCount parameter is ignored - Go's scheduler handles concurrency optimally.
func NewSimpleWorkerPool(workerCount int) *SimpleWorkerPool {
	return &SimpleWorkerPool{}
}

// ParallelXOR performs XOR operations on blocks in parallel
func (p *SimpleWorkerPool) ParallelXOR(ctx context.Context, dataBlocks, randomizer1Blocks, randomizer2Blocks []*blocks.Block) ([]*blocks.Block, error) {
	if len(dataBlocks) != len(randomizer1Blocks) || len(dataBlocks) != len(randomizer2Blocks) {
		return nil, fmt.Errorf("block arrays must have the same length")
	}
	
	results := make([]*blocks.Block, len(dataBlocks))
	errors := make([]error, len(dataBlocks))
	
	var wg sync.WaitGroup
	
	for i := range dataBlocks {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			
			// Check for cancellation
			select {
			case <-ctx.Done():
				errors[index] = ctx.Err()
				return
			default:
			}
			
			// Perform XOR operation
			result, err := dataBlocks[index].XOR(randomizer1Blocks[index], randomizer2Blocks[index])
			if err != nil {
				errors[index] = fmt.Errorf("XOR operation failed for block %d: %w", index, err)
				return
			}
			results[index] = result
		}(i)
	}
	
	wg.Wait()
	
	// Check for errors
	for i, err := range errors {
		if err != nil {
			return nil, fmt.Errorf("block %d: %w", i, err)
		}
	}
	
	return results, nil
}

// ParallelStorage stores blocks in parallel using a client interface
func (p *SimpleWorkerPool) ParallelStorage(ctx context.Context, blockList []*blocks.Block, client interface {
	StoreBlockWithCache(block *blocks.Block) (string, error)
}) ([]string, error) {
	results := make([]string, len(blockList))
	errors := make([]error, len(blockList))
	
	var wg sync.WaitGroup
	
	for i, block := range blockList {
		wg.Add(1)
		go func(index int, b *blocks.Block) {
			defer wg.Done()
			
			// Check for cancellation
			select {
			case <-ctx.Done():
				errors[index] = ctx.Err()
				return
			default:
			}
			
			// Store block
			cid, err := client.StoreBlockWithCache(b)
			if err != nil {
				errors[index] = fmt.Errorf("storage operation failed for block %d: %w", index, err)
				return
			}
			results[index] = cid
		}(i, block)
	}
	
	wg.Wait()
	
	// Check for errors
	for i, err := range errors {
		if err != nil {
			return nil, fmt.Errorf("block %d: %w", i, err)
		}
	}
	
	return results, nil
}

// ParallelRetrieval retrieves blocks in parallel using storage manager
func (p *SimpleWorkerPool) ParallelRetrieval(ctx context.Context, addresses []*storage.BlockAddress, storageManager interface {
	Get(ctx context.Context, address *storage.BlockAddress) (*blocks.Block, error)
}) ([]*blocks.Block, error) {
	results := make([]*blocks.Block, len(addresses))
	errors := make([]error, len(addresses))
	
	var wg sync.WaitGroup
	
	for i, address := range addresses {
		wg.Add(1)
		go func(index int, addr *storage.BlockAddress) {
			defer wg.Done()
			
			// Check for cancellation
			select {
			case <-ctx.Done():
				errors[index] = ctx.Err()
				return
			default:
			}
			
			// Retrieve block
			block, err := storageManager.Get(ctx, addr)
			if err != nil {
				errors[index] = fmt.Errorf("retrieval operation failed for block %d: %w", index, err)
				return
			}
			results[index] = block
		}(i, address)
	}
	
	wg.Wait()
	
	// Check for errors
	for i, err := range errors {
		if err != nil {
			return nil, fmt.Errorf("block %d: %w", i, err)
		}
	}
	
	return results, nil
}

// ParallelRandomizerGeneration generates randomizer blocks in parallel
func (p *SimpleWorkerPool) ParallelRandomizerGeneration(ctx context.Context, count, size int) ([]*blocks.Block, error) {
	results := make([]*blocks.Block, count)
	errors := make([]error, count)
	
	var wg sync.WaitGroup
	
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			
			// Check for cancellation
			select {
			case <-ctx.Done():
				errors[index] = ctx.Err()
				return
			default:
			}
			
			// Generate randomizer block
			block, err := blocks.NewRandomBlock(size)
			if err != nil {
				errors[index] = fmt.Errorf("randomizer generation failed for block %d: %w", index, err)
				return
			}
			results[index] = block
		}(i)
	}
	
	wg.Wait()
	
	// Check for errors
	for i, err := range errors {
		if err != nil {
			return nil, fmt.Errorf("block %d: %w", i, err)
		}
	}
	
	return results, nil
}