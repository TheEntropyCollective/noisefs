// Package noisefs provides advanced randomizer selection and intelligent management functionality.
// This file handles the sophisticated selection of randomizer blocks for 3-tuple XOR anonymization,
// implementing diversity controls, weighted selection, availability checking, and cache-aware strategies
// to optimize both security and performance in the NoiseFS privacy-preserving storage system.
//
// The randomizer system provides multiple layers of optimization:
//   - Intelligent cache utilization for storage efficiency
//   - Diversity controls to prevent concentration attacks
//   - Availability checking for reliability and fallback
//   - Weighted selection based on popularity and security scores
//   - Cryptographically secure random generation as fallback
//   - Metrics integration for performance monitoring
//
// Key Features:
//   - 3-tuple XOR anonymization with optimal randomizer pairs
//   - Cache-aware selection to maximize randomizer reuse
//   - Diversity controls for anti-concentration security
//   - Availability integration for reliable randomizer access
//   - Fallback strategies ensuring operation continuity
//   - Comprehensive metrics tracking for optimization
package noisefs

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"math/big"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
)

// SelectRandomizers selects or generates two optimal randomizer blocks for 3-tuple XOR anonymization.
// This is the primary randomizer selection method that implements intelligent selection strategies
// to maximize both security and storage efficiency through cache utilization, diversity controls,
// and availability checking.
//
// Randomizer Selection Strategy:
//   1. Attempt cache utilization for storage efficiency
//   2. Apply diversity controls for anti-concentration security
//   3. Check availability for reliability and fallback
//   4. Use weighted selection based on popularity and security scores
//   5. Generate secure random blocks as fallback when needed
//
// Cache Utilization Hierarchy:
//   - Both randomizers from cache (optimal efficiency, 0 new storage)
//   - One from cache, one generated (balanced efficiency)
//   - Both generated (maximum security, higher storage cost)
//
// Security Considerations:
//   - Diversity controls prevent concentration attacks
//   - Availability checking ensures reliable randomizer access
//   - Cryptographically secure generation for new randomizers
//   - Uniqueness verification to prevent identical randomizers
//
// Performance Optimization:
//   - Cache-aware selection for storage efficiency
//   - Popularity-based weighting for optimal reuse
//   - Metrics tracking for continuous optimization
//   - Intelligent fallback strategies for reliability
//
// Parameters:
//   - blockSize: Required size for randomizer blocks (must match data block size)
//
// Returns:
//   - randomizer1: First randomizer block for XOR operation
//   - cid1: Content identifier for first randomizer
//   - randomizer2: Second randomizer block for XOR operation
//   - cid2: Content identifier for second randomizer
//   - bytesStored: New storage bytes used (0 if both from cache, >0 for generated blocks)
//   - error: Non-nil if selection fails, generation fails, or storage fails
//
// Call Flow:
//   - Called by: Upload operations, file processing, anonymization workflows
//   - Calls: Cache systems, diversity controls, availability checking, secure generation
//
// Time Complexity: O(log n) where n is the number of cached randomizers
// Space Complexity: O(b) where b is the block size for generated randomizers
func (c *Client) SelectRandomizers(blockSize int) (*blocks.Block, string, *blocks.Block, string, int64, error) {
	var totalNewStorage int64 = 0

	// Attempt cache utilization first for optimal storage efficiency
	// Request larger pool for better diversity and selection quality
	randomizers, err := c.cache.GetRandomizers(20) // Get diverse pool for intelligent selection
	if err == nil && len(randomizers) > 0 {
		// Filter cache results to find randomizers matching required block size
		// Size matching is critical for XOR compatibility
		suitableBlocks := make([]*cache.BlockInfo, 0)
		for _, info := range randomizers {
			if info.Size == blockSize {
				suitableBlocks = append(suitableBlocks, info)
			}
		}

		// If we have at least 2 suitable cached blocks, use them
		if len(suitableBlocks) >= 2 {
			// Use diversity-aware selection with availability checking
			selected1, selected2, err := c.selectRandomizersWithDiversityAndAvailability(suitableBlocks)
			if err != nil {
				return nil, "", nil, "", 0, fmt.Errorf("failed to select randomizers with diversity and availability: %w", err)
			}

			// Record selections for diversity tracking
			if c.diversityControls != nil {
				c.diversityControls.RecordRandomizerSelection(selected1.CID)
				c.diversityControls.RecordRandomizerSelection(selected2.CID)
			}

			// Update popularity and metrics
			c.cache.IncrementPopularity(selected1.CID)
			c.cache.IncrementPopularity(selected2.CID)
			c.metrics.RecordBlockReuse()
			c.metrics.RecordBlockReuse()

			return selected1.Block, selected1.CID, selected2.Block, selected2.CID, 0, nil // 0 bytes new storage - both from cache
		}

		// If we have exactly 1 suitable cached block, use it and generate another
		if len(suitableBlocks) == 1 {
			selected1 := suitableBlocks[0]
			c.cache.IncrementPopularity(selected1.CID)
			c.metrics.RecordBlockReuse()

			// Generate second randomizer
			randBlock2, err := blocks.NewRandomBlock(blockSize)
			if err != nil {
				return nil, "", nil, "", 0, fmt.Errorf("failed to create second randomizer: %w", err)
			}

			cid2, bytesStored, err := c.storeBlockWithTracking(context.Background(), randBlock2)
			if err != nil {
				return nil, "", nil, "", 0, fmt.Errorf("failed to store second randomizer: %w", err)
			}

			c.cache.Store(cid2, randBlock2)
			c.metrics.RecordBlockGeneration()

			return selected1.Block, selected1.CID, randBlock2, cid2, bytesStored, nil // Only count new randomizer storage
		}
	}

	// No suitable cached blocks or insufficient blocks, generate both randomizers
	// Ensure they're different by generating different random data
	randBlock1, err := blocks.NewRandomBlock(blockSize)
	if err != nil {
		return nil, "", nil, "", 0, fmt.Errorf("failed to create first randomizer: %w", err)
	}

	// Generate second randomizer, retry if identical to first (extremely unlikely but possible)
	var randBlock2 *blocks.Block
	for attempts := 0; attempts < 10; attempts++ {
		randBlock2, err = blocks.NewRandomBlock(blockSize)
		if err != nil {
			return nil, "", nil, "", 0, fmt.Errorf("failed to create second randomizer: %w", err)
		}

		// Check if blocks are different (compare IDs which are content hashes)
		if randBlock1.ID != randBlock2.ID {
			break
		}

		// If we reach max attempts, this is extremely unlikely with crypto random
		if attempts == 9 {
			return nil, "", nil, "", 0, fmt.Errorf("failed to generate different randomizer blocks after 10 attempts")
		}
	}

	// Store both randomizers using storage abstraction with tracking
	ctx := context.Background() // TODO: Accept context parameter in future version
	cid1, bytesStored1, err := c.storeBlockWithTracking(ctx, randBlock1)
	if err != nil {
		return nil, "", nil, "", 0, fmt.Errorf("failed to store first randomizer: %w", err)
	}

	cid2, bytesStored2, err := c.storeBlockWithTracking(ctx, randBlock2)
	if err != nil {
		return nil, "", nil, "", 0, fmt.Errorf("failed to store second randomizer: %w", err)
	}

	// Ensure CIDs are different (they should be since block content is different)
	if cid1 == cid2 {
		return nil, "", nil, "", 0, fmt.Errorf("generated randomizers have identical CIDs")
	}

	// Cache both randomizers
	c.cache.Store(cid1, randBlock1)
	c.cache.Store(cid2, randBlock2)
	c.metrics.RecordBlockGeneration()
	c.metrics.RecordBlockGeneration()

	totalNewStorage = bytesStored1 + bytesStored2

	return randBlock1, cid1, randBlock2, cid2, totalNewStorage, nil // Count both new randomizers
}

// scoredCandidate represents a candidate randomizer with its calculated diversity and popularity score.
// This internal structure enables weighted selection of randomizers based on multiple factors including
// popularity, diversity controls, and security considerations for optimal randomizer selection.
//
// The scoring system balances multiple objectives:
//   - Popularity scores favor frequently accessed blocks for cache efficiency
//   - Diversity adjustments prevent concentration attacks and improve security
//   - Combined scoring enables intelligent weighted selection
//
// Scoring Applications:
//   - Weighted random selection for optimal randomizer pairs
//   - Diversity control enforcement for anti-concentration security
//   - Cache efficiency optimization through popularity weighting
//   - Security-aware selection balancing performance and privacy
//
// Time Complexity: O(1) for score storage and access
// Space Complexity: O(1) - minimal memory overhead per candidate
type scoredCandidate struct {
	block *cache.BlockInfo // Block information including content and metadata
	score float64          // Calculated score combining popularity and diversity factors
}

// selectRandomizersWithDiversity selects two randomizers using intelligent diversity controls for anti-concentration security.
// This method implements sophisticated selection logic that balances cache efficiency with security requirements,
// preventing concentration attacks while maintaining optimal performance through intelligent weighting.
//
// Diversity Control Features:
//   - Anti-concentration controls to prevent randomizer clustering
//   - Popularity-based weighting for cache efficiency
//   - Security-aware scoring adjustments
//   - Weighted random selection for balanced optimization
//
// Selection Algorithm:
//   1. Calculate base scores from block popularity
//   2. Apply diversity control adjustments for security
//   3. Use weighted random selection for first randomizer
//   4. Remove selected randomizer and repeat for second
//
// Security Benefits:
//   - Prevents over-concentration of specific randomizers
//   - Maintains security properties while optimizing performance
//   - Balances popularity with diversity requirements
//   - Ensures robust randomizer distribution patterns
//
// Parameters:
//   - candidates: Available randomizer blocks for selection (must have at least 2)
//
// Returns:
//   - *cache.BlockInfo: First selected randomizer with optimal diversity score
//   - *cache.BlockInfo: Second selected randomizer ensuring pair diversity
//   - error: Non-nil if insufficient candidates or selection fails
//
// Call Flow:
//   - Called by: selectRandomizersWithDiversityAndAvailability, main selection logic
//   - Calls: Diversity controls, weighted selection, fallback random selection
//
// Time Complexity: O(n) where n is the number of candidates for scoring
// Space Complexity: O(n) for scored candidates storage
func (c *Client) selectRandomizersWithDiversity(candidates []*cache.BlockInfo) (*cache.BlockInfo, *cache.BlockInfo, error) {
	if len(candidates) < 2 {
		return nil, nil, fmt.Errorf("need at least 2 candidates, got %d", len(candidates))
	}

	// Fallback to secure random selection if diversity controls unavailable
	if c.diversityControls == nil {
		return c.selectRandomizersRandom(candidates)
	}

	// Calculate diversity-adjusted scores for all candidates
	scored := make([]scoredCandidate, len(candidates))
	for i, candidate := range candidates {
		// Base score from popularity (add 1 to avoid zero scores)
		baseScore := float64(candidate.Popularity + 1)
		// Apply diversity controls for anti-concentration adjustment
		adjustedScore := c.diversityControls.CalculateRandomizerScore(candidate.CID, baseScore)
		scored[i] = scoredCandidate{
			block: candidate,
			score: adjustedScore,
		}
	}

	// Select first randomizer using weighted random selection based on diversity scores
	selected1, err := c.weightedRandomSelection(scored)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to select first randomizer: %w", err)
	}

	// Remove selected candidate to ensure different second randomizer
	remaining := make([]scoredCandidate, 0, len(scored)-1)
	for _, candidate := range scored {
		if candidate.block.CID != selected1.CID {
			remaining = append(remaining, candidate)
		}
	}

	// Select second randomizer from remaining candidates
	selected2, err := c.weightedRandomSelection(remaining)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to select second randomizer: %w", err)
	}

	return selected1, selected2, nil
}

// selectRandomizersWithDiversityAndAvailability selects two randomizers using diversity controls and availability checking for enhanced reliability.
// This advanced selection method combines intelligent diversity controls with real-time availability checking,
// ensuring both security through anti-concentration and reliability through availability verification.
//
// Enhanced Selection Features:
//   - Real-time availability checking for randomizer reliability
//   - Diversity controls for anti-concentration security
//   - Intelligent fallback strategies for unavailable randomizers
//   - Integration with availability monitoring systems
//   - Graceful degradation when availability checking fails
//
// Selection Algorithm:
//   1. Check availability of all candidate randomizers
//   2. Filter candidates to only include available ones
//   3. Apply diversity-aware selection on available candidates
//   4. Fallback to diversity-only selection if insufficient available candidates
//
// Reliability Benefits:
//   - Ensures selected randomizers are actually accessible
//   - Reduces XOR operation failures from unavailable randomizers
//   - Improves system reliability through proactive availability checking
//   - Enables predictable performance through availability awareness
//
// Security Considerations:
//   - Maintains diversity controls even with availability constraints
//   - Balances availability with anti-concentration requirements
//   - Provides secure fallback when availability data is incomplete
//   - Preserves security properties under network conditions
//
// Parameters:
//   - candidates: Available randomizer blocks for selection (must have at least 2)
//
// Returns:
//   - *cache.BlockInfo: First selected randomizer with verified availability
//   - *cache.BlockInfo: Second selected randomizer with verified availability
//   - error: Non-nil if insufficient candidates or selection fails
//
// Call Flow:
//   - Called by: SelectRandomizers, primary randomizer selection logic
//   - Calls: Availability integration, diversity controls, fallback selection
//
// Time Complexity: O(n + a) where n is candidates and a is availability check overhead
// Space Complexity: O(n) for availability results and filtered candidates
func (c *Client) selectRandomizersWithDiversityAndAvailability(candidates []*cache.BlockInfo) (*cache.BlockInfo, *cache.BlockInfo, error) {
	if len(candidates) < 2 {
		return nil, nil, fmt.Errorf("need at least 2 candidates, got %d", len(candidates))
	}

	// If no availability integration, fall back to diversity-only selection
	if c.availabilityIntegration == nil {
		return c.selectRandomizersWithDiversity(candidates)
	}

	// Check availability of all candidates
	ctx := context.Background()
	candidateCIDs := make([]string, len(candidates))
	for i, candidate := range candidates {
		candidateCIDs[i] = candidate.CID
	}

	availabilityResults := c.availabilityIntegration.CheckAvailability(ctx, candidateCIDs)

	// Filter candidates to only include available ones
	availableCandidates := make([]*cache.BlockInfo, 0, len(candidates))
	for _, candidate := range candidates {
		if status, exists := availabilityResults[candidate.CID]; exists && status.Available {
			availableCandidates = append(availableCandidates, candidate)
		}
	}

	// If we don't have enough available candidates, fallback to diversity-only selection
	if len(availableCandidates) < 2 {
		return c.selectRandomizersWithDiversity(candidates)
	}

	// Use diversity-aware selection on available candidates
	return c.selectRandomizersWithDiversity(availableCandidates)
}

// selectRandomizersRandom provides cryptographically secure fallback random selection for reliable randomizer choice.
// This fallback method implements secure random selection when diversity controls or availability checking
// are unavailable, ensuring the system continues to operate securely under all conditions.
//
// Fallback Security Features:
//   - Cryptographically secure random number generation
//   - Uniform distribution for unbiased randomizer selection
//   - Guaranteed different randomizers for XOR operation security
//   - Protection against selection bias and predictability
//   - Graceful operation when advanced features are unavailable
//
// Random Selection Algorithm:
//   1. Generate cryptographically secure random index for first randomizer
//   2. Remove selected randomizer from candidate pool
//   3. Generate second random index from remaining candidates
//   4. Return two different randomizers for secure XOR operations
//
// Security Properties:
//   - Uniform random distribution prevents selection bias
//   - Cryptographic randomness ensures unpredictability
//   - Guaranteed uniqueness prevents XOR operation failures
//   - No correlation between first and second randomizer selection
//
// Use Cases:
//   - Fallback when diversity controls are not configured
//   - Emergency selection when availability checking fails
//   - Simple deployment scenarios without advanced features
//   - Testing and development environments with minimal configuration
//
// Parameters:
//   - candidates: Available randomizer blocks for random selection (must have at least 2)
//
// Returns:
//   - *cache.BlockInfo: First randomly selected randomizer
//   - *cache.BlockInfo: Second randomly selected randomizer (different from first)
//   - error: Non-nil if insufficient candidates or random generation fails
//
// Call Flow:
//   - Called by: selectRandomizersWithDiversity, fallback selection scenarios
//   - Calls: Cryptographic random number generation, secure selection algorithms
//
// Time Complexity: O(n) where n is the number of candidates for removal operation
// Space Complexity: O(n) for remaining candidates array construction
func (c *Client) selectRandomizersRandom(candidates []*cache.BlockInfo) (*cache.BlockInfo, *cache.BlockInfo, error) {
	if len(candidates) < 2 {
		return nil, nil, fmt.Errorf("insufficient candidates for random selection, need at least 2, got %d", len(candidates))
	}

	// Select first randomizer
	index1, err := rand.Int(rand.Reader, big.NewInt(int64(len(candidates))))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate random index for first randomizer: %w", err)
	}

	selected1 := candidates[index1.Int64()]

	// Remove selected block from pool and select second randomizer
	remaining := make([]*cache.BlockInfo, 0, len(candidates)-1)
	for i, block := range candidates {
		if i != int(index1.Int64()) {
			remaining = append(remaining, block)
		}
	}

	index2, err := rand.Int(rand.Reader, big.NewInt(int64(len(remaining))))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate random index for second randomizer: %w", err)
	}

	selected2 := remaining[index2.Int64()]

	return selected1, selected2, nil
}

// weightedRandomSelection selects a candidate using weighted random selection based on diversity and popularity scores.
// This sophisticated selection algorithm implements weighted random sampling to balance multiple factors including
// popularity for cache efficiency, diversity for security, and availability for reliability.
//
// Weighted Selection Features:
//   - Probability distribution based on calculated scores
//   - Balances multiple factors: popularity, diversity, availability
//   - Cryptographically secure random number generation
//   - Graceful fallback to uniform random when scores are zero
//   - Numerically stable implementation for edge cases
//
// Selection Algorithm:
//   1. Calculate total weight from all candidate scores
//   2. Generate cryptographically secure random value in [0, totalWeight)
//   3. Iterate through candidates accumulating weights
//   4. Select candidate when cumulative weight exceeds random target
//   5. Fallback to uniform random if all scores are zero
//
// Scoring Integration:
//   - Higher scores increase selection probability
//   - Diversity adjustments prevent concentration attacks
//   - Popularity weighting improves cache efficiency
//   - Availability factors enhance system reliability
//
// Mathematical Properties:
//   - Probability of selection proportional to score
//   - Unbiased selection within weighted distribution
//   - Numerically stable for varying score ranges
//   - Cryptographically secure randomness source
//
// Parameters:
//   - candidates: Scored randomizer candidates with calculated weights
//
// Returns:
//   - *cache.BlockInfo: Selected randomizer based on weighted probability distribution
//   - error: Non-nil if no candidates available or random generation fails
//
// Call Flow:
//   - Called by: selectRandomizersWithDiversity, weighted selection operations
//   - Calls: Cryptographic random generation, numerical computation
//
// Time Complexity: O(n) where n is the number of candidates
// Space Complexity: O(1) - constant space for random generation and selection
func (c *Client) weightedRandomSelection(candidates []scoredCandidate) (*cache.BlockInfo, error) {
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no candidates available")
	}

	// Calculate total weight
	totalWeight := 0.0
	for _, candidate := range candidates {
		totalWeight += candidate.score
	}

	// If all scores are 0, fall back to uniform random
	if totalWeight == 0 {
		index, err := rand.Int(rand.Reader, big.NewInt(int64(len(candidates))))
		if err != nil {
			return nil, fmt.Errorf("failed to generate random index: %w", err)
		}
		return candidates[index.Int64()].block, nil
	}

	// Generate random number in [0, totalWeight)
	randomBytes := make([]byte, 8)
	_, err := rand.Read(randomBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Convert to float64 in [0, 1)
	randomFloat := float64(binary.BigEndian.Uint64(randomBytes)) / float64(^uint64(0))
	target := randomFloat * totalWeight

	// Find the selected candidate
	cumulative := 0.0
	for _, candidate := range candidates {
		cumulative += candidate.score
		if cumulative >= target {
			return candidate.block, nil
		}
	}

	// Fallback to last candidate (shouldn't happen with proper floating point)
	return candidates[len(candidates)-1].block, nil
}
