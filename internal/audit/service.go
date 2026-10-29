// Package audit keeps the trail of who changed what. Every domain records its
// changes through *Store.Record (behind a small interface of its own) in the
// same transaction as the change; this package's service and routes only
// serve the trail to the admin.
//
// Actions are named "<entity>.<verb>"; the entity types are user,
// department, subscription, assignment, request, settings and transaction.
package audit

import (
	"context"
	"strings"
	"time"

	"subtrack/internal/domain"
	"subtrack/internal/logger"
	"subtrack/internal/paging"
)

// repo is what the service needs; *Store satisfies it.
type repo interface {
	List(ctx context.Context, f domain.AuditFilter, limit, offset int) ([]domain.AuditEntry, int64, error)
}

// Service serves the audit trail.
type Service struct {
	repo repo
	// loc is the company's time zone (SCHEDULER_TZ): the days of the from/to
	// filters begin at its midnight.
	loc *time.Location
}

// New wires a Service over the audit repo. loc is the company's time zone.
func New(r repo, loc *time.Location) *Service {
	return &Service{repo: r, loc: loc}
}

// List returns one page of entries matching f, newest first. Access is
// limited to the admin by the routes.
func (s *Service) List(ctx context.Context, f domain.AuditFilter, p paging.Params) ([]domain.AuditEntry, int64, error) {
	log := logger.Op(ctx, "audit.Service.List")
	p.Normalize()
	f.EntityType = strings.TrimSpace(f.EntityType)
	f.Action = strings.TrimSpace(f.Action)
	items, total, err := s.repo.List(ctx, f, p.Limit, p.Offset)
	if err == nil {
		log.Debug("done", "count", len(items), "total", total, "limit", p.Limit, "offset", p.Offset)
	}
	return items, total, err
}
