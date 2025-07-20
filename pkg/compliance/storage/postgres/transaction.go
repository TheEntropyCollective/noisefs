package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// pgxTransaction implements the Transaction interface using pgx
type pgxTransaction struct {
	tx pgx.Tx
	db *ComplianceDatabase
}

// CreateTakedownRecord creates a new takedown record within the transaction
func (t *pgxTransaction) CreateTakedownRecord(ctx context.Context, record *TakedownRecord) error {
	query := `
		INSERT INTO takedown_records (
			takedown_id, descriptor_cid, file_path, requestor_name, requestor_email,
			copyright_work, takedown_date, status, dmca_notice_hash, uploader_id,
			original_notice, legal_basis, processing_notes, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW(), NOW()
		)`

	_, err := t.tx.Exec(ctx, query,
		record.TakedownID,
		record.DescriptorCID,
		record.FilePath,
		record.RequestorName,
		record.RequestorEmail,
		record.CopyrightWork,
		record.TakedownDate,
		record.Status,
		record.DMCANoticeHash,
		record.UploaderID,
		record.OriginalNotice,
		record.LegalBasis,
		record.ProcessingNotes,
	)

	if err != nil {
		return fmt.Errorf("failed to create takedown record: %w", err)
	}

	return nil
}

// GetTakedownRecord retrieves a takedown record by ID within the transaction
func (t *pgxTransaction) GetTakedownRecord(ctx context.Context, takedownID string) (*TakedownRecord, error) {
	query := `
		SELECT takedown_id, descriptor_cid, file_path, requestor_name, requestor_email,
			   copyright_work, takedown_date, status, dmca_notice_hash, uploader_id,
			   original_notice, legal_basis, processing_notes, created_at, updated_at
		FROM takedown_records 
		WHERE takedown_id = $1`

	record := &TakedownRecord{}
	err := t.tx.QueryRow(ctx, query, takedownID).Scan(
		&record.TakedownID,
		&record.DescriptorCID,
		&record.FilePath,
		&record.RequestorName,
		&record.RequestorEmail,
		&record.CopyrightWork,
		&record.TakedownDate,
		&record.Status,
		&record.DMCANoticeHash,
		&record.UploaderID,
		&record.OriginalNotice,
		&record.LegalBasis,
		&record.ProcessingNotes,
		&record.CreatedAt,
		&record.UpdatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("takedown record not found: %s", takedownID)
		}
		return nil, fmt.Errorf("failed to get takedown record: %w", err)
	}

	return record, nil
}

// UpdateTakedownRecord updates an existing takedown record within the transaction
func (t *pgxTransaction) UpdateTakedownRecord(ctx context.Context, record *TakedownRecord) error {
	query := `
		UPDATE takedown_records 
		SET descriptor_cid = $2, file_path = $3, requestor_name = $4, requestor_email = $5,
			copyright_work = $6, takedown_date = $7, status = $8, dmca_notice_hash = $9,
			uploader_id = $10, original_notice = $11, legal_basis = $12, 
			processing_notes = $13, updated_at = NOW()
		WHERE takedown_id = $1`

	result, err := t.tx.Exec(ctx, query,
		record.TakedownID,
		record.DescriptorCID,
		record.FilePath,
		record.RequestorName,
		record.RequestorEmail,
		record.CopyrightWork,
		record.TakedownDate,
		record.Status,
		record.DMCANoticeHash,
		record.UploaderID,
		record.OriginalNotice,
		record.LegalBasis,
		record.ProcessingNotes,
	)

	if err != nil {
		return fmt.Errorf("failed to update takedown record: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("takedown record not found: %s", record.TakedownID)
	}

	return nil
}

// CreateAuditEntry creates a new audit entry within the transaction
func (t *pgxTransaction) CreateAuditEntry(ctx context.Context, entry *AuditEntry) error {
	query := `
		INSERT INTO audit_entries (
			entry_id, timestamp, event_type, target_id, action, details,
			previous_hash, entry_hash, user_id, ip_address, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW()
		)`

	_, err := t.tx.Exec(ctx, query,
		entry.EntryID,
		entry.Timestamp,
		entry.EventType,
		entry.TargetID,
		entry.Action,
		entry.Details,
		entry.PreviousHash,
		entry.EntryHash,
		entry.UserID,
		entry.IPAddress,
	)

	if err != nil {
		return fmt.Errorf("failed to create audit entry: %w", err)
	}

	return nil
}

// CreateViolationRecord creates a new violation record within the transaction
func (t *pgxTransaction) CreateViolationRecord(ctx context.Context, record *ViolationRecord) error {
	query := `
		INSERT INTO violation_records (
			violation_id, user_id, descriptor_cid, takedown_id, violation_type,
			violation_date, severity, action_taken, resolution_status, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW()
		)`

	_, err := t.tx.Exec(ctx, query,
		record.ViolationID,
		record.UserID,
		record.DescriptorCID,
		record.TakedownID,
		record.ViolationType,
		record.ViolationDate,
		record.Severity,
		record.ActionTaken,
		record.ResolutionStatus,
	)

	if err != nil {
		return fmt.Errorf("failed to create violation record: %w", err)
	}

	return nil
}

// CreateOutboxEvent creates a new outbox event within the transaction
func (t *pgxTransaction) CreateOutboxEvent(ctx context.Context, event *OutboxEvent) error {
	query := `
		INSERT INTO outbox_events (
			event_id, event_type, aggregate_id, payload, status, created_at,
			published_at, retry_count, last_error
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9
		)`

	_, err := t.tx.Exec(ctx, query,
		event.EventID,
		event.EventType,
		event.AggregateID,
		event.Payload,
		event.Status,
		event.CreatedAt,
		event.PublishedAt,
		event.RetryCount,
		event.LastError,
	)

	if err != nil {
		return fmt.Errorf("failed to create outbox event: %w", err)
	}

	return nil
}

// Commit commits the transaction
func (t *pgxTransaction) Commit(ctx context.Context) error {
	err := t.tx.Commit(ctx)
	if err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

// Rollback rolls back the transaction
func (t *pgxTransaction) Rollback(ctx context.Context) error {
	err := t.tx.Rollback(ctx)
	if err != nil && err != pgx.ErrTxClosed {
		return fmt.Errorf("failed to rollback transaction: %w", err)
	}
	return nil
}