package compliance

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewComplianceDatabase tests database creation
func TestNewComplianceDatabase(t *testing.T) {
	db := NewComplianceDatabase()
	assert.NotNil(t, db, "Should create database instance")
	assert.NotNil(t, db.BlacklistedDescriptors, "Should initialize blacklisted descriptors map")
	assert.NotNil(t, db.TakedownHistory, "Should initialize takedown history")
	assert.NotNil(t, db.UserViolations, "Should initialize user violations map")
	assert.NotNil(t, db.ComplianceMetrics, "Should initialize compliance metrics")
}

// TestAddAndGetTakedownRecord tests basic takedown record operations
func TestAddAndGetTakedownRecord(t *testing.T) {
	db := NewComplianceDatabase()
	
	// Create test record
	record := &TakedownRecord{
		DescriptorCID:  "QmTestCID123",
		TakedownID:     "TD123",
		FilePath:       "/test/file.mp4",
		RequestorName:  "Test Requestor",
		RequestorEmail: "test@example.com",
		CopyrightWork:  "Test Work",
		TakedownDate:   time.Now(),
		Status:         "active",
		LegalBasis:     "DMCA",
	}
	
	// Add record
	err := db.AddTakedownRecord(record)
	require.NoError(t, err, "Should add takedown record without error")
	
	// Verify descriptor is blacklisted
	assert.True(t, db.IsDescriptorBlacklisted("QmTestCID123"), "Descriptor should be blacklisted")
	
	// Get record back
	retrieved, exists := db.GetTakedownRecord("QmTestCID123")
	require.True(t, exists, "Should find the takedown record")
	assert.Equal(t, record.DescriptorCID, retrieved.DescriptorCID, "Should return correct record")
	assert.Equal(t, record.RequestorName, retrieved.RequestorName, "Should preserve all fields")
}

// TestIsDescriptorBlacklisted tests blacklist checking
func TestIsDescriptorBlacklisted(t *testing.T) {
	db := NewComplianceDatabase()
	
	// Should not be blacklisted initially
	assert.False(t, db.IsDescriptorBlacklisted("QmNotBlacklisted"), "Should not be blacklisted initially")
	
	// Add a takedown record
	record := &TakedownRecord{
		DescriptorCID: "QmBlacklisted123",
		TakedownID:    "TD123",
		Status:        "active",
		TakedownDate:  time.Now(),
	}
	
	err := db.AddTakedownRecord(record)
	require.NoError(t, err)
	
	// Should now be blacklisted
	assert.True(t, db.IsDescriptorBlacklisted("QmBlacklisted123"), "Should be blacklisted after takedown")
}

// TestCounterNoticeProcessing tests counter notice handling
func TestCounterNoticeProcessing(t *testing.T) {
	db := NewComplianceDatabase()
	
	// Add initial takedown
	record := &TakedownRecord{
		DescriptorCID: "QmCounterTest123",
		TakedownID:    "TD123",
		Status:        "active",
		TakedownDate:  time.Now(),
	}
	
	err := db.AddTakedownRecord(record)
	require.NoError(t, err)
	
	// Process counter notice
	counterNotice := &CounterNotice{
		CounterNoticeID:       "CN123",
		UserName:              "Test Responder",
		UserEmail:             "responder@example.com",
		UserAddress:           "123 Test St",
		SwornStatement:        "This is a valid counter notice",
		GoodFaithBelief:       "I have a good faith belief that the material was disabled due to mistake or misidentification",
		Signature:             "Test Signature",
		SubmissionDate:        time.Now(),
		ConsentToJurisdiction: true,
	}
	
	err = db.ProcessCounterNotice("QmCounterTest123", counterNotice)
	require.NoError(t, err, "Should process counter notice without error")
	
	// Verify counter notice was added to record
	retrieved, exists := db.GetTakedownRecord("QmCounterTest123")
	require.True(t, exists)
	assert.NotNil(t, retrieved.CounterNotice, "Should have counter notice attached")
	assert.Equal(t, counterNotice.UserName, retrieved.CounterNotice.UserName)
}

// TestReinstateDescriptor tests descriptor reinstatement
func TestReinstateDescriptor(t *testing.T) {
	db := NewComplianceDatabase()
	
	// Add initial takedown
	record := &TakedownRecord{
		DescriptorCID: "QmReinstateTest123",
		TakedownID:    "TD123",
		Status:        "active",
		TakedownDate:  time.Now(),
	}
	
	err := db.AddTakedownRecord(record)
	require.NoError(t, err)
	
	// Should be blacklisted initially
	assert.True(t, db.IsDescriptorBlacklisted("QmReinstateTest123"))
	
	// Reinstate
	err = db.ReinstateDescriptor("QmReinstateTest123", "Counter notice period expired")
	require.NoError(t, err, "Should reinstate without error")
	
	// Should no longer be blacklisted
	assert.False(t, db.IsDescriptorBlacklisted("QmReinstateTest123"), "Should not be blacklisted after reinstatement")
	
	// Verify status updated
	retrieved, exists := db.GetTakedownRecord("QmReinstateTest123")
	require.True(t, exists)
	assert.Equal(t, "reinstated", retrieved.Status)
	assert.NotNil(t, retrieved.ReinstatementDate)
}

// TestUserViolations tests user violation tracking
func TestUserViolations(t *testing.T) {
	db := NewComplianceDatabase()
	
	userID := "user123"
	
	// Should have no violations initially
	violations := db.GetUserViolations(userID)
	assert.Empty(t, violations, "Should have no violations initially")
	assert.False(t, db.IsRepeatInfringer(userID), "Should not be repeat infringer initially")
	
	// Add takedown records for the same user
	for i := 0; i < 3; i++ {
		record := &TakedownRecord{
			DescriptorCID: fmt.Sprintf("QmUserTest%d", i),
			TakedownID:    fmt.Sprintf("TD%d", i),
			UploaderID:    userID,
			Status:        "active",
			TakedownDate:  time.Now(),
		}
		
		err := db.AddTakedownRecord(record)
		require.NoError(t, err)
	}
	
	// Should now have violations
	violations = db.GetUserViolations(userID)
	assert.Len(t, violations, 3, "Should have 3 violations")
	
	// Should be considered repeat infringer (3+ violations)
	assert.True(t, db.IsRepeatInfringer(userID), "Should be repeat infringer with 3+ violations")
}

// TestComplianceMetrics tests metrics tracking
func TestComplianceMetrics(t *testing.T) {
	db := NewComplianceDatabase()
	
	// Initial metrics should be zero
	metrics := db.GetComplianceMetrics()
	assert.Equal(t, 0, metrics.TotalTakedowns, "Should start with zero takedowns")
	assert.Equal(t, 0, metrics.ActiveTakedowns, "Should start with zero active takedowns")
	
	// Add a takedown
	record := &TakedownRecord{
		DescriptorCID: "QmMetricsTest123",
		TakedownID:    "TD123",
		Status:        "active",
		TakedownDate:  time.Now(),
	}
	
	err := db.AddTakedownRecord(record)
	require.NoError(t, err)
	
	// Metrics should be updated
	metrics = db.GetComplianceMetrics()
	assert.Equal(t, 1, metrics.TotalTakedowns, "Should have 1 total takedown")
	assert.Equal(t, 1, metrics.ActiveTakedowns, "Should have 1 active takedown")
	
	// Reinstate the descriptor
	err = db.ReinstateDescriptor("QmMetricsTest123", "Test reinstatement")
	require.NoError(t, err)
	
	// Active count should decrease
	metrics = db.GetComplianceMetrics()
	assert.Equal(t, 1, metrics.TotalTakedowns, "Should still have 1 total takedown")
	assert.Equal(t, 0, metrics.ActiveTakedowns, "Should have 0 active takedowns after reinstatement")
}

// TestTakedownHistory tests audit trail functionality
func TestTakedownHistory(t *testing.T) {
	db := NewComplianceDatabase()
	
	// Should have no history initially
	history := db.GetTakedownHistory(10, 0)
	assert.Empty(t, history, "Should have no history initially")
	
	// Add a takedown (this should create history events)
	record := &TakedownRecord{
		DescriptorCID: "QmHistoryTest123",
		TakedownID:    "TD123",
		Status:        "active",
		TakedownDate:  time.Now(),
	}
	
	err := db.AddTakedownRecord(record)
	require.NoError(t, err)
	
	// Should now have history
	history = db.GetTakedownHistory(10, 0)
	assert.NotEmpty(t, history, "Should have history after takedown")
	
	// Add counter notice (should create more history)
	counterNotice := &CounterNotice{
		CounterNoticeID:       "CN123",
		UserName:              "Test Responder",
		GoodFaithBelief:       "I believe in good faith this was a mistake",
		SubmissionDate:        time.Now(),
		ConsentToJurisdiction: true,
	}
	
	err = db.ProcessCounterNotice("QmHistoryTest123", counterNotice)
	require.NoError(t, err)
	
	// Should have more history entries
	newHistory := db.GetTakedownHistory(10, 0)
	assert.Greater(t, len(newHistory), len(history), "Should have more history after counter notice")
}