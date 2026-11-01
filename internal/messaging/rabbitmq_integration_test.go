//go:build integration

package messaging

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/isaacmain254/notification-system/internal/database/sqlc"
)

func TestRabbitMQConfirmedPublishAndUnroutableDetection(t *testing.T) {
	uri := os.Getenv("TEST_RABBITMQ_URL")
	if uri == "" {
		t.Skip("set TEST_RABBITMQ_URL to a dedicated RabbitMQ instance")
	}
	conn, err := amqp.Dial(uri)
	if err != nil {
		t.Fatalf("connect to RabbitMQ: %v", err)
	}
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		t.Fatal(err)
	}
	defer ch.Close()
	if err := declareTopology(ch); err != nil {
		t.Fatal(err)
	}
	publisher, err := newConfirmedPublisher(ch)
	if err != nil {
		t.Fatal(err)
	}
	deliveries, err := ch.Consume(EventQueue, "phase4-test", false, false, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	message := sqlc.OutboxMessage{
		ID:          pgtype.UUID{Bytes: [16]byte{0x71}, Valid: true},
		MessageType: EventReceivedType, RoutingKey: fmt.Sprintf("event.integration.%d", time.Now().UnixNano()),
		Payload: []byte(`{"version":1,"event_id":"integration"}`),
	}
	if err := publisher.publish(ctx, ch, message); err != nil {
		t.Fatalf("confirmed publish: %v", err)
	}
	select {
	case delivery := <-deliveries:
		if delivery.MessageId != uuidText(message.ID) || delivery.DeliveryMode != amqp.Persistent {
			t.Fatalf("received message metadata: ID=%q mode=%d", delivery.MessageId, delivery.DeliveryMode)
		}
		if err := delivery.Nack(false, true); err != nil {
			t.Fatalf("requeue received message: %v", err)
		}
		select {
		case redelivery := <-deliveries:
			if !redelivery.Redelivered {
				t.Fatal("requeued message was not marked as redelivered")
			}
			if err := redelivery.Ack(false); err != nil {
				t.Fatalf("ack redelivered message: %v", err)
			}
		case <-ctx.Done():
			t.Fatal("requeued event was not redelivered before timeout")
		}
	case <-ctx.Done():
		t.Fatal("published event was not consumed before timeout")
	}

	unroutable := message
	unroutable.RoutingKey = "unroutable." + strings.TrimPrefix(message.RoutingKey, "event.")
	if err := publisher.publish(ctx, ch, unroutable); err == nil || !strings.Contains(err.Error(), "unroutable") {
		t.Fatalf("unroutable publish error = %v, want unroutable error", err)
	}
}
