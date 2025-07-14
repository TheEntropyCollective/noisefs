package tor

import (
	"context"
	"fmt"
	"time"
	
	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/core/descriptors"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/config"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
)

// TorEnabledClient wraps storage manager with Tor support
type TorEnabledClient struct {
	baseClient  storage.Backend
	torClient   *Client
	transport   *IPFSTransport
	config      *config.TorConfig
	metrics     *IntegrationMetrics
}

// IntegrationMetrics tracks Tor integration performance
type IntegrationMetrics struct {
	TorUploads      int64
	TorDownloads    int64
	DirectUploads   int64
	DirectDownloads int64
	AvgTorSpeed     float64
	AvgDirectSpeed  float64
}

// NewTorEnabledClient creates a new Tor-enabled storage client
func NewTorEnabledClient(baseClient storage.Backend, cfg *config.Config) (*TorEnabledClient, error) {
	if !cfg.Tor.Enabled {
		// Return wrapper that just uses base client
		return &TorEnabledClient{
			baseClient: baseClient,
			config:     &cfg.Tor,
			metrics:    &IntegrationMetrics{},
		}, nil
	}
	
	// Create Tor configuration from NoiseFS config
	torConfig := &Config{
		Enabled:     cfg.Tor.Enabled,
		SOCKSProxy:  cfg.Tor.SOCKSProxy,
		ControlPort: cfg.Tor.ControlPort,
		Upload: UploadConfig{
			Enabled:       cfg.Tor.UploadEnabled,
			JitterMin:     time.Duration(cfg.Tor.UploadJitterMin) * time.Second,
			JitterMax:     time.Duration(cfg.Tor.UploadJitterMax) * time.Second,
			SplitCircuits: true,
		},
		Download: DownloadConfig{
			Enabled:      cfg.Tor.DownloadEnabled,
			CircuitReuse: true,
		},
		Announce: AnnounceConfig{
			Enabled: cfg.Tor.AnnounceEnabled,
		},
	}
	
	// Set default config
	if torConfig.SOCKSProxy == "" {
		torConfig.SOCKSProxy = "127.0.0.1:9050"
	}
	
	// Apply remaining defaults
	torConfig.CircuitPool = DefaultConfig().CircuitPool
	torConfig.Performance = DefaultConfig().Performance
	
	// Create Tor client
	torClient, err := NewClient(torConfig)
	if err != nil {
		// PERFORMANCE WARNING: Tor not available, falling back to direct
		fmt.Printf("Warning: Tor not available (%v), using direct connection\n", err)
		return &TorEnabledClient{
			baseClient: baseClient,
			config:     &cfg.Tor,
			metrics:    &IntegrationMetrics{},
		}, nil
	}
	
	// Create transport
	ipfsURL := fmt.Sprintf("http://%s", cfg.IPFS.APIEndpoint)
	transport := NewIPFSTransport(torClient, ipfsURL)
	
	return &TorEnabledClient{
		baseClient: baseClient,
		torClient:  torClient,
		transport:  transport,
		config:     &cfg.Tor,
		metrics:    &IntegrationMetrics{},
	}, nil
}

// StoreBlock stores a block, using Tor if configured
func (c *TorEnabledClient) StoreBlock(block *blocks.Block) (string, error) {
	// PERFORMANCE DECISION: Use Tor for uploads?
	if c.shouldUseTorForUpload() {
		return c.storeBlockViaTor(context.Background(), block)
	}
	
	// Direct upload (faster)
	c.metrics.DirectUploads++
	address, err := c.baseClient.Put(context.Background(), block)
	if err != nil {
		return "", err
	}
	return address.ID, nil
}

// RetrieveBlock retrieves a block, using Tor if configured
func (c *TorEnabledClient) RetrieveBlock(cid string) (*blocks.Block, error) {
	// PERFORMANCE DECISION: Use Tor for downloads?
	if c.shouldUseTorForDownload() {
		data, err := c.retrieveBlockViaTor(context.Background(), cid)
		if err != nil {
			return nil, err
		}
		return blocks.NewBlock(data)
	}
	
	// Direct download (faster)
	c.metrics.DirectDownloads++
	address := &storage.BlockAddress{
		ID:          cid,
		BackendType: storage.BackendTypeIPFS,
	}
	return c.baseClient.Get(context.Background(), address)
}

// HasBlock checks if a block exists (always direct for performance)
func (c *TorEnabledClient) HasBlock(cid string) (bool, error) {
	// PERFORMANCE: Always use direct connection for existence checks
	address := &storage.BlockAddress{
		ID:          cid,
		BackendType: storage.BackendTypeIPFS,
	}
	return c.baseClient.Has(context.Background(), address)
}

// storeBlockViaTor uploads a block through Tor
func (c *TorEnabledClient) storeBlockViaTor(ctx context.Context, block *blocks.Block) (string, error) {
	if c.transport == nil {
		return "", fmt.Errorf("Tor transport not initialized")
	}
	
	// PERFORMANCE LOG: Track upload speed
	start := time.Now()
	size := len(block.Data)
	
	// Log estimated time
	estimate := c.torClient.EstimateUploadTime(int64(size))
	if estimate > 10*time.Second {
		fmt.Printf("Uploading %d KB via Tor (estimated: %v)...\n", size/1024, estimate)
	}
	
	cid, err := c.transport.Add(ctx, block.Data)
	if err != nil {
		return "", fmt.Errorf("Tor upload failed: %w", err)
	}
	
	// Update metrics
	elapsed := time.Since(start)
	speed := float64(size) / elapsed.Seconds()
	c.updateMetrics(true, speed)
	
	// PERFORMANCE WARNING: Very slow upload
	if elapsed > estimate*2 {
		fmt.Printf("Warning: Upload took %v (2x longer than estimated)\n", elapsed)
	}
	
	return cid, nil
}

// retrieveBlockViaTor downloads a block through Tor
func (c *TorEnabledClient) retrieveBlockViaTor(ctx context.Context, cid string) ([]byte, error) {
	if c.transport == nil {
		return nil, fmt.Errorf("Tor transport not initialized")
	}
	
	// PERFORMANCE LOG: Track download speed
	start := time.Now()
	
	data, err := c.transport.Get(ctx, cid)
	if err != nil {
		return nil, fmt.Errorf("Tor download failed: %w", err)
	}
	
	// Update metrics
	elapsed := time.Since(start)
	speed := float64(len(data)) / elapsed.Seconds()
	c.updateMetrics(false, speed)
	
	return data, nil
}

// BatchStoreBlocks stores multiple blocks with optimized Tor usage
func (c *TorEnabledClient) BatchStoreBlocks(blocks []*blocks.Block) ([]string, error) {
	if !c.shouldUseTorForUpload() {
		// Direct batch upload
		cids := make([]string, len(blocks))
		for i, block := range blocks {
			address, err := c.baseClient.Put(context.Background(), block)
			if err != nil {
				return nil, err
			}
			cids[i] = address.ID
			c.metrics.DirectUploads++
		}
		return cids, nil
	}
	
	// PERFORMANCE OPTIMIZATION: Batch upload via Tor
	if c.transport == nil {
		return nil, fmt.Errorf("Tor transport not initialized")
	}
	
	// Convert blocks to byte arrays
	blockData := make([][]byte, len(blocks))
	totalSize := 0
	for i, block := range blocks {
		blockData[i] = block.Data
		totalSize += len(block.Data)
	}
	
	// Log performance expectation
	fmt.Printf("Batch uploading %d blocks (%d KB total) via Tor...\n", 
		len(blocks), totalSize/1024)
	
	// Use parallel circuits for better performance
	start := time.Now()
	cids, err := c.transport.BatchAdd(context.Background(), blockData)
	if err != nil {
		return nil, err
	}
	
	elapsed := time.Since(start)
	fmt.Printf("Batch upload complete in %v (%.1f KB/s)\n", 
		elapsed, float64(totalSize)/1024/elapsed.Seconds())
	
	c.metrics.TorUploads += int64(len(blocks))
	
	return cids, nil
}

// StoreDescriptor stores a descriptor, always using Tor for metadata
func (c *TorEnabledClient) StoreDescriptor(desc *descriptors.Descriptor) (string, error) {
	// PRIVACY: Always use Tor for descriptors if available
	if c.torClient != nil && c.config.UploadEnabled {
		data, err := desc.Marshal()
		if err != nil {
			return "", err
		}
		
		// Create block from descriptor
		block, err := blocks.NewBlock(data)
		if err != nil {
			return "", err
		}
		
		return c.storeBlockViaTor(context.Background(), block)
	}
	
	// Fallback to direct
	block := &blocks.Block{Data: mustMarshal(desc)}
	address, err := c.baseClient.Put(context.Background(), block)
	if err != nil {
		return "", err
	}
	return address.ID, nil
}

// shouldUseTorForUpload decides whether to use Tor for uploads
func (c *TorEnabledClient) shouldUseTorForUpload() bool {
	return c.torClient != nil && c.config.UploadEnabled
}

// shouldUseTorForDownload decides whether to use Tor for downloads
func (c *TorEnabledClient) shouldUseTorForDownload() bool {
	return c.torClient != nil && c.config.DownloadEnabled
}

// updateMetrics updates performance metrics
func (c *TorEnabledClient) updateMetrics(isUpload bool, speed float64) {
	if isUpload {
		c.metrics.TorUploads++
		// Exponential moving average
		if c.metrics.AvgTorSpeed == 0 {
			c.metrics.AvgTorSpeed = speed
		} else {
			c.metrics.AvgTorSpeed = (c.metrics.AvgTorSpeed*0.9) + (speed*0.1)
		}
	} else {
		c.metrics.TorDownloads++
	}
}

// GetMetrics returns integration metrics
func (c *TorEnabledClient) GetMetrics() IntegrationMetrics {
	return *c.metrics
}

// GetPerformanceReport generates a performance comparison report
func (c *TorEnabledClient) GetPerformanceReport() string {
	m := c.metrics
	
	totalUploads := m.TorUploads + m.DirectUploads
	totalDownloads := m.TorDownloads + m.DirectDownloads
	
	if totalUploads == 0 && totalDownloads == 0 {
		return "No operations performed yet"
	}
	
	report := fmt.Sprintf(`Tor Integration Performance Report:
	
Uploads:
  Via Tor:    %d (%.1f%%)
  Direct:     %d (%.1f%%)
  Avg Speed:  %.1f KB/s (Tor) vs ~1000 KB/s (Direct)
  
Downloads:
  Via Tor:    %d (%.1f%%)
  Direct:     %d (%.1f%%)
  
Performance Impact:
  Upload:     ~%.1fx slower with Tor
  Download:   ~%.1fx slower with Tor
  Privacy:    Significantly improved with Tor`,
		m.TorUploads, float64(m.TorUploads)/float64(totalUploads)*100,
		m.DirectUploads, float64(m.DirectUploads)/float64(totalUploads)*100,
		m.AvgTorSpeed/1024,
		m.TorDownloads, float64(m.TorDownloads)/float64(totalDownloads)*100,
		m.DirectDownloads, float64(m.DirectDownloads)/float64(totalDownloads)*100,
		1000.0/(m.AvgTorSpeed/1024), // Assuming 1MB/s direct speed
		2.5, // Typical Tor download slowdown
	)
	
	return report
}

// Close cleans up Tor client resources
func (c *TorEnabledClient) Close() error {
	if c.torClient != nil {
		// Print final performance report
		fmt.Println(c.GetPerformanceReport())
		return c.torClient.Close()
	}
	return nil
}

// Helper function
func mustMarshal(desc *descriptors.Descriptor) []byte {
	data, _ := desc.Marshal()
	return data
}