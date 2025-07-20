package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestDatabaseConnection tests basic database connectivity and setup using testcontainers
func TestDatabaseConnection(t *testing.T) {
	// This test will initially fail - guides implementation of database connection
	ctx := context.Background()
	container, connStr := setupTestContainer(t, ctx)
	defer container.Terminate(ctx)

	db, err := NewComplianceDatabase(ctx, &DatabaseConfig{
		ConnectionString: connStr,
		MaxConnections:   10,
		ConnectTimeout:   30 * time.Second,
	})
	require.NoError(t, err, "Should connect to test database")
	defer db.Close()

	// Test basic connectivity
	err = db.Ping(ctx)
	assert.NoError(t, err, "Database should be reachable")

	// Test connection pool information
	stats := db.PoolStats()
	assert.NotNil(t, stats, "Should have pool statistics")
	assert.True(t, stats.TotalConns >= 1, "Should have at least one connection")
}

// TestDatabaseConnectionFailure tests connection failure scenarios
func TestDatabaseConnectionFailure(t *testing.T) {
	ctx := context.Background()

	// Test with invalid connection string
	_, err := NewComplianceDatabase(ctx, &DatabaseConfig{
		ConnectionString: "postgres://invalid:invalid@localhost:9999/nonexistent",
		MaxConnections:   5,
		ConnectTimeout:   1 * time.Second,
	})
	assert.Error(t, err, "Should fail with invalid connection string")

	// Test with invalid configuration
	_, err = NewComplianceDatabase(ctx, nil)
	assert.Error(t, err, "Should fail with nil configuration")

	_, err = NewComplianceDatabase(ctx, &DatabaseConfig{
		ConnectionString: "",
		MaxConnections:   0,
	})
	assert.Error(t, err, "Should fail with empty configuration")
}

// TestDatabaseHealthMonitoring tests health check and monitoring functionality
func TestDatabaseHealthMonitoring(t *testing.T) {
	ctx := context.Background()
	container, connStr := setupTestContainer(t, ctx)
	defer container.Terminate(ctx)

	db, err := NewComplianceDatabase(ctx, &DatabaseConfig{
		ConnectionString: connStr,
		MaxConnections:   10,
		ConnectTimeout:   30 * time.Second,
	})
	require.NoError(t, err)
	defer db.Close()

	// Test health check
	health := db.HealthCheck(ctx)
	assert.True(t, health.Healthy, "Database should be healthy")
	assert.NotEmpty(t, health.Version, "Should have PostgreSQL version")
	assert.Greater(t, health.ActiveConnections, int32(0), "Should have active connections")
	assert.GreaterOrEqual(t, health.IdleConnections, int32(0), "Should report idle connections")

	// Test connection pool metrics
	stats := db.PoolStats()
	assert.NotNil(t, stats, "Should have pool statistics")
	assert.GreaterOrEqual(t, stats.TotalConns, int32(1), "Should have at least one connection")
	assert.GreaterOrEqual(t, stats.IdleConns, int32(0), "Should report idle connections")
	assert.GreaterOrEqual(t, stats.AcquiredConns, int32(0), "Should report acquired connections")
}

// TestDatabaseMigration tests schema migration functionality
func TestDatabaseMigration(t *testing.T) {
	tests := []struct {
		name           string
		fromVersion    int
		toVersion      int
		expectError    bool
		expectedTables []string
	}{
		{
			name:        "Fresh migration to v1",
			fromVersion: 0,
			toVersion:   1,
			expectError: false,
			expectedTables: []string{
				"schema_migrations",
				"takedown_records",
				"takedown_events",
				"violation_records",
				"audit_entries",
				"compliance_metrics",
			},
		},
		{
			name:        "Version upgrade v1 to v2",
			fromVersion: 1,
			toVersion:   2,
			expectError: false,
			expectedTables: []string{
				"schema_migrations",
				"takedown_records",
				"takedown_events",
				"violation_records",
				"audit_entries",
				"compliance_metrics",
				"counter_notices", // Added in v2
				"notification_records", // Added in v2
			},
		},
		{
			name:        "Invalid downgrade",
			fromVersion: 2,
			toVersion:   1,
			expectError: true,
		},
		{
			name:        "Migration to same version",
			fromVersion: 1,
			toVersion:   1,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			container, connStr := setupTestContainer(t, ctx)
			defer container.Terminate(ctx)

			db, err := NewComplianceDatabase(ctx, &DatabaseConfig{
				ConnectionString: connStr,
				MaxConnections:   10,
				ConnectTimeout:   30 * time.Second,
			})
			require.NoError(t, err)
			defer db.Close()

			// Set initial version if needed
			if tt.fromVersion > 0 {
				err = db.SetSchemaVersion(ctx, tt.fromVersion)
				require.NoError(t, err)
			}

			// Perform migration
			err = db.MigrateToVersion(ctx, tt.toVersion)

			if tt.expectError {
				assert.Error(t, err, "Should fail for invalid migration")
				return
			}

			require.NoError(t, err, "Migration should succeed")

			// Verify tables exist
			for _, table := range tt.expectedTables {
				exists, err := db.TableExists(ctx, table)
				require.NoError(t, err)
				assert.True(t, exists, "Table %s should exist after migration", table)
			}

			// Verify version is updated
			version, err := db.GetSchemaVersion(ctx)
			require.NoError(t, err)
			assert.Equal(t, tt.toVersion, version, "Schema version should be updated")
		})
	}
}

// TestMigrationToLatest tests migration to the latest schema version
func TestMigrationToLatest(t *testing.T) {
	ctx := context.Background()
	container, connStr := setupTestContainer(t, ctx)
	defer container.Terminate(ctx)

	db, err := NewComplianceDatabase(ctx, &DatabaseConfig{
		ConnectionString: connStr,
		MaxConnections:   10,
		ConnectTimeout:   30 * time.Second,
	})
	require.NoError(t, err)
	defer db.Close()

	// Migrate to latest
	err = db.MigrateToLatest(ctx)
	require.NoError(t, err, "Should migrate to latest version")

	// Verify we're at latest version
	version, err := db.GetSchemaVersion(ctx)
	require.NoError(t, err)
	
	latest := db.GetLatestSchemaVersion()
	assert.Equal(t, latest, version, "Should be at latest schema version")
	assert.Greater(t, version, 0, "Latest version should be greater than 0")
}

// TestSchemaVersionManagement tests schema version tracking
func TestSchemaVersionManagement(t *testing.T) {
	ctx := context.Background()
	container, connStr := setupTestContainer(t, ctx)
	defer container.Terminate(ctx)

	db, err := NewComplianceDatabase(ctx, &DatabaseConfig{
		ConnectionString: connStr,
		MaxConnections:   10,
		ConnectTimeout:   30 * time.Second,
	})
	require.NoError(t, err)
	defer db.Close()

	// Test initial version
	version, err := db.GetSchemaVersion(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 0, version, "Initial version should be 0")

	// Test setting version
	err = db.SetSchemaVersion(ctx, 1)
	assert.NoError(t, err, "Should set version to 1")

	version, err = db.GetSchemaVersion(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 1, version, "Version should be 1")

	// Test setting higher version
	err = db.SetSchemaVersion(ctx, 5)
	assert.NoError(t, err, "Should set version to 5")

	version, err = db.GetSchemaVersion(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 5, version, "Version should be 5")

	// Test invalid version
	err = db.SetSchemaVersion(ctx, -1)
	assert.Error(t, err, "Should fail with negative version")
}

// TestTableExistence tests table existence checking
func TestTableExistence(t *testing.T) {
	ctx := context.Background()
	container, connStr := setupTestContainer(t, ctx)
	defer container.Terminate(ctx)

	db, err := NewComplianceDatabase(ctx, &DatabaseConfig{
		ConnectionString: connStr,
		MaxConnections:   10,
		ConnectTimeout:   30 * time.Second,
	})
	require.NoError(t, err)
	defer db.Close()

	// Test non-existent table
	exists, err := db.TableExists(ctx, "nonexistent_table")
	assert.NoError(t, err)
	assert.False(t, exists, "Non-existent table should not exist")

	// Create a table and test
	_, err = db.pool.Exec(ctx, "CREATE TABLE test_table (id SERIAL PRIMARY KEY)")
	require.NoError(t, err)

	exists, err = db.TableExists(ctx, "test_table")
	assert.NoError(t, err)
	assert.True(t, exists, "Created table should exist")

	// Test case sensitivity
	exists, err = db.TableExists(ctx, "TEST_TABLE")
	assert.NoError(t, err)
	assert.True(t, exists, "Table name should be case insensitive")
}

// TestConcurrentConnections tests concurrent database access
func TestConcurrentConnections(t *testing.T) {
	ctx := context.Background()
	container, connStr := setupTestContainer(t, ctx)
	defer container.Terminate(ctx)

	db, err := NewComplianceDatabase(ctx, &DatabaseConfig{
		ConnectionString: connStr,
		MaxConnections:   20,
		ConnectTimeout:   30 * time.Second,
	})
	require.NoError(t, err)
	defer db.Close()

	// Test concurrent connections
	const numConcurrent = 10
	errChan := make(chan error, numConcurrent)
	doneChan := make(chan bool, numConcurrent)

	for i := 0; i < numConcurrent; i++ {
		go func(index int) {
			defer func() { doneChan <- true }()
			
			// Perform concurrent operations
			err := db.Ping(ctx)
			if err != nil {
				errChan <- fmt.Errorf("ping failed for goroutine %d: %w", index, err)
				return
			}

			// Test concurrent table existence check
			exists, err := db.TableExists(ctx, "pg_tables")
			if err != nil {
				errChan <- fmt.Errorf("table check failed for goroutine %d: %w", index, err)
				return
			}
			if !exists {
				errChan <- fmt.Errorf("pg_tables should exist for goroutine %d", index)
				return
			}

			errChan <- nil
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numConcurrent; i++ {
		select {
		case err := <-errChan:
			assert.NoError(t, err, "Concurrent operation should succeed")
		case <-doneChan:
			// Goroutine completed
		case <-time.After(30 * time.Second):
			t.Fatal("Timeout waiting for concurrent operations")
		}
	}

	// Verify connection pool is still healthy
	stats := db.PoolStats()
	assert.NotNil(t, stats, "Should have pool statistics after concurrent access")
	assert.True(t, stats.TotalConns > 0, "Should have connections after concurrent access")
}

// TestDatabaseBackupRestore tests backup and restore functionality for legal compliance
func TestDatabaseBackupRestore(t *testing.T) {
	ctx := context.Background()
	container, connStr := setupTestContainer(t, ctx)
	defer container.Terminate(ctx)

	db, err := NewComplianceDatabase(ctx, &DatabaseConfig{
		ConnectionString: connStr,
		MaxConnections:   10,
		ConnectTimeout:   30 * time.Second,
	})
	require.NoError(t, err)
	defer db.Close()

	// Migrate to latest to have tables
	err = db.MigrateToLatest(ctx)
	require.NoError(t, err)

	// Test backup creation
	backupData, err := db.CreateBackup(ctx, BackupOptions{
		IncludeData: true,
		IncludeSchema: true,
		Compression: true,
	})
	assert.NoError(t, err, "Should create backup")
	assert.NotEmpty(t, backupData, "Backup data should not be empty")

	// Test backup metadata
	metadata, err := db.GetBackupMetadata(backupData)
	assert.NoError(t, err, "Should get backup metadata")
	assert.NotEmpty(t, metadata.Timestamp, "Backup should have timestamp")
	assert.NotEmpty(t, metadata.Version, "Backup should have version")
	assert.True(t, metadata.Size > 0, "Backup should have size")

	// Test backup validation
	valid, err := db.ValidateBackup(ctx, backupData)
	assert.NoError(t, err, "Should validate backup")
	assert.True(t, valid, "Backup should be valid")

	// Test restore capability (note: this would be destructive in real usage)
	err = db.ValidateRestoreCompatibility(ctx, backupData)
	assert.NoError(t, err, "Should be able to restore backup")
}

// Helper functions and types for testing

// setupTestContainer creates a PostgreSQL testcontainer for testing
func setupTestContainer(t *testing.T, ctx context.Context) (testcontainers.Container, string) {
	t.Helper()

	postgresContainer, err := postgres.RunContainer(ctx,
		testcontainers.WithImage("postgres:15-alpine"),
		postgres.WithDatabase("noisefs_compliance_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second)),
	)
	require.NoError(t, err, "Should start PostgreSQL container")

	connStr, err := postgresContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err, "Should get connection string")

	return postgresContainer, connStr
}

// Types that need to be implemented - these will fail compilation initially

// ComplianceDatabase represents the PostgreSQL-backed compliance system
type ComplianceDatabase struct {
	pool   *pgxpool.Pool
	config *DatabaseConfig
}

// DatabaseConfig holds database connection configuration
type DatabaseConfig struct {
	ConnectionString string
	MaxConnections   int
	ConnectTimeout   time.Duration
	IdleTimeout      time.Duration
	MaxLifetime      time.Duration
}

// HealthStatus represents database health information
type HealthStatus struct {
	Healthy           bool
	Version           string
	ActiveConnections int32
	IdleConnections   int32
	MaxConnections    int32
	ResponseTime      time.Duration
}

// BackupOptions configures backup creation
type BackupOptions struct {
	IncludeData   bool
	IncludeSchema bool
	Compression   bool
	Tables        []string // Specific tables to backup, empty for all
}

// BackupMetadata contains information about a backup
type BackupMetadata struct {
	Timestamp       time.Time
	Version         int
	Size            int64
	Checksum        string
	CompressionType string
	Tables          []string
}

// These methods need to be implemented - they will fail compilation initially

func NewComplianceDatabase(ctx context.Context, config *DatabaseConfig) (*ComplianceDatabase, error) {
	panic("not implemented - TDD implementation needed")
}

func (db *ComplianceDatabase) Close() error {
	panic("not implemented - TDD implementation needed")
}

func (db *ComplianceDatabase) Ping(ctx context.Context) error {
	panic("not implemented - TDD implementation needed")
}

func (db *ComplianceDatabase) PoolStats() *pgxpool.Stat {
	panic("not implemented - TDD implementation needed")
}

func (db *ComplianceDatabase) HealthCheck(ctx context.Context) *HealthStatus {
	panic("not implemented - TDD implementation needed")
}

func (db *ComplianceDatabase) MigrateToVersion(ctx context.Context, version int) error {
	panic("not implemented - TDD implementation needed")
}

func (db *ComplianceDatabase) MigrateToLatest(ctx context.Context) error {
	panic("not implemented - TDD implementation needed")
}

func (db *ComplianceDatabase) GetSchemaVersion(ctx context.Context) (int, error) {
	panic("not implemented - TDD implementation needed")
}

func (db *ComplianceDatabase) SetSchemaVersion(ctx context.Context, version int) error {
	panic("not implemented - TDD implementation needed")
}

func (db *ComplianceDatabase) GetLatestSchemaVersion() int {
	panic("not implemented - TDD implementation needed")
}

func (db *ComplianceDatabase) TableExists(ctx context.Context, tableName string) (bool, error) {
	panic("not implemented - TDD implementation needed")
}

func (db *ComplianceDatabase) CreateBackup(ctx context.Context, options BackupOptions) ([]byte, error) {
	panic("not implemented - TDD implementation needed")
}

func (db *ComplianceDatabase) GetBackupMetadata(backupData []byte) (*BackupMetadata, error) {
	panic("not implemented - TDD implementation needed")
}

func (db *ComplianceDatabase) ValidateBackup(ctx context.Context, backupData []byte) (bool, error) {
	panic("not implemented - TDD implementation needed")
}

func (db *ComplianceDatabase) ValidateRestoreCompatibility(ctx context.Context, backupData []byte) error {
	panic("not implemented - TDD implementation needed")
}