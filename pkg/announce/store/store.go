package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/announce"
)

// Store provides local storage for announcements
type Store struct {
	dataDir string
	
	// In-memory index
	byTopic      map[string][]*StoredAnnouncement
	byDescriptor map[string][]*StoredAnnouncement
	byTimestamp  []*StoredAnnouncement
	
	// Synchronization
	mu sync.RWMutex
	
	// Configuration
	maxAge       time.Duration
	maxSize      int
	cleanupInterval time.Duration
	
	// Control
	stopCleanup chan struct{}
	wg          sync.WaitGroup
}

// StoredAnnouncement wraps an announcement with metadata
type StoredAnnouncement struct {
	*announce.Announcement
	ReceivedAt time.Time `json:"received_at"`
	Source     string    `json:"source"` // "dht" or "pubsub"
}

// StoreConfig holds configuration for the store
type StoreConfig struct {
	DataDir         string
	MaxAge          time.Duration // Maximum age of stored announcements
	MaxSize         int           // Maximum number of announcements
	CleanupInterval time.Duration // How often to run cleanup
}

// DefaultStoreConfig returns default store configuration
func DefaultStoreConfig(dataDir string) StoreConfig {
	return StoreConfig{
		DataDir:         dataDir,
		MaxAge:          7 * 24 * time.Hour, // 1 week
		MaxSize:         10000,
		CleanupInterval: 1 * time.Hour,
	}
}

// NewStore creates a new announcement store
func NewStore(config StoreConfig) (*Store, error) {
	// Create data directory
	if err := os.MkdirAll(config.DataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}
	
	store := &Store{
		dataDir:         config.DataDir,
		byTopic:         make(map[string][]*StoredAnnouncement),
		byDescriptor:    make(map[string][]*StoredAnnouncement),
		byTimestamp:     make([]*StoredAnnouncement, 0),
		maxAge:          config.MaxAge,
		maxSize:         config.MaxSize,
		cleanupInterval: config.CleanupInterval,
		stopCleanup:     make(chan struct{}),
	}
	
	// Load existing announcements
	if err := store.loadFromDisk(); err != nil {
		return nil, fmt.Errorf("failed to load announcements: %w", err)
	}
	
	// Start cleanup routine
	store.wg.Add(1)
	go store.cleanupLoop()
	
	return store, nil
}

// Add adds an announcement to the store
func (s *Store) Add(announcement *announce.Announcement, source string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// Check if we already have this announcement
	if s.hasAnnouncement(announcement) {
		return nil // Already stored
	}
	
	// Create stored announcement
	stored := &StoredAnnouncement{
		Announcement: announcement,
		ReceivedAt:   time.Now(),
		Source:       source,
	}
	
	// Add to indices
	s.byTopic[announcement.TopicHash] = append(s.byTopic[announcement.TopicHash], stored)
	s.byDescriptor[announcement.Descriptor] = append(s.byDescriptor[announcement.Descriptor], stored)
	s.byTimestamp = append(s.byTimestamp, stored)
	
	// Check size limit
	if len(s.byTimestamp) > s.maxSize {
		// Remove oldest
		oldest := s.byTimestamp[0]
		s.removeFromIndices(oldest)
		s.byTimestamp = s.byTimestamp[1:]
	}
	
	// Save to disk
	if err := s.saveToDisk(stored); err != nil {
		return fmt.Errorf("failed to save announcement: %w", err)
	}
	
	return nil
}

// GetByTopic returns announcements for a topic hash
func (s *Store) GetByTopic(topicHash string) ([]*StoredAnnouncement, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	announcements := s.byTopic[topicHash]
	
	// Filter out expired
	valid := make([]*StoredAnnouncement, 0, len(announcements))
	for _, ann := range announcements {
		if !ann.IsExpired() {
			valid = append(valid, ann)
		}
	}
	
	return valid, nil
}

// GetByDescriptor returns announcements for a descriptor
func (s *Store) GetByDescriptor(descriptor string) ([]*StoredAnnouncement, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	announcements := s.byDescriptor[descriptor]
	
	// Filter out expired
	valid := make([]*StoredAnnouncement, 0, len(announcements))
	for _, ann := range announcements {
		if !ann.IsExpired() {
			valid = append(valid, ann)
		}
	}
	
	return valid, nil
}

// GetRecent returns recent announcements
func (s *Store) GetRecent(since time.Time, limit int) ([]*StoredAnnouncement, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	recent := make([]*StoredAnnouncement, 0)
	
	// Iterate from newest to oldest
	for i := len(s.byTimestamp) - 1; i >= 0 && len(recent) < limit; i-- {
		ann := s.byTimestamp[i]
		if ann.ReceivedAt.Before(since) {
			break
		}
		if !ann.IsExpired() {
			recent = append(recent, ann)
		}
	}
	
	return recent, nil
}

// Search searches announcements by tag bloom filter
func (s *Store) Search(tags []string, limit int) ([]*StoredAnnouncement, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	results := make([]*StoredAnnouncement, 0)
	
	// Check recent announcements
	for i := len(s.byTimestamp) - 1; i >= 0 && len(results) < limit; i-- {
		ann := s.byTimestamp[i]
		if ann.IsExpired() {
			continue
		}
		
		// Check tag match
		if ann.TagBloom != "" {
			matches, _, err := announce.MatchesTags(ann.TagBloom, tags)
			if err == nil && matches {
				results = append(results, ann)
			}
		}
	}
	
	return results, nil
}

// GetAll returns all non-expired announcements
func (s *Store) GetAll() ([]*StoredAnnouncement, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	all := make([]*StoredAnnouncement, 0, len(s.byTimestamp))
	
	for _, ann := range s.byTimestamp {
		if !ann.IsExpired() {
			all = append(all, ann)
		}
	}
	
	return all, nil
}

// Close closes the store
func (s *Store) Close() error {
	close(s.stopCleanup)
	s.wg.Wait()
	return nil
}

// Helper methods

// hasAnnouncement checks if we already have this announcement
func (s *Store) hasAnnouncement(ann *announce.Announcement) bool {
	// Check by descriptor and nonce
	for _, stored := range s.byDescriptor[ann.Descriptor] {
		if stored.Nonce == ann.Nonce {
			return true
		}
	}
	return false
}

// removeFromIndices removes an announcement from all indices
func (s *Store) removeFromIndices(stored *StoredAnnouncement) {
	// Remove from topic index
	s.removeFromSlice(&s.byTopic[stored.TopicHash], stored)
	
	// Remove from descriptor index
	s.removeFromSlice(&s.byDescriptor[stored.Descriptor], stored)
}

// removeFromSlice removes an item from a slice
func (s *Store) removeFromSlice(slice *[]*StoredAnnouncement, item *StoredAnnouncement) {
	for i, v := range *slice {
		if v == item {
			*slice = append((*slice)[:i], (*slice)[i+1:]...)
			break
		}
	}
}

// cleanupLoop periodically cleans up old announcements
func (s *Store) cleanupLoop() {
	defer s.wg.Done()
	
	ticker := time.NewTicker(s.cleanupInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-s.stopCleanup:
			return
		case <-ticker.C:
			s.cleanup()
		}
	}
}

// cleanup removes expired and old announcements
func (s *Store) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	cutoff := time.Now().Add(-s.maxAge)
	kept := make([]*StoredAnnouncement, 0, len(s.byTimestamp))
	
	for _, ann := range s.byTimestamp {
		// Remove if expired or too old
		if ann.IsExpired() || ann.ReceivedAt.Before(cutoff) {
			s.removeFromIndices(ann)
			s.deleteFromDisk(ann)
		} else {
			kept = append(kept, ann)
		}
	}
	
	s.byTimestamp = kept
}

// Persistence methods

// saveToDisk saves an announcement to disk
func (s *Store) saveToDisk(stored *StoredAnnouncement) error {
	// Use descriptor + nonce as filename
	filename := fmt.Sprintf("%s_%s.json", stored.Descriptor[:8], stored.Nonce)
	path := filepath.Join(s.dataDir, filename)
	
	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(path, data, 0644)
}

// loadFromDisk loads announcements from disk
func (s *Store) loadFromDisk() error {
	entries, err := os.ReadDir(s.dataDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No data yet
		}
		return err
	}
	
	for _, entry := range entries {
		if entry.IsDir() || !isAnnouncementFile(entry.Name()) {
			continue
		}
		
		path := filepath.Join(s.dataDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue // Skip failed files
		}
		
		var stored StoredAnnouncement
		if err := json.Unmarshal(data, &stored); err != nil {
			continue // Skip invalid files
		}
		
		// Add to indices
		s.byTopic[stored.TopicHash] = append(s.byTopic[stored.TopicHash], &stored)
		s.byDescriptor[stored.Descriptor] = append(s.byDescriptor[stored.Descriptor], &stored)
		s.byTimestamp = append(s.byTimestamp, &stored)
	}
	
	return nil
}

// deleteFromDisk removes an announcement from disk
func (s *Store) deleteFromDisk(stored *StoredAnnouncement) {
	filename := fmt.Sprintf("%s_%s.json", stored.Descriptor[:8], stored.Nonce)
	path := filepath.Join(s.dataDir, filename)
	os.Remove(path) // Ignore errors
}

// isAnnouncementFile checks if a filename is an announcement file
func isAnnouncementFile(name string) bool {
	return filepath.Ext(name) == ".json"
}

// GetStats returns store statistics
func (s *Store) GetStats() (total int, byTopic map[string]int, expired int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	total = len(s.byTimestamp)
	byTopic = make(map[string]int)
	
	for topic, anns := range s.byTopic {
		byTopic[topic] = len(anns)
	}
	
	for _, ann := range s.byTimestamp {
		if ann.IsExpired() {
			expired++
		}
	}
	
	return
}