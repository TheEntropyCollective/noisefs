package compliance

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// UserNotificationManager handles all user notifications for compliance events
type UserNotificationManager struct {
	database        *ComplianceDatabase
	auditSystem     *ComplianceAuditSystem
	legalFramework  *LegalFramework
	config          *NotificationConfig
	notificationDB  *NotificationDatabase
}

// NotificationConfig defines configuration for user notifications
type NotificationConfig struct {
	EnableEmailNotifications    bool          `json:"enable_email_notifications"`
	EnableInAppNotifications   bool          `json:"enable_in_app_notifications"`
	EnableSMSNotifications     bool          `json:"enable_sms_notifications"`
	NotificationLanguages      []string      `json:"notification_languages"`
	RetentionPeriod           time.Duration `json:"retention_period"`
	EscalationTimelines       map[string]time.Duration `json:"escalation_timelines"`
	TemplateCustomization     bool          `json:"template_customization"`
	LegalNoticeRequirements   *LegalNoticeRequirements `json:"legal_notice_requirements"`
}

// LegalNoticeRequirements defines requirements for legal notices
type LegalNoticeRequirements struct {
	MinimumNoticeTime         time.Duration `json:"minimum_notice_time"`
	RequiredInformation       []string      `json:"required_information"`
	DeliveryConfirmation      bool          `json:"delivery_confirmation"`
	MultipleDeliveryMethods   bool          `json:"multiple_delivery_methods"`
	TranslationRequirements   []string      `json:"translation_requirements"`
}

// NotificationDatabase stores and manages all user notifications
type NotificationDatabase struct {
	notifications    map[string]*UserNotification
	userSubscriptions map[string]*NotificationPreferences
	deliveryLog      []*DeliveryRecord
	templates        map[string]*NotificationTemplate
}

// UserNotification represents a notification to be sent to a user
type UserNotification struct {
	NotificationID      string                 `json:"notification_id"`
	UserID              string                 `json:"user_id"`
	NotificationType    string                 `json:"notification_type"`
	Priority            string                 `json:"priority"` // "low", "medium", "high", "urgent", "legal"
	Subject             string                 `json:"subject"`
	Content             string                 `json:"content"`
	LegalContent        *LegalNotificationContent `json:"legal_content,omitempty"`
	
	// Timing and delivery
	CreatedAt           time.Time              `json:"created_at"`
	ScheduledAt         *time.Time             `json:"scheduled_at,omitempty"`
	ExpiresAt           *time.Time             `json:"expires_at,omitempty"`
	DeliveryAttempts    []*DeliveryAttempt     `json:"delivery_attempts"`
	DeliveryStatus      string                 `json:"delivery_status"` // "pending", "sent", "delivered", "failed", "expired"
	
	// User interaction
	ReadAt              *time.Time             `json:"read_at,omitempty"`
	AcknowledgedAt      *time.Time             `json:"acknowledged_at,omitempty"`
	ResponseRequired    bool                   `json:"response_required"`
	ResponseDeadline    *time.Time             `json:"response_deadline,omitempty"`
	UserResponse        string                 `json:"user_response,omitempty"`
	
	// Compliance and audit
	LegalRequirement    bool                   `json:"legal_requirement"`
	ComplianceCategory  string                 `json:"compliance_category"`
	RelatedCaseID       string                 `json:"related_case_id,omitempty"`
	AuditTrail          []*NotificationAuditEntry `json:"audit_trail"`
	
	// Content and formatting
	TemplateID          string                 `json:"template_id,omitempty"`
	Language            string                 `json:"language"`
	Formatting          string                 `json:"formatting"` // "plain", "html", "markdown"
	Attachments         []*NotificationAttachment `json:"attachments,omitempty"`
	
	// Metadata
	Metadata            map[string]interface{} `json:"metadata"`
	Tags                []string               `json:"tags"`
}

// LegalNotificationContent provides legal-specific notification content
type LegalNotificationContent struct {
	LegalBasis          string                 `json:"legal_basis"`
	StatutoryRequirements []string             `json:"statutory_requirements"`
	UserRights          []string               `json:"user_rights"`
	ResponseOptions     []*ResponseOption      `json:"response_options"`
	LegalDeadlines      map[string]time.Time   `json:"legal_deadlines"`
	ContactInformation  *LegalContactInfo      `json:"contact_information"`
	AttachedDocuments   []*LegalDocument       `json:"attached_documents"`
}

// ResponseOption defines available response options for users
type ResponseOption struct {
	OptionID            string    `json:"option_id"`
	OptionName          string    `json:"option_name"`
	Description         string    `json:"description"`
	LegalImplications   string    `json:"legal_implications"`
	RequiredInformation []string  `json:"required_information"`
	Deadline            *time.Time `json:"deadline,omitempty"`
}

// NotificationPreferences stores user notification preferences
type NotificationPreferences struct {
	UserID              string                 `json:"user_id"`
	EmailAddress        string                 `json:"email_address"`
	PhoneNumber         string                 `json:"phone_number,omitempty"`
	PreferredLanguage   string                 `json:"preferred_language"`
	PreferredMethods    []string               `json:"preferred_methods"`
	OptOutCategories    []string               `json:"opt_out_categories"`
	LegalNoticeMethod   string                 `json:"legal_notice_method"`
	BackupContacts      []*BackupContact       `json:"backup_contacts,omitempty"`
	DeliveryTimes       *DeliveryTimePreferences `json:"delivery_times,omitempty"`
	UpdatedAt           time.Time              `json:"updated_at"`
}

// DeliveryAttempt records an attempt to deliver a notification
type DeliveryAttempt struct {
	AttemptID           string    `json:"attempt_id"`
	AttemptNumber       int       `json:"attempt_number"`
	Method              string    `json:"method"` // "email", "sms", "in_app", "postal"
	AttemptedAt         time.Time `json:"attempted_at"`
	Status              string    `json:"status"` // "sent", "delivered", "failed", "bounced"
	ErrorMessage        string    `json:"error_message,omitempty"`
	DeliveryConfirmation string   `json:"delivery_confirmation,omitempty"`
	RetryScheduled      *time.Time `json:"retry_scheduled,omitempty"`
}

// NotificationTemplate defines reusable notification templates
type NotificationTemplate struct {
	TemplateID          string                 `json:"template_id"`
	TemplateName        string                 `json:"template_name"`
	NotificationType    string                 `json:"notification_type"`
	Language            string                 `json:"language"`
	Subject             string                 `json:"subject"`
	ContentTemplate     string                 `json:"content_template"`
	LegalTemplate       *LegalNotificationTemplate `json:"legal_template,omitempty"`
	RequiredVariables   []string               `json:"required_variables"`
	OptionalVariables   []string               `json:"optional_variables"`
	ComplianceNotes     string                 `json:"compliance_notes"`
	CreatedAt           time.Time              `json:"created_at"`
	UpdatedAt           time.Time              `json:"updated_at"`
}

// LegalNotificationTemplate provides legal-specific template content
type LegalNotificationTemplate struct {
	StatutoryLanguage   string   `json:"statutory_language"`
	RequiredDisclosures []string `json:"required_disclosures"`
	UserRightsNotice    string   `json:"user_rights_notice"`
	ContactRequirements string   `json:"contact_requirements"`
}

// NewUserNotificationManager creates a new user notification manager
func NewUserNotificationManager(database *ComplianceDatabase, auditSystem *ComplianceAuditSystem, framework *LegalFramework) *UserNotificationManager {
	manager := &UserNotificationManager{
		database:       database,
		auditSystem:    auditSystem,
		legalFramework: framework,
		config:         DefaultNotificationConfig(),
		notificationDB: &NotificationDatabase{
			notifications:     make(map[string]*UserNotification),
			userSubscriptions: make(map[string]*NotificationPreferences),
			deliveryLog:       make([]*DeliveryRecord, 0),
			templates:         make(map[string]*NotificationTemplate),
		},
	}
	
	// Initialize default templates
	manager.initializeDefaultTemplates()
	
	return manager
}

// DefaultNotificationConfig returns default notification configuration
func DefaultNotificationConfig() *NotificationConfig {
	return &NotificationConfig{
		EnableEmailNotifications:  true,
		EnableInAppNotifications: true,
		EnableSMSNotifications:   false,
		NotificationLanguages:    []string{"en-US", "es-ES", "fr-FR"},
		RetentionPeriod:          7 * 365 * 24 * time.Hour, // 7 years for legal compliance
		EscalationTimelines: map[string]time.Duration{
			"legal_notice":      4 * time.Hour,
			"takedown_notice":   24 * time.Hour,
			"account_warning":   72 * time.Hour,
			"general_notice":    7 * 24 * time.Hour,
		},
		TemplateCustomization: true,
		LegalNoticeRequirements: &LegalNoticeRequirements{
			MinimumNoticeTime:       24 * time.Hour,
			RequiredInformation:     []string{"legal_basis", "user_rights", "response_options", "contact_info"},
			DeliveryConfirmation:    true,
			MultipleDeliveryMethods: true,
			TranslationRequirements: []string{"en-US"},
		},
	}
}

// NotifyTakedownAction sends notification about a takedown action
func (manager *UserNotificationManager) NotifyTakedownAction(userID, descriptorCID string, takedownRecord *TakedownRecord) error {
	notification := &UserNotification{
		NotificationID:     manager.generateNotificationID(),
		UserID:             userID,
		NotificationType:   "dmca_takedown",
		Priority:           "legal",
		Subject:            "DMCA Takedown Notice - Content Removed",
		CreatedAt:          time.Now(),
		ResponseRequired:   true,
		LegalRequirement:   true,
		ComplianceCategory: "dmca_compliance",
		RelatedCaseID:      takedownRecord.TakedownID,
		Language:           manager.getUserPreferredLanguage(userID),
		Tags:               []string{"dmca", "takedown", "legal"},
	}
	
	// Set response deadline (14 days for counter-notice)
	deadline := time.Now().Add(14 * 24 * time.Hour)
	notification.ResponseDeadline = &deadline
	
	// Generate content using template
	content, err := manager.generateTakedownNotificationContent(descriptorCID, takedownRecord)
	if err != nil {
		return fmt.Errorf("failed to generate notification content: %w", err)
	}
	notification.Content = content
	
	// Add legal content
	notification.LegalContent = &LegalNotificationContent{
		LegalBasis: "DMCA 17 USC 512(c) takedown notice",
		StatutoryRequirements: []string{
			"Notice provided under DMCA safe harbor provisions",
			"Content access has been disabled",
			"User may submit counter-notification",
		},
		UserRights: []string{
			"Right to submit DMCA counter-notification",
			"Right to contest takedown if belief of mistake or misidentification",
			"Right to legal representation",
			"Right to file court action",
		},
		ResponseOptions: []*ResponseOption{
			{
				OptionID:    "counter_notice",
				OptionName:  "Submit Counter-Notification",
				Description: "Challenge the takedown if you believe it was made in error",
				LegalImplications: "Filing counter-notice may result in content restoration after 14-day waiting period",
				RequiredInformation: []string{"sworn_statement", "good_faith_belief", "contact_info", "jurisdiction_consent"},
				Deadline:    &deadline,
			},
			{
				OptionID:    "accept_takedown",
				OptionName:  "Accept Takedown",
				Description: "Accept the takedown without challenging",
				LegalImplications: "Content will remain inaccessible",
				RequiredInformation: []string{},
			},
			{
				OptionID:    "seek_legal_counsel",
				OptionName:  "Consult Legal Counsel",
				Description: "Seek legal advice about your options",
				LegalImplications: "Attorney may provide guidance on best course of action",
				RequiredInformation: []string{},
			},
		},
		LegalDeadlines: map[string]time.Time{
			"counter_notice_deadline": deadline,
		},
		ContactInformation: &LegalContactInfo{
			DMCAAgent:      "dmca@noisefs.org",
			LegalDepartment: "legal@noisefs.org",
			Phone:         "+1-555-0100", // Configure with actual DMCA agent phone
			Address:       "NoiseFS Legal Department\nDigital Service Provider\nUnited States",
		},
	}
	
	// Store notification
	manager.notificationDB.notifications[notification.NotificationID] = notification
	
	// Attempt delivery
	if err := manager.deliverNotification(notification); err != nil {
		return fmt.Errorf("failed to deliver notification: %w", err)
	}
	
	// Log the notification
	manager.auditSystem.LogComplianceEvent("user_notification_sent", userID, descriptorCID, "takedown_notification", map[string]interface{}{
		"notification_id": notification.NotificationID,
		"takedown_id":     takedownRecord.TakedownID,
		"notification_type": "dmca_takedown",
	})
	
	return nil
}

// NotifyCounterNoticeReceived sends notification about counter-notice receipt
func (manager *UserNotificationManager) NotifyCounterNoticeReceived(userID, descriptorCID string, counterNotice *CounterNotice) error {
	notification := &UserNotification{
		NotificationID:     manager.generateNotificationID(),
		UserID:             userID,
		NotificationType:   "counter_notice_received",
		Priority:           "high",
		Subject:            "Counter-Notice Received - Processing Started",
		CreatedAt:          time.Now(),
		LegalRequirement:   true,
		ComplianceCategory: "dmca_compliance",
		RelatedCaseID:      counterNotice.CounterNoticeID,
		Language:           manager.getUserPreferredLanguage(userID),
		Tags:               []string{"dmca", "counter_notice", "legal"},
	}
	
	// Generate content
	content := manager.generateCounterNoticeReceivedContent(descriptorCID, counterNotice)
	notification.Content = content
	
	// Add legal content
	reinstatementDate := time.Now().Add(14 * 24 * time.Hour)
	notification.LegalContent = &LegalNotificationContent{
		LegalBasis: "DMCA 17 USC 512(g) counter-notification procedures",
		StatutoryRequirements: []string{
			"Counter-notification has been received and validated",
			"Original requestor will be notified",
			"14-day waiting period has begun",
		},
		UserRights: []string{
			"Right to content restoration after waiting period",
			"Protection from repeat removal absent court order",
			"Right to legal representation during process",
		},
		LegalDeadlines: map[string]time.Time{
			"expected_reinstatement": reinstatementDate,
		},
		ContactInformation: &LegalContactInfo{
			DMCAAgent:      "dmca@noisefs.org",
			LegalDepartment: "legal@noisefs.org",
			Phone:         "+1-555-0100", // Configure with actual DMCA agent phone
		},
	}
	
	// Store and deliver notification
	manager.notificationDB.notifications[notification.NotificationID] = notification
	if err := manager.deliverNotification(notification); err != nil {
		return fmt.Errorf("failed to deliver notification: %w", err)
	}
	
	// Log the notification
	manager.auditSystem.LogComplianceEvent("user_notification_sent", userID, descriptorCID, "counter_notice_received", map[string]interface{}{
		"notification_id":    notification.NotificationID,
		"counter_notice_id":  counterNotice.CounterNoticeID,
		"expected_reinstatement": reinstatementDate,
	})
	
	return nil
}

// NotifyReinstatement sends notification about content reinstatement
func (manager *UserNotificationManager) NotifyReinstatement(userID, descriptorCID string, takedownRecord *TakedownRecord) error {
	notification := &UserNotification{
		NotificationID:     manager.generateNotificationID(),
		UserID:             userID,
		NotificationType:   "content_reinstated",
		Priority:           "high",
		Subject:            "Content Reinstated - Access Restored",
		CreatedAt:          time.Now(),
		LegalRequirement:   true,
		ComplianceCategory: "dmca_compliance",
		RelatedCaseID:      takedownRecord.TakedownID,
		Language:           manager.getUserPreferredLanguage(userID),
		Tags:               []string{"dmca", "reinstatement", "restored"},
	}
	
	// Generate content
	content := manager.generateReinstatementContent(descriptorCID, takedownRecord)
	notification.Content = content
	
	// Add legal content
	notification.LegalContent = &LegalNotificationContent{
		LegalBasis: "DMCA 17 USC 512(g) counter-notification procedures",
		StatutoryRequirements: []string{
			"14-day waiting period has elapsed without court order",
			"Content access has been restored",
			"DMCA counter-notification process completed",
		},
		UserRights: []string{
			"Right to continued access absent valid court order",
			"Protection from repeat takedown for same content",
			"Right to file complaint if future false claims occur",
		},
		ContactInformation: &LegalContactInfo{
			DMCAAgent:      "dmca@noisefs.org",
			LegalDepartment: "legal@noisefs.org",
			Phone:         "+1-555-0100", // Configure with actual DMCA agent phone
		},
	}
	
	// Store and deliver notification
	manager.notificationDB.notifications[notification.NotificationID] = notification
	if err := manager.deliverNotification(notification); err != nil {
		return fmt.Errorf("failed to deliver notification: %w", err)
	}
	
	// Log the notification
	manager.auditSystem.LogComplianceEvent("user_notification_sent", userID, descriptorCID, "content_reinstated", map[string]interface{}{
		"notification_id": notification.NotificationID,
		"takedown_id":     takedownRecord.TakedownID,
	})
	
	return nil
}

// NotifyAccountWarning sends notification about account warning
func (manager *UserNotificationManager) NotifyAccountWarning(userID string, violationType string, violationCount int) error {
	notification := &UserNotification{
		NotificationID:     manager.generateNotificationID(),
		UserID:             userID,
		NotificationType:   "account_warning",
		Priority:           "high",
		Subject:            fmt.Sprintf("Account Warning - Violation #%d", violationCount),
		CreatedAt:          time.Now(),
		ResponseRequired:   true,
		LegalRequirement:   true,
		ComplianceCategory: "repeat_infringer_policy",
		Language:           manager.getUserPreferredLanguage(userID),
		Tags:               []string{"warning", "repeat_infringer", "compliance"},
	}
	
	// Set response deadline
	deadline := time.Now().Add(7 * 24 * time.Hour)
	notification.ResponseDeadline = &deadline
	
	// Generate content based on violation count
	content := manager.generateAccountWarningContent(violationType, violationCount)
	notification.Content = content
	
	// Add legal content
	notification.LegalContent = &LegalNotificationContent{
		LegalBasis: "NoiseFS Terms of Service and DMCA repeat infringer policy",
		StatutoryRequirements: []string{
			"Notice of policy violation",
			"Warning of potential account consequences",
			"Educational materials provided",
		},
		UserRights: []string{
			"Right to appeal violation determination",
			"Right to review account activity",
			"Right to educational resources",
			"Right to legal representation",
		},
		ResponseOptions: []*ResponseOption{
			{
				OptionID:    "acknowledge_warning",
				OptionName:  "Acknowledge Warning",
				Description: "Acknowledge receipt and commit to compliance",
				RequiredInformation: []string{"acknowledgment_statement"},
				Deadline:    &deadline,
			},
			{
				OptionID:    "appeal_violation",
				OptionName:  "Appeal Violation",
				Description: "Contest the violation determination",
				LegalImplications: "Appeal will be reviewed by compliance team",
				RequiredInformation: []string{"appeal_basis", "supporting_evidence"},
				Deadline:    &deadline,
			},
		},
		LegalDeadlines: map[string]time.Time{
			"response_deadline": deadline,
		},
		ContactInformation: &LegalContactInfo{
			LegalDepartment: "legal@noisefs.org",
			UserSupport:     "support@noisefs.org",
			Phone:          "+1-555-0100", // Configure with actual DMCA agent phone
		},
	}
	
	// Store and deliver notification
	manager.notificationDB.notifications[notification.NotificationID] = notification
	if err := manager.deliverNotification(notification); err != nil {
		return fmt.Errorf("failed to deliver notification: %w", err)
	}
	
	// Log the notification
	manager.auditSystem.LogComplianceEvent("user_notification_sent", userID, "", "account_warning", map[string]interface{}{
		"notification_id":  notification.NotificationID,
		"violation_type":   violationType,
		"violation_count":  violationCount,
	})
	
	return nil
}

// NotifySystemUpdate sends notification about system updates affecting compliance
func (manager *UserNotificationManager) NotifySystemUpdate(userID string, updateType string, updateDetails map[string]interface{}) error {
	notification := &UserNotification{
		NotificationID:     manager.generateNotificationID(),
		UserID:             userID,
		NotificationType:   "system_update",
		Priority:           "medium",
		Subject:            "NoiseFS System Update - Important Changes",
		CreatedAt:          time.Now(),
		ComplianceCategory: "system_notification",
		Language:           manager.getUserPreferredLanguage(userID),
		Tags:               []string{"system", "update", "compliance"},
		Metadata:           updateDetails,
	}
	
	// Generate content based on update type
	content := manager.generateSystemUpdateContent(updateType, updateDetails)
	notification.Content = content
	
	// Store and deliver notification
	manager.notificationDB.notifications[notification.NotificationID] = notification
	if err := manager.deliverNotification(notification); err != nil {
		return fmt.Errorf("failed to deliver notification: %w", err)
	}
	
	return nil
}

// Content generation methods

func (manager *UserNotificationManager) generateTakedownNotificationContent(descriptorCID string, takedownRecord *TakedownRecord) (string, error) {
	template := `
DMCA TAKEDOWN NOTICE

Dear NoiseFS User,

We have received a DMCA takedown notice regarding content associated with your account. In compliance with the Digital Millennium Copyright Act, we have disabled access to the following content:

AFFECTED CONTENT:
- Descriptor: %s
- File: %s
- Takedown Date: %s
- Takedown ID: %s

DMCA NOTICE DETAILS:
- Requestor: %s
- Copyright Work: %s
- Legal Basis: %s

YOUR RIGHTS AND OPTIONS:

1. COUNTER-NOTIFICATION:
If you believe this takedown was made in error or misidentification, you may submit a DMCA counter-notification. This requires:
- A sworn statement that you have a good faith belief the content was disabled due to mistake or misidentification
- Your contact information and consent to federal court jurisdiction
- Your physical or electronic signature

2. SEEK LEGAL COUNSEL:
You may wish to consult with an attorney about your rights and options.

3. ACCEPT THE TAKEDOWN:
You may choose to accept the takedown without challenge.

IMPORTANT LEGAL INFORMATION:
- You have 14 days to submit a counter-notification
- Filing a false counter-notification may result in liability for damages
- We will forward your counter-notification to the original requestor
- Content may be restored after 14 days if no court order is received

NOISEFS TECHNICAL PROTECTIONS:
Please note that NoiseFS's technical architecture provides strong privacy protections:
- Individual blocks are anonymized with public domain content
- Blocks serve multiple files and cannot be individually copyrighted
- Our system complies with DMCA requirements while preserving user privacy

For questions or to submit a counter-notification, contact: dmca@noisefs.org

Sincerely,
NoiseFS Legal Compliance Team

Generated: %s
Reference: %s
`
	
	return fmt.Sprintf(template,
		descriptorCID[:8],
		takedownRecord.FilePath,
		takedownRecord.TakedownDate.Format("January 2, 2006"),
		takedownRecord.TakedownID,
		takedownRecord.RequestorName,
		takedownRecord.CopyrightWork,
		takedownRecord.LegalBasis,
		time.Now().Format("January 2, 2006 15:04:05 UTC"),
		takedownRecord.TakedownID,
	), nil
}

func (manager *UserNotificationManager) generateCounterNoticeReceivedContent(descriptorCID string, counterNotice *CounterNotice) string {
	return fmt.Sprintf(`
COUNTER-NOTICE RECEIVED

Dear %s,

We have received and validated your DMCA counter-notification for descriptor %s. Here's what happens next:

COUNTER-NOTICE STATUS:
- Counter-Notice ID: %s
- Received: %s
- Status: Validated and Processing

NEXT STEPS:
1. We have forwarded your counter-notification to the original copyright claimant
2. The claimant has 14 business days to file a court action
3. If no court order is received within 14 days, we will restore access to your content
4. Expected restoration date: %s

YOUR PROTECTIONS:
- Your content cannot be removed again for the same claim without a court order
- You have protection under DMCA counter-notification procedures
- You may file a complaint if false claims continue

We will notify you immediately when the waiting period expires and access is restored.

For questions, contact: dmca@noisefs.org

NoiseFS Legal Compliance Team
Generated: %s
`,
		counterNotice.UserName,
		descriptorCID[:8],
		counterNotice.CounterNoticeID,
		counterNotice.SubmissionDate.Format("January 2, 2006"),
		time.Now().Add(14*24*time.Hour).Format("January 2, 2006"),
		time.Now().Format("January 2, 2006 15:04:05 UTC"),
	)
}

func (manager *UserNotificationManager) generateReinstatementContent(descriptorCID string, takedownRecord *TakedownRecord) string {
	return fmt.Sprintf(`
CONTENT REINSTATED

Dear User,

Good news! Access to your content has been restored following the DMCA counter-notification process.

RESTORED CONTENT:
- Descriptor: %s
- Original Takedown: %s
- Takedown ID: %s
- Restored: %s

WHAT HAPPENED:
1. You submitted a valid DMCA counter-notification
2. The original claimant was notified
3. No court order was received within the required 14-day period
4. Access has now been restored per DMCA 512(g) procedures

YOUR PROTECTIONS:
- This content cannot be removed again for the same claim without a court order
- You have protection against repeat false claims
- NoiseFS will reject duplicate takedown requests for this content

If you experience any issues accessing your content or have questions about this process, please contact us.

For questions, contact: dmca@noisefs.org

NoiseFS Legal Compliance Team
Generated: %s
`,
		descriptorCID[:8],
		takedownRecord.TakedownDate.Format("January 2, 2006"),
		takedownRecord.TakedownID,
		time.Now().Format("January 2, 2006"),
		time.Now().Format("January 2, 2006 15:04:05 UTC"),
	)
}

func (manager *UserNotificationManager) generateAccountWarningContent(violationType string, violationCount int) string {
	escalationLevel := "Warning"
	consequences := "continued monitoring of your account"
	
	if violationCount >= 3 {
		escalationLevel = "Final Warning"
		consequences = "permanent account termination"
	} else if violationCount >= 2 {
		escalationLevel = "Second Warning"
		consequences = "temporary account restrictions or suspension"
	}
	
	return fmt.Sprintf(`
ACCOUNT %s - REPEAT INFRINGER POLICY

Dear NoiseFS User,

This is your %s for violations of our Terms of Service and copyright policy.

VIOLATION DETAILS:
- Violation Type: %s
- Violation Count: %d
- Warning Level: %s

REPEAT INFRINGER POLICY:
NoiseFS maintains a "three strikes" policy for copyright violations:
- Strike 1: Warning and educational materials
- Strike 2: Temporary restrictions
- Strike 3: Account termination

CURRENT STATUS:
You are currently at violation #%d. Further violations may result in %s.

YOUR OPTIONS:
1. ACKNOWLEDGE: Acknowledge this warning and commit to compliance
2. APPEAL: Contest this violation if you believe it was made in error
3. EDUCATION: Review our copyright compliance resources

EDUCATIONAL RESOURCES:
- NoiseFS Terms of Service: https://noisefs.org/terms
- Copyright Compliance Guide: https://noisefs.org/copyright
- Fair Use Guidelines: https://noisefs.org/fairuse
- Legal Resources: https://noisefs.org/legal

TECHNICAL PROTECTIONS:
Remember that NoiseFS provides strong technical protections:
- Upload only content you own or have permission to share
- NoiseFS's block anonymization provides privacy protection
- System compliance helps protect all users

Please respond within 7 days to acknowledge this notice or submit an appeal.

For questions, contact: legal@noisefs.org

NoiseFS Compliance Team
Generated: %s
`,
		escalationLevel,
		escalationLevel,
		violationType,
		violationCount,
		escalationLevel,
		violationCount,
		consequences,
		time.Now().Format("January 2, 2006 15:04:05 UTC"),
	)
}

func (manager *UserNotificationManager) generateSystemUpdateContent(updateType string, updateDetails map[string]interface{}) string {
	return fmt.Sprintf(`
NOISEFS SYSTEM UPDATE

Dear NoiseFS User,

We are writing to inform you of important updates to the NoiseFS system that may affect your use of the service.

UPDATE DETAILS:
- Update Type: %s
- Effective Date: %s
- Impact Level: %v

WHAT'S CHANGING:
%v

WHAT YOU NEED TO DO:
%v

TECHNICAL IMPROVEMENTS:
This update continues our commitment to providing strong privacy protection while maintaining legal compliance. NoiseFS's block anonymization and reuse enforcement remain fully operational.

If you have questions about these changes, please contact our support team.

For questions, contact: support@noisefs.org

NoiseFS Team
Generated: %s
`,
		updateType,
		time.Now().Format("January 2, 2006"),
		updateDetails["impact_level"],
		updateDetails["changes"],
		updateDetails["user_actions"],
		time.Now().Format("January 2, 2006 15:04:05 UTC"),
	)
}

// Delivery and utility methods

func (manager *UserNotificationManager) deliverNotification(notification *UserNotification) error {
	userPrefs := manager.getUserNotificationPreferences(notification.UserID)
	
	// Determine delivery methods based on priority and user preferences
	methods := manager.determineDeliveryMethods(notification, userPrefs)
	
	for _, method := range methods {
		attempt := &DeliveryAttempt{
			AttemptID:     manager.generateAttemptID(),
			AttemptNumber: len(notification.DeliveryAttempts) + 1,
			Method:        method,
			AttemptedAt:   time.Now(),
		}
		
		// Simulate delivery (in real implementation, would use actual email/SMS services)
		err := manager.simulateDelivery(notification, method)
		if err != nil {
			attempt.Status = "failed"
			attempt.ErrorMessage = err.Error()
			
			// Schedule retry for failed deliveries
			retryTime := time.Now().Add(1 * time.Hour)
			attempt.RetryScheduled = &retryTime
		} else {
			attempt.Status = "sent"
			attempt.DeliveryConfirmation = manager.generateDeliveryConfirmation()
		}
		
		notification.DeliveryAttempts = append(notification.DeliveryAttempts, attempt)
	}
	
	// Update overall delivery status
	manager.updateDeliveryStatus(notification)
	
	return nil
}

func (manager *UserNotificationManager) simulateDelivery(notification *UserNotification, method string) error {
	// Simulate delivery - in real implementation would integrate with email/SMS services
	fmt.Printf("NOTIFICATION DELIVERY [%s]: %s sent to user %s via %s\n", 
		notification.Priority, notification.NotificationType, notification.UserID[:8], method)
	return nil
}

// Helper methods

func (manager *UserNotificationManager) generateNotificationID() string {
	data := fmt.Sprintf("notification-%d", time.Now().UnixNano())
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("NOTIF-%s", hex.EncodeToString(hash[:8]))
}

func (manager *UserNotificationManager) generateAttemptID() string {
	data := fmt.Sprintf("attempt-%d", time.Now().UnixNano())
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("ATT-%s", hex.EncodeToString(hash[:8]))
}

func (manager *UserNotificationManager) generateDeliveryConfirmation() string {
	data := fmt.Sprintf("delivery-%d", time.Now().UnixNano())
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("CONF-%s", hex.EncodeToString(hash[:8]))
}

func (manager *UserNotificationManager) getUserPreferredLanguage(userID string) string {
	prefs := manager.getUserNotificationPreferences(userID)
	if prefs != nil && prefs.PreferredLanguage != "" {
		return prefs.PreferredLanguage
	}
	return "en-US" // Default
}

func (manager *UserNotificationManager) getUserNotificationPreferences(userID string) *NotificationPreferences {
	prefs, exists := manager.notificationDB.userSubscriptions[userID]
	if !exists {
		// Return default preferences
		return &NotificationPreferences{
			UserID:            userID,
			PreferredLanguage: "en-US",
			PreferredMethods:  []string{"email"},
			LegalNoticeMethod: "email",
			UpdatedAt:         time.Now(),
		}
	}
	return prefs
}

func (manager *UserNotificationManager) determineDeliveryMethods(notification *UserNotification, prefs *NotificationPreferences) []string {
	// For legal notifications, always use multiple methods
	if notification.LegalRequirement {
		methods := []string{"email"}
		if prefs.PhoneNumber != "" && contains(prefs.PreferredMethods, "sms") {
			methods = append(methods, "sms")
		}
		methods = append(methods, "in_app")
		return methods
	}
	
	// For non-legal notifications, use user preferences
	if len(prefs.PreferredMethods) > 0 {
		return prefs.PreferredMethods
	}
	
	return []string{"email"} // Default
}

func (manager *UserNotificationManager) updateDeliveryStatus(notification *UserNotification) {
	hasSuccessful := false
	allFailed := true
	
	for _, attempt := range notification.DeliveryAttempts {
		if attempt.Status == "sent" || attempt.Status == "delivered" {
			hasSuccessful = true
			allFailed = false
		}
		if attempt.Status != "failed" {
			allFailed = false
		}
	}
	
	if hasSuccessful {
		notification.DeliveryStatus = "sent"
	} else if allFailed {
		notification.DeliveryStatus = "failed"
	} else {
		notification.DeliveryStatus = "pending"
	}
}

func (manager *UserNotificationManager) initializeDefaultTemplates() {
	// Initialize default notification templates
	templates := []*NotificationTemplate{
		{
			TemplateID:       "dmca_takedown_en",
			TemplateName:     "DMCA Takedown Notice",
			NotificationType: "dmca_takedown",
			Language:         "en-US",
			Subject:          "DMCA Takedown Notice - Content Removed",
			ContentTemplate:  "DMCA takedown template content...",
			RequiredVariables: []string{"descriptor_cid", "takedown_id", "requestor_name"},
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		},
		// Add more templates as needed
	}
	
	for _, template := range templates {
		manager.notificationDB.templates[template.TemplateID] = template
	}
}

// Utility functions

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// Additional type definitions

type DeliveryRecord struct {
	RecordID        string    `json:"record_id"`
	NotificationID  string    `json:"notification_id"`
	UserID          string    `json:"user_id"`
	DeliveryMethod  string    `json:"delivery_method"`
	DeliveryStatus  string    `json:"delivery_status"`
	DeliveryTime    time.Time `json:"delivery_time"`
	DeliveryDetails map[string]interface{} `json:"delivery_details"`
}

type NotificationAuditEntry struct {
	EntryID     string                 `json:"entry_id"`
	Timestamp   time.Time              `json:"timestamp"`
	Action      string                 `json:"action"`
	Actor       string                 `json:"actor"`
	Details     map[string]interface{} `json:"details"`
}

type NotificationAttachment struct {
	AttachmentID   string `json:"attachment_id"`
	Filename       string `json:"filename"`
	ContentType    string `json:"content_type"`
	Size           int64  `json:"size"`
	Description    string `json:"description"`
	AttachmentData []byte `json:"attachment_data,omitempty"`
}

type BackupContact struct {
	ContactID    string `json:"contact_id"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	Phone        string `json:"phone,omitempty"`
	Relationship string `json:"relationship"`
	Priority     int    `json:"priority"`
}

type DeliveryTimePreferences struct {
	TimeZone        string             `json:"time_zone"`
	QuietHours      *QuietHoursConfig  `json:"quiet_hours,omitempty"`
	PreferredTimes  []string           `json:"preferred_times"`
	EmergencyOverride bool             `json:"emergency_override"`
}

type QuietHoursConfig struct {
	StartTime string `json:"start_time"` // "22:00"
	EndTime   string `json:"end_time"`   // "08:00"
	Days      []string `json:"days"`     // ["monday", "tuesday", ...]
}

type LegalContactInfo struct {
	DMCAAgent       string `json:"dmca_agent"`
	LegalDepartment string `json:"legal_department"`
	UserSupport     string `json:"user_support"`
	Phone          string `json:"phone"`
	Address        string `json:"address"`
}

type LegalDocument struct {
	DocumentID   string `json:"document_id"`
	DocumentType string `json:"document_type"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	URL          string `json:"url"`
	Language     string `json:"language"`
}