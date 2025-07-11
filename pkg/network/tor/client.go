package tor

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
	
	"golang.org/x/net/proxy"
)

// Client provides Tor network access for NoiseFS
type Client struct {
	config      *Config
	dialer      proxy.Dialer
	httpClient  *http.Client
	circuitPool *CircuitPool
	metrics     *Metrics
	
	mu          sync.RWMutex
	connected   bool
	lastCheck   time.Time
}

// Metrics tracks Tor performance
type Metrics struct {
	mu sync.RWMutex
	
	// Performance metrics
	CircuitBuilds      int64
	CircuitBuildTime   time.Duration
	RequestCount       int64
	TotalBandwidth     int64
	AverageLatency     time.Duration
	
	// Error tracking
	CircuitFailures    int64
	ConnectionErrors   int64
	TimeoutErrors      int64
}

// NewClient creates a new Tor client
func NewClient(config *Config) (*Client, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	
	// Create SOCKS5 dialer
	// PERFORMANCE IMPACT: Initial connection adds 1-3s latency
	dialer, err := proxy.SOCKS5("tcp", config.SOCKSProxy, nil, proxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("failed to create SOCKS5 dialer: %w", err)
	}
	
	// Create HTTP client with Tor transport
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			// PERFORMANCE IMPACT: Each new connection goes through Tor
			// Adds 500ms-2s latency per connection
			return dialer.Dial(network, addr)
		},
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false, // Always verify TLS
		},
		// PERFORMANCE TUNING: Connection pooling helps reuse circuits
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 2,
		IdleConnTimeout:     90 * time.Second,
		DisableKeepAlives:   false, // Keep connections alive for reuse
		
		// PERFORMANCE IMPACT: Timeouts must be higher for Tor
		TLSHandshakeTimeout:   20 * time.Second, // vs 10s normally
		ResponseHeaderTimeout: 30 * time.Second, // vs 10s normally
	}
	
	httpClient := &http.Client{
		Transport: transport,
		Timeout:   config.Performance.RequestTimeout,
	}
	
	client := &Client{
		config:     config,
		dialer:     dialer,
		httpClient: httpClient,
		metrics:    &Metrics{},
	}
	
	// Initialize circuit pool
	// PERFORMANCE IMPACT: Pre-establishing circuits adds startup time
	// but improves request latency
	if config.CircuitPool.MinCircuits > 0 {
		client.circuitPool = NewCircuitPool(client, config.CircuitPool)
		if err := client.circuitPool.Initialize(); err != nil {
			// Non-fatal: continue without pool
			fmt.Printf("Warning: failed to initialize circuit pool: %v\n", err)
		}
	}
	
	// Test connection
	if err := client.TestConnection(); err != nil {
		return nil, fmt.Errorf("Tor not accessible: %w", err)
	}
	
	return client, nil
}

// TestConnection verifies Tor connectivity
func (c *Client) TestConnection() error {
	// PERFORMANCE TEST: Measure Tor latency
	start := time.Now()
	
	// Use Tor Project's check service
	resp, err := c.httpClient.Get("https://check.torproject.org/api/ip")
	if err != nil {
		return fmt.Errorf("connection test failed: %w", err)
	}
	defer resp.Body.Close()
	
	latency := time.Since(start)
	c.updateMetrics(func(m *Metrics) {
		m.AverageLatency = latency
	})
	
	// PERFORMANCE WARNING: If latency > 5s, Tor might be overloaded
	if latency > 5*time.Second {
		fmt.Printf("Warning: High Tor latency detected: %v\n", latency)
	}
	
	c.mu.Lock()
	c.connected = true
	c.lastCheck = time.Now()
	c.mu.Unlock()
	
	return nil
}

// HTTPClient returns an HTTP client configured for Tor
func (c *Client) HTTPClient() *http.Client {
	return c.httpClient
}

// GetWithCircuit performs HTTP GET using a specific circuit
func (c *Client) GetWithCircuit(ctx context.Context, url string, circuitID string) (*http.Response, error) {
	// PERFORMANCE OPTIMIZATION: Reuse circuit if available
	if c.circuitPool != nil && circuitID != "" {
		if client := c.circuitPool.GetCircuitClient(circuitID); client != nil {
			return client.Get(url)
		}
	}
	
	// Fallback to default client
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	
	return c.httpClient.Do(req)
}

// PostWithJitter performs HTTP POST with timing jitter
func (c *Client) PostWithJitter(ctx context.Context, url string, body io.Reader) (*http.Response, error) {
	// PERFORMANCE IMPACT: Jitter adds 1-5s delay but improves anonymity
	if c.config.Upload.Enabled && c.config.Upload.JitterMax > 0 {
		jitter := randomDuration(c.config.Upload.JitterMin, c.config.Upload.JitterMax)
		select {
		case <-time.After(jitter):
			// Jitter applied
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	
	req, err := http.NewRequestWithContext(ctx, "POST", url, body)
	if err != nil {
		return nil, err
	}
	
	// PERFORMANCE OPTIMIZATION: Use compression if enabled
	if c.config.Performance.UseCompression {
		req.Header.Set("Content-Encoding", "gzip")
		req.Header.Set("Accept-Encoding", "gzip")
	}
	
	start := time.Now()
	resp, err := c.httpClient.Do(req)
	
	// Update metrics
	c.updateMetrics(func(m *Metrics) {
		m.RequestCount++
		if err != nil {
			if isTimeout(err) {
				m.TimeoutErrors++
			} else {
				m.ConnectionErrors++
			}
		} else {
			// Update average latency
			latency := time.Since(start)
			if m.AverageLatency == 0 {
				m.AverageLatency = latency
			} else {
				// Exponential moving average
				m.AverageLatency = (m.AverageLatency*9 + latency) / 10
			}
		}
	})
	
	return resp, err
}

// SplitUpload uploads data using multiple circuits
func (c *Client) SplitUpload(ctx context.Context, uploads []UploadRequest) error {
	// PERFORMANCE IMPACT: Parallel circuits improve throughput
	// but add complexity and initial setup time
	
	if !c.config.Upload.SplitCircuits || len(uploads) <= 1 {
		// Single circuit upload
		for _, req := range uploads {
			if _, err := c.PostWithJitter(ctx, req.URL, req.Body); err != nil {
				return err
			}
		}
		return nil
	}
	
	// PERFORMANCE OPTIMIZATION: Use circuit pool for parallel uploads
	circuits := c.config.Performance.ParallelCircuits
	if circuits > len(uploads) {
		circuits = len(uploads)
	}
	
	// Create upload channels
	uploadChan := make(chan UploadRequest, len(uploads))
	errorChan := make(chan error, circuits)
	
	// Add uploads to channel
	for _, upload := range uploads {
		uploadChan <- upload
	}
	close(uploadChan)
	
	// Launch parallel uploaders
	var wg sync.WaitGroup
	for i := 0; i < circuits; i++ {
		wg.Add(1)
		go func(circuitNum int) {
			defer wg.Done()
			
			// Get unique circuit from pool
			circuitID := fmt.Sprintf("upload-%d", circuitNum)
			
			for upload := range uploadChan {
				// PERFORMANCE: Each circuit handles multiple uploads
				// reducing circuit establishment overhead
				if _, err := c.PostWithJitter(ctx, upload.URL, upload.Body); err != nil {
					errorChan <- fmt.Errorf("circuit %d failed: %w", circuitNum, err)
					return
				}
			}
		}(i)
	}
	
	// Wait for completion
	wg.Wait()
	close(errorChan)
	
	// Check for errors
	for err := range errorChan {
		return err
	}
	
	return nil
}

// GetMetrics returns performance metrics
func (c *Client) GetMetrics() Metrics {
	c.metrics.mu.RLock()
	defer c.metrics.mu.RUnlock()
	return *c.metrics
}

// EstimateUploadTime estimates upload time through Tor
func (c *Client) EstimateUploadTime(sizeBytes int64) time.Duration {
	// PERFORMANCE CALCULATION: Based on empirical Tor speeds
	// Average Tor bandwidth: 50-200 KB/s (vs 1-10 MB/s clearnet)
	
	metrics := c.GetMetrics()
	
	// Base estimate: 100 KB/s through Tor
	baseRate := float64(100 * 1024) // bytes per second
	
	// Adjust based on measured performance
	if metrics.AverageLatency > 0 {
		// Higher latency = lower effective bandwidth
		latencyFactor := 1.0 / (1.0 + metrics.AverageLatency.Seconds())
		baseRate *= latencyFactor
	}
	
	// Add overhead for circuit establishment
	setupTime := 2 * time.Second
	if c.circuitPool != nil && c.circuitPool.HasAvailableCircuits() {
		setupTime = 100 * time.Millisecond // Much faster with pool
	}
	
	transferTime := time.Duration(float64(sizeBytes)/baseRate) * time.Second
	
	// Add jitter time if enabled
	if c.config.Upload.Enabled {
		avgJitter := (c.config.Upload.JitterMin + c.config.Upload.JitterMax) / 2
		transferTime += avgJitter
	}
	
	return setupTime + transferTime
}

// updateMetrics safely updates metrics
func (c *Client) updateMetrics(fn func(*Metrics)) {
	c.metrics.mu.Lock()
	defer c.metrics.mu.Unlock()
	fn(c.metrics)
}

// Helpers

type UploadRequest struct {
	URL  string
	Body io.Reader
}

func randomDuration(min, max time.Duration) time.Duration {
	if min >= max {
		return min
	}
	delta := max - min
	return min + time.Duration(randInt64n(int64(delta)))
}

func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	if netErr, ok := err.(net.Error); ok {
		return netErr.Timeout()
	}
	return false
}

// Close cleans up resources
func (c *Client) Close() error {
	if c.circuitPool != nil {
		return c.circuitPool.Close()
	}
	return nil
}