package relay

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"
	"sort"
	"sync"
	"time"
)

var (
	ErrNoHealthyRelays = errors.New("no healthy relays available")
	ErrInsufficientRelays = errors.New("insufficient relays available")
)

// RoundRobinSelector implements round-robin relay selection
type RoundRobinSelector struct {
	mu sync.Mutex
	index int
}

func NewRoundRobinSelector() *RoundRobinSelector {
	return &RoundRobinSelector{}
}

func (r *RoundRobinSelector) SelectRelays(ctx context.Context, pool *RelayPool, count int) ([]*RelayNode, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	healthy := pool.GetHealthyRelays()
	if len(healthy) == 0 {
		return nil, ErrNoHealthyRelays
	}
	
	if count > len(healthy) {
		count = len(healthy)
	}
	
	selected := make([]*RelayNode, 0, count)
	for i := 0; i < count; i++ {
		selected = append(selected, healthy[r.index%len(healthy)])
		r.index++
	}
	
	return selected, nil
}

// LeastLoadedSelector selects relays with the lowest load
type LeastLoadedSelector struct{}

func NewLeastLoadedSelector() *LeastLoadedSelector {
	return &LeastLoadedSelector{}
}

func (l *LeastLoadedSelector) SelectRelays(ctx context.Context, pool *RelayPool, count int) ([]*RelayNode, error) {
	healthy := pool.GetHealthyRelays()
	if len(healthy) == 0 {
		return nil, ErrNoHealthyRelays
	}
	
	if count > len(healthy) {
		count = len(healthy)
	}
	
	// Sort by load (using total requests as a proxy for load)
	sort.Slice(healthy, func(i, j int) bool {
		return healthy[i].Performance.TotalRequests < healthy[j].Performance.TotalRequests
	})
	
	return healthy[:count], nil
}

// RandomSelector selects relays randomly
type RandomSelector struct{}

func NewRandomSelector() *RandomSelector {
	return &RandomSelector{}
}

func (r *RandomSelector) SelectRelays(ctx context.Context, pool *RelayPool, count int) ([]*RelayNode, error) {
	healthy := pool.GetHealthyRelays()
	if len(healthy) == 0 {
		return nil, ErrNoHealthyRelays
	}
	
	if count > len(healthy) {
		count = len(healthy)
	}
	
	// Fisher-Yates shuffle
	shuffled := make([]*RelayNode, len(healthy))
	copy(shuffled, healthy)
	
	for i := len(shuffled) - 1; i > 0; i-- {
		j, _ := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		shuffled[i], shuffled[j.Int64()] = shuffled[j.Int64()], shuffled[i]
	}
	
	return shuffled[:count], nil
}

// PerformanceSelector selects relays based on performance metrics
type PerformanceSelector struct {
	weights PerformanceWeights
}

type PerformanceWeights struct {
	Latency     float64
	Bandwidth   float64
	Reliability float64
}

func NewPerformanceSelector(weights PerformanceWeights) *PerformanceSelector {
	return &PerformanceSelector{weights: weights}
}

func (p *PerformanceSelector) SelectRelays(ctx context.Context, pool *RelayPool, count int) ([]*RelayNode, error) {
	healthy := pool.GetHealthyRelays()
	if len(healthy) == 0 {
		return nil, ErrNoHealthyRelays
	}
	
	if count > len(healthy) {
		count = len(healthy)
	}
	
	// Calculate performance scores
	type relayScore struct {
		relay *RelayNode
		score float64
	}
	
	scores := make([]relayScore, 0, len(healthy))
	
	for _, relay := range healthy {
		score := p.calculateScore(relay)
		scores = append(scores, relayScore{relay: relay, score: score})
	}
	
	// Sort by score (higher is better)
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].score > scores[j].score
	})
	
	selected := make([]*RelayNode, count)
	for i := 0; i < count; i++ {
		selected[i] = scores[i].relay
	}
	
	return selected, nil
}

func (p *PerformanceSelector) calculateScore(relay *RelayNode) float64 {
	// Normalize metrics (lower latency is better, higher bandwidth/reliability is better)
	latencyScore := 1.0 / (1.0 + float64(relay.Health.Latency.Milliseconds()))
	bandwidthScore := relay.Health.Bandwidth / 100.0 // Normalize to 0-1 range
	reliabilityScore := relay.Health.Reliability
	
	// Calculate weighted score
	score := (latencyScore * p.weights.Latency) +
		(bandwidthScore * p.weights.Bandwidth) +
		(reliabilityScore * p.weights.Reliability)
	
	return score
}

// PrivacySelector selects relays optimized for privacy
type PrivacySelector struct {
	diversityThreshold float64
}

func NewPrivacySelector(diversityThreshold float64) *PrivacySelector {
	return &PrivacySelector{diversityThreshold: diversityThreshold}
}

func (p *PrivacySelector) SelectRelays(ctx context.Context, pool *RelayPool, count int) ([]*RelayNode, error) {
	healthy := pool.GetHealthyRelays()
	if len(healthy) == 0 {
		return nil, ErrNoHealthyRelays
	}
	
	if count > len(healthy) {
		count = len(healthy)
	}
	
	// For privacy, we want to select relays that are diverse
	// (different network locations, different operators, etc.)
	// For now, we'll use a simple approach of selecting relays
	// that haven't been used recently
	
	sort.Slice(healthy, func(i, j int) bool {
		return healthy[i].LastUsed.Before(healthy[j].LastUsed)
	})
	
	// Select from the least recently used relays
	selected := make([]*RelayNode, count)
	for i := 0; i < count; i++ {
		selected[i] = healthy[i]
	}
	
	return selected, nil
}

// HybridSelector combines multiple selection strategies
type HybridSelector struct {
	selectors []RelaySelector
	weights   []float64
}

func NewHybridSelector(selectors []RelaySelector, weights []float64) *HybridSelector {
	if len(selectors) != len(weights) {
		panic("selectors and weights must have the same length")
	}
	
	return &HybridSelector{
		selectors: selectors,
		weights:   weights,
	}
}

func (h *HybridSelector) SelectRelays(ctx context.Context, pool *RelayPool, count int) ([]*RelayNode, error) {
	healthy := pool.GetHealthyRelays()
	if len(healthy) == 0 {
		return nil, ErrNoHealthyRelays
	}
	
	if count > len(healthy) {
		count = len(healthy)
	}
	
	// Get selections from each selector
	selections := make([][]*RelayNode, len(h.selectors))
	for i, selector := range h.selectors {
		relays, err := selector.SelectRelays(ctx, pool, count*2) // Get more than needed
		if err != nil {
			continue
		}
		selections[i] = relays
	}
	
	// Score each relay based on how many selectors chose it
	relayScores := make(map[*RelayNode]float64)
	
	for i, selection := range selections {
		weight := h.weights[i]
		for j, relay := range selection {
			// Higher weight for earlier positions
			positionWeight := 1.0 / float64(j+1)
			relayScores[relay] += weight * positionWeight
		}
	}
	
	// Sort by score
	type relayScore struct {
		relay *RelayNode
		score float64
	}
	
	scores := make([]relayScore, 0, len(relayScores))
	for relay, score := range relayScores {
		scores = append(scores, relayScore{relay: relay, score: score})
	}
	
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].score > scores[j].score
	})
	
	// Select top scored relays
	selected := make([]*RelayNode, count)
	for i := 0; i < count && i < len(scores); i++ {
		selected[i] = scores[i].relay
	}
	
	return selected, nil
}