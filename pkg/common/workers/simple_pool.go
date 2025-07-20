package workers

import (
	"context"
	"fmt"
	"runtime"
	"sync"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
)

// SimpleWorkerPool provides a simplified worker pool for block operations
// This focuses on the core functionality needed for Sprint 1
type SimpleWorkerPool struct {
	workerCount int
}

// NewSimpleWorkerPool creates a simple worker pool
func NewSimpleWorkerPool(workerCount int) *SimpleWorkerPool {
	if workerCount <= 0 {
		workerCount = runtime.NumCPU()
	}
	return &SimpleWorkerPool{
		workerCount: workerCount,
	}
}

// ParallelXOR performs XOR operations on blocks in parallel
func (p *SimpleWorkerPool) ParallelXOR(ctx context.Context, dataBlocks, randomizer1Blocks, randomizer2Blocks []*blocks.Block) ([]*blocks.Block, error) {
	if len(dataBlocks) != len(randomizer1Blocks) || len(dataBlocks) != len(randomizer2Blocks) {
		return nil, fmt.Errorf("block arrays must have the same length")
	}
	
	results := make([]*blocks.Block, len(dataBlocks))
	errors := make([]error, len(dataBlocks))
	
	// Use a semaphore to limit concurrency
	semaphore := make(chan struct{}, p.workerCount)
	var wg sync.WaitGroup
	
	for i := range dataBlocks {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			
			// Acquire semaphore
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				errors[index] = ctx.Err()
				return
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
	
	// Use a semaphore to limit concurrency
	semaphore := make(chan struct{}, p.workerCount)
	var wg sync.WaitGroup
	
	for i, block := range blockList {
		wg.Add(1)
		go func(index int, b *blocks.Block) {
			defer wg.Done()
			
			// Acquire semaphore
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				errors[index] = ctx.Err()
				return
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
	
	// Use a semaphore to limit concurrency
	semaphore := make(chan struct{}, p.workerCount)
	var wg sync.WaitGroup
	
	for i, address := range addresses {
		wg.Add(1)
		go func(index int, addr *storage.BlockAddress) {
			defer wg.Done()
			
			// Acquire semaphore
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				errors[index] = ctx.Err()
				return
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
	
	// Use a semaphore to limit concurrency
	semaphore := make(chan struct{}, p.workerCount)
	var wg sync.WaitGroup
	
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			
			// Acquire semaphore
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				errors[index] = ctx.Err()
				return
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