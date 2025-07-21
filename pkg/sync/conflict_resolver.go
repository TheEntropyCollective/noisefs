package sync

import (
	"fmt"
	"sync"
	"time"
)

// ConflictResolver handles synchronization conflicts using various resolution strategies
type ConflictResolver struct {
	defaultStrategy ConflictResolution
	strategies      map[ConflictResolution]ConflictStrategy
}

// ConflictStrategy defines the interface for conflict resolution strategies
type ConflictStrategy interface {
	ResolveConflict(conflict *Conflict) (*ConflictResult, error)
	GetStrategyName() string
}

// ConflictResult represents the result of conflict resolution
type ConflictResult struct {
	Resolution    ConflictResolution `json:"resolution"`
	Action        ConflictAction     `json:"action"`
	LocalPath     string             `json:"local_path,omitempty"`
	RemotePath    string             `json:"remote_path,omitempty"`
	RenamedPath   string             `json:"renamed_path,omitempty"`
	Message       string             `json:"message"`
	RequiresInput bool               `json:"requires_input"`
	UserPrompt    string             `json:"user_prompt,omitempty"`
}

// ConflictAction represents the action to take for conflict resolution
type ConflictAction string

const (
	ActionUseLocal   ConflictAction = "use_local"
	ActionUseRemote  ConflictAction = "use_remote"
	ActionMerge      ConflictAction = "merge"
	ActionRename     ConflictAction = "rename"
	ActionPromptUser ConflictAction = "prompt_user"
	ActionSkip       ConflictAction = "skip"
)

// NewConflictResolver creates a new conflict resolver with the specified default strategy
func NewConflictResolver(defaultStrategy ConflictResolution) (*ConflictResolver, error) {
	resolver := &ConflictResolver{
		defaultStrategy: defaultStrategy,
		strategies:      make(map[ConflictResolution]ConflictStrategy),
	}

	// Register built-in strategies
	resolver.strategies[ConflictResolveLocal] = &LocalWinsStrategy{}
	resolver.strategies[ConflictResolveRemote] = &RemoteWinsStrategy{}
	resolver.strategies[ConflictResolveTimestamp] = &TimestampStrategy{}
	resolver.strategies[ConflictResolvePrompt] = &PromptStrategy{}

	// Validate default strategy
	if _, exists := resolver.strategies[defaultStrategy]; !exists {
		return nil, fmt.Errorf("unsupported conflict resolution strategy: %s", defaultStrategy)
	}

	return resolver, nil
}

// ResolveConflict resolves a conflict using the appropriate strategy
func (cr *ConflictResolver) ResolveConflict(conflict *Conflict) (*ConflictResult, error) {
	strategy := cr.strategies[conflict.Resolution]
	if strategy == nil {
		// Use default strategy
		strategy = cr.strategies[cr.defaultStrategy]
	}

	if strategy == nil {
		return nil, fmt.Errorf("no strategy available for conflict resolution")
	}

	result, err := strategy.ResolveConflict(conflict)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve conflict: %w", err)
	}

	return result, nil
}

// SetStrategy sets a custom strategy for a resolution type
func (cr *ConflictResolver) SetStrategy(resolution ConflictResolution, strategy ConflictStrategy) {
	cr.strategies[resolution] = strategy
}

// GetAvailableStrategies returns a list of available conflict resolution strategies
func (cr *ConflictResolver) GetAvailableStrategies() []ConflictResolution {
	strategies := make([]ConflictResolution, 0, len(cr.strategies))
	for resolution := range cr.strategies {
		strategies = append(strategies, resolution)
	}
	return strategies
}

// LocalWinsStrategy always chooses the local version
type LocalWinsStrategy struct{}

func (s *LocalWinsStrategy) ResolveConflict(conflict *Conflict) (*ConflictResult, error) {
	return &ConflictResult{
		Resolution:    ConflictResolveLocal,
		Action:        ActionUseLocal,
		LocalPath:     conflict.LocalPath,
		RemotePath:    conflict.RemotePath,
		Message:       "Using local version (local wins strategy)",
		RequiresInput: false,
	}, nil
}

func (s *LocalWinsStrategy) GetStrategyName() string {
	return "local_wins"
}

// RemoteWinsStrategy always chooses the remote version
type RemoteWinsStrategy struct{}

func (s *RemoteWinsStrategy) ResolveConflict(conflict *Conflict) (*ConflictResult, error) {
	return &ConflictResult{
		Resolution:    ConflictResolveRemote,
		Action:        ActionUseRemote,
		LocalPath:     conflict.LocalPath,
		RemotePath:    conflict.RemotePath,
		Message:       "Using remote version (remote wins strategy)",
		RequiresInput: false,
	}, nil
}

func (s *RemoteWinsStrategy) GetStrategyName() string {
	return "remote_wins"
}

// TimestampStrategy chooses the version with the latest modification time
type TimestampStrategy struct{}

func (s *TimestampStrategy) ResolveConflict(conflict *Conflict) (*ConflictResult, error) {
	localModTime := conflict.LocalMetadata.ModTime
	remoteModTime := conflict.RemoteMetadata.ModTime

	if localModTime.After(remoteModTime) {
		return &ConflictResult{
			Resolution:    ConflictResolveTimestamp,
			Action:        ActionUseLocal,
			LocalPath:     conflict.LocalPath,
			RemotePath:    conflict.RemotePath,
			Message:       fmt.Sprintf("Using local version (newer: %v vs %v)", localModTime, remoteModTime),
			RequiresInput: false,
		}, nil
	} else if remoteModTime.After(localModTime) {
		return &ConflictResult{
			Resolution:    ConflictResolveTimestamp,
			Action:        ActionUseRemote,
			LocalPath:     conflict.LocalPath,
			RemotePath:    conflict.RemotePath,
			Message:       fmt.Sprintf("Using remote version (newer: %v vs %v)", remoteModTime, localModTime),
			RequiresInput: false,
		}, nil
	} else {
		// Same timestamp, fall back to size comparison
		if conflict.LocalMetadata.Size > conflict.RemoteMetadata.Size {
			return &ConflictResult{
				Resolution:    ConflictResolveTimestamp,
				Action:        ActionUseLocal,
				LocalPath:     conflict.LocalPath,
				RemotePath:    conflict.RemotePath,
				Message:       "Using local version (same timestamp, larger size)",
				RequiresInput: false,
			}, nil
		} else {
			return &ConflictResult{
				Resolution:    ConflictResolveTimestamp,
				Action:        ActionUseRemote,
				LocalPath:     conflict.LocalPath,
				RemotePath:    conflict.RemotePath,
				Message:       "Using remote version (same timestamp, larger or equal size)",
				RequiresInput: false,
			}, nil
		}
	}
}

func (s *TimestampStrategy) GetStrategyName() string {
	return "timestamp"
}

// PromptStrategy requires user input for conflict resolution
type PromptStrategy struct{}

func (s *PromptStrategy) ResolveConflict(conflict *Conflict) (*ConflictResult, error) {
	prompt := s.buildPrompt(conflict)

	return &ConflictResult{
		Resolution:    ConflictResolvePrompt,
		Action:        ActionPromptUser,
		LocalPath:     conflict.LocalPath,
		RemotePath:    conflict.RemotePath,
		Message:       "User input required for conflict resolution",
		RequiresInput: true,
		UserPrompt:    prompt,
	}, nil
}

func (s *PromptStrategy) buildPrompt(conflict *Conflict) string {
	prompt := fmt.Sprintf("Conflict detected for: %s\n", conflict.LocalPath)
	prompt += fmt.Sprintf("Conflict type: %s\n", conflict.ConflictType)
	prompt += fmt.Sprintf("Conflict ID: %s\n\n", conflict.ID)

	prompt += "Local version:\n"
	prompt += fmt.Sprintf("  Size: %d bytes\n", conflict.LocalMetadata.Size)
	prompt += fmt.Sprintf("  Modified: %v\n", conflict.LocalMetadata.ModTime)
	if conflict.LocalMetadata.Checksum != "" {
		prompt += fmt.Sprintf("  Checksum: %s\n", conflict.LocalMetadata.Checksum)
	}

	prompt += "\nRemote version:\n"
	prompt += fmt.Sprintf("  Size: %d bytes\n", conflict.RemoteMetadata.Size)
	prompt += fmt.Sprintf("  Modified: %v\n", conflict.RemoteMetadata.ModTime)
	prompt += fmt.Sprintf("  CID: %s\n", conflict.RemoteMetadata.DescriptorCID)

	prompt += "\nChoose resolution:\n"
	prompt += "1. Use local version\n"
	prompt += "2. Use remote version\n"
	prompt += "3. Rename both (create copies)\n"
	prompt += "4. Skip this conflict\n"
	prompt += "\nEnter your choice (1-4): "

	return prompt
}

func (s *PromptStrategy) GetStrategyName() string {
	return "prompt"
}

// RenameStrategy creates renamed copies of both versions
type RenameStrategy struct{}

func (s *RenameStrategy) ResolveConflict(conflict *Conflict) (*ConflictResult, error) {
	timestamp := time.Now().Format("20060102_150405")

	// Generate unique names for both versions
	localRename := fmt.Sprintf("%s.local.%s", conflict.LocalPath, timestamp)
	remoteRename := fmt.Sprintf("%s.remote.%s", conflict.LocalPath, timestamp)

	return &ConflictResult{
		Resolution:    ConflictResolveLocal, // Arbitrary choice
		Action:        ActionRename,
		LocalPath:     conflict.LocalPath,
		RemotePath:    conflict.RemotePath,
		RenamedPath:   localRename,
		Message:       fmt.Sprintf("Renamed local to %s, remote to %s", localRename, remoteRename),
		RequiresInput: false,
	}, nil
}

func (s *RenameStrategy) GetStrategyName() string {
	return "rename"
}

// ConflictDetector detects conflicts between local and remote changes
type ConflictDetector struct{}

// DetectConflict detects if there's a conflict between local and remote metadata
func (cd *ConflictDetector) DetectConflict(localPath, remotePath string, localMeta FileMetadata, remoteMeta RemoteMetadata) *Conflict {
	// Check if both versions exist and are different
	if cd.hasMetadataConflict(localMeta, remoteMeta) {
		conflictType := cd.determineConflictType(localMeta, remoteMeta)

		return &Conflict{
			ID:             fmt.Sprintf("conflict_%d", time.Now().UnixNano()),
			LocalPath:      localPath,
			RemotePath:     remotePath,
			LocalMetadata:  localMeta,
			RemoteMetadata: remoteMeta,
			ConflictType:   conflictType,
			Timestamp:      time.Now(),
		}
	}

	return nil
}

// hasMetadataConflict checks if metadata indicates a conflict
func (cd *ConflictDetector) hasMetadataConflict(localMeta FileMetadata, remoteMeta RemoteMetadata) bool {
	// Different sizes or modification times suggest conflict
	return localMeta.Size != remoteMeta.Size ||
		!localMeta.ModTime.Equal(remoteMeta.ModTime) ||
		localMeta.IsDir != remoteMeta.IsDir
}

// determineConflictType determines the type of conflict
func (cd *ConflictDetector) determineConflictType(localMeta FileMetadata, remoteMeta RemoteMetadata) ConflictType {
	// Check if file/directory type changed
	if localMeta.IsDir != remoteMeta.IsDir {
		return ConflictTypeTypeChanged
	}

	// Check if one was deleted (size 0 could indicate deletion)
	if localMeta.Size == 0 && remoteMeta.Size > 0 {
		return ConflictTypeDeletedLocal
	}
	if remoteMeta.Size == 0 && localMeta.Size > 0 {
		return ConflictTypeDeletedRemote
	}

	// Default to both modified
	return ConflictTypeBothModified
}

// ConflictHistory tracks resolved conflicts
type ConflictHistory struct {
	conflicts []ResolvedConflict
	mu        sync.RWMutex
}

// ResolvedConflict represents a conflict that has been resolved
type ResolvedConflict struct {
	Conflict   *Conflict       `json:"conflict"`
	Resolution *ConflictResult `json:"resolution"`
	ResolvedAt time.Time       `json:"resolved_at"`
	ResolvedBy string          `json:"resolved_by"`
}

// NewConflictHistory creates a new conflict history tracker
func NewConflictHistory() *ConflictHistory {
	return &ConflictHistory{
		conflicts: make([]ResolvedConflict, 0),
	}
}

// AddResolvedConflict adds a resolved conflict to the history
func (ch *ConflictHistory) AddResolvedConflict(conflict *Conflict, result *ConflictResult, resolvedBy string) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	resolved := ResolvedConflict{
		Conflict:   conflict,
		Resolution: result,
		ResolvedAt: time.Now(),
		ResolvedBy: resolvedBy,
	}

	ch.conflicts = append(ch.conflicts, resolved)

	// Keep only last 1000 conflicts
	if len(ch.conflicts) > 1000 {
		ch.conflicts = ch.conflicts[len(ch.conflicts)-1000:]
	}
}

// GetRecentConflicts returns the most recent resolved conflicts
func (ch *ConflictHistory) GetRecentConflicts(limit int) []ResolvedConflict {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if limit <= 0 || limit > len(ch.conflicts) {
		limit = len(ch.conflicts)
	}

	// Return the last 'limit' conflicts
	start := len(ch.conflicts) - limit
	result := make([]ResolvedConflict, limit)
	copy(result, ch.conflicts[start:])

	return result
}

// GetConflictStats returns statistics about resolved conflicts
func (ch *ConflictHistory) GetConflictStats() ConflictStats {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	stats := ConflictStats{
		TotalConflicts: len(ch.conflicts),
		ByType:         make(map[ConflictType]int),
		ByResolution:   make(map[ConflictResolution]int),
	}

	for _, resolved := range ch.conflicts {
		stats.ByType[resolved.Conflict.ConflictType]++
		stats.ByResolution[resolved.Resolution.Resolution]++
	}

	return stats
}

// ConflictStats represents statistics about conflicts
type ConflictStats struct {
	TotalConflicts int                        `json:"total_conflicts"`
	ByType         map[ConflictType]int       `json:"by_type"`
	ByResolution   map[ConflictResolution]int `json:"by_resolution"`
}
