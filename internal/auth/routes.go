package auth

import (
	"context"
	"errors"
	"net/http"

	"github.com/gorilla/mux"

	"subtrack/internal/apperr"
	"subtrack/internal/httpx"
)

// RegisterRoutes mounts the auth endpoints. Login goes on the public router;
// "me" and password change go on the protected one, which the caller has
// already wrapped in Middleware.
func RegisterRoutes(public, protected *mux.Router, svc *Service) {
	h := &handler{svc: svc}
	httpx.Register(public, "/auth/login", map[string]http.HandlerFunc{"POST": h.login})
	httpx.Register(protected, "/auth/me", map[string]http.HandlerFunc{"GET": h.me})
	httpx.Register(protected, "/auth/password", map[string]http.HandlerFunc{"POST": h.changePassword})
}

// writeErr maps auth service errors onto HTTP statuses. Anything unrecognised
// is a 500.
func writeErr(ctx context.Context, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, apperr.ErrInvalidInput), errors.Is(err, ErrWrongPassword):
		httpx.Error(ctx, w, http.StatusBadRequest, err)
	case errors.Is(err, ErrInvalidCredentials), errors.Is(err, ErrUnauthenticated):
		unauthorized(ctx, w, err)
	case errors.Is(err, ErrUserBlocked), errors.Is(err, ErrForbidden):
		httpx.Error(ctx, w, http.StatusForbidden, err)
	default:
		httpx.Internal(ctx, w, err)
	}
}
