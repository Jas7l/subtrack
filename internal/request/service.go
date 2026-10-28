// Package request runs requests for software through approval: an employee
// asks, the head of their department approves or rejects, the admin gives
// the final word and, on approval, hands the requester a licence of an
// existing subscription or of one created on the spot.
//
// The state machine (see domain.RequestStatus):
//   - an employee's request starts at pending_manager, or at pending_admin
//     when their department has no head (or they have no department);
//   - a manager's request starts at pending_admin;
//   - only the current head of the request's department decides at
//     pending_manager; the admin decides at pending_admin, and at
//     pending_manager too when the department has no head (any more);
//   - the requester may cancel while the request is pending;
//   - a rejection needs a comment.
//
// The admin's approval runs in one transaction: create the subscription (if
// new), hand out the licence, mark the request approved. If the licence
// cannot be handed out, nothing changes and the request stays pending.
//
// Every change is recorded in the audit trail and notifies people, in the
// same transaction: the reviewer whose turn it is (the department head, or
// every active admin) and the requester on every decision.
package request

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"subtrack/internal/apperr"
	"subtrack/internal/auth"
	"subtrack/internal/domain"
	"subtrack/internal/logger"
	"subtrack/internal/paging"
	"subtrack/internal/storage"
)

var (
	// ErrRequestNotFound is returned when the request does not exist or is
	// outside the caller's view. Mapped to 404.
	ErrRequestNotFound = apperr.New(apperr.ErrNotFound, "service: request not found")

	// ErrNotPending is returned when the request was already decided or
	// cancelled. Mapped to 409.
	ErrNotPending = apperr.New(apperr.ErrConflict, "service: the request is already decided or cancelled")

	// ErrWaitsForAdmin is returned when a manager acts on a request that is
	// already at the admin's stage. Mapped to 409.
	ErrWaitsForAdmin = apperr.New(apperr.ErrConflict, "service: the request already waits for the admin")

	// ErrWaitsForHead is returned when the admin acts on a request that waits
	// for its department head. Mapped to 409.
	ErrWaitsForHead = apperr.New(apperr.ErrConflict, "service: the request waits for the department head")

	// ErrNotHead is returned when a manager who can see the request (same
	// department) is not its department's head. Mapped to 403.
	ErrNotHead = fmt.Errorf("%w: only the head of the department reviews its requests", auth.ErrForbidden)

	// ErrOwnRequest is returned when the reviewer is the requester: someone
	// promoted to head or admin while their request waited. Mapped to 403.
	ErrOwnRequest = fmt.Errorf("%w: you cannot decide your own request", auth.ErrForbidden)

	// ErrNotRequester is returned when someone who can see a request tries to
	// cancel it without having made it. Mapped to 403.
	ErrNotRequester = fmt.Errorf("%w: only the requester can cancel a request", auth.ErrForbidden)

	// ErrCommentRequired is returned for a rejection without a comment.
	// Mapped to 400.
	ErrCommentRequired = apperr.New(apperr.ErrInvalidInput, "service: invalid input: a rejection needs a comment")

	// ErrUnknownCurrency is returned when the currency is not in the
	// currencies table. Mapped to 400.
	ErrUnknownCurrency = apperr.New(apperr.ErrInvalidInput, "service: invalid input: unknown currency")
)

// repo is the slice of requests persistence the service needs; *Store
// satisfies it.
type repo interface {
	GetByID(ctx context.Context, id int64) (domain.Request, error)
	GetForUpdate(ctx context.Context, id int64) (domain.Request, error)
	List(ctx context.Context, f domain.RequestFilter, sc *domain.RequestScope, limit, offset int) ([]domain.Request, int64, error)
	CurrencyExists(ctx context.Context, code string) (bool, error)
	Create(ctx context.Context, r domain.Request) (domain.Request, error)
	DecideAsManager(ctx context.Context, id int64, d domain.Decision) (domain.Request, error)
	DecideAsAdmin(ctx context.Context, id int64, d domain.Decision) (domain.Request, error)
	Cancel(ctx context.Context, id, requesterID int64) (domain.Request, error)
}

// departments tells who heads a department; *department.Store satisfies it
// structurally.
type departments interface {
	GetByID(ctx context.Context, id int64) (domain.Department, error)
}

// subscriptions is what the admin's approval needs from the subscriptions
// domain; *subscription.Service satisfies it structurally. Its errors carry
// apperr kinds, which is how this package maps them without importing it.
type subscriptions interface {
	Create(ctx context.Context, actor domain.User, s domain.Subscription) (domain.Subscription, error)
	Assign(ctx context.Context, actor domain.User, subID, userID int64) (domain.Assignment, error)
}

// transactor runs fn in one transaction; *pg.TxRunner satisfies it.
type transactor interface {
	InTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// auditor records a change in the audit trail; *audit.Store satisfies it.
type auditor interface {
	Record(ctx context.Context, e domain.AuditEntry) error
}

// notifier stores in-app notifications; *notification.Store satisfies it.
type notifier interface {
	Send(ctx context.Context, n domain.Notification) error
	SendToAdmins(ctx context.Context, n domain.Notification) error
}

// Service implements the requests use cases.
type Service struct {
	repo   repo
	depts  departments
	subs   subscriptions
	tx     transactor
	audit  auditor
	notify notifier
}

// New wires a Service.
func New(r repo, d departments, subs subscriptions, tx transactor, a auditor, n notifier) *Service {
	return &Service{repo: r, depts: d, subs: subs, tx: tx, audit: a, notify: n}
}

// CreateRequestInput carries what the requester describes.
type CreateRequestInput struct {
	Name          string
	Vendor        string
	Justification string
	EstimatedCost *string
	// Currency of EstimatedCost; RUB when empty.
	Currency string
}

// Verdict is a reviewer's decision. Only the admin's approval picks the
// subscription: exactly one of SubscriptionID and NewSubscription.
type Verdict struct {
	Approve         bool
	Comment         string
	SubscriptionID  *int64
	NewSubscription *NewSubscription
}

// NewSubscription is a subscription the admin creates while approving. An
// empty Name or Vendor is taken from the request; DepartmentID left unset
// means the request's department (set to nil: company-wide).
type NewSubscription struct {
	Subscription domain.Subscription
	DepartmentID domain.IDPatch
}

// scopeFor returns what actor may see: everything for the admin; their
// department's requests plus their own for a manager; their own for an
// employee.
func scopeFor(actor domain.User) *domain.RequestScope {
	switch actor.Role {
	case domain.RoleAdmin:
		return nil
	case domain.RoleManager:
		return &domain.RequestScope{DepartmentID: actor.DepartmentID, RequesterID: &actor.ID}
	}
	return &domain.RequestScope{RequesterID: &actor.ID}
}

func visible(actor domain.User, r domain.Request) bool {
	switch {
	case actor.Role == domain.RoleAdmin, r.RequesterID == actor.ID:
		return true
	case actor.Role == domain.RoleManager:
		return actor.DepartmentID != nil && r.DepartmentID != nil && *actor.DepartmentID == *r.DepartmentID
	}
	return false
}

// Get returns one request as actor is allowed to see it.
//
// Errors:
//   - ErrRequestNotFound when it does not exist or is outside actor's view.
//   - any other datastore failure, propagated.
func (s *Service) Get(ctx context.Context, actor domain.User, id int64) (domain.Request, error) {
	logger.Op(ctx, "request.Service.Get").Debug("start", "id", id)
	r, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return domain.Request{}, apperr.MapStorage(err, ErrRequestNotFound)
	}
	if !visible(actor, r) {
		return domain.Request{}, ErrRequestNotFound
	}
	return r, nil
}

// List returns one page of the requests visible to actor that match f,
// newest first.
func (s *Service) List(ctx context.Context, actor domain.User, f domain.RequestFilter, p paging.Params) ([]domain.Request, int64, error) {
	log := logger.Op(ctx, "request.Service.List")
	p.Normalize()
	items, total, err := s.repo.List(ctx, f, scopeFor(actor), p.Limit, p.Offset)
	if err == nil {
		log.Debug("done", "count", len(items), "total", total, "limit", p.Limit, "offset", p.Offset)
	}
	return items, total, err
}

// Create files a request on behalf of actor (an employee or a manager) and
// routes it: to the department head, or straight to the admin for a manager
// or when there is no head.
//
// Errors:
//   - auth.ErrForbidden for the admin, who creates subscriptions directly.
//   - apperr.ErrInvalidInput for a missing or malformed field.
//   - ErrUnknownCurrency as its name says.
//   - any other datastore failure, propagated.
func (s *Service) Create(ctx context.Context, actor domain.User, in CreateRequestInput) (domain.Request, error) {
	log := logger.Op(ctx, "request.Service.Create")
	if actor.Role == domain.RoleAdmin {
		return domain.Request{}, fmt.Errorf("%w: the admin creates subscriptions directly", auth.ErrForbidden)
	}
	r := domain.Request{
		RequesterID:   actor.ID,
		DepartmentID:  actor.DepartmentID,
		Name:          strings.TrimSpace(in.Name),
		Vendor:        strings.TrimSpace(in.Vendor),
		Justification: strings.TrimSpace(in.Justification),
		Currency:      strings.ToUpper(strings.TrimSpace(in.Currency)),
	}
	if r.Currency == "" {
		r.Currency = "RUB"
	}
	var missing []string
	if r.Name == "" {
		missing = append(missing, "name")
	}
	if r.Justification == "" {
		missing = append(missing, "justification")
	}
	if len(missing) > 0 {
		return domain.Request{}, fmt.Errorf("%w: missing required fields: %s", apperr.ErrInvalidInput, strings.Join(missing, ", "))
	}
	if in.EstimatedCost != nil {
		c := strings.TrimSpace(*in.EstimatedCost)
		if !costRe.MatchString(c) {
			return domain.Request{}, fmt.Errorf("%w: estimated_cost must be a non-negative amount with at most 2 decimals, like 1500.00", apperr.ErrInvalidInput)
		}
		r.EstimatedCost = &c
	}
	if !currencyRe.MatchString(r.Currency) {
		return domain.Request{}, fmt.Errorf("%w: currency must be a three-letter code, like RUB", apperr.ErrInvalidInput)
	}
	ok, err := s.repo.CurrencyExists(ctx, r.Currency)
	if err != nil {
		return domain.Request{}, err
	}
	if !ok {
		return domain.Request{}, fmt.Errorf("%w %s", ErrUnknownCurrency, r.Currency)
	}

	r.Status = domain.ReqPendingAdmin
	var head *int64
	if actor.Role == domain.RoleEmployee {
		if head, err = s.headOf(ctx, r.DepartmentID); err != nil {
			return domain.Request{}, err
		}
		if head != nil {
			r.Status = domain.ReqPendingManager
		}
	}

	var out domain.Request
	err = s.tx.InTx(ctx, func(ctx context.Context) error {
		var err error
		out, err = s.repo.Create(ctx, r)
		if err != nil {
			return err
		}
		if err := s.audit.Record(ctx, domain.NewAuditEntry(actor.ID, "request.create", "request", out.ID,
			map[string]any{"name": out.Name, "status": string(out.Status)})); err != nil {
			return err
		}
		return s.notifyReviewer(ctx, out, head)
	})
	if err != nil {
		return domain.Request{}, err
	}
	log.Info("request created", "id", out.ID, "status", out.Status, "department_id", logger.Val(out.DepartmentID))
	return out, nil
}

// Decide applies actor's verdict to the request id: the department head's at
// pending_manager, the admin's at pending_admin (or at pending_manager when
// the department has no head).
//
// Errors:
//   - ErrRequestNotFound when it does not exist or is outside actor's view.
//   - ErrNotPending, ErrWaitsForAdmin, ErrWaitsForHead for the wrong stage.
//   - ErrNotHead (403) for a manager who is not the department head;
//     ErrOwnRequest (403) for the requester; auth.ErrForbidden for an
//     employee.
//   - ErrCommentRequired, apperr.ErrInvalidInput for a bad verdict.
//   - on the admin's approval, the subscription domain's errors (no free
//     seats, subscription not found, invalid new subscription...), each with
//     its apperr kind; the request is then left as it was.
//   - any other datastore failure, propagated.
func (s *Service) Decide(ctx context.Context, actor domain.User, id int64, v Verdict) (domain.Request, error) {
	log := logger.Op(ctx, "request.Service.Decide")
	log.Debug("start", "id", id, "approve", v.Approve)

	v.Comment = strings.TrimSpace(v.Comment)
	if !v.Approve && v.Comment == "" {
		return domain.Request{}, ErrCommentRequired
	}
	picked := v.SubscriptionID != nil || v.NewSubscription != nil
	if !v.Approve && picked {
		return domain.Request{}, fmt.Errorf("%w: a rejection takes no subscription", apperr.ErrInvalidInput)
	}

	var (
		out domain.Request
		err error
	)
	switch actor.Role {
	case domain.RoleManager:
		if picked {
			return domain.Request{}, fmt.Errorf("%w: only the admin picks the subscription", apperr.ErrInvalidInput)
		}
		err = s.tx.InTx(ctx, func(ctx context.Context) error {
			out, err = s.decideAsManager(ctx, actor, id, v)
			return err
		})
	case domain.RoleAdmin:
		if v.Approve && (v.SubscriptionID == nil) == (v.NewSubscription == nil) {
			return domain.Request{}, fmt.Errorf("%w: approve with either subscription_id or subscription, not both or neither", apperr.ErrInvalidInput)
		}
		err = s.tx.InTx(ctx, func(ctx context.Context) error {
			out, err = s.decideAsAdmin(ctx, actor, id, v)
			return err
		})
	default:
		return domain.Request{}, fmt.Errorf("%w: only a department head or the admin decides on requests", auth.ErrForbidden)
	}
	if err != nil {
		return domain.Request{}, err
	}
	log.Info("request decided", "id", id, "status", out.Status, "by", actor.ID, "subscription_id", logger.Val(out.SubscriptionID))
	return out, nil
}

func (s *Service) decideAsManager(ctx context.Context, actor domain.User, id int64, v Verdict) (domain.Request, error) {
	r, err := s.Get(ctx, actor, id)
	if err != nil {
		return domain.Request{}, err
	}
	switch r.Status {
	case domain.ReqPendingManager:
	case domain.ReqPendingAdmin:
		return domain.Request{}, ErrWaitsForAdmin
	default:
		return domain.Request{}, ErrNotPending
	}
	if r.RequesterID == actor.ID {
		return domain.Request{}, ErrOwnRequest
	}
	head, err := s.headOf(ctx, r.DepartmentID)
	if err != nil {
		return domain.Request{}, err
	}
	if head == nil || *head != actor.ID {
		return domain.Request{}, ErrNotHead
	}
	out, err := s.repo.DecideAsManager(ctx, id, domain.Decision{ReviewerID: actor.ID, Approve: v.Approve, Comment: v.Comment})
	if errors.Is(err, storage.ErrNotFound) {
		return domain.Request{}, ErrNotPending // decided or cancelled meanwhile
	}
	if err != nil {
		return domain.Request{}, err
	}
	if err := s.recordDecision(ctx, actor, out, "manager", v, nil); err != nil {
		return domain.Request{}, err
	}
	n := domain.Notification{
		UserID:     out.RequesterID,
		Type:       domain.NotifyRequestRejected,
		Title:      "Заявка отклонена: " + out.Name,
		Body:       withComment("Начальник отдела отклонил заявку.", v.Comment),
		EntityType: domain.EntityRequest,
		EntityID:   out.ID,
	}
	if v.Approve {
		n.Type = domain.NotifyRequestApproved
		n.Title = "Начальник одобрил заявку: " + out.Name
		n.Body = withComment("Заявка передана администратору.", v.Comment)
	}
	if err := s.notify.Send(ctx, n); err != nil {
		return domain.Request{}, err
	}
	if v.Approve {
		return out, s.notifyReviewer(ctx, out, nil)
	}
	return out, nil
}

// decideAsAdmin runs inside a transaction: the request row stays locked
// until the licence is handed out and the decision written.
func (s *Service) decideAsAdmin(ctx context.Context, actor domain.User, id int64, v Verdict) (domain.Request, error) {
	r, err := s.repo.GetForUpdate(ctx, id)
	if err != nil {
		return domain.Request{}, apperr.MapStorage(err, ErrRequestNotFound)
	}
	switch r.Status {
	case domain.ReqPendingAdmin:
	case domain.ReqPendingManager:
		head, err := s.headOf(ctx, r.DepartmentID)
		if err != nil {
			return domain.Request{}, err
		}
		if head != nil {
			return domain.Request{}, ErrWaitsForHead
		}
	default:
		return domain.Request{}, ErrNotPending
	}
	if r.RequesterID == actor.ID {
		return domain.Request{}, ErrOwnRequest
	}

	d := domain.Decision{ReviewerID: actor.ID, Approve: v.Approve, Comment: v.Comment}
	n := domain.Notification{
		UserID:     r.RequesterID,
		Type:       domain.NotifyRequestRejected,
		Title:      "Заявка отклонена: " + r.Name,
		Body:       withComment("Администратор отклонил заявку.", v.Comment),
		EntityType: domain.EntityRequest,
		EntityID:   r.ID,
	}
	if v.Approve {
		a, err := s.grant(ctx, actor, r, v)
		if err != nil {
			return domain.Request{}, err
		}
		d.SubscriptionID = &a.SubscriptionID
		n.Type = domain.NotifyRequestApproved
		n.Title = "Заявка одобрена: " + r.Name
		n.Body = withComment("Вам выдана лицензия «"+a.SubscriptionName+"».", v.Comment)
	}
	out, err := s.repo.DecideAsAdmin(ctx, id, d)
	if errors.Is(err, storage.ErrNotFound) {
		return domain.Request{}, ErrNotPending
	}
	if err != nil {
		return domain.Request{}, err
	}
	if err := s.recordDecision(ctx, actor, out, "admin", v, d.SubscriptionID); err != nil {
		return domain.Request{}, err
	}
	return out, s.notify.Send(ctx, n)
}

// recordDecision writes the audit entry of a decision at stage ("manager" or
// "admin").
func (s *Service) recordDecision(ctx context.Context, actor domain.User, r domain.Request, stage string, v Verdict, subID *int64) error {
	action := "request.reject"
	if v.Approve {
		action = "request.approve"
	}
	details := map[string]any{"stage": stage, "status": string(r.Status), "comment": v.Comment}
	if subID != nil {
		details["subscription_id"] = *subID
	}
	return s.audit.Record(ctx, domain.NewAuditEntry(actor.ID, action, "request", r.ID, details))
}

// notifyReviewer tells whoever decides next that r waits for them: head when
// it is at pending_manager, every active admin when it is at pending_admin.
func (s *Service) notifyReviewer(ctx context.Context, r domain.Request, head *int64) error {
	n := domain.Notification{
		Type:       domain.NotifyRequestCreated,
		Title:      "Новая заявка: " + r.Name,
		Body:       r.RequesterName + " просит «" + r.Name + "». Обоснование: " + r.Justification,
		EntityType: domain.EntityRequest,
		EntityID:   r.ID,
	}
	if r.Status == domain.ReqPendingManager && head != nil {
		n.UserID = *head
		return s.notify.Send(ctx, n)
	}
	return s.notify.SendToAdmins(ctx, n)
}

// withComment appends the reviewer's comment, if any, to a notification
// body.
func withComment(body, comment string) string {
	if comment == "" {
		return body
	}
	return body + " Комментарий: " + comment
}

// grant creates the subscription when the verdict brings a new one, then
// hands the requester a licence of it.
func (s *Service) grant(ctx context.Context, actor domain.User, r domain.Request, v Verdict) (domain.Assignment, error) {
	subID := int64(0)
	if v.SubscriptionID != nil {
		subID = *v.SubscriptionID
	} else {
		ns := v.NewSubscription.Subscription
		if strings.TrimSpace(ns.Name) == "" {
			ns.Name = r.Name
		}
		if strings.TrimSpace(ns.Vendor) == "" {
			ns.Vendor = r.Vendor
		}
		ns.DepartmentID = r.DepartmentID
		if v.NewSubscription.DepartmentID.Set {
			ns.DepartmentID = v.NewSubscription.DepartmentID.Value
		}
		sub, err := s.subs.Create(ctx, actor, ns)
		if err != nil {
			return domain.Assignment{}, err
		}
		subID = sub.ID
	}
	return s.subs.Assign(ctx, actor, subID, r.RequesterID)
}

// Cancel withdraws a pending request on behalf of its requester.
//
// Errors:
//   - ErrRequestNotFound when it does not exist or is outside actor's view.
//   - ErrNotRequester (403) when actor sees it but did not make it.
//   - ErrNotPending when it was already decided or cancelled.
//   - any other datastore failure, propagated.
func (s *Service) Cancel(ctx context.Context, actor domain.User, id int64) (domain.Request, error) {
	log := logger.Op(ctx, "request.Service.Cancel")
	r, err := s.Get(ctx, actor, id)
	if err != nil {
		return domain.Request{}, err
	}
	if r.RequesterID != actor.ID {
		return domain.Request{}, ErrNotRequester
	}
	if !r.Status.Pending() {
		return domain.Request{}, ErrNotPending
	}
	var out domain.Request
	err = s.tx.InTx(ctx, func(ctx context.Context) error {
		var err error
		out, err = s.repo.Cancel(ctx, id, actor.ID)
		if err != nil {
			return err
		}
		return s.audit.Record(ctx, domain.NewAuditEntry(actor.ID, "request.cancel", "request", id, nil))
	})
	if errors.Is(err, storage.ErrNotFound) {
		return domain.Request{}, ErrNotPending
	}
	if err != nil {
		return domain.Request{}, err
	}
	log.Info("request cancelled", "id", id)
	return out, nil
}

// headOf returns the id of the head of department dept, or nil when dept is
// nil, gone, or has no head.
func (s *Service) headOf(ctx context.Context, dept *int64) (*int64, error) {
	if dept == nil {
		return nil, nil
	}
	d, err := s.depts.GetByID(ctx, *dept)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return d.HeadID, nil
}

// costRe is a non-negative amount that fits NUMERIC(14,2).
var costRe = regexp.MustCompile(`^[0-9]{1,12}(\.[0-9]{1,2})?$`)

// currencyRe is an ISO 4217 code, after upper-casing.
var currencyRe = regexp.MustCompile(`^[A-Z]{3}$`)
