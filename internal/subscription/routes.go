package subscription

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

// RegisterRoutes mounts /subscriptions, /assignments and /release-requests
// on the protected router (already behind auth.Middleware). Subscription
// writes are admin-only; handing out licences and deciding release requests
// is for the admin and managers; asking to give a licence back for employees
// and managers (the admin revokes directly). Reads, revoking and cancelling
// a release request are open to everyone and checked by the service.
func RegisterRoutes(protected *mux.Router, svc *Service) {
	h := &handler{svc: svc}
	admin := auth.RequireRole(domain.RoleAdmin)
	staff := auth.RequireRole(domain.RoleAdmin, domain.RoleManager)
	holders := auth.RequireRole(domain.RoleEmployee, domain.RoleManager)
	httpx.Register(protected, "/subscriptions", map[string]http.HandlerFunc{
		"GET":  h.list,
		"POST": admin(h.create),
	})
	httpx.Register(protected, "/subscriptions/export", map[string]http.HandlerFunc{
		"GET": admin(h.export),
	})
	httpx.Register(protected, "/subscriptions/{id:[0-9]+}", map[string]http.HandlerFunc{
		"GET":    h.get,
		"PATCH":  admin(h.update),
		"DELETE": admin(h.delete),
	})
	httpx.Register(protected, "/assignments", map[string]http.HandlerFunc{
		"GET":  h.listAssignments,
		"POST": staff(h.assign),
	})
	httpx.Register(protected, "/assignments/{id:[0-9]+}", map[string]http.HandlerFunc{
		"GET": h.getAssignment,
	})
	httpx.Register(protected, "/assignments/{id:[0-9]+}/revoke", map[string]http.HandlerFunc{
		"POST": h.revoke,
	})
	httpx.Register(protected, "/release-requests", map[string]http.HandlerFunc{
		"GET":  h.listReleases,
		"POST": holders(h.createRelease),
	})
	httpx.Register(protected, "/release-requests/{id:[0-9]+}", map[string]http.HandlerFunc{
		"GET": h.getRelease,
	})
	httpx.Register(protected, "/release-requests/{id:[0-9]+}/approve", map[string]http.HandlerFunc{
		"POST": staff(h.approveRelease),
	})
	httpx.Register(protected, "/release-requests/{id:[0-9]+}/reject", map[string]http.HandlerFunc{
		"POST": staff(h.rejectRelease),
	})
	httpx.Register(protected, "/release-requests/{id:[0-9]+}/cancel", map[string]http.HandlerFunc{
		"POST": h.cancelRelease,
	})
}

// writeErr maps subscription service errors onto HTTP statuses. Anything
// unrecognised is a 500.
func writeErr(ctx context.Context, w http.ResponseWriter, err error) {
	// Every sentinel of this package carries its apperr kind.
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
