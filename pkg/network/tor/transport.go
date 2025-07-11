package tor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// IPFSTransport provides Tor-routed transport for IPFS operations
type IPFSTransport struct {
	torClient   *Client
	ipfsBaseURL string
	config      *Config
}

// NewIPFSTransport creates a new IPFS transport over Tor
func NewIPFSTransport(torClient *Client, ipfsURL string) *IPFSTransport {
	return &IPFSTransport{
		torClient:   torClient,
		ipfsBaseURL: strings.TrimSuffix(ipfsURL, "/"),
		config:      torClient.config,
	}
}

// Add adds a block to IPFS through Tor
func (t *IPFSTransport) Add(ctx context.Context, data []byte) (string, error) {
	// PERFORMANCE IMPACT: Upload through Tor
	// - Base overhead: 2-3x slower than direct
	// - With jitter: Additional 1-5s delay
	// - Circuit establishment: 1-3s if no pool
	
	if !t.shouldUseTor("upload") {
		return "", fmt.Errorf("Tor not enabled for uploads")
	}
	
	// Prepare request
	url := fmt.Sprintf("%s/api/v0/add?pin=false", t.ipfsBaseURL)
	
	// PERFORMANCE OPTIMIZATION: For small blocks, batch might help
	// but IPFS doesn't support batch add via API efficiently
	
	start := time.Now()
	
	// Use circuit pool for better performance
	var resp *http.Response
	var err error
	
	if t.config.Upload.SplitCircuits && t.torClient.circuitPool != nil {
		// PERFORMANCE: Use dedicated circuit for this upload
		circuit, cerr := t.torClient.circuitPool.GetCircuit()
		if cerr == nil {
			resp, err = t.uploadWithCircuit(ctx, url, data, circuit)
		} else {
			// Fallback to default client
			resp, err = t.uploadWithClient(ctx, url, data)
		}
	} else {
		resp, err = t.uploadWithClient(ctx, url, data)
	}
	
	if err != nil {
		return "", fmt.Errorf("upload failed: %w", err)
	}
	defer resp.Body.Close()
	
	// Log performance metrics
	uploadTime := time.Since(start)
	throughput := float64(len(data)) / uploadTime.Seconds()
	
	// PERFORMANCE WARNING: Very slow upload
	if throughput < 10*1024 { // Less than 10KB/s
		fmt.Printf("Warning: Very slow Tor upload: %.1f KB/s\n", throughput/1024)
	}
	
	// Parse response to get CID
	// ... (response parsing logic)
	
	return parseCIDFromResponse(resp.Body)
}

// Get retrieves a block from IPFS through Tor
func (t *IPFSTransport) Get(ctx context.Context, cid string) ([]byte, error) {
	// PERFORMANCE IMPACT: Download through Tor
	// - Typically 2-4x slower than direct
	// - Can be disabled for performance
	
	if !t.shouldUseTor("download") {
		return nil, fmt.Errorf("Tor not enabled for downloads")
	}
	
	url := fmt.Sprintf("%s/api/v0/cat?arg=%s", t.ipfsBaseURL, cid)
	
	start := time.Now()
	
	resp, err := t.torClient.GetWithCircuit(ctx, url, "download")
	if err != nil {
		return nil, fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()
	
	// Read response
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	
	// Performance metrics
	downloadTime := time.Since(start)
	throughput := float64(len(data)) / downloadTime.Seconds()
	
	// Update client metrics
	t.torClient.updateMetrics(func(m *Metrics) {
		m.TotalBandwidth += int64(len(data))
	})
	
	// PERFORMANCE LOG: Track download speeds
	if len(data) > 1024*1024 { // Log for files > 1MB
		fmt.Printf("Tor download: %d bytes in %v (%.1f KB/s)\n", 
			len(data), downloadTime, throughput/1024)
	}
	
	return data, nil
}

// PublishToIPNS publishes to IPNS through Tor
func (t *IPFSTransport) PublishToIPNS(ctx context.Context, name, value string) error {
	// PERFORMANCE IMPACT: IPNS operations are already slow
	// Tor adds 2-3x additional latency
	
	if !t.shouldUseTor("announce") {
		return fmt.Errorf("Tor not enabled for announcements")
	}
	
	url := fmt.Sprintf("%s/api/v0/name/publish?arg=%s&key=%s", 
		t.ipfsBaseURL, value, name)
	
	// PERFORMANCE WARNING: IPNS + Tor can take 30-60s
	fmt.Println("Publishing to IPNS via Tor (this may take 30-60 seconds)...")
	
	resp, err := t.torClient.PostWithJitter(ctx, url, nil)
	if err != nil {
		return fmt.Errorf("IPNS publish failed: %w", err)
	}
	defer resp.Body.Close()
	
	return nil
}

// uploadWithCircuit uploads using a specific circuit
func (t *IPFSTransport) uploadWithCircuit(ctx context.Context, url string, data []byte, circuit *Circuit) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	
	// Set multipart headers for IPFS
	req.Header.Set("Content-Type", "multipart/form-data; boundary=boundary")
	
	// Apply jitter if configured
	if t.config.Upload.JitterMax > 0 {
		jitter := randomDuration(t.config.Upload.JitterMin, t.config.Upload.JitterMax)
		time.Sleep(jitter)
	}
	
	return circuit.HTTPClient.Do(req)
}

// uploadWithClient uploads using the default Tor client
func (t *IPFSTransport) uploadWithClient(ctx context.Context, url string, data []byte) (*http.Response, error) {
	return t.torClient.PostWithJitter(ctx, url, bytes.NewReader(data))
}

// shouldUseTor checks if Tor should be used for operation
func (t *IPFSTransport) shouldUseTor(operation string) bool {
	if !t.config.Enabled {
		return false
	}
	
	switch operation {
	case "upload":
		return t.config.Upload.Enabled
	case "download":
		return t.config.Download.Enabled
	case "announce":
		return t.config.Announce.Enabled
	default:
		return false
	}
}

// BatchAdd performs parallel uploads with multiple circuits
func (t *IPFSTransport) BatchAdd(ctx context.Context, blocks [][]byte) ([]string, error) {
	// PERFORMANCE OPTIMIZATION: Parallel uploads on different circuits
	// Can achieve 3-5x speedup vs sequential
	
	if len(blocks) == 1 {
		cid, err := t.Add(ctx, blocks[0])
		return []string{cid}, err
	}
	
	// Prepare upload requests
	uploads := make([]UploadRequest, len(blocks))
	for i, block := range blocks {
		uploads[i] = UploadRequest{
			URL:  fmt.Sprintf("%s/api/v0/add?pin=false", t.ipfsBaseURL),
			Body: bytes.NewReader(block),
		}
	}
	
	// PERFORMANCE: Use SplitUpload for parallel circuits
	fmt.Printf("Uploading %d blocks via Tor (parallel circuits)...\n", len(blocks))
	start := time.Now()
	
	err := t.torClient.SplitUpload(ctx, uploads)
	if err != nil {
		return nil, err
	}
	
	uploadTime := time.Since(start)
	totalSize := 0
	for _, b := range blocks {
		totalSize += len(b)
	}
	
	throughput := float64(totalSize) / uploadTime.Seconds()
	fmt.Printf("Batch upload complete: %d blocks, %.1f KB in %v (%.1f KB/s)\n",
		len(blocks), float64(totalSize)/1024, uploadTime, throughput/1024)
	
	// TODO: Parse CIDs from responses
	cids := make([]string, len(blocks))
	for i := range cids {
		cids[i] = fmt.Sprintf("QmTorBlock%d", i) // Placeholder
	}
	
	return cids, nil
}

// GetWithMetadata retrieves a block and its metadata
func (t *IPFSTransport) GetWithMetadata(ctx context.Context, cid string, metadataOnly bool) ([]byte, map[string]string, error) {
	// PERFORMANCE OPTIMIZATION: Metadata-only requests are smaller
	if metadataOnly && t.config.Download.OnlyMetadata {
		// Only get metadata through Tor, not the full block
		return t.getMetadata(ctx, cid)
	}
	
	data, err := t.Get(ctx, cid)
	if err != nil {
		return nil, nil, err
	}
	
	// Extract metadata (simplified)
	metadata := map[string]string{
		"size": fmt.Sprintf("%d", len(data)),
		"cid":  cid,
	}
	
	return data, metadata, nil
}

// getMetadata retrieves only metadata
func (t *IPFSTransport) getMetadata(ctx context.Context, cid string) ([]byte, map[string]string, error) {
	// PERFORMANCE: Metadata requests are much smaller
	url := fmt.Sprintf("%s/api/v0/object/stat?arg=%s", t.ipfsBaseURL, cid)
	
	resp, err := t.torClient.GetWithCircuit(ctx, url, "metadata")
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	
	// Parse metadata response
	// ... (parsing logic)
	
	return nil, map[string]string{"cid": cid}, nil
}

// EstimateTransferTime estimates time for operation
func (t *IPFSTransport) EstimateTransferTime(operation string, sizeBytes int64) time.Duration {
	if !t.shouldUseTor(operation) {
		// Direct connection estimates
		return time.Duration(sizeBytes/1024/1024) * time.Second // 1MB/s
	}
	
	// Use Tor client's estimation
	return t.torClient.EstimateUploadTime(sizeBytes)
}

// Helper to parse CID from IPFS response
func parseCIDFromResponse(body io.Reader) (string, error) {
	// Simplified - actual implementation would parse JSON
	data, err := io.ReadAll(body)
	if err != nil {
		return "", err
	}
	
	// Look for CID in response
	response := string(data)
	if strings.Contains(response, "Hash") {
		// Extract CID from JSON response
		// ... (parsing logic)
		return "QmExampleCID", nil
	}
	
	return "", fmt.Errorf("CID not found in response")
}