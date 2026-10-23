package subscription

import (
	"context"
	"errors"

	"subtrack/internal/apperr"
	"subtrack/internal/auth"
	"subtrack/internal/domain"
	"subtrack/internal/logger"
	"subtrack/internal/paging"
	"subtrack/internal/storage"
)

var (
	// ErrAssignmentNotFound is returned when the assignment does not exist or
	// is outside the caller's view. Mapped to 404.
	ErrAssignmentNotFound = errors.New("service: assignment not found")

	// ErrInvalidAssignee is returned when the user to get a licence does not
	// exist, is blocked, or (for a manager) is not in the manager's
	// department. Mapped to 400.
	ErrInvalidAssignee = errors.New("service: invalid input: the licence can go only to an active user (a manager: of their own department)")

	// ErrSubscriptionClosed is returned when licences of a cancelled or
	// expired subscription are handed out. Mapped to 409.
	ErrSubscriptionClosed = errors.New("service: the subscription is cancelled or expired")

	// ErrAlreadyAssigned is returned when the user already holds a licence of
	// the subscription. Mapped to 409.
	ErrAlreadyAssigned = errors.New("service: the user already holds a licence of this subscription")

	// ErrNoFreeSeats is returned when every seat is taken. Mapped to 409.
	ErrNoFreeSeats = errors.New("service: no free seats left")

	// ErrAlreadyRevoked is returned when the licence was revoked before.
	// Mapped to 409.
	ErrAlreadyRevoked = errors.New("service: the licence is already revoked")
)

// GetAssignment returns one assignment as actor is allowed to see it: the
// admin sees all, a manager those of their department's subscriptions, and
// everyone their own.
//
// Errors:
//   - ErrAssignmentNotFound when it does not exist or is outside actor's view.
//   - any other datastore failure, propagated.
func (s *Service) GetAssignment(ctx context.Context, actor domain.User, id int64) (domain.Assignment, error) {
	logger.Op(ctx, "subscription.Service.GetAssignment").Debug("start", "id", id)
	a, err := s.repo.GetAssignment(ctx, id)
	if err != nil {
		return domain.Assignment{}, apperr.MapStorage(err, ErrAssignmentNotFound)
	}
	if a.UserID != actor.ID && !manages(actor, a.SubscriptionDepartmentID) {
		return domain.Assignment{}, ErrAssignmentNotFound
	}
	return a, nil
}

// ListAssignments returns one page of the assignments visible to actor that
// match f, newest first.
func (s *Service) ListAssignments(ctx context.Context, actor domain.User, f domain.AssignmentFilter, p paging.Params) ([]domain.Assignment, int64, error) {
	log := logger.Op(ctx, "subscription.Service.ListAssignments")
	p.Normalize()
	items, total, err := s.repo.ListAssignments(ctx, f, scopeFor(actor), p.Limit, p.Offset)
	if err == nil {
		log.Debug("done", "count", len(items), "total", total, "limit", p.Limit, "offset", p.Offset)
	}
	return items, total, err
}

// Assign gives userID a licence of subID on behalf of actor. The admin may
// give any open subscription to any active user; a manager only their
// department's subscriptions to the people of their department.
//
// Errors:
//   - ErrSubscriptionNotFound when the subscription does not exist or is
//     outside actor's view.
//   - auth.ErrForbidden when actor sees it (holds a licence) but does not
//     manage it.
//   - ErrInvalidAssignee, ErrSubscriptionClosed, ErrAlreadyAssigned,
//     ErrNoFreeSeats as their names say.
//   - any other datastore failure, propagated.
func (s *Service) Assign(ctx context.Context, actor domain.User, subID, userID int64) (domain.Assignment, error) {
	log := logger.Op(ctx, "subscription.Service.Assign")
	log.Debug("start", "subscription_id", subID, "user_id", userID)

	sub, err := s.Get(ctx, actor, subID)
	if err != nil {
		return domain.Assignment{}, err
	}
	if !manages(actor, sub.DepartmentID) {
		return domain.Assignment{}, auth.ErrForbidden
	}
	if !sub.Status.Open() {
		return domain.Assignment{}, ErrSubscriptionClosed
	}

	u, err := s.users.GetByID(ctx, userID)
	if errors.Is(err, storage.ErrNotFound) {
		return domain.Assignment{}, ErrInvalidAssignee
	}
	if err != nil {
		return domain.Assignment{}, err
	}
	if !u.IsActive {
		return domain.Assignment{}, ErrInvalidAssignee
	}
	if actor.Role != domain.RoleAdmin &&
		(u.DepartmentID == nil || actor.DepartmentID == nil || *u.DepartmentID != *actor.DepartmentID) {
		return domain.Assignment{}, ErrInvalidAssignee
	}

	held, err := s.repo.HasActiveAssignment(ctx, subID, userID)
	if err != nil {
		return domain.Assignment{}, err
	}
	if held {
		return domain.Assignment{}, ErrAlreadyAssigned
	}
	if sub.SeatsTotal != nil && sub.SeatsUsed >= *sub.SeatsTotal {
		return domain.Assignment{}, ErrNoFreeSeats
	}

	a, err := s.repo.Assign(ctx, subID, userID, actor.ID)
	if err != nil {
		return domain.Assignment{}, mapAssignErr(err)
	}
	log.Info("licence assigned", "id", a.ID, "subscription_id", subID, "user_id", userID)
	return a, nil
}

// Revoke ends a licence on behalf of actor and frees its seat. Whoever may
// see the assignment may revoke it: the admin, the manager of the
// subscription's department, and the holder giving it back.
//
// Errors:
//   - ErrAssignmentNotFound when it does not exist or is outside actor's view.
//   - ErrAlreadyRevoked when it was revoked before.
//   - any other datastore failure, propagated.
func (s *Service) Revoke(ctx context.Context, actor domain.User, id int64) (domain.Assignment, error) {
	log := logger.Op(ctx, "subscription.Service.Revoke")
	a, err := s.GetAssignment(ctx, actor, id)
	if err != nil {
		return domain.Assignment{}, err
	}
	if a.Status == domain.AssignmentRevoked {
		return domain.Assignment{}, ErrAlreadyRevoked
	}
	out, err := s.repo.Revoke(ctx, id, actor.ID)
	if errors.Is(err, storage.ErrNotFound) {
		// Revoked by someone else (or with the whole subscription) meanwhile.
		return domain.Assignment{}, ErrAlreadyRevoked
	}
	if err != nil {
		return domain.Assignment{}, err
	}
	log.Info("licence revoked", "id", id, "subscription_id", out.SubscriptionID, "user_id", out.UserID)
	return out, nil
}

// mapAssignErr translates the errors of repo.Assign.
func mapAssignErr(err error) error {
	var check *storage.CheckError
	var unique *storage.ConstraintError
	switch {
	case errors.Is(err, errAssigneeGone):
		return ErrInvalidAssignee
	case errors.Is(err, errSubscriptionGone):
		return ErrSubscriptionClosed
	case errors.As(err, &check) && check.Constraint == "subscriptions_seats_check":
		return ErrNoFreeSeats
	case errors.As(err, &unique) && unique.Constraint == "subscription_assignments_active_key":
		return ErrAlreadyAssigned
	}
	return err
}
