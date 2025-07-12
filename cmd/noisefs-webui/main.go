package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math/big"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/announce"
	"github.com/TheEntropyCollective/noisefs/pkg/announce/config"
	"github.com/TheEntropyCollective/noisefs/pkg/announce/dht"
	"github.com/TheEntropyCollective/noisefs/pkg/announce/pubsub"
	"github.com/TheEntropyCollective/noisefs/pkg/announce/security"
	"github.com/TheEntropyCollective/noisefs/pkg/announce/store"
	noisefs "github.com/TheEntropyCollective/noisefs/pkg/core/client"
	"github.com/TheEntropyCollective/noisefs/pkg/core/descriptors"
	noisefsConfig "github.com/TheEntropyCollective/noisefs/pkg/infrastructure/config"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/validation"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/ipfs"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	shell "github.com/ipfs/go-ipfs-api"
)

// UnifiedWebUI combines file management and announcement discovery
type UnifiedWebUI struct {
	// File management components
	ipfsClient    *ipfs.Client
	noisefsClient *noisefs.Client
	cache         cache.Cache
	config        *noisefsConfig.Config
	validator     *validation.Validator
	rateLimiter   *validation.RateLimiter
	
	// Announcement components
	store            *store.Store
	dhtSubscriber    *dht.Subscriber
	pubsubSubscriber *pubsub.RealtimeSubscriber
	dhtPublisher     *dht.Publisher
	pubsubPublisher  *pubsub.RealtimePublisher
	hierarchy        *announce.TopicHierarchy
	search           *announce.SearchEngine
	securityMgr      *security.Manager
	
	// WebSocket management
	wsUpgrader websocket.Upgrader
	wsClients  map[*websocket.Conn]chan interface{}
	wsMutex    sync.RWMutex
	
	// Subscriptions
	subscriptions *config.Subscriptions
	subMutex      sync.RWMutex
}

// Response types
type UploadResponse struct {
	Success       bool     `json:"success"`
	DescriptorCID string   `json:"descriptor_cid"`
	Filename      string   `json:"filename"`
	Size          int64    `json:"size"`
	Tags          []string `json:"tags,omitempty"`
	Error         string   `json:"error,omitempty"`
}

type DownloadInfo struct {
	Filename      string `json:"filename"`
	Size          int64  `json:"size"`
	ContentType   string `json:"content_type"`
	DescriptorCID string `json:"descriptor_cid"`
}

type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// Announcement-related types
type AnnouncementView struct {
	ID          string    `json:"id"`
	Descriptor  string    `json:"descriptor"`
	Topic       string    `json:"topic,omitempty"`
	TopicHash   string    `json:"topicHash"`
	Tags        []string  `json:"tags"`
	Category    string    `json:"category"`
	SizeClass   string    `json:"sizeClass"`
	Timestamp   time.Time `json:"timestamp"`
	TTL         int64     `json:"ttl"`
	Expiry      time.Time `json:"expiry"`
	Source      string    `json:"source"`
}

type TopicView struct {
	Path              string            `json:"path"`
	Name              string            `json:"name"`
	Hash              string            `json:"hash"`
	Parent            string            `json:"parent,omitempty"`
	Children          []string          `json:"children"`
	Metadata          map[string]string `json:"metadata,omitempty"`
	Subscribed        bool              `json:"subscribed"`
	AnnouncementCount int               `json:"announcementCount"`
}

type StatsView struct {
	TotalAnnouncements int            `json:"totalAnnouncements"`
	ByTopic           map[string]int `json:"byTopic"`
	ByCategory        map[string]int `json:"byCategory"`
	BySizeClass       map[string]int `json:"bySizeClass"`
	RecentCount       int            `json:"recentCount"`
	ExpiredCount      int            `json:"expiredCount"`
	ActiveSubs        int            `json:"activeSubscriptions"`
}

// storeAdapter adapts store.Store to announce.AnnouncementStore interface
type storeAdapter struct {
	store *store.Store
}

func (sa *storeAdapter) GetByID(id string) (*announce.Announcement, error) {
	// Parse ID (format: descriptor-nonce)
	parts := strings.Split(id, "-")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid announcement ID: %s", id)
	}
	descriptor := parts[0]
	nonce := parts[1]
	
	// Get by descriptor and find matching nonce
	storedAnns, err := sa.store.GetByDescriptor(descriptor)
	if err != nil {
		return nil, err
	}
	
	for _, stored := range storedAnns {
		if stored.Nonce == nonce {
			return stored.Announcement, nil
		}
	}
	
	return nil, fmt.Errorf("announcement not found: %s", id)
}

func (sa *storeAdapter) GetAll() ([]*announce.Announcement, error) {
	storedAnns, err := sa.store.GetAll()
	if err != nil {
		return nil, err
	}
	
	anns := make([]*announce.Announcement, len(storedAnns))
	for i, stored := range storedAnns {
		anns[i] = stored.Announcement
	}
	
	return anns, nil
}

func (sa *storeAdapter) GetByTopic(topicHash string) ([]*announce.Announcement, error) {
	storedAnns, err := sa.store.GetByTopic(topicHash)
	if err != nil {
		return nil, err
	}
	
	anns := make([]*announce.Announcement, len(storedAnns))
	for i, stored := range storedAnns {
		anns[i] = stored.Announcement
	}
	
	return anns, nil
}

func (sa *storeAdapter) GetRecent(since time.Time, limit int) ([]*announce.Announcement, error) {
	storedAnns, err := sa.store.GetRecent(since, limit)
	if err != nil {
		return nil, err
	}
	
	anns := make([]*announce.Announcement, len(storedAnns))
	for i, stored := range storedAnns {
		anns[i] = stored.Announcement
	}
	
	return anns, nil
}

func main() {
	// Parse command line flags
	var (
		configFile   = flag.String("config", "", "Path to NoiseFS configuration file")
		addr         = flag.String("addr", ":8080", "HTTP server address")
		ipfsAPI      = flag.String("ipfs", "http://127.0.0.1:5001", "IPFS API endpoint")
		dataDir      = flag.String("data", "./webui-data", "Data directory")
		pollInterval = flag.Duration("poll", 30*time.Second, "DHT poll interval")
		enableTLS    = flag.Bool("tls", false, "Enable HTTPS with self-signed certificate")
		certFile     = flag.String("cert", "", "TLS certificate file (optional)")
		keyFile      = flag.String("key", "", "TLS key file (optional)")
	)
	flag.Parse()

	// Load configuration
	cfg, err := noisefsConfig.LoadConfig(*configFile)
	if err != nil {
		log.Printf("Using default configuration: %v", err)
		cfg = noisefsConfig.DefaultConfig()
		cfg.IPFS.APIEndpoint = *ipfsAPI
	}

	// Create IPFS client
	ipfsClient, err := ipfs.NewClient(cfg.IPFS.APIEndpoint)
	if err != nil {
		log.Fatalf("Failed to connect to IPFS: %v", err)
	}

	// Create cache and NoiseFS client
	blockCache := cache.NewMemoryCache(cfg.Cache.BlockCacheSize)
	noisefsClient, err := noisefs.NewClient(ipfsClient, blockCache)
	if err != nil {
		log.Fatalf("Failed to create NoiseFS client: %v", err)
	}

	// Create announcement store
	announcementStore, err := store.NewStore(store.StoreConfig{
		DataDir:         *dataDir,
		MaxAge:          7 * 24 * time.Hour,
		MaxSize:         10000,
		CleanupInterval: 1 * time.Hour,
	})
	if err != nil {
		log.Fatalf("Failed to create announcement store: %v", err)
	}

	// Create topic hierarchy
	hierarchy := announce.NewTopicHierarchy()
	
	// Try to load topics from file first
	if err := loadTopicsFromFile(hierarchy, "cmd/noisefs-webui/topics.json"); err != nil {
		log.Printf("Loading topics from file failed, using defaults: %v", err)
		loadDefaultHierarchy(hierarchy)
	}

	// Create search engine with store adapter
	searchEngine := announce.NewSearchEngine(&storeAdapter{store: announcementStore}, hierarchy)

	// Create security manager
	securityMgr := security.NewManager(&security.Config{
		ValidationConfig:  announce.DefaultValidationConfig(),
		RateLimitConfig:   announce.DefaultRateLimitConfig(),
		SpamConfig:        announce.DefaultSpamConfig(),
		ReputationConfig:  announce.DefaultReputationConfig(),
		SpamThreshold:     70,
		TrustRequired:     false,
	})

	// Create IPFS shell
	ipfsShell := shell.NewShell(*ipfsAPI)

	// Create subscribers
	dhtSubscriber, err := dht.NewSubscriber(dht.SubscriberConfig{
		IPFSClient:   ipfsClient,
		IPFSShell:    ipfsShell,
		PollInterval: *pollInterval,
	})
	if err != nil {
		log.Fatalf("Failed to create DHT subscriber: %v", err)
	}

	pubsubSubscriber, err := pubsub.NewRealtimeSubscriber(ipfsShell)
	if err != nil {
		log.Fatalf("Failed to create PubSub subscriber: %v", err)
	}

	// Create publishers
	dhtPublisher, err := dht.NewPublisher(dht.PublisherConfig{
		IPFSClient:  ipfsClient,
		IPFSShell:   ipfsShell,
		PublishRate: 5 * time.Minute,
	})
	if err != nil {
		log.Fatalf("Failed to create DHT publisher: %v", err)
	}

	pubsubPublisher, err := pubsub.NewRealtimePublisher(ipfsShell)
	if err != nil {
		log.Fatalf("Failed to create PubSub publisher: %v", err)
	}

	// Create input validator
	validator := validation.NewValidator()
	validator.SetMaxFileSize(100 * 1024 * 1024) // 100MB limit
	
	// Create rate limiter
	rateLimitConfig := validation.DefaultRateLimitConfig()
	rateLimiter := validation.NewRateLimiter(rateLimitConfig)

	// Create unified web UI
	webui := &UnifiedWebUI{
		// File management
		ipfsClient:    ipfsClient,
		noisefsClient: noisefsClient,
		cache:         blockCache,
		config:        cfg,
		validator:     validator,
		rateLimiter:   rateLimiter,
		
		// Announcements
		store:            announcementStore,
		dhtSubscriber:    dhtSubscriber,
		pubsubSubscriber: pubsubSubscriber,
		dhtPublisher:     dhtPublisher,
		pubsubPublisher:  pubsubPublisher,
		hierarchy:        hierarchy,
		search:           searchEngine,
		securityMgr:      securityMgr,
		
		// WebSocket
		wsUpgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins for development
			},
		},
		wsClients:     make(map[*websocket.Conn]chan interface{}),
		subscriptions: config.NewSubscriptions(),
	}

	// Load saved subscriptions
	if err := webui.loadSubscriptions(); err != nil {
		log.Printf("Warning: Failed to load subscriptions: %v", err)
	}

	// Start subscribers
	dhtSubscriber.Start()
	defer dhtSubscriber.Stop()

	// Setup routes
	router := mux.NewRouter()

	// Static files
	router.PathPrefix("/static/").Handler(
		http.StripPrefix("/static/", http.FileServer(http.Dir("cmd/noisefs-webui/static"))),
	)

	// Page routes
	router.HandleFunc("/", webui.handleIndex).Methods("GET")
	router.HandleFunc("/disclaimer", webui.handleDisclaimer).Methods("GET")
	router.HandleFunc("/upload", webui.handleUploadPage).Methods("GET")
	router.HandleFunc("/download", webui.handleDownloadPage).Methods("GET")
	router.HandleFunc("/browse", webui.handleBrowsePage).Methods("GET")
	router.HandleFunc("/dashboard", webui.handleDashboard).Methods("GET")
	router.HandleFunc("/topics", webui.handleTopicsPage).Methods("GET")
	router.HandleFunc("/search", webui.handleSearchPage).Methods("GET")

	// File API routes
	api := router.PathPrefix("/api").Subrouter()
	api.HandleFunc("/upload", webui.handleUpload).Methods("POST")
	api.HandleFunc("/download/{cid}", webui.handleDownload).Methods("GET")
	api.HandleFunc("/stream/{cid}", webui.handleStream).Methods("GET")
	api.HandleFunc("/info/{cid}", webui.handleInfo).Methods("GET")
	api.HandleFunc("/announce", webui.handleAnnounce).Methods("POST")
	
	// Announcement API routes
	api.HandleFunc("/announcements", webui.handleGetAnnouncements).Methods("GET")
	api.HandleFunc("/announcements/search", webui.handleSearchAnnouncements).Methods("POST")
	api.HandleFunc("/topics", webui.handleGetTopics).Methods("GET")
	api.HandleFunc("/topics/{topic}/subscribe", webui.handleSubscribe).Methods("POST")
	api.HandleFunc("/topics/{topic}/unsubscribe", webui.handleUnsubscribe).Methods("POST")
	api.HandleFunc("/subscriptions", webui.handleGetSubscriptions).Methods("GET")
	api.HandleFunc("/stats", webui.handleGetStats).Methods("GET")
	api.HandleFunc("/metrics", webui.handleMetrics).Methods("GET")
	api.HandleFunc("/ws", webui.handleWebSocket)

	// Add disclaimer notice
	fmt.Printf("\n========================================\n")
	fmt.Printf("⚠️  LEGAL NOTICE: This software is for legitimate use only.\n")
	fmt.Printf("   By using NoiseFS, you agree to comply with all applicable laws.\n")
	fmt.Printf("   See /disclaimer for full terms of use.\n")
	fmt.Printf("========================================\n\n")
	
	// Start server
	fmt.Printf("NoiseFS Unified Web UI running at http://localhost%s\n", *addr)
	
	if *enableTLS {
		var tlsConfig *tls.Config
		
		if *certFile != "" && *keyFile != "" {
			// Use provided certificate
			cert, err := tls.LoadX509KeyPair(*certFile, *keyFile)
			if err != nil {
				log.Fatalf("Failed to load TLS certificates: %v", err)
			}
			tlsConfig = &tls.Config{Certificates: []tls.Certificate{cert}}
		} else {
			// Generate self-signed certificate
			cert, err := generateSelfSignedCert()
			if err != nil {
				log.Fatalf("Failed to generate self-signed certificate: %v", err)
			}
			tlsConfig = &tls.Config{Certificates: []tls.Certificate{cert}}
		}
		
		server := &http.Server{
			Addr:      *addr,
			Handler:   router,
			TLSConfig: tlsConfig,
		}
		
		fmt.Printf("HTTPS enabled (visit https://localhost%s)\n", *addr)
		log.Fatal(server.ListenAndServeTLS("", ""))
	} else {
		log.Fatal(http.ListenAndServe(*addr, router))
	}
}

// Page handlers

func (w *UnifiedWebUI) handleIndex(wr http.ResponseWriter, r *http.Request) {
	http.ServeFile(wr, r, "cmd/noisefs-webui/templates/index.html")
}

func (w *UnifiedWebUI) handleDisclaimer(wr http.ResponseWriter, r *http.Request) {
	http.ServeFile(wr, r, "cmd/noisefs-webui/templates/disclaimer.html")
}

func (w *UnifiedWebUI) handleUploadPage(wr http.ResponseWriter, r *http.Request) {
	http.ServeFile(wr, r, "cmd/noisefs-webui/templates/upload.html")
}

func (w *UnifiedWebUI) handleDownloadPage(wr http.ResponseWriter, r *http.Request) {
	http.ServeFile(wr, r, "cmd/noisefs-webui/templates/download.html")
}

func (w *UnifiedWebUI) handleBrowsePage(wr http.ResponseWriter, r *http.Request) {
	http.ServeFile(wr, r, "cmd/noisefs-webui/templates/browse.html")
}

func (w *UnifiedWebUI) handleDashboard(wr http.ResponseWriter, r *http.Request) {
	http.ServeFile(wr, r, "cmd/noisefs-webui/templates/dashboard.html")
}

func (w *UnifiedWebUI) handleTopicsPage(wr http.ResponseWriter, r *http.Request) {
	http.ServeFile(wr, r, "cmd/noisefs-webui/templates/topics.html")
}

func (w *UnifiedWebUI) handleSearchPage(wr http.ResponseWriter, r *http.Request) {
	http.ServeFile(wr, r, "cmd/noisefs-webui/templates/search.html")
}

// File management handlers

func (w *UnifiedWebUI) handleUpload(wr http.ResponseWriter, r *http.Request) {
	// Check rate limit
	if err := w.rateLimiter.CheckLimit(r); err != nil {
		sendError(wr, err, http.StatusTooManyRequests)
		return
	}
	defer w.rateLimiter.ReleaseRequest(r)

	// Parse multipart form
	err := r.ParseMultipartForm(100 << 20) // 100MB max
	if err != nil {
		sendError(wr, err, http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		sendError(wr, err, http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Validate file
	if err := w.validator.ValidateFilename(header.Filename); err != nil {
		sendError(wr, err, http.StatusBadRequest)
		return
	}
	if err := w.validator.ValidateFileSize(header.Size); err != nil {
		sendError(wr, err, http.StatusBadRequest)
		return
	}

	// Get optional metadata
	topic := r.FormValue("topic")
	tagsStr := r.FormValue("tags")
	ttlStr := r.FormValue("ttl")
	
	// Parse tags
	var tags []string
	if tagsStr != "" {
		tags = strings.Split(tagsStr, ",")
		for i := range tags {
			tags[i] = strings.TrimSpace(tags[i])
		}
	}

	// Create a progress tracker
	progressUpdates := make(chan string, 10)
	go func() {
		for update := range progressUpdates {
			log.Printf("Upload progress: %s", update)
		}
	}()
	
	// Upload file using the client's proper implementation with progress
	descriptorCID, err := w.noisefsClient.UploadWithProgress(file, header.Filename, func(stage string, current, total int) {
		percent := 0
		if total > 0 {
			percent = (current * 100) / total
		}
		select {
		case progressUpdates <- fmt.Sprintf("%s: %d%%", stage, percent):
		default:
		}
	})
	close(progressUpdates)
	
	if err != nil {
		sendError(wr, err, http.StatusInternalServerError)
		return
	}

	// Optionally announce the file
	if topic != "" {
		ttl := int64(86400) // 24 hours default
		if ttlStr != "" {
			if parsedTTL, err := strconv.ParseInt(ttlStr, 10, 64); err == nil {
				ttl = parsedTTL
			}
		}
		
		announcement := announce.NewAnnouncement(descriptorCID, announce.HashTopic(topic))
		announcement.Category = categorizeFile(header.Filename)
		announcement.SizeClass = announce.GetSizeClass(header.Size)
		announcement.TTL = ttl
		
		// Add tags to bloom filter
		if len(tags) > 0 {
			bloom := announce.NewBloomFilter(announce.DefaultBloomParams())
			for _, tag := range tags {
				bloom.Add(normalizeTag(tag))
			}
			announcement.TagBloom = bloom.Encode()
		}
		
		// Publish announcement
		ctx := context.Background()
		if err := w.dhtPublisher.Publish(ctx, announcement); err != nil {
			log.Printf("Failed to publish to DHT: %v", err)
		}
		if err := w.pubsubPublisher.Publish(ctx, announcement); err != nil {
			log.Printf("Failed to publish to PubSub: %v", err)
		}
		
		// Store locally
		w.store.Add(announcement, "upload")
		
		// Broadcast via WebSocket
		w.broadcastAnnouncement(announcement)
	}

	response := UploadResponse{
		Success:       true,
		DescriptorCID: descriptorCID,
		Filename:      header.Filename,
		Size:          header.Size,
		Tags:          tags,
	}

	sendJSON(wr, response)
}

func (w *UnifiedWebUI) handleDownload(wr http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	descriptorCID := vars["cid"]

	if err := w.validator.ValidateCID(descriptorCID); err != nil {
		sendError(wr, err, http.StatusBadRequest)
		return
	}

	// First, try to load as a NoiseFS descriptor
	_, err := w.loadDescriptor(descriptorCID)
	if err == nil {
		// It's a valid NoiseFS descriptor, proceed with normal download
		// Create a progress tracker
		progressUpdates := make(chan string, 10)
		go func() {
			for update := range progressUpdates {
				log.Printf("Download progress: %s", update)
			}
		}()
		
		// Download file using the client's proper implementation with progress
		data, filename, err := w.noisefsClient.DownloadWithMetadataAndProgress(descriptorCID, func(stage string, current, total int) {
			percent := 0
			if total > 0 {
				percent = (current * 100) / total
			}
			select {
			case progressUpdates <- fmt.Sprintf("%s: %d%%", stage, percent):
			default:
			}
		})
		close(progressUpdates)
		
		if err != nil {
			sendError(wr, err, http.StatusNotFound)
			return
		}

		// Set headers
		wr.Header().Set("Content-Type", "application/octet-stream")
		wr.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
		wr.Header().Set("Content-Length", strconv.Itoa(len(data)))

		// Write data
		if _, err := wr.Write(data); err != nil {
			log.Printf("Download error: %v", err)
		}
	} else {
		// Not a NoiseFS descriptor, try direct IPFS download
		log.Printf("Not a NoiseFS descriptor, attempting direct IPFS download: %v", err)
		
		// Download directly from IPFS
		reader, err := w.ipfsClient.Cat(descriptorCID)
		if err != nil {
			sendError(wr, fmt.Errorf("failed to download file: %w", err), http.StatusNotFound)
			return
		}
		defer reader.Close()

		// Read data from reader
		data, err := ioutil.ReadAll(reader)
		if err != nil {
			sendError(wr, fmt.Errorf("failed to read file: %w", err), http.StatusInternalServerError)
			return
		}

		// Generate filename based on CID and detected content type
		filename := fmt.Sprintf("file_%s", descriptorCID[:8])
		contentType := "application/octet-stream"
		
		// Try to detect content type from data
		if len(data) > 512 {
			detectedType := http.DetectContentType(data[:512])
			log.Printf("Download - Detected content type: %s for CID: %s", detectedType, descriptorCID)
			if detectedType != "application/octet-stream" {
				contentType = detectedType
			}
			
			// Also check for magic bytes for common formats
			if len(data) >= 12 {
				// Check for QuickTime/MOV format
				if string(data[4:12]) == "ftypqt  " || string(data[4:8]) == "ftyp" {
					contentType = "video/quicktime"
					filename += ".mov"
				} else if string(data[4:11]) == "ftypmp4" {
					contentType = "video/mp4"
					filename += ".mp4"
				}
			} else {
				// Fall back to content type based extensions
				switch contentType {
				case "video/mp4":
					filename += ".mp4"
				case "video/quicktime":
					filename += ".mov"
				case "image/jpeg":
					filename += ".jpg"
				case "image/png":
					filename += ".png"
				case "application/pdf":
					filename += ".pdf"
				case "text/plain":
					filename += ".txt"
				}
			}
		}

		// Set headers
		wr.Header().Set("Content-Type", contentType)
		wr.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
		wr.Header().Set("Content-Length", strconv.Itoa(len(data)))

		// Write data
		if _, err := wr.Write(data); err != nil {
			log.Printf("Download error: %v", err)
		}
	}
}

func (w *UnifiedWebUI) handleStream(wr http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cid := vars["cid"]

	if err := w.validator.ValidateCID(cid); err != nil {
		sendError(wr, err, http.StatusBadRequest)
		return
	}

	// Download file data  
	data, err := w.noisefsClient.Download(cid)
	if err != nil {
		sendError(wr, err, http.StatusNotFound)
		return
	}

	// Parse range header
	rangeHeader := r.Header.Get("Range")
	var start, end int64
	fileSize := int64(len(data))
	
	if rangeHeader != "" {
		if _, err := fmt.Sscanf(rangeHeader, "bytes=%d-%d", &start, &end); err != nil {
			// Try parsing single value range
			if _, err := fmt.Sscanf(rangeHeader, "bytes=%d-", &start); err != nil {
				sendError(wr, fmt.Errorf("invalid range header"), http.StatusBadRequest)
				return
			}
			end = fileSize - 1
		}
	} else {
		start = 0
		end = fileSize - 1
	}

	// Validate range
	if start < 0 || end >= fileSize || start > end {
		wr.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return
	}

	// Set headers for partial content
	contentType := "application/octet-stream"
	wr.Header().Set("Content-Type", contentType)
	wr.Header().Set("Accept-Ranges", "bytes")
	wr.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, fileSize))
	wr.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
	wr.WriteHeader(http.StatusPartialContent)

	// Write requested range
	if _, err := wr.Write(data[start:end+1]); err != nil {
		log.Printf("Streaming error: %v", err)
	}
}

func (w *UnifiedWebUI) handleInfo(wr http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	descriptorCID := vars["cid"]

	if err := w.validator.ValidateCID(descriptorCID); err != nil {
		sendError(wr, err, http.StatusBadRequest)
		return
	}

	// First, try to load as a NoiseFS descriptor
	descriptor, err := w.loadDescriptor(descriptorCID)
	if err == nil {
		// It's a valid NoiseFS descriptor
		// Determine content type from filename
		contentType := "application/octet-stream"
		if strings.HasSuffix(strings.ToLower(descriptor.Filename), ".txt") {
			contentType = "text/plain"
		} else if strings.HasSuffix(strings.ToLower(descriptor.Filename), ".pdf") {
			contentType = "application/pdf"
		} else if strings.HasSuffix(strings.ToLower(descriptor.Filename), ".jpg") || strings.HasSuffix(strings.ToLower(descriptor.Filename), ".jpeg") {
			contentType = "image/jpeg"
		} else if strings.HasSuffix(strings.ToLower(descriptor.Filename), ".png") {
			contentType = "image/png"
		} else if strings.HasSuffix(strings.ToLower(descriptor.Filename), ".mp4") {
			contentType = "video/mp4"
		} else if strings.HasSuffix(strings.ToLower(descriptor.Filename), ".mp3") {
			contentType = "audio/mpeg"
		}

		info := DownloadInfo{
			Filename:      descriptor.Filename,
			Size:          descriptor.FileSize,
			ContentType:   contentType,
			DescriptorCID: descriptorCID,
		}

		sendJSON(wr, APIResponse{Success: true, Data: info})
	} else {
		// Not a NoiseFS descriptor, get info about the raw IPFS file
		log.Printf("Not a NoiseFS descriptor, getting IPFS file info: %v", err)
		
		// Generate filename based on CID
		filename := fmt.Sprintf("file_%s", descriptorCID[:8])
		contentType := "application/octet-stream"
		fileSize := int64(0)
		
		// Try to get file size and detect content type by downloading first 512 bytes
		reader, err := w.ipfsClient.Cat(descriptorCID)
		if err == nil {
			defer reader.Close()
			
			// Read first 512 bytes for content type detection
			header := make([]byte, 512)
			n, err := reader.Read(header)
			if err == nil && n > 0 {
				detectedType := http.DetectContentType(header[:n])
				log.Printf("Detected content type: %s for CID: %s", detectedType, descriptorCID)
				if detectedType != "application/octet-stream" {
					contentType = detectedType
				}
				
				// Also check for magic bytes for common formats
				if n >= 12 {
					// Check for QuickTime/MOV format
					if string(header[4:12]) == "ftypqt  " || string(header[4:8]) == "ftyp" {
						contentType = "video/quicktime"
						filename += ".mov"
					} else if string(header[4:11]) == "ftypmp4" {
						contentType = "video/mp4" 
						filename += ".mp4"
					}
				} else {
					// Fall back to content type based extensions
					switch contentType {
					case "video/mp4":
						filename += ".mp4"
					case "video/quicktime":
						filename += ".mov"
					case "image/jpeg":
						filename += ".jpg"
					case "image/png":
						filename += ".png"
					case "application/pdf":
						filename += ".pdf"
					case "text/plain":
						filename += ".txt"
					}
				}
			}
			
			// Try to estimate file size (this is not exact for streaming)
			// For now, we'll set it to -1 to indicate unknown
			fileSize = -1
		}

		info := DownloadInfo{
			Filename:      filename,
			Size:          fileSize,
			ContentType:   contentType,
			DescriptorCID: descriptorCID,
		}

		sendJSON(wr, APIResponse{Success: true, Data: info})
	}
}

func (w *UnifiedWebUI) handleAnnounce(wr http.ResponseWriter, r *http.Request) {
	var req struct {
		DescriptorCID string   `json:"descriptor_cid"`
		Topic         string   `json:"topic"`
		Tags          []string `json:"tags"`
		TTL           int64    `json:"ttl"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(wr, err, http.StatusBadRequest)
		return
	}

	// Validate CID
	if err := w.validator.ValidateCID(req.DescriptorCID); err != nil {
		sendError(wr, err, http.StatusBadRequest)
		return
	}

	// Create announcement
	topicHash := announce.HashTopic(req.Topic)
	announcement := announce.NewAnnouncement(req.DescriptorCID, topicHash)
	announcement.Category = announce.CategoryOther // Default category
	announcement.SizeClass = announce.SizeClassMedium // Default size class
	
	if req.TTL > 0 {
		announcement.TTL = req.TTL
	}

	// Add tags to bloom filter
	if len(req.Tags) > 0 {
		bloom := announce.NewBloomFilter(announce.DefaultBloomParams())
		for _, tag := range req.Tags {
			bloom.Add(normalizeTag(tag))
		}
		announcement.TagBloom = bloom.Encode()
	}

	// Publish announcement
	ctx := context.Background()
	if err := w.dhtPublisher.Publish(ctx, announcement); err != nil {
		sendError(wr, fmt.Errorf("failed to publish to DHT: %w", err), http.StatusInternalServerError)
		return
	}
	
	if err := w.pubsubPublisher.Publish(ctx, announcement); err != nil {
		log.Printf("Failed to publish to PubSub: %v", err)
	}

	// Store locally
	w.store.Add(announcement, "announce")

	// Broadcast via WebSocket
	w.broadcastAnnouncement(announcement)

	sendJSON(wr, APIResponse{Success: true})
}

// Announcement handlers

func (w *UnifiedWebUI) handleGetAnnouncements(wr http.ResponseWriter, r *http.Request) {
	topic := r.URL.Query().Get("topic")
	limit := 100
	
	var storedAnnouncements []*store.StoredAnnouncement
	var err error
	
	if topic != "" {
		topicHash := announce.HashTopic(topic)
		storedAnnouncements, err = w.store.GetByTopic(topicHash)
	} else {
		storedAnnouncements, err = w.store.GetRecent(time.Now().Add(-24*time.Hour), limit)
	}
	
	if err != nil {
		sendError(wr, err, http.StatusInternalServerError)
		return
	}
	
	// Convert to view models
	views := make([]AnnouncementView, 0, len(storedAnnouncements))
	for _, stored := range storedAnnouncements {
		view := w.announcementToView(stored.Announcement)
		view.Source = stored.Source
		views = append(views, view)
	}
	
	sendJSON(wr, APIResponse{Success: true, Data: views})
}

func (w *UnifiedWebUI) handleSearchAnnouncements(wr http.ResponseWriter, r *http.Request) {
	var query announce.SearchQuery
	if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
		sendError(wr, err, http.StatusBadRequest)
		return
	}
	
	results, err := w.search.Search(query)
	if err != nil {
		sendError(wr, err, http.StatusInternalServerError)
		return
	}
	
	// Convert to view models
	views := make([]AnnouncementView, 0, len(results))
	for _, result := range results {
		view := w.announcementToView(result.Announcement)
		view.Tags = extractHighlightedTags(result.Highlights)
		views = append(views, view)
	}
	
	sendJSON(wr, APIResponse{Success: true, Data: views})
}

func (w *UnifiedWebUI) handleGetTopics(wr http.ResponseWriter, r *http.Request) {
	parent := r.URL.Query().Get("parent")
	
	var topics []*announce.TopicNode
	var err error
	if parent == "" {
		// Get root level topics
		if root, exists := w.hierarchy.GetTopic(""); exists {
			topics = make([]*announce.TopicNode, 0, len(root.Children))
			for _, child := range root.Children {
				topics = append(topics, child)
			}
		}
	} else {
		topics, err = w.hierarchy.GetChildren(parent)
		if err != nil {
			sendError(wr, err, http.StatusInternalServerError)
			return
		}
	}
	
	// Convert to view models
	views := make([]TopicView, 0, len(topics))
	for _, topic := range topics {
		view := w.topicToView(topic)
		views = append(views, view)
	}
	
	sendJSON(wr, APIResponse{Success: true, Data: views})
}

func (w *UnifiedWebUI) handleSubscribe(wr http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	topic := vars["topic"]
	
	// Create announcement handler
	handler := func(ann *announce.Announcement) error {
		// Validate with security manager
		if err := w.securityMgr.CheckAnnouncement(ann, "webui"); err != nil {
			log.Printf("Rejected announcement: %v", err)
			return nil // Don't propagate error
		}
		
		// Store announcement
		if err := w.store.Add(ann, "subscription"); err != nil {
			return err
		}
		
		// Broadcast to WebSocket clients
		w.broadcastAnnouncement(ann)
		
		return nil
	}
	
	// Subscribe to both DHT and PubSub
	if err := w.dhtSubscriber.Subscribe(topic, handler); err != nil {
		sendError(wr, err, http.StatusInternalServerError)
		return
	}
	
	if err := w.pubsubSubscriber.Subscribe(topic, handler); err != nil {
		// Rollback DHT subscription
		w.dhtSubscriber.Unsubscribe(topic)
		sendError(wr, err, http.StatusInternalServerError)
		return
	}
	
	// Save subscription
	w.saveSubscription(topic, true)
	
	sendJSON(wr, APIResponse{Success: true})
}

func (w *UnifiedWebUI) handleUnsubscribe(wr http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	topic := vars["topic"]
	
	// Unsubscribe from both
	w.dhtSubscriber.Unsubscribe(topic)
	w.pubsubSubscriber.Unsubscribe(topic)
	
	// Save subscription state
	w.saveSubscription(topic, false)
	
	sendJSON(wr, APIResponse{Success: true})
}

func (w *UnifiedWebUI) handleGetSubscriptions(wr http.ResponseWriter, r *http.Request) {
	w.subMutex.RLock()
	defer w.subMutex.RUnlock()
	
	activeSubs := []string{}
	for _, sub := range w.subscriptions.Subscriptions {
		if sub.Active {
			activeSubs = append(activeSubs, sub.Topic)
		}
	}
	
	sendJSON(wr, APIResponse{Success: true, Data: activeSubs})
}

func (w *UnifiedWebUI) handleGetStats(wr http.ResponseWriter, r *http.Request) {
	total, byTopic, expired := w.store.GetStats()
	
	// Get category and size class stats
	allAnnouncements, _ := w.store.GetAll()
	byCategory := make(map[string]int)
	bySizeClass := make(map[string]int)
	
	for _, ann := range allAnnouncements {
		byCategory[ann.Category]++
		bySizeClass[ann.SizeClass]++
	}
	
	// Count recent
	recent, _ := w.store.GetRecent(time.Now().Add(-24*time.Hour), 0)
	
	stats := StatsView{
		TotalAnnouncements: total,
		ByTopic:           byTopic,
		ByCategory:        byCategory,
		BySizeClass:       bySizeClass,
		RecentCount:       len(recent),
		ExpiredCount:      expired,
		ActiveSubs:        len(w.dhtSubscriber.GetSubscriptions()),
	}
	
	sendJSON(wr, APIResponse{Success: true, Data: stats})
}

// WebSocket handling

func (w *UnifiedWebUI) handleWebSocket(wr http.ResponseWriter, r *http.Request) {
	conn, err := w.wsUpgrader.Upgrade(wr, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	
	// Create client channel
	clientChan := make(chan interface{}, 100)
	
	w.wsMutex.Lock()
	w.wsClients[conn] = clientChan
	w.wsMutex.Unlock()
	
	defer func() {
		w.wsMutex.Lock()
		delete(w.wsClients, conn)
		w.wsMutex.Unlock()
		close(clientChan)
		conn.Close()
	}()
	
	// Send initial stats
	w.sendWebSocketStats(conn)
	
	// Handle outgoing messages
	go func() {
		for msg := range clientChan {
			if err := conn.WriteJSON(msg); err != nil {
				return
			}
		}
	}()
	
	// Handle incoming messages (ping/pong)
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

// Helper functions

func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		// Take the first IP in the chain
		parts := strings.Split(forwarded, ",")
		return strings.TrimSpace(parts[0])
	}
	
	// Check X-Real-IP header
	realIP := r.Header.Get("X-Real-IP")
	if realIP != "" {
		return realIP
	}
	
	// Fall back to RemoteAddr
	ip := r.RemoteAddr
	if colon := strings.LastIndex(ip, ":"); colon != -1 {
		ip = ip[:colon]
	}
	return ip
}

func categorizeFile(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".mp4", ".avi", ".mkv", ".mov", ".wmv", ".flv", ".webm":
		return announce.CategoryVideo
	case ".mp3", ".wav", ".flac", ".aac", ".ogg", ".wma", ".m4a":
		return announce.CategoryAudio
	case ".pdf", ".doc", ".docx", ".txt", ".odt", ".rtf":
		return announce.CategoryDocument
	case ".zip", ".rar", ".7z", ".tar", ".gz", ".bz2":
		return announce.CategoryData
	case ".exe", ".dmg", ".deb", ".rpm", ".apk", ".msi":
		return announce.CategorySoftware
	default:
		return announce.CategoryOther
	}
}

func normalizeTag(tag string) string {
	return strings.ToLower(strings.TrimSpace(tag))
}

func sendJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func sendError(w http.ResponseWriter, err error, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(APIResponse{
		Success: false,
		Error:   err.Error(),
	})
}

// Additional helper functions

func (w *UnifiedWebUI) broadcastAnnouncement(ann *announce.Announcement) {
	view := w.announcementToView(ann)
	message := map[string]interface{}{
		"type": "announcement",
		"data": view,
	}
	
	w.wsMutex.RLock()
	defer w.wsMutex.RUnlock()
	
	for _, clientChan := range w.wsClients {
		select {
		case clientChan <- message:
		default:
			// Client channel full, skip
		}
	}
}

func (w *UnifiedWebUI) sendWebSocketStats(conn *websocket.Conn) {
	total, _, _ := w.store.GetStats()
	
	message := map[string]interface{}{
		"type": "stats",
		"data": map[string]interface{}{
			"total":      total,
			"activeSubs": len(w.dhtSubscriber.GetSubscriptions()),
		},
	}
	
	conn.WriteJSON(message)
}

func (w *UnifiedWebUI) announcementToView(ann *announce.Announcement) AnnouncementView {
	// Try to extract tags from bloom filter (limited)
	tags := w.extractCommonTags(ann.TagBloom)
	
	// Try to reverse lookup topic (if in hierarchy)
	topic := w.reverseLookupTopic(ann.TopicHash)
	
	return AnnouncementView{
		ID:         ann.Descriptor + "-" + ann.Nonce,
		Descriptor: ann.Descriptor,
		Topic:      topic,
		TopicHash:  ann.TopicHash,
		Tags:       tags,
		Category:   ann.Category,
		SizeClass:  ann.SizeClass,
		Timestamp:  time.Unix(ann.Timestamp, 0),
		TTL:        ann.TTL,
		Expiry:     time.Unix(ann.Timestamp, 0).Add(time.Duration(ann.TTL) * time.Second),
		Source:     "network",
	}
}

func (w *UnifiedWebUI) topicToView(node *announce.TopicNode) TopicView {
	hash := announce.HashTopic(node.Path)
	children, _ := w.hierarchy.GetChildren(node.Path)
	childPaths := make([]string, len(children))
	for i, child := range children {
		childPaths[i] = child.Path
	}
	
	// Count announcements for this topic
	announcements, _ := w.store.GetByTopic(hash)
	
	// Check if subscribed
	subscribed := false
	w.subMutex.RLock()
	for _, sub := range w.subscriptions.Subscriptions {
		if sub.Topic == node.Path && sub.Active {
			subscribed = true
			break
		}
	}
	w.subMutex.RUnlock()
	
	// Extract name from path
	parts := strings.Split(node.Path, "/")
	name := parts[len(parts)-1]
	if name == "" && len(parts) > 1 {
		name = parts[len(parts)-2]
	}
	
	return TopicView{
		Path:              node.Path,
		Name:              name,
		Hash:              hash,
		Parent:            "", // TODO: get parent path from node
		Children:          childPaths,
		Metadata:          node.Metadata,
		Subscribed:        subscribed,
		AnnouncementCount: len(announcements),
	}
}

func (w *UnifiedWebUI) extractCommonTags(bloomStr string) []string {
	if bloomStr == "" {
		return []string{}
	}
	
	// Test common tags against bloom filter
	commonTags := []string{
		"res:720p", "res:1080p", "res:4k",
		"format:pdf", "format:epub",
		"lang:en", "lang:es",
		"type:video", "type:audio", "type:document",
	}
	
	bloom, err := announce.DecodeBloom(bloomStr)
	if err != nil {
		return []string{}
	}
	
	matches := []string{}
	for _, tag := range commonTags {
		normalizedTag := normalizeTag(tag)
		if bloom.Test(normalizedTag) {
			matches = append(matches, tag)
		}
	}
	
	return matches
}

func (w *UnifiedWebUI) reverseLookupTopic(topicHash string) string {
	// Try common topics
	commonTopics := []string{
		"content", "content/books", "content/documents",
		"content/media", "software", "software/opensource",
	}
	
	for _, topic := range commonTopics {
		if announce.HashTopic(topic) == topicHash {
			return topic
		}
	}
	
	return ""
}

func (w *UnifiedWebUI) loadSubscriptions() error {
	configDir := config.GetConfigDir()
	subs, err := config.LoadSubscriptions(configDir + "/subscriptions.json")
	if err != nil {
		return err
	}
	
	w.subMutex.Lock()
	w.subscriptions = subs
	w.subMutex.Unlock()
	
	// Activate subscriptions
	for _, sub := range subs.Subscriptions {
		if sub.Active {
			handler := func(ann *announce.Announcement) error {
				if err := w.securityMgr.CheckAnnouncement(ann, "webui"); err != nil {
					return nil
				}
				if err := w.store.Add(ann, "subscription"); err != nil {
					return err
				}
				w.broadcastAnnouncement(ann)
				return nil
			}
			
			w.dhtSubscriber.Subscribe(sub.Topic, handler)
			w.pubsubSubscriber.Subscribe(sub.Topic, handler)
		}
	}
	
	return nil
}

func (w *UnifiedWebUI) saveSubscription(topic string, active bool) {
	w.subMutex.Lock()
	defer w.subMutex.Unlock()
	
	// Update or add subscription
	found := false
	for i, sub := range w.subscriptions.Subscriptions {
		if sub.Topic == topic {
			w.subscriptions.Subscriptions[i].Active = active
			found = true
			break
		}
	}
	
	if !found && active {
		w.subscriptions.Add(config.Subscription{
			Topic:     topic,
			TopicHash: announce.HashTopic(topic),
			Active:    active,
		})
	}
	
	// Save to disk
	configDir := config.GetConfigDir()
	config.SaveSubscriptions(configDir+"/subscriptions.json", w.subscriptions)
}

// TopicConfig represents the structure of topics.json
type TopicConfig struct {
	Topics map[string]TopicNode `json:"topics"`
}

type TopicNode struct {
	Description string                `json:"description,omitempty"`
	Children    map[string]TopicNode  `json:"children,omitempty"`
}

// loadTopicsFromFile loads topic hierarchy from a JSON file
func loadTopicsFromFile(h *announce.TopicHierarchy, filename string) error {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}
	
	var config TopicConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return err
	}
	
	// Recursively add topics
	for name, node := range config.Topics {
		addTopicRecursive(h, name, node, "")
	}
	
	return nil
}

func addTopicRecursive(h *announce.TopicHierarchy, name string, node TopicNode, parentPath string) {
	// Build full path
	fullPath := name
	if parentPath != "" {
		fullPath = parentPath + "/" + name
	}
	
	// Add topic with metadata
	metadata := make(map[string]string)
	if node.Description != "" {
		metadata["description"] = node.Description
	}
	h.AddTopic(fullPath, metadata)
	
	// Add children recursively
	for childName, childNode := range node.Children {
		addTopicRecursive(h, childName, childNode, fullPath)
	}
}

func loadDefaultHierarchy(h *announce.TopicHierarchy) {
	// This function is now empty - all topics should be defined in topics.json
	// If you need to add topics programmatically, do it here
	log.Println("No topics.json found, starting with empty topic hierarchy")
}

func extractHighlightedTags(highlights map[string][]string) []string {
	if tags, ok := highlights["tags"]; ok {
		return tags
	}
	return []string{}
}

func (w *UnifiedWebUI) handleMetrics(wr http.ResponseWriter, r *http.Request) {
	metrics := w.noisefsClient.GetMetrics()
	
	response := struct {
		Metrics   interface{} `json:"metrics"`
		Timestamp time.Time   `json:"timestamp"`
	}{
		Metrics:   metrics,
		Timestamp: time.Now(),
	}
	
	sendJSON(wr, response)
}

// generateSelfSignedCert generates a self-signed certificate for HTTPS
func generateSelfSignedCert() (tls.Certificate, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"NoiseFS"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1)},
		DNSNames:              []string{"localhost"},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})

	return tls.X509KeyPair(certPEM, keyPEM)
}

// loadDescriptor loads a descriptor without downloading the file
func (w *UnifiedWebUI) loadDescriptor(descriptorCID string) (*descriptors.Descriptor, error) {
	// Create descriptor store
	descriptorStore, err := descriptors.NewStore(w.ipfsClient)
	if err != nil {
		return nil, fmt.Errorf("failed to create descriptor store: %w", err)
	}
	
	// Load descriptor
	descriptor, err := descriptorStore.Load(descriptorCID)
	if err != nil {
		return nil, fmt.Errorf("failed to load descriptor: %w", err)
	}
	
	return descriptor, nil
}

