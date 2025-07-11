package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

// Simple announcement structure for the web UI
type SimpleAnnouncement struct {
	Descriptor string   `json:"descriptor"`
	TopicHash  string   `json:"topicHash"`
	Topic      string   `json:"topic,omitempty"`
	Category   string   `json:"category"`
	SizeClass  string   `json:"sizeClass"`
	Tags       []string `json:"tags"`
	Timestamp  int64    `json:"timestamp"`
	Expiry     int64    `json:"expiry"`
}

// Simple topic structure
type SimpleTopic struct {
	Name             string   `json:"name"`
	Path             string   `json:"path"`
	Hash             string   `json:"hash"`
	Children         []string `json:"children"`
	AnnouncementCount int     `json:"announcementCount"`
}

// Server represents the announcement web UI server
type Server struct {
	// Mock data storage
	announcements []SimpleAnnouncement
	topics        map[string]SimpleTopic
	subscriptions []string
	
	// WebSocket management
	wsUpgrader websocket.Upgrader
	wsClients  map[*websocket.Conn]chan interface{}
	wsMutex    sync.RWMutex
	
	// Sync
	dataMutex sync.RWMutex
}

// API response types
type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

var (
	port     = flag.Int("port", 8080, "Port to listen on")
	ipfsAPI  = flag.String("ipfs", "http://127.0.0.1:5001", "IPFS API endpoint")
	dataDir  = flag.String("data", "./announce-data", "Data directory")
	debug    = flag.Bool("debug", false, "Enable debug logging")
)

var templates *template.Template

func main() {
	flag.Parse()

	// Load templates with correct path
	var err error
	templates, err = template.ParseGlob("templates/*.html")
	if err != nil {
		// Try alternative path
		templates, err = template.ParseGlob("cmd/announce-webui/templates/*.html")
		if err != nil {
			log.Fatalf("Failed to load templates: %v", err)
		}
	}

	// Create server with mock data
	server := &Server{
		announcements: generateMockAnnouncements(),
		topics:        generateMockTopics(),
		subscriptions: []string{"content/books", "software/tools"},
		wsUpgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins for development
			},
		},
		wsClients: make(map[*websocket.Conn]chan interface{}),
	}

	// Set up routes
	router := mux.NewRouter()
	
	// Web pages
	router.HandleFunc("/", server.handleIndex).Methods("GET")
	router.HandleFunc("/topics", server.handleTopics).Methods("GET")
	router.HandleFunc("/search", server.handleSearch).Methods("GET")
	
	// API endpoints
	api := router.PathPrefix("/api").Subrouter()
	api.HandleFunc("/announcements", server.handleGetAnnouncements).Methods("GET")
	api.HandleFunc("/topics", server.handleGetTopics).Methods("GET")
	api.HandleFunc("/topics/{path:.*}/subscribe", server.handleSubscribe).Methods("POST")
	api.HandleFunc("/topics/{path:.*}/unsubscribe", server.handleUnsubscribe).Methods("POST")
	api.HandleFunc("/subscriptions", server.handleGetSubscriptions).Methods("GET")
	api.HandleFunc("/search", server.handleSearchAPI).Methods("POST")
	api.HandleFunc("/stats", server.handleGetStats).Methods("GET")
	api.HandleFunc("/ws", server.handleWebSocket)
	
	// Static files
	router.PathPrefix("/static/").Handler(http.StripPrefix("/static/", 
		http.FileServer(http.Dir("static"))))
	// Try alternative path
	router.PathPrefix("/static/").Handler(http.StripPrefix("/static/", 
		http.FileServer(http.Dir("cmd/announce-webui/static"))))

	// Start mock announcement generator
	go server.generateAnnouncementsPeriodically()

	log.Printf("Starting web UI on port %d", *port)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", *port), router); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// Page handlers
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if err := templates.ExecuteTemplate(w, "index.html", map[string]interface{}{
		"Title": "NoiseFS Announcements",
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleTopics(w http.ResponseWriter, r *http.Request) {
	if err := templates.ExecuteTemplate(w, "topics.html", map[string]interface{}{
		"Title": "Topics - NoiseFS",
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	if err := templates.ExecuteTemplate(w, "search.html", map[string]interface{}{
		"Title": "Search - NoiseFS",
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// API handlers
func (s *Server) handleGetAnnouncements(w http.ResponseWriter, r *http.Request) {
	s.dataMutex.RLock()
	defer s.dataMutex.RUnlock()

	// Filter by topic if specified
	topic := r.URL.Query().Get("topic")
	
	var filtered []SimpleAnnouncement
	if topic != "" {
		for _, ann := range s.announcements {
			if ann.Topic == topic {
				filtered = append(filtered, ann)
			}
		}
	} else {
		filtered = s.announcements
	}

	s.sendJSON(w, APIResponse{
		Success: true,
		Data:    filtered,
	})
}

func (s *Server) handleGetTopics(w http.ResponseWriter, r *http.Request) {
	s.dataMutex.RLock()
	defer s.dataMutex.RUnlock()

	parent := r.URL.Query().Get("parent")
	
	var topics []SimpleTopic
	for _, topic := range s.topics {
		// Simple parent filtering
		if parent == "" && topic.Path != "" && len(topic.Path) > 0 && !containsSlash(topic.Path) {
			topics = append(topics, topic)
		} else if parent != "" && isDirectChild(parent, topic.Path) {
			topics = append(topics, topic)
		}
	}

	s.sendJSON(w, APIResponse{
		Success: true,
		Data:    topics,
	})
}

func (s *Server) handleSubscribe(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	path := vars["path"]

	s.dataMutex.Lock()
	defer s.dataMutex.Unlock()

	// Check if already subscribed
	for _, sub := range s.subscriptions {
		if sub == path {
			s.sendJSON(w, APIResponse{
				Success: false,
				Error:   "Already subscribed",
			})
			return
		}
	}

	s.subscriptions = append(s.subscriptions, path)
	
	s.sendJSON(w, APIResponse{
		Success: true,
	})
}

func (s *Server) handleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	path := vars["path"]

	s.dataMutex.Lock()
	defer s.dataMutex.Unlock()

	for i, sub := range s.subscriptions {
		if sub == path {
			s.subscriptions = append(s.subscriptions[:i], s.subscriptions[i+1:]...)
			s.sendJSON(w, APIResponse{
				Success: true,
			})
			return
		}
	}

	s.sendJSON(w, APIResponse{
		Success: false,
		Error:   "Not subscribed",
	})
}

func (s *Server) handleGetSubscriptions(w http.ResponseWriter, r *http.Request) {
	s.dataMutex.RLock()
	defer s.dataMutex.RUnlock()

	s.sendJSON(w, APIResponse{
		Success: true,
		Data:    s.subscriptions,
	})
}

func (s *Server) handleSearchAPI(w http.ResponseWriter, r *http.Request) {
	var searchParams map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&searchParams); err != nil {
		s.sendJSON(w, APIResponse{
			Success: false,
			Error:   "Invalid search parameters",
		})
		return
	}

	s.dataMutex.RLock()
	defer s.dataMutex.RUnlock()

	// Simple search implementation
	var results []SimpleAnnouncement
	for _, ann := range s.announcements {
		// Add some basic filtering logic here
		results = append(results, ann)
	}

	s.sendJSON(w, APIResponse{
		Success: true,
		Data:    results,
	})
}

func (s *Server) handleGetStats(w http.ResponseWriter, r *http.Request) {
	s.dataMutex.RLock()
	defer s.dataMutex.RUnlock()

	stats := map[string]interface{}{
		"totalAnnouncements":   len(s.announcements),
		"activeSubscriptions": len(s.subscriptions),
	}

	s.sendJSON(w, APIResponse{
		Success: true,
		Data:    stats,
	})
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	// Create channel for this client
	clientChan := make(chan interface{}, 10)
	
	s.wsMutex.Lock()
	s.wsClients[conn] = clientChan
	s.wsMutex.Unlock()

	defer func() {
		s.wsMutex.Lock()
		delete(s.wsClients, conn)
		s.wsMutex.Unlock()
		close(clientChan)
	}()

	// Send messages to client
	for msg := range clientChan {
		if err := conn.WriteJSON(msg); err != nil {
			log.Printf("WebSocket write error: %v", err)
			break
		}
	}
}

// Helper methods
func (s *Server) sendJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("JSON encoding error: %v", err)
	}
}

func (s *Server) broadcastToWebSockets(msgType string, data interface{}) {
	s.wsMutex.RLock()
	defer s.wsMutex.RUnlock()

	msg := map[string]interface{}{
		"type": msgType,
		"data": data,
	}

	for _, clientChan := range s.wsClients {
		select {
		case clientChan <- msg:
		default:
			// Client channel full, skip
		}
	}
}

// Mock data generators
func generateMockAnnouncements() []SimpleAnnouncement {
	return []SimpleAnnouncement{
		{
			Descriptor: "QmExample1234567890abcdef",
			TopicHash:  "abc123",
			Topic:      "content/books",
			Category:   "document",
			SizeClass:  "medium",
			Tags:       []string{"fiction", "classic", "english"},
			Timestamp:  time.Now().Unix() - 3600,
			Expiry:     time.Now().Unix() + 86400,
		},
		{
			Descriptor: "QmExample2345678901bcdefg",
			TopicHash:  "def456",
			Topic:      "software/tools",
			Category:   "software",
			SizeClass:  "small",
			Tags:       []string{"utility", "opensource", "linux"},
			Timestamp:  time.Now().Unix() - 1800,
			Expiry:     time.Now().Unix() + 172800,
		},
	}
}

func generateMockTopics() map[string]SimpleTopic {
	return map[string]SimpleTopic{
		"content": {
			Name:              "Content",
			Path:              "content",
			Hash:              "hash1",
			Children:          []string{"books", "audio", "video"},
			AnnouncementCount: 150,
		},
		"content/books": {
			Name:              "Books",
			Path:              "content/books",
			Hash:              "hash2",
			Children:          []string{},
			AnnouncementCount: 75,
		},
		"software": {
			Name:              "Software",
			Path:              "software",
			Hash:              "hash3",
			Children:          []string{"tools", "games"},
			AnnouncementCount: 200,
		},
		"software/tools": {
			Name:              "Tools",
			Path:              "software/tools",
			Hash:              "hash4",
			Children:          []string{},
			AnnouncementCount: 120,
		},
	}
}

func (s *Server) generateAnnouncementsPeriodically() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		// Generate a new mock announcement
		newAnn := SimpleAnnouncement{
			Descriptor: fmt.Sprintf("Qm%d", time.Now().Unix()),
			TopicHash:  "xyz789",
			Topic:      "content/books",
			Category:   "document",
			SizeClass:  "small",
			Tags:       []string{"new", "test"},
			Timestamp:  time.Now().Unix(),
			Expiry:     time.Now().Unix() + 86400,
		}

		s.dataMutex.Lock()
		s.announcements = append([]SimpleAnnouncement{newAnn}, s.announcements...)
		if len(s.announcements) > 100 {
			s.announcements = s.announcements[:100]
		}
		s.dataMutex.Unlock()

		// Broadcast to WebSocket clients
		s.broadcastToWebSockets("announcement", newAnn)
	}
}

// Helper functions
func containsSlash(s string) bool {
	for _, c := range s {
		if c == '/' {
			return true
		}
	}
	return false
}

func isDirectChild(parent, child string) bool {
	if parent == "" {
		return !containsSlash(child)
	}
	if len(child) <= len(parent) || child[:len(parent)] != parent {
		return false
	}
	remainder := child[len(parent):]
	if remainder[0] == '/' {
		remainder = remainder[1:]
	}
	return !containsSlash(remainder)
}