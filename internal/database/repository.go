package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/isaacmain254/notification-system/internal/database/sqlc"
	"github.com/isaacmain254/notification-system/internal/events"
)

// Repository exposes application-oriented persistence operations backed by sqlc.
type Repository struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, queries: sqlc.New(pool)}
}

func (r *Repository) CreateApplication(ctx context.Context, name, apiKeyHash string) (sqlc.Application, error) {
	return r.queries.CreateApplication(ctx, sqlc.CreateApplicationParams{Name: name, ApiKeyHash: apiKeyHash})
}

func (r *Repository) GetApplication(ctx context.Context, id pgtype.UUID) (sqlc.Application, error) {
	return r.queries.GetApplicationByID(ctx, id)
}

func (r *Repository) GetActiveApplicationByAPIKeyHash(ctx context.Context, hash string) (sqlc.Application, error) {
	return r.queries.GetActiveApplicationByAPIKeyHash(ctx, hash)
}

func (r *Repository) LookupActiveApplicationID(ctx context.Context, hash string) (pgtype.UUID, error) {
	app, err := r.GetActiveApplicationByAPIKeyHash(ctx, hash)
	return app.ID, err
}

func (r *Repository) UpsertRecipient(ctx context.Context, arg sqlc.UpsertRecipientParams) (sqlc.Recipient, error) {
	return r.queries.UpsertRecipient(ctx, arg)
}

func (r *Repository) GetRecipient(ctx context.Context, applicationID pgtype.UUID, externalUserID string) (sqlc.Recipient, error) {
	return r.queries.GetRecipientByExternalID(ctx, sqlc.GetRecipientByExternalIDParams{
		ApplicationID: applicationID, ExternalUserID: externalUserID,
	})
}

func (r *Repository) CreateEvent(ctx context.Context, arg sqlc.CreateEventParams) (sqlc.Event, error) {
	return r.queries.CreateEvent(ctx, arg)
}

func (r *Repository) PersistEvent(ctx context.Context, applicationID pgtype.UUID, command events.PersistCommand) (string, error) {
	var eventID string
	errReplay := errors.New("idempotent event replay")
	err := r.WithTx(ctx, func(txRepo *Repository) error {
		recipient, err := txRepo.UpsertRecipient(ctx, sqlc.UpsertRecipientParams{
			ApplicationID:  applicationID,
			ExternalUserID: command.Recipient.ExternalUserID,
			Email:          nullableText(command.Recipient.Email),
			Phone:          nullableText(command.Recipient.Phone),
		})
		if err != nil {
			return fmt.Errorf("upsert event recipient: %w", err)
		}

		event, err := txRepo.queries.InsertEventIfAbsent(ctx, sqlc.InsertEventIfAbsentParams{
			ApplicationID: applicationID, RecipientID: recipient.ID,
			EventType: command.EventType, IdempotencyKey: command.IdempotencyKey,
			Payload: command.DataJSON, RequestHash: command.RequestHash,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			existing, lookupErr := txRepo.queries.GetEventByIdempotencyKey(ctx, sqlc.GetEventByIdempotencyKeyParams{
				ApplicationID: applicationID, IdempotencyKey: command.IdempotencyKey,
			})
			if lookupErr != nil {
				return fmt.Errorf("load event for idempotency check: %w", lookupErr)
			}
			if existing.RequestHash != command.RequestHash {
				return events.ErrIdempotencyConflict
			}
			eventID = uuidString(existing.ID)
			return errReplay // Roll back recipient timestamp or any incidental changes on replay.
		}
		if err != nil {
			return fmt.Errorf("insert event: %w", err)
		}

		message, err := json.Marshal(struct {
			Version       int             `json:"version"`
			EventID       string          `json:"event_id"`
			ApplicationID string          `json:"application_id"`
			EventType     string          `json:"event_type"`
			RecipientID   string          `json:"recipient_id"`
			OccurredAt    time.Time       `json:"occurred_at"`
			Data          json.RawMessage `json:"data"`
		}{
			Version: 1, EventID: uuidString(event.ID), ApplicationID: uuidString(applicationID),
			EventType: event.EventType, RecipientID: uuidString(recipient.ID),
			OccurredAt: event.CreatedAt.Time, Data: command.DataJSON,
		})
		if err != nil {
			return fmt.Errorf("encode event outbox message: %w", err)
		}
		if _, err := txRepo.EnqueueOutboxMessage(ctx, sqlc.EnqueueOutboxMessageParams{
			MessageType: "event.received", AggregateID: event.ID,
			RoutingKey: "event." + event.EventType, Payload: message,
		}); err != nil {
			return fmt.Errorf("enqueue event outbox message: %w", err)
		}
		eventID = uuidString(event.ID)
		return nil
	})
	if errors.Is(err, errReplay) {
		return eventID, nil
	}
	if err != nil {
		return "", err
	}
	return eventID, nil
}

func nullableText(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: value != ""}
}

func uuidString(value pgtype.UUID) string {
	b := value.Bytes
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func (r *Repository) GetEvent(ctx context.Context, id pgtype.UUID) (sqlc.Event, error) {
	return r.queries.GetEventByID(ctx, id)
}

func (r *Repository) SetEventStatus(ctx context.Context, id pgtype.UUID, status string) (int64, error) {
	return r.queries.SetEventStatus(ctx, sqlc.SetEventStatusParams{ID: id, Status: status})
}

// BeginEventProcessing atomically moves a pending event into processing. A
// duplicate delivery for an event that is already processing or processed is
// treated as successfully handled without changing its state.
func (r *Repository) BeginEventProcessing(ctx context.Context, eventID string) (bool, error) {
	var id pgtype.UUID
	if err := id.Scan(eventID); err != nil {
		return false, fmt.Errorf("parse event ID: %w", err)
	}
	var transitioned bool
	err := r.pool.QueryRow(ctx, `
		UPDATE events SET status = 'processing'
		WHERE id = $1 AND status = 'pending'
		RETURNING true
	`, id).Scan(&transitioned)
	if err == nil {
		return transitioned, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Errorf("transition event to processing: %w", err)
	}
	var status string
	if err := r.pool.QueryRow(ctx, `SELECT status FROM events WHERE id = $1`, id).Scan(&status); err != nil {
		return false, fmt.Errorf("read event processing status: %w", err)
	}
	if status == "processing" || status == "processed" {
		return false, nil
	}
	return false, fmt.Errorf("event %s is in unexpected status %q", eventID, status)
}

func (r *Repository) CreateTemplate(ctx context.Context, arg sqlc.CreateTemplateParams) (sqlc.Template, error) {
	return r.queries.CreateTemplate(ctx, arg)
}

func (r *Repository) ListActiveTemplates(ctx context.Context, applicationID pgtype.UUID, eventType string) ([]sqlc.Template, error) {
	return r.queries.ListActiveTemplatesForEvent(ctx, sqlc.ListActiveTemplatesForEventParams{
		ApplicationID: applicationID, EventType: eventType,
	})
}

func (r *Repository) CreateNotification(ctx context.Context, arg sqlc.CreateNotificationParams) (sqlc.Notification, error) {
	return r.queries.CreateNotification(ctx, arg)
}

func (r *Repository) ListNotifications(ctx context.Context, recipientID pgtype.UUID, limit, offset int32) ([]sqlc.Notification, error) {
	return r.queries.ListNotificationsForRecipient(ctx, sqlc.ListNotificationsForRecipientParams{
		RecipientID: recipientID, Limit: limit, Offset: offset,
	})
}

func (r *Repository) MarkNotificationRead(ctx context.Context, notificationID, recipientID pgtype.UUID) (int64, error) {
	return r.queries.MarkNotificationRead(ctx, sqlc.MarkNotificationReadParams{
		ID: notificationID, RecipientID: recipientID,
	})
}

func (r *Repository) CreateDeliveryAttempt(ctx context.Context, arg sqlc.CreateDeliveryAttemptParams) (sqlc.DeliveryAttempt, error) {
	return r.queries.CreateDeliveryAttempt(ctx, arg)
}

func (r *Repository) ListDeliveryAttempts(ctx context.Context, notificationID pgtype.UUID) ([]sqlc.DeliveryAttempt, error) {
	return r.queries.ListDeliveryAttempts(ctx, notificationID)
}

func (r *Repository) EnqueueOutboxMessage(ctx context.Context, arg sqlc.EnqueueOutboxMessageParams) (sqlc.OutboxMessage, error) {
	return r.queries.EnqueueOutboxMessage(ctx, arg)
}

func (r *Repository) ClaimOutboxMessages(ctx context.Context, limit int32, leaseSeconds float64) ([]sqlc.OutboxMessage, error) {
	return r.queries.ClaimOutboxMessages(ctx, sqlc.ClaimOutboxMessagesParams{Limit: limit, Secs: leaseSeconds})
}

func (r *Repository) MarkOutboxMessagePublished(ctx context.Context, id pgtype.UUID) (int64, error) {
	return r.queries.MarkOutboxMessagePublished(ctx, id)
}

func (r *Repository) RecordOutboxPublishFailure(ctx context.Context, id pgtype.UUID, message string) (int64, error) {
	return r.queries.RecordOutboxPublishFailure(ctx, sqlc.RecordOutboxPublishFailureParams{
		ID: id, LastError: pgtype.Text{String: message, Valid: true},
	})
}

// WithTx runs fn with a repository whose queries participate in the same transaction.
func (r *Repository) WithTx(ctx context.Context, fn func(*Repository) error) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := fn(&Repository{pool: r.pool, queries: r.queries.WithTx(tx)}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
