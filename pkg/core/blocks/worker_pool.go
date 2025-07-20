package blocks

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// WorkerPoolOptimizer provides enhanced worker pool management for directory processing
type WorkerPoolOptimizer struct {
	// Configuration
	minWorkers      int           // Minimum number of workers
	maxWorkers      int           // Maximum number of workers
	adaptiveScaling bool          // Enable dynamic worker scaling
	workQueue       chan WorkItem // Buffered work queue
	
	// State tracking
	activeWorkers   int64         // Current number of active workers
	queuedTasks     int64         // Number of queued tasks
	completedTasks  int64         // Number of completed tasks
	averageTaskTime time.Duration // Moving average of task execution time
	
	// Synchronization
	mu              sync.RWMutex  // Protects internal state
	wg              sync.WaitGroup // Wait group for worker coordination
	ctx             context.Context
	cancel          context.CancelFunc
	
	// Metrics
	startTime       time.Time     // Pool start time for metrics
	taskTimes       []time.Duration // Recent task execution times
	maxTaskHistory  int           // Maximum number of task times to track
}

// WorkItem represents a unit of work to be processed by the worker pool
type WorkItem struct {
	ID          string                                         // Unique identifier for the work item
	Task        func(ctx context.Context) error                // The actual work function
	Priority    int                                           // Priority level (higher = more important)
	Callback    func(result WorkResult)                       // Completion callback
	StartTime   time.Time                                     // When the work was queued
}

// WorkResult contains the result of processing a work item
type WorkResult struct {
	ID          string        // Work item identifier
	Error       error         // Error if task failed
	Duration    time.Duration // Time taken to complete the task
	WorkerID    int           // ID of worker that processed the task
}

// WorkerPoolConfig holds configuration for the worker pool optimizer
type WorkerPoolConfig struct {
	MinWorkers      int           // Minimum workers (default: 2)
	MaxWorkers      int           // Maximum workers (default: runtime.NumCPU() * 2)
	AdaptiveScaling bool          // Enable adaptive scaling (default: true)
	QueueSize       int           // Work queue buffer size (default: 1000)
	TaskHistorySize int           // Number of recent tasks to track for metrics (default: 100)
}

// DefaultWorkerPoolConfig returns sensible defaults for worker pool configuration
func DefaultWorkerPoolConfig() *WorkerPoolConfig {
	return &WorkerPoolConfig{
		MinWorkers:      2,
		MaxWorkers:      runtime.NumCPU() * 2,
		AdaptiveScaling: true,
		QueueSize:       1000,
		TaskHistorySize: 100,
	}
}

// NewWorkerPoolOptimizer creates a new optimized worker pool
func NewWorkerPoolOptimizer(config *WorkerPoolConfig) *WorkerPoolOptimizer {
	if config == nil {
		config = DefaultWorkerPoolConfig()
	}
	
	// Validate and apply constraints
	if config.MinWorkers < 1 {
		config.MinWorkers = 1
	}
	if config.MaxWorkers < config.MinWorkers {
		config.MaxWorkers = config.MinWorkers
	}
	if config.QueueSize < 10 {
		config.QueueSize = 10
	}
	if config.TaskHistorySize < 10 {
		config.TaskHistorySize = 10
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	
	pool := &WorkerPoolOptimizer{
		minWorkers:      config.MinWorkers,
		maxWorkers:      config.MaxWorkers,
		adaptiveScaling: config.AdaptiveScaling,
		workQueue:       make(chan WorkItem, config.QueueSize),
		ctx:             ctx,
		cancel:          cancel,
		startTime:       time.Now(),
		taskTimes:       make([]time.Duration, 0, config.TaskHistorySize),
		maxTaskHistory:  config.TaskHistorySize,
	}
	
	// Start initial workers
	pool.scaleWorkers(config.MinWorkers)
	
	// Start adaptive scaling goroutine if enabled
	if config.AdaptiveScaling {
		go pool.adaptiveScalingLoop()
	}
	
	return pool
}

// SubmitWork submits a work item to the pool
func (wp *WorkerPoolOptimizer) SubmitWork(item WorkItem) error {
	if item.Task == nil {
		return errors.New("work item task cannot be nil")
	}
	
	item.StartTime = time.Now()
	atomic.AddInt64(&wp.queuedTasks, 1)
	
	select {
	case wp.workQueue <- item:
		return nil
	case <-wp.ctx.Done():
		return wp.ctx.Err()
	default:
		// Queue is full, could implement dropping strategy here
		return errors.New("work queue is full")
	}
}

// worker is the main worker goroutine that processes work items
func (wp *WorkerPoolOptimizer) worker(workerID int) {
	defer wp.wg.Done()
	
	for {
		select {
		case item := <-wp.workQueue:
			wp.processWorkItem(item, workerID)
		case <-wp.ctx.Done():
			return
		}
	}
}

// processWorkItem handles the execution of a single work item
func (wp *WorkerPoolOptimizer) processWorkItem(item WorkItem, workerID int) {
	start := time.Now()
	err := item.Task(wp.ctx)
	duration := time.Since(start)
	
	// Update metrics
	atomic.AddInt64(&wp.completedTasks, 1)
	atomic.AddInt64(&wp.queuedTasks, -1)
	
	wp.updateTaskMetrics(duration)
	
	// Call completion callback if provided
	if item.Callback != nil {
		result := WorkResult{
			ID:       item.ID,
			Error:    err,
			Duration: duration,
			WorkerID: workerID,
		}
		item.Callback(result)
	}
}

// updateTaskMetrics updates the moving average of task execution times
func (wp *WorkerPoolOptimizer) updateTaskMetrics(duration time.Duration) {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	
	// Add to task history
	wp.taskTimes = append(wp.taskTimes, duration)
	if len(wp.taskTimes) > wp.maxTaskHistory {
		wp.taskTimes = wp.taskTimes[1:] // Remove oldest
	}
	
	// Calculate moving average
	if len(wp.taskTimes) > 0 {
		var total time.Duration
		for _, t := range wp.taskTimes {
			total += t
		}
		wp.averageTaskTime = total / time.Duration(len(wp.taskTimes))
	}
}

// scaleWorkers adjusts the number of active workers
func (wp *WorkerPoolOptimizer) scaleWorkers(targetWorkers int) {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	
	current := int(atomic.LoadInt64(&wp.activeWorkers))
	
	if targetWorkers > current {
		// Scale up
		for i := current; i < targetWorkers && i < wp.maxWorkers; i++ {
			wp.wg.Add(1)
			atomic.AddInt64(&wp.activeWorkers, 1)
			go wp.worker(i)
		}
	} else if targetWorkers < current {
		// Scale down is more complex and handled by worker timeout
		// For now, we rely on adaptive scaling loop to manage this
	}
}

// adaptiveScalingLoop monitors pool metrics and adjusts worker count
func (wp *WorkerPoolOptimizer) adaptiveScalingLoop() {
	ticker := time.NewTicker(5 * time.Second) // Check every 5 seconds
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			wp.performAdaptiveScaling()
		case <-wp.ctx.Done():
			return
		}
	}
}

// performAdaptiveScaling implements the scaling logic
func (wp *WorkerPoolOptimizer) performAdaptiveScaling() {
	queued := atomic.LoadInt64(&wp.queuedTasks)
	active := atomic.LoadInt64(&wp.activeWorkers)
	
	// Simple scaling heuristics
	if queued > active*2 && active < int64(wp.maxWorkers) {
		// High queue backlog, scale up
		wp.scaleWorkers(int(active) + 1)
	} else if queued == 0 && active > int64(wp.minWorkers) {
		// No work, consider scaling down (implement with worker timeout)
		// For now, just track the condition
	}
}

// GetMetrics returns current pool performance metrics
func (wp *WorkerPoolOptimizer) GetMetrics() WorkerPoolMetrics {
	wp.mu.RLock()
	defer wp.mu.RUnlock()
	
	return WorkerPoolMetrics{
		ActiveWorkers:    int(atomic.LoadInt64(&wp.activeWorkers)),
		QueuedTasks:      int(atomic.LoadInt64(&wp.queuedTasks)),
		CompletedTasks:   int(atomic.LoadInt64(&wp.completedTasks)),
		AverageTaskTime:  wp.averageTaskTime,
		UpTime:          time.Since(wp.startTime),
		QueueUtilization: float64(atomic.LoadInt64(&wp.queuedTasks)) / float64(cap(wp.workQueue)) * 100,
	}
}

// WorkerPoolMetrics contains performance metrics for the worker pool
type WorkerPoolMetrics struct {
	ActiveWorkers    int           // Current number of active workers
	QueuedTasks      int           // Number of tasks waiting in queue
	CompletedTasks   int           // Total number of completed tasks
	AverageTaskTime  time.Duration // Average task execution time
	UpTime          time.Duration // Pool uptime
	QueueUtilization float64       // Queue utilization percentage
}

// Shutdown gracefully shuts down the worker pool
func (wp *WorkerPoolOptimizer) Shutdown(timeout time.Duration) error {
	// Signal shutdown
	wp.cancel()
	
	// Wait for workers to finish with timeout
	done := make(chan struct{})
	go func() {
		wp.wg.Wait()
		close(done)
	}()
	
	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		return errors.New("shutdown timeout exceeded")
	}
}

// EnhancedDirectoryProcessorConfig extends the original config with worker pool optimization
type EnhancedDirectoryProcessorConfig struct {
	ProcessorConfig
	WorkerPoolConfig *WorkerPoolConfig // Optional optimized worker pool config
}

// OptimizeDirectoryProcessor creates a directory processor with enhanced worker pool
func OptimizeDirectoryProcessor(config *EnhancedDirectoryProcessorConfig) (*DirectoryProcessor, *WorkerPoolOptimizer) {
	// Create base directory processor
	baseProcessor, err := NewDirectoryProcessor(&config.ProcessorConfig)
	if err != nil {
		return nil, nil
	}
	
	// Create optimized worker pool if config provided
	var workerPool *WorkerPoolOptimizer
	if config.WorkerPoolConfig != nil {
		workerPool = NewWorkerPoolOptimizer(config.WorkerPoolConfig)
	}
	
	return baseProcessor, workerPool
}