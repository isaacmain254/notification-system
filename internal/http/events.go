package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/isaacmain254/notification-system/internal/events"
)

const maxEventRequestBytes = 1 << 20

type EventSubmitter interface {
	Submit(rctx context.Context, applicationID pgtype.UUID, idempotencyKey string, request events.EventRequest) (string, error)
}

type EventHandler struct {
	service EventSubmitter
	logger  *slog.Logger
}

func NewEventHandler(service EventSubmitter, logger *slog.Logger) *EventHandler {
	return &EventHandler{service: service, logger: logger}
}

func (h *EventHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	appID, ok := authenticatedApplicationID(r.Context())
	if !ok {
		writeAPIError(w, http.StatusUnauthorized, "unauthorized", "valid API key required")
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		writeAPIError(w, http.StatusBadRequest, "missing_idempotency_key", "Idempotency-Key header is required")
		return
	}
	if mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mediaType != "application/json" {
		writeAPIError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "Content-Type must be application/json")
		return
	}
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxEventRequestBytes))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var request events.EventRequest
	if err := decoder.Decode(&request); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeAPIError(w, http.StatusRequestEntityTooLarge, "request_too_large", "request body exceeds the 1 MiB limit")
			return
		}
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "request body must be a valid event JSON object")
		return
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeAPIError(w, http.StatusRequestEntityTooLarge, "request_too_large", "request body exceeds the 1 MiB limit")
			return
		}
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "request body must contain exactly one JSON value")
		return
	}

	eventID, err := h.service.Submit(r.Context(), appID, idempotencyKey, request)
	if err != nil {
		var validationErr *events.ValidationError
		switch {
		case errors.As(err, &validationErr):
			writeAPIError(w, http.StatusUnprocessableEntity, "validation_error", validationErr.Error())
		case errors.Is(err, events.ErrIdempotencyConflict):
			writeAPIError(w, http.StatusConflict, "idempotency_conflict", "Idempotency-Key was previously used with a different request")
		default:
			h.logger.Error("event ingestion failed", "request_id", middleware.GetReqID(r.Context()), "application_id", appID.String(), "error_category", "persistence")
			writeAPIError(w, http.StatusServiceUnavailable, "service_unavailable", "event could not be accepted; retry with the same Idempotency-Key")
		}
		return
	}
	h.logger.Info("event accepted", "request_id", middleware.GetReqID(r.Context()), "application_id", appID.String(), "event_id", eventID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(struct {
		EventID string `json:"event_id"`
		Status  string `json:"status"`
	}{EventID: eventID, Status: "accepted"})
}
