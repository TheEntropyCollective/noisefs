package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// CreateOutboxEvent creates a new outbox event
func (db *ComplianceDatabase) CreateOutboxEvent(ctx context.Context, event *OutboxEvent) error {
	query := `
		INSERT INTO outbox_events (
			event_id, event_type, aggregate_id, payload, status, created_at,
			published_at, retry_count, last_error
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9
		)`

	_, err := db.pool.Exec(ctx, query,
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

// GetOutboxEvent retrieves an outbox event by ID
func (db *ComplianceDatabase) GetOutboxEvent(ctx context.Context, eventID string) (*OutboxEvent, error) {
	query := `
		SELECT event_id, event_type, aggregate_id, payload, status, created_at,
			   published_at, retry_count, last_error
		FROM outbox_events 
		WHERE event_id = $1`

	event := &OutboxEvent{}
	err := db.pool.QueryRow(ctx, query, eventID).Scan(
		&event.EventID,
		&event.EventType,
		&event.AggregateID,
		&event.Payload,
		&event.Status,
		&event.CreatedAt,
		&event.PublishedAt,
		&event.RetryCount,
		&event.LastError,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("outbox event not found: %s", eventID)
		}
		return nil, fmt.Errorf("failed to get outbox event: %w", err)
	}

	return event, nil
}

// GetPendingOutboxEvents retrieves pending outbox events for processing
func (db *ComplianceDatabase) GetPendingOutboxEvents(ctx context.Context, limit int) ([]*OutboxEvent, error) {
	query := `
		SELECT event_id, event_type, aggregate_id, payload, status, created_at,
			   published_at, retry_count, last_error
		FROM outbox_events 
		WHERE status = 'pending'
		ORDER BY created_at ASC
		LIMIT $1`

	rows, err := db.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending outbox events: %w", err)
	}
	defer rows.Close()

	var events []*OutboxEvent
	for rows.Next() {
		event := &OutboxEvent{}
		err := rows.Scan(
			&event.EventID,
			&event.EventType,
			&event.AggregateID,
			&event.Payload,
			&event.Status,
			&event.CreatedAt,
			&event.PublishedAt,
			&event.RetryCount,
			&event.LastError,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan outbox event: %w", err)
		}
		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating outbox events: %w", err)
	}

	return events, nil
}

// GetFailedOutboxEvents retrieves failed outbox events for retry
func (db *ComplianceDatabase) GetFailedOutboxEvents(ctx context.Context, limit int) ([]*OutboxEvent, error) {
	query := `
		SELECT event_id, event_type, aggregate_id, payload, status, created_at,
			   published_at, retry_count, last_error
		FROM outbox_events 
		WHERE status = 'failed' AND retry_count < 3
		ORDER BY created_at ASC
		LIMIT $1`

	rows, err := db.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query failed outbox events: %w", err)
	}
	defer rows.Close()

	var events []*OutboxEvent
	for rows.Next() {
		event := &OutboxEvent{}
		err := rows.Scan(
			&event.EventID,
			&event.EventType,
			&event.AggregateID,
			&event.Payload,
			&event.Status,
			&event.CreatedAt,
			&event.PublishedAt,
			&event.RetryCount,
			&event.LastError,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan outbox event: %w", err)
		}
		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating outbox events: %w", err)
	}

	return events, nil
}

// MarkOutboxEventPublished marks an outbox event as successfully published
func (db *ComplianceDatabase) MarkOutboxEventPublished(ctx context.Context, eventID string) error {
	query := `
		UPDATE outbox_events 
		SET status = 'published', published_at = NOW()
		WHERE event_id = $1`

	result, err := db.pool.Exec(ctx, query, eventID)
	if err != nil {
		return fmt.Errorf("failed to mark outbox event as published: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("outbox event not found: %s", eventID)
	}

	return nil
}

// MarkOutboxEventFailed marks an outbox event as failed with error message
func (db *ComplianceDatabase) MarkOutboxEventFailed(ctx context.Context, eventID string, errorMsg string) error {
	query := `
		UPDATE outbox_events 
		SET status = 'failed', 
			retry_count = retry_count + 1,
			last_error = $2
		WHERE event_id = $1`

	result, err := db.pool.Exec(ctx, query, eventID, errorMsg)
	if err != nil {
		return fmt.Errorf("failed to mark outbox event as failed: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("outbox event not found: %s", eventID)
	}

	return nil
}

// ResetOutboxEventForRetry resets a failed outbox event for retry
func (db *ComplianceDatabase) ResetOutboxEventForRetry(ctx context.Context, eventID string) error {
	query := `
		UPDATE outbox_events 
		SET status = 'pending', last_error = ''
		WHERE event_id = $1 AND status = 'failed'`

	result, err := db.pool.Exec(ctx, query, eventID)
	if err != nil {
		return fmt.Errorf("failed to reset outbox event for retry: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("outbox event not found or not in failed state: %s", eventID)
	}

	return nil
}

// CleanupOutboxEvents removes old published outbox events
func (db *ComplianceDatabase) CleanupOutboxEvents(ctx context.Context, cutoffDate time.Time) (int64, error) {
	query := `
		DELETE FROM outbox_events 
		WHERE status = 'published' AND published_at < $1`

	result, err := db.pool.Exec(ctx, query, cutoffDate)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup outbox events: %w", err)
	}

	return result.RowsAffected(), nil
}

// GetOutboxEventStats returns statistics about outbox events
func (db *ComplianceDatabase) GetOutboxEventStats(ctx context.Context) (*OutboxEventStats, error) {
	query := `
		SELECT 
			COUNT(CASE WHEN status = 'pending' THEN 1 END) as pending_count,
			COUNT(CASE WHEN status = 'published' THEN 1 END) as published_count,
			COUNT(CASE WHEN status = 'failed' THEN 1 END) as failed_count,
			COUNT(*) as total_count,
			AVG(CASE WHEN status = 'published' THEN EXTRACT(EPOCH FROM (published_at - created_at)) END) as avg_processing_time_seconds
		FROM outbox_events`

	stats := &OutboxEventStats{}
	var avgProcessingTime *float64

	err := db.pool.QueryRow(ctx, query).Scan(
		&stats.PendingCount,
		&stats.PublishedCount,
		&stats.FailedCount,
		&stats.TotalCount,
		&avgProcessingTime,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get outbox event stats: %w", err)
	}

	if avgProcessingTime != nil {
		stats.AvgProcessingTimeSeconds = *avgProcessingTime
	}

	return stats, nil
}

// OutboxEventStats provides statistics about outbox events
type OutboxEventStats struct {
	PendingCount              int64   `json:"pending_count"`
	PublishedCount            int64   `json:"published_count"`
	FailedCount               int64   `json:"failed_count"`
	TotalCount                int64   `json:"total_count"`
	AvgProcessingTimeSeconds  float64 `json:"avg_processing_time_seconds"`
}

// ProcessOutboxEvents processes pending outbox events in batch
func (db *ComplianceDatabase) ProcessOutboxEvents(ctx context.Context, batchSize int, processor func(*OutboxEvent) error) error {
	// Get pending events
	events, err := db.GetPendingOutboxEvents(ctx, batchSize)
	if err != nil {
		return fmt.Errorf("failed to get pending outbox events: %w", err)
	}

	// Process each event
	for _, event := range events {
		err := processor(event)
		if err != nil {
			// Mark as failed
			if markErr := db.MarkOutboxEventFailed(ctx, event.EventID, err.Error()); markErr != nil {
				return fmt.Errorf("failed to mark event as failed: %w", markErr)
			}
		} else {
			// Mark as published
			if markErr := db.MarkOutboxEventPublished(ctx, event.EventID); markErr != nil {
				return fmt.Errorf("failed to mark event as published: %w", markErr)
			}
		}
	}

	return nil
}