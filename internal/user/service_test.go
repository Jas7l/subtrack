package user

import (
	"context"
	"errors"
	"testing"

	"subtrack/internal/apperr"
	"subtrack/internal/auth"
	"subtrack/internal/domain"
	"subtrack/internal/paging"
	"subtrack/internal/password"
)

// fixture: departments 1 and 2; an admin, a manager and two employees in
// department 1, an employee in department 2, and a manager with none.
type fixture struct {
	repo                       *fakeRepo
	svc                        *Service
	admin, mgr, emp, emp2, out domain.User
	lone                       domain.User
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	repo := newFakeRepo()
	f := fixture{repo: repo, svc: New(repo, fakeDepts{1: true, 2: true})}
	f.admin = repo.add(t, "admin@corp.ru", domain.RoleAdmin, nil)
	f.mgr = repo.add(t, "mgr@corp.ru", domain.RoleManager, ptr(int64(1)))
	f.emp = repo.add(t, "emp@corp.ru", domain.RoleEmployee, ptr(int64(1)))
	f.emp2 = repo.add(t, "emp2@corp.ru", domain.RoleEmployee, ptr(int64(1)))
	f.out = repo.add(t, "out@corp.ru", domain.RoleEmployee, ptr(int64(2)))
	f.lone = repo.add(t, "lone@corp.ru", domain.RoleManager, nil)
	return f
}

func TestGetVisibility(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	tests := []struct {
		name   string
		actor  domain.User
		target domain.User
		ok     bool
	}{
		{"admin sees anyone", f.admin, f.out, true},
		{"manager sees own department", f.mgr, f.emp, true},
		{"manager does not see other department", f.mgr, f.out, false},
		{"manager without department sees only self", f.lone, f.emp, false},
		{"manager without department sees self", f.lone, f.lone, true},
		{"employee sees self", f.emp, f.emp, true},
		{"employee does not see colleague", f.emp, f.emp2, false},
		{"employee does not see manager", f.emp, f.mgr, false},
	}
	for _, tt := range tests {
		_, err := f.svc.Get(ctx, tt.actor, tt.target.ID)
		if tt.ok && err != nil {
			t.Errorf("%s: err = %v", tt.name, err)
		}
		if !tt.ok && !errors.Is(err, ErrUserNotFound) {
			t.Errorf("%s: err = %v, want ErrUserNotFound", tt.name, err)
		}
	}
	if _, err := f.svc.Get(ctx, f.admin, 999); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("missing user: err = %v", err)
	}
}

func TestListScope(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	items, total, err := f.svc.List(ctx, f.admin, domain.UserFilter{}, paging.Params{})
	if err != nil || total != 6 || len(items) != 6 {
		t.Errorf("admin: %d items, total %d, err %v; want 6", len(items), total, err)
	}

	// The manager asks for department 2 and still gets only department 1.
	items, total, err = f.svc.List(ctx, f.mgr, domain.UserFilter{DepartmentID: ptr(int64(2))}, paging.Params{})
	if err != nil || total != 3 {
		t.Fatalf("manager: total %d, err %v; want 3", total, err)
	}
	for _, u := range items {
		if *u.DepartmentID != 1 {
			t.Errorf("manager sees user %d of department %d", u.ID, *u.DepartmentID)
		}
	}

	if _, total, err := f.svc.List(ctx, f.lone, domain.UserFilter{}, paging.Params{}); err != nil || total != 0 {
		t.Errorf("manager without department: total %d, err %v; want 0", total, err)
	}
	if _, _, err := f.svc.List(ctx, f.emp, domain.UserFilter{}, paging.Params{}); !errors.Is(err, auth.ErrForbidden) {
		t.Errorf("employee: err = %v, want ErrForbidden", err)
	}

	items, _, _ = f.svc.List(ctx, f.admin, domain.UserFilter{Role: ptr(domain.RoleManager), Query: " MGR "}, paging.Params{})
	if len(items) != 1 || items[0].ID != f.mgr.ID {
		t.Errorf("role+query filter: got %v", items)
	}
}

func TestCreate(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	valid := CreateUserInput{
		FullName: "  Новый Сотрудник ", Email: " new@corp.ru ", Password: "initial-pass",
		Role: domain.RoleEmployee, DepartmentID: ptr(int64(2)),
	}

	u, err := f.svc.Create(ctx, valid)
	if err != nil {
		t.Fatal(err)
	}
	if u.FullName != "Новый Сотрудник" || u.Email != "new@corp.ru" || !u.IsActive {
		t.Errorf("created %+v: want trimmed fields and active", u)
	}
	if !password.Check(u.PasswordHash, "initial-pass") {
		t.Error("password not hashed from the input")
	}

	tests := []struct {
		name   string
		mutate func(*CreateUserInput)
		want   error
	}{
		{"duplicate email, other case", func(in *CreateUserInput) { in.Email = "NEW@corp.ru" }, ErrEmailTaken},
		{"no name", func(in *CreateUserInput) { in.FullName = " " }, apperr.ErrInvalidInput},
		{"bad email", func(in *CreateUserInput) { in.Email = "not-an-email" }, apperr.ErrInvalidInput},
		{"email with display name", func(in *CreateUserInput) { in.Email = "Bob <bob@corp.ru>" }, apperr.ErrInvalidInput},
		{"bad role", func(in *CreateUserInput) { in.Role = "boss" }, apperr.ErrInvalidInput},
		{"no role", func(in *CreateUserInput) { in.Role = "" }, apperr.ErrInvalidInput},
		{"short password", func(in *CreateUserInput) { in.Password = "123" }, apperr.ErrInvalidInput},
		{"unknown department", func(in *CreateUserInput) { in.DepartmentID = ptr(int64(42)) }, ErrUnknownDepartment},
	}
	for _, tt := range tests {
		in := valid
		in.Email = "other@corp.ru"
		tt.mutate(&in)
		if _, err := f.svc.Create(ctx, in); !errors.Is(err, tt.want) {
			t.Errorf("%s: err = %v, want %v", tt.name, err, tt.want)
		}
	}
}

func TestUpdate(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	no := false

	tests := []struct {
		name  string
		actor domain.User
		id    int64
		patch domain.UserPatch
		want  error
	}{
		{"empty patch", f.admin, f.emp.ID, domain.UserPatch{}, apperr.ErrInvalidInput},
		{"admin blocks self", f.admin, f.admin.ID, domain.UserPatch{IsActive: &no}, ErrSelfLockout},
		{"admin demotes self", f.admin, f.admin.ID, domain.UserPatch{Role: ptr(domain.RoleManager)}, ErrSelfLockout},
		{"blank name", f.admin, f.emp.ID, domain.UserPatch{FullName: ptr("  ")}, apperr.ErrInvalidInput},
		{"bad role", f.admin, f.emp.ID, domain.UserPatch{Role: ptr(domain.Role("boss"))}, apperr.ErrInvalidInput},
		{"email taken", f.admin, f.emp.ID, domain.UserPatch{Email: ptr("MGR@corp.ru")}, ErrEmailTaken},
		{"unknown department", f.admin, f.emp.ID,
			domain.UserPatch{DepartmentID: domain.IDPatch{Set: true, Value: ptr(int64(42))}}, ErrUnknownDepartment},
		{"missing user", f.admin, 999, domain.UserPatch{FullName: ptr("X")}, ErrUserNotFound},
	}
	for _, tt := range tests {
		if _, err := f.svc.Update(ctx, tt.actor, tt.id, tt.patch); !errors.Is(err, tt.want) {
			t.Errorf("%s: err = %v, want %v", tt.name, err, tt.want)
		}
	}

	// Renaming oneself is fine; so is keeping one's own admin role explicitly.
	if _, err := f.svc.Update(ctx, f.admin, f.admin.ID, domain.UserPatch{
		FullName: ptr("Root"), Role: ptr(domain.RoleAdmin),
	}); err != nil {
		t.Errorf("admin edits own name: %v", err)
	}

	u, err := f.svc.Update(ctx, f.admin, f.emp.ID, domain.UserPatch{
		IsActive:     &no,
		DepartmentID: domain.IDPatch{Set: true},
		Email:        ptr(" emp.new@corp.ru "),
	})
	if err != nil {
		t.Fatal(err)
	}
	if u.IsActive || u.DepartmentID != nil || u.Email != "emp.new@corp.ru" {
		t.Errorf("after update: %+v", u)
	}
}

func TestSetPassword(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if err := f.svc.SetPassword(ctx, f.emp.ID, "short"); !errors.Is(err, apperr.ErrInvalidInput) {
		t.Errorf("short: err = %v", err)
	}
	if err := f.svc.SetPassword(ctx, 999, "long-enough"); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("missing user: err = %v", err)
	}
	if err := f.svc.SetPassword(ctx, f.emp.ID, "long-enough"); err != nil {
		t.Fatal(err)
	}
	if !password.Check(f.repo.users[f.emp.ID].PasswordHash, "long-enough") {
		t.Error("password not replaced")
	}
}
