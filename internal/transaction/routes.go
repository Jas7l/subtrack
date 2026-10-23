package transaction

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

// RegisterRoutes mounts /transactions on the protected router; the admin
// only.
func RegisterRoutes(protected *mux.Router, svc *Service) {
	h := &handler{svc: svc}
	admin := auth.RequireRole(domain.RoleAdmin)
	httpx.Register(protected, "/transactions", map[string]http.HandlerFunc{
		"GET":  admin(h.list),
		"POST": admin(h.create),
	})
	httpx.Register(protected, "/transactions/export", map[string]http.HandlerFunc{
		"GET": admin(h.export),
	})
	httpx.Register(protected, "/transactions/{id:[0-9]+}", map[string]http.HandlerFunc{
		"GET": admin(h.get),
	})
	httpx.Register(protected, "/transactions/{id:[0-9]+}/pay", map[string]http.HandlerFunc{
		"POST": admin(h.pay),
	})
	httpx.Register(protected, "/transactions/{id:[0-9]+}/fail", map[string]http.HandlerFunc{
		"POST": admin(h.fail),
	})
}

// writeErr maps transaction errors onto HTTP statuses. Anything
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
