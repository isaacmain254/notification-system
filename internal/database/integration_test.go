//go:build integration

package database_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/isaacmain254/notification-system/internal/database"
	"github.com/isaacmain254/notification-system/internal/database/sqlc"
	"github.com/isaacmain254/notification-system/internal/events"
)

func TestPostgresRepositories(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to a dedicated database with migrations applied")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("create test pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping test database: %v", err)
	}
	repo := database.NewRepository(pool)
	suffix := fmt.Sprintf("integration-%d", time.Now().UnixNano())

	app, err := repo.CreateApplication(ctx, "integration app", suffix)
	if err != nil {
		t.Fatalf("create application: %v", err)
	}
	t.Cleanup(func() { cleanupApplication(pool, app.ID) })
	if _, err := repo.GetActiveApplicationByAPIKeyHash(ctx, suffix); err != nil {
		t.Fatalf("get application by key hash: %v", err)
	}

	recipient, err := repo.UpsertRecipient(ctx, sqlc.UpsertRecipientParams{
		ApplicationID: app.ID, ExternalUserID: suffix,
		Email: pgtype.Text{String: "test@example.invalid", Valid: true},
	})
	if err != nil {
		t.Fatalf("upsert recipient: %v", err)
	}
	if _, err := repo.GetRecipient(ctx, app.ID, suffix); err != nil {
		t.Fatalf("get recipient: %v", err)
	}

	event, err := repo.CreateEvent(ctx, sqlc.CreateEventParams{
		ApplicationID: app.ID, RecipientID: recipient.ID, EventType: "test.event",
		IdempotencyKey: suffix, Payload: []byte(`{"test":true}`),
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	if _, err := repo.CreateEvent(ctx, sqlc.CreateEventParams{
		ApplicationID: app.ID, RecipientID: recipient.ID, EventType: "test.event",
		IdempotencyKey: suffix, Payload: []byte(`{"test":true}`),
	}); err == nil {
		t.Fatal("duplicate event idempotency key unexpectedly succeeded")
	}

	template, err := repo.CreateTemplate(ctx, sqlc.CreateTemplateParams{
		ApplicationID: app.ID, EventType: "test.event", Channel: "email",
		Subject: pgtype.Text{String: "Hello", Valid: true}, Body: "body", IsActive: true,
	})
	if err != nil {
		t.Fatalf("create template: %v", err)
	}
	if got, err := repo.ListActiveTemplates(ctx, app.ID, template.EventType); err != nil || len(got) != 1 {
		t.Fatalf("list templates = %d, error = %v; want one template", len(got), err)
	}

	notification, err := repo.CreateNotification(ctx, sqlc.CreateNotificationParams{
		EventID: event.ID, RecipientID: recipient.ID, Channel: "email", Status: "pending",
		RenderedSubject: pgtype.Text{String: "Hello", Valid: true}, RenderedBody: "body",
	})
	if err != nil {
		t.Fatalf("create notification: %v", err)
	}
	if _, err := repo.CreateNotification(ctx, sqlc.CreateNotificationParams{
		EventID: event.ID, RecipientID: recipient.ID, Channel: "email", Status: "pending",
		RenderedSubject: pgtype.Text{String: "Hello", Valid: true}, RenderedBody: "body",
	}); err == nil {
		t.Fatal("duplicate event/channel notification unexpectedly succeeded")
	}
	if rows, err := repo.MarkNotificationRead(ctx, notification.ID, recipient.ID); err != nil || rows != 1 {
		t.Fatalf("mark notification read affected %d rows, error = %v", rows, err)
	}
	if _, err := repo.CreateDeliveryAttempt(ctx, sqlc.CreateDeliveryAttemptParams{
		NotificationID: notification.ID, AttemptNumber: 1, Status: "failed",
		ErrorMessage: pgtype.Text{String: "simulated", Valid: true},
	}); err != nil {
		t.Fatalf("create delivery attempt: %v", err)
	}
	if attempts, err := repo.ListDeliveryAttempts(ctx, notification.ID); err != nil || len(attempts) != 1 {
		t.Fatalf("list attempts = %d, error = %v; want one attempt", len(attempts), err)
	}

	outbox, err := repo.EnqueueOutboxMessage(ctx, sqlc.EnqueueOutboxMessageParams{
		MessageType: "test.event", AggregateID: event.ID, RoutingKey: "event.test",
		Payload: []byte(`{"event_id":"test"}`),
	})
	if err != nil {
		t.Fatalf("enqueue outbox message: %v", err)
	}
	claimed, err := repo.ClaimOutboxMessages(ctx, 10, 30)
	if err != nil {
		t.Fatalf("claim outbox messages: %v", err)
	}
	found := false
	for _, message := range claimed {
		if message.ID.Bytes == outbox.ID.Bytes {
			found = true
		}
	}
	if !found {
		t.Fatal("new outbox message was not claimed")
	}
	if rows, err := repo.MarkOutboxMessagePublished(ctx, outbox.ID); err != nil || rows != 1 {
		t.Fatalf("mark outbox message published affected %d rows, error = %v", rows, err)
	}
}

func TestConcurrentEventIngestionIsAtomicAndIdempotent(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to a dedicated database with migrations applied")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("create test pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping test database: %v", err)
	}
	repo := database.NewRepository(pool)
	suffix := fmt.Sprintf("concurrent-%d", time.Now().UnixNano())
	app, err := repo.CreateApplication(ctx, "concurrency test", suffix)
	if err != nil {
		t.Fatalf("create application: %v", err)
	}
	t.Cleanup(func() { cleanupApplication(pool, app.ID) })

	command := events.PersistCommand{
		EventType: "test.concurrent", IdempotencyKey: suffix, RequestHash: "request-hash-1",
		Recipient: events.Recipient{ExternalUserID: suffix}, DataJSON: []byte(`{"value":1}`),
	}
	const callers = 12
	start := make(chan struct{})
	ids := make(chan string, callers)
	errCh := make(chan error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			id, err := repo.PersistEvent(ctx, app.ID, command)
			if err != nil {
				errCh <- err
				return
			}
			ids <- id
		}()
	}
	close(start)
	wg.Wait()
	close(ids)
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent PersistEvent(): %v", err)
	}
	var eventID string
	for id := range ids {
		if eventID == "" {
			eventID = id
		} else if id != eventID {
			t.Fatalf("concurrent calls returned different event IDs: %q and %q", eventID, id)
		}
	}
	var eventCount, outboxCount int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM events WHERE application_id = $1 AND idempotency_key = $2", app.ID, suffix).Scan(&eventCount); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM outbox_messages o JOIN events e ON e.id = o.aggregate_id WHERE e.application_id = $1 AND e.idempotency_key = $2`, app.ID, suffix).Scan(&outboxCount); err != nil {
		t.Fatalf("count outbox messages: %v", err)
	}
	if eventCount != 1 || outboxCount != 1 {
		t.Fatalf("persisted event/outbox counts = %d/%d, want 1/1", eventCount, outboxCount)
	}

	conflicting := command
	conflicting.RequestHash = "request-hash-2"
	conflicting.Recipient.ExternalUserID = suffix + "-should-rollback"
	if _, err := repo.PersistEvent(ctx, app.ID, conflicting); !errors.Is(err, events.ErrIdempotencyConflict) {
		t.Fatalf("changed request error = %v, want idempotency conflict", err)
	}
	var recipientCount int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM recipients WHERE application_id = $1 AND external_user_id = $2", app.ID, conflicting.Recipient.ExternalUserID).Scan(&recipientCount); err != nil {
		t.Fatalf("count rolled-back recipient: %v", err)
	}
	if recipientCount != 0 {
		t.Fatalf("mismatched replay left %d recipient rows; want transaction rollback", recipientCount)
	}
}

func cleanupApplication(pool *pgxpool.Pool, applicationID pgtype.UUID) {
	ctx := context.Background()
	_, _ = pool.Exec(ctx, `DELETE FROM outbox_messages WHERE aggregate_id IN (SELECT id FROM events WHERE application_id = $1)`, applicationID)
	_, _ = pool.Exec(ctx, "DELETE FROM applications WHERE id = $1", applicationID)
}
