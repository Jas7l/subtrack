package department

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

// RegisterRoutes mounts /departments on the protected router (already behind
// auth.Middleware). Writes are admin-only; reads are scoped by the service.
func RegisterRoutes(protected *mux.Router, svc *Service) {
	h := &handler{svc: svc}
	admin := auth.RequireRole(domain.RoleAdmin)
	httpx.Register(protected, "/departments", map[string]http.HandlerFunc{
		"GET":  h.list,
		"POST": admin(h.create),
	})
	httpx.Register(protected, "/departments/{id:[0-9]+}", map[string]http.HandlerFunc{
		"GET":    h.get,
		"PATCH":  admin(h.update),
		"DELETE": admin(h.delete),
	})
}

// writeErr maps departments service errors onto HTTP statuses. Anything
// unrecognised is a 500.
func writeErr(ctx context.Context, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, apperr.ErrInvalidInput), errors.Is(err, ErrInvalidHead):
		httpx.Error(ctx, w, http.StatusBadRequest, err)
	case errors.Is(err, ErrDepartmentNotFound):
		httpx.Error(ctx, w, http.StatusNotFound, err)
	case errors.Is(err, ErrNameTaken), errors.Is(err, ErrHeadTaken), errors.Is(err, ErrDepartmentNotEmpty),
		errors.Is(err, apperr.ErrConflict):
		httpx.Error(ctx, w, http.StatusConflict, err)
	default:
		httpx.Internal(ctx, w, err)
	}
}
