package compliance

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"
)

// TakedownProcessor handles DMCA takedown notices and compliance procedures
type TakedownProcessor struct {
	database     *ComplianceDatabase
	validator    *DMCANoticeValidator
	notifier     *UserNotificationSystem
	auditLogger  *ComplianceAuditLogger
	config       *ProcessorConfig
}

// ProcessorConfig defines configuration for takedown processing
type ProcessorConfig struct {
	AutoProcessValid     bool          `json:"auto_process_valid"`     // Automatically process valid notices
	CounterNoticeWaitTime time.Duration `json:"counter_notice_wait_time"` // DMCA requires 10-14 business days
	RequireSwornStatement bool          `json:"require_sworn_statement"` // Require sworn statements
	ValidateEmailDomains  bool          `json:"validate_email_domains"`  // Validate requestor email domains
	MaxProcessingTime     time.Duration `json:"max_processing_time"`     // Maximum time to process notice
	AdminEmail           string        `json:"admin_email"`             // Email for compliance notifications
	DMCAAgentEmail       string        `json:"dmca_agent_email"`        // Designated DMCA agent email
}

// DMCANoticeValidator validates incoming DMCA notices
type DMCANoticeValidator struct {
	config *ProcessorConfig
}

// DMCANotice represents a DMCA takedown notice
type DMCANotice struct {
	NoticeID             string    `json:"notice_id"`
	ReceivedDate         time.Time `json:"received_date"`
	RequestorName        string    `json:"requestor_name"`
	RequestorEmail       string    `json:"requestor_email"`
	RequestorAddress     string    `json:"requestor_address"`
	RequestorPhone       string    `json:"requestor_phone,omitempty"`
	
	// Copyright work information
	CopyrightWork        string    `json:"copyright_work"`
	CopyrightOwner       string    `json:"copyright_owner"`
	RegistrationNumber   string    `json:"registration_number,omitempty"`
	
	// Infringing material identification
	InfringingURLs       []string  `json:"infringing_urls"`
	DescriptorCIDs       []string  `json:"descriptor_cids"`
	Description          string    `json:"description"`
	
	// Legal statements
	SwornStatement       string    `json:"sworn_statement"`
	GoodFaithBelief      string    `json:"good_faith_belief"`
	AccuracyStatement    string    `json:"accuracy_statement"`
	Signature            string    `json:"signature"`
	
	// Processing information
	OriginalNotice       string    `json:"original_notice"`
	ValidationResult     *ValidationResult `json:"validation_result"`
	ProcessingStatus     string    `json:"processing_status"` // "received", "validated", "processed", "rejected"
	ProcessingNotes      string    `json:"processing_notes"`
}

// ValidationResult contains the result of DMCA notice validation
type ValidationResult struct {
	Valid        bool     `json:"valid"`
	Errors       []string `json:"errors"`
	Warnings     []string `json:"warnings"`
	Requirements []string `json:"requirements"`
	Score        float64  `json:"score"` // Validation score (0.0-1.0)
}

// UserNotificationSystem handles notifying users about takedowns
type UserNotificationSystem struct {
	config *ProcessorConfig
}

// ComplianceAuditLogger logs all compliance-related activities
type ComplianceAuditLogger struct {
	logEntries []*AuditLogEntry
}

// AuditLogEntry represents a single audit log entry
type AuditLogEntry struct {
	EntryID     string                 `json:"entry_id"`
	Timestamp   time.Time              `json:"timestamp"`
	EventType   string                 `json:"event_type"`
	UserID      string                 `json:"user_id,omitempty"`
	TargetID    string                 `json:"target_id"` // Descriptor CID, user ID, etc.
	Action      string                 `json:"action"`
	Details     map[string]interface{} `json:"details"`
	Result      string                 `json:"result"`
	IPAddress   string                 `json:"ip_address,omitempty"`
	UserAgent   string                 `json:"user_agent,omitempty"`
}

// NewTakedownProcessor creates a new takedown processor
func NewTakedownProcessor(database *ComplianceDatabase, config *ProcessorConfig) *TakedownProcessor {
	if config == nil {
		config = DefaultProcessorConfig()
	}
	
	return &TakedownProcessor{
		database:    database,
		validator:   &DMCANoticeValidator{config: config},
		notifier:    &UserNotificationSystem{config: config},
		auditLogger: &ComplianceAuditLogger{logEntries: make([]*AuditLogEntry, 0)},
		config:      config,
	}
}

// DefaultProcessorConfig returns default processor configuration
func DefaultProcessorConfig() *ProcessorConfig {
	return &ProcessorConfig{
		AutoProcessValid:      true,
		CounterNoticeWaitTime: 14 * 24 * time.Hour, // 14 days
		RequireSwornStatement: true,
		ValidateEmailDomains:  true,
		MaxProcessingTime:     24 * time.Hour, // 24 hours
		AdminEmail:           "admin@noisefs.org",
		DMCAAgentEmail:       "dmca@noisefs.org",
	}
}

// ProcessDMCANotice processes an incoming DMCA takedown notice
func (processor *TakedownProcessor) ProcessDMCANotice(notice *DMCANotice) (*ProcessingResult, error) {
	// Generate notice ID if not provided
	if notice.NoticeID == "" {
		notice.NoticeID = processor.generateNoticeID(notice)
	}
	
	if notice.ReceivedDate.IsZero() {
		notice.ReceivedDate = time.Now()
	}
	
	result := &ProcessingResult{
		NoticeID:        notice.NoticeID,
		ProcessingDate:  time.Now(),
		Status:          "processing",
		ActionsToken:    make([]string, 0),
		AffectedDescriptors: make([]string, 0),
	}
	
	// Log notice receipt
	processor.auditLogger.LogEvent("dmca_notice_received", "", notice.NoticeID, "notice_received", map[string]interface{}{
		"requestor":  notice.RequestorName,
		"work_count": len(notice.DescriptorCIDs),
	}, "received")
	
	// Step 1: Validate the notice
	validation, err := processor.validator.ValidateNotice(notice)
	if err != nil {
		result.Status = "validation_error"
		result.Error = err.Error()
		return result, err
	}
	
	notice.ValidationResult = validation
	
	// Step 2: Handle validation results
	if !validation.Valid {
		notice.ProcessingStatus = "rejected"
		notice.ProcessingNotes = fmt.Sprintf("Notice rejected: %s", strings.Join(validation.Errors, "; "))
		
		result.Status = "rejected"
		result.ValidationErrors = validation.Errors
		
		processor.auditLogger.LogEvent("dmca_notice_rejected", "", notice.NoticeID, "notice_validation_failed", map[string]interface{}{
			"errors": validation.Errors,
			"score":  validation.Score,
		}, "rejected")
		
		return result, nil
	}
	
	// Step 3: Process valid notice
	if processor.config.AutoProcessValid {
		processingResult, err := processor.processValidNotice(notice)
		if err != nil {
			result.Status = "processing_error"
			result.Error = err.Error()
			return result, err
		}
		
		result.Status = "processed"
		result.ActionsToken = processingResult.ActionsToken
		result.AffectedDescriptors = processingResult.AffectedDescriptors
		result.TakedownRecords = processingResult.TakedownRecords
	} else {
		result.Status = "pending_manual_review"
		notice.ProcessingStatus = "pending_manual_review"
	}
	
	processor.auditLogger.LogEvent("dmca_notice_processed", "", notice.NoticeID, "notice_processed", map[string]interface{}{
		"status":             result.Status,
		"affected_count":     len(result.AffectedDescriptors),
		"auto_processed":     processor.config.AutoProcessValid,
	}, result.Status)
	
	return result, nil
}

// ProcessingResult contains the result of processing a DMCA notice
type ProcessingResult struct {
	NoticeID             string             `json:"notice_id"`
	ProcessingDate       time.Time          `json:"processing_date"`
	Status               string             `json:"status"` // "processed", "rejected", "pending_manual_review", "error"
	ValidationErrors     []string           `json:"validation_errors,omitempty"`
	ActionsToken         []string           `json:"actions_taken"`
	AffectedDescriptors  []string           `json:"affected_descriptors"`
	TakedownRecords      []*TakedownRecord  `json:"takedown_records,omitempty"`
	Error                string             `json:"error,omitempty"`
	CounterNoticeDeadline *time.Time        `json:"counter_notice_deadline,omitempty"`
}

// processValidNotice processes a validated DMCA notice
func (processor *TakedownProcessor) processValidNotice(notice *DMCANotice) (*ProcessingResult, error) {
	result := &ProcessingResult{
		NoticeID:            notice.NoticeID,
		ProcessingDate:      time.Now(),
		ActionsToken:        make([]string, 0),
		AffectedDescriptors: make([]string, 0),
		TakedownRecords:     make([]*TakedownRecord, 0),
	}
	
	// Process each descriptor CID mentioned in the notice
	for _, descriptorCID := range notice.DescriptorCIDs {
		// Check if already taken down
		if processor.database.IsDescriptorBlacklisted(descriptorCID) {
			result.ActionsToken = append(result.ActionsToken, fmt.Sprintf("Descriptor %s already blacklisted", descriptorCID[:8]))
			continue
		}
		
		// Create takedown record
		takedownRecord := &TakedownRecord{
			DescriptorCID:   descriptorCID,
			TakedownID:      processor.generateTakedownID(descriptorCID, notice.NoticeID),
			FilePath:        processor.extractFilePath(descriptorCID, notice),
			RequestorName:   notice.RequestorName,
			RequestorEmail:  notice.RequestorEmail,
			CopyrightWork:   notice.CopyrightWork,
			TakedownDate:    time.Now(),
			Status:          "active",
			DMCANoticeHash:  processor.calculateNoticeHash(notice),
			OriginalNotice:  notice.OriginalNotice,
			LegalBasis:      processor.extractLegalBasis(notice),
			ProcessingNotes: fmt.Sprintf("Processed DMCA notice %s", notice.NoticeID),
		}
		
		// Add to database
		if err := processor.database.AddTakedownRecord(takedownRecord); err != nil {
			return nil, fmt.Errorf("failed to add takedown record for %s: %w", descriptorCID, err)
		}
		
		result.AffectedDescriptors = append(result.AffectedDescriptors, descriptorCID)
		result.TakedownRecords = append(result.TakedownRecords, takedownRecord)
		result.ActionsToken = append(result.ActionsToken, fmt.Sprintf("Blacklisted descriptor %s", descriptorCID[:8]))
		
		// Notify affected users (if we can identify them)
		if err := processor.notifier.NotifyTakedown(descriptorCID, takedownRecord); err != nil {
			processor.auditLogger.LogEvent("notification_failed", "", descriptorCID, "user_notification", map[string]interface{}{
				"error": err.Error(),
			}, "failed")
		}
		
		processor.auditLogger.LogEvent("descriptor_blacklisted", "", descriptorCID, "takedown_executed", map[string]interface{}{
			"takedown_id": takedownRecord.TakedownID,
			"requestor":   notice.RequestorName,
			"work":        notice.CopyrightWork,
		}, "blacklisted")
	}
	
	// Set counter-notice deadline
	deadline := time.Now().Add(processor.config.CounterNoticeWaitTime)
	result.CounterNoticeDeadline = &deadline
	
	notice.ProcessingStatus = "processed"
	notice.ProcessingNotes = fmt.Sprintf("Processed %d descriptors", len(result.AffectedDescriptors))
	
	return result, nil
}

// ValidateNotice validates a DMCA takedown notice
func (validator *DMCANoticeValidator) ValidateNotice(notice *DMCANotice) (*ValidationResult, error) {
	result := &ValidationResult{
		Valid:        true,
		Errors:       make([]string, 0),
		Warnings:     make([]string, 0),
		Requirements: make([]string, 0),
		Score:        1.0,
	}
	
	score := 1.0
	
	// Required fields validation
	if notice.RequestorName == "" {
		result.Errors = append(result.Errors, "Requestor name is required")
		score -= 0.2
	}
	
	if notice.RequestorEmail == "" {
		result.Errors = append(result.Errors, "Requestor email is required")
		score -= 0.2
	} else if !validator.isValidEmail(notice.RequestorEmail) {
		result.Errors = append(result.Errors, "Invalid requestor email format")
		score -= 0.1
	}
	
	if notice.CopyrightWork == "" {
		result.Errors = append(result.Errors, "Copyright work description is required")
		score -= 0.2
	}
	
	if len(notice.DescriptorCIDs) == 0 && len(notice.InfringingURLs) == 0 {
		result.Errors = append(result.Errors, "Must specify at least one infringing descriptor CID or URL")
		score -= 0.3
	}
	
	// Validate descriptor CIDs format
	for _, cid := range notice.DescriptorCIDs {
		if !validator.isValidCID(cid) {
			result.Errors = append(result.Errors, fmt.Sprintf("Invalid descriptor CID format: %s", cid))
			score -= 0.1
		}
	}
	
	// DMCA statutory requirements
	if validator.config.RequireSwornStatement && notice.SwornStatement == "" {
		result.Errors = append(result.Errors, "Sworn statement is required under DMCA 512(c)(3)(A)(v)")
		score -= 0.2
	}
	
	if notice.GoodFaithBelief == "" {
		result.Errors = append(result.Errors, "Good faith belief statement is required under DMCA 512(c)(3)(A)(v)")
		score -= 0.2
	}
	
	if notice.AccuracyStatement == "" {
		result.Warnings = append(result.Warnings, "Accuracy statement recommended for DMCA compliance")
		score -= 0.05
	}
	
	if notice.Signature == "" {
		result.Errors = append(result.Errors, "Signature is required under DMCA 512(c)(3)(A)(vi)")
		score -= 0.2
	}
	
	// Additional validation
	if validator.config.ValidateEmailDomains {
		if validator.isSuspiciousEmailDomain(notice.RequestorEmail) {
			result.Warnings = append(result.Warnings, "Requestor email domain may require additional verification")
			score -= 0.05
		}
	}
	
	// Update result
	result.Score = score
	if len(result.Errors) > 0 {
		result.Valid = false
	}
	
	// Add requirements for improvement
	if !result.Valid {
		result.Requirements = append(result.Requirements, "All required fields must be completed")
		result.Requirements = append(result.Requirements, "DMCA statutory requirements must be met per 17 USC 512(c)(3)")
	}
	
	return result, nil
}

// ProcessCounterNotice processes a DMCA counter-notification
func (processor *TakedownProcessor) ProcessCounterNotice(descriptorCID string, counterNotice *CounterNotice) (*CounterNoticeResult, error) {
	counterNotice.CounterNoticeID = processor.generateCounterNoticeID(descriptorCID, counterNotice.UserEmail)
	counterNotice.SubmissionDate = time.Now()
	counterNotice.Status = "pending"
	
	// Validate counter notice
	if err := processor.database.ProcessCounterNotice(descriptorCID, counterNotice); err != nil {
		return &CounterNoticeResult{
			CounterNoticeID: counterNotice.CounterNoticeID,
			Status:          "rejected",
			Error:           err.Error(),
		}, err
	}
	
	// Schedule reinstatement after waiting period
	reinstatementDate := time.Now().Add(processor.config.CounterNoticeWaitTime)
	
	processor.auditLogger.LogEvent("counter_notice_received", counterNotice.UserID, descriptorCID, "counter_notice_submitted", map[string]interface{}{
		"counter_notice_id": counterNotice.CounterNoticeID,
		"user_name":         counterNotice.UserName,
		"reinstatement_date": reinstatementDate,
	}, "pending")
	
	return &CounterNoticeResult{
		CounterNoticeID:    counterNotice.CounterNoticeID,
		Status:             "accepted",
		ReinstatementDate:  &reinstatementDate,
		WaitingPeriod:      processor.config.CounterNoticeWaitTime,
	}, nil
}

// CounterNoticeResult contains the result of processing a counter-notice
type CounterNoticeResult struct {
	CounterNoticeID   string         `json:"counter_notice_id"`
	Status            string         `json:"status"` // "accepted", "rejected"
	ReinstatementDate *time.Time     `json:"reinstatement_date,omitempty"`
	WaitingPeriod     time.Duration  `json:"waiting_period,omitempty"`
	Error             string         `json:"error,omitempty"`
}

// ProcessPendingReinstatements processes descriptors pending reinstatement after counter-notice waiting period
func (processor *TakedownProcessor) ProcessPendingReinstatements() error {
	// Get all disputed takedowns
	for descriptorCID, record := range processor.database.BlacklistedDescriptors {
		if record.Status == "disputed" && record.CounterNotice != nil {
			// Check if waiting period has elapsed
			waitingPeriodEnd := record.CounterNotice.SubmissionDate.Add(processor.config.CounterNoticeWaitTime)
			if time.Now().After(waitingPeriodEnd) {
				// Reinstate the descriptor
				if err := processor.database.ReinstateDescriptor(descriptorCID, "Counter-notice waiting period elapsed"); err != nil {
					processor.auditLogger.LogEvent("reinstatement_failed", record.CounterNotice.UserID, descriptorCID, "automatic_reinstatement", map[string]interface{}{
						"error": err.Error(),
					}, "failed")
					continue
				}
				
				processor.auditLogger.LogEvent("descriptor_reinstated", record.CounterNotice.UserID, descriptorCID, "automatic_reinstatement", map[string]interface{}{
					"counter_notice_id": record.CounterNotice.CounterNoticeID,
					"waiting_period":    processor.config.CounterNoticeWaitTime.String(),
				}, "reinstated")
				
				// Notify user of reinstatement
				if err := processor.notifier.NotifyReinstatement(descriptorCID, record); err != nil {
					processor.auditLogger.LogEvent("notification_failed", record.CounterNotice.UserID, descriptorCID, "reinstatement_notification", map[string]interface{}{
						"error": err.Error(),
					}, "failed")
				}
			}
		}
	}
	
	return nil
}

// GetComplianceAuditLog returns paginated audit log entries
func (processor *TakedownProcessor) GetComplianceAuditLog(limit, offset int) []*AuditLogEntry {
	total := len(processor.auditLogger.logEntries)
	if offset >= total {
		return make([]*AuditLogEntry, 0)
	}
	
	end := offset + limit
	if end > total {
		end = total
	}
	
	result := make([]*AuditLogEntry, end-offset)
	copy(result, processor.auditLogger.logEntries[offset:end])
	return result
}

// Helper methods

func (processor *TakedownProcessor) generateNoticeID(notice *DMCANotice) string {
	data := fmt.Sprintf("%s-%s-%d", notice.RequestorEmail, notice.CopyrightWork, time.Now().Unix())
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("DMCA-%s", hex.EncodeToString(hash[:8]))
}

func (processor *TakedownProcessor) generateTakedownID(descriptorCID, noticeID string) string {
	data := fmt.Sprintf("%s-%s", descriptorCID, noticeID)
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("TD-%s", hex.EncodeToString(hash[:8]))
}

func (processor *TakedownProcessor) generateCounterNoticeID(descriptorCID, userEmail string) string {
	data := fmt.Sprintf("%s-%s-%d", descriptorCID, userEmail, time.Now().Unix())
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("CN-%s", hex.EncodeToString(hash[:8]))
}

func (processor *TakedownProcessor) calculateNoticeHash(notice *DMCANotice) string {
	data := fmt.Sprintf("%s-%s-%s", notice.RequestorEmail, notice.CopyrightWork, notice.OriginalNotice)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:16])
}

func (processor *TakedownProcessor) extractFilePath(descriptorCID string, notice *DMCANotice) string {
	// Extract file path from notice description or URLs
	for _, url := range notice.InfringingURLs {
		if strings.Contains(url, descriptorCID) {
			return url
		}
	}
	return fmt.Sprintf("descriptor://%s", descriptorCID)
}

func (processor *TakedownProcessor) extractLegalBasis(notice *DMCANotice) string {
	return fmt.Sprintf("DMCA 512(c) takedown notice - Copyright: %s", notice.CopyrightWork)
}

func (validator *DMCANoticeValidator) isValidEmail(email string) bool {
	_, err := mail.ParseAddress(email)
	return err == nil
}

func (validator *DMCANoticeValidator) isValidCID(cid string) bool {
	// Basic CID format validation - should start with appropriate prefix and be reasonable length
	if len(cid) < 10 || len(cid) > 100 {
		return false
	}
	// Allow alphanumeric characters and common CID prefixes
	matched, _ := regexp.MatchString(`^[A-Za-z0-9]+$`, cid)
	return matched
}

func (validator *DMCANoticeValidator) isSuspiciousEmailDomain(email string) bool {
	// List of domains that might require additional verification
	suspiciousDomains := []string{
		"temp-mail.org",
		"10minutemail.com",
		"guerrillamail.com",
		"mailinator.com",
	}
	
	for _, domain := range suspiciousDomains {
		if strings.HasSuffix(strings.ToLower(email), "@"+domain) {
			return true
		}
	}
	return false
}

// NotifyTakedown notifies users about takedown actions
func (notifier *UserNotificationSystem) NotifyTakedown(descriptorCID string, record *TakedownRecord) error {
	// In a real implementation, this would send notifications via email, in-app messages, etc.
	// For now, we'll just log the notification
	fmt.Printf("TAKEDOWN NOTIFICATION: Descriptor %s has been taken down due to DMCA notice %s\n", 
		descriptorCID[:8], record.TakedownID)
	return nil
}

// NotifyReinstatement notifies users about descriptor reinstatement
func (notifier *UserNotificationSystem) NotifyReinstatement(descriptorCID string, record *TakedownRecord) error {
	// In a real implementation, this would send notifications
	fmt.Printf("REINSTATEMENT NOTIFICATION: Descriptor %s has been reinstated after counter-notice\n", 
		descriptorCID[:8])
	return nil
}

// LogEvent logs a compliance-related event
func (logger *ComplianceAuditLogger) LogEvent(eventType, userID, targetID, action string, details map[string]interface{}, result string) {
	entry := &AuditLogEntry{
		EntryID:   logger.generateEntryID(),
		Timestamp: time.Now(),
		EventType: eventType,
		UserID:    userID,
		TargetID:  targetID,
		Action:    action,
		Details:   details,
		Result:    result,
	}
	
	logger.logEntries = append(logger.logEntries, entry)
}

func (logger *ComplianceAuditLogger) generateEntryID() string {
	data := fmt.Sprintf("audit-%d-%d", time.Now().UnixNano(), len(logger.logEntries))
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("AL-%s", hex.EncodeToString(hash[:8]))
}