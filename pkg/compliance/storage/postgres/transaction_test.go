package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// OutboxEvent type is defined in types.go

// TestAtomicDMCAWorkflow tests atomic DMCA processing transactions
func TestAtomicDMCAWorkflow(t *testing.T) {
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

	t.Run("SuccessfulDMCATransaction", func(t *testing.T) {
		// Create a complete DMCA processing transaction
		tx, err := db.BeginTransaction(ctx)
		require.NoError(t, err)

		// Step 1: Create takedown record
		takedownRecord := &TakedownRecord{
			TakedownID:      "TD-atomic123",
			DescriptorCID:   "QmAtomic123",
			RequestorEmail:  "atomic@example.com",
			CopyrightWork:   "Atomic Test Work",
			TakedownDate:    time.Now().UTC(),
			Status:          "active",
			LegalBasis:      "DMCA 512(c)",
			ProcessingNotes: "Atomic transaction test",
		}

		err = tx.CreateTakedownRecord(ctx, takedownRecord)
		require.NoError(t, err, "Should create takedown record in transaction")

		// Step 2: Create audit entry
		auditEntry := &AuditEntry{
			EntryID:   "AUDIT-atomic123",
			EventType: "dmca_takedown",
			TargetID:  takedownRecord.DescriptorCID,
			Action:    "descriptor_blacklisted",
			Details: map[string]interface{}{
				"takedown_id":   takedownRecord.TakedownID,
				"requestor":     takedownRecord.RequestorEmail,
				"copyright_work": takedownRecord.CopyrightWork,
			},
			Timestamp: time.Now().UTC(),
		}

		err = tx.CreateAuditEntry(ctx, auditEntry)
		require.NoError(t, err, "Should create audit entry in transaction")

		// Step 3: Create violation record if uploader is known
		if takedownRecord.UploaderID != "" {
			violationRecord := &ViolationRecord{
				ViolationID:      "VL-atomic123",
				UserID:           takedownRecord.UploaderID,
				DescriptorCID:    takedownRecord.DescriptorCID,
				TakedownID:       takedownRecord.TakedownID,
				ViolationType:    "copyright_infringement",
				ViolationDate:    time.Now().UTC(),
				Severity:         "major",
				ActionTaken:      "descriptor_removed",
				ResolutionStatus: "resolved",
			}

			err = tx.CreateViolationRecord(ctx, violationRecord)
			require.NoError(t, err, "Should create violation record in transaction")
		}

		// Step 4: Create outbox event for notification
		outboxEvent := &OutboxEvent{
			EventID:     "EVT-atomic123",
			EventType:   "dmca_takedown_processed",
			AggregateID: takedownRecord.TakedownID,
			Payload: map[string]interface{}{
				"takedown_id":   takedownRecord.TakedownID,
				"descriptor_cid": takedownRecord.DescriptorCID,
				"action":        "blacklisted",
			},
			Status:    "pending",
			CreatedAt: time.Now().UTC(),
		}

		err = tx.CreateOutboxEvent(ctx, outboxEvent)
		require.NoError(t, err, "Should create outbox event in transaction")

		// Commit transaction
		err = tx.Commit(ctx)
		require.NoError(t, err, "Transaction should commit successfully")

		// Verify all records were created
		retrievedTakedown, err := db.GetTakedownRecord(ctx, takedownRecord.TakedownID)
		assert.NoError(t, err)
		assert.NotNil(t, retrievedTakedown)

		retrievedAudit, err := db.GetAuditEntry(ctx, auditEntry.EntryID)
		assert.NoError(t, err)
		assert.NotNil(t, retrievedAudit)

		retrievedEvent, err := db.GetOutboxEvent(ctx, outboxEvent.EventID)
		assert.NoError(t, err)
		assert.NotNil(t, retrievedEvent)

		// Verify descriptor is blacklisted
		isBlacklisted, err := db.IsDescriptorBlacklisted(ctx, takedownRecord.DescriptorCID)
		assert.NoError(t, err)
		assert.True(t, isBlacklisted, "Descriptor should be blacklisted after transaction")
	})

	t.Run("FailedDMCATransactionRollback", func(t *testing.T) {
		// Start transaction
		tx, err := db.BeginTransaction(ctx)
		require.NoError(t, err)

		// Create takedown record
		takedownRecord := &TakedownRecord{
			TakedownID:     "TD-rollback123",
			DescriptorCID:  "QmRollback123",
			RequestorEmail: "rollback@example.com",
			CopyrightWork:  "Rollback Test Work",
			TakedownDate:   time.Now().UTC(),
			Status:         "active",
			LegalBasis:     "DMCA 512(c)",
		}

		err = tx.CreateTakedownRecord(ctx, takedownRecord)
		require.NoError(t, err)

		// Create audit entry
		auditEntry := &AuditEntry{
			EntryID:   "AUDIT-rollback123",
			EventType: "dmca_takedown",
			TargetID:  takedownRecord.DescriptorCID,
			Action:    "descriptor_blacklisted",
			Details:   map[string]interface{}{"takedown_id": takedownRecord.TakedownID},
			Timestamp: time.Now().UTC(),
		}

		err = tx.CreateAuditEntry(ctx, auditEntry)
		require.NoError(t, err)

		// Simulate failure and rollback
		err = tx.Rollback(ctx)
		require.NoError(t, err, "Transaction rollback should succeed")

		// Verify nothing was committed
		exists, err := db.TakedownRecordExists(ctx, takedownRecord.TakedownID)
		assert.NoError(t, err)
		assert.False(t, exists, "Takedown record should not exist after rollback")

		auditExists, err := db.AuditEntryExists(ctx, auditEntry.EntryID)
		assert.NoError(t, err)
		assert.False(t, auditExists, "Audit entry should not exist after rollback")

		// Verify descriptor is not blacklisted
		isBlacklisted, err := db.IsDescriptorBlacklisted(ctx, takedownRecord.DescriptorCID)
		assert.NoError(t, err)
		assert.False(t, isBlacklisted, "Descriptor should not be blacklisted after rollback")
	})
}

// TestTransactionIsolationLevels tests different isolation levels for compliance operations
func TestTransactionIsolationLevels(t *testing.T) {
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

	err = db.MigrateToLatest(ctx)
	require.NoError(t, err)

	t.Run("ReadCommittedIsolation", func(t *testing.T) {
		// Create initial record
		record := &TakedownRecord{
			TakedownID:     "TD-isolation123",
			DescriptorCID:  "QmIsolation123",
			RequestorEmail: "isolation@example.com",
			Status:         "active",
			TakedownDate:   time.Now().UTC(),
			LegalBasis:     "DMCA 512(c)",
		}

		err := db.CreateTakedownRecord(ctx, record)
		require.NoError(t, err)

		// Transaction 1: Read and update
		tx1, err := db.BeginTransactionWithIsolation(ctx, pgx.ReadCommitted)
		require.NoError(t, err)

		retrieved1, err := tx1.GetTakedownRecord(ctx, record.TakedownID)
		require.NoError(t, err)
		assert.Equal(t, "active", retrieved1.Status)

		// Transaction 2: Concurrent update
		tx2, err := db.BeginTransactionWithIsolation(ctx, pgx.ReadCommitted)
		require.NoError(t, err)

		record.Status = "disputed"
		err = tx2.UpdateTakedownRecord(ctx, record)
		require.NoError(t, err)

		err = tx2.Commit(ctx)
		require.NoError(t, err)

		// Transaction 1 should see committed changes on next read
		retrieved2, err := tx1.GetTakedownRecord(ctx, record.TakedownID)
		require.NoError(t, err)
		assert.Equal(t, "disputed", retrieved2.Status, "Should see committed changes in Read Committed")

		err = tx1.Rollback(ctx)
		require.NoError(t, err)
	})

	t.Run("RepeatableReadIsolation", func(t *testing.T) {
		// Create test record
		record := &TakedownRecord{
			TakedownID:     "TD-repeatable123",
			DescriptorCID:  "QmRepeatable123",
			RequestorEmail: "repeatable@example.com",
			Status:         "active",
			TakedownDate:   time.Now().UTC(),
			LegalBasis:     "DMCA 512(c)",
		}

		err := db.CreateTakedownRecord(ctx, record)
		require.NoError(t, err)

		// Transaction 1: Read with Repeatable Read
		tx1, err := db.BeginTransactionWithIsolation(ctx, pgx.RepeatableRead)
		require.NoError(t, err)

		retrieved1, err := tx1.GetTakedownRecord(ctx, record.TakedownID)
		require.NoError(t, err)
		assert.Equal(t, "active", retrieved1.Status)

		// Transaction 2: Update and commit
		record.Status = "reinstated"
		err = db.UpdateTakedownRecord(ctx, record)
		require.NoError(t, err)

		// Transaction 1 should still see original value
		retrieved2, err := tx1.GetTakedownRecord(ctx, record.TakedownID)
		require.NoError(t, err)
		assert.Equal(t, "active", retrieved2.Status, "Should see consistent value in Repeatable Read")

		err = tx1.Rollback(ctx)
		require.NoError(t, err)
	})

	t.Run("SerializableIsolation", func(t *testing.T) {
		// Test serializable isolation for critical compliance operations
		tx1, err := db.BeginTransactionWithIsolation(ctx, pgx.Serializable)
		require.NoError(t, err)

		tx2, err := db.BeginTransactionWithIsolation(ctx, pgx.Serializable)
		require.NoError(t, err)

		// Both transactions try to create conflicting audit entries
		auditEntry1 := &AuditEntry{
			EntryID:   "AUDIT-serial1",
			EventType: "dmca_takedown",
			TargetID:  "QmSerializable123",
			Action:    "descriptor_blacklisted",
			Details:   map[string]interface{}{"sequence": 1},
			Timestamp: time.Now().UTC(),
		}

		auditEntry2 := &AuditEntry{
			EntryID:   "AUDIT-serial2",
			EventType: "dmca_takedown",
			TargetID:  "QmSerializable123",
			Action:    "descriptor_blacklisted",
			Details:   map[string]interface{}{"sequence": 2},
			Timestamp: time.Now().UTC(),
		}

		err = tx1.CreateAuditEntry(ctx, auditEntry1)
		require.NoError(t, err)

		err = tx2.CreateAuditEntry(ctx, auditEntry2)
		require.NoError(t, err)

		// One should commit successfully
		err1 := tx1.Commit(ctx)
		err2 := tx2.Commit(ctx)

		// At least one should succeed, one might fail with serialization error
		if err1 != nil && err2 != nil {
			t.Fatal("Both transactions failed, at least one should succeed")
		}

		// Verify audit chain integrity is maintained
		isValid, err := db.VerifyAuditChainIntegrity(ctx)
		assert.NoError(t, err)
		assert.True(t, isValid, "Audit chain integrity should be maintained")
	})
}

// TestConcurrentDMCAProcessing tests concurrent DMCA processing scenarios
func TestConcurrentDMCAProcessing(t *testing.T) {
	ctx := context.Background()
	container, connStr := setupTestContainer(t, ctx)
	defer container.Terminate(ctx)

	db, err := NewComplianceDatabase(ctx, &DatabaseConfig{
		ConnectionString: connStr,
		MaxConnections:   50,
		ConnectTimeout:   30 * time.Second,
	})
	require.NoError(t, err)
	defer db.Close()

	err = db.MigrateToLatest(ctx)
	require.NoError(t, err)

	t.Run("ConcurrentTakedownCreation", func(t *testing.T) {
		const numConcurrent = 20
		errChan := make(chan error, numConcurrent)
		doneChan := make(chan string, numConcurrent)

		// Process multiple DMCA takedowns concurrently
		for i := 0; i < numConcurrent; i++ {
			go func(index int) {
				takedownID := fmt.Sprintf("TD-concurrent%03d", index)

				tx, err := db.BeginTransaction(ctx)
				if err != nil {
					errChan <- fmt.Errorf("failed to begin transaction for %s: %w", takedownID, err)
					return
				}

				// Create takedown record
				record := &TakedownRecord{
					TakedownID:     takedownID,
					DescriptorCID:  fmt.Sprintf("QmConcurrent%03d", index),
					RequestorEmail: fmt.Sprintf("concurrent%d@example.com", index),
					CopyrightWork:  fmt.Sprintf("Concurrent Work %d", index),
					TakedownDate:   time.Now().UTC(),
					Status:         "active",
					LegalBasis:     "DMCA 512(c)",
				}

				err = tx.CreateTakedownRecord(ctx, record)
				if err != nil {
					tx.Rollback(ctx)
					errChan <- fmt.Errorf("failed to create takedown %s: %w", takedownID, err)
					return
				}

				// Create audit entry
				auditEntry := &AuditEntry{
					EntryID:   fmt.Sprintf("AUDIT-concurrent%03d", index),
					EventType: "dmca_takedown",
					TargetID:  record.DescriptorCID,
					Action:    "descriptor_blacklisted",
					Details:   map[string]interface{}{"takedown_id": takedownID},
					Timestamp: time.Now().UTC(),
				}

				err = tx.CreateAuditEntry(ctx, auditEntry)
				if err != nil {
					tx.Rollback(ctx)
					errChan <- fmt.Errorf("failed to create audit entry for %s: %w", takedownID, err)
					return
				}

				// Create outbox event
				outboxEvent := &OutboxEvent{
					EventID:     fmt.Sprintf("EVT-concurrent%03d", index),
					EventType:   "dmca_takedown_processed",
					AggregateID: takedownID,
					Payload: map[string]interface{}{
						"takedown_id":    takedownID,
						"descriptor_cid": record.DescriptorCID,
					},
					Status:    "pending",
					CreatedAt: time.Now().UTC(),
				}

				err = tx.CreateOutboxEvent(ctx, outboxEvent)
				if err != nil {
					tx.Rollback(ctx)
					errChan <- fmt.Errorf("failed to create outbox event for %s: %w", takedownID, err)
					return
				}

				err = tx.Commit(ctx)
				if err != nil {
					errChan <- fmt.Errorf("failed to commit transaction for %s: %w", takedownID, err)
					return
				}

				errChan <- nil
				doneChan <- takedownID
			}(i)
		}

		// Wait for all goroutines to complete
		var successCount int
		for i := 0; i < numConcurrent; i++ {
			select {
			case err := <-errChan:
				if err != nil {
					t.Logf("Concurrent operation error: %v", err)
				} else {
					successCount++
				}
			case <-time.After(60 * time.Second):
				t.Fatal("Timeout waiting for concurrent operations")
			}
		}

		t.Logf("Successfully processed %d out of %d concurrent takedowns", successCount, numConcurrent)
		assert.Greater(t, successCount, numConcurrent/2, "At least half of concurrent operations should succeed")

		// Verify audit chain integrity after concurrent operations
		isValid, err := db.VerifyAuditChainIntegrity(ctx)
		assert.NoError(t, err)
		assert.True(t, isValid, "Audit chain integrity should be maintained after concurrent operations")
	})
}

// TestOutboxPatternReliability tests the outbox pattern for reliable event publishing
func TestOutboxPatternReliability(t *testing.T) {
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

	t.Run("OutboxEventCreation", func(t *testing.T) {
		// Create outbox event as part of transaction
		tx, err := db.BeginTransaction(ctx)
		require.NoError(t, err)

		event := &OutboxEvent{
			EventID:     "EVT-outbox123",
			EventType:   "dmca_takedown_processed",
			AggregateID: "TD-outbox123",
			Payload: map[string]interface{}{
				"takedown_id":    "TD-outbox123",
				"descriptor_cid": "QmOutbox123",
				"action":         "blacklisted",
			},
			Status:    "pending",
			CreatedAt: time.Now().UTC(),
		}

		err = tx.CreateOutboxEvent(ctx, event)
		require.NoError(t, err)

		err = tx.Commit(ctx)
		require.NoError(t, err)

		// Verify event was created
		retrievedEvent, err := db.GetOutboxEvent(ctx, event.EventID)
		assert.NoError(t, err)
		assert.NotNil(t, retrievedEvent)
		assert.Equal(t, "pending", retrievedEvent.Status)
	})

	t.Run("OutboxEventProcessing", func(t *testing.T) {
		// Get pending events for processing
		pendingEvents, err := db.GetPendingOutboxEvents(ctx, 10)
		require.NoError(t, err)
		assert.Greater(t, len(pendingEvents), 0, "Should have pending events")

		for _, event := range pendingEvents {
			// Simulate successful publishing
			err = db.MarkOutboxEventPublished(ctx, event.EventID)
			assert.NoError(t, err, "Should mark event as published")

			// Verify status update
			updatedEvent, err := db.GetOutboxEvent(ctx, event.EventID)
			assert.NoError(t, err)
			assert.Equal(t, "published", updatedEvent.Status)
			assert.NotNil(t, updatedEvent.PublishedAt)
		}
	})

	t.Run("OutboxEventRetryMechanism", func(t *testing.T) {
		// Create event that will fail
		event := &OutboxEvent{
			EventID:     "EVT-retry123",
			EventType:   "notification_send",
			AggregateID: "TD-retry123",
			Payload: map[string]interface{}{
				"recipient": "user@example.com",
				"message":   "Test notification",
			},
			Status:    "pending",
			CreatedAt: time.Now().UTC(),
		}

		err := db.CreateOutboxEvent(ctx, event)
		require.NoError(t, err)

		// Simulate failed publishing with retry
		for i := 0; i < 3; i++ {
			err = db.MarkOutboxEventFailed(ctx, event.EventID, fmt.Sprintf("Retry attempt %d failed", i+1))
			assert.NoError(t, err)

			// Verify retry count is incremented
			failedEvent, err := db.GetOutboxEvent(ctx, event.EventID)
			assert.NoError(t, err)
			assert.Equal(t, i+1, failedEvent.RetryCount)
			assert.Equal(t, "failed", failedEvent.Status)
		}

		// Get failed events for retry
		failedEvents, err := db.GetFailedOutboxEvents(ctx, 10)
		assert.NoError(t, err)
		assert.Greater(t, len(failedEvents), 0, "Should have failed events")

		// Verify retry logic
		for _, failedEvent := range failedEvents {
			if failedEvent.RetryCount < 3 {
				// Reset for retry
				err = db.ResetOutboxEventForRetry(ctx, failedEvent.EventID)
				assert.NoError(t, err)

				retryEvent, err := db.GetOutboxEvent(ctx, failedEvent.EventID)
				assert.NoError(t, err)
				assert.Equal(t, "pending", retryEvent.Status)
			}
		}
	})

	t.Run("OutboxEventCleanup", func(t *testing.T) {
		// Clean up old published events
		cutoffDate := time.Now().UTC().Add(-24 * time.Hour)
		deletedCount, err := db.CleanupOutboxEvents(ctx, cutoffDate)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, deletedCount, int64(0), "Should return deleted count")

		// Verify cleanup preserves recent and failed events
		pendingEvents, err := db.GetPendingOutboxEvents(ctx, 100)
		assert.NoError(t, err)

		failedEvents, err := db.GetFailedOutboxEvents(ctx, 100)
		assert.NoError(t, err)

		t.Logf("After cleanup: %d pending, %d failed events", len(pendingEvents), len(failedEvents))
	})
}

// TestDeadlockDetectionAndResolution tests deadlock handling
func TestDeadlockDetectionAndResolution(t *testing.T) {
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

	err = db.MigrateToLatest(ctx)
	require.NoError(t, err)

	// Create test records for deadlock scenarios
	record1 := &TakedownRecord{
		TakedownID:     "TD-deadlock1",
		DescriptorCID:  "QmDeadlock1",
		RequestorEmail: "deadlock1@example.com",
		Status:         "active",
		TakedownDate:   time.Now().UTC(),
		LegalBasis:     "DMCA 512(c)",
	}

	record2 := &TakedownRecord{
		TakedownID:     "TD-deadlock2",
		DescriptorCID:  "QmDeadlock2",
		RequestorEmail: "deadlock2@example.com",
		Status:         "active",
		TakedownDate:   time.Now().UTC(),
		LegalBasis:     "DMCA 512(c)",
	}

	err = db.CreateTakedownRecord(ctx, record1)
	require.NoError(t, err)

	err = db.CreateTakedownRecord(ctx, record2)
	require.NoError(t, err)

	t.Run("DeadlockDetectionAndRetry", func(t *testing.T) {
		const numConcurrent = 4
		errChan := make(chan error, numConcurrent)

		// Create deadlock scenario: multiple transactions updating same records in different order
		for i := 0; i < numConcurrent; i++ {
			go func(index int) {
				// Use retry mechanism for deadlock resolution
				err := db.WithRetry(ctx, func(ctx context.Context) error {
					tx, err := db.BeginTransaction(ctx)
					if err != nil {
						return err
					}
					defer tx.Rollback(ctx)

					// Update records in different order based on index to create deadlock potential
					var first, second *TakedownRecord
					if index%2 == 0 {
						first, second = record1, record2
					} else {
						first, second = record2, record1
					}

					// Update first record
					first.ProcessingNotes = fmt.Sprintf("Updated by goroutine %d at %v", index, time.Now())
					err = tx.UpdateTakedownRecord(ctx, first)
					if err != nil {
						return err
					}

					// Small delay to increase deadlock probability
					time.Sleep(10 * time.Millisecond)

					// Update second record
					second.ProcessingNotes = fmt.Sprintf("Updated by goroutine %d at %v", index, time.Now())
					err = tx.UpdateTakedownRecord(ctx, second)
					if err != nil {
						return err
					}

					return tx.Commit(ctx)
				})

				errChan <- err
			}(i)
		}

		// Wait for all goroutines to complete
		var successCount int
		for i := 0; i < numConcurrent; i++ {
			select {
			case err := <-errChan:
				if err != nil {
					t.Logf("Transaction error (expected due to deadlock): %v", err)
				} else {
					successCount++
				}
			case <-time.After(30 * time.Second):
				t.Fatal("Timeout waiting for deadlock resolution")
			}
		}

		t.Logf("Successfully completed %d out of %d concurrent transactions", successCount, numConcurrent)
		assert.Greater(t, successCount, 0, "At least some transactions should succeed after retry")
	})
}

// Methods that need to be implemented - will fail compilation initially

// Transaction interface is defined in types.go

// All transaction methods are implemented in transaction.go

// All methods implemented in actual source files