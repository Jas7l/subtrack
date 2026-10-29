package subscription

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"subtrack/internal/apperr"
	"subtrack/internal/auth"
	"subtrack/internal/domain"
	"subtrack/internal/paging"
	"subtrack/internal/storage"
)

func TestCreateValidation(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	valid := func() domain.Subscription {
		return domain.Subscription{
			Name: " Jira ", Vendor: "Atlassian", Type: domain.SubGroup, Cost: "1500.50", Currency: "rub",
			BillingPeriod: domain.BillingMonth, StartDate: day("2026-01-01"), SeatsTotal: ptr(10),
			DepartmentID: ptr(int64(1)),
		}
	}

	s, err := f.svc.Create(ctx, f.admin, valid())
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "Jira" || s.Currency != "RUB" || s.Status != domain.SubActive || s.CreatedBy == nil || *s.CreatedBy != f.admin.ID {
		t.Errorf("created %+v", s)
	}

	tests := []struct {
		name   string
		mutate func(*domain.Subscription)
		want   error
	}{
		{"no name", func(s *domain.Subscription) { s.Name = " " }, apperr.ErrInvalidInput},
		{"no start date", func(s *domain.Subscription) { s.StartDate = day("0001-01-01") }, apperr.ErrInvalidInput},
		{"bad type", func(s *domain.Subscription) { s.Type = "team" }, apperr.ErrInvalidInput},
		{"bad period", func(s *domain.Subscription) { s.BillingPeriod = "week" }, apperr.ErrInvalidInput},
		{"negative cost", func(s *domain.Subscription) { s.Cost = "-1" }, apperr.ErrInvalidInput},
		{"three decimals", func(s *domain.Subscription) { s.Cost = "1.005" }, apperr.ErrInvalidInput},
		{"cost too big", func(s *domain.Subscription) { s.Cost = "1234567890123" }, apperr.ErrInvalidInput},
		{"bad currency", func(s *domain.Subscription) { s.Currency = "RU" }, apperr.ErrInvalidInput},
		{"unknown currency", func(s *domain.Subscription) { s.Currency = "EUR" }, ErrUnknownCurrency},
		{"unknown department", func(s *domain.Subscription) { s.DepartmentID = ptr(int64(9)) }, ErrUnknownDepartment},
		{"ends before start", func(s *domain.Subscription) { s.EndDate = ptr(day("2025-12-31")) }, apperr.ErrInvalidInput},
		{"group without seats", func(s *domain.Subscription) { s.SeatsTotal = nil }, apperr.ErrInvalidInput},
		{"group with 0 seats", func(s *domain.Subscription) { s.SeatsTotal = ptr(0) }, apperr.ErrInvalidInput},
		{"seats past INT", func(s *domain.Subscription) { s.SeatsTotal = ptr(MaxSeats + 1) }, apperr.ErrInvalidInput},
		{"individual with 3 seats", func(s *domain.Subscription) { s.Type = domain.SubIndividual; s.SeatsTotal = ptr(3) }, apperr.ErrInvalidInput},
		{"enterprise with seats", func(s *domain.Subscription) { s.Type = domain.SubEnterprise }, apperr.ErrInvalidInput},
	}
	for _, tt := range tests {
		in := valid()
		tt.mutate(&in)
		if _, err := f.svc.Create(ctx, f.admin, in); !errors.Is(err, tt.want) {
			t.Errorf("%s: err = %v, want %v", tt.name, err, tt.want)
		}
	}

	in := valid()
	in.Type, in.SeatsTotal = domain.SubIndividual, nil
	if s, err := f.svc.Create(ctx, f.admin, in); err != nil || s.SeatsTotal == nil || *s.SeatsTotal != 1 {
		t.Errorf("individual gets 1 seat: %+v, %v", s.SeatsTotal, err)
	}
	in.Type = domain.SubEnterprise
	if s, err := f.svc.Create(ctx, f.admin, in); err != nil || s.SeatsTotal != nil {
		t.Errorf("enterprise is unlimited: %+v, %v", s.SeatsTotal, err)
	}
}

func TestVisibility(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	dev := f.create(t, domain.Subscription{Name: "Jira", Type: domain.SubGroup, SeatsTotal: ptr(5), DepartmentID: ptr(int64(1))})
	design := f.create(t, domain.Subscription{Name: "Figma", Type: domain.SubGroup, SeatsTotal: ptr(5), DepartmentID: ptr(int64(2))})
	company := f.create(t, domain.Subscription{Name: "Slack", Type: domain.SubEnterprise})
	if _, err := f.svc.Assign(ctx, f.admin, company.ID, f.emp.ID); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name  string
		actor domain.User
		sub   domain.Subscription
		ok    bool
	}{
		{"admin sees any", f.admin, design, true},
		{"manager sees own department", f.mgr, dev, true},
		{"manager does not see other department", f.mgr, design, false},
		{"manager does not see company-wide they do not hold", f.mgr, company, false},
		{"employee sees what they hold", f.emp, company, true},
		{"employee does not see own department's", f.emp, dev, false},
	}
	for _, tt := range tests {
		_, err := f.svc.Get(ctx, tt.actor, tt.sub.ID)
		if tt.ok && err != nil {
			t.Errorf("%s: err = %v", tt.name, err)
		}
		if !tt.ok && !errors.Is(err, ErrSubscriptionNotFound) {
			t.Errorf("%s: err = %v, want ErrSubscriptionNotFound", tt.name, err)
		}
	}

	count := func(actor domain.User, flt domain.SubscriptionFilter) int64 {
		_, total, err := f.svc.List(ctx, actor, flt, paging.Params{})
		if err != nil {
			t.Fatal(err)
		}
		return total
	}
	if n := count(f.admin, domain.SubscriptionFilter{}); n != 3 {
		t.Errorf("admin: %d, want 3", n)
	}
	if n := count(f.mgr, domain.SubscriptionFilter{}); n != 1 {
		t.Errorf("manager: %d, want 1", n)
	}
	if n := count(f.emp, domain.SubscriptionFilter{}); n != 1 {
		t.Errorf("employee: %d, want 1", n)
	}
	if n := count(f.homeless, domain.SubscriptionFilter{}); n != 0 {
		t.Errorf("nobody's licences: %d, want 0", n)
	}
	if n := count(f.admin, domain.SubscriptionFilter{Type: ptr(domain.SubEnterprise)}); n != 1 {
		t.Errorf("type filter: %d, want 1", n)
	}
}

func TestAssignRules(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	dev := f.create(t, domain.Subscription{Name: "Jira", Type: domain.SubGroup, SeatsTotal: ptr(2), DepartmentID: ptr(int64(1))})
	design := f.create(t, domain.Subscription{Name: "Figma", Type: domain.SubGroup, SeatsTotal: ptr(5), DepartmentID: ptr(int64(2))})
	company := f.create(t, domain.Subscription{Name: "Slack", Type: domain.SubEnterprise})
	closed := f.create(t, domain.Subscription{Name: "Old", Type: domain.SubGroup, SeatsTotal: ptr(5), DepartmentID: ptr(int64(1))})
	if _, err := f.svc.Update(ctx, f.admin, closed.ID, domain.SubscriptionPatch{Status: ptr(domain.SubCancelled)}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Assign(ctx, f.admin, company.ID, f.mgr.ID); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		actor     domain.User
		sub, user int64
		want      error
	}{
		{"manager: own department", f.mgr, dev.ID, f.emp.ID, nil},
		{"manager: again", f.mgr, dev.ID, f.emp.ID, ErrAlreadyAssigned},
		{"manager: person of another department", f.mgr, dev.ID, f.out.ID, ErrInvalidAssignee},
		{"manager: other department's subscription", f.mgr, design.ID, f.emp.ID, ErrSubscriptionNotFound},
		{"manager: company-wide they hold", f.mgr, company.ID, f.emp.ID, auth.ErrForbidden},
		{"admin: across departments", f.admin, design.ID, f.emp.ID, nil},
		{"admin: blocked user", f.admin, dev.ID, f.blocked.ID, ErrInvalidAssignee},
		{"admin: missing user", f.admin, dev.ID, 99, ErrInvalidAssignee},
		{"admin: cancelled subscription", f.admin, closed.ID, f.emp.ID, ErrSubscriptionClosed},
		{"admin: missing subscription", f.admin, 99, f.emp.ID, ErrSubscriptionNotFound},
		{"admin: last seat", f.admin, dev.ID, f.emp2.ID, nil},
		{"admin: no seats left", f.admin, dev.ID, f.mgr.ID, ErrNoFreeSeats},
		{"employee", f.emp, dev.ID, f.emp2.ID, auth.ErrForbidden},
	}
	for _, tt := range tests {
		_, err := f.svc.Assign(ctx, tt.actor, tt.sub, tt.user)
		if !errors.Is(err, tt.want) {
			t.Errorf("%s: err = %v, want %v", tt.name, err, tt.want)
		}
	}
	if s, _ := f.repo.GetByID(ctx, dev.ID); s.SeatsUsed != 2 {
		t.Errorf("seats_used = %d, want 2", s.SeatsUsed)
	}
}

func TestRevoke(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	dev := f.create(t, domain.Subscription{Name: "Jira", Type: domain.SubGroup, SeatsTotal: ptr(5), DepartmentID: ptr(int64(1))})
	a1, _ := f.svc.Assign(ctx, f.admin, dev.ID, f.emp.ID)
	a2, _ := f.svc.Assign(ctx, f.admin, dev.ID, f.emp2.ID)
	a3, _ := f.svc.Assign(ctx, f.admin, dev.ID, f.out.ID)

	tests := []struct {
		name  string
		actor domain.User
		id    int64
		want  error
	}{
		{"colleague cannot", f.emp2, a1.ID, ErrAssignmentNotFound},
		{"other department's manager cannot", f.mgrDesign, a1.ID, ErrAssignmentNotFound},
		{"holder gives it back", f.emp, a1.ID, nil},
		{"twice", f.admin, a1.ID, ErrAlreadyRevoked},
		{"manager of the subscription's department", f.mgr, a2.ID, nil},
		{"admin", f.admin, a3.ID, nil},
		{"missing", f.admin, 99, ErrAssignmentNotFound},
	}
	for _, tt := range tests {
		_, err := f.svc.Revoke(ctx, tt.actor, tt.id)
		if !errors.Is(err, tt.want) {
			t.Errorf("%s: err = %v, want %v", tt.name, err, tt.want)
		}
	}
	if s, _ := f.repo.GetByID(ctx, dev.ID); s.SeatsUsed != 0 {
		t.Errorf("seats_used = %d, want 0", s.SeatsUsed)
	}
	// After giving it back the same person can get it again.
	if _, err := f.svc.Assign(ctx, f.mgr, dev.ID, f.emp.ID); err != nil {
		t.Errorf("assign again: %v", err)
	}
}

func TestUpdate(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	s := f.create(t, domain.Subscription{Name: "Jira", Type: domain.SubGroup, SeatsTotal: ptr(5), DepartmentID: ptr(int64(1)),
		EndDate: ptr(day("2026-12-31"))})
	for _, u := range []domain.User{f.emp, f.emp2, f.mgr} {
		if _, err := f.svc.Assign(ctx, f.admin, s.ID, u.ID); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name  string
		patch domain.SubscriptionPatch
		want  error
	}{
		{"empty", domain.SubscriptionPatch{}, apperr.ErrInvalidInput},
		{"scheduler's status", domain.SubscriptionPatch{Status: ptr(domain.SubExpired)}, apperr.ErrInvalidInput},
		{"seats below used", domain.SubscriptionPatch{SeatsTotal: ptr(2)}, ErrSeatsBelowUsed},
		{"to individual with 3 in use", domain.SubscriptionPatch{Type: ptr(domain.SubIndividual)}, ErrSeatsBelowUsed},
		{"to group from group keeps seats", domain.SubscriptionPatch{Type: ptr(domain.SubGroup)}, nil},
		{"start after end", domain.SubscriptionPatch{StartDate: ptr(day("2027-01-01"))}, apperr.ErrInvalidInput},
		{"unknown currency", domain.SubscriptionPatch{Currency: ptr("EUR")}, ErrUnknownCurrency},
		{"unknown department", domain.SubscriptionPatch{DepartmentID: domain.IDPatch{Set: true, Value: ptr(int64(9))}}, ErrUnknownDepartment},
		{"seats down to used", domain.SubscriptionPatch{SeatsTotal: ptr(3)}, nil},
		{"clear end date", domain.SubscriptionPatch{EndDate: domain.DatePatch{Set: true}}, nil},
	}
	for _, tt := range tests {
		if _, err := f.svc.Update(ctx, f.admin, s.ID, tt.patch); !errors.Is(err, tt.want) {
			t.Errorf("%s: err = %v, want %v", tt.name, err, tt.want)
		}
	}

	got, _ := f.repo.GetByID(ctx, s.ID)
	if got.EndDate != nil || *got.SeatsTotal != 3 {
		t.Errorf("after updates: end %v, seats %d", got.EndDate, *got.SeatsTotal)
	}

	// Switching to enterprise drops the limit; back to group needs seats.
	if u, err := f.svc.Update(ctx, f.admin, s.ID, domain.SubscriptionPatch{Type: ptr(domain.SubEnterprise)}); err != nil || u.SeatsTotal != nil {
		t.Errorf("to enterprise: %+v, %v", u.SeatsTotal, err)
	}
	if _, err := f.svc.Update(ctx, f.admin, s.ID, domain.SubscriptionPatch{Type: ptr(domain.SubGroup)}); !errors.Is(err, apperr.ErrInvalidInput) {
		t.Errorf("to group without seats: err = %v", err)
	}

	// Cancelling revokes everything.
	u, err := f.svc.Update(ctx, f.admin, s.ID, domain.SubscriptionPatch{Status: ptr(domain.SubCancelled)})
	if err != nil || u.SeatsUsed != 0 {
		t.Fatalf("cancel: seats_used %d, %v", u.SeatsUsed, err)
	}
	active := domain.AssignmentActive
	if _, n, _ := f.svc.ListAssignments(ctx, f.admin, domain.AssignmentFilter{Status: &active}, paging.Params{}); n != 0 {
		t.Errorf("active licences after cancel: %d", n)
	}
}

func TestReopenMovesPaymentDate(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	// 22:00 UTC on the 27th is already the 28th in Moscow.
	f.svc.loc = time.FixedZone("MSK", 3*60*60)
	f.svc.now = func() time.Time { return time.Date(2026, 10, 27, 22, 0, 0, 0, time.UTC) }
	active, cancelled := ptr(domain.SubActive), ptr(domain.SubCancelled)
	date := func(t *time.Time) string {
		if t == nil {
			return "null"
		}
		return t.Format(time.DateOnly)
	}
	// newSub creates a subscription and puts it into status, as people or
	// the scheduler would have.
	newSub := func(period domain.BillingPeriod, next, end string, status domain.SubscriptionStatus) domain.Subscription {
		s := domain.Subscription{Name: "Jira", Type: domain.SubEnterprise, BillingPeriod: period, NextPaymentDate: ptr(day(next))}
		if end != "" {
			s.EndDate = ptr(day(end))
		}
		s = f.create(t, s)
		s.Status = status
		f.repo.subs[s.ID] = s
		return s
	}

	tests := []struct {
		name   string
		sub    domain.Subscription
		patch  domain.SubscriptionPatch
		want   string
		record bool // whether the audit record names the new date
	}{
		{"cancelled, monthly", newSub(domain.BillingMonth, "2026-03-15", "", domain.SubCancelled),
			domain.SubscriptionPatch{Status: active}, "2026-11-15", true},
		{"lands on today in the company's zone", newSub(domain.BillingMonth, "2026-09-27", "", domain.SubCancelled),
			domain.SubscriptionPatch{Status: active}, "2026-11-27", true},
		{"expired, renewed for a year", newSub(domain.BillingQuarter, "2026-04-10", "2026-06-30", domain.SubExpired),
			domain.SubscriptionPatch{Status: active, EndDate: domain.DatePatch{Set: true, Value: ptr(day("2027-06-30"))}}, "2027-01-10", true},
		{"next date past the new end", newSub(domain.BillingQuarter, "2026-04-10", "2026-06-30", domain.SubExpired),
			domain.SubscriptionPatch{Status: active, EndDate: domain.DatePatch{Set: true, Value: ptr(day("2026-12-31"))}}, "null", true},
		{"cancelled over a year ago, monthly on the 31st", newSub(domain.BillingMonth, "2025-10-31", "", domain.SubCancelled),
			domain.SubscriptionPatch{Status: active}, "2026-10-28", true},
		{"quarterly from 31.12 of last year", newSub(domain.BillingQuarter, "2025-12-31", "", domain.SubCancelled),
			domain.SubscriptionPatch{Status: active}, "2026-12-30", true},
		{"yearly from 29.02", newSub(domain.BillingYear, "2024-02-29", "", domain.SubCancelled),
			domain.SubscriptionPatch{Status: active}, "2027-02-28", true},
		{"date given in the patch", newSub(domain.BillingMonth, "2026-03-15", "", domain.SubCancelled),
			domain.SubscriptionPatch{Status: active, NextPaymentDate: domain.DatePatch{Set: true, Value: ptr(day("2026-05-01"))}}, "2026-05-01", true},
		{"future date", newSub(domain.BillingMonth, "2026-11-03", "", domain.SubCancelled),
			domain.SubscriptionPatch{Status: active}, "2026-11-03", false},
		{"not a reopen", newSub(domain.BillingMonth, "2026-03-15", "", domain.SubActive),
			domain.SubscriptionPatch{Name: ptr("Jira Cloud")}, "2026-03-15", false},
		{"cancelling", newSub(domain.BillingMonth, "2026-03-15", "", domain.SubActive),
			domain.SubscriptionPatch{Status: cancelled}, "2026-03-15", false},
	}
	for _, tt := range tests {
		got, err := f.svc.Update(ctx, f.admin, tt.sub.ID, tt.patch)
		if err != nil {
			t.Errorf("%s: %v", tt.name, err)
			continue
		}
		if date(got.NextPaymentDate) != tt.want {
			t.Errorf("%s: next_payment_date %s, want %s", tt.name, date(got.NextPaymentDate), tt.want)
		}
		details := f.audit.Entries[len(f.audit.Entries)-1].Details
		v, ok := details["next_payment_date"]
		if ok != tt.record || (ok && v != nil && v != tt.want) || (ok && v == nil && tt.want != "null") {
			t.Errorf("%s: audit next_payment_date = %v (set %v)", tt.name, v, ok)
		}
	}
}

func TestDelete(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	s := f.create(t, domain.Subscription{Name: "Jira", Type: domain.SubGroup, SeatsTotal: ptr(5)})
	a, _ := f.svc.Assign(ctx, f.admin, s.ID, f.emp.ID)

	if err := f.svc.Delete(ctx, f.admin, s.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Delete(ctx, f.admin, s.ID); !errors.Is(err, ErrSubscriptionNotFound) {
		t.Errorf("twice: err = %v", err)
	}
	if _, err := f.svc.Get(ctx, f.admin, s.ID); !errors.Is(err, ErrSubscriptionNotFound) {
		t.Errorf("get deleted: err = %v", err)
	}
	// The licence stays as revoked history, still visible to its holder.
	got, err := f.svc.GetAssignment(ctx, f.emp, a.ID)
	if err != nil || got.Status != domain.AssignmentRevoked {
		t.Errorf("licence after delete: %+v, %v", got, err)
	}
}

func TestListAssignmentsScope(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	dev := f.create(t, domain.Subscription{Name: "Jira", Type: domain.SubGroup, SeatsTotal: ptr(5), DepartmentID: ptr(int64(1))})
	company := f.create(t, domain.Subscription{Name: "Slack", Type: domain.SubEnterprise})
	for _, x := range []struct{ sub, user int64 }{{dev.ID, f.emp.ID}, {dev.ID, f.out.ID}, {company.ID, f.emp.ID}, {company.ID, f.mgr.ID}, {company.ID, f.out.ID}} {
		if _, err := f.svc.Assign(ctx, f.admin, x.sub, x.user); err != nil {
			t.Fatal(err)
		}
	}
	count := func(actor domain.User) int64 {
		_, n, err := f.svc.ListAssignments(ctx, actor, domain.AssignmentFilter{}, paging.Params{})
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count(f.admin); n != 5 {
		t.Errorf("admin: %d, want 5", n)
	}
	// dev's two licences plus the manager's own Slack.
	if n := count(f.mgr); n != 3 {
		t.Errorf("manager: %d, want 3", n)
	}
	if n := count(f.emp); n != 2 {
		t.Errorf("employee: %d, want 2", n)
	}
}

func TestAuditTrail(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	s := f.create(t, domain.Subscription{Name: "Jira", Type: domain.SubGroup, SeatsTotal: ptr(5), DepartmentID: ptr(int64(1))})
	a, _ := f.svc.Assign(ctx, f.mgr, s.ID, f.emp.ID)
	_, _ = f.svc.Assign(ctx, f.mgr, s.ID, f.emp2.ID)
	_, _ = f.svc.Revoke(ctx, f.emp, a.ID)
	_, _ = f.svc.Update(ctx, f.admin, s.ID, domain.SubscriptionPatch{Status: ptr(domain.SubCancelled), EndDate: domain.DatePatch{Set: true}})
	_ = f.svc.Delete(ctx, f.admin, s.ID)
	// A refused change leaves no record.
	_, _ = f.svc.Assign(ctx, f.mgr, s.ID, f.emp.ID)

	want := []string{"subscription.create", "assignment.create", "assignment.create", "assignment.revoke",
		"subscription.update", "subscription.delete"}
	got := f.audit.Actions()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("actions:\n got %v\nwant %v", got, want)
	}
	revoke := f.audit.Entries[3]
	if *revoke.ActorID != f.emp.ID || *revoke.EntityID != a.ID || revoke.Details["user_id"] != f.emp.ID {
		t.Errorf("revoke entry: %+v", revoke)
	}
	cancel := f.audit.Entries[4].Details
	if cancel["status"] != "cancelled" || cancel["end_date"] != nil || cancel["licences_revoked"] != 1 {
		t.Errorf("cancel details: %+v", cancel)
	}
	if del := f.audit.Entries[5].Details; del["name"] != "Jira" {
		t.Errorf("delete details: %+v", del)
	}
}

// A department or currency deleted between the check and the write is
// reported as such, not as a missing subscription.
func TestMapWriteErrForeignKeys(t *testing.T) {
	for _, tt := range []struct {
		constraint string
		want       error
	}{
		{"subscriptions_department_id_fkey", ErrUnknownDepartment},
		{"subscriptions_currency_fkey", ErrUnknownCurrency},
		{"subscriptions_created_by_fkey", ErrSubscriptionNotFound},
	} {
		err := fmt.Errorf("postgres: create subscription: %w", &storage.ForeignKeyError{Constraint: tt.constraint})
		if got := mapWriteErr(err); !errors.Is(got, tt.want) {
			t.Errorf("%s: got %v, want %v", tt.constraint, got, tt.want)
		}
	}
}
