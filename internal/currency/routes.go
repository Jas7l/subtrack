package currency

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

// RegisterRoutes mounts /currencies on the protected router (already behind
// auth.Middleware). Everyone reads; writes are admin-only.
func RegisterRoutes(protected *mux.Router, svc *Service) {
	h := &handler{svc: svc}
	admin := auth.RequireRole(domain.RoleAdmin)
	httpx.Register(protected, "/currencies", map[string]http.HandlerFunc{
		"GET":  h.list,
		"POST": admin(h.create),
	})
	httpx.Register(protected, "/currencies/{code:[A-Za-z]+}", map[string]http.HandlerFunc{
		"GET":    h.get,
		"PATCH":  admin(h.update),
		"DELETE": admin(h.delete),
	})
}

// writeErr maps currencies service errors onto HTTP statuses. Anything
// unrecognised is a 500.
func writeErr(ctx context.Context, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, apperr.ErrInvalidInput):
		httpx.Error(ctx, w, http.StatusBadRequest, err)
	case errors.Is(err, apperr.ErrNotFound):
		httpx.Error(ctx, w, http.StatusNotFound, err)
	case errors.Is(err, apperr.ErrConflict):
		httpx.Error(ctx, w, http.StatusConflict, err)
	default:
		httpx.Internal(ctx, w, err)
	}
}
