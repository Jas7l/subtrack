package request

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

// RegisterRoutes mounts /requests on the protected router (already behind
// auth.Middleware). Filing a request is for employees and managers, deciding
// for managers (the service checks they head the department) and the admin;
// reads and cancelling are scoped by the service.
func RegisterRoutes(protected *mux.Router, svc *Service) {
	h := &handler{svc: svc}
	requesters := auth.RequireRole(domain.RoleEmployee, domain.RoleManager)
	reviewers := auth.RequireRole(domain.RoleManager, domain.RoleAdmin)
	httpx.Register(protected, "/requests", map[string]http.HandlerFunc{
		"GET":  h.list,
		"POST": requesters(h.create),
	})
	httpx.Register(protected, "/requests/{id:[0-9]+}", map[string]http.HandlerFunc{
		"GET": h.get,
	})
	httpx.Register(protected, "/requests/{id:[0-9]+}/approve", map[string]http.HandlerFunc{
		"POST": reviewers(h.approve),
	})
	httpx.Register(protected, "/requests/{id:[0-9]+}/reject", map[string]http.HandlerFunc{
		"POST": reviewers(h.reject),
	})
	httpx.Register(protected, "/requests/{id:[0-9]+}/cancel", map[string]http.HandlerFunc{
		"POST": h.cancel,
	})
}

// writeErr maps service errors onto HTTP statuses by their apperr kind. That
// covers this package's sentinels and the subscription domain's, which an
// admin's approval passes through. Anything unrecognised is a 500.
func writeErr(ctx context.Context, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, apperr.ErrInvalidInput):
		httpx.Error(ctx, w, http.StatusBadRequest, err)
	case errors.Is(err, auth.ErrForbidden):
		httpx.Error(ctx, w, http.StatusForbidden, err)
	case errors.Is(err, apperr.ErrNotFound):
		httpx.Error(ctx, w, http.StatusNotFound, err)
	case errors.Is(err, apperr.ErrConflict):
		httpx.Error(ctx, w, http.StatusConflict, err)
	default:
		httpx.Internal(ctx, w, err)
	}
}
