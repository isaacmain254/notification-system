//go:build integration

package http_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/isaacmain254/notification-system/internal/applications"
	"github.com/isaacmain254/notification-system/internal/database"
	"github.com/isaacmain254/notification-system/internal/events"
	apphttp "github.com/isaacmain254/notification-system/internal/http"
)

func TestAuthenticatedEventIngestionEndToEnd(t *testing.T) {
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
	key, err := applications.GenerateAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	suffix := fmt.Sprintf("http-integration-%d", time.Now().UnixNano())
	app, err := repo.CreateApplication(ctx, "HTTP integration", applications.HashAPIKey(key))
	if err != nil {
		t.Fatalf("create application: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM outbox_messages WHERE aggregate_id IN (SELECT id FROM events WHERE application_id = $1)`, app.ID)
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM applications WHERE id = $1", app.ID)
	})

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := apphttp.NewRouter(repo, events.NewService(repo), logger)
	body := fmt.Sprintf(`{"event_type":"order.created","recipient":{"external_user_id":%q,"email":"user@example.test"},"data":{"order_id":"o-1"}}`, suffix)
	send := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/events", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-API-Key", key)
		req.Header.Set("Idempotency-Key", suffix)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		return res
	}

	first := send(body)
	if first.Code != http.StatusAccepted {
		t.Fatalf("first request status = %d, body=%s", first.Code, first.Body.String())
	}
	replay := send(body)
	if replay.Code != http.StatusAccepted || replay.Body.String() != first.Body.String() {
		t.Fatalf("replay response differs: status=%d body=%s", replay.Code, replay.Body.String())
	}
	changed := send(strings.Replace(body, "o-1", "o-2", 1))
	if changed.Code != http.StatusConflict {
		t.Fatalf("changed request status = %d, want %d", changed.Code, http.StatusConflict)
	}
	var eventsCount, outboxCount int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM events WHERE application_id=$1 AND idempotency_key=$2", app.ID, suffix).Scan(&eventsCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM outbox_messages o JOIN events e ON e.id=o.aggregate_id WHERE e.application_id=$1 AND e.idempotency_key=$2`, app.ID, suffix).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if eventsCount != 1 || outboxCount != 1 {
		t.Fatalf("persisted event/outbox counts = %d/%d, want 1/1", eventsCount, outboxCount)
	}
	if _, err := pool.Exec(ctx, "UPDATE applications SET is_active=FALSE WHERE id=$1", app.ID); err != nil {
		t.Fatal(err)
	}
	deactivated := send(strings.Replace(body, "o-1", "o-3", 1))
	if deactivated.Code != http.StatusUnauthorized {
		t.Fatalf("inactive application status = %d, want %d", deactivated.Code, http.StatusUnauthorized)
	}
}
