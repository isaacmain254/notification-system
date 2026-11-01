package messaging

const EventReceivedType = "event.received"

type EventMessage struct {
	Version       int    `json:"version"`
	EventID       string `json:"event_id"`
	ApplicationID string `json:"application_id"`
	EventType     string `json:"event_type"`
	RecipientID   string `json:"recipient_id"`
}
