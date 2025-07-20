package compliance

import (
	"testing"
	"time"
)

// TestCalculateDMCAComplianceScore tests DMCA compliance scoring with real logic
func TestCalculateDMCAComplianceScore(t *testing.T) {
	system := NewComplianceAuditSystem(nil)
	
	// Test with no entries
	score := system.calculateDMCAComplianceScore([]*DetailedAuditEntry{})
	if score != 1.0 {
		t.Errorf("Expected perfect score (1.0) with no entries, got %f", score)
	}
	
	// Test with successful DMCA events
	successfulEntries := []*DetailedAuditEntry{
		{EventType: "dmca_takedown", Result: "success"},
		{EventType: "dmca_takedown", Result: "processed"},
		{EventType: "dmca_reinstatement", Result: "reinstated"},
	}
	
	score = system.calculateDMCAComplianceScore(successfulEntries)
	if score != 1.0 {
		t.Errorf("Expected perfect score (1.0) with all successful DMCA events, got %f", score)
	}
	
	// Test with mixed success/failure
	mixedEntries := []*DetailedAuditEntry{
		{EventType: "dmca_takedown", Result: "success"},
		{EventType: "dmca_takedown", Result: "failed"},
		{EventType: "dmca_counter_notice", Result: "processed"},
		{EventType: "dmca_takedown", Result: "rejected"},
	}
	
	score = system.calculateDMCAComplianceScore(mixedEntries)
	// 2 successful out of 4 DMCA events = 0.5 base
	// 2 failures * 0.1 penalty = 0.2 penalty
	// Final score = 0.5 - 0.2 = 0.3
	expectedScore := 0.3
	if score != expectedScore {
		t.Errorf("Expected score %f with mixed DMCA events, got %f", expectedScore, score)
	}
	
	// Test with non-DMCA events (should be ignored)
	nonDMCAEntries := []*DetailedAuditEntry{
		{EventType: "user_login", Result: "success"},
		{EventType: "system_startup", Result: "completed"},
		{EventType: "dmca_takedown", Result: "success"},
	}
	
	score = system.calculateDMCAComplianceScore(nonDMCAEntries)
	if score != 1.0 {
		t.Errorf("Expected perfect score (1.0) with single successful DMCA event, got %f", score)
	}
}

// TestCalculateProcessingComplianceScore tests processing time compliance scoring
func TestCalculateProcessingComplianceScore(t *testing.T) {
	config := DefaultAuditConfig()
	config.AlertThresholds.ProcessingTimeThreshold = 1 * time.Hour // 1 hour threshold
	system := NewComplianceAuditSystem(config)
	
	// Test with no entries
	score := system.calculateProcessingComplianceScore([]*DetailedAuditEntry{})
	if score != 1.0 {
		t.Errorf("Expected perfect score (1.0) with no entries, got %f", score)
	}
	
	// Test with timely processing
	timelyEntries := []*DetailedAuditEntry{
		{EventType: "dmca_takedown", ProcessingTime: 30 * time.Minute},
		{EventType: "dmca_takedown", ProcessingTime: 45 * time.Minute},
		{EventType: "dmca_counter_notice", ProcessingTime: 15 * time.Minute},
	}
	
	score = system.calculateProcessingComplianceScore(timelyEntries)
	if score != 1.0 {
		t.Errorf("Expected perfect score (1.0) with all timely processing, got %f", score)
	}
	
	// Test with delayed processing
	delayedEntries := []*DetailedAuditEntry{
		{EventType: "dmca_takedown", ProcessingTime: 30 * time.Minute}, // Timely
		{EventType: "dmca_takedown", ProcessingTime: 90 * time.Minute}, // Delayed
		{EventType: "dmca_counter_notice", ProcessingTime: 15 * time.Minute}, // Timely
	}
	
	score = system.calculateProcessingComplianceScore(delayedEntries)
	// 2 timely out of 3 events = 0.667 base score
	expectedScore := 2.0 / 3.0
	if score < expectedScore-0.01 || score > expectedScore+0.01 {
		t.Errorf("Expected score around %f with delayed processing, got %f", expectedScore, score)
	}
	
	// Test with severely delayed processing (>2x threshold)
	severelyDelayedEntries := []*DetailedAuditEntry{
		{EventType: "dmca_takedown", ProcessingTime: 30 * time.Minute},  // Timely
		{EventType: "dmca_takedown", ProcessingTime: 3 * time.Hour},     // Severely delayed (>2x 1 hour)
	}
	
	score = system.calculateProcessingComplianceScore(severelyDelayedEntries)
	// 1 timely out of 2 events = 0.5 base
	// 1 severe delay * 0.05 penalty = 0.05 penalty
	// Final score = 0.5 - 0.05 = 0.45
	expectedScore = 0.45
	if score != expectedScore {
		t.Errorf("Expected score %f with severely delayed processing, got %f", expectedScore, score)
	}
	
	// Test with entries that have no processing time (should be ignored)
	noProcessingTimeEntries := []*DetailedAuditEntry{
		{EventType: "dmca_takedown", ProcessingTime: 0},
		{EventType: "user_login", ProcessingTime: 0},
	}
	
	score = system.calculateProcessingComplianceScore(noProcessingTimeEntries)
	if score != 1.0 {
		t.Errorf("Expected perfect score (1.0) with no processing time data, got %f", score)
	}
}

// TestCalculateAuditComplianceScore tests audit completeness scoring
func TestCalculateAuditComplianceScore(t *testing.T) {
	config := DefaultAuditConfig()
	config.RequireCryptographicProof = true
	system := NewComplianceAuditSystem(config)
	
	// Test with no entries
	score := system.calculateAuditComplianceScore([]*DetailedAuditEntry{})
	if score != 1.0 {
		t.Errorf("Expected perfect score (1.0) with no entries, got %f", score)
	}
	
	// Test with complete entries
	completeEntries := []*DetailedAuditEntry{
		{
			EntryID:      "AE-12345",
			Timestamp:    time.Now(),
			EventType:    "dmca_takedown",
			Action:       "descriptor_blacklisted",
			EntryHash:    "hash123",
			Signature:    "sig123",
			LegalContext: &LegalContext{LegalBasis: "DMCA 17 USC 512"},
		},
		{
			EntryID:   "AE-67890",
			Timestamp: time.Now(),
			EventType: "user_login",
			Action:    "login_successful",
			EntryHash: "hash456",
			Signature: "sig456",
		},
	}
	
	// Set up entry hashes correctly
	for _, entry := range completeEntries {
		entry.EntryHash = system.calculateEntryHash(entry)
	}
	
	score = system.calculateAuditComplianceScore(completeEntries)
	if score != 1.0 {
		t.Errorf("Expected perfect score (1.0) with complete entries, got %f", score)
	}
	
	// Test with missing required fields
	incompleteEntries := []*DetailedAuditEntry{
		{
			EntryID:   "", // Missing entry ID
			Timestamp: time.Now(),
			EventType: "dmca_takedown",
			Action:    "descriptor_blacklisted",
			EntryHash: "hash123",
			Signature: "sig123",
		},
		{
			EntryID:   "AE-67890",
			Timestamp: time.Time{}, // Missing timestamp
			EventType: "user_login",
			Action:    "login_successful",
			EntryHash: "hash456",
			Signature: "sig456",
		},
	}
	
	score = system.calculateAuditComplianceScore(incompleteEntries)
	// 0 complete entries out of 2 = 0 base score
	// 2 missing field issues * 0.05 penalty = 0.1 penalty
	// Final score = 0 - 0.1 = 0 (minimum)
	if score != 0.0 {
		t.Errorf("Expected zero score (0.0) with incomplete entries, got %f", score)
	}
	
	// Test with missing cryptographic integrity
	noCryptoEntries := []*DetailedAuditEntry{
		{
			EntryID:   "AE-12345",
			Timestamp: time.Now(),
			EventType: "dmca_takedown",
			Action:    "descriptor_blacklisted",
			EntryHash: "", // Missing hash
			Signature: "", // Missing signature
			LegalContext: &LegalContext{LegalBasis: "DMCA 17 USC 512"},
		},
	}
	
	score = system.calculateAuditComplianceScore(noCryptoEntries)
	// 0 complete entries out of 1 = 0 base score
	// 1 integrity issue * 0.15 penalty = 0.15 penalty
	// Final score = 0 - 0.15 = 0 (minimum)
	if score != 0.0 {
		t.Errorf("Expected zero score (0.0) with missing crypto integrity, got %f", score)
	}
	
	// Test with missing legal context for DMCA events
	noLegalContextEntries := []*DetailedAuditEntry{
		{
			EntryID:      "AE-12345",
			Timestamp:    time.Now(),
			EventType:    "dmca_takedown",
			Action:       "descriptor_blacklisted",
			EntryHash:    "hash123",
			Signature:    "sig123",
			LegalContext: nil, // Missing legal context
		},
	}
	
	score = system.calculateAuditComplianceScore(noLegalContextEntries)
	// 0 complete entries out of 1 = 0 base score
	// 1 missing field issue * 0.05 penalty = 0.05 penalty
	// Final score = 0 - 0.05 = 0 (minimum)
	if score != 0.0 {
		t.Errorf("Expected zero score (0.0) with missing legal context, got %f", score)
	}
}

// TestScoringIntegration tests how the scoring methods work together
func TestScoringIntegration(t *testing.T) {
	system := NewComplianceAuditSystem(nil)
	
	// Create realistic audit entries
	entries := []*DetailedAuditEntry{
		{
			EntryID:        "AE-001",
			Timestamp:      time.Now().Add(-1 * time.Hour),
			EventType:      "dmca_takedown",
			Action:         "descriptor_blacklisted",
			Result:         "success",
			ProcessingTime: 30 * time.Minute,
			EntryHash:      "hash001",
			Signature:      "sig001",
			LegalContext:   &LegalContext{LegalBasis: "DMCA 17 USC 512"},
		},
		{
			EntryID:        "AE-002",
			Timestamp:      time.Now().Add(-30 * time.Minute),
			EventType:      "dmca_counter_notice",
			Action:         "counter_notice_submitted",
			Result:         "processed",
			ProcessingTime: 15 * time.Minute,
			EntryHash:      "hash002",
			Signature:      "sig002",
			LegalContext:   &LegalContext{LegalBasis: "DMCA 17 USC 512"},
		},
	}
	
	// Fix entry hashes
	for _, entry := range entries {
		entry.EntryHash = system.calculateEntryHash(entry)
	}
	
	// Test DMCA compliance (should be perfect)
	dmcaScore := system.calculateDMCAComplianceScore(entries)
	if dmcaScore != 1.0 {
		t.Errorf("Expected perfect DMCA score (1.0), got %f", dmcaScore)
	}
	
	// Test processing compliance (should be perfect)
	processingScore := system.calculateProcessingComplianceScore(entries)
	if processingScore != 1.0 {
		t.Errorf("Expected perfect processing score (1.0), got %f", processingScore)
	}
	
	// Test audit compliance (should be perfect)
	auditScore := system.calculateAuditComplianceScore(entries)
	if auditScore != 1.0 {
		t.Errorf("Expected perfect audit score (1.0), got %f", auditScore)
	}
	
	// Test overall compliance score calculation
	overallScore := system.calculateComplianceScore(entries)
	if overallScore != 1.0 {
		t.Errorf("Expected perfect overall score (1.0), got %f", overallScore)
	}
}