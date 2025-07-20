// Package streaming provides progress reporting implementations.
// This file contains concrete implementations of the ProgressReporter interface.
package streaming

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// ConsoleProgressReporter provides simple console-based progress reporting.
// Suitable for command-line applications and debugging.
type ConsoleProgressReporter struct {
	name        string
	lastUpdate  time.Time
	updateFreq  time.Duration
	mu          sync.Mutex
}

// NewConsoleProgressReporter creates a new console progress reporter.
func NewConsoleProgressReporter(name string) *ConsoleProgressReporter {
	return &ConsoleProgressReporter{
		name:       name,
		updateFreq: time.Second, // Update at most once per second
	}
}

// NewConsoleProgressReporterWithFreq creates a console progress reporter with custom update frequency.
func NewConsoleProgressReporterWithFreq(name string, updateFreq time.Duration) *ConsoleProgressReporter {
	return &ConsoleProgressReporter{
		name:       name,
		updateFreq: updateFreq,
	}
}

// ReportProgress implements ProgressReporter interface.
func (r *ConsoleProgressReporter) ReportProgress(info ProgressInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Throttle updates to avoid console spam
	if time.Since(r.lastUpdate) < r.updateFreq {
		return
	}
	r.lastUpdate = time.Now()

	var percentage float64
	if info.TotalBytes > 0 {
		percentage = float64(info.BytesProcessed) / float64(info.TotalBytes) * 100
	}

	elapsed := info.CurrentTime.Sub(info.StartTime)
	throughputMBps := info.Throughput / (1024 * 1024)

	fmt.Printf("[%s] %s: %.1f%% (%d/%d blocks, %.2f MB/s, %v elapsed)\n",
		r.name, info.Stage, percentage, info.BlocksProcessed, info.TotalBlocks,
		throughputMBps, elapsed.Truncate(time.Second))
}

// ReportError implements ProgressReporter interface.
func (r *ConsoleProgressReporter) ReportError(err error, context string) {
	fmt.Printf("[%s] ERROR in %s: %v\n", r.name, context, err)
}

// SetTotal implements ProgressReporter interface.
func (r *ConsoleProgressReporter) SetTotal(totalBytes int64, totalBlocks int) {
	fmt.Printf("[%s] Starting operation: %d bytes, %d blocks\n",
		r.name, totalBytes, totalBlocks)
}

// Complete implements ProgressReporter interface.
func (r *ConsoleProgressReporter) Complete(finalInfo ProgressInfo) {
	duration := finalInfo.CurrentTime.Sub(finalInfo.StartTime)
	throughputMBps := finalInfo.Throughput / (1024 * 1024)

	fmt.Printf("[%s] COMPLETE: %d bytes in %v (%.2f MB/s average)\n",
		r.name, finalInfo.BytesProcessed, duration.Truncate(time.Second), throughputMBps)
}

// Cancel implements ProgressReporter interface.
func (r *ConsoleProgressReporter) Cancel(reason string) {
	fmt.Printf("[%s] CANCELLED: %s\n", r.name, reason)
}

// LogProgressReporter provides logging-based progress reporting.
// Suitable for server applications and production environments.
type LogProgressReporter struct {
	name       string
	logger     *log.Logger
	lastUpdate time.Time
	updateFreq time.Duration
	mu         sync.Mutex
}

// NewLogProgressReporter creates a new log-based progress reporter.
func NewLogProgressReporter(name string, logger *log.Logger) *LogProgressReporter {
	if logger == nil {
		logger = log.Default()
	}
	return &LogProgressReporter{
		name:       name,
		logger:     logger,
		updateFreq: 5 * time.Second, // Update every 5 seconds for logs
	}
}

// ReportProgress implements ProgressReporter interface.
func (r *LogProgressReporter) ReportProgress(info ProgressInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Throttle updates for log-based reporting
	if time.Since(r.lastUpdate) < r.updateFreq {
		return
	}
	r.lastUpdate = time.Now()

	var percentage float64
	if info.TotalBytes > 0 {
		percentage = float64(info.BytesProcessed) / float64(info.TotalBytes) * 100
	}

	r.logger.Printf("streaming[%s]: %s - %.1f%% complete (%d blocks, %.2f MB/s)",
		r.name, info.Stage, percentage, info.BlocksProcessed, info.Throughput/(1024*1024))
}

// ReportError implements ProgressReporter interface.
func (r *LogProgressReporter) ReportError(err error, context string) {
	r.logger.Printf("streaming[%s]: ERROR in %s: %v", r.name, context, err)
}

// SetTotal implements ProgressReporter interface.
func (r *LogProgressReporter) SetTotal(totalBytes int64, totalBlocks int) {
	r.logger.Printf("streaming[%s]: Starting - %d bytes, %d blocks", r.name, totalBytes, totalBlocks)
}

// Complete implements ProgressReporter interface.
func (r *LogProgressReporter) Complete(finalInfo ProgressInfo) {
	duration := finalInfo.CurrentTime.Sub(finalInfo.StartTime)
	r.logger.Printf("streaming[%s]: COMPLETE - %d bytes in %v (%.2f MB/s)",
		r.name, finalInfo.BytesProcessed, duration, finalInfo.Throughput/(1024*1024))
}

// Cancel implements ProgressReporter interface.
func (r *LogProgressReporter) Cancel(reason string) {
	r.logger.Printf("streaming[%s]: CANCELLED - %s", r.name, reason)
}

// ChannelProgressReporter provides channel-based progress reporting.
// Suitable for applications that need to process progress updates asynchronously.
type ChannelProgressReporter struct {
	name     string
	updates  chan ProgressInfo
	errors   chan error
	complete chan ProgressInfo
	cancel   chan string
}

// NewChannelProgressReporter creates a new channel-based progress reporter.
func NewChannelProgressReporter(name string, bufferSize int) *ChannelProgressReporter {
	return &ChannelProgressReporter{
		name:     name,
		updates:  make(chan ProgressInfo, bufferSize),
		errors:   make(chan error, bufferSize),
		complete: make(chan ProgressInfo, 1),
		cancel:   make(chan string, 1),
	}
}

// ReportProgress implements ProgressReporter interface.
func (r *ChannelProgressReporter) ReportProgress(info ProgressInfo) {
	select {
	case r.updates <- info:
	default:
		// Don't block if channel is full
	}
}

// ReportError implements ProgressReporter interface.
func (r *ChannelProgressReporter) ReportError(err error, context string) {
	select {
	case r.errors <- fmt.Errorf("%s: %w", context, err):
	default:
		// Don't block if channel is full
	}
}

// SetTotal implements ProgressReporter interface.
func (r *ChannelProgressReporter) SetTotal(totalBytes int64, totalBlocks int) {
	// Send initial progress with totals set
	r.ReportProgress(ProgressInfo{
		Stage:       "Initialized",
		TotalBytes:  totalBytes,
		TotalBlocks: totalBlocks,
		StartTime:   time.Now(),
		CurrentTime: time.Now(),
	})
}

// Complete implements ProgressReporter interface.
func (r *ChannelProgressReporter) Complete(finalInfo ProgressInfo) {
	select {
	case r.complete <- finalInfo:
	default:
		// Don't block
	}
}

// Cancel implements ProgressReporter interface.
func (r *ChannelProgressReporter) Cancel(reason string) {
	select {
	case r.cancel <- reason:
	default:
		// Don't block
	}
}

// Updates returns the progress updates channel.
func (r *ChannelProgressReporter) Updates() <-chan ProgressInfo {
	return r.updates
}

// Errors returns the errors channel.
func (r *ChannelProgressReporter) Errors() <-chan error {
	return r.errors
}

// Completed returns the completion channel.
func (r *ChannelProgressReporter) Completed() <-chan ProgressInfo {
	return r.complete
}

// Cancelled returns the cancellation channel.
func (r *ChannelProgressReporter) Cancelled() <-chan string {
	return r.cancel
}

// Close closes all channels. Should be called when progress reporting is complete.
func (r *ChannelProgressReporter) Close() {
	close(r.updates)
	close(r.errors)
	close(r.complete)
	close(r.cancel)
}

// MultiProgressReporter broadcasts progress updates to multiple reporters.
// Useful for applications that need multiple forms of progress reporting.
type MultiProgressReporter struct {
	reporters []ProgressReporter
}

// NewMultiProgressReporter creates a new multi-reporter that broadcasts to all provided reporters.
func NewMultiProgressReporter(reporters ...ProgressReporter) *MultiProgressReporter {
	return &MultiProgressReporter{
		reporters: reporters,
	}
}

// AddReporter adds a new progress reporter to the multi-reporter.
func (m *MultiProgressReporter) AddReporter(reporter ProgressReporter) {
	m.reporters = append(m.reporters, reporter)
}

// ReportProgress implements ProgressReporter interface.
func (m *MultiProgressReporter) ReportProgress(info ProgressInfo) {
	for _, reporter := range m.reporters {
		if reporter != nil {
			reporter.ReportProgress(info)
		}
	}
}

// ReportError implements ProgressReporter interface.
func (m *MultiProgressReporter) ReportError(err error, context string) {
	for _, reporter := range m.reporters {
		if reporter != nil {
			reporter.ReportError(err, context)
		}
	}
}

// SetTotal implements ProgressReporter interface.
func (m *MultiProgressReporter) SetTotal(totalBytes int64, totalBlocks int) {
	for _, reporter := range m.reporters {
		if reporter != nil {
			reporter.SetTotal(totalBytes, totalBlocks)
		}
	}
}

// Complete implements ProgressReporter interface.
func (m *MultiProgressReporter) Complete(finalInfo ProgressInfo) {
	for _, reporter := range m.reporters {
		if reporter != nil {
			reporter.Complete(finalInfo)
		}
	}
}

// Cancel implements ProgressReporter interface.
func (m *MultiProgressReporter) Cancel(reason string) {
	for _, reporter := range m.reporters {
		if reporter != nil {
			reporter.Cancel(reason)
		}
	}
}

// NoOpProgressReporter provides a no-operation progress reporter.
// Useful for testing or when progress reporting is not needed.
type NoOpProgressReporter struct{}

// NewNoOpProgressReporter creates a new no-operation progress reporter.
func NewNoOpProgressReporter() *NoOpProgressReporter {
	return &NoOpProgressReporter{}
}

// ReportProgress implements ProgressReporter interface (no-op).
func (n *NoOpProgressReporter) ReportProgress(info ProgressInfo) {}

// ReportError implements ProgressReporter interface (no-op).
func (n *NoOpProgressReporter) ReportError(err error, context string) {}

// SetTotal implements ProgressReporter interface (no-op).
func (n *NoOpProgressReporter) SetTotal(totalBytes int64, totalBlocks int) {}

// Complete implements ProgressReporter interface (no-op).
func (n *NoOpProgressReporter) Complete(finalInfo ProgressInfo) {}

// Cancel implements ProgressReporter interface (no-op).
func (n *NoOpProgressReporter) Cancel(reason string) {}