// Package noisefs provides randomizer selection and management functionality.
// This file handles the selection of randomizer blocks for XOR operations,
// implementing diversity controls, weighted selection, and cache-aware strategies.
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

// SelectRandomizers selects or generates two randomizer blocks for 3-tuple XOR anonymization
// Returns: randomizer1, cid1, randomizer2, cid2, bytesStored, error
// bytesStored indicates how many new bytes were stored (0 if both from cache)
func (c *Client) SelectRandomizers(blockSize int) (*blocks.Block, string, *blocks.Block, string, int64, error) {
	var totalNewStorage int64 = 0

	// Try to get popular blocks from cache first
	randomizers, err := c.cache.GetRandomizers(20) // Get more blocks for better selection
	if err == nil && len(randomizers) > 0 {
		// Filter by matching size
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

// scoredCandidate represents a candidate randomizer with its diversity score
type scoredCandidate struct {
	block *cache.BlockInfo
	score float64
}

// selectRandomizersWithDiversity selects two randomizers using diversity controls
func (c *Client) selectRandomizersWithDiversity(candidates []*cache.BlockInfo) (*cache.BlockInfo, *cache.BlockInfo, error) {
	if len(candidates) < 2 {
		return nil, nil, fmt.Errorf("need at least 2 candidates, got %d", len(candidates))
	}

	// If no diversity controls, fall back to random selection
	if c.diversityControls == nil {
		return c.selectRandomizersRandom(candidates)
	}

	// Score all candidates using diversity controls
	scored := make([]scoredCandidate, len(candidates))
	for i, candidate := range candidates {
		baseScore := float64(candidate.Popularity + 1) // Base score from popularity
		adjustedScore := c.diversityControls.CalculateRandomizerScore(candidate.CID, baseScore)
		scored[i] = scoredCandidate{
			block: candidate,
			score: adjustedScore,
		}
	}

	// Use weighted random selection based on scores
	selected1, err := c.weightedRandomSelection(scored)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to select first randomizer: %w", err)
	}

	// Remove selected candidate and select second
	remaining := make([]scoredCandidate, 0, len(scored)-1)
	for _, candidate := range scored {
		if candidate.block.CID != selected1.CID {
			remaining = append(remaining, candidate)
		}
	}

	selected2, err := c.weightedRandomSelection(remaining)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to select second randomizer: %w", err)
	}

	return selected1, selected2, nil
}

// selectRandomizersWithDiversityAndAvailability selects two randomizers using diversity controls and availability checking
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

// selectRandomizersRandom provides fallback random selection
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

// weightedRandomSelection selects a candidate using weighted random selection
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
