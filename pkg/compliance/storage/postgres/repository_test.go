package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Types are defined in types.go

// NotificationRecord type is defined in types.go

// TestTakedownRecordCRUD tests CRUD operations for takedown records
func TestTakedownRecordCRUD(t *testing.T) {
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

	// Migrate to latest schema
	err = db.MigrateToLatest(ctx)
	require.NoError(t, err)

	// Test data
	takedownRecord := &TakedownRecord{
		TakedownID:      "TD-test123",
		DescriptorCID:   "QmTest123456789abcdef",
		FilePath:        "/test/file.txt",
		RequestorName:   "Test Requestor",
		RequestorEmail:  "test@example.com",
		CopyrightWork:   "Test Work",
		TakedownDate:    time.Now().UTC().Truncate(time.Second),
		Status:          "active",
		DMCANoticeHash:  "hash123",
		UploaderID:      "user123",
		LegalBasis:      "DMCA 512(c)",
		ProcessingNotes: "Test takedown",
		OriginalNotice:  "Original DMCA notice content",
	}

	t.Run("Create", func(t *testing.T) {
		err := db.CreateTakedownRecord(ctx, takedownRecord)
		assert.NoError(t, err, "Should create takedown record")

		// Verify record exists
		exists, err := db.TakedownRecordExists(ctx, takedownRecord.TakedownID)
		require.NoError(t, err)
		assert.True(t, exists, "Record should exist after creation")
	})

	t.Run("Read", func(t *testing.T) {
		retrieved, err := db.GetTakedownRecord(ctx, takedownRecord.TakedownID)
		require.NoError(t, err, "Should retrieve takedown record")
		require.NotNil(t, retrieved, "Retrieved record should not be nil")

		assert.Equal(t, takedownRecord.TakedownID, retrieved.TakedownID)
		assert.Equal(t, takedownRecord.DescriptorCID, retrieved.DescriptorCID)
		assert.Equal(t, takedownRecord.RequestorEmail, retrieved.RequestorEmail)
		assert.Equal(t, takedownRecord.Status, retrieved.Status)
		assert.Equal(t, takedownRecord.TakedownDate.Unix(), retrieved.TakedownDate.Unix())
	})

	t.Run("Update", func(t *testing.T) {
		takedownRecord.Status = "disputed"
		takedownRecord.ProcessingNotes = "Counter-notice received"

		err := db.UpdateTakedownRecord(ctx, takedownRecord)
		assert.NoError(t, err, "Should update takedown record")

		// Verify update
		retrieved, err := db.GetTakedownRecord(ctx, takedownRecord.TakedownID)
		require.NoError(t, err)
		assert.Equal(t, "disputed", retrieved.Status)
		assert.Equal(t, "Counter-notice received", retrieved.ProcessingNotes)
	})

	t.Run("List with filters", func(t *testing.T) {
		// Create additional records for testing
		for i := 0; i < 5; i++ {
			record := &TakedownRecord{
				TakedownID:     fmt.Sprintf("TD-list%d", i),
				DescriptorCID:  fmt.Sprintf("QmList%d", i),
				RequestorEmail: "list@example.com",
				Status:         "active",
				TakedownDate:   time.Now().UTC().Add(-time.Duration(i) * time.Hour),
				CopyrightWork:  fmt.Sprintf("Work %d", i),
				LegalBasis:     "DMCA 512(c)",
			}
			err := db.CreateTakedownRecord(ctx, record)
			require.NoError(t, err)
		}

		// Test listing with pagination
		records, total, err := db.ListTakedownRecords(ctx, ListOptions{
			Limit:  3,
			Offset: 0,
			OrderBy: "takedown_date",
			OrderDirection: "DESC",
		})
		assert.NoError(t, err, "Should list takedown records")
		assert.Len(t, records, 3, "Should return 3 records")
		assert.Greater(t, total, int64(3), "Total should be greater than 3")

		// Test filtering by status
		activeRecords, _, err := db.ListTakedownRecords(ctx, ListOptions{
			Filters: map[string]interface{}{
				"status": "active",
			},
		})
		assert.NoError(t, err, "Should filter by status")
		for _, record := range activeRecords {
			assert.Equal(t, "active", record.Status)
		}

		// Test filtering by date range
		since := time.Now().UTC().Add(-2 * time.Hour)
		recentRecords, _, err := db.ListTakedownRecords(ctx, ListOptions{
			Filters: map[string]interface{}{
				"takedown_date_since": since,
			},
		})
		assert.NoError(t, err, "Should filter by date range")
		for _, record := range recentRecords {
			assert.True(t, record.TakedownDate.After(since))
		}
	})

	t.Run("Delete", func(t *testing.T) {
		err := db.DeleteTakedownRecord(ctx, takedownRecord.TakedownID)
		assert.NoError(t, err, "Should delete takedown record")

		// Verify deletion
		exists, err := db.TakedownRecordExists(ctx, takedownRecord.TakedownID)
		require.NoError(t, err)
		assert.False(t, exists, "Record should not exist after deletion")
	})
}

// TestDescriptorBlacklistLookup tests the critical descriptor blacklist functionality
func TestDescriptorBlacklistLookup(t *testing.T) {
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

	err = db.MigrateToLatest(ctx)
	require.NoError(t, err)

	descriptorCID := "QmTest123456789abcdef"

	t.Run("NotBlacklisted", func(t *testing.T) {
		isBlacklisted, err := db.IsDescriptorBlacklisted(ctx, descriptorCID)
		require.NoError(t, err)
		assert.False(t, isBlacklisted, "Descriptor should not be blacklisted initially")
	})

	t.Run("AddToBlacklist", func(t *testing.T) {
		// Create takedown record to blacklist descriptor
		takedownRecord := &TakedownRecord{
			TakedownID:     "TD-blacklist123",
			DescriptorCID:  descriptorCID,
			RequestorEmail: "test@example.com",
			CopyrightWork:  "Test Work",
			TakedownDate:   time.Now().UTC(),
			Status:         "active",
			LegalBasis:     "DMCA 512(c)",
		}

		err := db.CreateTakedownRecord(ctx, takedownRecord)
		require.NoError(t, err)

		// Check blacklist status
		isBlacklisted, err := db.IsDescriptorBlacklisted(ctx, descriptorCID)
		require.NoError(t, err)
		assert.True(t, isBlacklisted, "Descriptor should be blacklisted after takedown")
	})

	t.Run("ReinstatementRemovesFromBlacklist", func(t *testing.T) {
		// Reinstate descriptor
		err := db.ReinstateDescriptor(ctx, descriptorCID, "Counter-notice waiting period elapsed")
		require.NoError(t, err)

		// Check blacklist status
		isBlacklisted, err := db.IsDescriptorBlacklisted(ctx, descriptorCID)
		require.NoError(t, err)
		assert.False(t, isBlacklisted, "Descriptor should not be blacklisted after reinstatement")
	})
}

// TestRowLevelSecurity tests Row-Level Security (RLS) policy enforcement
func TestRowLevelSecurity(t *testing.T) {
	ctx := context.Background()
	container, connStr := setupTestContainer(t, ctx)
	defer container.Terminate(ctx)

	// Create separate database connections for different user roles
	adminDB, err := NewComplianceDatabase(ctx, &DatabaseConfig{
		ConnectionString: connStr,
		MaxConnections:   5,
		ConnectTimeout:   30 * time.Second,
	})
	require.NoError(t, err)
	defer adminDB.Close()

	err = adminDB.MigrateToLatest(ctx)
	require.NoError(t, err)

	// Enable RLS policies
	err = adminDB.EnableRLSPolicies(ctx)
	require.NoError(t, err)

	t.Run("AdminCanAccessAllRecords", func(t *testing.T) {
		// Set admin role context
		ctx := adminDB.SetUserRole(ctx, "admin", "admin_user_123")

		// Create records with different users
		for i := 0; i < 3; i++ {
			record := &TakedownRecord{
				TakedownID:     fmt.Sprintf("TD-admin%d", i),
				DescriptorCID:  fmt.Sprintf("QmAdmin%d", i),
				RequestorEmail: fmt.Sprintf("admin%d@example.com", i),
				UploaderID:     fmt.Sprintf("user%d", i),
				Status:         "active",
				TakedownDate:   time.Now().UTC(),
				LegalBasis:     "DMCA 512(c)",
			}
			err := adminDB.CreateTakedownRecord(ctx, record)
			require.NoError(t, err)
		}

		// Admin should see all records
		records, total, err := adminDB.ListTakedownRecords(ctx, ListOptions{})
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, total, int64(3), "Admin should see all records")
		assert.GreaterOrEqual(t, len(records), 3, "Admin should see all records")
	})

	t.Run("LegalCanAccessRelevantRecords", func(t *testing.T) {
		// Set legal role context
		ctx := adminDB.SetUserRole(ctx, "legal", "legal_user_123")

		// Legal users should access takedown records but not all user data
		records, _, err := adminDB.ListTakedownRecords(ctx, ListOptions{})
		assert.NoError(t, err, "Legal user should access takedown records")
		assert.Greater(t, len(records), 0, "Legal user should see takedown records")

		// But legal users should not access personal user violations for other users
		violations, _, err := adminDB.ListViolationRecords(ctx, ListOptions{
			Filters: map[string]interface{}{
				"user_id": "user1", // Different user
			},
		})
		assert.Error(t, err, "Legal user should not access other user's violations")
		assert.Empty(t, violations, "Should not return other user's violations")
	})

	t.Run("UserCanOnlyAccessOwnData", func(t *testing.T) {
		// Set user role context
		userID := "user1"
		ctx := adminDB.SetUserRole(ctx, "user", userID)

		// User should only see their own violation records
		violations, _, err := adminDB.ListViolationRecords(ctx, ListOptions{})
		assert.NoError(t, err, "User should access their own violations")
		
		// All returned violations should belong to this user
		for _, violation := range violations {
			assert.Equal(t, userID, violation.UserID, "User should only see their own violations")
		}

		// User should not be able to create takedown records
		record := &TakedownRecord{
			TakedownID:     "TD-user-attempt",
			DescriptorCID:  "QmUserAttempt",
			RequestorEmail: "user@example.com",
			Status:         "active",
			TakedownDate:   time.Now().UTC(),
			LegalBasis:     "DMCA 512(c)",
		}
		err = adminDB.CreateTakedownRecord(ctx, record)
		assert.Error(t, err, "User should not be able to create takedown records")
	})

	t.Run("RLSPolicyViolationAttempts", func(t *testing.T) {
		// Try to bypass RLS with SQL injection-like attempts
		ctx := adminDB.SetUserRole(ctx, "user", "user1")

		// Attempt to access other user's data through filters
		violations, _, err := adminDB.ListViolationRecords(ctx, ListOptions{
			Filters: map[string]interface{}{
				"user_id": "user2 OR 1=1", // SQL injection attempt
			},
		})
		assert.NoError(t, err, "Should handle injection attempts gracefully")
		
		// Should only return user1's violations, not all violations
		for _, violation := range violations {
			assert.Equal(t, "user1", violation.UserID, "Should only return own user's violations")
		}
	})
}

// TestQueryPerformance tests query performance and indexing
func TestQueryPerformance(t *testing.T) {
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

	err = db.MigrateToLatest(ctx)
	require.NoError(t, err)

	// Create test data for performance testing
	const numRecords = 1000
	t.Logf("Creating %d test records for performance testing", numRecords)

	start := time.Now()
	for i := 0; i < numRecords; i++ {
		record := &TakedownRecord{
			TakedownID:     fmt.Sprintf("TD-perf%06d", i),
			DescriptorCID:  fmt.Sprintf("QmPerf%06d", i),
			RequestorEmail: fmt.Sprintf("perf%d@example.com", i%100), // 100 unique emails
			Status:         []string{"active", "disputed", "reinstated"}[i%3],
			TakedownDate:   time.Now().UTC().Add(-time.Duration(i) * time.Minute),
			CopyrightWork:  fmt.Sprintf("Performance Test Work %d", i),
			LegalBasis:     "DMCA 512(c)",
		}
		err := db.CreateTakedownRecord(ctx, record)
		require.NoError(t, err)
	}
	createDuration := time.Since(start)
	t.Logf("Created %d records in %v (%.2f records/sec)", numRecords, createDuration, float64(numRecords)/createDuration.Seconds())

	t.Run("DescriptorCIDLookupPerformance", func(t *testing.T) {
		// Test descriptor CID lookup performance (must be O(1) for compliance)
		const numLookups = 100
		start := time.Now()

		for i := 0; i < numLookups; i++ {
			descriptorCID := fmt.Sprintf("QmPerf%06d", i)
			isBlacklisted, err := db.IsDescriptorBlacklisted(ctx, descriptorCID)
			require.NoError(t, err)
			assert.True(t, isBlacklisted, "Record should be blacklisted")
		}

		duration := time.Since(start)
		avgLookupTime := duration / numLookups
		t.Logf("Average descriptor lookup time: %v", avgLookupTime)
		
		// Should be sub-millisecond for compliance requirements
		assert.Less(t, avgLookupTime.Milliseconds(), int64(5), "Descriptor lookup should be < 5ms")
	})

	t.Run("PaginationPerformance", func(t *testing.T) {
		// Test pagination performance with different page sizes
		pageSizes := []int{10, 50, 100, 500}
		
		for _, pageSize := range pageSizes {
			start := time.Now()
			
			records, total, err := db.ListTakedownRecords(ctx, ListOptions{
				Limit:  pageSize,
				Offset: 0,
				OrderBy: "takedown_date",
				OrderDirection: "DESC",
			})
			
			duration := time.Since(start)
			t.Logf("Page size %d: %v duration, %d records, %d total", pageSize, duration, len(records), total)
			
			assert.NoError(t, err, "Pagination should work")
			assert.Len(t, records, pageSize, "Should return correct page size")
			assert.Equal(t, int64(numRecords), total, "Should return correct total")
			assert.Less(t, duration.Milliseconds(), int64(500), "Pagination should be < 500ms")
		}
	})

	t.Run("ComplexQueryPerformance", func(t *testing.T) {
		// Test complex queries with multiple filters
		start := time.Now()
		
		records, _, err := db.ListTakedownRecords(ctx, ListOptions{
			Filters: map[string]interface{}{
				"status": "active",
				"takedown_date_since": time.Now().UTC().Add(-500 * time.Minute),
				"requestor_email_pattern": "perf1%@example.com",
			},
			OrderBy: "takedown_date",
			OrderDirection: "DESC",
			Limit: 50,
		})
		
		duration := time.Since(start)
		t.Logf("Complex query: %v duration, %d records", duration, len(records))
		
		assert.NoError(t, err, "Complex query should work")
		assert.Less(t, duration.Milliseconds(), int64(100), "Complex query should be < 100ms")
		
		// Verify filtering worked correctly
		for _, record := range records {
			assert.Equal(t, "active", record.Status)
			assert.Contains(t, record.RequestorEmail, "perf1")
		}
	})
}

// TestDataConsistencyAndIntegrity tests referential integrity and constraints
func TestDataConsistencyAndIntegrity(t *testing.T) {
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

	err = db.MigrateToLatest(ctx)
	require.NoError(t, err)

	t.Run("UniqueConstraints", func(t *testing.T) {
		// Create a takedown record
		record := &TakedownRecord{
			TakedownID:     "TD-unique123",
			DescriptorCID:  "QmUnique123",
			RequestorEmail: "unique@example.com",
			Status:         "active",
			TakedownDate:   time.Now().UTC(),
			LegalBasis:     "DMCA 512(c)",
		}
		
		err := db.CreateTakedownRecord(ctx, record)
		require.NoError(t, err)

		// Try to create duplicate - should fail
		err = db.CreateTakedownRecord(ctx, record)
		assert.Error(t, err, "Should fail with duplicate takedown ID")
	})

	t.Run("ForeignKeyConstraints", func(t *testing.T) {
		// Create violation record with valid takedown ID
		violation := &ViolationRecord{
			ViolationID:   "VL-fk123",
			UserID:        "user123",
			DescriptorCID: "QmUnique123",
			TakedownID:    "TD-unique123", // References existing takedown
			ViolationType: "copyright_infringement",
			ViolationDate: time.Now().UTC(),
			Severity:      "major",
			ActionTaken:   "descriptor_removed",
			ResolutionStatus: "resolved",
		}
		
		err := db.CreateViolationRecord(ctx, violation)
		assert.NoError(t, err, "Should create violation with valid takedown reference")

		// Try to create violation with invalid takedown ID - should fail
		violation.ViolationID = "VL-fk456"
		violation.TakedownID = "TD-nonexistent"
		
		err = db.CreateViolationRecord(ctx, violation)
		assert.Error(t, err, "Should fail with invalid takedown reference")
	})

	t.Run("CheckConstraints", func(t *testing.T) {
		// Test invalid status values
		record := &TakedownRecord{
			TakedownID:     "TD-check123",
			DescriptorCID:  "QmCheck123",
			RequestorEmail: "check@example.com",
			Status:         "invalid_status", // Should fail check constraint
			TakedownDate:   time.Now().UTC(),
			LegalBasis:     "DMCA 512(c)",
		}
		
		err := db.CreateTakedownRecord(ctx, record)
		assert.Error(t, err, "Should fail with invalid status value")

		// Test valid status
		record.Status = "active"
		err = db.CreateTakedownRecord(ctx, record)
		assert.NoError(t, err, "Should succeed with valid status")
	})

	t.Run("NullConstraints", func(t *testing.T) {
		// Test required fields
		record := &TakedownRecord{
			TakedownID:    "TD-null123",
			DescriptorCID: "QmNull123",
			// RequestorEmail missing - should fail
			Status:       "active",
			TakedownDate: time.Now().UTC(),
			LegalBasis:   "DMCA 512(c)",
		}
		
		err := db.CreateTakedownRecord(ctx, record)
		assert.Error(t, err, "Should fail with missing required field")
	})
}

// Helper types and functions

// ListOptions represents options for listing records with pagination and filtering
type ListOptions struct {
	Limit          int
	Offset         int
	OrderBy        string
	OrderDirection string // ASC or DESC
	Filters        map[string]interface{}
}

// Methods that need to be implemented - will fail compilation initially

func (db *ComplianceDatabase) CreateTakedownRecord(ctx context.Context, record *TakedownRecord) error {
	panic("not implemented - TDD implementation needed")
}

func (db *ComplianceDatabase) GetTakedownRecord(ctx context.Context, takedownID string) (*TakedownRecord, error) {
	panic("not implemented - TDD implementation needed")
}

func (db *ComplianceDatabase) UpdateTakedownRecord(ctx context.Context, record *TakedownRecord) error {
	panic("not implemented - TDD implementation needed")
}

func (db *ComplianceDatabase) DeleteTakedownRecord(ctx context.Context, takedownID string) error {
	panic("not implemented - TDD implementation needed")
}

func (db *ComplianceDatabase) TakedownRecordExists(ctx context.Context, takedownID string) (bool, error) {
	panic("not implemented - TDD implementation needed")
}

func (db *ComplianceDatabase) ListTakedownRecords(ctx context.Context, options ListOptions) ([]*TakedownRecord, int64, error) {
	panic("not implemented - TDD implementation needed")
}

func (db *ComplianceDatabase) IsDescriptorBlacklisted(ctx context.Context, descriptorCID string) (bool, error) {
	panic("not implemented - TDD implementation needed")
}

func (db *ComplianceDatabase) ReinstateDescriptor(ctx context.Context, descriptorCID string, reason string) error {
	panic("not implemented - TDD implementation needed")
}

func (db *ComplianceDatabase) CreateViolationRecord(ctx context.Context, record *ViolationRecord) error {
	panic("not implemented - TDD implementation needed")
}

func (db *ComplianceDatabase) ListViolationRecords(ctx context.Context, options ListOptions) ([]*ViolationRecord, int64, error) {
	panic("not implemented - TDD implementation needed")
}

func (db *ComplianceDatabase) EnableRLSPolicies(ctx context.Context) error {
	panic("not implemented - TDD implementation needed")
}

func (db *ComplianceDatabase) SetUserRole(ctx context.Context, role, userID string) context.Context {
	panic("not implemented - TDD implementation needed")
}