package subscription

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"

	"subtrack/internal/domain"
	"subtrack/internal/paging"
)

// releaseFixture adds to fixture: a dev subscription (department 1), a
// design one (department 2) and a company-wide one, each with seats to spare.
type releaseFixture struct {
	fixture
	dev, design, common domain.Subscription
}

func newReleaseFixture(t *testing.T) releaseFixture {
	t.Helper()
	f := releaseFixture{fixture: newFixture(t)}
	f.dev = f.create(t, domain.Subscription{Name: "Jira", Type: domain.SubGroup, SeatsTotal: ptr(10), DepartmentID: ptr(int64(1))})
	f.design = f.create(t, domain.Subscription{Name: "Figma", Type: domain.SubGroup, SeatsTotal: ptr(10), DepartmentID: ptr(int64(2))})
	f.common = f.create(t, domain.Subscription{Name: "Slack", Type: domain.SubEnterprise})
	return f
}

// assign hands out a licence as the admin or fails the test.
func (f releaseFixture) assign(t *testing.T, sub domain.Subscription, u domain.User) domain.Assignment {
	t.Helper()
	a, err := f.svc.Assign(context.Background(), f.admin, sub.ID, u.ID)
	if err != nil {
		t.Fatalf("assign %s to %d: %v", sub.Name, u.ID, err)
	}
	return a
}

// file files a release request as the holder or fails the test.
func (f releaseFixture) file(t *testing.T, as domain.User, a domain.Assignment) domain.ReleaseRequest {
	t.Helper()
	r, err := f.svc.FileRelease(context.Background(), as, a.ID, "")
	if err != nil {
		t.Fatalf("file for licence %d: %v", a.ID, err)
	}
	return r
}

func TestFileRelease(t *testing.T) {
	f := newReleaseFixture(t)
	ctx := context.Background()
	mine := f.assign(t, f.dev, f.emp)
	crossed := f.assign(t, f.dev, f.out) // a design employee with a dev licence
	common := f.assign(t, f.common, f.homeless)
	own := f.assign(t, f.dev, f.mgr)
	foreign := f.assign(t, f.design, f.mgr) // the dev manager with a design licence
	gone := f.assign(t, f.dev, f.emp2)
	if _, err := f.svc.Revoke(ctx, f.admin, gone.ID); err != nil {
		t.Fatal(err)
	}
	f.notify.take()

	r, err := f.svc.FileRelease(ctx, f.emp, mine.ID, "  Перешёл в QA  ")
	if err != nil {
		t.Fatalf("file: %v", err)
	}
	if r.Status != domain.ReleasePending || r.Reason != "Перешёл в QA" || r.RequesterID != f.emp.ID ||
		r.SubscriptionID != f.dev.ID || r.DepartmentID == nil || *r.DepartmentID != 1 {
		t.Errorf("filed: %+v", r)
	}
	// The dev department's manager decides; the admins are not bothered.
	if got := f.notify.take(); !slices.Equal(got, []string{"release_created→2"}) {
		t.Errorf("notified %v", got)
	}
	if last := f.audit.Last(); last.Action != "release_request.create" || *last.EntityID != r.ID {
		t.Errorf("audit: %+v", last)
	}
	if a, _ := f.svc.GetAssignment(ctx, f.emp, mine.ID); a.ReleaseRequestID == nil || *a.ReleaseRequestID != r.ID {
		t.Errorf("licence does not point at the request: %+v", a)
	}

	tests := []struct {
		name  string
		actor domain.User
		id    int64
		want  error
	}{
		{"twice", f.emp, mine.ID, ErrReleasePending},
		{"the manager for an employee", f.mgr, mine.ID, ErrNotHolder},
		{"a colleague", f.emp2, mine.ID, ErrAssignmentNotFound},
		{"revoked", f.emp2, gone.ID, ErrAlreadyRevoked},
		{"a manager for their department's own", f.mgr, own.ID, ErrRevokeYourself},
		{"missing", f.emp, 99, ErrAssignmentNotFound},
	}
	for _, tt := range tests {
		if _, err := f.svc.FileRelease(ctx, tt.actor, tt.id, ""); !errors.Is(err, tt.want) {
			t.Errorf("%s: err = %v, want %v", tt.name, err, tt.want)
		}
	}

	// The subscription's department decides, not the holder's.
	f.file(t, f.out, crossed)
	if got := f.notify.take(); !slices.Equal(got, []string{"release_created→2"}) {
		t.Errorf("crossed: notified %v, want the dev manager", got)
	}
	// A manager's licence of another department's subscription goes there.
	f.file(t, f.mgr, foreign)
	if got := f.notify.take(); !slices.Equal(got, []string{"release_created→7"}) {
		t.Errorf("foreign: notified %v, want the design manager", got)
	}
	// Company-wide: the admins.
	f.file(t, f.homeless, common)
	if got := f.notify.take(); !slices.Equal(got, []string{"release_created→0"}) {
		t.Errorf("common: notified %v, want the admins", got)
	}
	// A department without an active manager: the admins.
	f.repo.users[f.mgrDesign.ID] = domain.User{ID: f.mgrDesign.ID, Role: domain.RoleManager, DepartmentID: ptr(int64(2))}
	headless := f.assign(t, f.design, f.out)
	f.file(t, f.out, headless)
	if got := f.notify.take(); !slices.Equal(got, []string{"release_created→0"}) {
		t.Errorf("no manager: notified %v, want the admins", got)
	}
}

func TestDecideRelease(t *testing.T) {
	f := newReleaseFixture(t)
	ctx := context.Background()
	a := f.assign(t, f.dev, f.emp)
	r := f.file(t, f.emp, a)
	f.notify.take()

	tests := []struct {
		name  string
		actor domain.User
		d     domain.ReleaseDecision
		want  error
	}{
		{"reject without a comment", f.mgr, domain.ReleaseDecision{Comment: "  "}, ErrReleaseCommentRequired},
		{"the requester", f.emp, domain.ReleaseDecision{Approve: true}, ErrNotReleaseReviewer},
		{"another department's manager", f.mgrDesign, domain.ReleaseDecision{Approve: true}, ErrReleaseNotFound},
		{"a colleague", f.emp2, domain.ReleaseDecision{Approve: true}, ErrReleaseNotFound},
	}
	for _, tt := range tests {
		if _, err := f.svc.DecideRelease(ctx, tt.actor, r.ID, tt.d); !errors.Is(err, tt.want) {
			t.Errorf("%s: err = %v, want %v", tt.name, err, tt.want)
		}
	}
	if _, err := f.svc.DecideRelease(ctx, f.admin, 99, domain.ReleaseDecision{Approve: true}); !errors.Is(err, ErrReleaseNotFound) {
		t.Errorf("missing: %v", err)
	}

	// Rejected: the licence stays, the holder learns why.
	out, err := f.svc.DecideRelease(ctx, f.mgr, r.ID, domain.ReleaseDecision{Comment: "Нужна до конца квартала"})
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if out.Status != domain.ReleaseRejected || out.Comment != "Нужна до конца квартала" || *out.DecidedBy != f.mgr.ID || out.DecidedAt == nil {
		t.Errorf("rejected: %+v", out)
	}
	if got, _ := f.svc.GetAssignment(ctx, f.emp, a.ID); got.Status != domain.AssignmentActive || got.ReleaseRequestID != nil {
		t.Errorf("licence after rejection: %+v", got)
	}
	if got := f.notify.take(); !slices.Equal(got, []string{"release_rejected→3"}) {
		t.Errorf("notified %v", got)
	}
	if _, err := f.svc.DecideRelease(ctx, f.admin, r.ID, domain.ReleaseDecision{Approve: true}); !errors.Is(err, ErrReleaseNotPending) {
		t.Errorf("decided twice: %v", err)
	}

	// Asked again, the admin approves - without waiting for the manager.
	r = f.file(t, f.emp, a)
	f.notify.take()
	before := len(f.audit.Entries)
	out, err = f.svc.DecideRelease(ctx, f.admin, r.ID, domain.ReleaseDecision{Approve: true})
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if out.Status != domain.ReleaseApproved || *out.DecidedBy != f.admin.ID {
		t.Errorf("approved: %+v", out)
	}
	if got, _ := f.svc.GetAssignment(ctx, f.emp, a.ID); got.Status != domain.AssignmentRevoked || *got.RevokedBy != f.admin.ID {
		t.Errorf("licence after approval: %+v", got)
	}
	if s, _ := f.repo.GetByID(ctx, f.dev.ID); s.SeatsUsed != 0 {
		t.Errorf("seats_used = %d, want 0", s.SeatsUsed)
	}
	if got := f.notify.take(); !slices.Equal(got, []string{"release_approved→3"}) {
		t.Errorf("notified %v", got)
	}
	var actions []string
	for _, e := range f.audit.Entries[before:] {
		actions = append(actions, e.Action)
	}
	if !slices.Equal(actions, []string{"assignment.revoke", "release_request.approve"}) {
		t.Errorf("audit: %v", actions)
	}
	if d := f.audit.Entries[before].Details; d["release_request_id"] != r.ID {
		t.Errorf("revoke details: %+v", d)
	}
}

// A subscription moved to another department while the request waits is
// that department's to decide.
func TestDecideReleaseFollowsTheSubscription(t *testing.T) {
	f := newReleaseFixture(t)
	ctx := context.Background()
	a := f.assign(t, f.dev, f.emp)
	r := f.file(t, f.emp, a)
	if _, err := f.svc.Update(ctx, f.admin, f.dev.ID, domain.SubscriptionPatch{DepartmentID: domain.IDPatch{Set: true, Value: ptr(int64(2))}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.DecideRelease(ctx, f.mgr, r.ID, domain.ReleaseDecision{Approve: true}); !errors.Is(err, ErrReleaseNotFound) {
		t.Errorf("the old department's manager: %v", err)
	}
	if out, err := f.svc.DecideRelease(ctx, f.mgrDesign, r.ID, domain.ReleaseDecision{Approve: true}); err != nil || out.Status != domain.ReleaseApproved {
		t.Errorf("the new department's manager: %+v, %v", out, err)
	}
}

// Made a manager of the subscription's department while the request waited,
// the requester may decide it: they may revoke the licence anyway.
func TestDecideOwnReleaseAfterPromotion(t *testing.T) {
	f := newReleaseFixture(t)
	ctx := context.Background()
	a := f.assign(t, f.dev, f.emp)
	r := f.file(t, f.emp, a)
	promoted := f.emp
	promoted.Role = domain.RoleManager
	f.repo.users[promoted.ID] = promoted
	if out, err := f.svc.DecideRelease(ctx, promoted, r.ID, domain.ReleaseDecision{Approve: true}); err != nil || out.Status != domain.ReleaseApproved {
		t.Errorf("own after promotion: %+v, %v", out, err)
	}
}

func TestCancelRelease(t *testing.T) {
	f := newReleaseFixture(t)
	ctx := context.Background()
	a := f.assign(t, f.dev, f.emp)
	r := f.file(t, f.emp, a)

	if _, err := f.svc.CancelRelease(ctx, f.mgr, r.ID); !errors.Is(err, ErrNotReleaseRequester) {
		t.Errorf("the manager: %v", err)
	}
	if _, err := f.svc.CancelRelease(ctx, f.emp2, r.ID); !errors.Is(err, ErrReleaseNotFound) {
		t.Errorf("a colleague: %v", err)
	}
	out, err := f.svc.CancelRelease(ctx, f.emp, r.ID)
	if err != nil || out.Status != domain.ReleaseCancelled {
		t.Fatalf("cancel: %+v, %v", out, err)
	}
	if last := f.audit.Last(); last.Action != "release_request.cancel" {
		t.Errorf("audit: %+v", last)
	}
	if _, err := f.svc.CancelRelease(ctx, f.emp, r.ID); !errors.Is(err, ErrReleaseNotPending) {
		t.Errorf("twice: %v", err)
	}
	if got, _ := f.svc.GetAssignment(ctx, f.emp, a.ID); got.Status != domain.AssignmentActive || got.ReleaseRequestID != nil {
		t.Errorf("licence after cancelling: %+v", got)
	}
}

// A licence revoked another way while its request waits closes the request
// as approved by whoever revoked it, and tells the requester.
func TestReleaseClosedWithTheLicence(t *testing.T) {
	f := newReleaseFixture(t)
	ctx := context.Background()

	t.Run("revoked directly", func(t *testing.T) {
		a := f.assign(t, f.dev, f.emp)
		r := f.file(t, f.emp, a)
		f.notify.take()
		if _, err := f.svc.Revoke(ctx, f.mgr, a.ID); err != nil {
			t.Fatal(err)
		}
		got, _ := f.svc.GetRelease(ctx, f.emp, r.ID)
		if got.Status != domain.ReleaseApproved || *got.DecidedBy != f.mgr.ID {
			t.Errorf("request: %+v", got)
		}
		if sent := f.notify.take(); !slices.Equal(sent, []string{"release_approved→3"}) {
			t.Errorf("notified %v", sent)
		}
		if d := f.audit.Last().Details; d["release_requests_closed"] != 1 {
			t.Errorf("revoke details: %+v", d)
		}
	})

	t.Run("subscription cancelled", func(t *testing.T) {
		a := f.assign(t, f.common, f.emp2)
		r := f.file(t, f.emp2, a)
		other := f.assign(t, f.common, f.out) // no request: not touched
		if _, err := f.svc.Update(ctx, f.admin, f.common.ID, domain.SubscriptionPatch{Status: ptr(domain.SubCancelled)}); err != nil {
			t.Fatal(err)
		}
		if got, _ := f.svc.GetRelease(ctx, f.emp2, r.ID); got.Status != domain.ReleaseApproved || *got.DecidedBy != f.admin.ID {
			t.Errorf("request: %+v", got)
		}
		if d := f.audit.Last().Details; d["release_requests_closed"] != 1 || d["licences_revoked"] != 2 {
			t.Errorf("cancel details: %+v", d)
		}
		if got, _ := f.svc.GetAssignment(ctx, f.admin, other.ID); got.Status != domain.AssignmentRevoked {
			t.Errorf("other licence: %+v", got)
		}
	})

	t.Run("subscription deleted", func(t *testing.T) {
		a := f.assign(t, f.design, f.out)
		r := f.file(t, f.out, a)
		if err := f.svc.Delete(ctx, f.admin, f.design.ID); err != nil {
			t.Fatal(err)
		}
		if got, _ := f.repo.GetRelease(ctx, r.ID); got.Status != domain.ReleaseApproved {
			t.Errorf("request: %+v", got)
		}
		if d := f.audit.Last().Details; d["release_requests_closed"] != 1 {
			t.Errorf("delete details: %+v", d)
		}
	})
}

func TestListReleases(t *testing.T) {
	f := newReleaseFixture(t)
	ctx := context.Background()
	dev := f.file(t, f.emp, f.assign(t, f.dev, f.emp))
	common := f.file(t, f.emp, f.assign(t, f.common, f.emp))
	design := f.file(t, f.out, f.assign(t, f.design, f.out))
	foreign := f.file(t, f.mgr, f.assign(t, f.design, f.mgr))
	decided := f.file(t, f.emp2, f.assign(t, f.dev, f.emp2))
	if _, err := f.svc.DecideRelease(ctx, f.mgr, decided.ID, domain.ReleaseDecision{Comment: "Нет"}); err != nil {
		t.Fatal(err)
	}

	// name lists requests as "Jira#1,Slack#2".
	name := func(rs ...domain.ReleaseRequest) string {
		var out []string
		for _, r := range rs {
			out = append(out, r.SubscriptionName+"#"+strconv.FormatInt(r.ID, 10))
		}
		return strings.Join(out, ",")
	}
	ids := func(actor domain.User, flt domain.ReleaseFilter, inbox bool) string {
		items, total, err := f.svc.ListReleases(ctx, actor, flt, inbox, paging.Params{})
		if err != nil {
			t.Fatal(err)
		}
		if int(total) != len(items) {
			t.Errorf("total %d, items %d", total, len(items))
		}
		return name(items...)
	}

	tests := []struct {
		name  string
		actor domain.User
		flt   domain.ReleaseFilter
		inbox bool
		want  string
	}{
		{"admin: all, newest first", f.admin, domain.ReleaseFilter{}, false, name(decided, foreign, design, common, dev)},
		{"manager: the department's and their own", f.mgr, domain.ReleaseFilter{}, false, name(decided, foreign, dev)},
		{"employee: their own", f.emp, domain.ReleaseFilter{}, false, name(common, dev)},
		{"by status", f.admin, domain.ReleaseFilter{Status: ptr(domain.ReleaseRejected)}, false, name(decided)},
		{"by department", f.admin, domain.ReleaseFilter{DepartmentID: ptr(int64(2))}, false, name(foreign, design)},
		{"by subscription", f.admin, domain.ReleaseFilter{SubscriptionID: &f.common.ID}, false, name(common)},
		{"by requester", f.admin, domain.ReleaseFilter{RequesterID: &f.emp.ID}, false, name(common, dev)},
		// The inbox: what waits for the caller.
		{"manager's inbox", f.mgr, domain.ReleaseFilter{}, true, name(dev)},
		{"other manager's inbox", f.mgrDesign, domain.ReleaseFilter{}, true, name(foreign, design)},
		{"admin's inbox: nobody else can decide", f.admin, domain.ReleaseFilter{}, true, name(common)},
		{"employee's inbox", f.emp, domain.ReleaseFilter{}, true, ""},
		{"a manager without a department", domain.User{ID: 50, Role: domain.RoleManager}, domain.ReleaseFilter{}, true, ""},
	}
	for _, tt := range tests {
		if got := ids(tt.actor, tt.flt, tt.inbox); got != tt.want {
			t.Errorf("%s:\n got %s\nwant %s", tt.name, got, tt.want)
		}
	}

	// The design department loses its manager: its requests are the admin's.
	f.repo.users[f.mgrDesign.ID] = domain.User{ID: f.mgrDesign.ID, Role: domain.RoleManager, DepartmentID: ptr(int64(2))}
	if got := ids(f.admin, domain.ReleaseFilter{}, true); got != name(foreign, design, common) {
		t.Errorf("admin's inbox without a design manager: %s", got)
	}
}
