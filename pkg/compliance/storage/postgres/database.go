package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
)

// DatabaseConfig holds configuration for PostgreSQL compliance database
type DatabaseConfig struct {
	ConnectionString string
	MaxConnections   int32
	ConnectTimeout   time.Duration
	MigrationsPath   string
}

// ComplianceDatabase provides PostgreSQL storage for NoiseFS compliance data
type ComplianceDatabase struct {
	pool   *pgxpool.Pool
	config *DatabaseConfig
}

// NewComplianceDatabase creates a new compliance database connection
func NewComplianceDatabase(ctx context.Context, config *DatabaseConfig) (*ComplianceDatabase, error) {
	if config == nil {
		return nil, fmt.Errorf("database config is required")
	}

	if config.ConnectionString == "" {
		return nil, fmt.Errorf("connection string is required")
	}

	// Set defaults
	if config.MaxConnections == 0 {
		config.MaxConnections = 10
	}
	if config.ConnectTimeout == 0 {
		config.ConnectTimeout = 30 * time.Second
	}
	if config.MigrationsPath == "" {
		config.MigrationsPath = "file://migrations"
	}

	// Create connection pool configuration
	poolConfig, err := pgxpool.ParseConfig(config.ConnectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse connection string: %w", err)
	}

	poolConfig.MaxConns = config.MaxConnections
	poolConfig.MaxConnLifetime = 1 * time.Hour
	poolConfig.MaxConnIdleTime = 30 * time.Minute
	poolConfig.HealthCheckPeriod = 1 * time.Minute

	// Create connection pool with timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, config.ConnectTimeout)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(timeoutCtx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Test connection
	if err := pool.Ping(timeoutCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &ComplianceDatabase{
		pool:   pool,
		config: config,
	}, nil
}

// Close closes the database connection pool
func (db *ComplianceDatabase) Close() {
	if db.pool != nil {
		db.pool.Close()
	}
}

// Ping verifies database connectivity
func (db *ComplianceDatabase) Ping(ctx context.Context) error {
	return db.pool.Ping(ctx)
}

// MigrateToLatest applies all pending database migrations
func (db *ComplianceDatabase) MigrateToLatest(ctx context.Context) error {
	// Get a single connection for migration
	conn, err := db.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("failed to acquire connection for migration: %w", err)
	}
	defer conn.Release()

	// Create postgres driver for migrations using connection string
	// Note: For production, consider using dedicated migration database connection
	migrationDB, err := sql.Open("postgres", db.config.ConnectionString)
	if err != nil {
		return fmt.Errorf("failed to open migration connection: %w", err)
	}
	defer migrationDB.Close()

	driver, err := postgres.WithInstance(migrationDB, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("failed to create migration driver: %w", err)
	}

	// Create migrator
	m, err := migrate.NewWithDatabaseInstance(
		db.config.MigrationsPath,
		"postgres",
		driver,
	)
	if err != nil {
		return fmt.Errorf("failed to create migrator: %w", err)
	}
	defer m.Close()

	// Apply migrations
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	return nil
}

// GetPool returns the underlying connection pool for advanced operations
func (db *ComplianceDatabase) GetPool() *pgxpool.Pool {
	return db.pool
}

// HealthCheck performs a comprehensive health check
func (db *ComplianceDatabase) HealthCheck(ctx context.Context) error {
	// Check pool stats
	stats := db.pool.Stat()
	if stats.TotalConns() == 0 {
		return fmt.Errorf("no database connections available")
	}

	// Test query execution
	var result int
	err := db.pool.QueryRow(ctx, "SELECT 1").Scan(&result)
	if err != nil {
		return fmt.Errorf("failed to execute test query: %w", err)
	}

	if result != 1 {
		return fmt.Errorf("unexpected test query result: %d", result)
	}

	return nil
}

// GetStats returns database connection pool statistics
func (db *ComplianceDatabase) GetStats() *DatabaseStats {
	stats := db.pool.Stat()
	return &DatabaseStats{
		TotalConnections:     int(stats.TotalConns()),
		IdleConnections:      int(stats.IdleConns()),
		AcquiredConnections:  int(stats.AcquiredConns()),
		ConstructingConnections: int(stats.ConstructingConns()),
		MaxConnections:       int(db.config.MaxConnections),
		AcquireCount:         stats.AcquireCount(),
		AcquireDuration:      stats.AcquireDuration(),
		EmptyAcquireCount:    stats.EmptyAcquireCount(),
		CanceledAcquireCount: stats.CanceledAcquireCount(),
	}
}

// DatabaseStats provides database connection pool statistics
type DatabaseStats struct {
	TotalConnections        int           `json:"total_connections"`
	IdleConnections         int           `json:"idle_connections"`
	AcquiredConnections     int           `json:"acquired_connections"`
	ConstructingConnections int           `json:"constructing_connections"`
	MaxConnections          int           `json:"max_connections"`
	AcquireCount            int64         `json:"acquire_count"`
	AcquireDuration         time.Duration `json:"acquire_duration"`
	EmptyAcquireCount       int64         `json:"empty_acquire_count"`
	CanceledAcquireCount    int64         `json:"canceled_acquire_count"`
}

// BeginTransaction starts a new database transaction
func (db *ComplianceDatabase) BeginTransaction(ctx context.Context) (Transaction, error) {
	return db.BeginTransactionWithIsolation(ctx, pgx.ReadCommitted)
}

// BeginTransactionWithIsolation starts a new database transaction with specified isolation level
func (db *ComplianceDatabase) BeginTransactionWithIsolation(ctx context.Context, isolation pgx.TxIsoLevel) (Transaction, error) {
	tx, err := db.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: isolation,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

	return &pgxTransaction{
		tx: tx,
		db: db,
	}, nil
}

// WithRetry executes a function with retry logic for deadlock resolution
func (db *ComplianceDatabase) WithRetry(ctx context.Context, fn func(context.Context) error) error {
	const maxRetries = 3
	const baseDelay = 100 * time.Millisecond

	for attempt := 0; attempt < maxRetries; attempt++ {
		err := fn(ctx)
		if err == nil {
			return nil
		}

		// Check if it's a retryable error (deadlock, serialization failure)
		if isRetryableError(err) && attempt < maxRetries-1 {
			// Exponential backoff with jitter
			delay := baseDelay * time.Duration(1<<attempt)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
				continue
			}
		}

		return err
	}

	return fmt.Errorf("operation failed after %d retries", maxRetries)
}

// isRetryableError checks if an error is retryable (deadlock, serialization failure)
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check for PostgreSQL error codes that indicate retryable conditions
	errStr := err.Error()
	
	// Deadlock detected
	if contains(errStr, "deadlock detected") {
		return true
	}
	
	// Serialization failure
	if contains(errStr, "could not serialize access") {
		return true
	}
	
	// Lock not available
	if contains(errStr, "lock not available") {
		return true
	}

	return false
}

// contains checks if a string contains a substring (case-insensitive helper)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && 
		   (s == substr || len(s) > len(substr) && 
		   (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || 
		   indexOfSubstring(s, substr) >= 0))
}

// indexOfSubstring finds the index of a substring in a string
func indexOfSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}