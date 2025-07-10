package cache

import (
	"container/heap"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/logging"
)

// EvictionPolicy defines the interface for cache eviction policies
type EvictionPolicy interface {
	// OnAccess is called when a block is accessed
	OnAccess(cid string)
	
	// OnStore is called when a block is stored
	OnStore(cid string, block *blocks.Block)
	
	// OnRemove is called when a block is removed
	OnRemove(cid string)
	
	// SelectVictim selects a block for eviction
	SelectVictim() (string, bool)
	
	// Clear resets the eviction policy state
	Clear()
}

// LRUEvictionPolicy implements Least Recently Used eviction
type LRUEvictionPolicy struct {
	mu          sync.RWMutex
	accessOrder []string
	accessMap   map[string]int
}

// NewLRUEvictionPolicy creates a new LRU eviction policy
func NewLRUEvictionPolicy() *LRUEvictionPolicy {
	return &LRUEvictionPolicy{
		accessOrder: make([]string, 0),
		accessMap:   make(map[string]int),
	}
}

// OnAccess updates the access order for LRU
func (p *LRUEvictionPolicy) OnAccess(cid string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	// Remove from current position
	if index, exists := p.accessMap[cid]; exists {
		p.accessOrder = append(p.accessOrder[:index], p.accessOrder[index+1:]...)
		// Update indices for moved elements
		for i := index; i < len(p.accessOrder); i++ {
			p.accessMap[p.accessOrder[i]] = i
		}
	}
	
	// Add to end (most recently used)
	p.accessOrder = append(p.accessOrder, cid)
	p.accessMap[cid] = len(p.accessOrder) - 1
}

// OnStore handles block storage for LRU
func (p *LRUEvictionPolicy) OnStore(cid string, block *blocks.Block) {
	p.OnAccess(cid)
}

// OnRemove handles block removal for LRU
func (p *LRUEvictionPolicy) OnRemove(cid string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if index, exists := p.accessMap[cid]; exists {
		p.accessOrder = append(p.accessOrder[:index], p.accessOrder[index+1:]...)
		delete(p.accessMap, cid)
		
		// Update indices for moved elements
		for i := index; i < len(p.accessOrder); i++ {
			p.accessMap[p.accessOrder[i]] = i
		}
	}
}

// SelectVictim selects the least recently used block
func (p *LRUEvictionPolicy) SelectVictim() (string, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	if len(p.accessOrder) == 0 {
		return "", false
	}
	
	return p.accessOrder[0], true
}

// Clear resets the LRU state
func (p *LRUEvictionPolicy) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	p.accessOrder = make([]string, 0)
	p.accessMap = make(map[string]int)
}

// LFUEvictionPolicy implements Least Frequently Used eviction
type LFUEvictionPolicy struct {
	mu         sync.RWMutex
	frequency  map[string]int
	minFreq    int
	freqToCIDs map[int]map[string]bool
}

// NewLFUEvictionPolicy creates a new LFU eviction policy
func NewLFUEvictionPolicy() *LFUEvictionPolicy {
	return &LFUEvictionPolicy{
		frequency:  make(map[string]int),
		freqToCIDs: make(map[int]map[string]bool),
	}
}

// OnAccess updates the frequency for LFU
func (p *LFUEvictionPolicy) OnAccess(cid string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	oldFreq := p.frequency[cid]
	newFreq := oldFreq + 1
	
	// Remove from old frequency bucket
	if oldFreq > 0 {
		delete(p.freqToCIDs[oldFreq], cid)
		if len(p.freqToCIDs[oldFreq]) == 0 {
			delete(p.freqToCIDs, oldFreq)
		}
	}
	
	// Add to new frequency bucket
	p.frequency[cid] = newFreq
	if p.freqToCIDs[newFreq] == nil {
		p.freqToCIDs[newFreq] = make(map[string]bool)
	}
	p.freqToCIDs[newFreq][cid] = true
	
	// Update minimum frequency
	if oldFreq == p.minFreq && len(p.freqToCIDs[oldFreq]) == 0 {
		p.minFreq = newFreq
	}
}

// OnStore handles block storage for LFU
func (p *LFUEvictionPolicy) OnStore(cid string, block *blocks.Block) {
	p.OnAccess(cid)
}

// OnRemove handles block removal for LFU
func (p *LFUEvictionPolicy) OnRemove(cid string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if freq, exists := p.frequency[cid]; exists {
		delete(p.frequency, cid)
		delete(p.freqToCIDs[freq], cid)
		if len(p.freqToCIDs[freq]) == 0 {
			delete(p.freqToCIDs, freq)
		}
	}
}

// SelectVictim selects the least frequently used block
func (p *LFUEvictionPolicy) SelectVictim() (string, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	// Find minimum frequency with available blocks
	for freq := p.minFreq; freq <= p.minFreq+10; freq++ {
		if cids, exists := p.freqToCIDs[freq]; exists && len(cids) > 0 {
			// Return any block from this frequency
			for cid := range cids {
				return cid, true
			}
		}
	}
	
	return "", false
}

// Clear resets the LFU state
func (p *LFUEvictionPolicy) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	p.frequency = make(map[string]int)
	p.freqToCIDs = make(map[int]map[string]bool)
	p.minFreq = 0
}

// TTLEvictionPolicy implements Time-To-Live eviction
type TTLEvictionPolicy struct {
	mu        sync.RWMutex
	ttl       time.Duration
	timestamps map[string]time.Time
	heap      *TTLHeap
}

// TTLEntry represents an entry in the TTL heap
type TTLEntry struct {
	CID       string
	ExpiresAt time.Time
	Index     int
}

// TTLHeap implements a min-heap for TTL entries
type TTLHeap []*TTLEntry

func (h TTLHeap) Len() int           { return len(h) }
func (h TTLHeap) Less(i, j int) bool { return h[i].ExpiresAt.Before(h[j].ExpiresAt) }
func (h TTLHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].Index = i
	h[j].Index = j
}

func (h *TTLHeap) Push(x interface{}) {
	entry := x.(*TTLEntry)
	entry.Index = len(*h)
	*h = append(*h, entry)
}

func (h *TTLHeap) Pop() interface{} {
	old := *h
	n := len(old)
	entry := old[n-1]
	old[n-1] = nil
	entry.Index = -1
	*h = old[0 : n-1]
	return entry
}

// NewTTLEvictionPolicy creates a new TTL eviction policy
func NewTTLEvictionPolicy(ttl time.Duration) *TTLEvictionPolicy {
	return &TTLEvictionPolicy{
		ttl:        ttl,
		timestamps: make(map[string]time.Time),
		heap:       &TTLHeap{},
	}
}

// OnAccess updates the timestamp for TTL
func (p *TTLEvictionPolicy) OnAccess(cid string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	now := time.Now()
	p.timestamps[cid] = now
	
	// Add to heap for expiration tracking
	entry := &TTLEntry{
		CID:       cid,
		ExpiresAt: now.Add(p.ttl),
	}
	heap.Push(p.heap, entry)
}

// OnStore handles block storage for TTL
func (p *TTLEvictionPolicy) OnStore(cid string, block *blocks.Block) {
	p.OnAccess(cid)
}

// OnRemove handles block removal for TTL
func (p *TTLEvictionPolicy) OnRemove(cid string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	delete(p.timestamps, cid)
}

// SelectVictim selects an expired block
func (p *TTLEvictionPolicy) SelectVictim() (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	now := time.Now()
	
	// Clean up expired entries
	for p.heap.Len() > 0 {
		entry := (*p.heap)[0]
		if entry.ExpiresAt.After(now) {
			break
		}
		
		// Remove expired entry
		heap.Pop(p.heap)
		if _, exists := p.timestamps[entry.CID]; exists {
			return entry.CID, true
		}
	}
	
	return "", false
}

// Clear resets the TTL state
func (p *TTLEvictionPolicy) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	p.timestamps = make(map[string]time.Time)
	p.heap = &TTLHeap{}
}

// AdaptiveEvictionPolicyImpl combines multiple eviction policies
type AdaptiveEvictionPolicyImpl struct {
	mu         sync.RWMutex
	policies   []EvictionPolicy
	weights    []float64
	logger     *logging.Logger
	stats      map[string]*EvictionStats
}

// EvictionStats tracks eviction policy performance
type EvictionStats struct {
	Evictions   int64
	HitRate     float64
	LastUpdated time.Time
}

// NewAdaptiveEvictionPolicy creates a new adaptive eviction policy
func NewAdaptiveEvictionPolicy(logger *logging.Logger) *AdaptiveEvictionPolicyImpl {
	policies := []EvictionPolicy{
		NewLRUEvictionPolicy(),
		NewLFUEvictionPolicy(),
		NewTTLEvictionPolicy(30 * time.Minute),
	}
	
	return &AdaptiveEvictionPolicyImpl{
		policies: policies,
		weights:  []float64{0.4, 0.4, 0.2}, // Initial weights
		logger:   logger,
		stats:    make(map[string]*EvictionStats),
	}
}

// OnAccess forwards to all policies
func (p *AdaptiveEvictionPolicyImpl) OnAccess(cid string) {
	for _, policy := range p.policies {
		policy.OnAccess(cid)
	}
}

// OnStore forwards to all policies
func (p *AdaptiveEvictionPolicyImpl) OnStore(cid string, block *blocks.Block) {
	for _, policy := range p.policies {
		policy.OnStore(cid, block)
	}
}

// OnRemove forwards to all policies
func (p *AdaptiveEvictionPolicyImpl) OnRemove(cid string) {
	for _, policy := range p.policies {
		policy.OnRemove(cid)
	}
}

// SelectVictim selects a victim using weighted policy selection
func (p *AdaptiveEvictionPolicyImpl) SelectVictim() (string, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	// Simple weighted round-robin for now
	// In a real implementation, this would use more sophisticated adaptation
	for i, policy := range p.policies {
		if p.weights[i] > 0.1 { // Only consider policies with significant weight
			if victim, ok := policy.SelectVictim(); ok {
				p.logger.Debug("Selected victim using policy", map[string]interface{}{
					"policy": i,
					"victim": victim,
					"weight": p.weights[i],
				})
				return victim, true
			}
		}
	}
	
	return "", false
}

// Clear resets all policies
func (p *AdaptiveEvictionPolicyImpl) Clear() {
	for _, policy := range p.policies {
		policy.Clear()
	}
}

// EvictingCache implements a cache with pluggable eviction policies
type EvictingCache struct {
	underlying Cache
	policy     EvictionPolicy
	maxSize    int
	logger     *logging.Logger
	mu         sync.RWMutex
}

// NewEvictingCache creates a new cache with eviction policy
func NewEvictingCache(underlying Cache, policy EvictionPolicy, maxSize int, logger *logging.Logger) *EvictingCache {
	return &EvictingCache{
		underlying: underlying,
		policy:     policy,
		maxSize:    maxSize,
		logger:     logger,
	}
}

// Store adds a block to the cache with eviction
func (c *EvictingCache) Store(cid string, block *blocks.Block) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	// Check if we need to evict
	if c.underlying.Size() >= c.maxSize {
		if victim, ok := c.policy.SelectVictim(); ok {
			if err := c.underlying.Remove(victim); err != nil {
				c.logger.Warn("Failed to evict block", map[string]interface{}{
					"victim": victim,
					"error":  err.Error(),
				})
			} else {
				c.policy.OnRemove(victim)
				c.logger.Debug("Evicted block", map[string]interface{}{
					"victim": victim,
				})
			}
		}
	}
	
	// Store the block
	if err := c.underlying.Store(cid, block); err != nil {
		return err
	}
	
	// Notify policy
	c.policy.OnStore(cid, block)
	
	return nil
}

// Get retrieves a block from the cache
func (c *EvictingCache) Get(cid string) (*blocks.Block, error) {
	block, err := c.underlying.Get(cid)
	if err != nil {
		return nil, err
	}
	
	// Notify policy of access
	c.policy.OnAccess(cid)
	
	return block, nil
}

// Has checks if a block exists in the cache
func (c *EvictingCache) Has(cid string) bool {
	return c.underlying.Has(cid)
}

// Remove removes a block from the cache
func (c *EvictingCache) Remove(cid string) error {
	if err := c.underlying.Remove(cid); err != nil {
		return err
	}
	
	// Notify policy
	c.policy.OnRemove(cid)
	
	return nil
}

// GetRandomizers returns popular blocks suitable as randomizers
func (c *EvictingCache) GetRandomizers(count int) ([]*BlockInfo, error) {
	return c.underlying.GetRandomizers(count)
}

// IncrementPopularity increases the popularity score of a block
func (c *EvictingCache) IncrementPopularity(cid string) error {
	return c.underlying.IncrementPopularity(cid)
}

// Size returns the number of blocks in the cache
func (c *EvictingCache) Size() int {
	return c.underlying.Size()
}

// Clear removes all blocks from the cache
func (c *EvictingCache) Clear() {
	c.underlying.Clear()
	c.policy.Clear()
}