package subscription

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"subtrack/internal/domain"
	"subtrack/internal/storage"
	"subtrack/internal/testkit"
)

// fakeRepo is an in-memory subscriptions + assignments + release requests
// store with the rules the service relies on: soft delete, the seats CHECK,
// one active licence per (subscription, user), revoke-all on cancel and
// delete, one pending release request per licence, and the visibility
// scope. Locking is not modelled.
type fakeRepo struct {
	mu         sync.Mutex
	subs       map[int64]domain.Subscription
	deleted    map[int64]bool
	asg        map[int64]domain.Assignment
	rel        map[int64]domain.ReleaseRequest
	currencies map[string]bool
	users      fakeUsers
	nextSub    int64
	nextAsg    int64
	nextRel    int64
}

func newFakeRepo(users fakeUsers) *fakeRepo {
	return &fakeRepo{
		subs:       map[int64]domain.Subscription{},
		deleted:    map[int64]bool{},
		asg:        map[int64]domain.Assignment{},
		rel:        map[int64]domain.ReleaseRequest{},
		currencies: map[string]bool{"RUB": true, "USD": true},
		users:      users,
		nextSub:    1,
		nextAsg:    1,
		nextRel:    1,
	}
}

func (f *fakeRepo) GetByID(_ context.Context, id int64) (domain.Subscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.subs[id]
	if !ok || f.deleted[id] {
		return domain.Subscription{}, storage.ErrNotFound
	}
	return s, nil
}

func (f *fakeRepo) GetForUpdate(ctx context.Context, id int64) (domain.Subscription, error) {
	return f.GetByID(ctx, id)
}

func (f *fakeRepo) holds(subID, userID int64) bool {
	for _, a := range f.asg {
		if a.SubscriptionID == subID && a.UserID == userID && a.Status == domain.AssignmentActive {
			return true
		}
	}
	return false
}

func inScope(sc *domain.SubscriptionScope, dept *int64, holder bool) bool {
	if sc == nil {
		return true
	}
	return (sc.DepartmentID != nil && dept != nil && *sc.DepartmentID == *dept) || (sc.HolderID != nil && holder)
}

func (f *fakeRepo) List(_ context.Context, flt domain.SubscriptionFilter, sc *domain.SubscriptionScope, limit, offset int) ([]domain.Subscription, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Subscription
	for id := int64(1); id < f.nextSub; id++ {
		s, ok := f.subs[id]
		switch {
		case !ok, f.deleted[id]:
		case !inScope(sc, s.DepartmentID, sc != nil && sc.HolderID != nil && f.holds(id, *sc.HolderID)):
		case flt.DepartmentID != nil && (s.DepartmentID == nil || *s.DepartmentID != *flt.DepartmentID):
		case flt.Category != "" && !strings.EqualFold(s.Category, flt.Category):
		case flt.Status != nil && s.Status != *flt.Status:
		case flt.Type != nil && s.Type != *flt.Type:
		case flt.EndFrom != nil && (s.EndDate == nil || s.EndDate.Before(*flt.EndFrom)):
		case flt.EndTo != nil && (s.EndDate == nil || s.EndDate.After(*flt.EndTo)):
		case flt.Query != "" && !strings.Contains(strings.ToLower(s.Name+" "+s.Vendor), strings.ToLower(flt.Query)):
		case flt.FreeSeats != nil && (s.SeatsTotal == nil || (s.SeatsUsed < *s.SeatsTotal) != *flt.FreeSeats):
		default:
			out = append(out, s)
		}
	}
	total := int64(len(out))
	out = out[min(offset, len(out)):]
	return out[:min(limit, len(out))], total, nil
}

func (f *fakeRepo) CurrencyExists(_ context.Context, code string) (bool, error) {
	return f.currencies[code], nil
}

func (f *fakeRepo) Create(_ context.Context, s domain.Subscription) (domain.Subscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s.ID = f.nextSub
	f.nextSub++
	s.SeatsUsed = 0
	s.CreatedAt, s.UpdatedAt = time.Now(), time.Now()
	f.subs[s.ID] = s
	return s, nil
}

func (f *fakeRepo) revokeAll(subID, actorID int64) {
	now := time.Now()
	for id, a := range f.asg {
		if a.SubscriptionID == subID && a.Status == domain.AssignmentActive {
			a.Status, a.RevokedAt, a.RevokedBy = domain.AssignmentRevoked, &now, &actorID
			f.asg[id] = a
		}
	}
}

func (f *fakeRepo) Update(_ context.Context, s domain.Subscription, actorID int64) (domain.Subscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cur, ok := f.subs[s.ID]
	if !ok || f.deleted[s.ID] {
		return domain.Subscription{}, storage.ErrNotFound
	}
	s.SeatsUsed = cur.SeatsUsed
	if s.Status == domain.SubCancelled {
		s.SeatsUsed = 0
		f.revokeAll(s.ID, actorID)
	}
	if s.SeatsTotal != nil && s.SeatsUsed > *s.SeatsTotal {
		return domain.Subscription{}, &storage.CheckError{Constraint: "subscriptions_seats_check"}
	}
	f.subs[s.ID] = s
	return s, nil
}

func (f *fakeRepo) Delete(_ context.Context, id, actorID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.subs[id]
	if !ok || f.deleted[id] {
		return storage.ErrNotFound
	}
	f.deleted[id] = true
	s.SeatsUsed = 0
	f.subs[id] = s
	f.revokeAll(id, actorID)
	return nil
}

func (f *fakeRepo) GetAssignment(_ context.Context, id int64) (domain.Assignment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.asg[id]
	if !ok {
		return domain.Assignment{}, storage.ErrNotFound
	}
	return f.fill(a), nil
}

// fill sets what the store's reads join in: the subscription's department
// and the release request that waits.
func (f *fakeRepo) fill(a domain.Assignment) domain.Assignment {
	a.SubscriptionDepartmentID = f.subs[a.SubscriptionID].DepartmentID
	a.ReleaseRequestID = nil
	for id, r := range f.rel {
		if r.AssignmentID == a.ID && r.Status == domain.ReleasePending {
			a.ReleaseRequestID = &id
		}
	}
	return a
}

func (f *fakeRepo) HasActiveAssignment(_ context.Context, subID, userID int64) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.holds(subID, userID), nil
}

func (f *fakeRepo) ListAssignments(_ context.Context, flt domain.AssignmentFilter, sc *domain.SubscriptionScope, limit, offset int) ([]domain.Assignment, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Assignment
	for id := f.nextAsg - 1; id >= 1; id-- {
		a, ok := f.asg[id]
		if !ok {
			continue
		}
		a = f.fill(a)
		switch {
		case !inScope(sc, a.SubscriptionDepartmentID, sc != nil && sc.HolderID != nil && a.UserID == *sc.HolderID):
		case flt.SubscriptionID != nil && a.SubscriptionID != *flt.SubscriptionID:
		case flt.UserID != nil && a.UserID != *flt.UserID:
		case flt.Status != nil && a.Status != *flt.Status:
		default:
			out = append(out, a)
		}
	}
	total := int64(len(out))
	out = out[min(offset, len(out)):]
	return out[:min(limit, len(out))], total, nil
}

func (f *fakeRepo) Assign(_ context.Context, subID, userID, actorID int64) (domain.Assignment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if u, ok := f.users[userID]; !ok || !u.IsActive {
		return domain.Assignment{}, errAssigneeGone
	}
	s, ok := f.subs[subID]
	if !ok || f.deleted[subID] || !s.Status.Open() {
		return domain.Assignment{}, errSubscriptionGone
	}
	if s.SeatsTotal != nil && s.SeatsUsed+1 > *s.SeatsTotal {
		return domain.Assignment{}, &storage.CheckError{Constraint: "subscriptions_seats_check"}
	}
	if f.holds(subID, userID) {
		return domain.Assignment{}, &storage.ConstraintError{Constraint: "subscription_assignments_active_key"}
	}
	s.SeatsUsed++
	f.subs[subID] = s
	assignedBy := f.users[actorID].FullName
	a := domain.Assignment{
		ID: f.nextAsg, SubscriptionID: subID, UserID: userID, Status: domain.AssignmentActive,
		AssignedBy: &actorID, AssignedByName: &assignedBy, AssignedAt: time.Now(),
		SubscriptionName: s.Name, SubscriptionDepartmentID: s.DepartmentID, UserName: f.users[userID].FullName,
	}
	f.nextAsg++
	f.asg[a.ID] = a
	return a, nil
}

func (f *fakeRepo) Revoke(_ context.Context, id, actorID int64) (domain.Assignment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.asg[id]
	if !ok || a.Status != domain.AssignmentActive {
		return domain.Assignment{}, storage.ErrNotFound
	}
	now := time.Now()
	revokedBy := f.users[actorID].FullName
	a.Status, a.RevokedAt, a.RevokedBy, a.RevokedByName = domain.AssignmentRevoked, &now, &actorID, &revokedBy
	f.asg[id] = a
	s := f.subs[a.SubscriptionID]
	s.SeatsUsed--
	f.subs[a.SubscriptionID] = s
	return f.fill(a), nil
}

// release returns r with what the store's reads join in, as of now.
func (f *fakeRepo) release(r domain.ReleaseRequest) domain.ReleaseRequest {
	a := f.asg[r.AssignmentID]
	s := f.subs[a.SubscriptionID]
	r.SubscriptionID, r.SubscriptionName, r.DepartmentID = s.ID, s.Name, s.DepartmentID
	r.RequesterName = f.users[r.RequesterID].FullName
	r.DecidedByName = nil
	if r.DecidedBy != nil {
		name := f.users[*r.DecidedBy].FullName
		r.DecidedByName = &name
	}
	return r
}

func (f *fakeRepo) GetRelease(_ context.Context, id int64) (domain.ReleaseRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rel[id]
	if !ok {
		return domain.ReleaseRequest{}, storage.ErrNotFound
	}
	return f.release(r), nil
}

func (f *fakeRepo) LockRelease(ctx context.Context, id int64) (domain.ReleaseRequest, error) {
	return f.GetRelease(ctx, id)
}

// attended reports whether department dept has an active manager.
func (f *fakeRepo) attended(dept *int64) bool {
	if dept == nil {
		return false
	}
	for _, u := range f.users {
		if u.Role == domain.RoleManager && u.IsActive && u.DepartmentID != nil && *u.DepartmentID == *dept {
			return true
		}
	}
	return false
}

func (f *fakeRepo) ListReleases(_ context.Context, flt domain.ReleaseFilter, sc *domain.SubscriptionScope, limit, offset int) ([]domain.ReleaseRequest, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.ReleaseRequest
	for id := f.nextRel - 1; id >= 1; id-- {
		r, ok := f.rel[id]
		if !ok {
			continue
		}
		r = f.release(r)
		switch {
		case !inScope(sc, r.DepartmentID, sc != nil && sc.HolderID != nil && r.RequesterID == *sc.HolderID):
		case flt.Status != nil && r.Status != *flt.Status:
		case flt.RequesterID != nil && r.RequesterID != *flt.RequesterID:
		case flt.SubscriptionID != nil && r.SubscriptionID != *flt.SubscriptionID:
		case flt.DepartmentID != nil && (r.DepartmentID == nil || *r.DepartmentID != *flt.DepartmentID):
		case flt.Unattended && f.attended(r.DepartmentID):
		default:
			out = append(out, r)
		}
	}
	total := int64(len(out))
	out = out[min(offset, len(out)):]
	return out[:min(limit, len(out))], total, nil
}

func (f *fakeRepo) CreateRelease(_ context.Context, r domain.ReleaseRequest) (domain.ReleaseRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.asg[r.AssignmentID]
	if !ok || a.Status != domain.AssignmentActive || a.UserID != r.RequesterID {
		return domain.ReleaseRequest{}, errLicenceGone
	}
	for _, other := range f.rel {
		if other.AssignmentID == r.AssignmentID && other.Status == domain.ReleasePending {
			return domain.ReleaseRequest{}, &storage.ConstraintError{Constraint: "release_requests_pending_key"}
		}
	}
	r.ID = f.nextRel
	f.nextRel++
	r.Status = domain.ReleasePending
	r.CreatedAt, r.UpdatedAt = time.Now(), time.Now()
	f.rel[r.ID] = r
	return f.release(r), nil
}

func (f *fakeRepo) DecideRelease(_ context.Context, id int64, d domain.ReleaseDecision) (domain.ReleaseRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rel[id]
	if !ok || r.Status != domain.ReleasePending {
		return domain.ReleaseRequest{}, storage.ErrNotFound
	}
	now := time.Now()
	r.Status = domain.ReleaseRejected
	if d.Approve {
		r.Status = domain.ReleaseApproved
	}
	r.DecidedBy, r.Comment, r.DecidedAt, r.UpdatedAt = &d.ReviewerID, d.Comment, &now, now
	f.rel[id] = r
	return f.release(r), nil
}

func (f *fakeRepo) CancelRelease(_ context.Context, id, requesterID int64) (domain.ReleaseRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rel[id]
	if !ok || r.RequesterID != requesterID || r.Status != domain.ReleasePending {
		return domain.ReleaseRequest{}, storage.ErrNotFound
	}
	r.Status, r.UpdatedAt = domain.ReleaseCancelled, time.Now()
	f.rel[id] = r
	return f.release(r), nil
}

func (f *fakeRepo) CloseReleases(_ context.Context, subID, actorID int64) ([]domain.ReleaseRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.ReleaseRequest
	for id := int64(1); id < f.nextRel; id++ {
		r, ok := f.rel[id]
		if !ok || r.Status != domain.ReleasePending {
			continue
		}
		a := f.asg[r.AssignmentID]
		if a.SubscriptionID != subID || a.Status != domain.AssignmentRevoked {
			continue
		}
		now := time.Now()
		r.Status, r.DecidedBy, r.DecidedAt, r.UpdatedAt = domain.ReleaseApproved, &actorID, &now, now
		f.rel[id] = r
		out = append(out, f.release(r))
	}
	return out, nil
}

// fakeNotify keeps what was sent: to one user, to the managers of a
// department (as many entries as it has active managers), to every admin
// (one entry with UserID 0).
type fakeNotify struct {
	mu    sync.Mutex
	users fakeUsers
	sent  []domain.Notification
}

func (f *fakeNotify) Send(_ context.Context, n domain.Notification) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, n)
	return nil
}

func (f *fakeNotify) SendToAdmins(_ context.Context, n domain.Notification) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	n.UserID = 0
	f.sent = append(f.sent, n)
	return nil
}

func (f *fakeNotify) SendToManagers(_ context.Context, deptID int64, n domain.Notification) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var count int64
	for id := int64(1); id <= int64(len(f.users))+10; id++ {
		u, ok := f.users[id]
		if ok && u.Role == domain.RoleManager && u.IsActive && u.DepartmentID != nil && *u.DepartmentID == deptID {
			n.UserID = id
			f.sent = append(f.sent, n)
			count++
		}
	}
	return count, nil
}

// take returns what was sent since the last call, as "type→user" with
// user 0 for the admins, and forgets it.
func (f *fakeNotify) take() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.sent))
	for _, n := range f.sent {
		out = append(out, string(n.Type)+"→"+strconv.FormatInt(n.UserID, 10))
	}
	f.sent = nil
	return out
}

// fakeUsers is a fixed set of users.
type fakeUsers map[int64]domain.User

func (f fakeUsers) GetByID(_ context.Context, id int64) (domain.User, error) {
	u, ok := f[id]
	if !ok {
		return domain.User{}, storage.ErrNotFound
	}
	return u, nil
}

// fakeDepts knows which department ids exist.
type fakeDepts map[int64]bool

func (f fakeDepts) GetByID(_ context.Context, id int64) (domain.Department, error) {
	if !f[id] {
		return domain.Department{}, storage.ErrNotFound
	}
	return domain.Department{ID: id}, nil
}

func ptr[T any](v T) *T { return &v }

func day(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

// fixture: departments 1 (dev) and 2 (design); an admin; the dev manager
// with two of their people; a design employee; a blocked dev employee.
type fixture struct {
	repo                         *fakeRepo
	audit                        *testkit.Audit
	notify                       *fakeNotify
	svc                          *Service
	admin, mgr, emp, emp2, out   domain.User
	blocked, mgrDesign, homeless domain.User
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	dev, design := int64(1), int64(2)
	users := fakeUsers{}
	add := func(id int64, role domain.Role, dept *int64, active bool) domain.User {
		u := domain.User{ID: id, FullName: "user" + string(rune('0'+id)), Role: role, DepartmentID: dept, IsActive: active}
		users[id] = u
		return u
	}
	f := fixture{
		admin:     add(1, domain.RoleAdmin, nil, true),
		mgr:       add(2, domain.RoleManager, &dev, true),
		emp:       add(3, domain.RoleEmployee, &dev, true),
		emp2:      add(4, domain.RoleEmployee, &dev, true),
		out:       add(5, domain.RoleEmployee, &design, true),
		blocked:   add(6, domain.RoleEmployee, &dev, false),
		mgrDesign: add(7, domain.RoleManager, &design, true),
		homeless:  add(8, domain.RoleEmployee, nil, true),
	}
	f.repo = newFakeRepo(users)
	f.audit = &testkit.Audit{}
	f.notify = &fakeNotify{users: users}
	f.svc = New(f.repo, users, fakeDepts{dev: true, design: true}, &testkit.Tx{}, f.audit, f.notify, time.UTC)
	return f
}

// create makes a subscription as the admin or fails the test.
func (f fixture) create(t *testing.T, s domain.Subscription) domain.Subscription {
	t.Helper()
	if s.Cost == "" {
		s.Cost = "100"
	}
	if s.Currency == "" {
		s.Currency = "RUB"
	}
	if s.BillingPeriod == "" {
		s.BillingPeriod = domain.BillingMonth
	}
	if s.StartDate.IsZero() {
		s.StartDate = day("2026-01-01")
	}
	out, err := f.svc.Create(context.Background(), f.admin, s)
	if err != nil {
		t.Fatalf("create %+v: %v", s, err)
	}
	return out
}
