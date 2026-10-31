package database

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/isaacmain254/notification-system/internal/database/sqlc"
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

func (r *Repository) GetEvent(ctx context.Context, id pgtype.UUID) (sqlc.Event, error) {
	return r.queries.GetEventByID(ctx, id)
}

func (r *Repository) SetEventStatus(ctx context.Context, id pgtype.UUID, status string) (int64, error) {
	return r.queries.SetEventStatus(ctx, sqlc.SetEventStatusParams{ID: id, Status: status})
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
