package auth

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"

	"subtrack/internal/domain"
	"subtrack/internal/httpx"
	"subtrack/internal/logger"
)

type ctxKey struct{}

// WithUser returns a context carrying u as the signed-in user. Middleware uses
// it; handler tests in other packages use it to fake a caller.
func WithUser(ctx context.Context, u domain.User) context.Context {
	return context.WithValue(ctx, ctxKey{}, u)
}

// UserFromContext returns the signed-in user put there by Middleware. ok is
// false only on a route that was mounted without Middleware.
func UserFromContext(ctx context.Context) (domain.User, bool) {
	u, ok := ctx.Value(ctxKey{}).(domain.User)
	return u, ok
}

// Middleware requires a valid "Authorization: Bearer <token>" header. On
// success the user is stored in the request context and bound to the
// request-scoped logger as user_id, so every log line below names the caller.
// On failure it answers 401 and the handler never runs.
func (s *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		token, ok := bearerToken(r)
		if !ok {
			unauthorized(ctx, w, ErrUnauthenticated)
			return
		}
		u, err := s.Authenticate(ctx, token)
		if err != nil {
			if errors.Is(err, ErrUnauthenticated) {
				unauthorized(ctx, w, ErrUnauthenticated)
				return
			}
			httpx.Internal(ctx, w, err)
			return
		}
		ctx = logger.WithContext(ctx, logger.FromContext(ctx).With("user_id", u.ID))
		next.ServeHTTP(w, r.WithContext(WithUser(ctx, u)))
	})
}

// RequireRole lets the request through only when the signed-in user has one
// of the given roles, and answers 403 otherwise. It must run inside
// Middleware; a request with no user in its context gets 401.
//
//	httpx.Register(api, "/users", map[string]http.HandlerFunc{
//		"POST": auth.RequireRole(domain.RoleAdmin)(h.create),
//	})
func RequireRole(roles ...domain.Role) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			u, ok := UserFromContext(r.Context())
			if !ok {
				unauthorized(r.Context(), w, ErrUnauthenticated)
				return
			}
			if !slices.Contains(roles, u.Role) {
				httpx.Error(r.Context(), w, http.StatusForbidden, ErrForbidden)
				return
			}
			next(w, r)
		}
	}
}

// bearerToken extracts the token from the Authorization header. The scheme is
// matched case-insensitively, as RFC 7235 requires.
func bearerToken(r *http.Request) (string, bool) {
	scheme, token, found := strings.Cut(r.Header.Get("Authorization"), " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	token = strings.TrimSpace(token)
	return token, token != ""
}

// unauthorized answers 401 with the WWW-Authenticate challenge RFC 7235
// requires on that status.
func unauthorized(ctx context.Context, w http.ResponseWriter, err error) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="subtrack"`)
	httpx.Error(ctx, w, http.StatusUnauthorized, err)
}
