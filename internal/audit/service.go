// Package audit keeps the trail of who changed what. Every domain records its
// changes through *Store.Record (behind a small interface of its own) in the
// same transaction as the change; this package's service and routes only
// serve the trail to the admin.
//
// Actions are named "<entity>.<verb>"; the entity types are user,
// department, subscription, assignment and request.
package audit

import (
	"context"
	"strings"

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
}

// New wires a Service over the audit repo.
func New(r repo) *Service {
	return &Service{repo: r}
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
