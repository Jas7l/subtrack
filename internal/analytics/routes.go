package analytics

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

// RegisterRoutes mounts /analytics on the protected router. The summary,
// the breakdowns by category and vendor and the forecast are for the admin
// and department heads (their own department); the rest is the admin's.
func RegisterRoutes(protected *mux.Router, svc *Service) {
	h := &handler{svc: svc}
	admin := auth.RequireRole(domain.RoleAdmin)
	heads := auth.RequireRole(domain.RoleAdmin, domain.RoleManager)
	for path, fn := range map[string]http.HandlerFunc{
		"/analytics/summary":           heads(h.summary),
		"/analytics/by-category":       heads(h.byCategory),
		"/analytics/by-vendor":         heads(h.byVendor),
		"/analytics/forecast":          heads(h.forecast),
		"/analytics/by-department":     admin(h.byDepartment),
		"/analytics/top-subscriptions": admin(h.topSubscriptions),
		"/analytics/spending":          admin(h.spending),
		"/analytics/recommendations":   admin(h.recommendations),
		"/analytics/export":            admin(h.export),
	} {
		httpx.Register(protected, path, map[string]http.HandlerFunc{"GET": fn})
	}
}

// writeErr maps analytics errors onto HTTP statuses. Anything unrecognised
// is a 500.
func writeErr(ctx context.Context, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, apperr.ErrInvalidInput):
		httpx.Error(ctx, w, http.StatusBadRequest, err)
	case errors.Is(err, auth.ErrForbidden):
		httpx.Error(ctx, w, http.StatusForbidden, err)
	default:
		httpx.Internal(ctx, w, err)
	}
}
