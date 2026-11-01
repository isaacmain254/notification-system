package events

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrIdempotencyConflict = errors.New("idempotency key was already used for a different request")
	eventTypePattern       = regexp.MustCompile(`^[a-z][a-z0-9]*(\.[a-z][a-z0-9]*)+$`)
	phonePattern           = regexp.MustCompile(`^\+[1-9][0-9]{7,14}$`)
)

type Recipient struct {
	ExternalUserID string `json:"external_user_id"`
	Email          string `json:"email,omitempty"`
	Phone          string `json:"phone,omitempty"`
}

type EventRequest struct {
	EventType string         `json:"event_type"`
	Recipient Recipient      `json:"recipient"`
	Data      map[string]any `json:"data"`
}

type PersistCommand struct {
	EventType      string
	IdempotencyKey string
	RequestHash    string
	Recipient      Recipient
	DataJSON       []byte
}

type Store interface {
	PersistEvent(context.Context, pgtype.UUID, PersistCommand) (string, error)
}

type Service struct{ store Store }

func NewService(store Store) *Service { return &Service{store: store} }

func (s *Service) Submit(ctx context.Context, applicationID pgtype.UUID, idempotencyKey string, request EventRequest) (string, error) {
	request, err := NormalizeAndValidate(request)
	if err != nil {
		return "", err
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" || len(idempotencyKey) > 255 {
		return "", &ValidationError{Field: "Idempotency-Key", Message: "must contain 1 to 255 characters"}
	}

	canonical, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("encode canonical request: %w", err)
	}
	digest := sha256.Sum256(canonical)
	dataJSON, err := json.Marshal(request.Data)
	if err != nil {
		return "", fmt.Errorf("encode event data: %w", err)
	}
	return s.store.PersistEvent(ctx, applicationID, PersistCommand{
		EventType: request.EventType, IdempotencyKey: idempotencyKey,
		RequestHash: hex.EncodeToString(digest[:]), Recipient: request.Recipient,
		DataJSON: dataJSON,
	})
}

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Field + " " + e.Message }

func NormalizeAndValidate(request EventRequest) (EventRequest, error) {
	request.EventType = strings.TrimSpace(request.EventType)
	if len(request.EventType) > 128 || !eventTypePattern.MatchString(request.EventType) {
		return EventRequest{}, &ValidationError{Field: "event_type", Message: "must be lowercase dot-separated words, up to 128 characters"}
	}
	request.Recipient.ExternalUserID = strings.TrimSpace(request.Recipient.ExternalUserID)
	request.Recipient.Email = strings.TrimSpace(request.Recipient.Email)
	request.Recipient.Phone = strings.TrimSpace(request.Recipient.Phone)
	if request.Recipient.ExternalUserID == "" || len(request.Recipient.ExternalUserID) > 255 {
		return EventRequest{}, &ValidationError{Field: "recipient.external_user_id", Message: "must contain 1 to 255 characters"}
	}
	if request.Recipient.Email != "" {
		address, err := mail.ParseAddress(request.Recipient.Email)
		if err != nil || address.Address != request.Recipient.Email || len(request.Recipient.Email) > 320 {
			return EventRequest{}, &ValidationError{Field: "recipient.email", Message: "must be a valid email address"}
		}
	}
	if request.Recipient.Phone != "" && !phonePattern.MatchString(request.Recipient.Phone) {
		return EventRequest{}, &ValidationError{Field: "recipient.phone", Message: "must use E.164 format"}
	}
	if request.Data == nil {
		return EventRequest{}, &ValidationError{Field: "data", Message: "must be a JSON object"}
	}
	return request, nil
}
