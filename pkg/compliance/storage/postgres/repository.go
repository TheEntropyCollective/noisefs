package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// CreateTakedownRecord creates a new takedown record
func (db *ComplianceDatabase) CreateTakedownRecord(ctx context.Context, record *TakedownRecord) error {
	query := `
		INSERT INTO takedown_records (
			takedown_id, descriptor_cid, file_path, requestor_name, requestor_email,
			copyright_work, takedown_date, status, dmca_notice_hash, uploader_id,
			original_notice, legal_basis, processing_notes, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW(), NOW()
		)`

	_, err := db.pool.Exec(ctx, query,
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

// GetTakedownRecord retrieves a takedown record by ID
func (db *ComplianceDatabase) GetTakedownRecord(ctx context.Context, takedownID string) (*TakedownRecord, error) {
	query := `
		SELECT takedown_id, descriptor_cid, file_path, requestor_name, requestor_email,
			   copyright_work, takedown_date, status, dmca_notice_hash, uploader_id,
			   original_notice, legal_basis, processing_notes, created_at, updated_at
		FROM takedown_records 
		WHERE takedown_id = $1`

	record := &TakedownRecord{}
	err := db.pool.QueryRow(ctx, query, takedownID).Scan(
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

// UpdateTakedownRecord updates an existing takedown record
func (db *ComplianceDatabase) UpdateTakedownRecord(ctx context.Context, record *TakedownRecord) error {
	query := `
		UPDATE takedown_records 
		SET descriptor_cid = $2, file_path = $3, requestor_name = $4, requestor_email = $5,
			copyright_work = $6, takedown_date = $7, status = $8, dmca_notice_hash = $9,
			uploader_id = $10, original_notice = $11, legal_basis = $12, 
			processing_notes = $13, updated_at = NOW()
		WHERE takedown_id = $1`

	result, err := db.pool.Exec(ctx, query,
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

// DeleteTakedownRecord deletes a takedown record
func (db *ComplianceDatabase) DeleteTakedownRecord(ctx context.Context, takedownID string) error {
	query := `DELETE FROM takedown_records WHERE takedown_id = $1`

	result, err := db.pool.Exec(ctx, query, takedownID)
	if err != nil {
		return fmt.Errorf("failed to delete takedown record: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("takedown record not found: %s", takedownID)
	}

	return nil
}

// TakedownRecordExists checks if a takedown record exists
func (db *ComplianceDatabase) TakedownRecordExists(ctx context.Context, takedownID string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM takedown_records WHERE takedown_id = $1)`

	var exists bool
	err := db.pool.QueryRow(ctx, query, takedownID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check takedown record existence: %w", err)
	}

	return exists, nil
}

// ListTakedownRecords lists takedown records with pagination and filtering
func (db *ComplianceDatabase) ListTakedownRecords(ctx context.Context, options ListOptions) ([]*TakedownRecord, int64, error) {
	// Build WHERE clause
	whereClause, args := buildWhereClause(options.Filters)
	
	// Count total records
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM takedown_records %s", whereClause)
	var total int64
	err := db.pool.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count takedown records: %w", err)
	}

	// Build main query with pagination
	orderBy := "created_at"
	if options.OrderBy != "" {
		orderBy = options.OrderBy
	}
	
	orderDirection := "DESC"
	if options.OrderDirection != "" {
		orderDirection = options.OrderDirection
	}

	limit := 100
	if options.Limit > 0 {
		limit = options.Limit
	}

	query := fmt.Sprintf(`
		SELECT takedown_id, descriptor_cid, file_path, requestor_name, requestor_email,
			   copyright_work, takedown_date, status, dmca_notice_hash, uploader_id,
			   original_notice, legal_basis, processing_notes, created_at, updated_at
		FROM takedown_records 
		%s
		ORDER BY %s %s
		LIMIT $%d OFFSET $%d`,
		whereClause, orderBy, orderDirection, len(args)+1, len(args)+2)

	args = append(args, limit, options.Offset)

	rows, err := db.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query takedown records: %w", err)
	}
	defer rows.Close()

	var records []*TakedownRecord
	for rows.Next() {
		record := &TakedownRecord{}
		err := rows.Scan(
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
			return nil, 0, fmt.Errorf("failed to scan takedown record: %w", err)
		}
		records = append(records, record)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating takedown records: %w", err)
	}

	return records, total, nil
}

// IsDescriptorBlacklisted checks if a descriptor CID is blacklisted
func (db *ComplianceDatabase) IsDescriptorBlacklisted(ctx context.Context, descriptorCID string) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1 FROM takedown_records 
			WHERE descriptor_cid = $1 AND status IN ('active', 'disputed')
		)`

	var isBlacklisted bool
	err := db.pool.QueryRow(ctx, query, descriptorCID).Scan(&isBlacklisted)
	if err != nil {
		return false, fmt.Errorf("failed to check descriptor blacklist status: %w", err)
	}

	return isBlacklisted, nil
}

// ReinstateDescriptor reinstates a blacklisted descriptor
func (db *ComplianceDatabase) ReinstateDescriptor(ctx context.Context, descriptorCID string, reason string) error {
	query := `
		UPDATE takedown_records 
		SET status = 'reinstated', 
			processing_notes = COALESCE(processing_notes, '') || E'\nReinstatement: ' || $2,
			updated_at = NOW()
		WHERE descriptor_cid = $1 AND status IN ('active', 'disputed')`

	result, err := db.pool.Exec(ctx, query, descriptorCID, reason)
	if err != nil {
		return fmt.Errorf("failed to reinstate descriptor: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("no active takedown found for descriptor: %s", descriptorCID)
	}

	return nil
}

// CreateViolationRecord creates a new violation record
func (db *ComplianceDatabase) CreateViolationRecord(ctx context.Context, record *ViolationRecord) error {
	query := `
		INSERT INTO violation_records (
			violation_id, user_id, descriptor_cid, takedown_id, violation_type,
			violation_date, severity, action_taken, resolution_status, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW()
		)`

	_, err := db.pool.Exec(ctx, query,
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

// ListViolationRecords lists violation records with filtering
func (db *ComplianceDatabase) ListViolationRecords(ctx context.Context, options ListOptions) ([]*ViolationRecord, int64, error) {
	// Build WHERE clause
	whereClause, args := buildWhereClauseForViolations(options.Filters)
	
	// Count total records
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM violation_records %s", whereClause)
	var total int64
	err := db.pool.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count violation records: %w", err)
	}

	// Build main query with pagination
	orderBy := "created_at"
	if options.OrderBy != "" {
		orderBy = options.OrderBy
	}
	
	orderDirection := "DESC"
	if options.OrderDirection != "" {
		orderDirection = options.OrderDirection
	}

	limit := 100
	if options.Limit > 0 {
		limit = options.Limit
	}

	query := fmt.Sprintf(`
		SELECT violation_id, user_id, descriptor_cid, takedown_id, violation_type,
			   violation_date, severity, action_taken, resolution_status, created_at, updated_at
		FROM violation_records 
		%s
		ORDER BY %s %s
		LIMIT $%d OFFSET $%d`,
		whereClause, orderBy, orderDirection, len(args)+1, len(args)+2)

	args = append(args, limit, options.Offset)

	rows, err := db.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query violation records: %w", err)
	}
	defer rows.Close()

	var records []*ViolationRecord
	for rows.Next() {
		record := &ViolationRecord{}
		err := rows.Scan(
			&record.ViolationID,
			&record.UserID,
			&record.DescriptorCID,
			&record.TakedownID,
			&record.ViolationType,
			&record.ViolationDate,
			&record.Severity,
			&record.ActionTaken,
			&record.ResolutionStatus,
			&record.CreatedAt,
			&record.UpdatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan violation record: %w", err)
		}
		records = append(records, record)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating violation records: %w", err)
	}

	return records, total, nil
}

// buildWhereClause builds a WHERE clause from filters
func buildWhereClause(filters map[string]interface{}) (string, []interface{}) {
	if len(filters) == 0 {
		return "", nil
	}

	var conditions []string
	var args []interface{}
	argIndex := 1

	for key, value := range filters {
		switch key {
		case "status":
			conditions = append(conditions, fmt.Sprintf("status = $%d", argIndex))
			args = append(args, value)
			argIndex++
		case "takedown_date_since":
			if dateValue, ok := value.(time.Time); ok {
				conditions = append(conditions, fmt.Sprintf("takedown_date >= $%d", argIndex))
				args = append(args, dateValue)
				argIndex++
			}
		case "requestor_email_pattern":
			conditions = append(conditions, fmt.Sprintf("requestor_email ILIKE $%d", argIndex))
			args = append(args, value)
			argIndex++
		}
	}

	if len(conditions) == 0 {
		return "", nil
	}

	return "WHERE " + strings.Join(conditions, " AND "), args
}

// buildWhereClauseForViolations builds a WHERE clause for violation records
func buildWhereClauseForViolations(filters map[string]interface{}) (string, []interface{}) {
	if len(filters) == 0 {
		return "", nil
	}

	var conditions []string
	var args []interface{}
	argIndex := 1

	for key, value := range filters {
		switch key {
		case "user_id":
			conditions = append(conditions, fmt.Sprintf("user_id = $%d", argIndex))
			args = append(args, value)
			argIndex++
		case "severity":
			conditions = append(conditions, fmt.Sprintf("severity = $%d", argIndex))
			args = append(args, value)
			argIndex++
		case "resolution_status":
			conditions = append(conditions, fmt.Sprintf("resolution_status = $%d", argIndex))
			args = append(args, value)
			argIndex++
		}
	}

	if len(conditions) == 0 {
		return "", nil
	}

	return "WHERE " + strings.Join(conditions, " AND "), args
}