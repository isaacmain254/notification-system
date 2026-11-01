package messaging

import "fmt"

func deliveryQueue(channel string) string {
	return fmt.Sprintf("notifications.%s", channel)
}
