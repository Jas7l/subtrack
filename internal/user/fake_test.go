package user

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"subtrack/internal/domain"
	"subtrack/internal/storage"
)

// fakeRepo is an in-memory users table with the rules the service relies on:
// unique emails (case-insensitive) and storage.ErrNotFound for a missing row.
// The side effects of Update (department head, licences, requests) live in
// SQL and are not modelled; Update reports undone as they were.
type fakeRepo struct {
	mu     sync.Mutex
	users  map[int64]domain.User
	nextID int64
	// undone is what every Update reports it took back.
	undone Undone
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{users: map[int64]domain.User{}, nextID: 1}
}

func (f *fakeRepo) GetByID(_ context.Context, id int64) (domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.users[id]
	if !ok {
		return domain.User{}, storage.ErrNotFound
	}
	return u, nil
}

func (f *fakeRepo) List(_ context.Context, flt domain.UserFilter, limit, offset int) ([]domain.User, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.User
	for id := int64(1); id < f.nextID; id++ {
		u, ok := f.users[id]
		switch {
		case !ok:
		case flt.DepartmentID != nil && (u.DepartmentID == nil || *u.DepartmentID != *flt.DepartmentID):
		case flt.Role != nil && u.Role != *flt.Role:
		case flt.Active != nil && u.IsActive != *flt.Active:
		case flt.Query != "" && !strings.Contains(strings.ToLower(u.FullName+" "+u.Email), strings.ToLower(flt.Query)):
		default:
			out = append(out, u)
		}
	}
	total := int64(len(out))
	out = out[min(offset, len(out)):]
	return out[:min(limit, len(out))], total, nil
}

func (f *fakeRepo) emailTaken(email string, except int64) bool {
	for _, u := range f.users {
		if u.ID != except && strings.EqualFold(u.Email, email) {
			return true
		}
	}
	return false
}

func (f *fakeRepo) Create(_ context.Context, u domain.User) (domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.emailTaken(u.Email, 0) {
		return domain.User{}, &storage.ConstraintError{Constraint: "users_email_key"}
	}
	u.ID = f.nextID
	f.nextID++
	u.CreatedAt, u.UpdatedAt = time.Now(), time.Now()
	f.users[u.ID] = u
	return u, nil
}

func (f *fakeRepo) Update(_ context.Context, id int64, p domain.UserPatch, _ int64) (domain.User, Undone, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.users[id]
	if !ok {
		return domain.User{}, Undone{}, storage.ErrNotFound
	}
	if p.Email != nil && f.emailTaken(*p.Email, id) {
		return domain.User{}, Undone{}, &storage.ConstraintError{Constraint: "users_email_key"}
	}
	if p.FullName != nil {
		u.FullName = *p.FullName
	}
	if p.Email != nil {
		u.Email = *p.Email
	}
	if p.Role != nil {
		u.Role = *p.Role
	}
	if p.DepartmentID.Set {
		u.DepartmentID = p.DepartmentID.Value
	}
	if p.IsActive != nil {
		u.IsActive = *p.IsActive
	}
	f.users[id] = u
	return u, f.undone, nil
}

func (f *fakeRepo) SetPasswordHash(_ context.Context, id int64, hash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.users[id]
	if !ok {
		return storage.ErrNotFound
	}
	u.PasswordHash = hash
	f.users[id] = u
	return nil
}

// add stores a user directly, bypassing validation.
func (f *fakeRepo) add(t *testing.T, email string, role domain.Role, dept *int64) domain.User {
	t.Helper()
	u, err := f.Create(context.Background(), domain.User{
		FullName: email, Email: email, PasswordHash: "x", Role: role, DepartmentID: dept, IsActive: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return u
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
