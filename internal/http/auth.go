package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/isaacmain254/notification-system/internal/applications"
)

type applicationIDKey struct{}

type ApplicationKeyLookup interface {
	LookupActiveApplicationID(context.Context, string) (pgtype.UUID, error)
}

func authenticatedApplicationID(ctx context.Context) (pgtype.UUID, bool) {
	id, ok := ctx.Value(applicationIDKey{}).(pgtype.UUID)
	return id, ok
}

func ApplicationAuth(lookup ApplicationKeyLookup, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := strings.TrimSpace(r.Header.Get("X-API-Key"))
			if key == "" || len(key) > 256 {
				writeAPIError(w, http.StatusUnauthorized, "unauthorized", "valid API key required")
				return
			}
			appID, err := lookup.LookupActiveApplicationID(r.Context(), applications.HashAPIKey(key))
			if errors.Is(err, pgx.ErrNoRows) {
				writeAPIError(w, http.StatusUnauthorized, "unauthorized", "valid API key required")
				return
			}
			if err != nil {
				logger.Error("application authentication lookup failed", "request_id", middleware.GetReqID(r.Context()), "error_category", "database")
				writeAPIError(w, http.StatusServiceUnavailable, "service_unavailable", "authentication service unavailable")
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), applicationIDKey{}, appID)))
		})
	}
}
