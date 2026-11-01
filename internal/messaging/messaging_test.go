package messaging

import (
	"context"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/isaacmain254/notification-system/internal/config"
	"github.com/isaacmain254/notification-system/internal/database/sqlc"
)

func TestConnectionStringEscapesCredentials(t *testing.T) {
	got, err := url.Parse(connectionString(config.RabbitmqConfig{
		Host: "localhost", Port: 5672, User: "user@host", Password: "p:a/ss",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if got.Scheme != "amqp" || got.Host != "localhost:5672" {
		t.Fatalf("connection URL = %s", got)
	}
	user := got.User.Username()
	password, ok := got.User.Password()
	if user != "user@host" || !ok || password != "p:a/ss" {
		t.Fatalf("credentials = %q %q, has password=%t", user, password, ok)
	}
}

func TestReconnectDelayGrowsAndIsCapped(t *testing.T) {
	if reconnectDelay(1) != time.Second || reconnectDelay(2) != 2*time.Second {
		t.Fatalf("initial reconnect delays = %s, %s", reconnectDelay(1), reconnectDelay(2))
	}
	if got := reconnectDelay(20); got != maximumReconnectDelay {
		t.Fatalf("capped reconnect delay = %s, want %s", got, maximumReconnectDelay)
	}
}

func TestOutboxPublishingIsPersistentAndIdentified(t *testing.T) {
	id := pgtype.UUID{Bytes: [16]byte{0x12, 0x34}, Valid: true}
	message := sqlc.OutboxMessage{
		ID: id, MessageType: EventReceivedType, RoutingKey: "event.order.created", Payload: []byte(`{"event_id":"id"}`),
	}
	publishing := outboxPublishing(message)
	if publishing.DeliveryMode != amqp.Persistent {
		t.Fatalf("delivery mode = %d, want persistent", publishing.DeliveryMode)
	}
	if publishing.MessageId != uuidText(id) || publishing.Type != EventReceivedType {
		t.Fatalf("message metadata = id %q type %q", publishing.MessageId, publishing.Type)
	}
	if publishing.ContentType != "application/json" || string(publishing.Body) != string(message.Payload) {
		t.Fatalf("publishing content = %q %q", publishing.ContentType, publishing.Body)
	}
	if message.RoutingKey != "event.order.created" || EventsExchange != "notifications.events" {
		t.Fatalf("routing = %s via %s", message.RoutingKey, EventsExchange)
	}
}

type eventStoreFunc func(context.Context, string) (bool, error)

func (f eventStoreFunc) BeginEventProcessing(ctx context.Context, id string) (bool, error) {
	return f(ctx, id)
}

func TestHandleEventPersistsStatusAndAcceptsDuplicate(t *testing.T) {
	for _, transitioned := range []bool{true, false} {
		calls := 0
		err := handleEvent(context.Background(), eventStoreFunc(func(_ context.Context, id string) (bool, error) {
			calls++
			if id != "event-id" {
				t.Fatalf("event ID = %q", id)
			}
			return transitioned, nil
		}), amqp.Delivery{
			Type: EventReceivedType,
			Body: []byte(`{"version":1,"event_id":"event-id"}`),
		})
		if err != nil {
			t.Fatalf("handle event (transitioned=%t): %v", transitioned, err)
		}
		if calls != 1 {
			t.Fatalf("status transition calls = %d, want 1", calls)
		}
	}
}

func TestHandleEventRejectsInvalidMessages(t *testing.T) {
	store := eventStoreFunc(func(context.Context, string) (bool, error) {
		t.Fatal("invalid message reached event store")
		return false, nil
	})
	for _, delivery := range []amqp.Delivery{
		{Type: EventReceivedType, Body: []byte("{")},
		{Type: EventReceivedType, Body: []byte(`{"version":2,"event_id":"id"}`)},
		{Type: "unexpected", Body: []byte(`{"version":1,"event_id":"id"}`)},
	} {
		if err := handleEvent(context.Background(), store, delivery); err == nil {
			t.Fatal("handleEvent() error = nil, want invalid-message error")
		}
	}
}

type unusedOutboxStore struct{ claims atomic.Int32 }

func (s *unusedOutboxStore) ClaimOutboxMessages(context.Context, int32, float64) ([]sqlc.OutboxMessage, error) {
	s.claims.Add(1)
	return nil, nil
}
func (*unusedOutboxStore) MarkOutboxMessagePublished(context.Context, pgtype.UUID) (int64, error) {
	return 0, nil
}
func (*unusedOutboxStore) RecordOutboxPublishFailure(context.Context, pgtype.UUID, string) (int64, error) {
	return 0, nil
}

func TestOutboxPublisherStopsDuringUnavailableBrokerReconnect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	store := &unusedOutboxStore{}
	err := RunOutboxPublisher(ctx, config.RabbitmqConfig{
		Host: "127.0.0.1", Port: 1, User: "test", Password: "test",
	}, store, 5, nil)
	if err != nil {
		t.Fatalf("RunOutboxPublisher() error = %v", err)
	}
	if store.claims.Load() != 0 {
		t.Fatalf("outbox claims = %d while RabbitMQ was unavailable, want 0", store.claims.Load())
	}
}
