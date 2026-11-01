package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/isaacmain254/notification-system/internal/config"
)

type EventStore interface {
	BeginEventProcessing(context.Context, string) (bool, error)
}

func handleEvent(ctx context.Context, store EventStore, delivery amqp.Delivery) error {
	if delivery.Type != EventReceivedType {
		return fmt.Errorf("unexpected event message type %q", delivery.Type)
	}
	var message EventMessage
	if err := json.Unmarshal(delivery.Body, &message); err != nil {
		return fmt.Errorf("decode event message: %w", err)
	}
	if message.Version != 1 || message.EventID == "" {
		return fmt.Errorf("unsupported or incomplete event message")
	}
	if _, err := store.BeginEventProcessing(ctx, message.EventID); err != nil {
		return fmt.Errorf("begin processing event %s: %w", message.EventID, err)
	}
	return nil
}

// RunEventConsumer reconnects until the context is cancelled. Delivery
// acknowledgements follow the durable, idempotent event status transition.
func RunEventConsumer(ctx context.Context, cfg config.RabbitmqConfig, store EventStore, prefetch int, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}
	if prefetch < 1 {
		prefetch = 1
	}
	reconnectAttempt := 0
	for ctx.Err() == nil {
		conn, err := dial(ctx, cfg)
		if err != nil {
			reconnectAttempt++
			logger.Warn("connect to RabbitMQ event consumer", "error", err)
			if waitReconnect(ctx, reconnectAttempt) != nil {
				return nil
			}
			continue
		}
		ch, err := conn.Channel()
		if err == nil {
			err = declareTopology(ch)
		}
		if err == nil {
			err = ch.Qos(prefetch, 0, false)
		}
		var deliveries <-chan amqp.Delivery
		if err == nil {
			deliveries, err = ch.Consume(EventQueue, "", false, false, false, false, nil)
		}
		if err != nil {
			reconnectAttempt++
			if ch != nil {
				_ = ch.Close()
			}
			_ = conn.Close()
			logger.Warn("initialize RabbitMQ event consumer", "error", err)
			if waitReconnect(ctx, reconnectAttempt) != nil {
				return nil
			}
			continue
		}

		reconnectAttempt = 0
		connectionClosed := conn.NotifyClose(make(chan *amqp.Error, 1))
		channelClosed := ch.NotifyClose(make(chan *amqp.Error, 1))
		connected := true
		for connected && ctx.Err() == nil {
			select {
			case <-ctx.Done():
				connected = false
			case <-connectionClosed:
				connected = false
			case <-channelClosed:
				connected = false
			case delivery, ok := <-deliveries:
				if !ok {
					connected = false
					continue
				}
				if err := handleEvent(ctx, store, delivery); err != nil {
					logger.Error("process RabbitMQ event", "message_id", delivery.MessageId, "error", err)
					if nackErr := delivery.Nack(false, true); nackErr != nil {
						logger.Error("requeue RabbitMQ event", "error", nackErr)
						connected = false
					}
					continue
				}
				if err := delivery.Ack(false); err != nil {
					logger.Error("acknowledge RabbitMQ event", "message_id", delivery.MessageId, "error", err)
					connected = false
				}
			}
		}
		_ = ch.Close()
		_ = conn.Close()
	}
	return nil
}
