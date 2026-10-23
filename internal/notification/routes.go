package notification

import (
	"context"
	"errors"
	"net/http"

	"github.com/gorilla/mux"

	"subtrack/internal/apperr"
	"subtrack/internal/httpx"
)

// RegisterRoutes mounts /notifications on the protected router. Everyone
// reaches only their own inbox.
func RegisterRoutes(protected *mux.Router, svc *Service) {
	h := &handler{svc: svc}
	httpx.Register(protected, "/notifications", map[string]http.HandlerFunc{
		"GET": h.list,
	})
	httpx.Register(protected, "/notifications/{id:[0-9]+}/read", map[string]http.HandlerFunc{
		"POST": h.markRead,
	})
	httpx.Register(protected, "/notifications/read-all", map[string]http.HandlerFunc{
		"POST": h.markAllRead,
	})
}

// writeErr maps notification errors onto HTTP statuses. Anything
// unrecognised is a 500.
func writeErr(ctx context.Context, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, apperr.ErrInvalidInput):
		httpx.Error(ctx, w, http.StatusBadRequest, err)
	case errors.Is(err, apperr.ErrNotFound):
		httpx.Error(ctx, w, http.StatusNotFound, err)
	default:
		httpx.Internal(ctx, w, err)
	}
}
