package sync

import (
	"path/filepath"
	"sort"
	"strings"
)

// MoveDetector provides functionality for detecting move and rename operations
type MoveDetector struct {
	// Configuration for move detection sensitivity
	MinConfidence float64
}

// NewMoveDetector creates a new move detector with default settings
func NewMoveDetector() *MoveDetector {
	return &MoveDetector{
		MinConfidence: 0.7, // Minimum confidence threshold for move detection
	}
}

// DetectMoves detects move and rename operations between two state snapshots
func (md *MoveDetector) DetectMoves(oldSnapshot, newSnapshot *StateSnapshot) []MoveCandidate {
	var candidates []MoveCandidate

	// Detect local moves
	localMoves := md.detectLocalMoves(oldSnapshot.LocalFiles, newSnapshot.LocalFiles)
	candidates = append(candidates, localMoves...)

	// Detect remote moves
	remoteMoves := md.detectRemoteMoves(oldSnapshot.RemoteFiles, newSnapshot.RemoteFiles)
	candidates = append(candidates, remoteMoves...)

	// Filter candidates by confidence threshold
	var filteredCandidates []MoveCandidate
	for _, candidate := range candidates {
		if candidate.Confidence >= md.MinConfidence {
			filteredCandidates = append(filteredCandidates, candidate)
		}
	}

	// Sort by confidence (highest first)
	sort.Slice(filteredCandidates, func(i, j int) bool {
		return filteredCandidates[i].Confidence > filteredCandidates[j].Confidence
	})

	return filteredCandidates
}

// detectLocalMoves detects moves in local files
func (md *MoveDetector) detectLocalMoves(oldFiles, newFiles map[string]FileMetadata) []MoveCandidate {
	var candidates []MoveCandidate

	// Find deleted files (potential sources of moves)
	deletedFiles := make(map[string]FileMetadata)
	for path, file := range oldFiles {
		if _, exists := newFiles[path]; !exists {
			deletedFiles[path] = file
		}
	}

	// Find created files (potential targets of moves)
	createdFiles := make(map[string]FileMetadata)
	for path, file := range newFiles {
		if _, exists := oldFiles[path]; !exists {
			createdFiles[path] = file
		}
	}

	// Match deleted and created files for potential moves
	for deletedPath, deletedFile := range deletedFiles {
		for createdPath, createdFile := range createdFiles {
			confidence := md.calculateLocalMoveConfidence(deletedFile, createdFile)
			if confidence > 0 {
				reason := md.buildMoveReason(deletedFile, createdFile, confidence)
				candidates = append(candidates, MoveCandidate{
					OldPath:    deletedPath,
					NewPath:    createdPath,
					Confidence: confidence,
					Reason:     reason,
					IsLocal:    true,
					Metadata:   createdFile,
				})
			}
		}
	}

	return candidates
}

// detectRemoteMoves detects moves in remote files
func (md *MoveDetector) detectRemoteMoves(oldFiles, newFiles map[string]RemoteMetadata) []MoveCandidate {
	var candidates []MoveCandidate

	// Find deleted files (potential sources of moves)
	deletedFiles := make(map[string]RemoteMetadata)
	for path, file := range oldFiles {
		if _, exists := newFiles[path]; !exists {
			deletedFiles[path] = file
		}
	}

	// Find created files (potential targets of moves)
	createdFiles := make(map[string]RemoteMetadata)
	for path, file := range newFiles {
		if _, exists := oldFiles[path]; !exists {
			createdFiles[path] = file
		}
	}

	// Match deleted and created files for potential moves
	for deletedPath, deletedFile := range deletedFiles {
		for createdPath, createdFile := range createdFiles {
			confidence := md.calculateRemoteMoveConfidence(deletedFile, createdFile)
			if confidence > 0 {
				reason := md.buildRemoteMoveReason(deletedFile, createdFile, confidence)
				candidates = append(candidates, MoveCandidate{
					OldPath:    deletedPath,
					NewPath:    createdPath,
					Confidence: confidence,
					Reason:     reason,
					IsLocal:    false,
					Metadata:   createdFile,
				})
			}
		}
	}

	return candidates
}

// calculateLocalMoveConfidence calculates confidence that two local files represent a move
func (md *MoveDetector) calculateLocalMoveConfidence(deleted, created FileMetadata) float64 {
	var confidence float64

	// Same inode and device = definite move
	if deleted.Inode != 0 && created.Inode != 0 &&
		deleted.Inode == created.Inode && deleted.Device == created.Device {
		return 1.0
	}

	// Same checksum = very likely move (if not zero-length file)
	if deleted.Checksum != "" && created.Checksum != "" &&
		deleted.Checksum == created.Checksum && deleted.Size > 0 {
		confidence += 0.8
	}

	// Same size and modification time = likely move
	if deleted.Size == created.Size && !deleted.ModTime.IsZero() &&
		deleted.ModTime.Equal(created.ModTime) {
		confidence += 0.4
	}

	// Similar file names boost confidence
	nameScore := md.calculateNameSimilarity(deleted.Path, created.Path)
	confidence += nameScore * 0.3

	// Same file type (regular file vs directory)
	if deleted.IsDir == created.IsDir {
		confidence += 0.1
	}

	// Same permissions
	if deleted.Permissions == created.Permissions {
		confidence += 0.1
	}

	// Cap confidence at 1.0
	if confidence > 1.0 {
		confidence = 1.0
	}

	return confidence
}

// calculateRemoteMoveConfidence calculates confidence that two remote files represent a move
func (md *MoveDetector) calculateRemoteMoveConfidence(deleted, created RemoteMetadata) float64 {
	var confidence float64

	// Same descriptor CID = definite move
	if deleted.DescriptorCID != "" && created.DescriptorCID != "" &&
		deleted.DescriptorCID == created.DescriptorCID {
		return 1.0
	}

	// Same content CID = very likely move
	if deleted.ContentCID != "" && created.ContentCID != "" &&
		deleted.ContentCID == created.ContentCID {
		confidence += 0.9
	}

	// Same size and modification time = likely move
	if deleted.Size == created.Size && !deleted.ModTime.IsZero() &&
		deleted.ModTime.Equal(created.ModTime) {
		confidence += 0.5
	}

	// Similar file names boost confidence
	nameScore := md.calculateNameSimilarity(deleted.Path, created.Path)
	confidence += nameScore * 0.3

	// Same file type
	if deleted.IsDir == created.IsDir {
		confidence += 0.1
	}

	// Cap confidence at 1.0
	if confidence > 1.0 {
		confidence = 1.0
	}

	return confidence
}

// calculateNameSimilarity calculates similarity between two file paths
func (md *MoveDetector) calculateNameSimilarity(path1, path2 string) float64 {
	name1 := filepath.Base(path1)
	name2 := filepath.Base(path2)

	// Exact name match
	if name1 == name2 {
		return 1.0
	}

	// Check if it's just a directory move (same filename, different directory)
	if name1 == name2 && filepath.Dir(path1) != filepath.Dir(path2) {
		return 0.9
	}

	// Calculate string similarity using simple algorithm
	return md.calculateStringSimilarity(name1, name2)
}

// calculateStringSimilarity calculates similarity between two strings
func (md *MoveDetector) calculateStringSimilarity(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}

	// Convert to lowercase for comparison
	s1 = strings.ToLower(s1)
	s2 = strings.ToLower(s2)

	// Calculate Levenshtein distance
	distance := md.levenshteinDistance(s1, s2)
	maxLen := len(s1)
	if len(s2) > maxLen {
		maxLen = len(s2)
	}

	if maxLen == 0 {
		return 1.0
	}

	return 1.0 - float64(distance)/float64(maxLen)
}

// levenshteinDistance calculates the Levenshtein distance between two strings
func (md *MoveDetector) levenshteinDistance(s1, s2 string) int {
	if len(s1) == 0 {
		return len(s2)
	}
	if len(s2) == 0 {
		return len(s1)
	}

	// Create a matrix
	matrix := make([][]int, len(s1)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(s2)+1)
	}

	// Initialize first row and column
	for i := 0; i <= len(s1); i++ {
		matrix[i][0] = i
	}
	for j := 0; j <= len(s2); j++ {
		matrix[0][j] = j
	}

	// Fill the matrix
	for i := 1; i <= len(s1); i++ {
		for j := 1; j <= len(s2); j++ {
			cost := 0
			if s1[i-1] != s2[j-1] {
				cost = 1
			}

			matrix[i][j] = min(
				matrix[i-1][j]+1,      // deletion
				matrix[i][j-1]+1,      // insertion
				matrix[i-1][j-1]+cost, // substitution
			)
		}
	}

	return matrix[len(s1)][len(s2)]
}

// min returns the minimum of three integers
func min(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

// buildMoveReason builds a human-readable explanation for a move detection
func (md *MoveDetector) buildMoveReason(deleted, created FileMetadata, confidence float64) string {
	var reasons []string

	if deleted.Inode == created.Inode && deleted.Device == created.Device {
		reasons = append(reasons, "same inode")
	}
	if deleted.Checksum == created.Checksum && deleted.Checksum != "" {
		reasons = append(reasons, "same checksum")
	}
	if deleted.Size == created.Size {
		reasons = append(reasons, "same size")
	}
	if deleted.ModTime.Equal(created.ModTime) {
		reasons = append(reasons, "same modification time")
	}

	nameScore := md.calculateNameSimilarity(deleted.Path, created.Path)
	if nameScore > 0.8 {
		reasons = append(reasons, "similar name")
	}

	return strings.Join(reasons, ", ")
}

// buildRemoteMoveReason builds a human-readable explanation for a remote move detection
func (md *MoveDetector) buildRemoteMoveReason(deleted, created RemoteMetadata, confidence float64) string {
	var reasons []string

	if deleted.DescriptorCID == created.DescriptorCID && deleted.DescriptorCID != "" {
		reasons = append(reasons, "same descriptor CID")
	}
	if deleted.ContentCID == created.ContentCID && deleted.ContentCID != "" {
		reasons = append(reasons, "same content CID")
	}
	if deleted.Size == created.Size {
		reasons = append(reasons, "same size")
	}
	if deleted.ModTime.Equal(created.ModTime) {
		reasons = append(reasons, "same modification time")
	}

	nameScore := md.calculateNameSimilarity(deleted.Path, created.Path)
	if nameScore > 0.8 {
		reasons = append(reasons, "similar name")
	}

	return strings.Join(reasons, ", ")
}
