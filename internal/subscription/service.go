// Package subscription keeps the company's registry of software subscriptions
// and the licences handed out from them.
//
// Who does what:
//   - the admin creates, edits, cancels and deletes subscriptions and hands
//     out and revokes licences of any of them to any active user;
//   - a manager sees their department's subscriptions, hands out their
//     licences to the people of their department and revokes them;
//   - everybody sees the subscriptions they hold a licence for. To give a
//     licence back its holder files a release request, unless they manage
//     the subscription themselves (then they just revoke it).
//
// Anything outside the caller's view is a 404, as everywhere else.
//
// Seats: individual is always one seat, group has seats_total seats,
// enterprise is unlimited. seats_used counts active licences and moves only
// with assignments; the database refuses more licences than seats, and an
// edit that would leave seats_total below seats_used. Cancelling or deleting a
// subscription revokes all its licences in the same transaction.
//
// Release requests (release_*.go): one decision, by whoever manages the
// subscription - a manager of its department or the admin, whoever is first.
// A licence revoked another way while its request waits closes the request
// as approved by whoever revoked it, in the same transaction.
package subscription

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"subtrack/internal/apperr"
	"subtrack/internal/domain"
	"subtrack/internal/logger"
	"subtrack/internal/paging"
	"subtrack/internal/storage"
)

var (
	// ErrSubscriptionNotFound is returned when the subscription does not
	// exist, is deleted, or is outside the caller's view. Mapped to 404.
	ErrSubscriptionNotFound = apperr.New(apperr.ErrNotFound, "service: subscription not found")

	// ErrUnknownDepartment is returned when department_id names no
	// department. Mapped to 400.
	ErrUnknownDepartment = apperr.New(apperr.ErrInvalidInput, "service: invalid input: department does not exist")

	// ErrUnknownCurrency is returned when the currency is not in the
	// currencies table. Mapped to 400.
	ErrUnknownCurrency = apperr.New(apperr.ErrInvalidInput, "service: invalid input: unknown currency")

	// ErrSeatsBelowUsed is returned when an edit would leave fewer seats than
	// licences in use. Mapped to 409.
	ErrSeatsBelowUsed = apperr.New(apperr.ErrConflict, "service: seats_total cannot be lower than the number of seats in use; revoke licences first")
)

// repo is the slice of persistence the service needs; *Store satisfies it.
type repo interface {
	GetByID(ctx context.Context, id int64) (domain.Subscription, error)
	GetForUpdate(ctx context.Context, id int64) (domain.Subscription, error)
	List(ctx context.Context, f domain.SubscriptionFilter, sc *domain.SubscriptionScope, limit, offset int) ([]domain.Subscription, int64, error)
	CurrencyExists(ctx context.Context, code string) (bool, error)
	Create(ctx context.Context, s domain.Subscription) (domain.Subscription, error)
	Update(ctx context.Context, s domain.Subscription, actorID int64) (domain.Subscription, error)
	Delete(ctx context.Context, id, actorID int64) error

	GetAssignment(ctx context.Context, id int64) (domain.Assignment, error)
	HasActiveAssignment(ctx context.Context, subID, userID int64) (bool, error)
	ListAssignments(ctx context.Context, f domain.AssignmentFilter, sc *domain.SubscriptionScope, limit, offset int) ([]domain.Assignment, int64, error)
	Assign(ctx context.Context, subID, userID, actorID int64) (domain.Assignment, error)
	Revoke(ctx context.Context, id, actorID int64) (domain.Assignment, error)

	GetRelease(ctx context.Context, id int64) (domain.ReleaseRequest, error)
	LockRelease(ctx context.Context, id int64) (domain.ReleaseRequest, error)
	ListReleases(ctx context.Context, f domain.ReleaseFilter, sc *domain.SubscriptionScope, limit, offset int) ([]domain.ReleaseRequest, int64, error)
	CreateRelease(ctx context.Context, r domain.ReleaseRequest) (domain.ReleaseRequest, error)
	DecideRelease(ctx context.Context, id int64, d domain.ReleaseDecision) (domain.ReleaseRequest, error)
	CancelRelease(ctx context.Context, id, requesterID int64) (domain.ReleaseRequest, error)
	CloseReleases(ctx context.Context, subID, actorID int64) ([]domain.ReleaseRequest, error)
}

// users is what the service needs to vet an assignee; *user.Store satisfies
// it structurally.
type users interface {
	GetByID(ctx context.Context, id int64) (domain.User, error)
}

// departments is what the service needs to know about departments: whether
// one exists. *department.Store satisfies it structurally.
type departments interface {
	GetByID(ctx context.Context, id int64) (domain.Department, error)
}

// transactor runs fn in one transaction; *pg.TxRunner satisfies it.
type transactor interface {
	InTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// auditor records a change in the audit trail; *audit.Store satisfies it.
// It is called inside the change's transaction.
type auditor interface {
	Record(ctx context.Context, e domain.AuditEntry) error
}

// notifier stores in-app notifications; *notification.Store satisfies it.
// It is called inside the change's transaction.
type notifier interface {
	Send(ctx context.Context, n domain.Notification) error
	SendToAdmins(ctx context.Context, n domain.Notification) error
	SendToManagers(ctx context.Context, deptID int64, n domain.Notification) (int64, error)
}

// Service implements the subscriptions, assignments and release requests
// use cases. Every write runs in a transaction together with its audit
// record (and notifications); called inside another domain's transaction
// (an approved request), it joins it.
type Service struct {
	repo   repo
	users  users
	depts  departments
	tx     transactor
	audit  auditor
	notify notifier
	loc    *time.Location
	// now is time.Now; tests replace it.
	now func() time.Time
}

// New wires a Service over its repo, the users and departments readers, the
// transaction runner, the audit trail and the notifications. loc decides
// what "today" is (the company's zone, the scheduler's).
func New(r repo, u users, d departments, tx transactor, a auditor, n notifier, loc *time.Location) *Service {
	return &Service{repo: r, users: u, depts: d, tx: tx, audit: a, notify: n, loc: loc, now: time.Now}
}

// today is the calendar day in the company's zone, as midnight UTC.
func (s *Service) today() time.Time {
	y, m, d := s.now().In(s.loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// scopeFor returns what actor may see: everything for the admin; their
// department's subscriptions plus their own licences for a manager; only
// their own licences for an employee.
func scopeFor(actor domain.User) *domain.SubscriptionScope {
	switch actor.Role {
	case domain.RoleAdmin:
		return nil
	case domain.RoleManager:
		return &domain.SubscriptionScope{DepartmentID: actor.DepartmentID, HolderID: &actor.ID}
	}
	return &domain.SubscriptionScope{HolderID: &actor.ID}
}

// manages reports whether actor may hand out licences of a subscription
// owned by deptID: the admin always, a manager only for their own
// department.
func manages(actor domain.User, deptID *int64) bool {
	switch actor.Role {
	case domain.RoleAdmin:
		return true
	case domain.RoleManager:
		return actor.DepartmentID != nil && deptID != nil && *actor.DepartmentID == *deptID
	}
	return false
}

// visible reports whether actor may see sub: whoever manages it, and whoever
// holds a licence of it.
func (s *Service) visible(ctx context.Context, actor domain.User, sub domain.Subscription) (bool, error) {
	if manages(actor, sub.DepartmentID) {
		return true, nil
	}
	return s.repo.HasActiveAssignment(ctx, sub.ID, actor.ID)
}

// Get returns one subscription as actor is allowed to see it.
//
// Errors:
//   - ErrSubscriptionNotFound when it does not exist, is deleted or is
//     outside actor's view.
//   - any other datastore failure, propagated.
func (s *Service) Get(ctx context.Context, actor domain.User, id int64) (domain.Subscription, error) {
	logger.Op(ctx, "subscription.Service.Get").Debug("start", "id", id)
	sub, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return domain.Subscription{}, apperr.MapStorage(err, ErrSubscriptionNotFound)
	}
	ok, err := s.visible(ctx, actor, sub)
	if err != nil {
		return domain.Subscription{}, err
	}
	if !ok {
		return domain.Subscription{}, ErrSubscriptionNotFound
	}
	return sub, nil
}

// List returns one page of the subscriptions visible to actor that match f.
func (s *Service) List(ctx context.Context, actor domain.User, f domain.SubscriptionFilter, p paging.Params) ([]domain.Subscription, int64, error) {
	log := logger.Op(ctx, "subscription.Service.List")
	p.Normalize()
	f.Query = strings.TrimSpace(f.Query)
	f.Category = strings.TrimSpace(f.Category)

	items, total, err := s.repo.List(ctx, f, scopeFor(actor), p.Limit, p.Offset)
	if err == nil {
		log.Debug("done", "count", len(items), "total", total, "limit", p.Limit, "offset", p.Offset)
	}
	return items, total, err
}

// All returns every subscription visible to actor that matches f, page by
// page, for an export.
func (s *Service) All(ctx context.Context, actor domain.User, f domain.SubscriptionFilter) ([]domain.Subscription, error) {
	var out []domain.Subscription
	for p := (paging.Params{Limit: paging.MaxLimit}); ; p.Offset += p.Limit {
		items, total, err := s.List(ctx, actor, f, p)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
		if len(items) == 0 || int64(len(out)) >= total {
			return out, nil
		}
	}
}

// Create validates sub and inserts it as an active subscription with no
// licences handed out, recording actor as its creator. ID, Status, SeatsUsed,
// CreatedBy and the timestamps of sub are ignored. SeatsTotal is required for
// group, must be absent (or 1) for individual and absent for enterprise.
//
// Errors:
//   - apperr.ErrInvalidInput for a missing or malformed field.
//   - ErrUnknownDepartment, ErrUnknownCurrency as their names say.
//   - any other datastore failure, propagated.
func (s *Service) Create(ctx context.Context, actor domain.User, sub domain.Subscription) (domain.Subscription, error) {
	log := logger.Op(ctx, "subscription.Service.Create")
	log.Debug("start", "name", sub.Name, "type", sub.Type)

	sub.Status = domain.SubActive
	sub.CreatedBy = &actor.ID
	if err := normalize(&sub); err != nil {
		return domain.Subscription{}, err
	}
	if err := s.checkRefs(ctx, sub.Currency, sub.DepartmentID); err != nil {
		return domain.Subscription{}, err
	}
	var out domain.Subscription
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		var err error
		out, err = s.repo.Create(ctx, sub)
		if err != nil {
			return err
		}
		return s.audit.Record(ctx, domain.NewAuditEntry(actor.ID, "subscription.create", "subscription", out.ID, map[string]any{
			"name": out.Name, "type": string(out.Type), "cost": out.Cost, "currency": out.Currency,
			"department_id": out.DepartmentID, "seats_total": out.SeatsTotal,
		}))
	})
	if err != nil {
		return domain.Subscription{}, mapWriteErr(err)
	}
	log.Info("subscription created", "id", out.ID, "type", out.Type, "department_id", logger.Val(out.DepartmentID))
	return out, nil
}

// Update applies a partial patch on behalf of actor (an admin). People may
// set the status to active or cancelled only; expiring_soon and expired are
// the scheduler's. Cancelling revokes every licence in the same transaction
// and closes the release requests that waited as approved.
// Bringing a cancelled or expired subscription back to active moves a past
// next_payment_date forward to today or later (see reopen). Changing the type without
// seats_total resets the seats to the new type's default (1 for individual,
// unlimited for enterprise; group then needs seats_total).
//
// The row is read under lock in the same transaction as the write, since
// every column is written back: a status or payment date the scheduler
// changes meanwhile is not overwritten with a stale one.
//
// Errors:
//   - apperr.ErrInvalidInput for an empty patch or an invalid value.
//   - ErrSubscriptionNotFound, ErrUnknownDepartment, ErrUnknownCurrency.
//   - ErrSeatsBelowUsed when seats_total would fall below seats_used.
//   - any other datastore failure, propagated.
func (s *Service) Update(ctx context.Context, actor domain.User, id int64, p domain.SubscriptionPatch) (domain.Subscription, error) {
	log := logger.Op(ctx, "subscription.Service.Update")
	log.Debug("start", "id", id)

	if !p.HasAny() {
		return domain.Subscription{}, fmt.Errorf("%w: at least one field must be provided", apperr.ErrInvalidInput)
	}
	if p.Status != nil && *p.Status != domain.SubActive && *p.Status != domain.SubCancelled {
		return domain.Subscription{}, fmt.Errorf("%w: status may be set to active or cancelled only", apperr.ErrInvalidInput)
	}

	var cur, out domain.Subscription
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		var err error
		cur, err = s.repo.GetForUpdate(ctx, id)
		if err != nil {
			return err
		}
		sub := apply(cur, p)
		if err := normalize(&sub); err != nil {
			return err
		}
		currency := ""
		if sub.Currency != cur.Currency {
			currency = sub.Currency
		}
		var dept *int64
		if p.DepartmentID.Set {
			dept = sub.DepartmentID
		}
		if err := s.checkRefs(ctx, currency, dept); err != nil {
			return err
		}
		moved := reopen(cur, &sub, p, s.today())

		out, err = s.repo.Update(ctx, sub, actor.ID)
		if err != nil {
			return err
		}
		details := patchDetails(p)
		if moved {
			details["next_payment_date"] = dayOrNil(out.NextPaymentDate)
		}
		if cur.Status != domain.SubCancelled && out.Status == domain.SubCancelled {
			details["licences_revoked"] = cur.SeatsUsed
			closed, err := s.closeReleases(ctx, actor, id)
			if err != nil {
				return err
			}
			if closed > 0 {
				details["release_requests_closed"] = closed
			}
		}
		return s.audit.Record(ctx, domain.NewAuditEntry(actor.ID, "subscription.update", "subscription", id, details))
	})
	if err != nil {
		return domain.Subscription{}, mapWriteErr(err)
	}
	if cur.Status != domain.SubCancelled && out.Status == domain.SubCancelled {
		log.Info("subscription cancelled, licences revoked", "id", id, "revoked", cur.SeatsUsed)
	}
	log.Info("subscription updated", "id", id)
	return out, nil
}

// Delete soft-deletes a subscription and revokes its licences on behalf of
// actor (an admin); release requests that waited are closed as approved.
//
// Errors:
//   - ErrSubscriptionNotFound when no live subscription has that id.
//   - any other datastore failure, propagated.
func (s *Service) Delete(ctx context.Context, actor domain.User, id int64) error {
	log := logger.Op(ctx, "subscription.Service.Delete")
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		// Read it first: the record names what was deleted.
		cur, err := s.repo.GetByID(ctx, id)
		if err != nil {
			return err
		}
		if err := s.repo.Delete(ctx, id, actor.ID); err != nil {
			return err
		}
		details := map[string]any{"name": cur.Name, "licences_revoked": cur.SeatsUsed}
		closed, err := s.closeReleases(ctx, actor, id)
		if err != nil {
			return err
		}
		if closed > 0 {
			details["release_requests_closed"] = closed
		}
		return s.audit.Record(ctx, domain.NewAuditEntry(actor.ID, "subscription.delete", "subscription", id, details))
	})
	if err != nil {
		return apperr.MapStorage(err, ErrSubscriptionNotFound)
	}
	log.Info("subscription deleted", "id", id)
	return nil
}

// apply returns cur with the fields of p written over it.
func apply(cur domain.Subscription, p domain.SubscriptionPatch) domain.Subscription {
	s := cur
	if p.Name != nil {
		s.Name = *p.Name
	}
	if p.Vendor != nil {
		s.Vendor = *p.Vendor
	}
	if p.Category != nil {
		s.Category = *p.Category
	}
	if p.Type != nil && *p.Type != cur.Type {
		s.Type = *p.Type
		// The old seat count belonged to the old type; normalize sets the
		// new type's default unless the patch gives one.
		s.SeatsTotal = nil
	}
	if p.SeatsTotal != nil {
		s.SeatsTotal = p.SeatsTotal
	}
	if p.Cost != nil {
		s.Cost = *p.Cost
	}
	if p.Currency != nil {
		s.Currency = *p.Currency
	}
	if p.BillingPeriod != nil {
		s.BillingPeriod = *p.BillingPeriod
	}
	if p.StartDate != nil {
		s.StartDate = *p.StartDate
	}
	if p.EndDate.Set {
		s.EndDate = p.EndDate.Value
	}
	if p.NextPaymentDate.Set {
		s.NextPaymentDate = p.NextPaymentDate.Value
	}
	if p.Status != nil {
		s.Status = *p.Status
	}
	if p.DepartmentID.Set {
		s.DepartmentID = p.DepartmentID.Value
	}
	return s
}

// reopen moves next_payment_date of a subscription brought back to active
// from cancelled or expired past the payments it missed meanwhile, so the
// scheduler does not create a pending payment for each of them: it steps by
// the billing period to the first date not before today, and clears it when
// that is past the end date. A date given in the same patch is kept as it
// is. It reports whether it changed the date.
func reopen(cur domain.Subscription, sub *domain.Subscription, p domain.SubscriptionPatch, today time.Time) bool {
	back := (cur.Status == domain.SubCancelled || cur.Status == domain.SubExpired) && sub.Status == domain.SubActive
	if !back || p.NextPaymentDate.Set || sub.NextPaymentDate == nil || !sub.NextPaymentDate.Before(today) {
		return false
	}
	next := *sub.NextPaymentDate
	for next.Before(today) {
		next = sub.BillingPeriod.Next(next)
	}
	if sub.EndDate != nil && next.After(*sub.EndDate) {
		sub.NextPaymentDate = nil
	} else {
		sub.NextPaymentDate = &next
	}
	return true
}

// MaxSeats is the most seats a group subscription may have: seats_total is
// an INT column, and a larger number would fail in the database as a 500.
const MaxSeats = math.MaxInt32

// costRe is a non-negative amount that fits NUMERIC(14,2).
var costRe = regexp.MustCompile(`^[0-9]{1,12}(\.[0-9]{1,2})?$`)

// currencyRe is an ISO 4217 code, after upper-casing.
var currencyRe = regexp.MustCompile(`^[A-Z]{3}$`)

// normalize trims and validates a whole subscription and fills in the seats
// its type implies.
func normalize(s *domain.Subscription) error {
	s.Name = strings.TrimSpace(s.Name)
	s.Vendor = strings.TrimSpace(s.Vendor)
	s.Category = strings.TrimSpace(s.Category)
	s.Cost = strings.TrimSpace(s.Cost)
	s.Currency = strings.ToUpper(strings.TrimSpace(s.Currency))

	var missing []string
	if s.Name == "" {
		missing = append(missing, "name")
	}
	if s.Type == "" {
		missing = append(missing, "type")
	}
	if s.Cost == "" {
		missing = append(missing, "cost")
	}
	if s.Currency == "" {
		missing = append(missing, "currency")
	}
	if s.BillingPeriod == "" {
		missing = append(missing, "billing_period")
	}
	if s.StartDate.IsZero() {
		missing = append(missing, "start_date")
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: missing required fields: %s", apperr.ErrInvalidInput, strings.Join(missing, ", "))
	}

	if !s.Type.Valid() {
		return fmt.Errorf("%w: type must be one of individual, group, enterprise", apperr.ErrInvalidInput)
	}
	if !s.BillingPeriod.Valid() {
		return fmt.Errorf("%w: billing_period must be one of month, quarter, year", apperr.ErrInvalidInput)
	}
	if !s.Status.Valid() {
		return fmt.Errorf("%w: unknown status %q", apperr.ErrInvalidInput, s.Status)
	}
	if !costRe.MatchString(s.Cost) {
		return fmt.Errorf("%w: cost must be a non-negative amount with at most 2 decimals, like 1500.00", apperr.ErrInvalidInput)
	}
	if !currencyRe.MatchString(s.Currency) {
		return fmt.Errorf("%w: currency must be a three-letter code, like RUB", apperr.ErrInvalidInput)
	}
	if s.EndDate != nil && s.EndDate.Before(s.StartDate) {
		return fmt.Errorf("%w: end_date must not be before start_date", apperr.ErrInvalidInput)
	}

	switch s.Type {
	case domain.SubIndividual:
		if s.SeatsTotal != nil && *s.SeatsTotal != 1 {
			return fmt.Errorf("%w: an individual subscription has exactly 1 seat; use type group for more", apperr.ErrInvalidInput)
		}
		one := 1
		s.SeatsTotal = &one
	case domain.SubGroup:
		if s.SeatsTotal == nil {
			return fmt.Errorf("%w: seats_total is required for a group subscription", apperr.ErrInvalidInput)
		}
		if *s.SeatsTotal < 1 || *s.SeatsTotal > MaxSeats {
			return fmt.Errorf("%w: seats_total must be between 1 and %d", apperr.ErrInvalidInput, MaxSeats)
		}
	case domain.SubEnterprise:
		if s.SeatsTotal != nil {
			return fmt.Errorf("%w: an enterprise subscription has no seat limit; omit seats_total", apperr.ErrInvalidInput)
		}
	}
	return nil
}

// checkRefs returns ErrUnknownCurrency / ErrUnknownDepartment when currency
// (if not empty) or dept (if set) names nothing. Checking up front, rather
// than relying on the foreign keys, keeps "no such subscription" and "no such
// department" apart.
func (s *Service) checkRefs(ctx context.Context, currency string, dept *int64) error {
	if currency != "" {
		ok, err := s.repo.CurrencyExists(ctx, currency)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("%w %s", ErrUnknownCurrency, currency)
		}
	}
	if dept != nil {
		_, err := s.depts.GetByID(ctx, *dept)
		if errors.Is(err, storage.ErrNotFound) {
			return ErrUnknownDepartment
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// mapWriteErr translates the storage errors of Create/Update.
func mapWriteErr(err error) error {
	var ce *storage.CheckError
	if errors.As(err, &ce) && ce.Constraint == "subscriptions_seats_check" {
		return ErrSeatsBelowUsed
	}
	// The department and the currency were checked up front, but may be
	// deleted meanwhile; that is not "no such subscription".
	var fk *storage.ForeignKeyError
	if errors.As(err, &fk) {
		switch fk.Constraint {
		case "subscriptions_department_id_fkey":
			return ErrUnknownDepartment
		case "subscriptions_currency_fkey":
			return ErrUnknownCurrency
		}
	}
	return apperr.MapStorage(err, ErrSubscriptionNotFound)
}

// patchDetails lists the fields an update sets, with their new values, for
// the audit trail.
func patchDetails(p domain.SubscriptionPatch) map[string]any {
	d := map[string]any{}
	if p.Name != nil {
		d["name"] = *p.Name
	}
	if p.Vendor != nil {
		d["vendor"] = *p.Vendor
	}
	if p.Category != nil {
		d["category"] = *p.Category
	}
	if p.Type != nil {
		d["type"] = string(*p.Type)
	}
	if p.Cost != nil {
		d["cost"] = *p.Cost
	}
	if p.Currency != nil {
		d["currency"] = *p.Currency
	}
	if p.BillingPeriod != nil {
		d["billing_period"] = string(*p.BillingPeriod)
	}
	if p.StartDate != nil {
		d["start_date"] = dayOrNil(p.StartDate)
	}
	if p.EndDate.Set {
		d["end_date"] = dayOrNil(p.EndDate.Value)
	}
	if p.NextPaymentDate.Set {
		d["next_payment_date"] = dayOrNil(p.NextPaymentDate.Value)
	}
	if p.Status != nil {
		d["status"] = string(*p.Status)
	}
	if p.DepartmentID.Set {
		d["department_id"] = p.DepartmentID.Value
	}
	if p.SeatsTotal != nil {
		d["seats_total"] = *p.SeatsTotal
	}
	return d
}

// dayOrNil formats a date for the audit trail, nil staying nil.
func dayOrNil(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.Format(time.DateOnly)
}
