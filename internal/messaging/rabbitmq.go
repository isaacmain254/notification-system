package messaging

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/isaacmain254/notification-system/internal/config"
)

const (
	EventsExchange   = "notifications.events"
	EventQueue       = "notifications.event_processor"
	DeliveryExchange = "notifications.delivery"
)

const (
	initialReconnectDelay = time.Second
	maximumReconnectDelay = 30 * time.Second
)

func connectionString(cfg config.RabbitmqConfig) string {
	u := &url.URL{
		Scheme: "amqp",
		User:   url.UserPassword(cfg.User, cfg.Password),
		Host:   net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.Port)),
		Path:   "/",
	}
	return u.String()
}

// dial establishes a connection to the RabbitMQ server with a timeout and heartbeat configuration.
func dial(ctx context.Context, cfg config.RabbitmqConfig) (*amqp.Connection, error) {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	return amqp.DialConfig(connectionString(cfg), amqp.Config{
		Dial: func(network, address string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, address)
		},
		Heartbeat: 10 * time.Second,
	})
}

// declareTopology sets up the necessary exchanges, queues, and bindings in RabbitMQ for the notification system.
func declareTopology(ch *amqp.Channel) error {
	if err := ch.ExchangeDeclare(EventsExchange, "topic", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare events exchange: %w", err)
	}
	if _, err := ch.QueueDeclare(EventQueue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare event queue: %w", err)
	}
	if err := ch.QueueBind(EventQueue, "event.#", EventsExchange, false, nil); err != nil {
		return fmt.Errorf("bind event queue: %w", err)
	}
	if err := ch.ExchangeDeclare(DeliveryExchange, "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare delivery exchange: %w", err)
	}
	for _, name := range []string{"email", "sms", "inapp"} {
		queue := deliveryQueue(name)
		if _, err := ch.QueueDeclare(queue, true, false, false, false, nil); err != nil {
			return fmt.Errorf("declare %s queue: %w", queue, err)
		}
		if err := ch.QueueBind(queue, name, DeliveryExchange, false, nil); err != nil {
			return fmt.Errorf("bind %s queue: %w", queue, err)
		}
	}
	return nil
}

// reconnectDelay calculates the delay before the next reconnection attempt based on the number of previous attempts, using an exponential backoff strategy.
func reconnectDelay(attempt int) time.Duration {
	delay := initialReconnectDelay
	for i := 1; i < attempt && delay < maximumReconnectDelay; i++ {
		delay *= 2
	}
	if delay > maximumReconnectDelay {
		return maximumReconnectDelay
	}
	return delay
}

// waitReconnect waits for a duration based on the number of reconnection attempts before allowing another connection attempt.
func waitReconnect(ctx context.Context, attempt int) error {
	timer := time.NewTimer(reconnectDelay(attempt))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
