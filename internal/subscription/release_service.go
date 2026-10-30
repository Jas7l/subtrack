package subscription

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"subtrack/internal/apperr"
	"subtrack/internal/auth"
	"subtrack/internal/domain"
	"subtrack/internal/logger"
	"subtrack/internal/paging"
	"subtrack/internal/storage"
)

var (
	// ErrReleaseNotFound is returned when the release request does not exist
	// or is outside the caller's view. Mapped to 404.
	ErrReleaseNotFound = apperr.New(apperr.ErrNotFound, "service: release request not found")

	// ErrReleaseNotPending is returned when the request was already decided
	// or cancelled. Mapped to 409.
	ErrReleaseNotPending = apperr.New(apperr.ErrConflict, "service: the release request is already decided or cancelled")

	// ErrReleasePending is returned when a request to give the licence back
	// already waits. Mapped to 409.
	ErrReleasePending = apperr.New(apperr.ErrConflict, "service: a release request for this licence already waits")

	// ErrRevokeYourself is returned when the holder asks to give back a
	// licence of a subscription they manage: they revoke it themselves.
	// Mapped to 409.
	ErrRevokeYourself = apperr.New(apperr.ErrConflict, "service: you manage this subscription: revoke the licence yourself")

	// ErrNotHolder is returned when someone who sees a licence without
	// holding it (a manager of its department) asks to give it back. Mapped
	// to 403.
	ErrNotHolder = fmt.Errorf("%w: only the licence holder asks to give it back", auth.ErrForbidden)

	// ErrNotReleaseReviewer is returned when someone who sees a release
	// request (its requester) decides it without managing the subscription.
	// Mapped to 403.
	ErrNotReleaseReviewer = fmt.Errorf("%w: only the admin or a manager of the subscription's department decides a release request", auth.ErrForbidden)

	// ErrNotReleaseRequester is returned when someone who sees a release
	// request cancels it without having made it. Mapped to 403.
	ErrNotReleaseRequester = fmt.Errorf("%w: only the requester can cancel a release request", auth.ErrForbidden)

	// ErrReleaseCommentRequired is returned for a rejection without a
	// comment. Mapped to 400.
	ErrReleaseCommentRequired = apperr.New(apperr.ErrInvalidInput, "service: invalid input: a rejection needs a comment")
)

// releaseVisible reports whether actor may see r: whoever manages the
// subscription (the admin, a manager of its department) and the requester.
func releaseVisible(actor domain.User, r domain.ReleaseRequest) bool {
	return r.RequesterID == actor.ID || manages(actor, r.DepartmentID)
}

// GetRelease returns one release request as actor is allowed to see it.
//
// Errors:
//   - ErrReleaseNotFound when it does not exist or is outside actor's view.
//   - any other datastore failure, propagated.
func (s *Service) GetRelease(ctx context.Context, actor domain.User, id int64) (domain.ReleaseRequest, error) {
	logger.Op(ctx, "subscription.Service.GetRelease").Debug("start", "id", id)
	r, err := s.repo.GetRelease(ctx, id)
	if err != nil {
		return domain.ReleaseRequest{}, apperr.MapStorage(err, ErrReleaseNotFound)
	}
	if !releaseVisible(actor, r) {
		return domain.ReleaseRequest{}, ErrReleaseNotFound
	}
	return r, nil
}

// ListReleases returns one page of the release requests visible to actor
// that match f, newest first: all for the admin; for a manager those about
// their department's subscriptions plus their own; for an employee their
// own.
//
// inbox narrows the list to the pending requests waiting for actor: for a
// manager those of their department's subscriptions; for the admin only
// those nobody else can decide (a company-wide subscription, a department
// without an active manager) - the rest are the managers', though the admin
// may decide them too. An employee's inbox is empty.
func (s *Service) ListReleases(ctx context.Context, actor domain.User, f domain.ReleaseFilter, inbox bool, p paging.Params) ([]domain.ReleaseRequest, int64, error) {
	log := logger.Op(ctx, "subscription.Service.ListReleases")
	p.Normalize()
	if inbox {
		pending := domain.ReleasePending
		f.Status = &pending
		switch {
		case actor.Role == domain.RoleAdmin:
			f.Unattended = true
		case actor.Role == domain.RoleManager && actor.DepartmentID != nil:
			f.DepartmentID = actor.DepartmentID
		default:
			return []domain.ReleaseRequest{}, 0, nil
		}
	}
	items, total, err := s.repo.ListReleases(ctx, f, scopeFor(actor), p.Limit, p.Offset)
	if err == nil {
		log.Debug("done", "count", len(items), "total", total, "inbox", inbox, "limit", p.Limit, "offset", p.Offset)
	}
	return items, total, err
}

// FileRelease files actor's request to give back the licence assignmentID,
// with an optional reason. It goes to whoever manages the subscription: its
// department's active managers are notified, or every active admin when
// there are none or the subscription is company-wide.
//
// Errors:
//   - ErrAssignmentNotFound when the licence does not exist or is outside
//     actor's view.
//   - ErrNotHolder when actor sees it but does not hold it.
//   - ErrAlreadyRevoked when it is revoked.
//   - ErrRevokeYourself when actor manages the subscription.
//   - ErrReleasePending when a request for it already waits.
//   - any other datastore failure, propagated.
func (s *Service) FileRelease(ctx context.Context, actor domain.User, assignmentID int64, reason string) (domain.ReleaseRequest, error) {
	log := logger.Op(ctx, "subscription.Service.FileRelease")
	log.Debug("start", "assignment_id", assignmentID)

	a, err := s.GetAssignment(ctx, actor, assignmentID)
	if err != nil {
		return domain.ReleaseRequest{}, err
	}
	switch {
	case a.UserID != actor.ID:
		return domain.ReleaseRequest{}, ErrNotHolder
	case a.Status == domain.AssignmentRevoked:
		return domain.ReleaseRequest{}, ErrAlreadyRevoked
	case manages(actor, a.SubscriptionDepartmentID):
		return domain.ReleaseRequest{}, ErrRevokeYourself
	case a.ReleaseRequestID != nil:
		return domain.ReleaseRequest{}, ErrReleasePending
	}

	var out domain.ReleaseRequest
	err = s.tx.InTx(ctx, func(ctx context.Context) error {
		var err error
		out, err = s.repo.CreateRelease(ctx, domain.ReleaseRequest{
			AssignmentID: a.ID,
			RequesterID:  actor.ID,
			Reason:       strings.TrimSpace(reason),
		})
		if err != nil {
			return err
		}
		if err := s.audit.Record(ctx, domain.NewAuditEntry(actor.ID, "release_request.create", "release_request", out.ID,
			map[string]any{"assignment_id": a.ID, "subscription_id": out.SubscriptionID,
				"subscription": out.SubscriptionName, "reason": out.Reason})); err != nil {
			return err
		}
		return s.notifyReviewers(ctx, out)
	})
	if err != nil {
		return domain.ReleaseRequest{}, mapReleaseErr(err)
	}
	log.Info("release request filed", "id", out.ID, "assignment_id", a.ID,
		"subscription_id", out.SubscriptionID, "department_id", logger.Val(out.DepartmentID))
	return out, nil
}

// DecideRelease applies actor's verdict to the release request id. Whoever
// manages the subscription decides: the admin or a manager of its
// department - the requester too, if they came to manage it while the
// request waited (they may revoke the licence anyway). Approval revokes the
// licence in the same transaction; rejection needs a comment. The requester
// is notified either way.
//
// The subscription and the request are locked first, so the decision is made
// on the department and status as they are, and nobody revokes the licence
// meanwhile.
//
// Errors:
//   - ErrReleaseCommentRequired for a rejection without a comment.
//   - ErrReleaseNotFound when it does not exist or is outside actor's view.
//   - ErrNotReleaseReviewer when actor sees it (made it) but does not manage
//     the subscription.
//   - ErrReleaseNotPending when it was already decided or cancelled.
//   - any other datastore failure, propagated.
func (s *Service) DecideRelease(ctx context.Context, actor domain.User, id int64, d domain.ReleaseDecision) (domain.ReleaseRequest, error) {
	log := logger.Op(ctx, "subscription.Service.DecideRelease")
	log.Debug("start", "id", id, "approve", d.Approve)

	d.ReviewerID = actor.ID
	d.Comment = strings.TrimSpace(d.Comment)
	if !d.Approve && d.Comment == "" {
		return domain.ReleaseRequest{}, ErrReleaseCommentRequired
	}

	var out domain.ReleaseRequest
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		r, err := s.repo.LockRelease(ctx, id)
		if err != nil {
			return apperr.MapStorage(err, ErrReleaseNotFound)
		}
		switch {
		case !releaseVisible(actor, r):
			return ErrReleaseNotFound
		case !manages(actor, r.DepartmentID):
			return ErrNotReleaseReviewer
		case r.Status != domain.ReleasePending:
			return ErrReleaseNotPending
		}
		if d.Approve {
			// A pending request points at an active licence, and the
			// subscription is held: the revoke cannot miss.
			a, err := s.repo.Revoke(ctx, r.AssignmentID, actor.ID)
			if err != nil {
				return apperr.MapStorage(err, ErrAlreadyRevoked)
			}
			if err := s.audit.Record(ctx, domain.NewAuditEntry(actor.ID, "assignment.revoke", "assignment", a.ID,
				map[string]any{"subscription_id": a.SubscriptionID, "subscription": a.SubscriptionName,
					"user_id": a.UserID, "user": a.UserName, "release_request_id": r.ID})); err != nil {
				return err
			}
		}
		out, err = s.repo.DecideRelease(ctx, id, d)
		if err != nil {
			return apperr.MapStorage(err, ErrReleaseNotPending)
		}
		action := "release_request.reject"
		if d.Approve {
			action = "release_request.approve"
		}
		if err := s.audit.Record(ctx, domain.NewAuditEntry(actor.ID, action, "release_request", id,
			map[string]any{"assignment_id": out.AssignmentID, "subscription_id": out.SubscriptionID,
				"subscription": out.SubscriptionName, "user_id": out.RequesterID, "user": out.RequesterName,
				"comment": d.Comment})); err != nil {
			return err
		}
		return s.notifyRequester(ctx, out)
	})
	if err != nil {
		return domain.ReleaseRequest{}, err
	}
	log.Info("release request decided", "id", id, "status", out.Status, "by", actor.ID)
	return out, nil
}

// CancelRelease withdraws a pending release request on behalf of its
// requester.
//
// Errors:
//   - ErrReleaseNotFound when it does not exist or is outside actor's view.
//   - ErrNotReleaseRequester when actor sees it but did not make it.
//   - ErrReleaseNotPending when it was already decided or cancelled.
//   - any other datastore failure, propagated.
func (s *Service) CancelRelease(ctx context.Context, actor domain.User, id int64) (domain.ReleaseRequest, error) {
	log := logger.Op(ctx, "subscription.Service.CancelRelease")
	r, err := s.GetRelease(ctx, actor, id)
	if err != nil {
		return domain.ReleaseRequest{}, err
	}
	if r.RequesterID != actor.ID {
		return domain.ReleaseRequest{}, ErrNotReleaseRequester
	}
	if r.Status != domain.ReleasePending {
		return domain.ReleaseRequest{}, ErrReleaseNotPending
	}
	var out domain.ReleaseRequest
	err = s.tx.InTx(ctx, func(ctx context.Context) error {
		var err error
		out, err = s.repo.CancelRelease(ctx, id, actor.ID)
		if err != nil {
			return err
		}
		return s.audit.Record(ctx, domain.NewAuditEntry(actor.ID, "release_request.cancel", "release_request", id,
			map[string]any{"subscription_id": out.SubscriptionID, "subscription": out.SubscriptionName}))
	})
	if errors.Is(err, storage.ErrNotFound) {
		return domain.ReleaseRequest{}, ErrReleaseNotPending // decided meanwhile
	}
	if err != nil {
		return domain.ReleaseRequest{}, err
	}
	log.Info("release request cancelled", "id", id)
	return out, nil
}

// closeReleases closes, on behalf of actor, the release requests of
// subscription subID whose licence was just revoked another way, and tells
// their requesters. It runs in the transaction that revoked. It returns how
// many it closed.
func (s *Service) closeReleases(ctx context.Context, actor domain.User, subID int64) (int, error) {
	closed, err := s.repo.CloseReleases(ctx, subID, actor.ID)
	if err != nil {
		return 0, err
	}
	for _, r := range closed {
		if err := s.notifyRequester(ctx, r); err != nil {
			return 0, err
		}
	}
	return len(closed), nil
}

// notifyReviewers tells whoever decides r that it waits: the active managers
// of the subscription's department, or every active admin when there are
// none or the subscription is company-wide.
func (s *Service) notifyReviewers(ctx context.Context, r domain.ReleaseRequest) error {
	body := r.RequesterName + " просит отозвать свою лицензию «" + r.SubscriptionName + "»."
	if r.Reason != "" {
		body += " Причина: " + r.Reason
	}
	n := domain.Notification{
		Type:       domain.NotifyReleaseCreated,
		Title:      "Отказ от лицензии: " + r.SubscriptionName,
		Body:       body,
		EntityType: domain.EntityRelease,
		EntityID:   r.ID,
	}
	if r.DepartmentID != nil {
		sent, err := s.notify.SendToManagers(ctx, *r.DepartmentID, n)
		if err != nil || sent > 0 {
			return err
		}
	}
	return s.notify.SendToAdmins(ctx, n)
}

// notifyRequester tells the requester how r ended: approved (the licence is
// revoked) or rejected (it stays theirs), with the comment if any.
func (s *Service) notifyRequester(ctx context.Context, r domain.ReleaseRequest) error {
	n := domain.Notification{
		UserID:     r.RequesterID,
		Type:       domain.NotifyReleaseApproved,
		Title:      "Отказ принят: " + r.SubscriptionName,
		Body:       withComment("Лицензия «"+r.SubscriptionName+"» отозвана.", r.Comment),
		EntityType: domain.EntityRelease,
		EntityID:   r.ID,
	}
	if r.Status == domain.ReleaseRejected {
		n.Type = domain.NotifyReleaseRejected
		n.Title = "Отказ отклонён: " + r.SubscriptionName
		n.Body = withComment("Лицензия «"+r.SubscriptionName+"» остаётся за вами.", r.Comment)
	}
	return s.notify.Send(ctx, n)
}

// withComment appends the reviewer's comment, if any, to a notification
// body.
func withComment(body, comment string) string {
	if comment == "" {
		return body
	}
	return body + " Комментарий: " + comment
}

// mapReleaseErr translates the errors of repo.CreateRelease.
func mapReleaseErr(err error) error {
	var unique *storage.ConstraintError
	switch {
	case errors.Is(err, errLicenceGone):
		return ErrAlreadyRevoked
	case errors.As(err, &unique) && unique.Constraint == "release_requests_pending_key":
		return ErrReleasePending
	}
	return err
}
