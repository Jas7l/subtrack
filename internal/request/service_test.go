package request

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"subtrack/internal/apperr"
	"subtrack/internal/auth"
	"subtrack/internal/domain"
	"subtrack/internal/paging"
	"subtrack/internal/storage"
	"subtrack/internal/testkit"
)

// fakeRepo is an in-memory requests table with the conditional decision
// writes of the real store.
type fakeRepo struct {
	mu     sync.Mutex
	reqs   map[int64]domain.Request
	nextID int64
}

func (f *fakeRepo) GetByID(_ context.Context, id int64) (domain.Request, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.reqs[id]
	if !ok {
		return domain.Request{}, storage.ErrNotFound
	}
	return r, nil
}

func (f *fakeRepo) GetForUpdate(ctx context.Context, id int64) (domain.Request, error) {
	return f.GetByID(ctx, id)
}

func (f *fakeRepo) List(_ context.Context, flt domain.RequestFilter, sc *domain.RequestScope, limit, offset int) ([]domain.Request, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Request
	for id := f.nextID - 1; id >= 1; id-- {
		r, ok := f.reqs[id]
		inScope := sc == nil ||
			(sc.DepartmentID != nil && r.DepartmentID != nil && *sc.DepartmentID == *r.DepartmentID) ||
			(sc.RequesterID != nil && *sc.RequesterID == r.RequesterID)
		switch {
		case !ok, !inScope:
		case flt.Status != nil && r.Status != *flt.Status:
		default:
			out = append(out, r)
		}
	}
	total := int64(len(out))
	out = out[min(offset, len(out)):]
	return out[:min(limit, len(out))], total, nil
}

func (f *fakeRepo) CurrencyExists(_ context.Context, code string) (bool, error) {
	return code == "RUB" || code == "USD", nil
}

func (f *fakeRepo) Create(_ context.Context, r domain.Request) (domain.Request, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r.ID = f.nextID
	f.nextID++
	r.CreatedAt, r.UpdatedAt = time.Now(), time.Now()
	f.reqs[r.ID] = r
	return r, nil
}

func (f *fakeRepo) DecideAsManager(_ context.Context, id int64, d domain.Decision) (domain.Request, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.reqs[id]
	if !ok || r.Status != domain.ReqPendingManager {
		return domain.Request{}, storage.ErrNotFound
	}
	r.Status = domain.ReqRejectedByManager
	if d.Approve {
		r.Status = domain.ReqPendingAdmin
	}
	now := time.Now()
	r.ManagerID, r.ManagerComment, r.ManagerDecidedAt = &d.ReviewerID, d.Comment, &now
	f.reqs[id] = r
	return r, nil
}

func (f *fakeRepo) DecideAsAdmin(_ context.Context, id int64, d domain.Decision) (domain.Request, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.reqs[id]
	if !ok || !r.Status.Pending() {
		return domain.Request{}, storage.ErrNotFound
	}
	r.Status = domain.ReqRejectedByAdmin
	if d.Approve {
		r.Status = domain.ReqApproved
	}
	now := time.Now()
	r.AdminID, r.AdminComment, r.AdminDecidedAt = &d.ReviewerID, d.Comment, &now
	if d.SubscriptionID != nil {
		r.SubscriptionID = d.SubscriptionID
	}
	f.reqs[id] = r
	return r, nil
}

func (f *fakeRepo) Cancel(_ context.Context, id, requesterID int64) (domain.Request, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.reqs[id]
	if !ok || r.RequesterID != requesterID || !r.Status.Pending() {
		return domain.Request{}, storage.ErrNotFound
	}
	r.Status = domain.ReqCancelled
	f.reqs[id] = r
	return r, nil
}

// fakeDepts maps department id → head id (nil: no head).
type fakeDepts map[int64]*int64

func (f fakeDepts) GetByID(_ context.Context, id int64) (domain.Department, error) {
	head, ok := f[id]
	if !ok {
		return domain.Department{}, storage.ErrNotFound
	}
	return domain.Department{ID: id, HeadID: head}, nil
}

// errNoSeats stands in for subscription.ErrNoFreeSeats.
var errNoSeats = apperr.New(apperr.ErrConflict, "service: no free seats left")

// fakeSubs records what an approval asked of the subscriptions domain.
type fakeSubs struct {
	created  []domain.Subscription
	assigned [][2]int64 // {subscription, user}
	failWith error
}

func (f *fakeSubs) Create(_ context.Context, _ domain.User, s domain.Subscription) (domain.Subscription, error) {
	s.ID = int64(100 + len(f.created))
	f.created = append(f.created, s)
	return s, nil
}

func (f *fakeSubs) Assign(_ context.Context, _ domain.User, subID, userID int64) (domain.Assignment, error) {
	if f.failWith != nil {
		return domain.Assignment{}, f.failWith
	}
	f.assigned = append(f.assigned, [2]int64{subID, userID})
	return domain.Assignment{SubscriptionID: subID, UserID: userID}, nil
}

// fakeTx runs fn directly; the fakes have nothing to roll back because the
// service writes the decision last.
type fakeTx struct{ calls int }

func (f *fakeTx) InTx(ctx context.Context, fn func(ctx context.Context) error) error {
	f.calls++
	return fn(ctx)
}

// fakeNotify keeps what was sent; admins get one entry with UserID 0.
type fakeNotify struct {
	mu   sync.Mutex
	sent []domain.Notification
}

func (f *fakeNotify) Send(_ context.Context, n domain.Notification) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, n)
	return nil
}

func (f *fakeNotify) SendToAdmins(_ context.Context, n domain.Notification) error {
	n.UserID = 0
	return f.Send(context.Background(), n)
}

// to lists who got notified, in order (0: all admins), with the type.
func (f *fakeNotify) to() []string {
	var out []string
	for _, n := range f.sent {
		out = append(out, strconv.FormatInt(n.UserID, 10)+":"+string(n.Type))
	}
	return out
}

func ptr[T any](v T) *T { return &v }

// fixture: department 1 (dev) headed by head; department 2 (design) without a
// head; dev has a second, non-head manager.
type fixture struct {
	svc    *Service
	repo   *fakeRepo
	subs   *fakeSubs
	tx     *fakeTx
	audit  *testkit.Audit
	notify *fakeNotify

	admin, head, mgr2, emp, emp2, designer, homeless domain.User
}

func newFixture() fixture {
	dev, design := int64(1), int64(2)
	f := fixture{
		repo:     &fakeRepo{reqs: map[int64]domain.Request{}, nextID: 1},
		subs:     &fakeSubs{},
		tx:       &fakeTx{},
		audit:    &testkit.Audit{},
		notify:   &fakeNotify{},
		admin:    domain.User{ID: 1, Role: domain.RoleAdmin, IsActive: true},
		head:     domain.User{ID: 2, Role: domain.RoleManager, DepartmentID: &dev, IsActive: true},
		mgr2:     domain.User{ID: 3, Role: domain.RoleManager, DepartmentID: &dev, IsActive: true},
		emp:      domain.User{ID: 4, Role: domain.RoleEmployee, DepartmentID: &dev, IsActive: true},
		emp2:     domain.User{ID: 5, Role: domain.RoleEmployee, DepartmentID: &dev, IsActive: true},
		designer: domain.User{ID: 6, Role: domain.RoleEmployee, DepartmentID: &design, IsActive: true},
		homeless: domain.User{ID: 7, Role: domain.RoleEmployee, IsActive: true},
	}
	f.svc = New(f.repo, fakeDepts{dev: &f.head.ID, design: nil}, f.subs, f.tx, f.audit, f.notify)
	return f
}

func (f fixture) file(t *testing.T, as domain.User) domain.Request {
	t.Helper()
	r, err := f.svc.Create(context.Background(), as, CreateRequestInput{Name: "Figma", Justification: "макеты"})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestCreateRouting(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	tests := []struct {
		name string
		as   domain.User
		want domain.RequestStatus
	}{
		{"employee with a head", f.emp, domain.ReqPendingManager},
		{"employee, department without head", f.designer, domain.ReqPendingAdmin},
		{"employee without department", f.homeless, domain.ReqPendingAdmin},
		{"the head", f.head, domain.ReqPendingAdmin},
		{"another manager", f.mgr2, domain.ReqPendingAdmin},
	}
	for _, tt := range tests {
		if r := f.file(t, tt.as); r.Status != tt.want {
			t.Errorf("%s: status %s, want %s", tt.name, r.Status, tt.want)
		}
	}

	bad := []struct {
		name string
		as   domain.User
		in   CreateRequestInput
		want error
	}{
		{"admin", f.admin, CreateRequestInput{Name: "X", Justification: "y"}, auth.ErrForbidden},
		{"no name", f.emp, CreateRequestInput{Justification: "y"}, apperr.ErrInvalidInput},
		{"no justification", f.emp, CreateRequestInput{Name: "X", Justification: " "}, apperr.ErrInvalidInput},
		{"bad cost", f.emp, CreateRequestInput{Name: "X", Justification: "y", EstimatedCost: ptr("1,5")}, apperr.ErrInvalidInput},
		{"unknown currency", f.emp, CreateRequestInput{Name: "X", Justification: "y", Currency: "eur"}, ErrUnknownCurrency},
	}
	for _, tt := range bad {
		if _, err := f.svc.Create(ctx, tt.as, tt.in); !errors.Is(err, tt.want) {
			t.Errorf("%s: err = %v, want %v", tt.name, err, tt.want)
		}
	}
	r, err := f.svc.Create(ctx, f.emp, CreateRequestInput{Name: "X", Justification: "y", EstimatedCost: ptr("10"), Currency: "usd"})
	if err != nil || r.Currency != "USD" || *r.EstimatedCost != "10" {
		t.Errorf("currency/cost: %+v, %v", r, err)
	}
}

func TestFullApprovalNewSubscription(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	r := f.file(t, f.emp)

	if _, err := f.svc.Decide(ctx, f.admin, r.ID, Verdict{Approve: true, SubscriptionID: ptr(int64(9))}); !errors.Is(err, ErrWaitsForHead) {
		t.Fatalf("admin before head: err = %v", err)
	}
	r, err := f.svc.Decide(ctx, f.head, r.ID, Verdict{Approve: true})
	if err != nil || r.Status != domain.ReqPendingAdmin || *r.ManagerID != f.head.ID {
		t.Fatalf("head approves: %+v, %v", r, err)
	}
	if _, err := f.svc.Decide(ctx, f.head, r.ID, Verdict{Approve: true}); !errors.Is(err, ErrWaitsForAdmin) {
		t.Errorf("head twice: err = %v", err)
	}

	r, err = f.svc.Decide(ctx, f.admin, r.ID, Verdict{Approve: true, NewSubscription: &NewSubscription{
		Subscription: domain.Subscription{Type: domain.SubGroup, Cost: "100", Currency: "RUB", SeatsTotal: ptr(5)},
	}})
	if err != nil || r.Status != domain.ReqApproved || r.SubscriptionID == nil || *r.SubscriptionID != 100 {
		t.Fatalf("admin approves: %+v, %v", r, err)
	}
	created := f.subs.created[0]
	if created.Name != "Figma" || created.DepartmentID == nil || *created.DepartmentID != 1 {
		t.Errorf("new subscription defaults: %+v", created)
	}
	if len(f.subs.assigned) != 1 || f.subs.assigned[0] != [2]int64{100, f.emp.ID} {
		t.Errorf("licence: %v", f.subs.assigned)
	}
	if _, err := f.svc.Decide(ctx, f.admin, r.ID, Verdict{Comment: "нет"}); !errors.Is(err, ErrNotPending) {
		t.Errorf("decide approved: err = %v", err)
	}

	// head gets the new request; the requester hears of the head's approval;
	// the admins get it next; the requester hears of the final approval.
	wantTo := "2:request_created,4:request_approved,0:request_created,4:request_approved"
	if got := strings.Join(f.notify.to(), ","); got != wantTo {
		t.Errorf("notifications:\n got %s\nwant %s", got, wantTo)
	}
	for _, n := range f.notify.sent {
		if n.EntityType != domain.EntityRequest || n.EntityID != r.ID {
			t.Errorf("%s links to %s %d, want request %d", n.Type, n.EntityType, n.EntityID, r.ID)
		}
	}
	wantAudit := "request.create,request.approve,request.approve"
	if got := strings.Join(f.audit.Actions(), ","); got != wantAudit {
		t.Errorf("audit: got %s, want %s", got, wantAudit)
	}
	if d := f.audit.Last().Details; d["stage"] != "admin" || d["subscription_id"] != int64(100) {
		t.Errorf("final audit details: %+v", d)
	}
}

func TestRejectionNotifiesWithComment(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	r := f.file(t, f.emp)
	if _, err := f.svc.Decide(ctx, f.head, r.ID, Verdict{Comment: "есть Canva"}); err != nil {
		t.Fatal(err)
	}
	last := f.notify.sent[len(f.notify.sent)-1]
	if last.UserID != f.emp.ID || last.Type != domain.NotifyRequestRejected || !strings.Contains(last.Body, "есть Canva") {
		t.Errorf("rejection notification: %+v", last)
	}
	// A request straight to the admin notifies the admins, not a head.
	f.file(t, f.designer)
	if last := f.notify.sent[len(f.notify.sent)-1]; last.UserID != 0 {
		t.Errorf("headless request notified user %d", last.UserID)
	}
}

func TestAdminApprovalFails(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	r := f.file(t, f.designer) // pending_admin: no head
	f.subs.failWith = errNoSeats

	_, err := f.svc.Decide(ctx, f.admin, r.ID, Verdict{Approve: true, SubscriptionID: ptr(int64(9))})
	if !errors.Is(err, errNoSeats) || !errors.Is(err, apperr.ErrConflict) {
		t.Fatalf("no seats: err = %v", err)
	}
	if got, _ := f.repo.GetByID(ctx, r.ID); got.Status != domain.ReqPendingAdmin {
		t.Errorf("status after failed approval: %s", got.Status)
	}
	if f.tx.calls != 2 { // filing + the failed approval
		t.Errorf("transactions: %d", f.tx.calls)
	}

	f.subs.failWith = nil
	got, err := f.svc.Decide(ctx, f.admin, r.ID, Verdict{Approve: true, SubscriptionID: ptr(int64(9))})
	if err != nil || got.Status != domain.ReqApproved || *got.SubscriptionID != 9 {
		t.Errorf("retry: %+v, %v", got, err)
	}
}

func TestDecideRules(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	devReq := f.file(t, f.emp)
	designReq := f.file(t, f.designer)

	tests := []struct {
		name string
		as   domain.User
		id   int64
		v    Verdict
		want error
	}{
		{"reject without comment", f.head, devReq.ID, Verdict{Comment: "  "}, ErrCommentRequired},
		{"employee", f.emp2, devReq.ID, Verdict{Approve: true}, auth.ErrForbidden},
		{"non-head manager of the department", f.mgr2, devReq.ID, Verdict{Approve: true}, ErrNotHead},
		{"head picks a subscription", f.head, devReq.ID, Verdict{Approve: true, SubscriptionID: ptr(int64(1))}, apperr.ErrInvalidInput},
		{"head of another department", f.head, designReq.ID, Verdict{Approve: true}, ErrRequestNotFound},
		{"admin approves without subscription", f.admin, designReq.ID, Verdict{Approve: true}, apperr.ErrInvalidInput},
		{"admin approves with both", f.admin, designReq.ID, Verdict{Approve: true, SubscriptionID: ptr(int64(1)),
			NewSubscription: &NewSubscription{}}, apperr.ErrInvalidInput},
		{"rejection with subscription", f.admin, designReq.ID, Verdict{Comment: "x", SubscriptionID: ptr(int64(1))}, apperr.ErrInvalidInput},
		{"missing", f.admin, 99, Verdict{Comment: "x"}, ErrRequestNotFound},
		{"head rejects", f.head, devReq.ID, Verdict{Comment: "есть Canva"}, nil},
		{"after rejection", f.admin, devReq.ID, Verdict{Comment: "x"}, ErrNotPending},
		{"admin rejects", f.admin, designReq.ID, Verdict{Comment: "дорого"}, nil},
	}
	for _, tt := range tests {
		if _, err := f.svc.Decide(ctx, tt.as, tt.id, tt.v); !errors.Is(err, tt.want) {
			t.Errorf("%s: err = %v, want %v", tt.name, err, tt.want)
		}
	}
	if r, _ := f.repo.GetByID(ctx, devReq.ID); r.Status != domain.ReqRejectedByManager || r.ManagerComment != "есть Canva" {
		t.Errorf("dev request: %+v", r)
	}
	if r, _ := f.repo.GetByID(ctx, designReq.ID); r.Status != domain.ReqRejectedByAdmin {
		t.Errorf("design request: %s", r.Status)
	}
}

func TestAdminDecidesWhenHeadIsGone(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	r := f.file(t, f.emp) // pending_manager
	// The head was removed after the request was filed.
	f.svc.depts = fakeDepts{1: nil}
	got, err := f.svc.Decide(ctx, f.admin, r.ID, Verdict{Approve: true, SubscriptionID: ptr(int64(9))})
	if err != nil || got.Status != domain.ReqApproved {
		t.Errorf("admin on headless pending_manager: %+v, %v", got, err)
	}
}

func TestNoDecisionOnOwnRequest(t *testing.T) {
	f := newFixture()
	ctx := context.Background()

	// The employee was made the head while their request waited for the head.
	r := f.file(t, f.emp)
	head := f.emp
	head.Role = domain.RoleManager
	f.svc.depts = fakeDepts{1: &head.ID}
	if _, err := f.svc.Decide(ctx, head, r.ID, Verdict{Approve: true}); !errors.Is(err, ErrOwnRequest) || !errors.Is(err, auth.ErrForbidden) {
		t.Errorf("head on own request: err = %v, want ErrOwnRequest", err)
	}
	// Nobody else heads the department, so the admin decides it.
	if got, err := f.svc.Decide(ctx, f.admin, r.ID, Verdict{Approve: true, SubscriptionID: ptr(int64(9))}); err != nil || got.Status != domain.ReqApproved {
		t.Errorf("admin on the head's own request: %+v, %v", got.Status, err)
	}

	// A manager's request goes to the admin; they were made admin meanwhile.
	r = f.file(t, f.mgr2)
	admin := f.mgr2
	admin.Role = domain.RoleAdmin
	if _, err := f.svc.Decide(ctx, admin, r.ID, Verdict{Comment: "x"}); !errors.Is(err, ErrOwnRequest) {
		t.Errorf("admin on own request: err = %v, want ErrOwnRequest", err)
	}
	if got, _ := f.repo.GetByID(ctx, r.ID); got.Status != domain.ReqPendingAdmin {
		t.Errorf("refused decision changed the request: %s", got.Status)
	}
}

func TestCancel(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	r := f.file(t, f.emp)

	if _, err := f.svc.Cancel(ctx, f.head, r.ID); !errors.Is(err, ErrNotRequester) {
		t.Errorf("head cancels: err = %v", err)
	}
	if _, err := f.svc.Cancel(ctx, f.emp2, r.ID); !errors.Is(err, ErrRequestNotFound) {
		t.Errorf("colleague cancels: err = %v", err)
	}
	got, err := f.svc.Cancel(ctx, f.emp, r.ID)
	if err != nil || got.Status != domain.ReqCancelled {
		t.Fatalf("cancel: %+v, %v", got, err)
	}
	if _, err := f.svc.Cancel(ctx, f.emp, r.ID); !errors.Is(err, ErrNotPending) {
		t.Errorf("twice: err = %v", err)
	}
	if _, err := f.svc.Decide(ctx, f.head, r.ID, Verdict{Approve: true}); !errors.Is(err, ErrNotPending) {
		t.Errorf("decide cancelled: err = %v", err)
	}
}

func TestListScope(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	f.file(t, f.emp)
	f.file(t, f.emp2)
	f.file(t, f.designer)
	f.file(t, f.head)

	count := func(as domain.User, flt domain.RequestFilter) int64 {
		_, n, err := f.svc.List(ctx, as, flt, paging.Params{})
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count(f.admin, domain.RequestFilter{}); n != 4 {
		t.Errorf("admin: %d", n)
	}
	if n := count(f.head, domain.RequestFilter{}); n != 3 {
		t.Errorf("head: %d", n)
	}
	if n := count(f.head, domain.RequestFilter{Status: ptr(domain.ReqPendingManager)}); n != 2 {
		t.Errorf("head inbox: %d", n)
	}
	if n := count(f.emp, domain.RequestFilter{}); n != 1 {
		t.Errorf("employee: %d", n)
	}
}
