package reuse

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/descriptors"
)

// ReuseEnforcer validates and enforces block reuse policies
type ReuseEnforcer struct {
	pool            *UniversalBlockPool
	policy          *ReusePolicy
	blockRegistry   *BlockRegistry
	auditLog        *AuditLog
	mutex           sync.RWMutex
}

// ReusePolicy defines the mandatory reuse requirements
type ReusePolicy struct {
	MinReuseRatio        float64 `json:"min_reuse_ratio"`         // Minimum % of blocks that must be reused
	PublicDomainRatio    float64 `json:"public_domain_ratio"`     // Minimum % of public domain blocks
	PopularBlockRatio    float64 `json:"popular_block_ratio"`     // Minimum % of popular blocks
	MaxNewBlocks         int     `json:"max_new_blocks"`          // Maximum new blocks per upload
	MinFileAssociations  int     `json:"min_file_associations"`   // Minimum files each block must serve
	EnforcementLevel     string  `json:"enforcement_level"`       // "strict", "moderate", "permissive"
}

// BlockRegistry tracks block usage across files
type BlockRegistry struct {
	blockAssociations map[string][]string // CID -> []fileHash
	fileBlocks        map[string][]string // fileHash -> []CID
	mutex             sync.RWMutex
}

// AuditLog records enforcement decisions and violations
type AuditLog struct {
	entries []AuditEntry
	mutex   sync.RWMutex
}

// AuditEntry represents a single audit log entry
type AuditEntry struct {
	Timestamp    time.Time              `json:"timestamp"`
	Action       string                 `json:"action"`       // "accept", "reject", "warning"
	FileHash     string                 `json:"file_hash"`
	Descriptor   string                 `json:"descriptor"`
	Reason       string                 `json:"reason"`
	PolicyCheck  map[string]interface{} `json:"policy_check"`
	BlocksUsed   []string               `json:"blocks_used"`
}

// ValidationResult contains the result of reuse validation
type ValidationResult struct {
	Valid              bool                   `json:"valid"`
	ReuseRatio         float64                `json:"reuse_ratio"`
	PublicDomainRatio  float64                `json:"public_domain_ratio"`
	PopularBlockRatio  float64                `json:"popular_block_ratio"`
	NewBlockCount      int                    `json:"new_block_count"`
	Violations         []string               `json:"violations"`
	Warnings           []string               `json:"warnings"`
	BlockAnalysis      map[string]interface{} `json:"block_analysis"`
}

// ReuseProof provides cryptographic evidence of block reuse compliance
type ReuseProof struct {
	FileHash          string            `json:"file_hash"`
	DescriptorCID     string            `json:"descriptor_cid"`
	BlockCIDs         []string          `json:"block_cids"`
	ReuseEvidence     []ReuseEvidence   `json:"reuse_evidence"`
	PublicDomainProof []string          `json:"public_domain_proof"`
	Timestamp         time.Time         `json:"timestamp"`
	Signature         string            `json:"signature"`
}

// ReuseEvidence proves a block is used in multiple files
type ReuseEvidence struct {
	BlockCID            string    `json:"block_cid"`
	FileAssociations    []string  `json:"file_associations"`
	FirstUse            time.Time `json:"first_use"`
	TotalUsages         int64     `json:"total_usages"`
	IsPublicDomain      bool      `json:"is_public_domain"`
	PopularityScore     float64   `json:"popularity_score"`
}

// DefaultReusePolicy returns the default enforcement policy
func DefaultReusePolicy() *ReusePolicy {
	return &ReusePolicy{
		MinReuseRatio:       0.5,  // 50% of blocks must be reused
		PublicDomainRatio:   0.3,  // 30% must be public domain
		PopularBlockRatio:   0.4,  // 40% should be popular blocks
		MaxNewBlocks:        10,   // Maximum 10 new blocks per upload
		MinFileAssociations: 3,    // Each block must serve at least 3 files
		EnforcementLevel:    "strict",
	}
}

// NewReuseEnforcer creates a new reuse enforcer
func NewReuseEnforcer(pool *UniversalBlockPool, policy *ReusePolicy) *ReuseEnforcer {
	if policy == nil {
		policy = DefaultReusePolicy()
	}

	return &ReuseEnforcer{
		pool:   pool,
		policy: policy,
		blockRegistry: &BlockRegistry{
			blockAssociations: make(map[string][]string),
			fileBlocks:        make(map[string][]string),
		},
		auditLog: &AuditLog{
			entries: make([]AuditEntry, 0),
		},
	}
}

// ValidateUpload validates that an upload meets reuse requirements
func (enforcer *ReuseEnforcer) ValidateUpload(descriptor *descriptors.Descriptor, fileData []byte) (*ValidationResult, error) {
	enforcer.mutex.RLock()
	defer enforcer.mutex.RUnlock()

	result := &ValidationResult{
		Valid:         true,
		Violations:    make([]string, 0),
		Warnings:      make([]string, 0),
		BlockAnalysis: make(map[string]interface{}),
	}

	// Calculate file hash for tracking
	fileHash := enforcer.calculateFileHash(fileData)
	
	// Analyze blocks used in descriptor
	blockCIDs := enforcer.extractBlockCIDs(descriptor)
	result.BlockAnalysis["total_blocks"] = len(blockCIDs)

	// Check reuse ratio
	if err := enforcer.checkReuseRatio(blockCIDs, result); err != nil {
		return nil, fmt.Errorf("reuse ratio check failed: %w", err)
	}

	// Check public domain ratio
	if err := enforcer.checkPublicDomainRatio(blockCIDs, result); err != nil {
		return nil, fmt.Errorf("public domain ratio check failed: %w", err)
	}

	// Check popular block usage
	if err := enforcer.checkPopularBlockRatio(blockCIDs, result); err != nil {
		return nil, fmt.Errorf("popular block ratio check failed: %w", err)
	}

	// Check new block limit
	if err := enforcer.checkNewBlockLimit(blockCIDs, result); err != nil {
		return nil, fmt.Errorf("new block limit check failed: %w", err)
	}

	// Check minimum file associations for reused blocks
	if err := enforcer.checkMinFileAssociations(blockCIDs, result); err != nil {
		return nil, fmt.Errorf("file associations check failed: %w", err)
	}

	// Determine overall validity based on enforcement level
	result.Valid = enforcer.determineValidity(result)

	// Log the validation result
	enforcer.logValidation(fileHash, descriptor, result)

	return result, nil
}

// extractBlockCIDs extracts all block CIDs from a descriptor
func (enforcer *ReuseEnforcer) extractBlockCIDs(descriptor *descriptors.Descriptor) []string {
	var cids []string
	
	for _, blockPair := range descriptor.Blocks {
		cids = append(cids, blockPair.DataCID)
		cids = append(cids, blockPair.RandomizerCID1)
		if blockPair.RandomizerCID2 != "" {
			cids = append(cids, blockPair.RandomizerCID2)
		}
	}
	
	// Remove duplicates
	seen := make(map[string]bool)
	uniqueCIDs := make([]string, 0)
	for _, cid := range cids {
		if !seen[cid] {
			uniqueCIDs = append(uniqueCIDs, cid)
			seen[cid] = true
		}
	}
	
	return uniqueCIDs
}

// checkReuseRatio validates that sufficient blocks are being reused
func (enforcer *ReuseEnforcer) checkReuseRatio(blockCIDs []string, result *ValidationResult) error {
	reusedCount := 0
	newCount := 0

	for _, cid := range blockCIDs {
		if enforcer.isBlockReused(cid) {
			reusedCount++
		} else {
			newCount++
		}
	}

	result.ReuseRatio = float64(reusedCount) / float64(len(blockCIDs))
	result.NewBlockCount = newCount

	if result.ReuseRatio < enforcer.policy.MinReuseRatio {
		violation := fmt.Sprintf("insufficient reuse ratio: %.2f%% (required: %.2f%%)", 
			result.ReuseRatio*100, enforcer.policy.MinReuseRatio*100)
		result.Violations = append(result.Violations, violation)
	}

	if newCount > enforcer.policy.MaxNewBlocks {
		violation := fmt.Sprintf("too many new blocks: %d (maximum: %d)", 
			newCount, enforcer.policy.MaxNewBlocks)
		result.Violations = append(result.Violations, violation)
	}

	return nil
}

// checkPublicDomainRatio validates public domain block usage
func (enforcer *ReuseEnforcer) checkPublicDomainRatio(blockCIDs []string, result *ValidationResult) error {
	publicDomainCount := 0

	for _, cid := range blockCIDs {
		if enforcer.isPublicDomainBlock(cid) {
			publicDomainCount++
		}
	}

	result.PublicDomainRatio = float64(publicDomainCount) / float64(len(blockCIDs))

	if result.PublicDomainRatio < enforcer.policy.PublicDomainRatio {
		violation := fmt.Sprintf("insufficient public domain blocks: %.2f%% (required: %.2f%%)", 
			result.PublicDomainRatio*100, enforcer.policy.PublicDomainRatio*100)
		result.Violations = append(result.Violations, violation)
	}

	return nil
}

// checkPopularBlockRatio validates popular block usage
func (enforcer *ReuseEnforcer) checkPopularBlockRatio(blockCIDs []string, result *ValidationResult) error {
	popularCount := 0

	for _, cid := range blockCIDs {
		if enforcer.isPopularBlock(cid) {
			popularCount++
		}
	}

	result.PopularBlockRatio = float64(popularCount) / float64(len(blockCIDs))

	if result.PopularBlockRatio < enforcer.policy.PopularBlockRatio {
		warning := fmt.Sprintf("low popular block usage: %.2f%% (recommended: %.2f%%)", 
			result.PopularBlockRatio*100, enforcer.policy.PopularBlockRatio*100)
		result.Warnings = append(result.Warnings, warning)
	}

	return nil
}

// checkNewBlockLimit validates that new block creation is within limits
func (enforcer *ReuseEnforcer) checkNewBlockLimit(blockCIDs []string, result *ValidationResult) error {
	// Already checked in checkReuseRatio, but we can add additional logic here
	return nil
}

// checkMinFileAssociations validates that reused blocks serve multiple files
func (enforcer *ReuseEnforcer) checkMinFileAssociations(blockCIDs []string, result *ValidationResult) error {
	enforcer.blockRegistry.mutex.RLock()
	defer enforcer.blockRegistry.mutex.RUnlock()

	for _, cid := range blockCIDs {
		associations := enforcer.blockRegistry.blockAssociations[cid]
		if len(associations) > 0 && len(associations) < enforcer.policy.MinFileAssociations {
			warning := fmt.Sprintf("block %s has insufficient file associations: %d (minimum: %d)", 
				cid[:8], len(associations), enforcer.policy.MinFileAssociations)
			result.Warnings = append(result.Warnings, warning)
		}
	}

	return nil
}

// isBlockReused checks if a block is already in the universal pool
func (enforcer *ReuseEnforcer) isBlockReused(cid string) bool {
	if !enforcer.pool.IsInitialized() {
		return false
	}

	enforcer.pool.mutex.RLock()
	defer enforcer.pool.mutex.RUnlock()
	
	_, exists := enforcer.pool.blocks[cid]
	return exists
}

// isPublicDomainBlock checks if a block is from public domain content
func (enforcer *ReuseEnforcer) isPublicDomainBlock(cid string) bool {
	if !enforcer.pool.IsInitialized() {
		return false
	}

	enforcer.pool.mutex.RLock()
	defer enforcer.pool.mutex.RUnlock()
	
	return enforcer.pool.publicDomainCIDs[cid]
}

// isPopularBlock checks if a block is considered popular
func (enforcer *ReuseEnforcer) isPopularBlock(cid string) bool {
	if !enforcer.pool.IsInitialized() {
		return false
	}

	enforcer.pool.mutex.RLock()
	defer enforcer.pool.mutex.RUnlock()

	block := enforcer.pool.blocks[cid]
	if block == nil {
		return false
	}

	// Consider blocks with popularity score > 0.5 as popular
	return block.PopularityScore > 0.5
}

// determineValidity determines overall upload validity based on enforcement level
func (enforcer *ReuseEnforcer) determineValidity(result *ValidationResult) bool {
	switch enforcer.policy.EnforcementLevel {
	case "strict":
		// No violations allowed
		return len(result.Violations) == 0
	case "moderate":
		// Allow some violations but not critical ones
		criticalViolations := 0
		for _, violation := range result.Violations {
			if enforcer.isCriticalViolation(violation) {
				criticalViolations++
			}
		}
		return criticalViolations == 0
	case "permissive":
		// Allow violations but log them
		return true
	default:
		// Default to strict
		return len(result.Violations) == 0
	}
}

// isCriticalViolation determines if a violation is critical
func (enforcer *ReuseEnforcer) isCriticalViolation(violation string) bool {
	// Critical violations that always block uploads
	criticalKeywords := []string{
		"insufficient reuse ratio",
		"insufficient public domain blocks",
		"too many new blocks",
	}

	for _, keyword := range criticalKeywords {
		if len(violation) >= len(keyword) && violation[:len(keyword)] == keyword {
			return true
		}
	}

	return false
}

// RegisterFileBlocks registers blocks used by a file after successful upload
func (enforcer *ReuseEnforcer) RegisterFileBlocks(fileHash string, blockCIDs []string) error {
	enforcer.blockRegistry.mutex.Lock()
	defer enforcer.blockRegistry.mutex.Unlock()

	// Register file -> blocks mapping
	enforcer.blockRegistry.fileBlocks[fileHash] = blockCIDs

	// Register block -> files mapping
	for _, cid := range blockCIDs {
		if _, exists := enforcer.blockRegistry.blockAssociations[cid]; !exists {
			enforcer.blockRegistry.blockAssociations[cid] = make([]string, 0)
		}
		
		// Add file association if not already present
		found := false
		for _, existingFile := range enforcer.blockRegistry.blockAssociations[cid] {
			if existingFile == fileHash {
				found = true
				break
			}
		}
		
		if !found {
			enforcer.blockRegistry.blockAssociations[cid] = append(
				enforcer.blockRegistry.blockAssociations[cid], fileHash)
		}
	}

	return nil
}

// GenerateReuseProof creates cryptographic proof of reuse compliance
func (enforcer *ReuseEnforcer) GenerateReuseProof(fileHash, descriptorCID string, blockCIDs []string) (*ReuseProof, error) {
	enforcer.mutex.RLock()
	defer enforcer.mutex.RUnlock()

	proof := &ReuseProof{
		FileHash:          fileHash,
		DescriptorCID:     descriptorCID,
		BlockCIDs:         blockCIDs,
		ReuseEvidence:     make([]ReuseEvidence, 0),
		PublicDomainProof: make([]string, 0),
		Timestamp:         time.Now(),
	}

	// Generate evidence for each block
	for _, cid := range blockCIDs {
		evidence := ReuseEvidence{
			BlockCID:        cid,
			IsPublicDomain:  enforcer.isPublicDomainBlock(cid),
			PopularityScore: enforcer.getBlockPopularity(cid),
		}

		// Get file associations
		enforcer.blockRegistry.mutex.RLock()
		associations := enforcer.blockRegistry.blockAssociations[cid]
		evidence.FileAssociations = make([]string, len(associations))
		copy(evidence.FileAssociations, associations)
		evidence.TotalUsages = int64(len(associations))
		enforcer.blockRegistry.mutex.RUnlock()

		// Get usage statistics from pool
		if block := enforcer.pool.blocks[cid]; block != nil {
			evidence.FirstUse = block.CreatedAt
			evidence.TotalUsages = block.UsageCount
		}

		proof.ReuseEvidence = append(proof.ReuseEvidence, evidence)

		// Add to public domain proof if applicable
		if evidence.IsPublicDomain {
			proof.PublicDomainProof = append(proof.PublicDomainProof, cid)
		}
	}

	// Generate cryptographic signature
	signature, err := enforcer.signProof(proof)
	if err != nil {
		return nil, fmt.Errorf("failed to sign proof: %w", err)
	}
	proof.Signature = signature

	return proof, nil
}

// getBlockPopularity gets popularity score for a block
func (enforcer *ReuseEnforcer) getBlockPopularity(cid string) float64 {
	if !enforcer.pool.IsInitialized() {
		return 0.0
	}

	enforcer.pool.mutex.RLock()
	defer enforcer.pool.mutex.RUnlock()

	if block := enforcer.pool.blocks[cid]; block != nil {
		return block.PopularityScore
	}

	return 0.0
}

// signProof generates a cryptographic signature for the proof
func (enforcer *ReuseEnforcer) signProof(proof *ReuseProof) (string, error) {
	// Simple hash-based signature for now
	// In production, this would use proper cryptographic signing
	data := fmt.Sprintf("%s-%s-%v-%d", 
		proof.FileHash, proof.DescriptorCID, proof.BlockCIDs, proof.Timestamp.Unix())
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:]), nil
}

// calculateFileHash calculates hash of file content for tracking
func (enforcer *ReuseEnforcer) calculateFileHash(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// logValidation logs a validation result to the audit log
func (enforcer *ReuseEnforcer) logValidation(fileHash string, descriptor *descriptors.Descriptor, result *ValidationResult) {
	enforcer.auditLog.mutex.Lock()
	defer enforcer.auditLog.mutex.Unlock()

	action := "accept"
	if !result.Valid {
		action = "reject"
	} else if len(result.Warnings) > 0 {
		action = "warning"
	}

	reason := "compliance check"
	if !result.Valid {
		reason = fmt.Sprintf("violations: %v", result.Violations)
	} else if len(result.Warnings) > 0 {
		reason = fmt.Sprintf("warnings: %v", result.Warnings)
	}

	entry := AuditEntry{
		Timestamp:   time.Now(),
		Action:      action,
		FileHash:    fileHash,
		Descriptor:  descriptor.Filename,
		Reason:      reason,
		PolicyCheck: map[string]interface{}{
			"reuse_ratio":         result.ReuseRatio,
			"public_domain_ratio": result.PublicDomainRatio,
			"popular_block_ratio": result.PopularBlockRatio,
			"new_block_count":     result.NewBlockCount,
		},
		BlocksUsed: enforcer.extractBlockCIDs(descriptor),
	}

	enforcer.auditLog.entries = append(enforcer.auditLog.entries, entry)
}

// GetAuditLog returns recent audit log entries
func (enforcer *ReuseEnforcer) GetAuditLog(limit int) []AuditEntry {
	enforcer.auditLog.mutex.RLock()
	defer enforcer.auditLog.mutex.RUnlock()

	if limit <= 0 || limit > len(enforcer.auditLog.entries) {
		limit = len(enforcer.auditLog.entries)
	}

	// Return most recent entries
	start := len(enforcer.auditLog.entries) - limit
	entries := make([]AuditEntry, limit)
	copy(entries, enforcer.auditLog.entries[start:])

	return entries
}

// GetStatistics returns enforcement statistics
func (enforcer *ReuseEnforcer) GetStatistics() map[string]interface{} {
	enforcer.auditLog.mutex.RLock()
	defer enforcer.auditLog.mutex.RUnlock()

	enforcer.blockRegistry.mutex.RLock()
	defer enforcer.blockRegistry.mutex.RUnlock()

	stats := make(map[string]interface{})

	// Audit statistics
	accepted := 0
	rejected := 0
	warnings := 0

	for _, entry := range enforcer.auditLog.entries {
		switch entry.Action {
		case "accept":
			accepted++
		case "reject":
			rejected++
		case "warning":
			warnings++
		}
	}

	stats["total_validations"] = len(enforcer.auditLog.entries)
	stats["accepted"] = accepted
	stats["rejected"] = rejected
	stats["warnings"] = warnings

	if len(enforcer.auditLog.entries) > 0 {
		stats["acceptance_rate"] = float64(accepted) / float64(len(enforcer.auditLog.entries))
	}

	// Registry statistics
	stats["total_files"] = len(enforcer.blockRegistry.fileBlocks)
	stats["total_block_associations"] = len(enforcer.blockRegistry.blockAssociations)

	// Block reuse statistics
	totalAssociations := 0
	for _, associations := range enforcer.blockRegistry.blockAssociations {
		totalAssociations += len(associations)
	}

	if len(enforcer.blockRegistry.blockAssociations) > 0 {
		stats["avg_block_reuse"] = float64(totalAssociations) / float64(len(enforcer.blockRegistry.blockAssociations))
	}

	return stats
}