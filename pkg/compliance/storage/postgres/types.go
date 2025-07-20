package postgres

import (
	"context"
	"time"
)

// Transaction interface defines database transaction operations
type Transaction interface {
	CreateTakedownRecord(ctx context.Context, record *TakedownRecord) error
	UpdateTakedownRecord(ctx context.Context, record *TakedownRecord) error
	GetTakedownRecord(ctx context.Context, takedownID string) (*TakedownRecord, error)
	CreateAuditEntry(ctx context.Context, entry *AuditEntry) error
	CreateViolationRecord(ctx context.Context, record *ViolationRecord) error
	CreateOutboxEvent(ctx context.Context, event *OutboxEvent) error
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

// TakedownRecord represents a DMCA takedown record
type TakedownRecord struct {
	TakedownID       string    `db:"takedown_id"`
	DescriptorCID    string    `db:"descriptor_cid"`
	FilePath         string    `db:"file_path"`
	RequestorName    string    `db:"requestor_name"`
	RequestorEmail   string    `db:"requestor_email"`
	CopyrightWork    string    `db:"copyright_work"`
	TakedownDate     time.Time `db:"takedown_date"`
	Status           string    `db:"status"`
	DMCANoticeHash   string    `db:"dmca_notice_hash"`
	UploaderID       string    `db:"uploader_id"`
	OriginalNotice   string    `db:"original_notice"`
	LegalBasis       string    `db:"legal_basis"`
	ProcessingNotes  string    `db:"processing_notes"`
	CreatedAt        time.Time `db:"created_at"`
	UpdatedAt        time.Time `db:"updated_at"`
}

// ViolationRecord represents a user violation record
type ViolationRecord struct {
	ViolationID      string    `db:"violation_id"`
	UserID           string    `db:"user_id"`
	DescriptorCID    string    `db:"descriptor_cid"`
	TakedownID       string    `db:"takedown_id"`
	ViolationType    string    `db:"violation_type"`
	ViolationDate    time.Time `db:"violation_date"`
	Severity         string    `db:"severity"`
	ActionTaken      string    `db:"action_taken"`
	ResolutionStatus string    `db:"resolution_status"`
	CreatedAt        time.Time `db:"created_at"`
	UpdatedAt        time.Time `db:"updated_at"`
}

// AuditEntry represents an audit log entry
type AuditEntry struct {
	EntryID      string                 `db:"entry_id"`
	Timestamp    time.Time              `db:"timestamp"`
	EventType    string                 `db:"event_type"`
	TargetID     string                 `db:"target_id"`
	Action       string                 `db:"action"`
	Details      map[string]interface{} `db:"details"`
	PreviousHash string                 `db:"previous_hash"`
	EntryHash    string                 `db:"entry_hash"`
	UserID       string                 `db:"user_id"`
	IPAddress    string                 `db:"ip_address"`
	CreatedAt    time.Time              `db:"created_at"`
}

// OutboxEvent represents an event for reliable outbox pattern publishing
type OutboxEvent struct {
	EventID     string                 `db:"event_id"`
	EventType   string                 `db:"event_type"`
	AggregateID string                 `db:"aggregate_id"`
	Payload     map[string]interface{} `db:"payload"`
	Status      string                 `db:"status"` // pending, published, failed
	CreatedAt   time.Time              `db:"created_at"`
	PublishedAt *time.Time             `db:"published_at"`
	RetryCount  int                    `db:"retry_count"`
	LastError   string                 `db:"last_error"`
}

// NotificationRecord represents a notification record
type NotificationRecord struct {
	NotificationID string                 `db:"notification_id"`
	TargetUserID   string                 `db:"target_user_id"`
	Type           string                 `db:"type"`
	Subject        string                 `db:"subject"`
	Content        string                 `db:"content"`
	Metadata       map[string]interface{} `db:"metadata"`
	Status         string                 `db:"status"`
	SentAt         *time.Time             `db:"sent_at"`
	ReadAt         *time.Time             `db:"read_at"`
	CreatedAt      time.Time              `db:"created_at"`
	UpdatedAt      time.Time              `db:"updated_at"`
}

// ListOptions represents options for listing records with pagination and filtering
type ListOptions struct {
	Limit          int
	Offset         int
	OrderBy        string
	OrderDirection string // ASC or DESC
	Filters        map[string]interface{}
}

// DataRetentionPolicy represents data retention configuration
type DataRetentionPolicy struct {
	TableName        string        `db:"table_name"`
	RetentionPeriod  time.Duration `db:"retention_period"`
	ArchiveAfter     time.Duration `db:"archive_after"`
	DeleteAfter      time.Duration `db:"delete_after"`
	ComplianceReason string        `db:"compliance_reason"`
	Enabled          bool          `db:"enabled"`
}

// PointInTimeSnapshot represents a database snapshot for recovery
type PointInTimeSnapshot struct {
	SnapshotID   string    `db:"snapshot_id"`
	Timestamp    time.Time `db:"timestamp"`
	Description  string    `db:"description"`
	Size         int64     `db:"size"`
	Checksum     string    `db:"checksum"`
	Status       string    `db:"status"` // creating, completed, failed
	ExpiresAt    time.Time `db:"expires_at"`
	CreatedAt    time.Time `db:"created_at"`
}

// ComplianceReport represents a compliance report for legal purposes
type ComplianceReport struct {
	ReportID        string                 `db:"report_id"`
	ReportType      string                 `db:"report_type"`
	StartDate       time.Time              `db:"start_date"`
	EndDate         time.Time              `db:"end_date"`
	RequestedBy     string                 `db:"requested_by"`
	RequestedAt     time.Time              `db:"requested_at"`
	CompletedAt     *time.Time             `db:"completed_at"`
	Status          string                 `db:"status"` // pending, completed, failed
	ReportData      map[string]interface{} `db:"report_data"`
	DigitalSignature string                `db:"digital_signature"`
	LegalHash       string                 `db:"legal_hash"`
}