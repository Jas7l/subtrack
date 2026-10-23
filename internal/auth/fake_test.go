package auth

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"subtrack/internal/domain"
	"subtrack/internal/password"
	"subtrack/internal/storage"

	"subtrack/internal/testkit"
)

// fakeRepo is an in-memory users table. It mirrors the two rules of the real
// one that auth depends on: emails are unique case-insensitively, and a
// missing row is storage.ErrNotFound.
type fakeRepo struct {
	mu     sync.Mutex
	users  map[int64]domain.User
	nextID int64
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

func (f *fakeRepo) GetByEmail(_ context.Context, email string) (domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, u := range f.users {
		if strings.EqualFold(u.Email, email) {
			return u, nil
		}
	}
	return domain.User{}, storage.ErrNotFound
}

func (f *fakeRepo) HasActiveAdmin(context.Context) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, u := range f.users {
		if u.Role == domain.RoleAdmin && u.IsActive {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeRepo) Create(_ context.Context, u domain.User) (domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, existing := range f.users {
		if strings.EqualFold(existing.Email, u.Email) {
			return domain.User{}, &storage.ConstraintError{Constraint: "users_email_key"}
		}
	}
	u.ID = f.nextID
	f.nextID++
	u.CreatedAt, u.UpdatedAt = time.Now(), time.Now()
	f.users[u.ID] = u
	return u, nil
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

// add stores a user with the given plaintext password and returns it.
func (f *fakeRepo) add(t *testing.T, email, pw string, role domain.Role, active bool) domain.User {
	t.Helper()
	h, err := password.Hash(pw)
	if err != nil {
		t.Fatal(err)
	}
	u, err := f.Create(context.Background(), domain.User{
		FullName: email, Email: email, PasswordHash: h, Role: role, IsActive: active,
	})
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func (f *fakeRepo) setActive(id int64, active bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u := f.users[id]
	u.IsActive = active
	f.users[id] = u
}

func newTestService(r repo) *Service {
	return New(r, Config{Secret: testSecret, TokenTTL: time.Hour}, &testkit.Tx{}, &testkit.Audit{})
}
