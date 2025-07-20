package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// setupTestContainer creates a PostgreSQL test container for integration tests
func setupTestContainer(t *testing.T, ctx context.Context) (testcontainers.Container, string) {
	t.Helper()

	// Create PostgreSQL container
	postgresContainer, err := postgres.RunContainer(ctx,
		testcontainers.WithImage("postgres:15-alpine"),
		postgres.WithDatabase("compliance_test"),
		postgres.WithUsername("test_user"),
		postgres.WithPassword("test_password"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second)),
	)
	if err != nil {
		t.Fatalf("Failed to start PostgreSQL container: %v", err)
	}

	// Get connection string
	connStr, err := postgresContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("Failed to get connection string: %v", err)
	}

	return postgresContainer, connStr
}

// setupTestDatabase creates the database schema for testing
func setupTestDatabase(ctx context.Context, connStr string) error {
	// Create a temporary database config for schema setup
	config := &DatabaseConfig{
		ConnectionString: connStr,
		MaxConnections:   5,
		ConnectTimeout:   30 * time.Second,
	}

	db, err := NewComplianceDatabase(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to connect to test database: %w", err)
	}
	defer db.Close()

	// Create tables manually since we don't have migration files in tests
	if err := createTestTables(ctx, db); err != nil {
		return fmt.Errorf("failed to create test tables: %w", err)
	}

	return nil
}

// createTestTables creates the required tables for testing
func createTestTables(ctx context.Context, db *ComplianceDatabase) error {
	tables := []string{
		// Takedown records table
		`CREATE TABLE IF NOT EXISTS takedown_records (
			takedown_id VARCHAR(255) PRIMARY KEY,
			descriptor_cid VARCHAR(255) NOT NULL,
			file_path TEXT,
			requestor_name VARCHAR(255),
			requestor_email VARCHAR(255) NOT NULL,
			copyright_work TEXT,
			takedown_date TIMESTAMP NOT NULL,
			status VARCHAR(50) NOT NULL CHECK (status IN ('active', 'disputed', 'reinstated', 'expired')),
			dmca_notice_hash VARCHAR(255),
			uploader_id VARCHAR(255),
			original_notice TEXT,
			legal_basis VARCHAR(255),
			processing_notes TEXT,
			created_at TIMESTAMP DEFAULT NOW(),
			updated_at TIMESTAMP DEFAULT NOW()
		)`,

		// Violation records table
		`CREATE TABLE IF NOT EXISTS violation_records (
			violation_id VARCHAR(255) PRIMARY KEY,
			user_id VARCHAR(255) NOT NULL,
			descriptor_cid VARCHAR(255),
			takedown_id VARCHAR(255),
			violation_type VARCHAR(100) NOT NULL,
			violation_date TIMESTAMP NOT NULL,
			severity VARCHAR(50) CHECK (severity IN ('minor', 'major', 'severe', 'critical')),
			action_taken VARCHAR(255),
			resolution_status VARCHAR(50),
			created_at TIMESTAMP DEFAULT NOW(),
			updated_at TIMESTAMP DEFAULT NOW(),
			FOREIGN KEY (takedown_id) REFERENCES takedown_records(takedown_id)
		)`,

		// Audit entries table
		`CREATE TABLE IF NOT EXISTS audit_entries (
			entry_id VARCHAR(255) PRIMARY KEY,
			timestamp TIMESTAMP NOT NULL,
			event_type VARCHAR(100) NOT NULL,
			target_id VARCHAR(255),
			action VARCHAR(255) NOT NULL,
			details JSONB,
			previous_hash VARCHAR(64),
			entry_hash VARCHAR(64) NOT NULL,
			user_id VARCHAR(255),
			ip_address VARCHAR(45),
			created_at TIMESTAMP DEFAULT NOW()
		)`,

		// Outbox events table
		`CREATE TABLE IF NOT EXISTS outbox_events (
			event_id VARCHAR(255) PRIMARY KEY,
			event_type VARCHAR(100) NOT NULL,
			aggregate_id VARCHAR(255) NOT NULL,
			payload JSONB NOT NULL,
			status VARCHAR(50) DEFAULT 'pending' CHECK (status IN ('pending', 'published', 'failed')),
			created_at TIMESTAMP NOT NULL,
			published_at TIMESTAMP,
			retry_count INTEGER DEFAULT 0,
			last_error TEXT
		)`,

		// Notification records table
		`CREATE TABLE IF NOT EXISTS notification_records (
			notification_id VARCHAR(255) PRIMARY KEY,
			target_user_id VARCHAR(255) NOT NULL,
			type VARCHAR(100) NOT NULL,
			subject VARCHAR(255),
			content TEXT,
			metadata JSONB,
			status VARCHAR(50) DEFAULT 'pending',
			sent_at TIMESTAMP,
			read_at TIMESTAMP,
			created_at TIMESTAMP DEFAULT NOW(),
			updated_at TIMESTAMP DEFAULT NOW()
		)`,

		// Audit snapshots table
		`CREATE TABLE IF NOT EXISTS audit_snapshots (
			snapshot_id VARCHAR(255) PRIMARY KEY,
			timestamp TIMESTAMP NOT NULL,
			description TEXT,
			total_entries BIGINT NOT NULL,
			chain_head_hash VARCHAR(64) NOT NULL,
			checksum VARCHAR(64) NOT NULL,
			is_valid BOOLEAN NOT NULL,
			created_at TIMESTAMP DEFAULT NOW()
		)`,
	}

	for _, tableSQL := range tables {
		if _, err := db.pool.Exec(ctx, tableSQL); err != nil {
			return fmt.Errorf("failed to create table: %w", err)
		}
	}

	// Create indexes for performance
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_takedown_descriptor ON takedown_records(descriptor_cid)",
		"CREATE INDEX IF NOT EXISTS idx_takedown_status ON takedown_records(status)",
		"CREATE INDEX IF NOT EXISTS idx_takedown_date ON takedown_records(takedown_date)",
		"CREATE INDEX IF NOT EXISTS idx_violation_user ON violation_records(user_id)",
		"CREATE INDEX IF NOT EXISTS idx_audit_timestamp ON audit_entries(timestamp)",
		"CREATE INDEX IF NOT EXISTS idx_audit_target ON audit_entries(target_id)",
		"CREATE INDEX IF NOT EXISTS idx_outbox_status ON outbox_events(status)",
		"CREATE INDEX IF NOT EXISTS idx_outbox_created ON outbox_events(created_at)",
		"CREATE INDEX IF NOT EXISTS idx_notification_user ON notification_records(target_user_id)",
	}

	for _, indexSQL := range indexes {
		if _, err := db.pool.Exec(ctx, indexSQL); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	return nil
}

// clearTestData clears all test data from tables
func clearTestData(ctx context.Context, db *ComplianceDatabase) error {
	tables := []string{
		"audit_snapshots",
		"notification_records", 
		"outbox_events",
		"audit_entries",
		"violation_records",
		"takedown_records",
	}

	for _, table := range tables {
		query := fmt.Sprintf("DELETE FROM %s", table)
		if _, err := db.pool.Exec(ctx, query); err != nil {
			return fmt.Errorf("failed to clear table %s: %w", table, err)
		}
	}

	return nil
}

// insertTestData inserts sample test data
func insertTestData(ctx context.Context, db *ComplianceDatabase) error {
	// Insert sample takedown record
	takedown := &TakedownRecord{
		TakedownID:     "TD-test001",
		DescriptorCID:  "QmTest001",
		RequestorEmail: "test@example.com",
		CopyrightWork:  "Test Work",
		TakedownDate:   time.Now().UTC(),
		Status:         "active",
		LegalBasis:     "DMCA 512(c)",
	}

	if err := db.CreateTakedownRecord(ctx, takedown); err != nil {
		return fmt.Errorf("failed to insert test takedown: %w", err)
	}

	// Insert sample violation record
	violation := &ViolationRecord{
		ViolationID:      "VL-test001",
		UserID:           "user001",
		DescriptorCID:    "QmTest001",
		TakedownID:       "TD-test001",
		ViolationType:    "copyright_infringement",
		ViolationDate:    time.Now().UTC(),
		Severity:         "major",
		ActionTaken:      "descriptor_removed",
		ResolutionStatus: "resolved",
	}

	if err := db.CreateViolationRecord(ctx, violation); err != nil {
		return fmt.Errorf("failed to insert test violation: %w", err)
	}

	return nil
}