package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// GetAuditEntry retrieves an audit entry by ID
func (db *ComplianceDatabase) GetAuditEntry(ctx context.Context, entryID string) (*AuditEntry, error) {
	query := `
		SELECT entry_id, timestamp, event_type, target_id, action, details,
			   previous_hash, entry_hash, user_id, ip_address, created_at
		FROM audit_entries 
		WHERE entry_id = $1`

	entry := &AuditEntry{}
	err := db.pool.QueryRow(ctx, query, entryID).Scan(
		&entry.EntryID,
		&entry.Timestamp,
		&entry.EventType,
		&entry.TargetID,
		&entry.Action,
		&entry.Details,
		&entry.PreviousHash,
		&entry.EntryHash,
		&entry.UserID,
		&entry.IPAddress,
		&entry.CreatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("audit entry not found: %s", entryID)
		}
		return nil, fmt.Errorf("failed to get audit entry: %w", err)
	}

	return entry, nil
}

// AuditEntryExists checks if an audit entry exists
func (db *ComplianceDatabase) AuditEntryExists(ctx context.Context, entryID string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM audit_entries WHERE entry_id = $1)`

	var exists bool
	err := db.pool.QueryRow(ctx, query, entryID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check audit entry existence: %w", err)
	}

	return exists, nil
}

// CreateAuditEntry creates a new audit entry with cryptographic chaining
func (db *ComplianceDatabase) CreateAuditEntry(ctx context.Context, entry *AuditEntry) error {
	// Get the previous hash for chaining
	previousHash, err := db.getLastAuditHash(ctx)
	if err != nil {
		return fmt.Errorf("failed to get previous audit hash: %w", err)
	}

	// Calculate entry hash for integrity
	entryHash := db.calculateAuditEntryHash(entry, previousHash)
	entry.PreviousHash = previousHash
	entry.EntryHash = entryHash

	query := `
		INSERT INTO audit_entries (
			entry_id, timestamp, event_type, target_id, action, details,
			previous_hash, entry_hash, user_id, ip_address, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW()
		)`

	_, err = db.pool.Exec(ctx, query,
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

// getLastAuditHash gets the hash of the most recent audit entry for chaining
func (db *ComplianceDatabase) getLastAuditHash(ctx context.Context) (string, error) {
	query := `
		SELECT entry_hash 
		FROM audit_entries 
		ORDER BY created_at DESC, entry_id DESC 
		LIMIT 1`

	var hash string
	err := db.pool.QueryRow(ctx, query).Scan(&hash)
	if err != nil {
		if err == pgx.ErrNoRows {
			// First audit entry, use genesis hash
			return "0000000000000000000000000000000000000000000000000000000000000000", nil
		}
		return "", fmt.Errorf("failed to get last audit hash: %w", err)
	}

	return hash, nil
}

// calculateAuditEntryHash calculates the cryptographic hash for an audit entry
func (db *ComplianceDatabase) calculateAuditEntryHash(entry *AuditEntry, previousHash string) string {
	// Create hash input from entry fields
	hashInput := fmt.Sprintf("%s|%s|%s|%s|%s|%v|%s|%s|%s",
		entry.EntryID,
		entry.Timestamp.Format(time.RFC3339),
		entry.EventType,
		entry.TargetID,
		entry.Action,
		entry.Details,
		previousHash,
		entry.UserID,
		entry.IPAddress,
	)

	hash := sha256.Sum256([]byte(hashInput))
	return hex.EncodeToString(hash[:])
}

// VerifyAuditChainIntegrity verifies the integrity of the audit chain
func (db *ComplianceDatabase) VerifyAuditChainIntegrity(ctx context.Context) (bool, error) {
	query := `
		SELECT entry_id, timestamp, event_type, target_id, action, details,
			   previous_hash, entry_hash, user_id, ip_address, created_at
		FROM audit_entries 
		ORDER BY created_at ASC, entry_id ASC`

	rows, err := db.pool.Query(ctx, query)
	if err != nil {
		return false, fmt.Errorf("failed to query audit entries: %w", err)
	}
	defer rows.Close()

	var previousHash = "0000000000000000000000000000000000000000000000000000000000000000" // Genesis hash
	entryCount := 0

	for rows.Next() {
		entry := &AuditEntry{}
		err := rows.Scan(
			&entry.EntryID,
			&entry.Timestamp,
			&entry.EventType,
			&entry.TargetID,
			&entry.Action,
			&entry.Details,
			&entry.PreviousHash,
			&entry.EntryHash,
			&entry.UserID,
			&entry.IPAddress,
			&entry.CreatedAt,
		)
		if err != nil {
			return false, fmt.Errorf("failed to scan audit entry: %w", err)
		}

		// Verify previous hash matches
		if entry.PreviousHash != previousHash {
			return false, fmt.Errorf("audit chain broken at entry %s: expected previous hash %s, got %s",
				entry.EntryID, previousHash, entry.PreviousHash)
		}

		// Verify entry hash is correct
		expectedHash := db.calculateAuditEntryHash(entry, entry.PreviousHash)
		if entry.EntryHash != expectedHash {
			return false, fmt.Errorf("audit entry hash mismatch at entry %s: expected %s, got %s",
				entry.EntryID, expectedHash, entry.EntryHash)
		}

		previousHash = entry.EntryHash
		entryCount++
	}

	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("error iterating audit entries: %w", err)
	}

	return true, nil
}

// GetAuditEntriesInRange gets audit entries within a time range
func (db *ComplianceDatabase) GetAuditEntriesInRange(ctx context.Context, startTime, endTime time.Time) ([]*AuditEntry, error) {
	query := `
		SELECT entry_id, timestamp, event_type, target_id, action, details,
			   previous_hash, entry_hash, user_id, ip_address, created_at
		FROM audit_entries 
		WHERE timestamp >= $1 AND timestamp <= $2
		ORDER BY timestamp ASC, entry_id ASC`

	rows, err := db.pool.Query(ctx, query, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to query audit entries: %w", err)
	}
	defer rows.Close()

	var entries []*AuditEntry
	for rows.Next() {
		entry := &AuditEntry{}
		err := rows.Scan(
			&entry.EntryID,
			&entry.Timestamp,
			&entry.EventType,
			&entry.TargetID,
			&entry.Action,
			&entry.Details,
			&entry.PreviousHash,
			&entry.EntryHash,
			&entry.UserID,
			&entry.IPAddress,
			&entry.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan audit entry: %w", err)
		}
		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating audit entries: %w", err)
	}

	return entries, nil
}

// GetAuditEntriesByTarget gets audit entries for a specific target
func (db *ComplianceDatabase) GetAuditEntriesByTarget(ctx context.Context, targetID string) ([]*AuditEntry, error) {
	query := `
		SELECT entry_id, timestamp, event_type, target_id, action, details,
			   previous_hash, entry_hash, user_id, ip_address, created_at
		FROM audit_entries 
		WHERE target_id = $1
		ORDER BY timestamp ASC, entry_id ASC`

	rows, err := db.pool.Query(ctx, query, targetID)
	if err != nil {
		return nil, fmt.Errorf("failed to query audit entries: %w", err)
	}
	defer rows.Close()

	var entries []*AuditEntry
	for rows.Next() {
		entry := &AuditEntry{}
		err := rows.Scan(
			&entry.EntryID,
			&entry.Timestamp,
			&entry.EventType,
			&entry.TargetID,
			&entry.Action,
			&entry.Details,
			&entry.PreviousHash,
			&entry.EntryHash,
			&entry.UserID,
			&entry.IPAddress,
			&entry.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan audit entry: %w", err)
		}
		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating audit entries: %w", err)
	}

	return entries, nil
}

// CreateImmutableAuditSnapshot creates a point-in-time snapshot of audit data
func (db *ComplianceDatabase) CreateImmutableAuditSnapshot(ctx context.Context, description string) (*AuditSnapshot, error) {
	// Get current audit chain state
	chainValid, err := db.VerifyAuditChainIntegrity(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to verify audit chain: %w", err)
	}

	if !chainValid {
		return nil, fmt.Errorf("audit chain integrity check failed")
	}

	// Count total entries
	var totalEntries int64
	err = db.pool.QueryRow(ctx, "SELECT COUNT(*) FROM audit_entries").Scan(&totalEntries)
	if err != nil {
		return nil, fmt.Errorf("failed to count audit entries: %w", err)
	}

	// Get last entry hash as chain head
	lastHash, err := db.getLastAuditHash(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get last audit hash: %w", err)
	}

	// Create snapshot record
	snapshot := &AuditSnapshot{
		SnapshotID:    fmt.Sprintf("AUDIT-SNAP-%d", time.Now().Unix()),
		Timestamp:     time.Now().UTC(),
		Description:   description,
		TotalEntries:  totalEntries,
		ChainHeadHash: lastHash,
		IsValid:       chainValid,
		CreatedAt:     time.Now().UTC(),
	}

	// Calculate snapshot checksum
	checksumInput := fmt.Sprintf("%s|%s|%d|%s",
		snapshot.SnapshotID,
		snapshot.Timestamp.Format(time.RFC3339),
		snapshot.TotalEntries,
		snapshot.ChainHeadHash,
	)
	checksumHash := sha256.Sum256([]byte(checksumInput))
	snapshot.Checksum = hex.EncodeToString(checksumHash[:])

	// Store snapshot metadata
	query := `
		INSERT INTO audit_snapshots (
			snapshot_id, timestamp, description, total_entries, chain_head_hash,
			checksum, is_valid, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, NOW()
		)`

	_, err = db.pool.Exec(ctx, query,
		snapshot.SnapshotID,
		snapshot.Timestamp,
		snapshot.Description,
		snapshot.TotalEntries,
		snapshot.ChainHeadHash,
		snapshot.Checksum,
		snapshot.IsValid,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create audit snapshot: %w", err)
	}

	return snapshot, nil
}

// AuditSnapshot represents a point-in-time snapshot of audit data
type AuditSnapshot struct {
	SnapshotID    string    `db:"snapshot_id"`
	Timestamp     time.Time `db:"timestamp"`
	Description   string    `db:"description"`
	TotalEntries  int64     `db:"total_entries"`
	ChainHeadHash string    `db:"chain_head_hash"`
	Checksum      string    `db:"checksum"`
	IsValid       bool      `db:"is_valid"`
	CreatedAt     time.Time `db:"created_at"`
}