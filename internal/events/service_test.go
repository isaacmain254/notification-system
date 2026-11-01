package events

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

type memoryStore struct {
	commands map[string]PersistCommand
	ids      map[string]string
}

func (s *memoryStore) PersistEvent(_ context.Context, appID pgtype.UUID, command PersistCommand) (string, error) {
	key := appID.String() + ":" + command.IdempotencyKey
	if existing, ok := s.commands[key]; ok {
		if existing.RequestHash != command.RequestHash {
			return "", ErrIdempotencyConflict
		}
		return s.ids[key], nil
	}
	s.commands[key] = command
	s.ids[key] = "event-id"
	return "event-id", nil
}

func TestSubmitIsIdempotentAndCanonical(t *testing.T) {
	store := &memoryStore{commands: make(map[string]PersistCommand), ids: make(map[string]string)}
	service := NewService(store)
	appID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	one := EventRequest{EventType: "order.created", Recipient: Recipient{ExternalUserID: "user-1"}, Data: map[string]any{"a": "x", "b": "y"}}
	two := EventRequest{EventType: "order.created", Recipient: Recipient{ExternalUserID: "user-1"}, Data: map[string]any{"b": "y", "a": "x"}}
	firstID, err := service.Submit(context.Background(), appID, "key-1", one)
	if err != nil {
		t.Fatalf("first Submit() error = %v", err)
	}
	secondID, err := service.Submit(context.Background(), appID, "key-1", two)
	if err != nil || secondID != firstID {
		t.Fatalf("replay Submit() = (%q, %v), want original event", secondID, err)
	}
	two.Data["b"] = "different"
	if _, err := service.Submit(context.Background(), appID, "key-1", two); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("different request error = %v, want idempotency conflict", err)
	}
}

func TestNormalizeAndValidateRejectsInvalidFields(t *testing.T) {
	tests := []struct {
		name    string
		request EventRequest
	}{
		{"invalid event type", EventRequest{EventType: "Order Created", Recipient: Recipient{ExternalUserID: "u"}, Data: map[string]any{}}},
		{"missing external ID", EventRequest{EventType: "order.created", Data: map[string]any{}}},
		{"invalid email", EventRequest{EventType: "order.created", Recipient: Recipient{ExternalUserID: "u", Email: "bad"}, Data: map[string]any{}}},
		{"invalid phone", EventRequest{EventType: "order.created", Recipient: Recipient{ExternalUserID: "u", Phone: "123"}, Data: map[string]any{}}},
		{"missing data", EventRequest{EventType: "order.created", Recipient: Recipient{ExternalUserID: "u"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NormalizeAndValidate(test.request); err == nil {
				t.Fatal("NormalizeAndValidate() error = nil")
			}
		})
	}
}
