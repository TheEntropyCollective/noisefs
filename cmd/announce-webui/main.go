package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/announce"
	"github.com/TheEntropyCollective/noisefs/pkg/announce/config"
	"github.com/TheEntropyCollective/noisefs/pkg/announce/dht"
	"github.com/TheEntropyCollective/noisefs/pkg/announce/pubsub"
	"github.com/TheEntropyCollective/noisefs/pkg/announce/security"
	"github.com/TheEntropyCollective/noisefs/pkg/announce/store"
	"github.com/TheEntropyCollective/noisefs/pkg/ipfs"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

// Server represents the announcement web UI server
type Server struct {
	store            *store.Store
	dhtSubscriber    *dht.Subscriber
	pubsubSubscriber *pubsub.RealtimeSubscriber
	hierarchy        *announce.TopicHierarchy
	search           *announce.SearchEngine
	securityMgr      *security.Manager
	ipfsClient       ipfs.Client
	
	// WebSocket management
	wsUpgrader websocket.Upgrader
	wsClients  map[*websocket.Conn]chan interface{}
	wsMutex    sync.RWMutex
	
	// Subscriptions
	subscriptions *config.SubscriptionList
	subMutex      sync.RWMutex
}

// API response types
type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

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
	Path        string            `json:"path"`
	Name        string            `json:"name"`
	Hash        string            `json:"hash"`
	Parent      string            `json:"parent,omitempty"`
	Children    []string          `json:"children"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Subscribed  bool              `json:"subscribed"`
	AnnouncementCount int         `json:"announcementCount"`
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

var (
	templates *template.Template
	startTime = time.Now()
)

func main() {
	var (
		addr         = flag.String("addr", ":8090", "HTTP server address")
		ipfsAPI      = flag.String("ipfs", "http://127.0.0.1:5001", "IPFS API endpoint")
		dataDir      = flag.String("data", "./announce-webui-data", "Data directory")
		pollInterval = flag.Duration("poll", 30*time.Second, "DHT poll interval")
	)
	flag.Parse()

	// Load templates
	var err error
	templates, err = template.ParseGlob("cmd/announce-webui/templates/*.html")
	if err != nil {
		log.Fatalf("Failed to load templates: %v", err)
	}

	// Initialize IPFS client
	ipfsClient, err := ipfs.NewClient(*ipfsAPI)
	if err != nil {
		log.Fatalf("Failed to create IPFS client: %v", err)
	}

	// Create announcement store
	announcementStore, err := store.NewStore(store.StoreConfig{
		DataDir:         *dataDir,
		MaxAge:          7 * 24 * time.Hour,
		MaxSize:         10000,
		CleanupInterval: 1 * time.Hour,
	})
	if err != nil {
		log.Fatalf("Failed to create store: %v", err)
	}

	// Create topic hierarchy
	hierarchy := announce.NewTopicHierarchy()
	loadDefaultHierarchy(hierarchy)

	// Create search engine
	searchEngine := announce.NewSearchEngine(announcementStore, hierarchy)

	// Create security manager
	securityMgr := security.NewManager(&security.Config{
		ValidationConfig:  announce.DefaultValidationConfig(),
		RateLimitConfig:   announce.DefaultRateLimitConfig(),
		SpamConfig:        announce.DefaultSpamConfig(),
		ReputationConfig:  announce.DefaultReputationConfig(),
		SpamThreshold:     70,
		TrustRequired:     false,
	})

	// Create subscribers
	dhtSubscriber, err := dht.NewSubscriber(dht.SubscriberConfig{
		IPFSClient:   ipfsClient,
		IPFSShell:    ipfsClient.Shell(),
		PollInterval: *pollInterval,
	})
	if err != nil {
		log.Fatalf("Failed to create DHT subscriber: %v", err)
	}

	pubsubSubscriber, err := pubsub.NewRealtimeSubscriber(ipfsClient.Shell())
	if err != nil {
		log.Fatalf("Failed to create PubSub subscriber: %v", err)
	}

	// Create server
	server := &Server{
		store:            announcementStore,
		dhtSubscriber:    dhtSubscriber,
		pubsubSubscriber: pubsubSubscriber,
		hierarchy:        hierarchy,
		search:           searchEngine,
		securityMgr:      securityMgr,
		ipfsClient:       ipfsClient,
		wsUpgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins for development
			},
		},
		wsClients:     make(map[*websocket.Conn]chan interface{}),
		subscriptions: &config.SubscriptionList{},
	}

	// Load saved subscriptions
	if err := server.loadSubscriptions(); err != nil {
		log.Printf("Warning: Failed to load subscriptions: %v", err)
	}

	// Start subscribers
	dhtSubscriber.Start()
	pubsubSubscriber.Start()
	defer func() {
		dhtSubscriber.Stop()
		pubsubSubscriber.Stop()
	}()

	// Setup routes
	router := mux.NewRouter()

	// Static files
	router.PathPrefix("/static/").Handler(
		http.StripPrefix("/static/", http.FileServer(http.Dir("cmd/announce-webui/static"))),
	)

	// Page routes
	router.HandleFunc("/", server.handleIndex).Methods("GET")
	router.HandleFunc("/topics", server.handleTopicsPage).Methods("GET")
	router.HandleFunc("/search", server.handleSearchPage).Methods("GET")

	// API routes
	api := router.PathPrefix("/api").Subrouter()
	api.HandleFunc("/announcements", server.handleGetAnnouncements).Methods("GET")
	api.HandleFunc("/announcements/search", server.handleSearchAnnouncements).Methods("POST")
	api.HandleFunc("/topics", server.handleGetTopics).Methods("GET")
	api.HandleFunc("/topics/{topic}/subscribe", server.handleSubscribe).Methods("POST")
	api.HandleFunc("/topics/{topic}/unsubscribe", server.handleUnsubscribe).Methods("POST")
	api.HandleFunc("/subscriptions", server.handleGetSubscriptions).Methods("GET")
	api.HandleFunc("/stats", server.handleGetStats).Methods("GET")
	api.HandleFunc("/ws", server.handleWebSocket)

	// Start server
	fmt.Printf("NoiseFS Announcement Web UI running at http://localhost%s\n", *addr)
	log.Fatal(http.ListenAndServe(*addr, router))
}

// Page handlers

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Title string
	}{
		Title: "NoiseFS Announcements",
	}
	
	if err := templates.ExecuteTemplate(w, "index.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleTopicsPage(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Title string
	}{
		Title: "Topic Browser - NoiseFS",
	}
	
	if err := templates.ExecuteTemplate(w, "topics.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleSearchPage(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Title string
	}{
		Title: "Search - NoiseFS",
	}
	
	if err := templates.ExecuteTemplate(w, "search.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// API handlers

func (s *Server) handleGetAnnouncements(w http.ResponseWriter, r *http.Request) {
	topic := r.URL.Query().Get("topic")
	limit := 100
	
	var announcements []*announce.Announcement
	var err error
	
	if topic != "" {
		topicHash := announce.HashTopic(topic)
		announcements, err = s.store.GetByTopic(topicHash)
	} else {
		announcements, err = s.store.GetRecent(time.Now().Add(-24*time.Hour), limit)
	}
	
	if err != nil {
		sendError(w, err)
		return
	}
	
	// Convert to view models
	views := make([]AnnouncementView, 0, len(announcements))
	for _, ann := range announcements {
		view := s.announcementToView(ann)
		views = append(views, view)
	}
	
	sendJSON(w, APIResponse{Success: true, Data: views})
}

func (s *Server) handleSearchAnnouncements(w http.ResponseWriter, r *http.Request) {
	var query announce.SearchQuery
	if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
		sendError(w, err)
		return
	}
	
	results, err := s.search.Search(query)
	if err != nil {
		sendError(w, err)
		return
	}
	
	// Convert to view models
	views := make([]AnnouncementView, 0, len(results))
	for _, result := range results {
		view := s.announcementToView(result.Announcement)
		view.Tags = extractHighlightedTags(result.Highlights)
		views = append(views, view)
	}
	
	sendJSON(w, APIResponse{Success: true, Data: views})
}

func (s *Server) handleGetTopics(w http.ResponseWriter, r *http.Request) {
	parent := r.URL.Query().Get("parent")
	
	var topics []announce.TopicNode
	if parent == "" {
		topics = s.hierarchy.GetRoots()
	} else {
		topics = s.hierarchy.GetChildren(parent)
	}
	
	// Convert to view models
	views := make([]TopicView, 0, len(topics))
	for _, topic := range topics {
		view := s.topicToView(&topic)
		views = append(views, view)
	}
	
	sendJSON(w, APIResponse{Success: true, Data: views})
}

func (s *Server) handleSubscribe(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	topic := vars["topic"]
	
	// Create announcement handler
	handler := func(ann *announce.Announcement) error {
		// Validate with security manager
		if err := s.securityMgr.CheckAnnouncement(ann, "webui"); err != nil {
			log.Printf("Rejected announcement: %v", err)
			return nil // Don't propagate error
		}
		
		// Store announcement
		if err := s.store.Add(ann, "subscription"); err != nil {
			return err
		}
		
		// Broadcast to WebSocket clients
		s.broadcastAnnouncement(ann)
		
		return nil
	}
	
	// Subscribe to both DHT and PubSub
	if err := s.dhtSubscriber.Subscribe(topic, handler); err != nil {
		sendError(w, err)
		return
	}
	
	if err := s.pubsubSubscriber.Subscribe(topic, handler); err != nil {
		// Rollback DHT subscription
		s.dhtSubscriber.Unsubscribe(topic)
		sendError(w, err)
		return
	}
	
	// Save subscription
	s.saveSubscription(topic, true)
	
	sendJSON(w, APIResponse{Success: true})
}

func (s *Server) handleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	topic := vars["topic"]
	
	// Unsubscribe from both
	s.dhtSubscriber.Unsubscribe(topic)
	s.pubsubSubscriber.Unsubscribe(topic)
	
	// Save subscription state
	s.saveSubscription(topic, false)
	
	sendJSON(w, APIResponse{Success: true})
}

func (s *Server) handleGetSubscriptions(w http.ResponseWriter, r *http.Request) {
	s.subMutex.RLock()
	defer s.subMutex.RUnlock()
	
	activeSubs := []string{}
	for _, sub := range s.subscriptions.Subscriptions {
		if sub.Active {
			activeSubs = append(activeSubs, sub.Topic)
		}
	}
	
	sendJSON(w, APIResponse{Success: true, Data: activeSubs})
}

func (s *Server) handleGetStats(w http.ResponseWriter, r *http.Request) {
	total, byTopic, expired := s.store.GetStats()
	
	// Get category and size class stats
	allAnnouncements, _ := s.store.GetAll()
	byCategory := make(map[string]int)
	bySizeClass := make(map[string]int)
	
	for _, ann := range allAnnouncements {
		byCategory[ann.Category]++
		bySizeClass[ann.SizeClass]++
	}
	
	// Count recent
	recent, _ := s.store.GetRecent(time.Now().Add(-24*time.Hour), 0)
	
	stats := StatsView{
		TotalAnnouncements: total,
		ByTopic:           byTopic,
		ByCategory:        byCategory,
		BySizeClass:       bySizeClass,
		RecentCount:       len(recent),
		ExpiredCount:      expired,
		ActiveSubs:        len(s.dhtSubscriber.GetSubscriptions()),
	}
	
	sendJSON(w, APIResponse{Success: true, Data: stats})
}

// WebSocket handling

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	
	// Create client channel
	clientChan := make(chan interface{}, 100)
	
	s.wsMutex.Lock()
	s.wsClients[conn] = clientChan
	s.wsMutex.Unlock()
	
	defer func() {
		s.wsMutex.Lock()
		delete(s.wsClients, conn)
		s.wsMutex.Unlock()
		close(clientChan)
		conn.Close()
	}()
	
	// Send initial stats
	s.sendWebSocketStats(conn)
	
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

func (s *Server) broadcastAnnouncement(ann *announce.Announcement) {
	view := s.announcementToView(ann)
	message := map[string]interface{}{
		"type": "announcement",
		"data": view,
	}
	
	s.wsMutex.RLock()
	defer s.wsMutex.RUnlock()
	
	for _, clientChan := range s.wsClients {
		select {
		case clientChan <- message:
		default:
			// Client channel full, skip
		}
	}
}

func (s *Server) sendWebSocketStats(conn *websocket.Conn) {
	total, _, _ := s.store.GetStats()
	
	message := map[string]interface{}{
		"type": "stats",
		"data": map[string]interface{}{
			"total":       total,
			"activeSubs":  len(s.dhtSubscriber.GetSubscriptions()),
		},
	}
	
	conn.WriteJSON(message)
}

// Helper functions

func (s *Server) announcementToView(ann *announce.Announcement) AnnouncementView {
	// Try to extract tags from bloom filter (limited)
	tags := s.extractCommonTags(ann.TagBloom)
	
	// Try to reverse lookup topic (if in hierarchy)
	topic := s.reverseLookupTopic(ann.TopicHash)
	
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
		Expiry:     ann.GetExpiry(),
		Source:     "network",
	}
}

func (s *Server) topicToView(node *announce.TopicNode) TopicView {
	hash := announce.HashTopic(node.Path)
	children := s.hierarchy.GetChildren(node.Path)
	childPaths := make([]string, len(children))
	for i, child := range children {
		childPaths[i] = child.Path
	}
	
	// Count announcements for this topic
	announcements, _ := s.store.GetByTopic(hash)
	
	// Check if subscribed
	subscribed := false
	s.subMutex.RLock()
	for _, sub := range s.subscriptions.Subscriptions {
		if sub.Topic == node.Path && sub.Active {
			subscribed = true
			break
		}
	}
	s.subMutex.RUnlock()
	
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
		Parent:            node.Parent,
		Children:          childPaths,
		Metadata:          node.Metadata,
		Subscribed:        subscribed,
		AnnouncementCount: len(announcements),
	}
}

func (s *Server) extractCommonTags(bloomStr string) []string {
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
		if bloom.Test(announce.NormalizeTag(tag)) {
			matches = append(matches, tag)
		}
	}
	
	return matches
}

func (s *Server) reverseLookupTopic(topicHash string) string {
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

func (s *Server) loadSubscriptions() error {
	configDir := config.GetConfigDir()
	subs, err := config.LoadSubscriptions(configDir + "/subscriptions.json")
	if err != nil {
		return err
	}
	
	s.subMutex.Lock()
	s.subscriptions = subs
	s.subMutex.Unlock()
	
	// Activate subscriptions
	for _, sub := range subs.Subscriptions {
		if sub.Active {
			handler := func(ann *announce.Announcement) error {
				if err := s.securityMgr.CheckAnnouncement(ann, "webui"); err != nil {
					return nil
				}
				if err := s.store.Add(ann, "subscription"); err != nil {
					return err
				}
				s.broadcastAnnouncement(ann)
				return nil
			}
			
			s.dhtSubscriber.Subscribe(sub.Topic, handler)
			s.pubsubSubscriber.Subscribe(sub.Topic, handler)
		}
	}
	
	return nil
}

func (s *Server) saveSubscription(topic string, active bool) {
	s.subMutex.Lock()
	defer s.subMutex.Unlock()
	
	// Update or add subscription
	found := false
	for i, sub := range s.subscriptions.Subscriptions {
		if sub.Topic == topic {
			s.subscriptions.Subscriptions[i].Active = active
			found = true
			break
		}
	}
	
	if !found && active {
		s.subscriptions.Add(config.Subscription{
			Topic:     topic,
			TopicHash: announce.HashTopic(topic),
			Active:    active,
		})
	}
	
	// Save to disk
	configDir := config.GetConfigDir()
	config.SaveSubscriptions(configDir+"/subscriptions.json", s.subscriptions)
}

func loadDefaultHierarchy(h *announce.TopicHierarchy) {
	// Root categories
	h.AddTopic("content", map[string]string{"description": "All content"})
	h.AddTopic("software", map[string]string{"description": "Software and tools"})
	
	// Content subcategories
	h.AddTopic("content/books", map[string]string{"description": "Books and literature"})
	h.AddTopic("content/books/fiction", nil)
	h.AddTopic("content/books/technical", nil)
	h.AddTopic("content/books/public-domain", nil)
	
	h.AddTopic("content/documents", map[string]string{"description": "Documents and papers"})
	h.AddTopic("content/documents/research", nil)
	h.AddTopic("content/documents/government", nil)
	
	h.AddTopic("content/media", map[string]string{"description": "Media files"})
	h.AddTopic("content/media/documentaries", nil)
	h.AddTopic("content/media/educational", nil)
	h.AddTopic("content/media/public-domain", nil)
	
	// Software subcategories
	h.AddTopic("software/opensource", map[string]string{"description": "Open source projects"})
	h.AddTopic("software/tools", map[string]string{"description": "Software tools"})
	h.AddTopic("software/linux", map[string]string{"description": "Linux distributions"})
}

func extractHighlightedTags(highlights map[string][]string) []string {
	if tags, ok := highlights["tags"]; ok {
		return tags
	}
	return []string{}
}

func sendJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func sendError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	json.NewEncoder(w).Encode(APIResponse{
		Success: false,
		Error:   err.Error(),
	})
}