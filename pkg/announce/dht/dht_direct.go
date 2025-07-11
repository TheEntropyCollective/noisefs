package dht

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/announce"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/ipfs"
)

// DirectDHT provides enhanced DHT operations using IPFS API
type DirectDHT struct {
	ipfsClient    *ipfs.Client
	
	// Local cache of announcements
	cache         map[string][]*announce.Announcement
	cacheMutex    sync.RWMutex
	cacheExpiry   time.Duration
	
	// Metrics
	putCount      int64
	getCount      int64
	cacheHits     int64
	cacheMisses   int64
	metricsMutex  sync.RWMutex
}

// DirectDHTConfig configuration for direct DHT
type DirectDHTConfig struct {
	IPFSClient   *ipfs.Client
	CacheExpiry  time.Duration
}

// NewDirectDHT creates a new direct DHT implementation
func NewDirectDHT(config DirectDHTConfig) (*DirectDHT, error) {
	if config.IPFSClient == nil {
		return nil, errors.New("IPFS client is required")
	}
	
	cacheExpiry := config.CacheExpiry
	if cacheExpiry == 0 {
		cacheExpiry = 5 * time.Minute
	}
	
	dht := &DirectDHT{
		ipfsClient:  config.IPFSClient,
		cache:       make(map[string][]*announce.Announcement),
		cacheExpiry: cacheExpiry,
	}
	
	// Start cache cleanup routine
	go dht.cleanupLoop()
	
	return dht, nil
}

// PutAnnouncement stores an announcement using enhanced DHT pattern
func (d *DirectDHT) PutAnnouncement(ctx context.Context, announcement *announce.Announcement) error {
	// Validate announcement
	if err := announcement.Validate(); err != nil {
		return fmt.Errorf("invalid announcement: %w", err)
	}
	
	// Serialize announcement
	data, err := json.Marshal(announcement)
	if err != nil {
		return fmt.Errorf("failed to serialize: %w", err)
	}
	
	// Store in IPFS
	cid, err := d.ipfsClient.Add(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to store in IPFS: %w", err)
	}
	_ = cid // CID is stored for future DHT implementation
	
	// Create composite key for DHT-like storage
	compositeKey := fmt.Sprintf("noisefs-announce-%s-%d", announcement.TopicHash, announcement.Timestamp)
	
	// In a real DHT implementation, we would:
	// 1. Store the CID at the composite key in the DHT
	// 2. Maintain a local index for fast lookups
	// 3. Propagate to DHT peers
	
	// For now, we'll use the local cache and rely on IPFS for distribution
	_ = compositeKey // Mark as used
	
	// Update local cache
	d.updateCache(announcement)
	
	// Update metrics
	d.incrementPuts()
	
	return nil
}

// GetAnnouncements retrieves announcements for a topic
func (d *DirectDHT) GetAnnouncements(ctx context.Context, topicHash string, since time.Time) ([]*announce.Announcement, error) {
	// Check cache first
	if cached := d.getFromCache(topicHash, since); len(cached) > 0 {
		d.incrementCacheHits()
		return cached, nil
	}
	
	d.incrementCacheMisses()
	
	// In a real implementation, this would query the DHT network
	// For now, we return empty results as IPFS HTTP API doesn't expose DHT directly
	// The actual announcements are stored and can be retrieved if we know the CIDs
	
	d.incrementGets()
	return []*announce.Announcement{}, nil
}

// SearchByTags searches for announcements with specific tags
func (d *DirectDHT) SearchByTags(ctx context.Context, topicHash string, tags []string) ([]*announce.Announcement, error) {
	announcements, err := d.GetAnnouncements(ctx, topicHash, time.Now().Add(-24*time.Hour))
	if err != nil {
		return nil, err
	}
	
	// Filter by tags using bloom filter
	var filtered []*announce.Announcement
	for _, ann := range announcements {
		if ann.TagBloom != "" {
			// In a real implementation, we'd check tags against the bloom filter
			// For now, include all announcements with tag bloom filters
			filtered = append(filtered, ann)
		}
	}
	
	return filtered, nil
}

// GetMetrics returns DHT operation metrics
func (d *DirectDHT) GetMetrics() map[string]int64 {
	d.metricsMutex.RLock()
	defer d.metricsMutex.RUnlock()
	
	return map[string]int64{
		"puts":         d.putCount,
		"gets":         d.getCount,
		"cache_hits":   d.cacheHits,
		"cache_misses": d.cacheMisses,
		"cache_size":   int64(len(d.cache)),
	}
}

// Helper methods

// updateCache adds an announcement to the local cache
func (d *DirectDHT) updateCache(announcement *announce.Announcement) {
	d.cacheMutex.Lock()
	defer d.cacheMutex.Unlock()
	
	// Add to cache
	d.cache[announcement.TopicHash] = append(d.cache[announcement.TopicHash], announcement)
	
	// Limit cache size per topic
	if len(d.cache[announcement.TopicHash]) > 100 {
		d.cache[announcement.TopicHash] = d.cache[announcement.TopicHash][1:]
	}
}

// getFromCache retrieves announcements from cache
func (d *DirectDHT) getFromCache(topicHash string, since time.Time) []*announce.Announcement {
	d.cacheMutex.RLock()
	defer d.cacheMutex.RUnlock()
	
	cached, exists := d.cache[topicHash]
	if !exists {
		return nil
	}
	
	var result []*announce.Announcement
	sinceUnix := since.Unix()
	
	for _, ann := range cached {
		if ann.Timestamp >= sinceUnix && !ann.IsExpired() {
			result = append(result, ann)
		}
	}
	
	return result
}

// cleanupLoop periodically cleans expired entries from cache
func (d *DirectDHT) cleanupLoop() {
	ticker := time.NewTicker(d.cacheExpiry)
	defer ticker.Stop()
	
	for range ticker.C {
		d.cleanupCache()
	}
}

// cleanupCache removes expired announcements from cache
func (d *DirectDHT) cleanupCache() {
	d.cacheMutex.Lock()
	defer d.cacheMutex.Unlock()
	
	for topicHash, announcements := range d.cache {
		var valid []*announce.Announcement
		for _, ann := range announcements {
			if !ann.IsExpired() {
				valid = append(valid, ann)
			}
		}
		
		if len(valid) > 0 {
			d.cache[topicHash] = valid
		} else {
			delete(d.cache, topicHash)
		}
	}
}

// Metric tracking methods

func (d *DirectDHT) incrementPuts() {
	d.metricsMutex.Lock()
	defer d.metricsMutex.Unlock()
	d.putCount++
}

func (d *DirectDHT) incrementGets() {
	d.metricsMutex.Lock()
	defer d.metricsMutex.Unlock()
	d.getCount++
}

func (d *DirectDHT) incrementCacheHits() {
	d.metricsMutex.Lock()
	defer d.metricsMutex.Unlock()
	d.cacheHits++
}

func (d *DirectDHT) incrementCacheMisses() {
	d.metricsMutex.Lock()
	defer d.metricsMutex.Unlock()
	d.cacheMisses++
}

