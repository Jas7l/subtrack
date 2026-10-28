package subscription

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"subtrack/internal/domain"
	"subtrack/internal/storage"
	"subtrack/internal/testkit"
)

// fakeRepo is an in-memory subscriptions + assignments store with the rules
// the service relies on: soft delete, the seats CHECK, one active licence per
// (subscription, user), revoke-all on cancel and delete, and the visibility
// scope. Locking is not modelled.
type fakeRepo struct {
	mu         sync.Mutex
	subs       map[int64]domain.Subscription
	deleted    map[int64]bool
	asg        map[int64]domain.Assignment
	currencies map[string]bool
	users      fakeUsers
	nextSub    int64
	nextAsg    int64
}

func newFakeRepo(users fakeUsers) *fakeRepo {
	return &fakeRepo{
		subs:       map[int64]domain.Subscription{},
		deleted:    map[int64]bool{},
		asg:        map[int64]domain.Assignment{},
		currencies: map[string]bool{"RUB": true, "USD": true},
		users:      users,
		nextSub:    1,
		nextAsg:    1,
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
	a.SubscriptionDepartmentID = f.subs[a.SubscriptionID].DepartmentID
	return a, nil
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
		a.SubscriptionDepartmentID = f.subs[a.SubscriptionID].DepartmentID
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
	return a, nil
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
	f.svc = New(f.repo, users, fakeDepts{dev: true, design: true}, &testkit.Tx{}, f.audit, time.UTC)
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
