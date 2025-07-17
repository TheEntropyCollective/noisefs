package sync

import (
	"testing"
	"time"
)

func TestConflictResolver_Creation(t *testing.T) {
	// Test creating resolver with valid strategy
	resolver, err := NewConflictResolver(ConflictResolveLocal)
	if err != nil {
		t.Fatalf("Failed to create conflict resolver: %v", err)
	}

	if resolver.defaultStrategy != ConflictResolveLocal {
		t.Errorf("Expected default strategy %s, got %s", ConflictResolveLocal, resolver.defaultStrategy)
	}

	// Test creating resolver with invalid strategy
	_, err = NewConflictResolver(ConflictResolution("invalid"))
	if err == nil {
		t.Error("Expected error for invalid strategy")
	}

	// Test available strategies
	strategies := resolver.GetAvailableStrategies()
	expectedStrategies := []ConflictResolution{
		ConflictResolveLocal,
		ConflictResolveRemote,
		ConflictResolveTimestamp,
		ConflictResolvePrompt,
	}

	if len(strategies) != len(expectedStrategies) {
		t.Errorf("Expected %d strategies, got %d", len(expectedStrategies), len(strategies))
	}

	// Check if all expected strategies are present
	for _, expected := range expectedStrategies {
		found := false
		for _, actual := range strategies {
			if actual == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected strategy %s not found", expected)
		}
	}
}

func TestLocalWinsStrategy(t *testing.T) {
	strategy := &LocalWinsStrategy{}
	
	if strategy.GetStrategyName() != "local_wins" {
		t.Errorf("Expected strategy name 'local_wins', got '%s'", strategy.GetStrategyName())
	}

	now := time.Now()
	conflict := &Conflict{
		ID:         "test-conflict",
		LocalPath:  "/local/file.txt",
		RemotePath: "/remote/file.txt",
		LocalMetadata: FileMetadata{
			Path:    "/local/file.txt",
			Size:    1024,
			ModTime: now,
		},
		RemoteMetadata: RemoteMetadata{
			Path:    "/remote/file.txt",
			Size:    2048,
			ModTime: now.Add(time.Hour),
		},
		ConflictType: ConflictTypeBothModified,
		Timestamp:    now,
	}

	result, err := strategy.ResolveConflict(conflict)
	if err != nil {
		t.Fatalf("Failed to resolve conflict: %v", err)
	}

	if result.Resolution != ConflictResolveLocal {
		t.Errorf("Expected resolution %s, got %s", ConflictResolveLocal, result.Resolution)
	}

	if result.Action != ActionUseLocal {
		t.Errorf("Expected action %s, got %s", ActionUseLocal, result.Action)
	}

	if result.LocalPath != "/local/file.txt" {
		t.Errorf("Expected local path '/local/file.txt', got '%s'", result.LocalPath)
	}

	if result.RemotePath != "/remote/file.txt" {
		t.Errorf("Expected remote path '/remote/file.txt', got '%s'", result.RemotePath)
	}

	if result.RequiresInput {
		t.Error("LocalWinsStrategy should not require input")
	}
}

func TestRemoteWinsStrategy(t *testing.T) {
	strategy := &RemoteWinsStrategy{}
	
	if strategy.GetStrategyName() != "remote_wins" {
		t.Errorf("Expected strategy name 'remote_wins', got '%s'", strategy.GetStrategyName())
	}

	now := time.Now()
	conflict := &Conflict{
		ID:         "test-conflict",
		LocalPath:  "/local/file.txt",
		RemotePath: "/remote/file.txt",
		LocalMetadata: FileMetadata{
			Path:    "/local/file.txt",
			Size:    1024,
			ModTime: now,
		},
		RemoteMetadata: RemoteMetadata{
			Path:    "/remote/file.txt",
			Size:    2048,
			ModTime: now.Add(-time.Hour),
		},
		ConflictType: ConflictTypeBothModified,
		Timestamp:    now,
	}

	result, err := strategy.ResolveConflict(conflict)
	if err != nil {
		t.Fatalf("Failed to resolve conflict: %v", err)
	}

	if result.Resolution != ConflictResolveRemote {
		t.Errorf("Expected resolution %s, got %s", ConflictResolveRemote, result.Resolution)
	}

	if result.Action != ActionUseRemote {
		t.Errorf("Expected action %s, got %s", ActionUseRemote, result.Action)
	}

	if result.RequiresInput {
		t.Error("RemoteWinsStrategy should not require input")
	}
}

func TestTimestampStrategy(t *testing.T) {
	strategy := &TimestampStrategy{}
	
	if strategy.GetStrategyName() != "timestamp" {
		t.Errorf("Expected strategy name 'timestamp', got '%s'", strategy.GetStrategyName())
	}

	now := time.Now()

	// Test case 1: Local file is newer
	conflict1 := &Conflict{
		ID:         "test-conflict-1",
		LocalPath:  "/local/file.txt",
		RemotePath: "/remote/file.txt",
		LocalMetadata: FileMetadata{
			Path:    "/local/file.txt",
			Size:    1024,
			ModTime: now,
		},
		RemoteMetadata: RemoteMetadata{
			Path:    "/remote/file.txt",
			Size:    2048,
			ModTime: now.Add(-time.Hour),
		},
		ConflictType: ConflictTypeBothModified,
		Timestamp:    now,
	}

	result1, err := strategy.ResolveConflict(conflict1)
	if err != nil {
		t.Fatalf("Failed to resolve conflict: %v", err)
	}

	if result1.Action != ActionUseLocal {
		t.Errorf("Expected action %s for newer local file, got %s", ActionUseLocal, result1.Action)
	}

	// Test case 2: Remote file is newer
	conflict2 := &Conflict{
		ID:         "test-conflict-2",
		LocalPath:  "/local/file.txt",
		RemotePath: "/remote/file.txt",
		LocalMetadata: FileMetadata{
			Path:    "/local/file.txt",
			Size:    1024,
			ModTime: now.Add(-time.Hour),
		},
		RemoteMetadata: RemoteMetadata{
			Path:    "/remote/file.txt",
			Size:    2048,
			ModTime: now,
		},
		ConflictType: ConflictTypeBothModified,
		Timestamp:    now,
	}

	result2, err := strategy.ResolveConflict(conflict2)
	if err != nil {
		t.Fatalf("Failed to resolve conflict: %v", err)
	}

	if result2.Action != ActionUseRemote {
		t.Errorf("Expected action %s for newer remote file, got %s", ActionUseRemote, result2.Action)
	}

	// Test case 3: Same timestamp, different sizes
	conflict3 := &Conflict{
		ID:         "test-conflict-3",
		LocalPath:  "/local/file.txt",
		RemotePath: "/remote/file.txt",
		LocalMetadata: FileMetadata{
			Path:    "/local/file.txt",
			Size:    2048,
			ModTime: now,
		},
		RemoteMetadata: RemoteMetadata{
			Path:    "/remote/file.txt",
			Size:    1024,
			ModTime: now,
		},
		ConflictType: ConflictTypeBothModified,
		Timestamp:    now,
	}

	result3, err := strategy.ResolveConflict(conflict3)
	if err != nil {
		t.Fatalf("Failed to resolve conflict: %v", err)
	}

	if result3.Action != ActionUseLocal {
		t.Errorf("Expected action %s for larger local file, got %s", ActionUseLocal, result3.Action)
	}

	// Test case 4: Same timestamp, remote file is larger
	conflict4 := &Conflict{
		ID:         "test-conflict-4",
		LocalPath:  "/local/file.txt",
		RemotePath: "/remote/file.txt",
		LocalMetadata: FileMetadata{
			Path:    "/local/file.txt",
			Size:    1024,
			ModTime: now,
		},
		RemoteMetadata: RemoteMetadata{
			Path:    "/remote/file.txt",
			Size:    2048,
			ModTime: now,
		},
		ConflictType: ConflictTypeBothModified,
		Timestamp:    now,
	}

	result4, err := strategy.ResolveConflict(conflict4)
	if err != nil {
		t.Fatalf("Failed to resolve conflict: %v", err)
	}

	if result4.Action != ActionUseRemote {
		t.Errorf("Expected action %s for larger remote file, got %s", ActionUseRemote, result4.Action)
	}
}

func TestPromptStrategy(t *testing.T) {
	strategy := &PromptStrategy{}
	
	if strategy.GetStrategyName() != "prompt" {
		t.Errorf("Expected strategy name 'prompt', got '%s'", strategy.GetStrategyName())
	}

	now := time.Now()
	conflict := &Conflict{
		ID:         "test-conflict",
		LocalPath:  "/local/file.txt",
		RemotePath: "/remote/file.txt",
		LocalMetadata: FileMetadata{
			Path:     "/local/file.txt",
			Size:     1024,
			ModTime:  now,
			Checksum: "abc123",
		},
		RemoteMetadata: RemoteMetadata{
			Path:          "/remote/file.txt",
			DescriptorCID: "QmTest123",
			Size:          2048,
			ModTime:       now.Add(time.Hour),
		},
		ConflictType: ConflictTypeBothModified,
		Timestamp:    now,
	}

	result, err := strategy.ResolveConflict(conflict)
	if err != nil {
		t.Fatalf("Failed to resolve conflict: %v", err)
	}

	if result.Resolution != ConflictResolvePrompt {
		t.Errorf("Expected resolution %s, got %s", ConflictResolvePrompt, result.Resolution)
	}

	if result.Action != ActionPromptUser {
		t.Errorf("Expected action %s, got %s", ActionPromptUser, result.Action)
	}

	if !result.RequiresInput {
		t.Error("PromptStrategy should require input")
	}

	if result.UserPrompt == "" {
		t.Error("PromptStrategy should provide user prompt")
	}

	// Check that prompt contains expected information
	prompt := result.UserPrompt
	if len(prompt) == 0 {
		t.Error("Expected non-empty user prompt")
	}

	// Should contain file path
	if !contains(prompt, "/local/file.txt") {
		t.Error("Prompt should contain file path")
	}

	// Should contain conflict type
	if !contains(prompt, string(ConflictTypeBothModified)) {
		t.Error("Prompt should contain conflict type")
	}

	// Should contain conflict ID
	if !contains(prompt, "test-conflict") {
		t.Error("Prompt should contain conflict ID")
	}

	// Should contain size information
	if !contains(prompt, "1024") || !contains(prompt, "2048") {
		t.Error("Prompt should contain size information")
	}

	// Should contain choices
	if !contains(prompt, "1. Use local version") {
		t.Error("Prompt should contain choice for local version")
	}

	if !contains(prompt, "2. Use remote version") {
		t.Error("Prompt should contain choice for remote version")
	}
}

func TestRenameStrategy(t *testing.T) {
	strategy := &RenameStrategy{}
	
	if strategy.GetStrategyName() != "rename" {
		t.Errorf("Expected strategy name 'rename', got '%s'", strategy.GetStrategyName())
	}

	now := time.Now()
	conflict := &Conflict{
		ID:         "test-conflict",
		LocalPath:  "/local/file.txt",
		RemotePath: "/remote/file.txt",
		LocalMetadata: FileMetadata{
			Path:    "/local/file.txt",
			Size:    1024,
			ModTime: now,
		},
		RemoteMetadata: RemoteMetadata{
			Path:    "/remote/file.txt",
			Size:    2048,
			ModTime: now.Add(time.Hour),
		},
		ConflictType: ConflictTypeBothModified,
		Timestamp:    now,
	}

	result, err := strategy.ResolveConflict(conflict)
	if err != nil {
		t.Fatalf("Failed to resolve conflict: %v", err)
	}

	if result.Action != ActionRename {
		t.Errorf("Expected action %s, got %s", ActionRename, result.Action)
	}

	if result.RenamedPath == "" {
		t.Error("RenameStrategy should provide renamed path")
	}

	if result.RequiresInput {
		t.Error("RenameStrategy should not require input")
	}

	// Should contain timestamp in renamed path
	if !contains(result.RenamedPath, ".local.") {
		t.Error("Renamed path should contain .local. identifier")
	}
}

func TestConflictDetector(t *testing.T) {
	detector := &ConflictDetector{}
	now := time.Now()

	// Test case 1: No conflict (same metadata)
	localMeta1 := FileMetadata{
		Path:    "/local/file.txt",
		Size:    1024,
		ModTime: now,
		IsDir:   false,
	}

	remoteMeta1 := RemoteMetadata{
		Path:    "/remote/file.txt",
		Size:    1024,
		ModTime: now,
		IsDir:   false,
	}

	conflict1 := detector.DetectConflict("/local/file.txt", "/remote/file.txt", localMeta1, remoteMeta1)
	if conflict1 != nil {
		t.Error("Expected no conflict for identical metadata")
	}

	// Test case 2: Size conflict
	localMeta2 := FileMetadata{
		Path:    "/local/file.txt",
		Size:    1024,
		ModTime: now,
		IsDir:   false,
	}

	remoteMeta2 := RemoteMetadata{
		Path:    "/remote/file.txt",
		Size:    2048,
		ModTime: now,
		IsDir:   false,
	}

	conflict2 := detector.DetectConflict("/local/file.txt", "/remote/file.txt", localMeta2, remoteMeta2)
	if conflict2 == nil {
		t.Error("Expected conflict for different sizes")
	}

	if conflict2.ConflictType != ConflictTypeBothModified {
		t.Errorf("Expected conflict type %s, got %s", ConflictTypeBothModified, conflict2.ConflictType)
	}

	// Test case 3: Modification time conflict
	localMeta3 := FileMetadata{
		Path:    "/local/file.txt",
		Size:    1024,
		ModTime: now,
		IsDir:   false,
	}

	remoteMeta3 := RemoteMetadata{
		Path:    "/remote/file.txt",
		Size:    1024,
		ModTime: now.Add(time.Hour),
		IsDir:   false,
	}

	conflict3 := detector.DetectConflict("/local/file.txt", "/remote/file.txt", localMeta3, remoteMeta3)
	if conflict3 == nil {
		t.Error("Expected conflict for different modification times")
	}

	// Test case 4: Type conflict (file vs directory)
	localMeta4 := FileMetadata{
		Path:    "/local/item",
		Size:    1024,
		ModTime: now,
		IsDir:   false,
	}

	remoteMeta4 := RemoteMetadata{
		Path:    "/remote/item",
		Size:    0,
		ModTime: now,
		IsDir:   true,
	}

	conflict4 := detector.DetectConflict("/local/item", "/remote/item", localMeta4, remoteMeta4)
	if conflict4 == nil {
		t.Error("Expected conflict for different types")
	}

	if conflict4.ConflictType != ConflictTypeTypeChanged {
		t.Errorf("Expected conflict type %s, got %s", ConflictTypeTypeChanged, conflict4.ConflictType)
	}

	// Test case 5: Deleted local (size 0)
	localMeta5 := FileMetadata{
		Path:    "/local/file.txt",
		Size:    0,
		ModTime: now,
		IsDir:   false,
	}

	remoteMeta5 := RemoteMetadata{
		Path:    "/remote/file.txt",
		Size:    1024,
		ModTime: now,
		IsDir:   false,
	}

	conflict5 := detector.DetectConflict("/local/file.txt", "/remote/file.txt", localMeta5, remoteMeta5)
	if conflict5 == nil {
		t.Error("Expected conflict for deleted local file")
	}

	if conflict5.ConflictType != ConflictTypeDeletedLocal {
		t.Errorf("Expected conflict type %s, got %s", ConflictTypeDeletedLocal, conflict5.ConflictType)
	}
}

func TestConflictHistory(t *testing.T) {
	history := NewConflictHistory()

	now := time.Now()
	conflict := &Conflict{
		ID:           "test-conflict",
		LocalPath:    "/local/file.txt",
		RemotePath:   "/remote/file.txt",
		ConflictType: ConflictTypeBothModified,
		Timestamp:    now,
	}

	result := &ConflictResult{
		Resolution: ConflictResolveLocal,
		Action:     ActionUseLocal,
		Message:    "Using local version",
	}

	// Add resolved conflict
	history.AddResolvedConflict(conflict, result, "user")

	// Get recent conflicts
	recent := history.GetRecentConflicts(10)
	if len(recent) != 1 {
		t.Errorf("Expected 1 recent conflict, got %d", len(recent))
	}

	if recent[0].Conflict.ID != "test-conflict" {
		t.Errorf("Expected conflict ID 'test-conflict', got '%s'", recent[0].Conflict.ID)
	}

	if recent[0].ResolvedBy != "user" {
		t.Errorf("Expected resolved by 'user', got '%s'", recent[0].ResolvedBy)
	}

	// Get conflict stats
	stats := history.GetConflictStats()
	if stats.TotalConflicts != 1 {
		t.Errorf("Expected 1 total conflict, got %d", stats.TotalConflicts)
	}

	if stats.ByType[ConflictTypeBothModified] != 1 {
		t.Errorf("Expected 1 both_modified conflict, got %d", stats.ByType[ConflictTypeBothModified])
	}

	if stats.ByResolution[ConflictResolveLocal] != 1 {
		t.Errorf("Expected 1 local resolution, got %d", stats.ByResolution[ConflictResolveLocal])
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[len(s)-len(substr):] == substr || 
		   len(s) > len(substr) && (s[:len(substr)] == substr || 
		   findInString(s, substr))
}

func findInString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}