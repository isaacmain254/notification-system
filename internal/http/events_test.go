package http

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/isaacmain254/notification-system/internal/applications"
	"github.com/isaacmain254/notification-system/internal/events"
)

const testAPIKey = "test-api-key"

type testApplicationLookup struct{ id pgtype.UUID }

func (l testApplicationLookup) LookupActiveApplicationID(_ context.Context, hash string) (pgtype.UUID, error) {
	if hash != applications.HashAPIKey(testAPIKey) {
		return pgtype.UUID{}, pgx.ErrNoRows
	}
	return l.id, nil
}

type testEventStore struct {
	hashes map[string]string
	ids    map[string]string
}

func (s *testEventStore) PersistEvent(_ context.Context, appID pgtype.UUID, command events.PersistCommand) (string, error) {
	key := appID.String() + ":" + command.IdempotencyKey
	if hash, ok := s.hashes[key]; ok {
		if hash != command.RequestHash {
			return "", events.ErrIdempotencyConflict
		}
		return s.ids[key], nil
	}
	s.hashes[key] = command.RequestHash
	s.ids[key] = "00000000-0000-0000-0000-000000000001"
	return s.ids[key], nil
}

func newTestRouter() http.Handler {
	appID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	store := &testEventStore{hashes: make(map[string]string), ids: make(map[string]string)}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRouter(testApplicationLookup{id: appID}, events.NewService(store), logger)
}

func sendEvent(router http.Handler, body, key, idempotencyKey string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("X-API-Key", key)
	}
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	return res
}

const validBody = `{"event_type":"order.created","recipient":{"external_user_id":"user-1"},"data":{"order_id":"o-1"}}`

func TestEventHandlerAuthenticationAndRequestErrors(t *testing.T) {
	tests := []struct {
		name, body, key, idempotencyKey string
		wantStatus                      int
	}{
		{"missing API key", validBody, "", "key-1", http.StatusUnauthorized},
		{"invalid API key", validBody, "invalid", "key-1", http.StatusUnauthorized},
		{"missing idempotency key", validBody, testAPIKey, "", http.StatusBadRequest},
		{"malformed JSON", "{", testAPIKey, "key-1", http.StatusBadRequest},
		{"invalid event type", `{"event_type":"Bad Event","recipient":{"external_user_id":"u"},"data":{}}`, testAPIKey, "key-1", http.StatusUnprocessableEntity},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			res := sendEvent(newTestRouter(), test.body, test.key, test.idempotencyKey)
			if res.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", res.Code, test.wantStatus, res.Body.String())
			}
		})
	}
}

func TestEventHandlerAcceptsReplayAndRejectsChangedRequest(t *testing.T) {
	router := newTestRouter()
	first := sendEvent(router, validBody, testAPIKey, "idem-1")
	if first.Code != http.StatusAccepted {
		t.Fatalf("first status = %d, want %d; body=%s", first.Code, http.StatusAccepted, first.Body.String())
	}
	replay := sendEvent(router, validBody, testAPIKey, "idem-1")
	if replay.Code != http.StatusAccepted || replay.Body.String() != first.Body.String() {
		t.Fatalf("replay response = (%d, %q), want original (%d, %q)", replay.Code, replay.Body.String(), first.Code, first.Body.String())
	}
	changed := strings.Replace(validBody, "o-1", "o-2", 1)
	conflict := sendEvent(router, changed, testAPIKey, "idem-1")
	if conflict.Code != http.StatusConflict {
		t.Fatalf("changed request status = %d, want %d", conflict.Code, http.StatusConflict)
	}
}

func TestAuthLookupFailureReturnsUnavailable(t *testing.T) {
	lookup := failingApplicationLookup{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouter(lookup, events.NewService(&testEventStore{hashes: map[string]string{}, ids: map[string]string{}}), logger)
	res := sendEvent(router, validBody, testAPIKey, "key-1")
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusServiceUnavailable)
	}
}

type failingApplicationLookup struct{}

func (failingApplicationLookup) LookupActiveApplicationID(context.Context, string) (pgtype.UUID, error) {
	return pgtype.UUID{}, errors.New("database offline")
}
