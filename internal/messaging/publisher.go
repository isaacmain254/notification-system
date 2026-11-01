package messaging

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/isaacmain254/notification-system/internal/config"
	"github.com/isaacmain254/notification-system/internal/database/sqlc"
)

const (
	outboxPollInterval = time.Second
	outboxLeaseSeconds = 30
)

type OutboxStore interface {
	ClaimOutboxMessages(context.Context, int32, float64) ([]sqlc.OutboxMessage, error)
	MarkOutboxMessagePublished(context.Context, pgtype.UUID) (int64, error)
	RecordOutboxPublishFailure(context.Context, pgtype.UUID, string) (int64, error)
}

type confirmedPublisher struct {
	confirms <-chan amqp.Confirmation
	returns  <-chan amqp.Return
}

func newConfirmedPublisher(ch *amqp.Channel) (*confirmedPublisher, error) {
	if err := ch.Confirm(false); err != nil {
		return nil, fmt.Errorf("enable publisher confirms: %w", err)
	}
	return &confirmedPublisher{
		confirms: ch.NotifyPublish(make(chan amqp.Confirmation, 1)),
		returns:  ch.NotifyReturn(make(chan amqp.Return, 1)),
	}, nil
}

func uuidText(id pgtype.UUID) string {
	b := id.Bytes
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func outboxPublishing(message sqlc.OutboxMessage) amqp.Publishing {
	return amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		MessageId:    uuidText(message.ID),
		Type:         message.MessageType,
		Timestamp:    time.Now().UTC(),
		Body:         message.Payload,
	}
}

func (p *confirmedPublisher) publish(ctx context.Context, ch *amqp.Channel, message sqlc.OutboxMessage) error {
	if err := ch.PublishWithContext(ctx, EventsExchange, message.RoutingKey, true, false, outboxPublishing(message)); err != nil {
		return fmt.Errorf("publish outbox message %s: %w", uuidText(message.ID), err)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case confirmation, ok := <-p.confirms:
		if !ok {
			return fmt.Errorf("publisher confirm channel closed")
		}
		if !confirmation.Ack {
			return fmt.Errorf("broker negatively acknowledged outbox message %s", uuidText(message.ID))
		}
		// RabbitMQ sends a mandatory return before its confirm for an unroutable
		// publication. Check the return notification after receiving that confirm.
		select {
		case returned := <-p.returns:
			return fmt.Errorf("outbox message %s was unroutable: %s", uuidText(message.ID), returned.ReplyText)
		default:
			return nil
		}
	}
}

// RunOutboxPublisher continuously claims durable outbox work and publishes it.
// Unpublished rows remain eligible after broker failures or process restarts.
func RunOutboxPublisher(ctx context.Context, cfg config.RabbitmqConfig, store OutboxStore, batchSize int32, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}
	if batchSize < 1 {
		batchSize = 50
	}
	reconnectAttempt := 0
	for ctx.Err() == nil {
		conn, err := dial(ctx, cfg)
		if err != nil {
			reconnectAttempt++
			logger.Warn("connect to RabbitMQ for outbox publisher", "error", err)
			if waitReconnect(ctx, reconnectAttempt) != nil {
				return nil
			}
			continue
		}
		ch, err := conn.Channel()
		if err == nil {
			err = declareTopology(ch)
		}
		var publisher *confirmedPublisher
		if err == nil {
			publisher, err = newConfirmedPublisher(ch)
		}
		if err != nil {
			reconnectAttempt++
			if ch != nil {
				_ = ch.Close()
			}
			_ = conn.Close()
			logger.Warn("initialize RabbitMQ outbox publisher", "error", err)
			if waitReconnect(ctx, reconnectAttempt) != nil {
				return nil
			}
			continue
		}

		reconnectAttempt = 0
		connectionClosed := conn.NotifyClose(make(chan *amqp.Error, 1))
		for ctx.Err() == nil {
			select {
			case <-connectionClosed:
				err = fmt.Errorf("RabbitMQ connection closed")
			default:
			}
			if err != nil {
				break
			}
			messages, claimErr := store.ClaimOutboxMessages(ctx, batchSize, outboxLeaseSeconds)
			if claimErr != nil {
				logger.Error("claim outbox messages", "error", claimErr)
				if waitReconnect(ctx, 1) != nil {
					break
				}
				continue
			}
			if len(messages) == 0 {
				timer := time.NewTimer(outboxPollInterval)
				select {
				case <-ctx.Done():
					timer.Stop()
				case <-connectionClosed:
					timer.Stop()
					err = fmt.Errorf("RabbitMQ connection closed")
				case <-timer.C:
				}
				continue
			}
			for _, message := range messages {
				publishErr := publisher.publish(ctx, ch, message)
				if publishErr != nil {
					logger.Error("publish outbox message", "message_id", uuidText(message.ID), "error", publishErr)
					if _, recordErr := store.RecordOutboxPublishFailure(ctx, message.ID, publishErr.Error()); recordErr != nil {
						logger.Error("record outbox publish failure", "message_id", uuidText(message.ID), "error", recordErr)
					}
					continue
				}
				if _, err = store.MarkOutboxMessagePublished(ctx, message.ID); err != nil {
					logger.Error("mark outbox message published", "message_id", uuidText(message.ID), "error", err)
				}
			}
		}
		_ = ch.Close()
		_ = conn.Close()
		if ctx.Err() != nil {
			return nil
		}
		logger.Warn("reconnecting RabbitMQ outbox publisher", "error", err)
		reconnectAttempt++
		if waitReconnect(ctx, reconnectAttempt) != nil {
			return nil
		}
	}
	return nil
}
