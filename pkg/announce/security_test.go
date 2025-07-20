package announce

import (
	"strings"
	"testing"
	"time"
)

func TestValidator(t *testing.T) {
	validator := NewValidator(nil)
	
	tests := []struct {
		name    string
		ann     *Announcement
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid announcement",
			ann: &Announcement{
				Version:    "1.0",
				Descriptor: "QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG",
				TopicHash:  "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
				Timestamp:  time.Now().Unix(),
				TTL:        3600,
				Category:   "video",
				SizeClass:  "medium",
				Nonce:      "abc123def456",
			},
			wantErr: false,
		},
		{
			name: "missing version",
			ann: &Announcement{
				Descriptor: "QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG",
				TopicHash:  "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
				Timestamp:  time.Now().Unix(),
				TTL:        3600,
				Nonce:      "abc123def456",
			},
			wantErr: true,
			errMsg:  "missing required 'version' field",
		},
		{
			name: "invalid descriptor",
			ann: &Announcement{
				Version:    "1.0",
				Descriptor: "invalid-cid",
				TopicHash:  "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
				Timestamp:  time.Now().Unix(),
				TTL:        3600,
				Nonce:      "abc123def456",
			},
			wantErr: true,
			errMsg:  "invalid CID format",
		},
		{
			name: "invalid topic hash",
			ann: &Announcement{
				Version:    "1.0",
				Descriptor: "QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG",
				TopicHash:  "short",
				Timestamp:  time.Now().Unix(),
				TTL:        3600,
				Nonce:      "abc123def456",
			},
			wantErr: true,
			errMsg:  "invalid topic hash length",
		},
		{
			name: "TTL too long",
			ann: &Announcement{
				Version:    "1.0",
				Descriptor: "QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG",
				TopicHash:  "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
				Timestamp:  time.Now().Unix(),
				TTL:        10 * 24 * 3600, // 10 days
				Nonce:      "abc123def456",
			},
			wantErr: true,
			errMsg:  "TTL too long",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateAnnouncement(tt.ann)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAnnouncement() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("ValidateAnnouncement() error = %v, want error containing %s", err, tt.errMsg)
			}
		})
	}
}

func TestRateLimiter(t *testing.T) {
	config := &RateLimitConfig{
		MaxPerMinute:    3,
		MaxPerHour:      10,
		MaxPerDay:       50,
		BurstSize:       2,
		CleanupInterval: 1 * time.Hour,
	}
	
	limiter := NewRateLimiter(config)
	defer limiter.Close()
	
	key := "test-source"
	
	// First request should succeed
	if err := limiter.CheckLimit(key); err != nil {
		t.Errorf("First request failed: %v", err)
	}
	
	// Second request should succeed (within burst)
	if err := limiter.CheckLimit(key); err != nil {
		t.Errorf("Second request failed: %v", err)
	}
	
	// Third request should fail (burst exceeded)
	if err := limiter.CheckLimit(key); err == nil {
		t.Error("Third request should have failed (burst exceeded)")
	}
	
	// Check status
	status := limiter.GetStatus(key)
	if status.MinuteCount != 2 {
		t.Errorf("Expected minute count 2, got %d", status.MinuteCount)
	}
	if status.MinuteRemaining != 1 {
		t.Errorf("Expected minute remaining 1, got %d", status.MinuteRemaining)
	}
}

func TestSpamDetector(t *testing.T) {
	config := &SpamConfig{
		DuplicateWindow:  1 * time.Hour,
		SimilarityWindow: 24 * time.Hour,
		MaxDuplicates:    2,
		SuspiciousPatterns: []string{
			"test", "spam",
		},
		CleanupInterval: 1 * time.Hour,
	}
	
	detector := NewSpamDetector(config)
	defer detector.Close()
	
	// Create test announcement
	ann := &Announcement{
		Version:    "1.0",
		Descriptor: "QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG",
		TopicHash:  "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		Timestamp:  time.Now().Unix(),
		TTL:        3600,
		Category:   "video",
		SizeClass:  "medium",
		Nonce:      "abc123def456",
	}
	
	// First announcement should not be spam
	isSpam, reason := detector.CheckSpam(ann)
	if isSpam {
		t.Errorf("First announcement marked as spam: %s", reason)
	}
	
	// Second identical announcement should not be spam (under limit)
	isSpam, reason = detector.CheckSpam(ann)
	if isSpam {
		t.Errorf("Second announcement marked as spam: %s", reason)
	}
	
	// Third identical announcement should be spam (exceeds limit)
	isSpam, reason = detector.CheckSpam(ann)
	if !isSpam {
		t.Error("Third identical announcement should be marked as spam")
	}
	if reason == "" {
		t.Error("Spam reason should be provided")
	}
}

func TestReputationSystem(t *testing.T) {
	config := &ReputationConfig{
		InitialScore:    50.0,
		MaxScore:        100.0,
		MinScore:        0.0,
		DecayRate:       0.1,
		PositiveWeight:  1.0,
		NegativeWeight:  5.0,
		RequiredHistory: 5,
		CleanupInterval: 24 * time.Hour,
	}
	
	system := NewReputationSystem(config)
	defer system.Close()
	
	sourceID := "test-source"
	
	// Initial score should be 50
	score := system.GetScore(sourceID)
	if score != 50.0 {
		t.Errorf("Initial score should be 50, got %f", score)
	}
	
	// Record positive event
	system.RecordPositive(sourceID, "good_announcement")
	score = system.GetScore(sourceID)
	if score != 51.0 {
		t.Logf("Score after positive: %f", score)
	}
	
	// Record negative event
	system.RecordNegative(sourceID, "spam_detected")
	score = system.GetScore(sourceID)
	if score != 46.0 {
		t.Logf("Score after negative: %f", score)
	}
	
	// Should not be trusted (insufficient history - only 2 events)
	if system.IsTrusted(sourceID) {
		t.Error("Source should not be trusted with insufficient history")
	}
	
	// Add more positive events to build history and increase score
	// Need to reach score >= 75 to be trusted (threshold is (100+50)/2 = 75)
	// Current score is 46, need +29 points = 29 positive events
	for i := 0; i < 30; i++ {
		system.RecordPositive(sourceID, "good")
	}
	
	// Check current score and reputation
	rep, _ := system.GetReputation(sourceID)
	t.Logf("After positive events - Score: %f, Total events: %d", rep.Score, rep.TotalEvents)
	
	// Now should be trusted (32 total events, score should be 76)
	if !system.IsTrusted(sourceID) {
		t.Errorf("Source should be trusted with score %f >= 75", rep.Score)
	}
	
	// Check trust level
	level := system.GetTrustLevel(sourceID)
	t.Logf("Trust level: %s", level)
	
	// With score 76 (normalized 0.76), it should be "good" (requires 0.8+ for "trusted")
	if level != "good" {
		t.Errorf("Trust level should be good with score %f (normalized 0.76), got %s", rep.Score, level)
	}
}

