package audit

import (
	"context"
	"errors"
	"net/http"

	"github.com/gorilla/mux"

	"subtrack/internal/apperr"
	"subtrack/internal/auth"
	"subtrack/internal/domain"
	"subtrack/internal/httpx"
)

// RegisterRoutes mounts /audit on the protected router; the admin only.
func RegisterRoutes(protected *mux.Router, svc *Service) {
	h := &handler{svc: svc}
	admin := auth.RequireRole(domain.RoleAdmin)
	httpx.Register(protected, "/audit", map[string]http.HandlerFunc{
		"GET": admin(h.list),
	})
}

// writeErr maps audit errors onto HTTP statuses. Anything unrecognised is a
// 500.
func writeErr(ctx context.Context, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, apperr.ErrInvalidInput):
		httpx.Error(ctx, w, http.StatusBadRequest, err)
	default:
		httpx.Internal(ctx, w, err)
	}
}
