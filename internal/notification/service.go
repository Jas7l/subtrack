// Package notification is the in-app inbox: every user reads their own
// notifications and marks them read. Other domains send notifications
// through *Store.Send / SendToAdmins (behind a small interface of their own)
// inside the transaction of the change that causes them.
//
// Reading a notification changes only the reader's own inbox and is not
// written to the audit trail.
package notification

import (
	"context"

	"subtrack/internal/apperr"
	"subtrack/internal/domain"
	"subtrack/internal/logger"
	"subtrack/internal/paging"
)

// ErrNotificationNotFound is returned when the caller has no notification
// with that id. Mapped to 404.
var ErrNotificationNotFound = apperr.New(apperr.ErrNotFound, "service: notification not found")

// repo is what the service needs; *Store satisfies it.
type repo interface {
	List(ctx context.Context, userID int64, unreadOnly bool, limit, offset int) ([]domain.Notification, int64, int64, error)
	MarkRead(ctx context.Context, userID, id int64) (domain.Notification, error)
	MarkAllRead(ctx context.Context, userID int64) (int64, error)
}

// Service serves a user's own inbox.
type Service struct {
	repo repo
}

// New wires a Service over the notifications repo.
func New(r repo) *Service {
	return &Service{repo: r}
}

// List returns one page of actor's notifications (only unread ones when
// unreadOnly), newest first, with the total and the unread count.
func (s *Service) List(ctx context.Context, actor domain.User, unreadOnly bool, p paging.Params) ([]domain.Notification, int64, int64, error) {
	log := logger.Op(ctx, "notification.Service.List")
	p.Normalize()
	items, total, unread, err := s.repo.List(ctx, actor.ID, unreadOnly, p.Limit, p.Offset)
	if err == nil {
		log.Debug("done", "count", len(items), "total", total, "unread", unread)
	}
	return items, total, unread, err
}

// MarkRead marks one of actor's notifications read.
//
// Errors:
//   - ErrNotificationNotFound when actor has no notification with that id.
//   - any other datastore failure, propagated.
func (s *Service) MarkRead(ctx context.Context, actor domain.User, id int64) (domain.Notification, error) {
	n, err := s.repo.MarkRead(ctx, actor.ID, id)
	return n, apperr.MapStorage(err, ErrNotificationNotFound)
}

// MarkAllRead marks all of actor's notifications read and returns how many
// were unread.
func (s *Service) MarkAllRead(ctx context.Context, actor domain.User) (int64, error) {
	return s.repo.MarkAllRead(ctx, actor.ID)
}
