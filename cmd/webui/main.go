package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/cache"
	"github.com/TheEntropyCollective/noisefs/pkg/descriptors"
	"github.com/TheEntropyCollective/noisefs/pkg/ipfs"
	"github.com/TheEntropyCollective/noisefs/pkg/noisefs"
)

type WebUI struct {
	ipfsClient   *ipfs.Client
	noisefsClient *noisefs.Client
	cache        cache.Cache
}

type UploadResponse struct {
	Success       bool   `json:"success"`
	DescriptorCID string `json:"descriptor_cid"`
	Filename      string `json:"filename"`
	Size          int64  `json:"size"`
	Error         string `json:"error,omitempty"`
}

type MetricsResponse struct {
	noisefs.MetricsSnapshot
	Timestamp time.Time `json:"timestamp"`
}

func main() {
	// Create IPFS client
	ipfsClient, err := ipfs.NewClient("localhost:5001")
	if err != nil {
		log.Fatalf("Failed to connect to IPFS: %v", err)
	}

	// Create cache and NoiseFS client
	blockCache := cache.NewMemoryCache(1000)
	noisefsClient, err := noisefs.NewClient(ipfsClient, blockCache)
	if err != nil {
		log.Fatalf("Failed to create NoiseFS client: %v", err)
	}

	webui := &WebUI{
		ipfsClient:    ipfsClient,
		noisefsClient: noisefsClient,
		cache:         blockCache,
	}

	// Serve static files
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("cmd/webui/static/"))))

	// API endpoints
	http.HandleFunc("/", webui.indexHandler)
	http.HandleFunc("/api/upload", webui.uploadHandler)
	http.HandleFunc("/api/download", webui.downloadHandler)
	http.HandleFunc("/api/metrics", webui.metricsHandler)

	fmt.Println("NoiseFS Web UI starting on http://localhost:8080")
	fmt.Println("Make sure IPFS daemon is running on localhost:5001")
	
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func (w *WebUI) indexHandler(rw http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>NoiseFS - Anonymous Distributed Storage</title>
    <link rel="stylesheet" href="/static/css/style.css">
</head>
<body>
    <div class="container">
        <header>
            <h1>NoiseFS</h1>
            <p>Anonymous Distributed File Storage</p>
        </header>

        <main>
            <div class="section">
                <h2>Upload File</h2>
                <form id="uploadForm" enctype="multipart/form-data">
                    <div class="form-group">
                        <label for="file">Choose file to upload:</label>
                        <input type="file" id="file" name="file" required>
                    </div>
                    <div class="form-group">
                        <label for="blockSize">Block Size (bytes):</label>
                        <select id="blockSize" name="blockSize">
                            <option value="131072" selected>128 KB (default)</option>
                            <option value="65536">64 KB</option>
                            <option value="262144">256 KB</option>
                            <option value="524288">512 KB</option>
                        </select>
                    </div>
                    <button type="submit">Upload</button>
                </form>
                <div id="uploadResult"></div>
            </div>

            <div class="section">
                <h2>Download File</h2>
                <form id="downloadForm">
                    <div class="form-group">
                        <label for="descriptorCID">Descriptor CID:</label>
                        <input type="text" id="descriptorCID" name="descriptorCID" required 
                               placeholder="Enter descriptor CID from upload">
                    </div>
                    <button type="submit">Download</button>
                </form>
                <div id="downloadResult"></div>
            </div>

            <div class="section">
                <h2>System Metrics</h2>
                <div id="metrics">
                    <p>Loading metrics...</p>
                </div>
            </div>
        </main>
    </div>

    <script src="/static/js/app.js"></script>
</body>
</html>`

	rw.Header().Set("Content-Type", "text/html")
	rw.Write([]byte(html))
}

func (w *WebUI) uploadHandler(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(rw, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse multipart form
	err := r.ParseMultipartForm(32 << 20) // 32MB max
	if err != nil {
		w.sendError(rw, "Failed to parse form", err)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		w.sendError(rw, "Failed to get file", err)
		return
	}
	defer file.Close()

	// Get block size
	blockSizeStr := r.FormValue("blockSize")
	blockSize, err := strconv.Atoi(blockSizeStr)
	if err != nil {
		blockSize = blocks.DefaultBlockSize
	}

	// Upload file
	descriptorCID, err := w.uploadFile(file, header.Filename, int64(header.Size), blockSize)
	if err != nil {
		w.sendError(rw, "Upload failed", err)
		return
	}

	response := UploadResponse{
		Success:       true,
		DescriptorCID: descriptorCID,
		Filename:      header.Filename,
		Size:          header.Size,
	}

	rw.Header().Set("Content-Type", "application/json")
	json.NewEncoder(rw).Encode(response)
}

func (w *WebUI) downloadHandler(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(rw, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	descriptorCID := r.URL.Query().Get("cid")
	if descriptorCID == "" {
		w.sendError(rw, "Missing descriptor CID", nil)
		return
	}

	// Download file
	data, filename, err := w.downloadFile(descriptorCID)
	if err != nil {
		w.sendError(rw, "Download failed", err)
		return
	}

	// Set headers for file download
	rw.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	rw.Header().Set("Content-Type", "application/octet-stream")
	rw.Header().Set("Content-Length", strconv.Itoa(len(data)))

	rw.Write(data)
}

func (w *WebUI) metricsHandler(rw http.ResponseWriter, r *http.Request) {
	metrics := w.noisefsClient.GetMetrics()
	response := MetricsResponse{
		MetricsSnapshot: metrics,
		Timestamp:       time.Now(),
	}

	rw.Header().Set("Content-Type", "application/json")
	json.NewEncoder(rw).Encode(response)
}

func (w *WebUI) uploadFile(file io.Reader, filename string, fileSize int64, blockSize int) (string, error) {
	// Create splitter
	splitter, err := blocks.NewSplitter(blockSize)
	if err != nil {
		return "", err
	}

	// Split file into blocks
	fileBlocks, err := splitter.Split(file)
	if err != nil {
		return "", err
	}

	// Create descriptor
	descriptor := descriptors.NewDescriptor(filename, fileSize, blockSize)

	// Process each block
	for _, block := range fileBlocks {
		// Select randomizer
		randomizerBlock, randomizerCID, err := w.noisefsClient.SelectRandomizer(block.Size())
		if err != nil {
			return "", err
		}

		// XOR with randomizer
		anonymizedBlock, err := block.XOR(randomizerBlock)
		if err != nil {
			return "", err
		}

		// Store anonymized block
		dataCID, err := w.noisefsClient.StoreBlockWithCache(anonymizedBlock)
		if err != nil {
			return "", err
		}

		// Add to descriptor
		if err := descriptor.AddBlockPair(dataCID, randomizerCID); err != nil {
			return "", err
		}
	}

	// Store descriptor
	store, err := descriptors.NewStore(w.ipfsClient)
	if err != nil {
		return "", err
	}

	descriptorCID, err := store.Save(descriptor)
	if err != nil {
		return "", err
	}

	// Record metrics
	totalStoredBytes := int64(0)
	for _, block := range fileBlocks {
		totalStoredBytes += int64(len(block.Data))
	}
	w.noisefsClient.RecordUpload(fileSize, totalStoredBytes*2)

	return descriptorCID, nil
}

func (w *WebUI) downloadFile(descriptorCID string) ([]byte, string, error) {
	// Load descriptor
	store, err := descriptors.NewStore(w.ipfsClient)
	if err != nil {
		return nil, "", err
	}

	descriptor, err := store.Load(descriptorCID)
	if err != nil {
		return nil, "", err
	}

	// Retrieve blocks
	dataBlocks := make([]*blocks.Block, len(descriptor.Blocks))
	randomizerBlocks := make([]*blocks.Block, len(descriptor.Blocks))

	for i, blockPair := range descriptor.Blocks {
		// Get data block
		dataBlock, err := w.ipfsClient.RetrieveBlock(blockPair.DataCID)
		if err != nil {
			return nil, "", err
		}
		dataBlocks[i] = dataBlock

		// Get randomizer block
		randomizerBlock, err := w.ipfsClient.RetrieveBlock(blockPair.RandomizerCID)
		if err != nil {
			return nil, "", err
		}
		randomizerBlocks[i] = randomizerBlock
	}

	// XOR to reconstruct original blocks
	originalBlocks := make([]*blocks.Block, len(dataBlocks))
	for i := range dataBlocks {
		originalBlock, err := dataBlocks[i].XOR(randomizerBlocks[i])
		if err != nil {
			return nil, "", err
		}
		originalBlocks[i] = originalBlock
	}

	// Assemble file
	assembler := blocks.NewAssembler()
	data, err := assembler.Assemble(originalBlocks)
	if err != nil {
		return nil, "", err
	}

	// Record download
	w.noisefsClient.RecordDownload()

	return data, descriptor.Filename, nil
}

func (w *WebUI) sendError(rw http.ResponseWriter, message string, err error) {
	errorMsg := message
	if err != nil {
		errorMsg += ": " + err.Error()
	}

	response := UploadResponse{
		Success: false,
		Error:   errorMsg,
	}

	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(rw).Encode(response)
}