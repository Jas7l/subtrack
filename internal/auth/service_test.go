package auth

import (
	"context"
	"errors"
	"testing"

	"subtrack/internal/apperr"
	"subtrack/internal/domain"
)

func TestLogin(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo()
	alice := repo.add(t, "alice@corp.ru", "alice-password", domain.RoleEmployee, true)
	repo.add(t, "bob@corp.ru", "bob-password", domain.RoleEmployee, false)
	svc := newTestService(repo)

	res, err := svc.Login(ctx, "  ALICE@corp.ru ", "alice-password")
	if err != nil {
		t.Fatalf("valid login: %v", err)
	}
	if res.User.ID != alice.ID || res.Token == "" {
		t.Errorf("got user %d token %q, want user %d and a token", res.User.ID, res.Token, alice.ID)
	}
	if id, err := svc.tokens.parse(res.Token); err != nil || id != alice.ID {
		t.Errorf("issued token parses to (%d, %v), want (%d, nil)", id, err, alice.ID)
	}

	tests := []struct {
		name, email, pw string
		want            error
	}{
		{"wrong password", "alice@corp.ru", "not-her-password", ErrInvalidCredentials},
		{"unknown email", "nobody@corp.ru", "whatever-pass", ErrInvalidCredentials},
		{"blocked user", "bob@corp.ru", "bob-password", ErrUserBlocked},
		{"blocked user, wrong password", "bob@corp.ru", "guess-guess", ErrInvalidCredentials},
		{"empty email", "", "alice-password", apperr.ErrInvalidInput},
		{"empty password", "alice@corp.ru", "", apperr.ErrInvalidInput},
	}
	for _, tt := range tests {
		if _, err := svc.Login(ctx, tt.email, tt.pw); !errors.Is(err, tt.want) {
			t.Errorf("%s: err = %v, want %v", tt.name, err, tt.want)
		}
	}
}

func TestAuthenticate(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo()
	alice := repo.add(t, "alice@corp.ru", "alice-password", domain.RoleManager, true)
	svc := newTestService(repo)

	res, err := svc.Login(ctx, "alice@corp.ru", "alice-password")
	if err != nil {
		t.Fatal(err)
	}
	u, err := svc.Authenticate(ctx, res.Token)
	if err != nil || u.ID != alice.ID || u.Role != domain.RoleManager {
		t.Fatalf("Authenticate = (%+v, %v), want alice as manager", u, err)
	}

	// Blocking takes effect on the next request, without waiting for expiry.
	repo.setActive(alice.ID, false)
	if _, err := svc.Authenticate(ctx, res.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("blocked user: err = %v, want ErrUnauthenticated", err)
	}

	ghost, _, err := svc.tokens.issue(999)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Authenticate(ctx, ghost); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("missing user: err = %v, want ErrUnauthenticated", err)
	}
	if _, err := svc.Authenticate(ctx, "garbage"); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("garbage token: err = %v, want ErrUnauthenticated", err)
	}
}

func TestChangePassword(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo()
	alice := repo.add(t, "alice@corp.ru", "old-password", domain.RoleEmployee, true)
	svc := newTestService(repo)

	if err := svc.ChangePassword(ctx, alice.ID, "not-the-old-one", "new-password"); !errors.Is(err, ErrWrongPassword) {
		t.Errorf("wrong current: err = %v, want ErrWrongPassword", err)
	}
	if err := svc.ChangePassword(ctx, alice.ID, "old-password", "short"); !errors.Is(err, apperr.ErrInvalidInput) {
		t.Errorf("short new: err = %v, want ErrInvalidInput", err)
	}
	if err := svc.ChangePassword(ctx, alice.ID, "old-password", "old-password"); !errors.Is(err, apperr.ErrInvalidInput) {
		t.Errorf("same as old: err = %v, want ErrInvalidInput", err)
	}
	if err := svc.ChangePassword(ctx, alice.ID, "old-password", "new-password"); err != nil {
		t.Fatalf("valid change: %v", err)
	}
	if _, err := svc.Login(ctx, "alice@corp.ru", "old-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("old password still works: err = %v", err)
	}
	if _, err := svc.Login(ctx, "alice@corp.ru", "new-password"); err != nil {
		t.Errorf("new password rejected: %v", err)
	}
}

func TestEnsureAdmin(t *testing.T) {
	ctx := context.Background()
	boot := BootstrapAdmin{Email: "admin@corp.ru", Password: "admin-password", FullName: "Admin"}

	t.Run("creates the first admin once", func(t *testing.T) {
		repo := newFakeRepo()
		svc := newTestService(repo)
		for range 2 {
			if err := svc.EnsureAdmin(ctx, boot); err != nil {
				t.Fatal(err)
			}
		}
		if n := len(repo.users); n != 1 {
			t.Fatalf("%d users after two runs, want 1", n)
		}
		res, err := svc.Login(ctx, boot.Email, boot.Password)
		if err != nil || res.User.Role != domain.RoleAdmin {
			t.Errorf("bootstrap admin login = (%v, %v), want an admin", res.User.Role, err)
		}
	})

	t.Run("no credentials, no admin: starts anyway", func(t *testing.T) {
		repo := newFakeRepo()
		if err := newTestService(repo).EnsureAdmin(ctx, BootstrapAdmin{}); err != nil {
			t.Fatal(err)
		}
		if len(repo.users) != 0 {
			t.Error("created a user without credentials")
		}
	})

	t.Run("existing admin: does nothing", func(t *testing.T) {
		repo := newFakeRepo()
		repo.add(t, "root@corp.ru", "root-password", domain.RoleAdmin, true)
		if err := newTestService(repo).EnsureAdmin(ctx, boot); err != nil {
			t.Fatal(err)
		}
		if len(repo.users) != 1 {
			t.Error("created a second admin")
		}
	})

	t.Run("email taken by a non-admin: refuses", func(t *testing.T) {
		repo := newFakeRepo()
		repo.add(t, "ADMIN@corp.ru", "employee-password", domain.RoleEmployee, true)
		if err := newTestService(repo).EnsureAdmin(ctx, boot); !errors.Is(err, apperr.ErrConflict) {
			t.Errorf("err = %v, want ErrConflict", err)
		}
	})

	t.Run("weak bootstrap password: refuses", func(t *testing.T) {
		weak := boot
		weak.Password = "123"
		if err := newTestService(newFakeRepo()).EnsureAdmin(ctx, weak); !errors.Is(err, apperr.ErrInvalidInput) {
			t.Errorf("err = %v, want ErrInvalidInput", err)
		}
	})
}

func TestConfigValidate(t *testing.T) {
	ok := Config{Secret: testSecret, TokenTTL: 1}
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
	}{
		{"valid", func(*Config) {}, false},
		{"no secret", func(c *Config) { c.Secret = nil }, true},
		{"short secret", func(c *Config) { c.Secret = []byte("short") }, true},
		{"zero ttl", func(c *Config) { c.TokenTTL = 0 }, true},
		{"email without password", func(c *Config) { c.Bootstrap.Email = "a@b.c" }, true},
		{"both bootstrap vars", func(c *Config) { c.Bootstrap = BootstrapAdmin{Email: "a@b.c", Password: "p"} }, false},
	}
	for _, tt := range tests {
		c := ok
		tt.mutate(&c)
		if err := c.Validate(); (err != nil) != tt.wantErr {
			t.Errorf("%s: err = %v, wantErr %v", tt.name, err, tt.wantErr)
		}
	}
}
