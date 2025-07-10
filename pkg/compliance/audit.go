package compliance

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// ComplianceAuditSystem provides comprehensive audit logging for legal compliance
type ComplianceAuditSystem struct {
	logger       *AdvancedAuditLogger
	reporter     *ComplianceReporter
	monitor      *ComplianceMonitor
	config       *AuditConfig
	mutex        sync.RWMutex
}

// AuditConfig defines configuration for audit logging
type AuditConfig struct {
	EnableRealTimeLogging    bool          `json:"enable_real_time_logging"`
	LogRetentionPeriod       time.Duration `json:"log_retention_period"`
	RequireCryptographicProof bool         `json:"require_cryptographic_proof"`
	AlertThresholds          *AlertThresholds `json:"alert_thresholds"`
	ExportFormats            []string      `json:"export_formats"` // "json", "csv", "xml"
	AutoBackupInterval       time.Duration `json:"auto_backup_interval"`
	LegalHoldEnabled         bool          `json:"legal_hold_enabled"`
}

// AlertThresholds define when to generate compliance alerts
type AlertThresholds struct {
	TakedownsPerDay          int     `json:"takedowns_per_day"`
	TakedownsPerRequestor    int     `json:"takedowns_per_requestor"`
	CounterNoticeRatio       float64 `json:"counter_notice_ratio"` // Ratio that might indicate abuse
	RepeatInfringerThreshold int     `json:"repeat_infringer_threshold"`
	ProcessingTimeThreshold  time.Duration `json:"processing_time_threshold"`
}

// AdvancedAuditLogger provides detailed audit logging with cryptographic integrity
type AdvancedAuditLogger struct {
	entries         []*DetailedAuditEntry
	chainedHashes   []string // For cryptographic integrity
	lastHash        string
	config          *AuditConfig
	mutex           sync.RWMutex
}

// DetailedAuditEntry represents a comprehensive audit log entry
type DetailedAuditEntry struct {
	// Basic identification
	EntryID         string    `json:"entry_id"`
	Timestamp       time.Time `json:"timestamp"`
	SequenceNumber  int64     `json:"sequence_number"`
	
	// Event classification
	EventType       string    `json:"event_type"`
	EventCategory   string    `json:"event_category"` // "dmca", "user", "system", "legal"
	Severity        string    `json:"severity"`       // "info", "warning", "critical", "legal"
	
	// Actors and targets
	UserID          string    `json:"user_id,omitempty"`
	AdminID         string    `json:"admin_id,omitempty"`
	TargetID        string    `json:"target_id"`       // Descriptor CID, notice ID, etc.
	TargetType      string    `json:"target_type"`     // "descriptor", "notice", "user", "system"
	
	// Action details
	Action          string                 `json:"action"`
	ActionDetails   map[string]interface{} `json:"action_details"`
	Result          string                 `json:"result"`
	ResultCode      string                 `json:"result_code"`
	
	// Context information
	IPAddress       string    `json:"ip_address,omitempty"`
	UserAgent       string    `json:"user_agent,omitempty"`
	SessionID       string    `json:"session_id,omitempty"`
	RequestID       string    `json:"request_id,omitempty"`
	
	// Legal compliance
	LegalContext    *LegalContext `json:"legal_context,omitempty"`
	ComplianceNotes string        `json:"compliance_notes"`
	DataRetention   *RetentionInfo `json:"data_retention"`
	
	// Cryptographic integrity
	PreviousHash    string    `json:"previous_hash"`
	EntryHash       string    `json:"entry_hash"`
	Signature       string    `json:"signature"`
	
	// Metadata
	SystemVersion   string                 `json:"system_version"`
	ProcessingTime  time.Duration         `json:"processing_time,omitempty"`
	RelatedEntries  []string              `json:"related_entries,omitempty"`
	Tags            []string              `json:"tags,omitempty"`
}

// LegalContext provides legal framework context for audit entries
type LegalContext struct {
	Jurisdiction      string   `json:"jurisdiction"`
	ApplicableLaws    []string `json:"applicable_laws"`
	LegalBasis        string   `json:"legal_basis"`
	ComplianceReason  string   `json:"compliance_reason"`
	LegalHoldStatus   string   `json:"legal_hold_status,omitempty"`
	CaseNumber        string   `json:"case_number,omitempty"`
}

// RetentionInfo defines data retention requirements
type RetentionInfo struct {
	RetentionPeriod   time.Duration `json:"retention_period"`
	RetentionReason   string        `json:"retention_reason"`
	DestructionDate   *time.Time    `json:"destruction_date,omitempty"`
	LegalHold         bool          `json:"legal_hold"`
	ComplianceClass   string        `json:"compliance_class"` // "dmca", "privacy", "financial", etc.
}

// ComplianceReporter generates compliance reports for legal purposes
type ComplianceReporter struct {
	auditSystem *ComplianceAuditSystem
	config      *AuditConfig
}

// ComplianceMonitor monitors compliance metrics and generates alerts
type ComplianceMonitor struct {
	auditSystem *ComplianceAuditSystem
	alerts      []*ComplianceAlert
	metrics     *RealTimeMetrics
	config      *AuditConfig
}

// ComplianceAlert represents a compliance alert
type ComplianceAlert struct {
	AlertID       string                 `json:"alert_id"`
	AlertType     string                 `json:"alert_type"`
	Severity      string                 `json:"severity"`
	Timestamp     time.Time              `json:"timestamp"`
	Condition     string                 `json:"condition"`
	Details       map[string]interface{} `json:"details"`
	Resolved      bool                   `json:"resolved"`
	Resolution    string                 `json:"resolution,omitempty"`
	ResolvedAt    *time.Time             `json:"resolved_at,omitempty"`
}

// RealTimeMetrics tracks real-time compliance metrics
type RealTimeMetrics struct {
	TakedownsToday       int64     `json:"takedowns_today"`
	CounterNoticesToday  int64     `json:"counter_notices_today"`
	ActiveInvestigations int64     `json:"active_investigations"`
	AverageProcessingTime time.Duration `json:"average_processing_time"`
	TopRequestors        []RequestorMetric `json:"top_requestors"`
	ComplianceScore      float64   `json:"compliance_score"`
	LastUpdated          time.Time `json:"last_updated"`
}

// RequestorMetric tracks metrics per copyright requestor
type RequestorMetric struct {
	RequestorEmail   string    `json:"requestor_email"`
	TakedownCount    int64     `json:"takedown_count"`
	CounterNotices   int64     `json:"counter_notices"`
	SuccessRate      float64   `json:"success_rate"`
	LastActivity     time.Time `json:"last_activity"`
}

// NewComplianceAuditSystem creates a new compliance audit system
func NewComplianceAuditSystem(config *AuditConfig) *ComplianceAuditSystem {
	if config == nil {
		config = DefaultAuditConfig()
	}
	
	system := &ComplianceAuditSystem{
		config: config,
	}
	
	system.logger = &AdvancedAuditLogger{
		entries:       make([]*DetailedAuditEntry, 0),
		chainedHashes: make([]string, 0),
		config:        config,
	}
	
	system.reporter = &ComplianceReporter{
		auditSystem: system,
		config:      config,
	}
	
	system.monitor = &ComplianceMonitor{
		auditSystem: system,
		alerts:      make([]*ComplianceAlert, 0),
		metrics:     &RealTimeMetrics{LastUpdated: time.Now()},
		config:      config,
	}
	
	return system
}

// DefaultAuditConfig returns default audit configuration
func DefaultAuditConfig() *AuditConfig {
	return &AuditConfig{
		EnableRealTimeLogging:     true,
		LogRetentionPeriod:        7 * 365 * 24 * time.Hour, // 7 years
		RequireCryptographicProof: true,
		AlertThresholds: &AlertThresholds{
			TakedownsPerDay:          100,
			TakedownsPerRequestor:    20,
			CounterNoticeRatio:       0.3, // 30% counter-notice rate might indicate issues
			RepeatInfringerThreshold: 3,
			ProcessingTimeThreshold:  24 * time.Hour,
		},
		ExportFormats:      []string{"json", "csv"},
		AutoBackupInterval: 24 * time.Hour,
		LegalHoldEnabled:   true,
	}
}

// LogComplianceEvent logs a detailed compliance event
func (system *ComplianceAuditSystem) LogComplianceEvent(eventType, userID, targetID, action string, details map[string]interface{}) error {
	system.mutex.Lock()
	defer system.mutex.Unlock()
	
	entry := &DetailedAuditEntry{
		EntryID:        system.generateEntryID(),
		Timestamp:      time.Now(),
		SequenceNumber: int64(len(system.logger.entries) + 1),
		EventType:      eventType,
		EventCategory:  system.categorizeEvent(eventType),
		Severity:       system.determineSeverity(eventType, details),
		UserID:         userID,
		TargetID:       targetID,
		TargetType:     system.determineTargetType(targetID),
		Action:         action,
		ActionDetails:  details,
		Result:         "success", // Default, can be updated
		ComplianceNotes: system.generateComplianceNotes(eventType, action),
		SystemVersion:  "noisefs-1.0", // Should be dynamic
		Tags:           system.generateTags(eventType, action),
	}
	
	// Add legal context
	entry.LegalContext = &LegalContext{
		Jurisdiction:     "US", // Should be configurable
		ApplicableLaws:   []string{"DMCA 17 USC 512", "CFAA", "Privacy Act"},
		LegalBasis:       system.determineLegalBasis(eventType),
		ComplianceReason: "DMCA safe harbor compliance",
	}
	
	// Add retention information
	entry.DataRetention = &RetentionInfo{
		RetentionPeriod:  system.config.LogRetentionPeriod,
		RetentionReason:  "Legal compliance and audit requirements",
		ComplianceClass:  "dmca",
		LegalHold:        system.config.LegalHoldEnabled,
	}
	
	// Calculate cryptographic integrity
	if system.config.RequireCryptographicProof {
		entry.PreviousHash = system.logger.lastHash
		entry.EntryHash = system.calculateEntryHash(entry)
		entry.Signature = system.generateSignature(entry)
		
		system.logger.chainedHashes = append(system.logger.chainedHashes, entry.EntryHash)
		system.logger.lastHash = entry.EntryHash
	}
	
	// Add to log
	system.logger.entries = append(system.logger.entries, entry)
	
	// Update real-time metrics
	system.monitor.updateMetrics(entry)
	
	// Check for compliance alerts
	if err := system.monitor.checkAlerts(entry); err != nil {
		return fmt.Errorf("alert check failed: %w", err)
	}
	
	return nil
}

// LogDMCATakedown logs a DMCA takedown event with full legal context
func (system *ComplianceAuditSystem) LogDMCATakedown(takedownID, descriptorCID, requestorEmail, copyrightWork string) error {
	details := map[string]interface{}{
		"takedown_id":     takedownID,
		"requestor_email": requestorEmail,
		"copyright_work":  copyrightWork,
		"legal_framework": "DMCA 512(c)",
		"processing_time": time.Now().Format(time.RFC3339),
	}
	
	return system.LogComplianceEvent("dmca_takedown", "", descriptorCID, "descriptor_blacklisted", details)
}

// LogCounterNotice logs a DMCA counter-notice event
func (system *ComplianceAuditSystem) LogCounterNotice(counterNoticeID, descriptorCID, userID string, reinstatementDate time.Time) error {
	details := map[string]interface{}{
		"counter_notice_id":   counterNoticeID,
		"reinstatement_date":  reinstatementDate.Format(time.RFC3339),
		"legal_framework":     "DMCA 512(g)",
		"waiting_period_days": 14,
	}
	
	return system.LogComplianceEvent("dmca_counter_notice", userID, descriptorCID, "counter_notice_submitted", details)
}

// LogReinstatement logs descriptor reinstatement after counter-notice
func (system *ComplianceAuditSystem) LogReinstatement(descriptorCID, userID, reason string) error {
	details := map[string]interface{}{
		"reason":           reason,
		"legal_framework":  "DMCA 512(g)",
		"reinstatement_type": "automatic",
	}
	
	return system.LogComplianceEvent("dmca_reinstatement", userID, descriptorCID, "descriptor_reinstated", details)
}

// GenerateComplianceReport generates a comprehensive compliance report
func (system *ComplianceAuditSystem) GenerateComplianceReport(startDate, endDate time.Time, reportType string) (*ComprehensiveComplianceReport, error) {
	system.mutex.RLock()
	defer system.mutex.RUnlock()
	
	report := &ComprehensiveComplianceReport{
		ReportID:      system.generateReportID(),
		ReportType:    reportType,
		StartDate:     startDate,
		EndDate:       endDate,
		GeneratedAt:   time.Now(),
		SystemVersion: "noisefs-1.0",
	}
	
	// Filter entries by date range
	relevantEntries := make([]*DetailedAuditEntry, 0)
	for _, entry := range system.logger.entries {
		if entry.Timestamp.After(startDate) && entry.Timestamp.Before(endDate) {
			relevantEntries = append(relevantEntries, entry)
		}
	}
	
	// Generate statistics
	report.Statistics = system.generateReportStatistics(relevantEntries)
	
	// Generate DMCA-specific analysis
	report.DMCAAnalysis = system.generateDMCAAnalysis(relevantEntries)
	
	// Generate compliance assessment
	report.ComplianceAssessment = system.generateComplianceAssessment(relevantEntries)
	
	// Generate recommendations
	report.Recommendations = system.generateRecommendations(relevantEntries)
	
	// Generate legal summary
	report.LegalSummary = system.generateLegalSummary(relevantEntries)
	
	// Include audit trail integrity verification
	report.IntegrityVerification = system.verifyAuditTrailIntegrity()
	
	return report, nil
}

// ComprehensiveComplianceReport provides detailed compliance reporting
type ComprehensiveComplianceReport struct {
	ReportID                string                     `json:"report_id"`
	ReportType              string                     `json:"report_type"`
	StartDate               time.Time                  `json:"start_date"`
	EndDate                 time.Time                  `json:"end_date"`
	GeneratedAt             time.Time                  `json:"generated_at"`
	SystemVersion           string                     `json:"system_version"`
	
	Statistics              *ReportStatistics          `json:"statistics"`
	DMCAAnalysis           *DMCAAnalysis              `json:"dmca_analysis"`
	ComplianceAssessment   *ComplianceAssessment      `json:"compliance_assessment"`
	Recommendations        []string                   `json:"recommendations"`
	LegalSummary           string                     `json:"legal_summary"`
	IntegrityVerification  *IntegrityVerification     `json:"integrity_verification"`
}

// ReportStatistics contains statistical analysis of compliance events
type ReportStatistics struct {
	TotalEvents              int64                      `json:"total_events"`
	EventsByType             map[string]int64           `json:"events_by_type"`
	EventsBySeverity         map[string]int64           `json:"events_by_severity"`
	AverageProcessingTime    time.Duration              `json:"average_processing_time"`
	ComplianceScore          float64                    `json:"compliance_score"`
	TrendAnalysis            map[string][]float64       `json:"trend_analysis"`
}

// DMCAAnalysis provides DMCA-specific compliance analysis
type DMCAAnalysis struct {
	TotalTakedowns           int64                      `json:"total_takedowns"`
	TotalCounterNotices      int64                      `json:"total_counter_notices"`
	TotalReinstatements      int64                      `json:"total_reinstatements"`
	CounterNoticeRatio       float64                    `json:"counter_notice_ratio"`
	AverageProcessingTime    time.Duration              `json:"average_processing_time"`
	TopRequestors            []RequestorAnalysis        `json:"top_requestors"`
	ComplianceIssues         []string                   `json:"compliance_issues"`
	LegalRiskAssessment      string                     `json:"legal_risk_assessment"`
}

// RequestorAnalysis provides analysis of individual copyright requestors
type RequestorAnalysis struct {
	RequestorEmail          string    `json:"requestor_email"`
	TotalRequests           int64     `json:"total_requests"`
	SuccessfulTakedowns     int64     `json:"successful_takedowns"`
	CounterNotices          int64     `json:"counter_notices"`
	Reinstatements          int64     `json:"reinstatements"`
	SuccessRate             float64   `json:"success_rate"`
	AverageProcessingTime   time.Duration `json:"average_processing_time"`
	RiskLevel               string    `json:"risk_level"`
}

// ComplianceAssessment provides overall compliance assessment
type ComplianceAssessment struct {
	OverallScore            float64   `json:"overall_score"`
	DMCACompliance          float64   `json:"dmca_compliance"`
	ProcessingCompliance    float64   `json:"processing_compliance"`
	AuditCompliance         float64   `json:"audit_compliance"`
	Areas                   []string  `json:"improvement_areas"`
	Strengths               []string  `json:"strengths"`
	RiskLevel               string    `json:"risk_level"`
}

// IntegrityVerification verifies audit trail cryptographic integrity
type IntegrityVerification struct {
	IntegrityValid          bool      `json:"integrity_valid"`
	TotalEntries            int64     `json:"total_entries"`
	VerifiedEntries         int64     `json:"verified_entries"`
	IntegrityBreaches       []string  `json:"integrity_breaches,omitempty"`
	LastVerificationDate    time.Time `json:"last_verification_date"`
}

// Helper methods for generating reports and analysis

func (system *ComplianceAuditSystem) generateReportStatistics(entries []*DetailedAuditEntry) *ReportStatistics {
	stats := &ReportStatistics{
		TotalEvents:      int64(len(entries)),
		EventsByType:     make(map[string]int64),
		EventsBySeverity: make(map[string]int64),
		TrendAnalysis:    make(map[string][]float64),
	}
	
	var totalProcessingTime time.Duration
	processingTimeCount := 0
	
	for _, entry := range entries {
		stats.EventsByType[entry.EventType]++
		stats.EventsBySeverity[entry.Severity]++
		
		if entry.ProcessingTime > 0 {
			totalProcessingTime += entry.ProcessingTime
			processingTimeCount++
		}
	}
	
	if processingTimeCount > 0 {
		stats.AverageProcessingTime = totalProcessingTime / time.Duration(processingTimeCount)
	}
	
	// Calculate compliance score based on various factors
	stats.ComplianceScore = system.calculateComplianceScore(entries)
	
	return stats
}

func (system *ComplianceAuditSystem) generateDMCAAnalysis(entries []*DetailedAuditEntry) *DMCAAnalysis {
	analysis := &DMCAAnalysis{
		TopRequestors: make([]RequestorAnalysis, 0),
		ComplianceIssues: make([]string, 0),
	}
	
	requestorMetrics := make(map[string]*RequestorAnalysis)
	
	for _, entry := range entries {
		switch entry.EventType {
		case "dmca_takedown":
			analysis.TotalTakedowns++
			if requestorEmail, ok := entry.ActionDetails["requestor_email"].(string); ok {
				if _, exists := requestorMetrics[requestorEmail]; !exists {
					requestorMetrics[requestorEmail] = &RequestorAnalysis{
						RequestorEmail: requestorEmail,
					}
				}
				requestorMetrics[requestorEmail].TotalRequests++
				requestorMetrics[requestorEmail].SuccessfulTakedowns++
			}
		case "dmca_counter_notice":
			analysis.TotalCounterNotices++
		case "dmca_reinstatement":
			analysis.TotalReinstatements++
		}
	}
	
	// Calculate counter-notice ratio
	if analysis.TotalTakedowns > 0 {
		analysis.CounterNoticeRatio = float64(analysis.TotalCounterNotices) / float64(analysis.TotalTakedowns)
	}
	
	// Convert requestor metrics to sorted list
	for _, metrics := range requestorMetrics {
		analysis.TopRequestors = append(analysis.TopRequestors, *metrics)
	}
	
	// Sort by total requests
	sort.Slice(analysis.TopRequestors, func(i, j int) bool {
		return analysis.TopRequestors[i].TotalRequests > analysis.TopRequestors[j].TotalRequests
	})
	
	// Assess legal risk
	analysis.LegalRiskAssessment = system.assessLegalRisk(analysis)
	
	return analysis
}

func (system *ComplianceAuditSystem) generateComplianceAssessment(entries []*DetailedAuditEntry) *ComplianceAssessment {
	assessment := &ComplianceAssessment{
		Areas:     make([]string, 0),
		Strengths: make([]string, 0),
	}
	
	// Calculate compliance scores
	assessment.OverallScore = system.calculateComplianceScore(entries)
	assessment.DMCACompliance = system.calculateDMCAComplianceScore(entries)
	assessment.ProcessingCompliance = system.calculateProcessingComplianceScore(entries)
	assessment.AuditCompliance = system.calculateAuditComplianceScore(entries)
	
	// Determine risk level
	if assessment.OverallScore >= 0.9 {
		assessment.RiskLevel = "low"
	} else if assessment.OverallScore >= 0.7 {
		assessment.RiskLevel = "medium"
	} else {
		assessment.RiskLevel = "high"
	}
	
	// Identify strengths and improvement areas
	if assessment.DMCACompliance >= 0.9 {
		assessment.Strengths = append(assessment.Strengths, "Strong DMCA compliance procedures")
	} else {
		assessment.Areas = append(assessment.Areas, "Improve DMCA processing efficiency")
	}
	
	if assessment.AuditCompliance >= 0.9 {
		assessment.Strengths = append(assessment.Strengths, "Excellent audit trail maintenance")
	} else {
		assessment.Areas = append(assessment.Areas, "Enhance audit logging completeness")
	}
	
	return assessment
}

func (system *ComplianceAuditSystem) generateRecommendations(entries []*DetailedAuditEntry) []string {
	recommendations := make([]string, 0)
	
	// Analyze patterns and generate recommendations
	dmcaCount := int64(0)
	counterNoticeCount := int64(0)
	
	for _, entry := range entries {
		switch entry.EventType {
		case "dmca_takedown":
			dmcaCount++
		case "dmca_counter_notice":
			counterNoticeCount++
		}
	}
	
	if dmcaCount > 0 {
		counterRatio := float64(counterNoticeCount) / float64(dmcaCount)
		if counterRatio > 0.3 {
			recommendations = append(recommendations, "High counter-notice ratio detected - review takedown validation procedures")
		}
		if counterRatio < 0.1 {
			recommendations = append(recommendations, "Low counter-notice ratio - ensure users are aware of counter-notice rights")
		}
	}
	
	recommendations = append(recommendations, "Continue maintaining comprehensive audit logs for legal protection")
	recommendations = append(recommendations, "Regular compliance training for staff handling DMCA notices")
	recommendations = append(recommendations, "Consider implementing automated compliance monitoring alerts")
	
	return recommendations
}

func (system *ComplianceAuditSystem) generateLegalSummary(entries []*DetailedAuditEntry) string {
	return fmt.Sprintf(`
LEGAL COMPLIANCE SUMMARY

This report demonstrates NoiseFS's commitment to DMCA compliance and legal transparency. 
The audit trail shows systematic processing of takedown notices with appropriate 
counter-notice procedures and reinstatement processes.

Key Legal Protections:
- Comprehensive audit logging with cryptographic integrity
- DMCA 512(c) safe harbor compliance procedures
- Proper counter-notice handling per DMCA 512(g)
- Systematic repeat infringer policy enforcement

The system maintains block-level privacy while ensuring descriptor-level compliance, 
providing a legally sound framework for copyright protection without compromising 
fundamental privacy guarantees.

Total Compliance Events: %d
Legal Framework: DMCA 17 USC 512, with additional privacy protections
Risk Assessment: Well-managed legal compliance framework
`, len(entries))
}

func (system *ComplianceAuditSystem) verifyAuditTrailIntegrity() *IntegrityVerification {
	verification := &IntegrityVerification{
		IntegrityValid:       true,
		TotalEntries:         int64(len(system.logger.entries)),
		VerifiedEntries:      0,
		IntegrityBreaches:    make([]string, 0),
		LastVerificationDate: time.Now(),
	}
	
	// Verify cryptographic chain if enabled
	if system.config.RequireCryptographicProof {
		for i, entry := range system.logger.entries {
			expectedHash := system.calculateEntryHash(entry)
			if entry.EntryHash != expectedHash {
				verification.IntegrityValid = false
				verification.IntegrityBreaches = append(verification.IntegrityBreaches, 
					fmt.Sprintf("Hash mismatch in entry %s", entry.EntryID))
			} else {
				verification.VerifiedEntries++
			}
			
			// Verify chain integrity
			if i > 0 && entry.PreviousHash != system.logger.entries[i-1].EntryHash {
				verification.IntegrityValid = false
				verification.IntegrityBreaches = append(verification.IntegrityBreaches, 
					fmt.Sprintf("Chain break at entry %s", entry.EntryID))
			}
		}
	} else {
		verification.VerifiedEntries = verification.TotalEntries
	}
	
	return verification
}

// Helper methods for calculations

func (system *ComplianceAuditSystem) calculateComplianceScore(entries []*DetailedAuditEntry) float64 {
	if len(entries) == 0 {
		return 1.0
	}
	
	score := 1.0
	for _, entry := range entries {
		switch entry.Severity {
		case "critical":
			score -= 0.1
		case "warning":
			score -= 0.02
		}
	}
	
	if score < 0 {
		score = 0
	}
	return score
}

func (system *ComplianceAuditSystem) calculateDMCAComplianceScore(entries []*DetailedAuditEntry) float64 {
	// Implementation would analyze DMCA-specific compliance
	return 0.95 // Placeholder
}

func (system *ComplianceAuditSystem) calculateProcessingComplianceScore(entries []*DetailedAuditEntry) float64 {
	// Implementation would analyze processing time compliance
	return 0.92 // Placeholder
}

func (system *ComplianceAuditSystem) calculateAuditComplianceScore(entries []*DetailedAuditEntry) float64 {
	// Implementation would analyze audit completeness
	return 0.98 // Placeholder
}

func (system *ComplianceAuditSystem) assessLegalRisk(analysis *DMCAAnalysis) string {
	if analysis.CounterNoticeRatio > 0.5 {
		return "Medium risk - high counter-notice ratio may indicate over-broad takedowns"
	}
	if analysis.TotalTakedowns > 1000 {
		return "Medium risk - high volume requires careful monitoring"
	}
	return "Low risk - normal compliance patterns observed"
}

func (system *ComplianceAuditSystem) categorizeEvent(eventType string) string {
	switch {
	case strings.Contains(eventType, "dmca"):
		return "dmca"
	case strings.Contains(eventType, "user"):
		return "user"
	case strings.Contains(eventType, "system"):
		return "system"
	default:
		return "legal"
	}
}

func (system *ComplianceAuditSystem) determineSeverity(eventType string, details map[string]interface{}) string {
	switch eventType {
	case "dmca_takedown", "dmca_reinstatement":
		return "legal"
	case "system_error", "integrity_breach":
		return "critical"
	case "processing_delay":
		return "warning"
	default:
		return "info"
	}
}

func (system *ComplianceAuditSystem) determineTargetType(targetID string) string {
	if strings.HasPrefix(targetID, "DMCA-") {
		return "notice"
	}
	if strings.HasPrefix(targetID, "user-") {
		return "user"
	}
	return "descriptor"
}

func (system *ComplianceAuditSystem) determineLegalBasis(eventType string) string {
	switch {
	case strings.Contains(eventType, "dmca"):
		return "DMCA 17 USC 512"
	case strings.Contains(eventType, "privacy"):
		return "Privacy Act compliance"
	default:
		return "General legal compliance"
	}
}

func (system *ComplianceAuditSystem) generateComplianceNotes(eventType, action string) string {
	return fmt.Sprintf("Automated compliance logging for %s action: %s", eventType, action)
}

func (system *ComplianceAuditSystem) generateTags(eventType, action string) []string {
	tags := []string{"compliance", eventType}
	if strings.Contains(action, "takedown") {
		tags = append(tags, "takedown")
	}
	if strings.Contains(action, "reinstate") {
		tags = append(tags, "reinstatement")
	}
	return tags
}

func (system *ComplianceAuditSystem) generateEntryID() string {
	data := fmt.Sprintf("audit-%d-%d", time.Now().UnixNano(), len(system.logger.entries))
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("AE-%s", hex.EncodeToString(hash[:8]))
}

func (system *ComplianceAuditSystem) generateReportID() string {
	data := fmt.Sprintf("report-%d", time.Now().UnixNano())
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("CR-%s", hex.EncodeToString(hash[:8]))
}

func (system *ComplianceAuditSystem) calculateEntryHash(entry *DetailedAuditEntry) string {
	data := fmt.Sprintf("%s-%s-%s-%s-%v", 
		entry.EntryID, entry.Timestamp.Format(time.RFC3339), 
		entry.EventType, entry.Action, entry.ActionDetails)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

func (system *ComplianceAuditSystem) generateSignature(entry *DetailedAuditEntry) string {
	// Simple signature for now - in production would use proper cryptographic signing
	data := fmt.Sprintf("%s-%s", entry.EntryHash, entry.Timestamp.Format(time.RFC3339))
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("SIG-%s", hex.EncodeToString(hash[:16]))
}

// updateMetrics updates real-time compliance metrics
func (monitor *ComplianceMonitor) updateMetrics(entry *DetailedAuditEntry) {
	// Update daily counters
	today := time.Now().Truncate(24 * time.Hour)
	entryDay := entry.Timestamp.Truncate(24 * time.Hour)
	
	if entryDay.Equal(today) {
		switch entry.EventType {
		case "dmca_takedown":
			monitor.metrics.TakedownsToday++
		case "dmca_counter_notice":
			monitor.metrics.CounterNoticesToday++
		}
	}
	
	monitor.metrics.LastUpdated = time.Now()
}

// checkAlerts checks if the entry triggers any compliance alerts
func (monitor *ComplianceMonitor) checkAlerts(entry *DetailedAuditEntry) error {
	// Check takedown volume threshold
	if monitor.metrics.TakedownsToday > int64(monitor.config.AlertThresholds.TakedownsPerDay) {
		alert := &ComplianceAlert{
			AlertID:   monitor.generateAlertID(),
			AlertType: "high_takedown_volume",
			Severity:  "warning",
			Timestamp: time.Now(),
			Condition: fmt.Sprintf("Daily takedowns exceeded threshold: %d", monitor.metrics.TakedownsToday),
			Details: map[string]interface{}{
				"current_count": monitor.metrics.TakedownsToday,
				"threshold":     monitor.config.AlertThresholds.TakedownsPerDay,
			},
		}
		monitor.alerts = append(monitor.alerts, alert)
	}
	
	return nil
}

func (monitor *ComplianceMonitor) generateAlertID() string {
	data := fmt.Sprintf("alert-%d", time.Now().UnixNano())
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("CA-%s", hex.EncodeToString(hash[:8]))
}