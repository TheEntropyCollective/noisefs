package p2p

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"sort"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// PrivacyStrategy implements privacy-preserving peer selection and load distribution
type PrivacyStrategy struct {
	peerManager      *PeerManager
	privacyMetrics   map[peer.ID]*PrivacyMetrics
	routingTable     *PrivacyRoutingTable
	decoyManager     *DecoyTrafficManager
	mutex            sync.RWMutex
	
	// Configuration
	maxHops             int
	minAnonymitySet     int
	decoyTrafficRate    float64
	routingDelay        time.Duration
	batchWindow         time.Duration
}

// PrivacyMetrics tracks privacy-related metrics for a peer
type PrivacyMetrics struct {
	PeerID              peer.ID              `json:"peer_id"`
	LastUpdate          time.Time            `json:"last_update"`
	
	// Anonymity metrics
	ForwardedRequests   int64                `json:"forwarded_requests"`
	DirectRequests      int64                `json:"direct_requests"`
	AnonymityRatio      float64              `json:"anonymity_ratio"`
	
	// Timing analysis resistance
	RequestTiming       []time.Time          `json:"request_timing"`
	TimingVariance      time.Duration        `json:"timing_variance"`
	
	// Load distribution
	RequestLoad         int64                `json:"request_load"`
	LastRequestTime     time.Time            `json:"last_request_time"`
	LoadScore           float64              `json:"load_score"`
	
	// Trust and reputation
	TrustScore          float64              `json:"trust_score"`
	PrivacyViolations   int                  `json:"privacy_violations"`
	
	mutex               sync.RWMutex
}

// PrivacyRoutingTable manages privacy-aware routing paths
type PrivacyRoutingTable struct {
	routes              map[string]*RoutingPath
	pathCache           map[string][]peer.ID
	lastCacheUpdate     time.Time
	cacheExpiry         time.Duration
	mutex               sync.RWMutex
}

// RoutingPath represents a privacy-preserving routing path
type RoutingPath struct {
	Source              peer.ID
	Destination         peer.ID
	IntermediateHops    []peer.ID
	PathDelay           time.Duration
	AnonymityLevel      int
	CreatedAt           time.Time
	UsageCount          int64
}

// DecoyTrafficManager generates and manages decoy traffic for privacy
type DecoyTrafficManager struct {
	decoyRequests       map[peer.ID]*DecoySchedule
	trafficPattern      *TrafficPattern
	mutex               sync.RWMutex
}

// DecoySchedule manages decoy traffic for a specific peer
type DecoySchedule struct {
	PeerID              peer.ID
	NextDecoyTime       time.Time
	DecoyInterval       time.Duration
	RecentDecoys        []time.Time
	TrafficVolume       int64
}

// TrafficPattern defines patterns for generating realistic decoy traffic
type TrafficPattern struct {
	PeakHours           []int                // Hours of day with peak traffic
	MinRequestInterval  time.Duration
	MaxRequestInterval  time.Duration
	BurstProbability    float64
	BurstSize           int
}

// NewPrivacyStrategy creates a new privacy-preserving peer selection strategy
func NewPrivacyStrategy(pm *PeerManager) *PrivacyStrategy {
	return &PrivacyStrategy{
		peerManager:       pm,
		privacyMetrics:    make(map[peer.ID]*PrivacyMetrics),
		routingTable:      NewPrivacyRoutingTable(),
		decoyManager:      NewDecoyTrafficManager(),
		maxHops:           3,
		minAnonymitySet:   5,
		decoyTrafficRate:  0.1, // 10% decoy traffic
		routingDelay:      time.Millisecond * 100,
		batchWindow:       time.Second * 5,
	}
}

// SelectPeers selects peers with privacy-preserving considerations
func (ps *PrivacyStrategy) SelectPeers(ctx context.Context, criteria SelectionCriteria) ([]peer.ID, error) {
	ps.mutex.RLock()
	defer ps.mutex.RUnlock()
	
	if criteria.RequirePrivacy {
		return ps.selectPrivacyPreservingPeers(ctx, criteria)
	}
	
	// Standard selection with privacy awareness
	return ps.selectWithPrivacyConsiderations(ctx, criteria)
}

// selectPrivacyPreservingPeers selects peers for maximum privacy
func (ps *PrivacyStrategy) selectPrivacyPreservingPeers(ctx context.Context, criteria SelectionCriteria) ([]peer.ID, error) {
	healthyPeers := ps.peerManager.GetHealthyPeers()
	if len(healthyPeers) < ps.minAnonymitySet {
		return nil, fmt.Errorf("insufficient peers for anonymity set (need %d, have %d)", 
			ps.minAnonymitySet, len(healthyPeers))
	}
	
	// Create anonymity sets
	anonymitySets := ps.createAnonymitySets(healthyPeers, criteria)
	
	// Select from different sets to maximize privacy
	return ps.selectFromAnonymitySets(anonymitySets, criteria.Count)
}

// selectWithPrivacyConsiderations performs standard selection with privacy awareness
func (ps *PrivacyStrategy) selectWithPrivacyConsiderations(ctx context.Context, criteria SelectionCriteria) ([]peer.ID, error) {
	healthyPeers := ps.peerManager.GetHealthyPeers()
	if len(healthyPeers) == 0 {
		return nil, fmt.Errorf("no healthy peers available")
	}
	
	// Filter and score peers
	candidates := ps.scorePrivacyPeers(healthyPeers, criteria)
	
	// Apply load balancing
	candidates = ps.applyLoadBalancing(candidates, criteria.LoadBalancing)
	
	// Sort by privacy score
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})
	
	// Select peers with temporal obfuscation
	return ps.selectWithTemporalObfuscation(candidates, criteria.Count)
}

// createAnonymitySets creates groups of peers for anonymity
func (ps *PrivacyStrategy) createAnonymitySets(peers []peer.ID, criteria SelectionCriteria) [][]peer.ID {
	// Shuffle peers to create random sets
	shuffled := make([]peer.ID, len(peers))
	copy(shuffled, peers)
	ps.shufflePeers(shuffled)
	
	var sets [][]peer.ID
	setSize := ps.minAnonymitySet
	
	for i := 0; i < len(shuffled); i += setSize {
		end := i + setSize
		if end > len(shuffled) {
			end = len(shuffled)
		}
		
		if end-i >= ps.minAnonymitySet {
			sets = append(sets, shuffled[i:end])
		}
	}
	
	return sets
}

// selectFromAnonymitySets selects peers from different anonymity sets
func (ps *PrivacyStrategy) selectFromAnonymitySets(sets [][]peer.ID, count int) ([]peer.ID, error) {
	if len(sets) == 0 {
		return nil, fmt.Errorf("no anonymity sets available")
	}
	
	var result []peer.ID
	setIndex := 0
	
	for len(result) < count && setIndex < len(sets)*count {
		currentSet := sets[setIndex%len(sets)]
		peerIndex := (setIndex / len(sets)) % len(currentSet)
		
		peer := currentSet[peerIndex]
		
		// Avoid duplicates
		if !ps.containsPeer(result, peer) {
			result = append(result, peer)
		}
		
		setIndex++
	}
	
	return result, nil
}

// scorePrivacyPeers scores peers based on privacy metrics
func (ps *PrivacyStrategy) scorePrivacyPeers(peers []peer.ID, criteria SelectionCriteria) []PeerCandidate {
	var candidates []PeerCandidate
	
	for _, peerID := range peers {
		// Skip excluded peers
		if ps.isPeerExcluded(peerID, criteria.ExcludePeers) {
			continue
		}
		
		metrics := ps.getOrCreateMetrics(peerID)
		metrics.mutex.RLock()
		
		score := ps.calculatePrivacyScore(metrics)
		
		metrics.mutex.RUnlock()
		
		candidates = append(candidates, PeerCandidate{
			PeerID: peerID,
			Score:  score,
		})
	}
	
	return candidates
}

// calculatePrivacyScore calculates a privacy score for a peer
func (ps *PrivacyStrategy) calculatePrivacyScore(metrics *PrivacyMetrics) float64 {
	// Base score
	baseScore := 0.5
	
	// Anonymity ratio score (prefer peers that forward requests)
	anonymityScore := metrics.AnonymityRatio
	
	// Load distribution score (prefer less loaded peers)
	loadScore := 1.0 - metrics.LoadScore
	
	// Trust score
	trustScore := metrics.TrustScore
	
	// Privacy violation penalty
	violationPenalty := 1.0
	if metrics.PrivacyViolations > 0 {
		violationPenalty = 1.0 / (1.0 + float64(metrics.PrivacyViolations)*0.1)
	}
	
	// Composite score
	score := (baseScore + anonymityScore*0.3 + loadScore*0.3 + trustScore*0.4) * violationPenalty
	
	return score
}

// applyLoadBalancing adjusts peer selection for load balancing
func (ps *PrivacyStrategy) applyLoadBalancing(candidates []PeerCandidate, enableLoadBalancing bool) []PeerCandidate {
	if !enableLoadBalancing {
		return candidates
	}
	
	// Calculate load distribution
	totalLoad := int64(0)
	for _, candidate := range candidates {
		metrics := ps.getOrCreateMetrics(candidate.PeerID)
		metrics.mutex.RLock()
		totalLoad += metrics.RequestLoad
		metrics.mutex.RUnlock()
	}
	
	if totalLoad == 0 {
		return candidates
	}
	
	// Adjust scores based on load
	for i := range candidates {
		metrics := ps.getOrCreateMetrics(candidates[i].PeerID)
		metrics.mutex.RLock()
		
		loadRatio := float64(metrics.RequestLoad) / float64(totalLoad)
		loadAdjustment := 1.0 - loadRatio // Prefer less loaded peers
		
		candidates[i].Score *= loadAdjustment
		
		metrics.mutex.RUnlock()
	}
	
	return candidates
}

// selectWithTemporalObfuscation adds timing obfuscation to peer selection
func (ps *PrivacyStrategy) selectWithTemporalObfuscation(candidates []PeerCandidate, count int) ([]peer.ID, error) {
	if count > len(candidates) {
		count = len(candidates)
	}
	
	// Add random delay for timing obfuscation
	delay, _ := rand.Int(rand.Reader, big.NewInt(int64(ps.routingDelay.Nanoseconds())))
	time.Sleep(time.Duration(delay.Int64()))
	
	result := make([]peer.ID, count)
	for i := 0; i < count; i++ {
		result[i] = candidates[i].PeerID
	}
	
	return result, nil
}

// CreatePrivacyRoutingPath creates a privacy-preserving routing path
func (ps *PrivacyStrategy) CreatePrivacyRoutingPath(source, destination peer.ID, maxHops int) (*RoutingPath, error) {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()
	
	// Get available intermediate peers
	availablePeers := ps.peerManager.GetHealthyPeers()
	intermediates := ps.selectIntermediatePeers(availablePeers, source, destination, maxHops)
	
	path := &RoutingPath{
		Source:           source,
		Destination:      destination,
		IntermediateHops: intermediates,
		PathDelay:        ps.calculatePathDelay(len(intermediates)),
		AnonymityLevel:   len(intermediates) + 1,
		CreatedAt:        time.Now(),
	}
	
	// Cache the path
	pathKey := fmt.Sprintf("%s->%s", source, destination)
	ps.routingTable.AddPath(pathKey, path)
	
	return path, nil
}

// selectIntermediatePeers selects intermediate peers for routing path
func (ps *PrivacyStrategy) selectIntermediatePeers(available []peer.ID, source, destination peer.ID, maxHops int) []peer.ID {
	// Filter out source and destination
	candidates := make([]peer.ID, 0)
	for _, peerID := range available {
		if peerID != source && peerID != destination {
			candidates = append(candidates, peerID)
		}
	}
	
	if len(candidates) == 0 {
		return nil
	}
	
	// Determine number of hops (1 to maxHops)
	numHops := 1
	if maxHops > 1 && len(candidates) >= maxHops {
		hopBig, _ := rand.Int(rand.Reader, big.NewInt(int64(maxHops)))
		numHops = int(hopBig.Int64()) + 1
	}
	
	if numHops > len(candidates) {
		numHops = len(candidates)
	}
	
	// Randomly select intermediate peers
	ps.shufflePeers(candidates)
	return candidates[:numHops]
}

// calculatePathDelay calculates the expected delay for a routing path
func (ps *PrivacyStrategy) calculatePathDelay(hops int) time.Duration {
	baseDelay := ps.routingDelay
	return baseDelay * time.Duration(hops+1)
}

// RouteRequest routes a request through privacy-preserving path
func (ps *PrivacyStrategy) RouteRequest(ctx context.Context, source, destination peer.ID, request interface{}) error {
	// Check if we should use direct routing or privacy routing
	if ps.shouldUseDirectRoute() {
		return ps.routeDirectly(ctx, destination, request)
	}
	
	// Create or get privacy routing path
	path, err := ps.CreatePrivacyRoutingPath(source, destination, ps.maxHops)
	if err != nil {
		return fmt.Errorf("failed to create privacy path: %w", err)
	}
	
	// Route through intermediate peers
	return ps.routeThroughPath(ctx, path, request)
}

// shouldUseDirectRoute determines if direct routing should be used
func (ps *PrivacyStrategy) shouldUseDirectRoute() bool {
	// Use probability-based decision
	threshold, _ := rand.Int(rand.Reader, big.NewInt(100))
	return threshold.Int64() < 30 // 30% chance of direct routing
}

// routeDirectly routes request directly to destination
func (ps *PrivacyStrategy) routeDirectly(ctx context.Context, destination peer.ID, request interface{}) error {
	// Update metrics for direct request
	metrics := ps.getOrCreateMetrics(destination)
	metrics.mutex.Lock()
	metrics.DirectRequests++
	metrics.updateAnonymityRatio()
	metrics.mutex.Unlock()
	
	// In actual implementation, this would send the request
	// For now, just simulate the operation
	return nil
}

// routeThroughPath routes request through privacy path
func (ps *PrivacyStrategy) routeThroughPath(ctx context.Context, path *RoutingPath, request interface{}) error {
	// Update path usage
	path.UsageCount++
	
	// Route through each intermediate hop
	currentPeer := path.Source
	allHops := append(path.IntermediateHops, path.Destination)
	
	for _, nextPeer := range allHops {
		// Add routing delay
		time.Sleep(ps.routingDelay)
		
		// Update metrics for forwarded request
		metrics := ps.getOrCreateMetrics(nextPeer)
		metrics.mutex.Lock()
		metrics.ForwardedRequests++
		metrics.updateAnonymityRatio()
		metrics.mutex.Unlock()
		
		currentPeer = nextPeer
	}
	
	return nil
}

// GenerateDecoyTraffic generates decoy traffic for privacy
func (ps *PrivacyStrategy) GenerateDecoyTraffic(ctx context.Context, peerID peer.ID) {
	ps.decoyManager.mutex.Lock()
	defer ps.decoyManager.mutex.Unlock()
	
	schedule, exists := ps.decoyManager.decoyRequests[peerID]
	if !exists {
		schedule = &DecoySchedule{
			PeerID:        peerID,
			DecoyInterval: time.Minute * 5, // Default 5-minute interval
			RecentDecoys:  make([]time.Time, 0),
		}
		ps.decoyManager.decoyRequests[peerID] = schedule
	}
	
	// Check if it's time to send decoy
	now := time.Now()
	if now.After(schedule.NextDecoyTime) {
		ps.sendDecoyRequest(ctx, peerID)
		
		// Schedule next decoy
		schedule.NextDecoyTime = now.Add(schedule.DecoyInterval)
		schedule.RecentDecoys = append(schedule.RecentDecoys, now)
		
		// Keep only recent decoys (last hour)
		cutoff := now.Add(-time.Hour)
		filtered := make([]time.Time, 0)
		for _, decoyTime := range schedule.RecentDecoys {
			if decoyTime.After(cutoff) {
				filtered = append(filtered, decoyTime)
			}
		}
		schedule.RecentDecoys = filtered
	}
}

// sendDecoyRequest sends a decoy request to obfuscate traffic patterns
func (ps *PrivacyStrategy) sendDecoyRequest(ctx context.Context, peerID peer.ID) {
	// In actual implementation, this would send a realistic decoy request
	// For now, just update metrics
	metrics := ps.getOrCreateMetrics(peerID)
	metrics.mutex.Lock()
	metrics.ForwardedRequests++ // Count decoys as forwarded requests
	metrics.updateAnonymityRatio()
	metrics.mutex.Unlock()
}

// UpdateMetrics updates privacy metrics for a peer
func (ps *PrivacyStrategy) UpdateMetrics(peerID peer.ID, success bool, latency time.Duration, bytes int64) {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()
	
	metrics := ps.getOrCreateMetrics(peerID)
	metrics.mutex.Lock()
	defer metrics.mutex.Unlock()
	
	now := time.Now()
	metrics.LastUpdate = now
	metrics.LastRequestTime = now
	metrics.RequestLoad++
	
	// Update timing analysis resistance
	metrics.RequestTiming = append(metrics.RequestTiming, now)
	
	// Keep only recent timing data (last hour)
	cutoff := now.Add(-time.Hour)
	filtered := make([]time.Time, 0)
	for _, requestTime := range metrics.RequestTiming {
		if requestTime.After(cutoff) {
			filtered = append(filtered, requestTime)
		}
	}
	metrics.RequestTiming = filtered
	
	// Calculate timing variance
	if len(metrics.RequestTiming) > 1 {
		metrics.TimingVariance = ps.calculateTimingVariance(metrics.RequestTiming)
	}
	
	// Update load score (higher load = higher score, which is worse for privacy)
	timeSinceLastRequest := now.Sub(metrics.LastRequestTime)
	if timeSinceLastRequest > 0 {
		metrics.LoadScore = 1.0 / (1.0 + timeSinceLastRequest.Hours())
	}
	
	// Update trust score based on success
	if success {
		metrics.TrustScore = metrics.TrustScore*0.9 + 0.1 // Slight increase
	} else {
		metrics.TrustScore = metrics.TrustScore * 0.95 // Slight decrease
	}
	
	// Ensure trust score stays in valid range
	if metrics.TrustScore > 1.0 {
		metrics.TrustScore = 1.0
	} else if metrics.TrustScore < 0.0 {
		metrics.TrustScore = 0.0
	}
}

// calculateTimingVariance calculates variance in request timing
func (ps *PrivacyStrategy) calculateTimingVariance(timings []time.Time) time.Duration {
	if len(timings) < 2 {
		return 0
	}
	
	// Calculate intervals between requests
	intervals := make([]time.Duration, len(timings)-1)
	for i := 1; i < len(timings); i++ {
		intervals[i-1] = timings[i].Sub(timings[i-1])
	}
	
	// Calculate mean interval
	totalInterval := time.Duration(0)
	for _, interval := range intervals {
		totalInterval += interval
	}
	meanInterval := totalInterval / time.Duration(len(intervals))
	
	// Calculate variance
	variance := time.Duration(0)
	for _, interval := range intervals {
		diff := interval - meanInterval
		variance += diff * diff / time.Duration(len(intervals))
	}
	
	return variance
}

// GetPeerInfo returns privacy information for a specific peer
func (ps *PrivacyStrategy) GetPeerInfo(peerID peer.ID) (*PeerInfo, bool) {
	ps.mutex.RLock()
	defer ps.mutex.RUnlock()
	
	if metrics, exists := ps.privacyMetrics[peerID]; exists {
		metrics.mutex.RLock()
		defer metrics.mutex.RUnlock()
		
		peerInfo := &PeerInfo{
			ID:              peerID,
			LastSeen:        metrics.LastUpdate,
			Reputation:      metrics.TrustScore,
			TotalRequests:   metrics.DirectRequests + metrics.ForwardedRequests,
		}
		
		return peerInfo, true
	}
	
	return nil, false
}

// Helper methods

func (ps *PrivacyStrategy) getOrCreateMetrics(peerID peer.ID) *PrivacyMetrics {
	if metrics, exists := ps.privacyMetrics[peerID]; exists {
		return metrics
	}
	
	metrics := &PrivacyMetrics{
		PeerID:        peerID,
		LastUpdate:    time.Now(),
		TrustScore:    0.5, // Start with neutral trust
		RequestTiming: make([]time.Time, 0),
	}
	ps.privacyMetrics[peerID] = metrics
	
	return metrics
}

func (ps *PrivacyStrategy) isPeerExcluded(peerID peer.ID, excludeList []peer.ID) bool {
	for _, excluded := range excludeList {
		if peerID == excluded {
			return true
		}
	}
	return false
}

func (ps *PrivacyStrategy) containsPeer(peers []peer.ID, target peer.ID) bool {
	for _, peerID := range peers {
		if peerID == target {
			return true
		}
	}
	return false
}

func (ps *PrivacyStrategy) shufflePeers(peers []peer.ID) {
	for i := len(peers) - 1; i > 0; i-- {
		j, _ := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		peers[i], peers[int(j.Int64())] = peers[int(j.Int64())], peers[i]
	}
}

// updateAnonymityRatio updates the anonymity ratio for metrics
func (pm *PrivacyMetrics) updateAnonymityRatio() {
	total := pm.DirectRequests + pm.ForwardedRequests
	if total > 0 {
		pm.AnonymityRatio = float64(pm.ForwardedRequests) / float64(total)
	}
}

// NewPrivacyRoutingTable creates a new privacy routing table
func NewPrivacyRoutingTable() *PrivacyRoutingTable {
	return &PrivacyRoutingTable{
		routes:        make(map[string]*RoutingPath),
		pathCache:     make(map[string][]peer.ID),
		cacheExpiry:   time.Minute * 10, // Cache paths for 10 minutes
	}
}

// AddPath adds a routing path to the table
func (prt *PrivacyRoutingTable) AddPath(key string, path *RoutingPath) {
	prt.mutex.Lock()
	defer prt.mutex.Unlock()
	
	prt.routes[key] = path
	prt.pathCache[key] = append([]peer.ID{path.Source}, append(path.IntermediateHops, path.Destination)...)
	prt.lastCacheUpdate = time.Now()
}

// GetPath retrieves a routing path
func (prt *PrivacyRoutingTable) GetPath(key string) (*RoutingPath, bool) {
	prt.mutex.RLock()
	defer prt.mutex.RUnlock()
	
	path, exists := prt.routes[key]
	if !exists {
		return nil, false
	}
	
	// Check if path is still valid (not expired)
	if time.Since(path.CreatedAt) > prt.cacheExpiry {
		return nil, false
	}
	
	return path, true
}

// NewDecoyTrafficManager creates a new decoy traffic manager
func NewDecoyTrafficManager() *DecoyTrafficManager {
	return &DecoyTrafficManager{
		decoyRequests: make(map[peer.ID]*DecoySchedule),
		trafficPattern: &TrafficPattern{
			PeakHours:          []int{9, 10, 11, 14, 15, 16, 19, 20, 21}, // Business hours + evening
			MinRequestInterval: time.Second * 30,
			MaxRequestInterval: time.Minute * 10,
			BurstProbability:   0.1,
			BurstSize:          5,
		},
	}
}