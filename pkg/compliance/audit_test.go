package compliance

import (
	"testing"
	"time"
)

// TestNewComplianceAuditSystem tests the creation of a new compliance audit system
func TestNewComplianceAuditSystem(t *testing.T) {
	// Test with nil config
	system := NewComplianceAuditSystem(nil)
	if system == nil {
		t.Fatal("NewComplianceAuditSystem() returned nil with nil config")
	}
	
	// Verify default config was applied
	if system.config == nil {
		t.Error("Expected default config to be applied when nil config provided")
	}
	
	// Test with custom config
	config := &AuditConfig{
		EnableRealTimeLogging:     false,
		LogRetentionPeriod:        30 * 24 * time.Hour,
		RequireCryptographicProof: false,
		AlertThresholds: &AlertThresholds{
			TakedownsPerDay:          50,
			TakedownsPerRequestor:    10,
			CounterNoticeRatio:       0.2,
			RepeatInfringerThreshold: 2,
			ProcessingTimeThreshold:  12 * time.Hour,
		},
		ExportFormats:      []string{"json"},
		AutoBackupInterval: 12 * time.Hour,
		LegalHoldEnabled:   false,
	}
	
	system = NewComplianceAuditSystem(config)
	if system == nil {
		t.Fatal("NewComplianceAuditSystem() returned nil with custom config")
	}
	
	if system.config != config {
		t.Error("Expected custom config to be used")
	}
	
	// Verify all components are initialized
	if system.logger == nil {
		t.Error("Expected audit logger to be initialized")
	}
	
	if system.reporter == nil {
		t.Error("Expected compliance reporter to be initialized")
	}
	
	if system.monitor == nil {
		t.Error("Expected compliance monitor to be initialized")
	}
}

// TestDefaultAuditConfig tests the default audit configuration
func TestDefaultAuditConfig(t *testing.T) {
	config := DefaultAuditConfig()
	
	if config == nil {
		t.Fatal("DefaultAuditConfig() returned nil")
	}
	
	// Verify default values
	if !config.EnableRealTimeLogging {
		t.Error("Expected real-time logging to be enabled by default")
	}
	
	expectedRetention := 7 * 365 * 24 * time.Hour // 7 years
	if config.LogRetentionPeriod != expectedRetention {
		t.Errorf("Expected log retention period of %v, got %v", expectedRetention, config.LogRetentionPeriod)
	}
	
	if !config.RequireCryptographicProof {
		t.Error("Expected cryptographic proof to be required by default")
	}
	
	if config.AlertThresholds == nil {
		t.Fatal("Expected alert thresholds to be configured")
	}
	
	if config.AlertThresholds.TakedownsPerDay != 100 {
		t.Errorf("Expected takedowns per day threshold of 100, got %d", config.AlertThresholds.TakedownsPerDay)
	}
	
	if !config.LegalHoldEnabled {
		t.Error("Expected legal hold to be enabled by default")
	}
}

// TestLogComplianceEvent tests basic compliance event logging
func TestLogComplianceEvent(t *testing.T) {
	system := NewComplianceAuditSystem(nil)
	
	// Test successful event logging
	err := system.LogComplianceEvent("test_event", "user123", "target456", "test_action", map[string]interface{}{
		"test_key": "test_value",
		"count":    42,
	})
	
	if err != nil {
		t.Errorf("LogComplianceEvent() returned unexpected error: %v", err)
	}
	
	// Verify event was logged
	if len(system.logger.entries) != 1 {
		t.Errorf("Expected 1 audit entry, got %d", len(system.logger.entries))
	}
	
	entry := system.logger.entries[0]
	if entry.EventType != "test_event" {
		t.Errorf("Expected event type 'test_event', got '%s'", entry.EventType)
	}
	
	if entry.UserID != "user123" {
		t.Errorf("Expected user ID 'user123', got '%s'", entry.UserID)
	}
	
	if entry.TargetID != "target456" {
		t.Errorf("Expected target ID 'target456', got '%s'", entry.TargetID)
	}
	
	if entry.Action != "test_action" {
		t.Errorf("Expected action 'test_action', got '%s'", entry.Action)
	}
	
	// Verify action details
	if len(entry.ActionDetails) != 2 {
		t.Errorf("Expected 2 action details, got %d", len(entry.ActionDetails))
	}
	
	if entry.ActionDetails["test_key"] != "test_value" {
		t.Errorf("Expected test_key value 'test_value', got '%v'", entry.ActionDetails["test_key"])
	}
}

// TestLogDMCATakedown tests DMCA takedown event logging
func TestLogDMCATakedown(t *testing.T) {
	system := NewComplianceAuditSystem(nil)
	
	err := system.LogDMCATakedown("DMCA-12345", "QmDescriptor123456", "requestor@example.com", "Copyright Work Title")
	
	if err != nil {
		t.Errorf("LogDMCATakedown() returned unexpected error: %v", err)
	}
	
	// Verify event was logged
	if len(system.logger.entries) != 1 {
		t.Errorf("Expected 1 audit entry, got %d", len(system.logger.entries))
	}
	
	entry := system.logger.entries[0]
	if entry.EventType != "dmca_takedown" {
		t.Errorf("Expected event type 'dmca_takedown', got '%s'", entry.EventType)
	}
	
	if entry.TargetID != "QmDescriptor123456" {
		t.Errorf("Expected target ID 'QmDescriptor123456', got '%s'", entry.TargetID)
	}
	
	if entry.Action != "descriptor_blacklisted" {
		t.Errorf("Expected action 'descriptor_blacklisted', got '%s'", entry.Action)
	}
	
	// Verify DMCA-specific details
	if entry.ActionDetails["takedown_id"] != "DMCA-12345" {
		t.Errorf("Expected takedown_id 'DMCA-12345', got '%v'", entry.ActionDetails["takedown_id"])
	}
	
	if entry.ActionDetails["requestor_email"] != "requestor@example.com" {
		t.Errorf("Expected requestor_email 'requestor@example.com', got '%v'", entry.ActionDetails["requestor_email"])
	}
}

// TestLogCounterNotice tests counter-notice event logging
func TestLogCounterNotice(t *testing.T) {
	system := NewComplianceAuditSystem(nil)
	
	reinstatementDate := time.Now().Add(14 * 24 * time.Hour)
	err := system.LogCounterNotice("CN-12345", "QmDescriptor123456", "user456", reinstatementDate)
	
	if err != nil {
		t.Errorf("LogCounterNotice() returned unexpected error: %v", err)
	}
	
	// Verify event was logged
	if len(system.logger.entries) != 1 {
		t.Errorf("Expected 1 audit entry, got %d", len(system.logger.entries))
	}
	
	entry := system.logger.entries[0]
	if entry.EventType != "dmca_counter_notice" {
		t.Errorf("Expected event type 'dmca_counter_notice', got '%s'", entry.EventType)
	}
	
	if entry.UserID != "user456" {
		t.Errorf("Expected user ID 'user456', got '%s'", entry.UserID)
	}
	
	if entry.Action != "counter_notice_submitted" {
		t.Errorf("Expected action 'counter_notice_submitted', got '%s'", entry.Action)
	}
}

// TestLogReinstatement tests descriptor reinstatement event logging
func TestLogReinstatement(t *testing.T) {
	system := NewComplianceAuditSystem(nil)
	
	err := system.LogReinstatement("QmDescriptor123456", "user456", "Counter-notice waiting period elapsed")
	
	if err != nil {
		t.Errorf("LogReinstatement() returned unexpected error: %v", err)
	}
	
	// Verify event was logged
	if len(system.logger.entries) != 1 {
		t.Errorf("Expected 1 audit entry, got %d", len(system.logger.entries))
	}
	
	entry := system.logger.entries[0]
	if entry.EventType != "dmca_reinstatement" {
		t.Errorf("Expected event type 'dmca_reinstatement', got '%s'", entry.EventType)
	}
	
	if entry.UserID != "user456" {
		t.Errorf("Expected user ID 'user456', got '%s'", entry.UserID)
	}
	
	if entry.Action != "descriptor_reinstated" {
		t.Errorf("Expected action 'descriptor_reinstated', got '%s'", entry.Action)
	}
}

// TestGenerateComplianceReport tests compliance report generation
func TestGenerateComplianceReport(t *testing.T) {
	system := NewComplianceAuditSystem(nil)
	
	// Add some test events
	_ = system.LogDMCATakedown("DMCA-1", "QmDescriptor123456", "req1@example.com", "Work1")
	_ = system.LogDMCATakedown("DMCA-2", "QmDescriptor789012", "req2@example.com", "Work2")
	_ = system.LogCounterNotice("CN-1", "QmDescriptor123456", "user1", time.Now().Add(14*24*time.Hour))
	
	startDate := time.Now().Add(-24 * time.Hour)
	endDate := time.Now().Add(1 * time.Hour)
	
	report, err := system.GenerateComplianceReport(startDate, endDate, "weekly")
	
	if err != nil {
		t.Errorf("GenerateComplianceReport() returned unexpected error: %v", err)
	}
	
	if report == nil {
		t.Fatal("GenerateComplianceReport() returned nil report")
	}
	
	// Verify report structure
	if report.ReportType != "weekly" {
		t.Errorf("Expected report type 'weekly', got '%s'", report.ReportType)
	}
	
	if report.Statistics == nil {
		t.Error("Expected statistics to be generated")
	}
	
	if report.DMCAAnalysis == nil {
		t.Error("Expected DMCA analysis to be generated")
	}
	
	if report.ComplianceAssessment == nil {
		t.Error("Expected compliance assessment to be generated")
	}
	
	if report.IntegrityVerification == nil {
		t.Error("Expected integrity verification to be generated")
	}
	
	// Verify statistics
	if report.Statistics.TotalEvents != 3 {
		t.Errorf("Expected 3 total events, got %d", report.Statistics.TotalEvents)
	}
}

// TestCryptographicIntegrity tests audit trail cryptographic integrity
func TestCryptographicIntegrity(t *testing.T) {
	// Test with cryptographic proof enabled
	config := DefaultAuditConfig()
	config.RequireCryptographicProof = true
	system := NewComplianceAuditSystem(config)
	
	// Add test events
	_ = system.LogComplianceEvent("test1", "user1", "target1", "action1", nil)
	_ = system.LogComplianceEvent("test2", "user2", "target2", "action2", nil)
	
	// Verify hash chaining
	if len(system.logger.entries) != 2 {
		t.Fatalf("Expected 2 entries, got %d", len(system.logger.entries))
	}
	
	entry1 := system.logger.entries[0]
	entry2 := system.logger.entries[1]
	
	// First entry should have empty previous hash
	if entry1.PreviousHash != "" {
		t.Errorf("Expected first entry to have empty previous hash, got '%s'", entry1.PreviousHash)
	}
	
	// Second entry should reference first entry's hash
	if entry2.PreviousHash != entry1.EntryHash {
		t.Errorf("Expected second entry previous hash to match first entry hash")
	}
	
	// Verify integrity verification
	verification := system.verifyAuditTrailIntegrity()
	if !verification.IntegrityValid {
		t.Error("Expected audit trail integrity to be valid")
	}
	
	if verification.TotalEntries != 2 {
		t.Errorf("Expected 2 total entries in verification, got %d", verification.TotalEntries)
	}
	
	if verification.VerifiedEntries != 2 {
		t.Errorf("Expected 2 verified entries, got %d", verification.VerifiedEntries)
	}
}

// TestComplianceScoring tests compliance score calculations
func TestComplianceScoring(t *testing.T) {
	system := NewComplianceAuditSystem(nil)
	
	// Add mix of events with different severities
	_ = system.LogComplianceEvent("info_event", "user1", "target1", "action1", nil)
	_ = system.LogComplianceEvent("warning_event", "user2", "target2", "action2", nil)
	_ = system.LogComplianceEvent("critical_event", "user3", "target3", "action3", nil)
	
	// Manually set severities for testing
	system.logger.entries[0].Severity = "info"
	system.logger.entries[1].Severity = "warning" 
	system.logger.entries[2].Severity = "critical"
	
	score := system.calculateComplianceScore(system.logger.entries)
	
	// Score should be less than 1.0 due to warning and critical events
	if score >= 1.0 {
		t.Errorf("Expected compliance score to be reduced due to warning/critical events, got %f", score)
	}
	
	// Score should not be negative
	if score < 0 {
		t.Errorf("Expected compliance score to be non-negative, got %f", score)
	}
}

// TestAlertGeneration tests compliance alert generation
func TestAlertGeneration(t *testing.T) {
	config := DefaultAuditConfig()
	config.AlertThresholds.TakedownsPerDay = 2 // Low threshold for testing
	system := NewComplianceAuditSystem(config)
	
	// Add events that should trigger alert
	today := time.Now()
	entry1 := &DetailedAuditEntry{
		EventType: "dmca_takedown",
		Timestamp: today,
	}
	entry2 := &DetailedAuditEntry{
		EventType: "dmca_takedown", 
		Timestamp: today,
	}
	entry3 := &DetailedAuditEntry{
		EventType: "dmca_takedown",
		Timestamp: today,
	}
	
	// Update metrics for each entry
	system.monitor.updateMetrics(entry1)
	system.monitor.updateMetrics(entry2)
	system.monitor.updateMetrics(entry3)
	
	// Check alert generation
	err := system.monitor.checkAlerts(entry3)
	if err != nil {
		t.Errorf("checkAlerts() returned unexpected error: %v", err)
	}
	
	// Verify alert was generated
	if len(system.monitor.alerts) == 0 {
		t.Error("Expected alert to be generated for high takedown volume")
	}
	
	alert := system.monitor.alerts[0]
	if alert.AlertType != "high_takedown_volume" {
		t.Errorf("Expected alert type 'high_takedown_volume', got '%s'", alert.AlertType)
	}
}

// TestEventCategorization tests event categorization logic
func TestEventCategorization(t *testing.T) {
	system := NewComplianceAuditSystem(nil)
	
	testCases := []struct {
		eventType    string
		expectedCategory string
	}{
		{"dmca_takedown", "dmca"},
		{"dmca_counter_notice", "dmca"},
		{"user_registration", "user"},
		{"system_startup", "system"},
		{"unknown_event", "legal"},
	}
	
	for _, tc := range testCases {
		category := system.categorizeEvent(tc.eventType)
		if category != tc.expectedCategory {
			t.Errorf("categorizeEvent('%s') = '%s', expected '%s'", tc.eventType, category, tc.expectedCategory)
		}
	}
}

// TestSeverityDetermination tests event severity determination
func TestSeverityDetermination(t *testing.T) {
	system := NewComplianceAuditSystem(nil)
	
	testCases := []struct {
		eventType        string
		expectedSeverity string
	}{
		{"dmca_takedown", "legal"},
		{"dmca_reinstatement", "legal"},
		{"system_error", "critical"},
		{"integrity_breach", "critical"},
		{"processing_delay", "warning"},
		{"info_event", "info"},
	}
	
	for _, tc := range testCases {
		severity := system.determineSeverity(tc.eventType, nil)
		if severity != tc.expectedSeverity {
			t.Errorf("determineSeverity('%s') = '%s', expected '%s'", tc.eventType, severity, tc.expectedSeverity)
		}
	}
}

// TestTargetTypeDetection tests target type detection logic
func TestTargetTypeDetection(t *testing.T) {
	system := NewComplianceAuditSystem(nil)
	
	testCases := []struct {
		targetID         string
		expectedType     string
	}{
		{"DMCA-12345", "notice"},
		{"user-789", "user"},
		{"QmDescriptor123456", "descriptor"},
	}
	
	for _, tc := range testCases {
		targetType := system.determineTargetType(tc.targetID)
		if targetType != tc.expectedType {
			t.Errorf("determineTargetType('%s') = '%s', expected '%s'", tc.targetID, targetType, tc.expectedType)
		}
	}
}

// TestIDGeneration tests unique ID generation
func TestIDGeneration(t *testing.T) {
	system := NewComplianceAuditSystem(nil)
	
	// Test entry ID generation with slight delay to ensure uniqueness
	id1 := system.generateEntryID()
	time.Sleep(1 * time.Nanosecond) // Ensure different timestamp
	id2 := system.generateEntryID()
	
	if id1 == id2 {
		t.Error("generateEntryID() should generate unique IDs")
	}
	
	if !containsPrefix(id1, "AE-") {
		t.Errorf("Expected entry ID to start with 'AE-', got '%s'", id1)
	}
	
	// Test report ID generation
	reportID1 := system.generateReportID()
	time.Sleep(1 * time.Nanosecond) // Ensure different timestamp
	reportID2 := system.generateReportID()
	
	if reportID1 == reportID2 {
		t.Error("generateReportID() should generate unique IDs")
	}
	
	if !containsPrefix(reportID1, "CR-") {
		t.Errorf("Expected report ID to start with 'CR-', got '%s'", reportID1)
	}
}

// Helper function to check string prefix
func containsPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// TestConcurrentAccess tests thread safety of audit system
func TestConcurrentAccess(t *testing.T) {
	system := NewComplianceAuditSystem(nil)
	
	// Use a channel to coordinate goroutines
	done := make(chan bool, 10)
	
	// Start multiple goroutines that log events concurrently
	for i := 0; i < 10; i++ {
		go func(id int) {
			err := system.LogComplianceEvent("concurrent_test", "user", "target", "action", map[string]interface{}{
				"goroutine_id": id,
			})
			if err != nil {
				t.Errorf("Concurrent LogComplianceEvent() failed: %v", err)
			}
			done <- true
		}(i)
	}
	
	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
	
	// Verify all events were logged
	if len(system.logger.entries) != 10 {
		t.Errorf("Expected 10 concurrent entries, got %d", len(system.logger.entries))
	}
}