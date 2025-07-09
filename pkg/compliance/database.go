package compliance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// ComplianceDatabase tracks DMCA takedowns and compliance at the descriptor level
type ComplianceDatabase struct {
	BlacklistedDescriptors map[string]*TakedownRecord `json:"blacklisted_descriptors"`
	TakedownHistory        []*TakedownEvent           `json:"takedown_history"`
	UserViolations         map[string][]*ViolationRecord `json:"user_violations"`
	ComplianceMetrics      *ComplianceMetrics         `json:"compliance_metrics"`
	mutex                  sync.RWMutex
}

// TakedownRecord contains details about a taken-down descriptor
type TakedownRecord struct {
	DescriptorCID    string    `json:"descriptor_cid"`
	TakedownID       string    `json:"takedown_id"`
	FilePath         string    `json:"file_path"`
	RequestorName    string    `json:"requestor_name"`
	RequestorEmail   string    `json:"requestor_email"`
	CopyrightWork    string    `json:"copyright_work"`
	TakedownDate     time.Time `json:"takedown_date"`
	Status           string    `json:"status"` // "active", "disputed", "reinstated"
	DMCANoticeHash   string    `json:"dmca_notice_hash"`
	UploaderID       string    `json:"uploader_id,omitempty"`
	
	// Legal documentation
	OriginalNotice   string    `json:"original_notice"`
	LegalBasis       string    `json:"legal_basis"`
	ProcessingNotes  string    `json:"processing_notes"`
	
	// Reinstatement tracking
	CounterNotice    *CounterNotice `json:"counter_notice,omitempty"`
	ReinstatementDate *time.Time    `json:"reinstatement_date,omitempty"`
}

// TakedownEvent tracks all takedown-related events for audit purposes
type TakedownEvent struct {
	EventID          string                 `json:"event_id"`
	EventType        string                 `json:"event_type"` // "takedown_request", "takedown_processed", "counter_notice", "reinstatement"
	DescriptorCID    string                 `json:"descriptor_cid"`
	TakedownID       string                 `json:"takedown_id"`
	Timestamp        time.Time              `json:"timestamp"`
	UserID           string                 `json:"user_id,omitempty"`
	Details          map[string]interface{} `json:"details"`
	ProcessedBy      string                 `json:"processed_by"`
	ComplianceNotes  string                 `json:"compliance_notes"`
}

// ViolationRecord tracks repeat infringer status
type ViolationRecord struct {
	ViolationID      string    `json:"violation_id"`
	UserID           string    `json:"user_id"`
	DescriptorCID    string    `json:"descriptor_cid"`
	TakedownID       string    `json:"takedown_id"`
	ViolationType    string    `json:"violation_type"` // "copyright_infringement", "repeat_infringement", "false_notice"
	ViolationDate    time.Time `json:"violation_date"`
	Severity         string    `json:"severity"` // "minor", "major", "severe"
	ActionTaken      string    `json:"action_taken"` // "warning", "temporary_suspension", "permanent_ban"
	ResolutionStatus string    `json:"resolution_status"` // "pending", "resolved", "appealed"
}

// CounterNotice represents a DMCA counter-notification
type CounterNotice struct {
	CounterNoticeID  string    `json:"counter_notice_id"`
	UserID           string    `json:"user_id"`
	UserName         string    `json:"user_name"`
	UserEmail        string    `json:"user_email"`
	UserAddress      string    `json:"user_address"`
	SwornStatement   string    `json:"sworn_statement"`
	GoodFaithBelief  string    `json:"good_faith_belief"`
	ConsentToJurisdiction bool `json:"consent_to_jurisdiction"`
	Signature        string    `json:"signature"`
	SubmissionDate   time.Time `json:"submission_date"`
	Status           string    `json:"status"` // "pending", "valid", "invalid", "processed"
	ProcessingNotes  string    `json:"processing_notes"`
}

// ComplianceMetrics tracks overall compliance statistics
type ComplianceMetrics struct {
	TotalTakedowns       int64     `json:"total_takedowns"`
	ActiveTakedowns      int64     `json:"active_takedowns"`
	DisputedTakedowns    int64     `json:"disputed_takedowns"`
	ReinstatedDescriptors int64    `json:"reinstated_descriptors"`
	UniqueRequestors     int64     `json:"unique_requestors"`
	RepeatInfringers     int64     `json:"repeat_infringers"`
	CounterNotices       int64     `json:"counter_notices"`
	AverageProcessingTime time.Duration `json:"average_processing_time"`
	LastUpdated          time.Time `json:"last_updated"`
}

// NewComplianceDatabase creates a new compliance database
func NewComplianceDatabase() *ComplianceDatabase {
	return &ComplianceDatabase{
		BlacklistedDescriptors: make(map[string]*TakedownRecord),
		TakedownHistory:        make([]*TakedownEvent, 0),
		UserViolations:         make(map[string][]*ViolationRecord),
		ComplianceMetrics: &ComplianceMetrics{
			LastUpdated: time.Now(),
		},
	}
}

// IsDescriptorBlacklisted checks if a descriptor CID is currently blacklisted
func (db *ComplianceDatabase) IsDescriptorBlacklisted(descriptorCID string) bool {
	db.mutex.RLock()
	defer db.mutex.RUnlock()
	
	record, exists := db.BlacklistedDescriptors[descriptorCID]
	if !exists {
		return false
	}
	
	// Check if takedown is still active
	return record.Status == "active"
}

// AddTakedownRecord adds a new takedown record to the database
func (db *ComplianceDatabase) AddTakedownRecord(record *TakedownRecord) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()
	
	// Generate unique takedown ID if not provided
	if record.TakedownID == "" {
		record.TakedownID = db.generateTakedownID(record.DescriptorCID, record.RequestorEmail)
	}
	
	// Add to blacklist
	db.BlacklistedDescriptors[record.DescriptorCID] = record
	
	// Record the event
	event := &TakedownEvent{
		EventID:       db.generateEventID(),
		EventType:     "takedown_processed",
		DescriptorCID: record.DescriptorCID,
		TakedownID:    record.TakedownID,
		Timestamp:     time.Now(),
		Details: map[string]interface{}{
			"requestor":     record.RequestorName,
			"copyright_work": record.CopyrightWork,
			"legal_basis":   record.LegalBasis,
		},
		ProcessedBy:     "dmca_compliance_system",
		ComplianceNotes: "Descriptor takedown processed per DMCA 512(c)",
	}
	
	db.TakedownHistory = append(db.TakedownHistory, event)
	
	// Track user violation if uploader ID is known
	if record.UploaderID != "" {
		violation := &ViolationRecord{
			ViolationID:   db.generateViolationID(),
			UserID:        record.UploaderID,
			DescriptorCID: record.DescriptorCID,
			TakedownID:    record.TakedownID,
			ViolationType: "copyright_infringement",
			ViolationDate: time.Now(),
			Severity:      "major",
			ActionTaken:   "descriptor_removed",
			ResolutionStatus: "resolved",
		}
		
		db.UserViolations[record.UploaderID] = append(db.UserViolations[record.UploaderID], violation)
	}
	
	// Update metrics
	db.updateMetrics()
	
	return nil
}

// GetTakedownRecord retrieves a takedown record by descriptor CID
func (db *ComplianceDatabase) GetTakedownRecord(descriptorCID string) (*TakedownRecord, bool) {
	db.mutex.RLock()
	defer db.mutex.RUnlock()
	
	record, exists := db.BlacklistedDescriptors[descriptorCID]
	return record, exists
}

// ProcessCounterNotice processes a DMCA counter-notification
func (db *ComplianceDatabase) ProcessCounterNotice(descriptorCID string, counterNotice *CounterNotice) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()
	
	record, exists := db.BlacklistedDescriptors[descriptorCID]
	if !exists {
		return fmt.Errorf("no takedown record found for descriptor %s", descriptorCID)
	}
	
	// Validate counter notice
	if err := db.validateCounterNotice(counterNotice); err != nil {
		return fmt.Errorf("invalid counter notice: %w", err)
	}
	
	// Add counter notice to record
	record.CounterNotice = counterNotice
	record.Status = "disputed"
	
	// Record the event
	event := &TakedownEvent{
		EventID:       db.generateEventID(),
		EventType:     "counter_notice",
		DescriptorCID: descriptorCID,
		TakedownID:    record.TakedownID,
		Timestamp:     time.Now(),
		UserID:        counterNotice.UserID,
		Details: map[string]interface{}{
			"counter_notice_id": counterNotice.CounterNoticeID,
			"user_name":         counterNotice.UserName,
			"sworn_statement":   counterNotice.SwornStatement,
		},
		ProcessedBy:     "dmca_compliance_system",
		ComplianceNotes: "Counter-notification received and processed per DMCA 512(g)",
	}
	
	db.TakedownHistory = append(db.TakedownHistory, event)
	db.updateMetrics()
	
	return nil
}

// ReinstateDescriptor reinstates a descriptor after counter-notice waiting period
func (db *ComplianceDatabase) ReinstateDescriptor(descriptorCID string, reason string) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()
	
	record, exists := db.BlacklistedDescriptors[descriptorCID]
	if !exists {
		return fmt.Errorf("no takedown record found for descriptor %s", descriptorCID)
	}
	
	// Update record status
	record.Status = "reinstated"
	now := time.Now()
	record.ReinstatementDate = &now
	
	// Record the event
	event := &TakedownEvent{
		EventID:       db.generateEventID(),
		EventType:     "reinstatement",
		DescriptorCID: descriptorCID,
		TakedownID:    record.TakedownID,
		Timestamp:     now,
		Details: map[string]interface{}{
			"reason": reason,
		},
		ProcessedBy:     "dmca_compliance_system",
		ComplianceNotes: "Descriptor reinstated per DMCA 512(g) procedures",
	}
	
	db.TakedownHistory = append(db.TakedownHistory, event)
	db.updateMetrics()
	
	return nil
}

// GetUserViolations returns violation history for a user
func (db *ComplianceDatabase) GetUserViolations(userID string) []*ViolationRecord {
	db.mutex.RLock()
	defer db.mutex.RUnlock()
	
	violations, exists := db.UserViolations[userID]
	if !exists {
		return make([]*ViolationRecord, 0)
	}
	
	// Return copy to avoid race conditions
	result := make([]*ViolationRecord, len(violations))
	copy(result, violations)
	return result
}

// IsRepeatInfringer checks if a user qualifies as a repeat infringer
func (db *ComplianceDatabase) IsRepeatInfringer(userID string) bool {
	violations := db.GetUserViolations(userID)
	
	// Count major violations in the last 6 months
	cutoff := time.Now().Add(-6 * 30 * 24 * time.Hour)
	count := 0
	
	for _, violation := range violations {
		if violation.ViolationDate.After(cutoff) && 
		   (violation.Severity == "major" || violation.Severity == "severe") {
			count++
		}
	}
	
	// Three strikes rule - 3 major violations in 6 months
	return count >= 3
}

// GetComplianceMetrics returns current compliance metrics
func (db *ComplianceDatabase) GetComplianceMetrics() *ComplianceMetrics {
	db.mutex.RLock()
	defer db.mutex.RUnlock()
	
	// Return copy to avoid race conditions
	metrics := *db.ComplianceMetrics
	return &metrics
}

// GetTakedownHistory returns paginated takedown history
func (db *ComplianceDatabase) GetTakedownHistory(limit, offset int) []*TakedownEvent {
	db.mutex.RLock()
	defer db.mutex.RUnlock()
	
	total := len(db.TakedownHistory)
	if offset >= total {
		return make([]*TakedownEvent, 0)
	}
	
	end := offset + limit
	if end > total {
		end = total
	}
	
	// Return copy to avoid race conditions
	result := make([]*TakedownEvent, end-offset)
	copy(result, db.TakedownHistory[offset:end])
	return result
}

// ExportComplianceReport generates a compliance report for legal purposes
func (db *ComplianceDatabase) ExportComplianceReport(startDate, endDate time.Time) (*ComplianceReport, error) {
	db.mutex.RLock()
	defer db.mutex.RUnlock()
	
	report := &ComplianceReport{
		ReportID:    db.generateReportID(),
		StartDate:   startDate,
		EndDate:     endDate,
		GeneratedAt: time.Now(),
		Takedowns:   make([]*TakedownRecord, 0),
		Events:      make([]*TakedownEvent, 0),
		Summary:     make(map[string]interface{}),
	}
	
	// Collect takedowns in date range
	for _, record := range db.BlacklistedDescriptors {
		if record.TakedownDate.After(startDate) && record.TakedownDate.Before(endDate) {
			report.Takedowns = append(report.Takedowns, record)
		}
	}
	
	// Collect events in date range
	for _, event := range db.TakedownHistory {
		if event.Timestamp.After(startDate) && event.Timestamp.Before(endDate) {
			report.Events = append(report.Events, event)
		}
	}
	
	// Generate summary statistics
	report.Summary["total_takedowns"] = len(report.Takedowns)
	report.Summary["total_events"] = len(report.Events)
	report.Summary["active_takedowns"] = db.ComplianceMetrics.ActiveTakedowns
	report.Summary["disputed_takedowns"] = db.ComplianceMetrics.DisputedTakedowns
	report.Summary["counter_notices"] = db.ComplianceMetrics.CounterNotices
	
	return report, nil
}

// ComplianceReport contains compliance data for legal reporting
type ComplianceReport struct {
	ReportID    string                 `json:"report_id"`
	StartDate   time.Time              `json:"start_date"`
	EndDate     time.Time              `json:"end_date"`
	GeneratedAt time.Time              `json:"generated_at"`
	Takedowns   []*TakedownRecord      `json:"takedowns"`
	Events      []*TakedownEvent       `json:"events"`
	Summary     map[string]interface{} `json:"summary"`
}

// Helper methods

func (db *ComplianceDatabase) generateTakedownID(descriptorCID, requestorEmail string) string {
	data := fmt.Sprintf("%s-%s-%d", descriptorCID, requestorEmail, time.Now().Unix())
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("TD-%s", hex.EncodeToString(hash[:8]))
}

func (db *ComplianceDatabase) generateEventID() string {
	data := fmt.Sprintf("event-%d-%d", time.Now().UnixNano(), len(db.TakedownHistory))
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("EV-%s", hex.EncodeToString(hash[:8]))
}

func (db *ComplianceDatabase) generateViolationID() string {
	data := fmt.Sprintf("violation-%d", time.Now().UnixNano())
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("VL-%s", hex.EncodeToString(hash[:8]))
}

func (db *ComplianceDatabase) generateReportID() string {
	data := fmt.Sprintf("report-%d", time.Now().UnixNano())
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("RPT-%s", hex.EncodeToString(hash[:8]))
}

func (db *ComplianceDatabase) validateCounterNotice(notice *CounterNotice) error {
	if notice.UserName == "" {
		return fmt.Errorf("user name is required")
	}
	if notice.UserEmail == "" {
		return fmt.Errorf("user email is required")
	}
	if notice.UserAddress == "" {
		return fmt.Errorf("user address is required")
	}
	if notice.SwornStatement == "" {
		return fmt.Errorf("sworn statement is required")
	}
	if notice.GoodFaithBelief == "" {
		return fmt.Errorf("good faith belief statement is required")
	}
	if !notice.ConsentToJurisdiction {
		return fmt.Errorf("consent to jurisdiction is required")
	}
	if notice.Signature == "" {
		return fmt.Errorf("signature is required")
	}
	return nil
}

func (db *ComplianceDatabase) updateMetrics() {
	// Count active takedowns
	activeCount := int64(0)
	disputedCount := int64(0)
	reinstatedCount := int64(0)
	
	for _, record := range db.BlacklistedDescriptors {
		switch record.Status {
		case "active":
			activeCount++
		case "disputed":
			disputedCount++
		case "reinstated":
			reinstatedCount++
		}
	}
	
	// Count counter notices
	counterNoticeCount := int64(0)
	for _, record := range db.BlacklistedDescriptors {
		if record.CounterNotice != nil {
			counterNoticeCount++
		}
	}
	
	// Count unique requestors
	requestors := make(map[string]bool)
	for _, record := range db.BlacklistedDescriptors {
		requestors[record.RequestorEmail] = true
	}
	
	// Count repeat infringers
	repeatInfringers := int64(0)
	for userID := range db.UserViolations {
		if db.IsRepeatInfringer(userID) {
			repeatInfringers++
		}
	}
	
	// Update metrics
	db.ComplianceMetrics.TotalTakedowns = int64(len(db.BlacklistedDescriptors))
	db.ComplianceMetrics.ActiveTakedowns = activeCount
	db.ComplianceMetrics.DisputedTakedowns = disputedCount
	db.ComplianceMetrics.ReinstatedDescriptors = reinstatedCount
	db.ComplianceMetrics.UniqueRequestors = int64(len(requestors))
	db.ComplianceMetrics.RepeatInfringers = repeatInfringers
	db.ComplianceMetrics.CounterNotices = counterNoticeCount
	db.ComplianceMetrics.LastUpdated = time.Now()
}

// SaveToFile saves the compliance database to a JSON file
func (db *ComplianceDatabase) SaveToFile(filename string) error {
	db.mutex.RLock()
	defer db.mutex.RUnlock()
	
	data, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal compliance database: %w", err)
	}
	
	// In a real implementation, this would write to a file
	// For now, we'll just validate the JSON is correct
	var test ComplianceDatabase
	if err := json.Unmarshal(data, &test); err != nil {
		return fmt.Errorf("failed to validate JSON: %w", err)
	}
	
	return nil
}

// LoadFromFile loads the compliance database from a JSON file
func (db *ComplianceDatabase) LoadFromFile(filename string) error {
	// In a real implementation, this would read from a file
	// For now, we'll just initialize with empty data
	db.mutex.Lock()
	defer db.mutex.Unlock()
	
	db.BlacklistedDescriptors = make(map[string]*TakedownRecord)
	db.TakedownHistory = make([]*TakedownEvent, 0)
	db.UserViolations = make(map[string][]*ViolationRecord)
	db.ComplianceMetrics = &ComplianceMetrics{
		LastUpdated: time.Now(),
	}
	
	return nil
}