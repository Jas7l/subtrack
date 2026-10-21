// Package server is the composition root of the SubTrack backend. It wires
// every domain package's Store + Service together against a single pgx pool
// and mounts their routes under /api/v1. It is the one place that imports
// every domain package; the domains themselves never import each other
// (cross-domain reads go through small unexported interfaces satisfied
// structurally by the neighbouring Store). The one exception is auth, which is
// cross-cutting: domains import it to learn who the caller is.
package server

import (
	"context"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/jackc/pgx/v5/pgxpool"

	"subtrack/internal/auth"
	"subtrack/internal/httpx"
	"subtrack/internal/user"
)

// Mount builds every domain Store + Service from the given pool and registers
// their routes on a fresh /api/v1 subrouter of r. It also installs the
// router-level NotFound handler so unrouted requests get the same JSON error
// body (per-path 405 fallbacks are added by each domain via httpx.Register).
//
// Routes are split in two: public ones (only POST /auth/login) and protected
// ones, which sit behind auth.Middleware and so always have a signed-in user
// in their context. Role checks are per handler, via auth.RequireRole.
//
// Mount also creates the bootstrap admin when the database has none (see
// auth.Service.EnsureAdmin); an error from that stops startup.
func Mount(ctx context.Context, r *mux.Router, pool *pgxpool.Pool, authCfg auth.Config) error {
	r.NotFoundHandler = http.HandlerFunc(httpx.RouteNotFound)
	api := r.PathPrefix("/api/v1").Subrouter()
	api.NotFoundHandler = r.NotFoundHandler

	users := user.NewStore(pool)
	authSvc := auth.New(users, authCfg)
	if err := authSvc.EnsureAdmin(ctx, authCfg.Bootstrap); err != nil {
		return err
	}

	// The protected subrouter matches every path under /api/v1 but has no
	// NotFoundHandler of its own, so a path none of its routes match falls
	// back to api's 404 instead of being answered here. Its middleware only
	// runs for a matched route: an unknown path is a 404, not a 401.
	protected := api.NewRoute().Subrouter()
	protected.Use(authSvc.Middleware)

	auth.RegisterRoutes(api, protected, authSvc)
	return nil
}
