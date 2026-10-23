package user

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

// RegisterRoutes mounts /users on the protected router (already behind
// auth.Middleware). Writes are admin-only; reads are open to every signed-in
// user and scoped by the service.
func RegisterRoutes(protected *mux.Router, svc *Service) {
	h := &handler{svc: svc}
	admin := auth.RequireRole(domain.RoleAdmin)
	httpx.Register(protected, "/users", map[string]http.HandlerFunc{
		"GET":  h.list,
		"POST": admin(h.create),
	})
	httpx.Register(protected, "/users/{id:[0-9]+}", map[string]http.HandlerFunc{
		"GET":   h.get,
		"PATCH": admin(h.update),
	})
	httpx.Register(protected, "/users/{id:[0-9]+}/password", map[string]http.HandlerFunc{
		"PUT": admin(h.setPassword),
	})
}

// writeErr maps users service errors onto HTTP statuses. Anything
// unrecognised is a 500.
func writeErr(ctx context.Context, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, apperr.ErrInvalidInput), errors.Is(err, ErrUnknownDepartment):
		httpx.Error(ctx, w, http.StatusBadRequest, err)
	case errors.Is(err, auth.ErrForbidden):
		httpx.Error(ctx, w, http.StatusForbidden, err)
	case errors.Is(err, ErrUserNotFound):
		httpx.Error(ctx, w, http.StatusNotFound, err)
	case errors.Is(err, ErrEmailTaken), errors.Is(err, ErrSelfLockout), errors.Is(err, apperr.ErrConflict):
		httpx.Error(ctx, w, http.StatusConflict, err)
	default:
		httpx.Internal(ctx, w, err)
	}
}
