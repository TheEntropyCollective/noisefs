package workers

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// Task represents a unit of work that can be executed by a worker
type Task interface {
	// Execute performs the task and returns a result or error
	Execute(ctx context.Context) (interface{}, error)
	
	// ID returns a unique identifier for this task (for progress tracking)
	ID() string
}

// Result holds the outcome of a task execution
type Result struct {
	TaskID string
	Value  interface{}
	Error  error
	Duration time.Duration
}

// ProgressReporter is called when a task completes
type ProgressReporter func(completed, total int64)

// Config holds configuration for the worker pool
type Config struct {
	// WorkerCount is the number of workers to spawn
	// If 0, defaults to runtime.NumCPU()
	WorkerCount int
	
	// BufferSize is the size of the task queue buffer
	// If 0, defaults to WorkerCount * 2
	BufferSize int
	
	// ShutdownTimeout is how long to wait for graceful shutdown
	ShutdownTimeout time.Duration
	
	// ProgressReporter is called when tasks complete (optional)
	ProgressReporter ProgressReporter
}

// Pool manages a pool of workers for parallel task execution
type Pool struct {
	config   Config
	tasks    chan Task
	results  chan Result
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	
	// Statistics
	submitted   int64
	completed   int64
	failed      int64
	
	// State
	mutex    sync.RWMutex
	started  bool
	shutdown bool
}

// NewPool creates a new worker pool with the given configuration
func NewPool(config Config) *Pool {
	// Set defaults
	if config.WorkerCount <= 0 {
		config.WorkerCount = runtime.NumCPU()
	}
	if config.BufferSize <= 0 {
		config.BufferSize = config.WorkerCount * 2
	}
	if config.ShutdownTimeout <= 0 {
		config.ShutdownTimeout = 30 * time.Second
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	
	return &Pool{
		config:  config,
		tasks:   make(chan Task, config.BufferSize),
		results: make(chan Result, config.BufferSize),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Start initializes and starts the worker pool
func (p *Pool) Start() error {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	
	if p.started {
		return fmt.Errorf("pool already started")
	}
	if p.shutdown {
		return fmt.Errorf("pool has been shutdown")
	}
	
	// Start workers
	for i := 0; i < p.config.WorkerCount; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}
	
	// Start result processor
	p.wg.Add(1)
	go p.resultProcessor()
	
	p.started = true
	return nil
}

// Submit adds a task to the worker pool for execution
func (p *Pool) Submit(task Task) error {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	
	if !p.started {
		return fmt.Errorf("pool not started")
	}
	if p.shutdown {
		return fmt.Errorf("pool is shutting down")
	}
	
	select {
	case p.tasks <- task:
		atomic.AddInt64(&p.submitted, 1)
		return nil
	case <-p.ctx.Done():
		return fmt.Errorf("pool context cancelled")
	default:
		return fmt.Errorf("task queue full")
	}
}

// SubmitBlocking adds a task to the worker pool, blocking if the queue is full
func (p *Pool) SubmitBlocking(ctx context.Context, task Task) error {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	
	if !p.started {
		return fmt.Errorf("pool not started")
	}
	if p.shutdown {
		return fmt.Errorf("pool is shutting down")
	}
	
	select {
	case p.tasks <- task:
		atomic.AddInt64(&p.submitted, 1)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-p.ctx.Done():
		return fmt.Errorf("pool context cancelled")
	}
}

// ExecuteAll executes all tasks and returns results in order
// This is a convenience method for batch processing
func (p *Pool) ExecuteAll(ctx context.Context, tasks []Task) ([]*Result, error) {
	if len(tasks) == 0 {
		return []*Result{}, nil
	}
	
	// Submit all tasks
	for _, task := range tasks {
		if err := p.SubmitBlocking(ctx, task); err != nil {
			return nil, fmt.Errorf("failed to submit task %s: %w", task.ID(), err)
		}
	}
	
	// Collect results
	results := make([]*Result, len(tasks))
	resultMap := make(map[string]*Result)
	
	for i := 0; i < len(tasks); i++ {
		select {
		case result := <-p.results:
			resultMap[result.TaskID] = &result
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-p.ctx.Done():
			return nil, fmt.Errorf("pool context cancelled")
		}
	}
	
	// Order results to match input order
	for i, task := range tasks {
		result, exists := resultMap[task.ID()]
		if !exists {
			return nil, fmt.Errorf("missing result for task %s", task.ID())
		}
		results[i] = result
	}
	
	return results, nil
}

// Shutdown gracefully shuts down the worker pool
func (p *Pool) Shutdown() error {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	
	if p.shutdown {
		return nil // Already shutdown
	}
	if !p.started {
		return fmt.Errorf("pool not started")
	}
	
	p.shutdown = true
	
	// Close task channel to signal workers to stop
	close(p.tasks)
	
	// Wait for workers to finish with timeout
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()
	
	select {
	case <-done:
		// Graceful shutdown completed
	case <-time.After(p.config.ShutdownTimeout):
		// Force shutdown by cancelling context
		p.cancel()
		p.wg.Wait()
	}
	
	// Close results channel
	close(p.results)
	
	return nil
}

// Stats returns current pool statistics
func (p *Pool) Stats() PoolStats {
	return PoolStats{
		WorkerCount: p.config.WorkerCount,
		Submitted:   atomic.LoadInt64(&p.submitted),
		Completed:   atomic.LoadInt64(&p.completed),
		Failed:      atomic.LoadInt64(&p.failed),
		Pending:     len(p.tasks),
	}
}

// PoolStats holds statistics about pool performance
type PoolStats struct {
	WorkerCount int
	Submitted   int64
	Completed   int64
	Failed      int64
	Pending     int
}

// worker is the main worker goroutine
func (p *Pool) worker(id int) {
	defer p.wg.Done()
	
	for task := range p.tasks {
		start := time.Now()
		
		// Execute task with pool context
		value, err := task.Execute(p.ctx)
		
		result := Result{
			TaskID:   task.ID(),
			Value:    value,
			Error:    err,
			Duration: time.Since(start),
		}
		
		// Update statistics
		if err != nil {
			atomic.AddInt64(&p.failed, 1)
		}
		atomic.AddInt64(&p.completed, 1)
		
		// Send result (blocking to ensure delivery)
		select {
		case p.results <- result:
			// Result sent successfully
		case <-p.ctx.Done():
			// Pool is shutting down
			return
		}
	}
}

// resultProcessor handles result reporting and progress tracking
func (p *Pool) resultProcessor() {
	defer p.wg.Done()
	
	// Use a ticker for progress reporting instead of blocking on results
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			// Report progress if configured
			if p.config.ProgressReporter != nil {
				completed := atomic.LoadInt64(&p.completed)
				total := atomic.LoadInt64(&p.submitted)
				p.config.ProgressReporter(completed, total)
			}
		case <-p.ctx.Done():
			return
		}
	}
}