package util

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// ProgressBar provides a simple terminal progress bar
type ProgressBar struct {
	mu       sync.Mutex
	total    int64
	current  int64
	start    time.Time
	prefix   string
	width    int
	writer   io.Writer
	lastDraw time.Time
}

// NewProgressBar creates a new progress bar
func NewProgressBar(total int64, prefix string, writer io.Writer) *ProgressBar {
	return &ProgressBar{
		total:  total,
		prefix: prefix,
		width:  40,
		writer: writer,
		start:  time.Now(),
	}
}

// Add increments the progress
func (p *ProgressBar) Add(n int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	p.current += n
	if p.current > p.total {
		p.current = p.total
	}
	
	// Throttle updates to avoid excessive redraws
	if time.Since(p.lastDraw) < 100*time.Millisecond && p.current < p.total {
		return
	}
	
	p.draw()
	p.lastDraw = time.Now()
}

// SetCurrent sets the current progress value
func (p *ProgressBar) SetCurrent(current int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	p.current = current
	if p.current > p.total {
		p.current = p.total
	}
	
	p.draw()
}

// SetTotal sets the total progress value
func (p *ProgressBar) SetTotal(total int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	p.total = total
	p.draw()
}

// SetDescription sets the progress bar description
func (p *ProgressBar) SetDescription(description string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	p.prefix = description
	p.draw()
}

// Finish completes the progress bar
func (p *ProgressBar) Finish() {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	p.current = p.total
	p.draw()
	fmt.Fprintln(p.writer) // New line after completion
}

// draw renders the progress bar
func (p *ProgressBar) draw() {
	if p.total <= 0 {
		return
	}
	
	// Calculate percentage
	percent := float64(p.current) / float64(p.total) * 100
	
	// Calculate filled width
	filled := int(float64(p.width) * float64(p.current) / float64(p.total))
	if filled > p.width {
		filled = p.width
	}
	
	// Build the bar
	bar := strings.Repeat("█", filled) + strings.Repeat("░", p.width-filled)
	
	// Calculate speed
	elapsed := time.Since(p.start)
	speed := ""
	if elapsed > 0 && p.current > 0 {
		bytesPerSec := float64(p.current) / elapsed.Seconds()
		speed = fmt.Sprintf(" %s/s", FormatBytes(int64(bytesPerSec)))
	}
	
	// Calculate ETA
	eta := ""
	if p.current > 0 && p.current < p.total {
		remainingBytes := p.total - p.current
		bytesPerSec := float64(p.current) / elapsed.Seconds()
		if bytesPerSec > 0 {
			remainingSecs := float64(remainingBytes) / bytesPerSec
			eta = fmt.Sprintf(" ETA: %s", FormatDuration(time.Duration(remainingSecs)*time.Second))
		}
	}
	
	// Print the progress bar
	fmt.Fprintf(p.writer, "\r%s [%s] %.1f%% %s/%s%s%s",
		p.prefix,
		bar,
		percent,
		FormatBytes(p.current),
		FormatBytes(p.total),
		speed,
		eta,
	)
}

// FormatBytes converts bytes to human-readable format
func FormatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	
	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// FormatDuration formats a duration to a human-readable string
func FormatDuration(d time.Duration) string {
	if d < time.Second {
		return "< 1s"
	}
	
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	
	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// ProgressReader wraps an io.Reader to track progress
type ProgressReader struct {
	reader io.Reader
	bar    *ProgressBar
}

// NewProgressReader creates a new progress tracking reader
func NewProgressReader(r io.Reader, total int64, prefix string) *ProgressReader {
	return &ProgressReader{
		reader: r,
		bar:    NewProgressBar(total, prefix, os.Stdout),
	}
}

// Read implements io.Reader and updates progress
func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	if n > 0 {
		pr.bar.Add(int64(n))
	}
	if err == io.EOF {
		pr.bar.Finish()
	}
	return n, err
}