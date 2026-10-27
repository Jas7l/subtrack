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
	"time"

	"github.com/gorilla/mux"
	"github.com/jackc/pgx/v5/pgxpool"

	"subtrack/internal/analytics"
	"subtrack/internal/audit"
	"subtrack/internal/auth"
	"subtrack/internal/currency"
	"subtrack/internal/department"
	"subtrack/internal/httpx"
	"subtrack/internal/notification"
	"subtrack/internal/pg"
	"subtrack/internal/request"
	"subtrack/internal/scheduler"
	"subtrack/internal/settings"
	"subtrack/internal/subscription"
	"subtrack/internal/transaction"
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
//
// loc is the company's time zone (SCHEDULER_TZ): it decides what "today" and
// "this month" are in the reports, as it does for the scheduler.
func Mount(ctx context.Context, r *mux.Router, pool *pgxpool.Pool, authCfg auth.Config, loc *time.Location) error {
	authSvc := routes(r, pool, authCfg, loc)
	return authSvc.EnsureAdmin(ctx, authCfg.Bootstrap)
}

// routes builds the domains and registers their routes; it does not touch
// the database, so a test can build the router over a nil pool.
func routes(r *mux.Router, pool *pgxpool.Pool, authCfg auth.Config, loc *time.Location) *auth.Service {
	r.NotFoundHandler = http.HandlerFunc(httpx.RouteNotFound)
	api := r.PathPrefix("/api/v1").Subrouter()
	api.NotFoundHandler = r.NotFoundHandler

	// Every write runs in a transaction (tx) together with its audit record;
	// stores join it through pg.Conn.
	tx := pg.NewTxRunner(pool)
	trail := audit.NewStore(pool)
	inbox := notification.NewStore(pool)
	users := user.NewStore(pool)
	depts := department.NewStore(pool)
	authSvc := auth.New(users, authCfg, tx, trail)

	// The protected subrouter matches every path under /api/v1 but has no
	// NotFoundHandler of its own, so a path none of its routes match falls
	// back to api's 404 instead of being answered here. Its middleware only
	// runs for a matched route: an unknown path is a 404, not a 401.
	protected := api.NewRoute().Subrouter()
	protected.Use(authSvc.Middleware)

	auth.RegisterRoutes(api, protected, authSvc)
	user.RegisterRoutes(protected, user.New(users, depts, tx, trail))
	department.RegisterRoutes(protected, department.New(depts, users, tx, trail))
	subStore := subscription.NewStore(pool)
	subs := subscription.New(subStore, users, depts, tx, trail)
	subscription.RegisterRoutes(protected, subs)
	request.RegisterRoutes(protected, request.New(request.NewStore(pool), depts, subs, tx, trail, inbox))
	notification.RegisterRoutes(protected, notification.New(inbox))
	audit.RegisterRoutes(protected, audit.New(trail))
	settings.RegisterRoutes(protected, settings.New(settings.NewStore(pool), tx, trail))
	currency.RegisterRoutes(protected, currency.New(currency.NewStore(pool), tx, trail))
	transaction.RegisterRoutes(protected, transaction.New(transaction.NewStore(pool), subStore, tx, trail))
	analytics.RegisterRoutes(protected, analytics.New(analytics.NewStore(pool), loc))
	return authSvc
}

// NewScheduler builds the background date checker over the same pool. The
// caller runs it (Scheduler.Run) and stops it by cancelling its context.
func NewScheduler(pool *pgxpool.Pool, cfg scheduler.Config) *scheduler.Scheduler {
	return scheduler.New(scheduler.NewStore(pool), settings.NewStore(pool), pg.NewTxRunner(pool),
		audit.NewStore(pool), notification.NewStore(pool), cfg)
}
