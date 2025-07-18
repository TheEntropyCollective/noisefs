package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/config"
	"github.com/TheEntropyCollective/noisefs/pkg/core/descriptors"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/TheEntropyCollective/noisefs/pkg/core/client"
	noisefsX509 "github.com/TheEntropyCollective/noisefs/cmd/webui/tls"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/validation"
)

type WebUI struct {
	storageManager *storage.Manager
	noisefsClient  *noisefs.Client
	cache          cache.Cache
	config         *config.Config
	validator      *validation.Validator
	rateLimiter    *validation.RateLimiter
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

type RangeRequest struct {
	Start int64
	End   int64
	Size  int64
}

type StreamingFile struct {
	descriptor *descriptors.Descriptor
	blocks     []*blocks.Block
	size       int64
	filename   string
}

func main() {
	// Parse command line flags
	var configFile = flag.String("config", "", "Path to configuration file")
	flag.Parse()

	// Load configuration
	cfg, err := config.LoadConfig(*configFile)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Create storage manager
	storageConfig := storage.DefaultConfig()
	if ipfsBackend, exists := storageConfig.Backends["ipfs"]; exists {
		ipfsBackend.Connection.Endpoint = cfg.IPFS.APIEndpoint
	}
	
	storageManager, err := storage.NewManager(storageConfig)
	if err != nil {
		log.Fatalf("Failed to create storage manager: %v", err)
	}
	
	err = storageManager.Start(context.Background())
	if err != nil {
		log.Fatalf("Failed to start storage manager: %v", err)
	}
	defer storageManager.Stop(context.Background())
	
	// Create cache and NoiseFS client
	blockCache := cache.NewMemoryCache(cfg.Cache.BlockCacheSize)
	noisefsClient, err := noisefs.NewClient(storageManager, blockCache)
	if err != nil {
		log.Fatalf("Failed to create NoiseFS client: %v", err)
	}

	// Create input validator
	validator := validation.NewValidator()
	validator.SetMaxFileSize(100 * 1024 * 1024) // 100MB limit
	
	// Create rate limiter
	rateLimitConfig := validation.DefaultRateLimitConfig()
	rateLimiter := validation.NewRateLimiter(rateLimitConfig)

	webui := &WebUI{
		storageManager: storageManager,
		noisefsClient:  noisefsClient,
		cache:          blockCache,
		config:         cfg,
		validator:      validator,
		rateLimiter:    rateLimiter,
	}

	// Set up HTTP routes
	setupRoutes(webui)

	// Create and start server with TLS
	server, err := createServer(cfg)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	// Print startup information
	protocol := "https"
	if !cfg.WebUI.TLSEnabled {
		protocol = "http"
	}
	fmt.Printf("NoiseFS Web UI starting on %s://%s:%d\n", protocol, cfg.WebUI.Host, cfg.WebUI.Port)
	fmt.Printf("IPFS endpoint: %s\n", cfg.IPFS.APIEndpoint)
	
	if cfg.WebUI.TLSEnabled {
		log.Fatal(server.ListenAndServeTLS("", ""))
	} else {
		log.Fatal(server.ListenAndServe())
	}
}

func setupRoutes(webui *WebUI) {
	// Serve static files with security headers
	http.Handle("/static/", securityHeaders(http.StripPrefix("/static/", http.FileServer(http.Dir("cmd/webui/static/"))).ServeHTTP))

	// Create request size limiter for upload endpoints
	uploadSizeLimiter := validation.RequestSizeLimiter(105 * 1024 * 1024) // 105MB (5MB buffer)

	// Add security headers, rate limiting, and request size limits
	http.HandleFunc("/", securityHeaders(webui.indexHandler))
	http.HandleFunc("/api/upload", uploadSizeLimiter(webui.rateLimiter.Middleware(securityHeaders(webui.uploadHandler))))
	http.HandleFunc("/api/download", webui.rateLimiter.Middleware(securityHeaders(webui.downloadHandler)))
	http.HandleFunc("/api/metrics", securityHeaders(webui.metricsHandler))
}

func createServer(cfg *config.Config) (*http.Server, error) {
	addr := fmt.Sprintf("%s:%d", cfg.WebUI.Host, cfg.WebUI.Port)
	
	server := &http.Server{
		Addr:           addr,
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
		IdleTimeout:    120 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1MB max header size
		Handler:        nil,
	}

	if cfg.WebUI.TLSEnabled {
		var certFile, keyFile string
		var err error

		if cfg.WebUI.TLSAutoGen {
			// Auto-generate certificate
			certDir, err := noisefsX509.GetDefaultCertificateDir()
			if err != nil {
				return nil, fmt.Errorf("failed to get certificate directory: %w", err)
			}

			generator := noisefsX509.NewCertificateGenerator(certDir)
			certFile, keyFile, err = generator.LoadOrGenerateCertificate(cfg.WebUI.TLSHostnames)
			if err != nil {
				return nil, fmt.Errorf("failed to generate certificate: %w", err)
			}

			fmt.Printf("Using auto-generated TLS certificate: %s\n", certFile)
		} else {
			// Use provided certificate files
			certFile = cfg.WebUI.TLSCertFile
			keyFile = cfg.WebUI.TLSKeyFile
		}

		// Load TLS certificate
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load TLS certificate: %w", err)
		}

		// Configure TLS
		server.TLSConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
			CipherSuites: []uint16{
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			},
		}
	}

	return server, nil
}

func securityHeaders(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Comprehensive security headers
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-Permitted-Cross-Domain-Policies", "none")
		w.Header().Set("X-DNS-Prefetch-Control", "off")
		
		// Enhanced Content Security Policy
		csp := "default-src 'self'; " +
			"script-src 'self' 'unsafe-inline'; " +
			"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; " +
			"font-src 'self' https://fonts.gstatic.com; " +
			"img-src 'self' data: blob:; " +
			"media-src 'self' blob:; " +
			"connect-src 'self'; " +
			"object-src 'none'; " +
			"base-uri 'self'; " +
			"form-action 'self'; " +
			"frame-ancestors 'none'; " +
			"upgrade-insecure-requests"
		w.Header().Set("Content-Security-Policy", csp)
		
		// HSTS header for HTTPS
		if r.TLS != nil {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")
		}
		
		// Prevent MIME type confusion
		w.Header().Set("X-Download-Options", "noopen")
		
		// Permissions Policy (comprehensive permissions control for modern browsers)
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=(), payment=(), usb=(), magnetometer=(), gyroscope=(), speaker=(), fullscreen=(), autoplay=()")
		
		// Cross-Origin policies
		w.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")

		next(w, r)
	}
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
    <button class="theme-toggle" id="themeToggle" aria-label="Toggle theme">
        🌙
    </button>
    
    <div class="container">
        <header>
            <h1>NoiseFS</h1>
            <p>Anonymous Distributed File Storage</p>
        </header>

        <main>
            <div class="section upload-card">
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
                    <div class="form-group">
                        <label for="encrypt">Encryption:</label>
                        <select id="encrypt" name="encrypt">
                            <option value="encrypted" selected>Private (Encrypted)</option>
                            <option value="public">Public (Unencrypted)</option>
                        </select>
                    </div>
                    <div class="form-group" id="passwordGroup">
                        <label for="password">Password (for private files):</label>
                        <input type="password" id="password" name="password" 
                               placeholder="Enter password to encrypt file metadata">
                    </div>
                    <button type="submit">Upload</button>
                </form>
                <div id="uploadResult"></div>
            </div>

            <div class="section download-card">
                <h2>Download File</h2>
                <form id="downloadForm">
                    <div class="form-group">
                        <label for="descriptorCID">Descriptor CID:</label>
                        <input type="text" id="descriptorCID" name="descriptorCID" required 
                               placeholder="Enter descriptor CID from upload">
                    </div>
                    <div class="form-group">
                        <label for="downloadPassword">Password (if file is encrypted):</label>
                        <input type="password" id="downloadPassword" name="downloadPassword" 
                               placeholder="Enter password for encrypted files">
                    </div>
                    <div class="form-group">
                        <label for="downloadMode">Download Mode:</label>
                        <select id="downloadMode" name="downloadMode">
                            <option value="traditional" selected>Traditional (Full Download)</option>
                            <option value="streaming">Progressive (Streaming)</option>
                        </select>
                        <small class="help-text">
                            Traditional: Download complete file (better privacy)<br>
                            Progressive: Stream on-demand (better for media playback)
                        </small>
                    </div>
                    <button type="submit">Download</button>
                    <button type="button" id="streamPreviewBtn" style="display: none;">Stream Preview</button>
                </form>
                <div id="downloadResult"></div>
                <div id="streamingPreview" style="display: none;">
                    <h3>Media Preview</h3>
                    <div id="mediaContainer"></div>
                </div>
            </div>

            <div class="section metrics-card">
                <h2>System Metrics</h2>
                <div id="metrics">
                    <p>Loading metrics...</p>
                </div>
            </div>

            <div class="section info-card full-width">
                <h2>How NoiseFS Works</h2>
                <p>NoiseFS implements the OFFSystem architecture to provide anonymous, distributed file storage:</p>
                <div class="flow-diagram" id="flowDiagram">
                    <svg viewBox="0 0 800 400" width="100%" height="400">
                        <!-- File Input -->
                        <rect x="20" y="50" width="80" height="40" rx="5" fill="#4299e1" stroke="#2b6cb0"/>
                        <text x="60" y="75" text-anchor="middle" fill="white" font-size="12">Original File</text>
                        
                        <!-- Arrow 1 -->
                        <path d="M 110 70 L 140 70" stroke="#4a5568" stroke-width="2" marker-end="url(#arrowhead)"/>
                        <text x="125" y="65" text-anchor="middle" font-size="10" fill="#4a5568">Split</text>
                        
                        <!-- File Blocks -->
                        <g id="fileBlocks">
                            <rect x="150" y="30" width="40" height="30" rx="3" fill="#48bb78" stroke="#2f855a"/>
                            <text x="170" y="50" text-anchor="middle" fill="white" font-size="10">Block 1</text>
                            
                            <rect x="150" y="70" width="40" height="30" rx="3" fill="#48bb78" stroke="#2f855a"/>
                            <text x="170" y="90" text-anchor="middle" fill="white" font-size="10">Block 2</text>
                            
                            <rect x="150" y="110" width="40" height="30" rx="3" fill="#48bb78" stroke="#2f855a"/>
                            <text x="170" y="130" text-anchor="middle" fill="white" font-size="10">Block 3</text>
                        </g>
                        
                        <!-- Arrow 2 -->
                        <path d="M 200 70 L 230 70" stroke="#4a5568" stroke-width="2" marker-end="url(#arrowhead)"/>
                        <text x="215" y="65" text-anchor="middle" font-size="10" fill="#4a5568">XOR</text>
                        
                        <!-- Randomizer Blocks -->
                        <g id="randomizerBlocks">
                            <rect x="240" y="180" width="40" height="30" rx="3" fill="#ed8936" stroke="#c05621"/>
                            <text x="260" y="195" text-anchor="middle" fill="white" font-size="8">Random 1</text>
                            <text x="260" y="205" text-anchor="middle" fill="white" font-size="8">(cached)</text>
                            
                            <rect x="290" y="180" width="40" height="30" rx="3" fill="#ed8936" stroke="#c05621"/>
                            <text x="310" y="195" text-anchor="middle" fill="white" font-size="8">Random 2</text>
                            <text x="310" y="205" text-anchor="middle" fill="white" font-size="8">(cached)</text>
                            
                            <rect x="340" y="180" width="40" height="30" rx="3" fill="#ed8936" stroke="#c05621"/>
                            <text x="360" y="195" text-anchor="middle" fill="white" font-size="8">Random 3</text>
                            <text x="360" y="205" text-anchor="middle" fill="white" font-size="8">(new)</text>
                        </g>
                        
                        <!-- XOR Operation Indicators -->
                        <path d="M 260 170 L 170 110" stroke="#9f7aea" stroke-width="2" stroke-dasharray="3,3"/>
                        <path d="M 310 170 L 170 90" stroke="#9f7aea" stroke-width="2" stroke-dasharray="3,3"/>
                        <path d="M 360 170 L 170 50" stroke="#9f7aea" stroke-width="2" stroke-dasharray="3,3"/>
                        
                        <!-- XOR Symbol -->
                        <circle cx="300" cy="120" r="15" fill="#9f7aea" stroke="#6b46c1"/>
                        <text x="300" y="127" text-anchor="middle" fill="white" font-size="14" font-weight="bold">⊕</text>
                        
                        <!-- Arrow 3 -->
                        <path d="M 400 70 L 430 70" stroke="#4a5568" stroke-width="2" marker-end="url(#arrowhead)"/>
                        <text x="415" y="65" text-anchor="middle" font-size="10" fill="#4a5568">Store</text>
                        
                        <!-- Anonymized Blocks -->
                        <g id="anonymizedBlocks">
                            <rect x="440" y="30" width="50" height="30" rx="3" fill="#e53e3e" stroke="#c53030"/>
                            <text x="465" y="45" text-anchor="middle" fill="white" font-size="8">Anonymous</text>
                            <text x="465" y="55" text-anchor="middle" fill="white" font-size="8">Block 1</text>
                            
                            <rect x="440" y="70" width="50" height="30" rx="3" fill="#e53e3e" stroke="#c53030"/>
                            <text x="465" y="85" text-anchor="middle" fill="white" font-size="8">Anonymous</text>
                            <text x="465" y="95" text-anchor="middle" fill="white" font-size="8">Block 2</text>
                            
                            <rect x="440" y="110" width="50" height="30" rx="3" fill="#e53e3e" stroke="#c53030"/>
                            <text x="465" y="125" text-anchor="middle" fill="white" font-size="8">Anonymous</text>
                            <text x="465" y="135" text-anchor="middle" fill="white" font-size="8">Block 3</text>
                        </g>
                        
                        <!-- Arrow 4 -->
                        <path d="M 500 70 L 530 70" stroke="#4a5568" stroke-width="2" marker-end="url(#arrowhead)"/>
                        
                        <!-- IPFS -->
                        <rect x="540" y="50" width="80" height="40" rx="5" fill="#2d3748" stroke="#1a202c"/>
                        <text x="580" y="75" text-anchor="middle" fill="white" font-size="12">IPFS Network</text>
                        
                        <!-- Descriptor -->
                        <rect x="540" y="250" width="100" height="60" rx="5" fill="#805ad5" stroke="#553c9a"/>
                        <text x="590" y="270" text-anchor="middle" fill="white" font-size="10">File Descriptor</text>
                        <text x="590" y="285" text-anchor="middle" fill="white" font-size="8">Contains CIDs for</text>
                        <text x="590" y="295" text-anchor="middle" fill="white" font-size="8">data + randomizer</text>
                        <text x="590" y="305" text-anchor="middle" fill="white" font-size="8">blocks</text>
                        
                        <!-- Arrow to descriptor -->
                        <path d="M 590 100 L 590 240" stroke="#4a5568" stroke-width="2" marker-end="url(#arrowhead)"/>
                        <text x="610" y="170" font-size="10" fill="#4a5568">Create</text>
                        <text x="610" y="185" font-size="10" fill="#4a5568">Descriptor</text>
                        
                        <!-- Key/Legend -->
                        <g id="legend">
                            <text x="50" y="280" font-size="14" font-weight="bold" fill="#2d3748">Key Benefits:</text>
                            <text x="50" y="300" font-size="12" fill="#4a5568">• All stored blocks appear as random data</text>
                            <text x="50" y="315" font-size="12" fill="#4a5568">• Randomizers are reused for efficiency</text>
                            <text x="50" y="330" font-size="12" fill="#4a5568">• No original content is ever stored</text>
                            <text x="50" y="345" font-size="12" fill="#4a5568">• Plausible deniability for all participants</text>
                        </g>
                        
                        <!-- Arrow marker definition -->
                        <defs>
                            <marker id="arrowhead" markerWidth="10" markerHeight="7" 
                                    refX="9" refY="3.5" orient="auto">
                                <polygon points="0 0, 10 3.5, 0 7" fill="#4a5568"/>
                            </marker>
                        </defs>
                    </svg>
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

	// Get encryption settings
	encryptType := r.FormValue("encrypt")
	password := r.FormValue("password")
	
	// Determine if file should be encrypted
	useEncryption := encryptType == "encrypted" && password != ""

	// Input validation
	validationErrors := w.validator.ValidateUploadRequest(header.Filename, header.Size, blockSize)
	
	// Additional password validation if encryption is requested
	if useEncryption {
		if err := w.validator.ValidatePassword(password); err != nil {
			if ve, ok := err.(validation.ValidationError); ok {
				validationErrors = append(validationErrors, ve)
			}
		}
	}
	
	// Return validation errors if any
	if len(validationErrors) > 0 {
		errorMsg := "Validation failed:"
		for _, ve := range validationErrors {
			errorMsg += " " + ve.Error() + ";"
		}
		w.sendError(rw, errorMsg, nil)
		return
	}

	// Upload file
	descriptorCID, err := w.uploadFile(file, header.Filename, int64(header.Size), blockSize, useEncryption, password)
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

	password := r.URL.Query().Get("password")
	streaming := r.URL.Query().Get("stream") == "true"

	// Input validation
	validationErrors := w.validator.ValidateDownloadRequest(descriptorCID, password)
	
	// Return validation errors if any
	if len(validationErrors) > 0 {
		errorMsg := "Validation failed:"
		for _, ve := range validationErrors {
			errorMsg += " " + ve.Error() + ";"
		}
		w.sendError(rw, errorMsg, nil)
		return
	}

	if streaming {
		// Use streaming download for progressive/range requests
		w.streamingDownloadHandler(rw, r, descriptorCID, password)
	} else {
		// Use traditional download for full file downloads
		w.traditionalDownloadHandler(rw, r, descriptorCID, password)
	}
}

func (w *WebUI) traditionalDownloadHandler(rw http.ResponseWriter, r *http.Request, descriptorCID, password string) {
	// Download file
	data, filename, err := w.downloadFile(descriptorCID, password)
	if err != nil {
		w.sendError(rw, "Download failed", err)
		return
	}

	// Detect content type
	contentType := w.detectContentType(filename, data)

	// Set headers for file download
	rw.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	rw.Header().Set("Content-Type", contentType)
	rw.Header().Set("Content-Length", strconv.Itoa(len(data)))
	rw.Header().Set("Accept-Ranges", "bytes")

	rw.Write(data)
}

func (w *WebUI) streamingDownloadHandler(rw http.ResponseWriter, r *http.Request, descriptorCID, password string) {
	// Load streaming file metadata
	streamFile, err := w.loadStreamingFile(descriptorCID, password)
	if err != nil {
		w.sendError(rw, "Failed to load file for streaming", err)
		return
	}

	// Detect content type
	contentType := w.detectContentType(streamFile.filename, nil)

	// Parse range request if present
	rangeHeader := r.Header.Get("Range")
	if rangeHeader != "" {
		w.serveRangeRequest(rw, r, streamFile, contentType, rangeHeader)
	} else {
		w.serveFullStreamingFile(rw, r, streamFile, contentType)
	}
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

func (w *WebUI) uploadFile(file io.Reader, filename string, fileSize int64, blockSize int, useEncryption bool, password string) (string, error) {
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
	descriptor := descriptors.NewDescriptor(filename, fileSize, fileSize, blockSize)

	// Process each block (using 3-tuple format)
	for _, block := range fileBlocks {
		// Select two randomizers
		randomizer1Block, randomizer1CID, randomizer2Block, randomizer2CID, err := w.noisefsClient.SelectRandomizers(block.Size())
		if err != nil {
			return "", err
		}

		// XOR with both randomizers (3-tuple: data XOR randomizer1 XOR randomizer2)
		anonymizedBlock, err := block.XOR(randomizer1Block, randomizer2Block)
		if err != nil {
			return "", err
		}

		// Store anonymized block
		dataCID, err := w.noisefsClient.StoreBlockWithCache(anonymizedBlock)
		if err != nil {
			return "", err
		}

		// Add to descriptor (3-tuple format)
		if err := descriptor.AddBlockTriple(dataCID, randomizer1CID, randomizer2CID); err != nil {
			return "", err
		}
	}

	// Store descriptor (encrypted or unencrypted)
	var descriptorCID string
	
	if useEncryption {
		// Use encrypted store
		encStore, storeErr := descriptors.NewEncryptedStoreWithPassword(w.storageManager, password)
		if storeErr != nil {
			return "", storeErr
		}
		cid, saveErr := encStore.Save(descriptor)
		if saveErr != nil {
			return "", saveErr
		}
		descriptorCID = cid
	} else {
		// Use regular store for public content
		store, storeErr := descriptors.NewStore(w.storageManager)
		if storeErr != nil {
			return "", storeErr
		}
		cid, saveErr := store.Save(descriptor)
		if saveErr != nil {
			return "", saveErr
		}
		descriptorCID = cid
	}

	// Record metrics
	totalStoredBytes := int64(0)
	for _, block := range fileBlocks {
		totalStoredBytes += int64(len(block.Data))
	}
	w.noisefsClient.RecordUpload(fileSize, totalStoredBytes*2)

	return descriptorCID, nil
}

func (w *WebUI) downloadFile(descriptorCID string, password string) ([]byte, string, error) {
	// Try to load descriptor (encrypted first if password provided, then fallback to unencrypted)
	var descriptor *descriptors.Descriptor
	var loadErr error
	
	if password != "" {
		// Try encrypted store first
		encStore, encErr := descriptors.NewEncryptedStoreWithPassword(w.storageManager, password)
		if encErr == nil {
			descriptor, loadErr = encStore.Load(descriptorCID)
			if loadErr == nil {
				// Successfully loaded encrypted descriptor
			} else {
				// Try fallback to unencrypted
				descriptor = nil
			}
		}
	}
	
	if descriptor == nil {
		// Fallback to regular store (for unencrypted descriptors or if password fails)
		store, storeErr := descriptors.NewStore(w.storageManager)
		if storeErr != nil {
			return nil, "", storeErr
		}

		var err error
		descriptor, err = store.Load(descriptorCID)
		if err != nil {
			if password != "" {
				return nil, "", fmt.Errorf("failed to load descriptor (wrong password or not encrypted): %w", err)
			}
			return nil, "", err
		}
	}

	// Retrieve blocks
	dataBlocks := make([]*blocks.Block, len(descriptor.Blocks))
	randomizer1Blocks := make([]*blocks.Block, len(descriptor.Blocks))
	randomizer2Blocks := make([]*blocks.Block, len(descriptor.Blocks))

	for i, blockPair := range descriptor.Blocks {
		// Get data block
		address := &storage.BlockAddress{ID: blockPair.DataCID}
		dataBlock, err := w.storageManager.Get(context.Background(), address)
		if err != nil {
			return nil, "", err
		}
		dataBlocks[i] = dataBlock

		// Get first randomizer block
		rand1Address := &storage.BlockAddress{ID: blockPair.RandomizerCID1}
		randomizer1Block, err := w.storageManager.Get(context.Background(), rand1Address)
		if err != nil {
			return nil, "", err
		}
		randomizer1Blocks[i] = randomizer1Block

		// Get second randomizer block (3-tuple format)
		rand2Address := &storage.BlockAddress{ID: blockPair.RandomizerCID2}
		randomizer2Block, err := w.storageManager.Get(context.Background(), rand2Address)
		if err != nil {
			return nil, "", err
		}
		randomizer2Blocks[i] = randomizer2Block
	}

	// XOR to reconstruct original blocks
	originalBlocks := make([]*blocks.Block, len(dataBlocks))
	for i := range dataBlocks {
		// Use 3-tuple XOR
		originalBlock, err := dataBlocks[i].XOR(randomizer1Blocks[i], randomizer2Blocks[i])
		
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

// detectContentType detects the MIME type based on filename and optional data
func (w *WebUI) detectContentType(filename string, data []byte) string {
	// First try to detect from file extension
	ext := filepath.Ext(filename)
	if contentType := mime.TypeByExtension(ext); contentType != "" {
		return contentType
	}
	
	// For common media types not covered by mime package
	switch strings.ToLower(ext) {
	case ".mkv":
		return "video/x-matroska"
	case ".avi":
		return "video/x-msvideo"
	case ".flv":
		return "video/x-flv"
	case ".m4v":
		return "video/mp4"
	case ".ts":
		return "video/mp2t"
	}
	
	// Try to detect from content if data is available
	if data != nil && len(data) > 512 {
		return http.DetectContentType(data[:512])
	}
	
	// Default fallback
	return "application/octet-stream"
}

// loadStreamingFile loads file metadata for streaming without loading full content
func (w *WebUI) loadStreamingFile(descriptorCID string, password string) (*StreamingFile, error) {
	// Load descriptor (same logic as downloadFile)
	var descriptor *descriptors.Descriptor
	var loadErr error
	
	if password != "" {
		encStore, encErr := descriptors.NewEncryptedStoreWithPassword(w.storageManager, password)
		if encErr == nil {
			descriptor, loadErr = encStore.Load(descriptorCID)
			if loadErr != nil {
				descriptor = nil
			}
		}
	}
	
	if descriptor == nil {
		store, storeErr := descriptors.NewStore(w.storageManager)
		if storeErr != nil {
			return nil, storeErr
		}

		var err error
		descriptor, err = store.Load(descriptorCID)
		if err != nil {
			if password != "" {
				return nil, fmt.Errorf("failed to load descriptor (wrong password or not encrypted): %w", err)
			}
			return nil, err
		}
	}

	// Calculate total file size
	var totalSize int64
	for range descriptor.Blocks {
		totalSize += int64(descriptor.BlockSize)
	}

	return &StreamingFile{
		descriptor: descriptor,
		blocks:     nil, // Blocks will be loaded on-demand
		size:       totalSize,
		filename:   descriptor.Filename,
	}, nil
}

// parseRangeHeader parses HTTP Range header
func (w *WebUI) parseRangeHeader(rangeHeader string, fileSize int64) (*RangeRequest, error) {
	if !strings.HasPrefix(rangeHeader, "bytes=") {
		return nil, fmt.Errorf("unsupported range unit")
	}
	
	rangeSpec := strings.TrimPrefix(rangeHeader, "bytes=")
	parts := strings.Split(rangeSpec, "-")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid range format")
	}
	
	var start, end int64
	var err error
	
	if parts[0] == "" {
		// Suffix range: -500 (last 500 bytes)
		if parts[1] == "" {
			return nil, fmt.Errorf("invalid range format")
		}
		suffix, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return nil, err
		}
		start = fileSize - suffix
		if start < 0 {
			start = 0
		}
		end = fileSize - 1
	} else {
		// Normal range: 0-499 or 500-
		start, err = strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			return nil, err
		}
		
		if parts[1] == "" {
			// Open-ended range: 500-
			end = fileSize - 1
		} else {
			end, err = strconv.ParseInt(parts[1], 10, 64)
			if err != nil {
				return nil, err
			}
		}
	}
	
	// Validate range
	if start < 0 || end >= fileSize || start > end {
		return nil, fmt.Errorf("invalid range")
	}
	
	return &RangeRequest{
		Start: start,
		End:   end,
		Size:  end - start + 1,
	}, nil
}

// serveRangeRequest serves a partial content response for range requests
func (w *WebUI) serveRangeRequest(rw http.ResponseWriter, r *http.Request, streamFile *StreamingFile, contentType, rangeHeader string) {
	rangeReq, err := w.parseRangeHeader(rangeHeader, streamFile.size)
	if err != nil {
		http.Error(rw, "Invalid range request", http.StatusRequestedRangeNotSatisfiable)
		return
	}
	
	// Set partial content headers
	rw.Header().Set("Content-Type", contentType)
	rw.Header().Set("Accept-Ranges", "bytes")
	rw.Header().Set("Content-Length", strconv.FormatInt(rangeReq.Size, 10))
	rw.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", rangeReq.Start, rangeReq.End, streamFile.size))
	rw.WriteHeader(http.StatusPartialContent)
	
	// Stream the requested range
	err = w.streamFileRange(rw, streamFile, rangeReq.Start, rangeReq.End)
	if err != nil {
		log.Printf("Error streaming range: %v", err)
	}
}

// serveFullStreamingFile serves the complete file progressively
func (w *WebUI) serveFullStreamingFile(rw http.ResponseWriter, r *http.Request, streamFile *StreamingFile, contentType string) {
	// Set headers for full streaming response
	rw.Header().Set("Content-Type", contentType)
	rw.Header().Set("Accept-Ranges", "bytes")
	rw.Header().Set("Content-Length", strconv.FormatInt(streamFile.size, 10))
	
	// Stream the complete file
	err := w.streamFileRange(rw, streamFile, 0, streamFile.size-1)
	if err != nil {
		log.Printf("Error streaming file: %v", err)
	}
}

// streamFileRange streams a byte range from the file by reconstructing blocks on-demand
func (w *WebUI) streamFileRange(writer io.Writer, streamFile *StreamingFile, startByte, endByte int64) error {
	blockSize := int64(streamFile.descriptor.BlockSize)
	
	// Calculate which blocks we need (for optimization, not currently used)
	_ = int(startByte / blockSize)
	_ = int(endByte / blockSize)
	
	var currentPos int64 = 0
	
	for blockIdx := 0; blockIdx < len(streamFile.descriptor.Blocks); blockIdx++ {
		blockEndPos := currentPos + blockSize
		
		// Skip blocks that are completely before our range
		if blockEndPos <= startByte {
			currentPos = blockEndPos
			continue
		}
		
		// Stop if we're past our range
		if currentPos > endByte {
			break
		}
		
		// Retrieve and reconstruct this block
		originalBlock, err := w.reconstructBlock(streamFile.descriptor, blockIdx)
		if err != nil {
			return fmt.Errorf("failed to reconstruct block %d: %w", blockIdx, err)
		}
		
		// Calculate what portion of this block to write
		blockStart := int64(0)
		if currentPos < startByte {
			blockStart = startByte - currentPos
		}
		
		blockEnd := blockSize
		if blockEndPos > endByte+1 {
			blockEnd = blockSize - (blockEndPos-endByte-1)
		}
		
		// Write the relevant portion of the block
		if blockStart < blockEnd {
			_, err = writer.Write(originalBlock.Data[blockStart:blockEnd])
			if err != nil {
				return err
			}
		}
		
		currentPos = blockEndPos
	}
	
	return nil
}

// reconstructBlock retrieves and reconstructs a single block by index
func (w *WebUI) reconstructBlock(descriptor *descriptors.Descriptor, blockIdx int) (*blocks.Block, error) {
	blockPair := descriptor.Blocks[blockIdx]
	
	// Get data block
	address := &storage.BlockAddress{ID: blockPair.DataCID}
	dataBlock, err := w.storageManager.Get(context.Background(), address)
	if err != nil {
		return nil, err
	}
	
	// Get first randomizer block
	rand1Address := &storage.BlockAddress{ID: blockPair.RandomizerCID1}
	randomizer1Block, err := w.storageManager.Get(context.Background(), rand1Address)
	if err != nil {
		return nil, err
	}
	
	// Get second randomizer block (3-tuple format)
	rand2Address := &storage.BlockAddress{ID: blockPair.RandomizerCID2}
	randomizer2Block, err := w.storageManager.Get(context.Background(), rand2Address)
	if err != nil {
		return nil, err
	}
	
	// XOR to reconstruct original block
	originalBlock, err := dataBlock.XOR(randomizer1Block, randomizer2Block)
	if err != nil {
		return nil, err
	}
	
	return originalBlock, nil
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