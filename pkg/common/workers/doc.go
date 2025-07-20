// Package workers provides two worker pool implementations for parallel task execution.
//
// This package offers two distinct approaches to parallel processing, each optimized
// for different use cases within the NoiseFS system:
//
// # Pool vs SimpleWorkerPool - Decision Matrix
//
// Use Pool when you need:
//   - Progress tracking and statistics
//   - Complex task workflows with dependencies
//   - Graceful shutdown with timeout control
//   - Result ordering and task tracking by ID
//   - Advanced lifecycle management (start/stop)
//   - Debugging and monitoring capabilities
//
// Use SimpleWorkerPool when you need:
//   - Maximum performance for simple operations
//   - Minimal memory overhead
//   - Straightforward parallel processing
//   - Domain-specific operations (XOR, storage, retrieval)
//   - Block processing in NoiseFS core operations
//
// # Implementation Comparison
//
// ## Pool (Advanced)
//
// Pool provides a full-featured task queue system with the following characteristics:
//
//   - Task Interface: Requires implementing Task interface with ID() and Execute()
//   - Lifecycle Management: Explicit Start() and Shutdown() methods
//   - Progress Tracking: Optional ProgressReporter callback for completion updates
//   - Statistics: Real-time metrics (submitted, completed, failed, pending)
//   - Result Management: Ordered results with error handling and timing data
//   - Memory Usage: Higher due to result buffering and progress tracking
//   - API Complexity: More complex but feature-rich
//   - Performance: Slight overhead from task abstraction and result processing
//
// ## SimpleWorkerPool (Optimized)
//
// SimpleWorkerPool provides a semaphore-based concurrency model optimized for NoiseFS:
//
//   - Direct Operations: Specialized methods for XOR, storage, retrieval operations
//   - No Lifecycle: Ready to use immediately after creation
//   - No Progress Tracking: Focused on execution efficiency
//   - No Statistics: Minimal overhead for maximum performance
//   - Error Handling: Simple error aggregation and immediate failure reporting
//   - Memory Usage: Minimal - only goroutines and semaphore channels
//   - API Complexity: Simple, domain-specific methods
//   - Performance: Optimized for NoiseFS block operations
//
// # Performance Characteristics
//
// ## Pool Performance
//
//   - Task Creation Overhead: ~100-200ns per task
//   - Result Processing: Additional 50-100ns per result
//   - Memory Per Task: ~200-400 bytes (task metadata, result storage)
//   - Throughput: Excellent for complex workflows, good for simple tasks
//   - Latency: Slight increase due to task abstraction layer
//
// ## SimpleWorkerPool Performance
//
//   - Direct Function Calls: No abstraction overhead
//   - Memory Per Operation: ~50-100 bytes (goroutine stack, semaphore)
//   - Throughput: Optimized for high-volume block operations
//   - Latency: Minimal - direct execution path
//
// # Use Case Examples
//
// ## When to Use Pool
//
//	// Complex file processing with progress tracking
//	config := workers.Config{
//		WorkerCount: 8,
//		ProgressReporter: func(completed, total int64) {
//			fmt.Printf("Progress: %d/%d (%.1f%%)\n", 
//				completed, total, float64(completed)/float64(total)*100)
//		},
//	}
//	pool := workers.NewPool(config)
//	pool.Start()
//	defer pool.Shutdown()
//
//	// Execute heterogeneous tasks with result tracking
//	tasks := []workers.Task{
//		&CustomProcessingTask{...},
//		&ValidationTask{...},
//		&CompressionTask{...},
//	}
//	results, err := pool.ExecuteAll(ctx, tasks)
//
// ## When to Use SimpleWorkerPool
//
//	// High-performance NoiseFS block operations
//	pool := workers.NewSimpleWorkerPool(runtime.NumCPU())
//	
//	// Parallel XOR operations for anonymization
//	anonymizedBlocks, err := pool.ParallelXOR(ctx, 
//		dataBlocks, randomizer1Blocks, randomizer2Blocks)
//	
//	// Parallel block storage
//	cids, err := pool.ParallelStorage(ctx, blocks, client)
//	
//	// Parallel block retrieval
//	blocks, err := pool.ParallelRetrieval(ctx, addresses, storage)
//
// # Migration Guide
//
// ## From SimpleWorkerPool to Pool
//
// If you need to migrate from SimpleWorkerPool to Pool for additional features:
//
//	// Before: SimpleWorkerPool
//	simple := workers.NewSimpleWorkerPool(8)
//	results, err := simple.ParallelXOR(ctx, data, r1, r2)
//	
//	// After: Pool with BlockOperationBatch
//	pool := workers.NewPool(workers.Config{WorkerCount: 8})
//	pool.Start()
//	defer pool.Shutdown()
//	
//	batch := workers.NewBlockOperationBatch(pool)
//	results, err := batch.ParallelXOR(ctx, data, r1, r2)
//
// ## From Pool to SimpleWorkerPool
//
// If you need to optimize performance by switching to SimpleWorkerPool:
//
//	// Before: Pool with task abstraction
//	pool := workers.NewPool(workers.Config{WorkerCount: 8})
//	tasks := make([]workers.Task, len(blocks))
//	// ... create XORTask instances ...
//	results, err := pool.ExecuteAll(ctx, tasks)
//	
//	// After: SimpleWorkerPool direct operation
//	simple := workers.NewSimpleWorkerPool(8)
//	results, err := simple.ParallelXOR(ctx, data, r1, r2)
//
// # Concurrency Considerations
//
// ## Pool Concurrency
//
//   - Thread Safety: Fully thread-safe for all operations
//   - Goroutine Model: Fixed worker pool + result processor
//   - Resource Management: Controlled via Start()/Shutdown() lifecycle
//   - Context Handling: Supports cancellation and timeouts
//   - Backpressure: Configurable buffer sizes prevent memory exhaustion
//
// ## SimpleWorkerPool Concurrency
//
//   - Thread Safety: Safe for concurrent method calls
//   - Goroutine Model: Semaphore-controlled dynamic goroutines
//   - Resource Management: Automatic cleanup when operations complete
//   - Context Handling: Supports cancellation via context
//   - Backpressure: Semaphore naturally limits concurrent operations
//
// # Resource Management
//
// ## Pool Resource Management
//
//	// Always pair Start() with Shutdown()
//	pool := workers.NewPool(config)
//	if err := pool.Start(); err != nil {
//		return err
//	}
//	defer func() {
//		if err := pool.Shutdown(); err != nil {
//			log.Printf("Pool shutdown error: %v", err)
//		}
//	}()
//
// ## SimpleWorkerPool Resource Management
//
//	// No explicit cleanup needed - automatically managed
//	pool := workers.NewSimpleWorkerPool(runtime.NumCPU())
//	// Use pool directly - resources cleaned up automatically
//	results, err := pool.ParallelXOR(ctx, data, r1, r2)
//
// # Recommended Patterns
//
// For NoiseFS core operations (block processing, anonymization):
//   - Use SimpleWorkerPool for maximum performance
//   - Prefer domain-specific methods (ParallelXOR, ParallelStorage)
//   - Use context cancellation for timeout control
//
// For application-level operations (file processing, user workflows):
//   - Use Pool for comprehensive task management
//   - Implement custom Task types for specific operations
//   - Leverage progress reporting for user feedback
//   - Use statistics for performance monitoring
//
// For hybrid scenarios:
//   - Use Pool with BlockOperationBatch for NoiseFS operations with tracking
//   - Combine both pools for different operation types within same application
//
package workers