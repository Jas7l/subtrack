// Package auth signs users in and tells every other handler who is calling.
//
//   - Login checks an email + password against subtrack.users and issues a
//     JWT (HS256) carrying the user id.
//   - Middleware verifies that token on every protected request, reloads the
//     user, and puts it into the request context; handlers read it back with
//     UserFromContext and gate on roles with RequireRole.
//   - EnsureAdmin creates the first admin from env at startup, because users
//     can only be created by an admin.
//
// Unlike the entity domains, auth is cross-cutting: other domain packages may
// import it for UserFromContext and RequireRole. It never imports them; the
// users table is reached through the repo interface below, which *user.Store
// satisfies structurally.
package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"subtrack/internal/apperr"
	"subtrack/internal/domain"
	"subtrack/internal/logger"
	"subtrack/internal/password"
	"subtrack/internal/storage"
)

// repo is the slice of users persistence auth needs.
type repo interface {
	GetByID(ctx context.Context, id int64) (domain.User, error)
	GetByEmail(ctx context.Context, email string) (domain.User, error)
	HasActiveAdmin(ctx context.Context) (bool, error)
	Create(ctx context.Context, u domain.User) (domain.User, error)
	SetPasswordHash(ctx context.Context, id int64, hash string) error
}

var (
	// ErrInvalidCredentials is the one answer to a failed login, whether the
	// email is unknown or the password is wrong, so the API does not reveal
	// which emails exist. Mapped to 401.
	ErrInvalidCredentials = errors.New("service: invalid email or password")

	// ErrUserBlocked is returned by Login when the password is right but the
	// account is blocked. It is only reported after the password check, so it
	// leaks nothing to someone who does not know the password. Mapped to 403.
	ErrUserBlocked = errors.New("service: user is blocked")

	// ErrUnauthenticated covers every reason a request carries no usable
	// identity: no token, a forged or expired one, or one whose user has since
	// been blocked. Mapped to 401, which tells the client to sign in again.
	ErrUnauthenticated = errors.New("service: authentication required")

	// ErrForbidden is returned when the caller is signed in but their role
	// does not allow the action. Mapped to 403.
	ErrForbidden = errors.New("service: insufficient permissions")

	// ErrWrongPassword is returned by ChangePassword when the current password
	// does not match. Mapped to 400 rather than 401: the caller's session is
	// fine, only the form input is wrong.
	ErrWrongPassword = errors.New("service: current password is incorrect")
)

// dummyHash is compared against when a login names an unknown email, so that
// the request takes as long as one with a wrong password and response timing
// does not reveal which emails exist. Computed once, on first use.
var dummyHash = sync.OnceValue(func() string {
	h, _ := password.Hash("subtrack-timing-equalizer")
	return h
})

// transactor runs fn in one transaction; *pg.TxRunner satisfies it.
type transactor interface {
	InTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// auditor records a change in the audit trail; *audit.Store satisfies it.
type auditor interface {
	Record(ctx context.Context, e domain.AuditEntry) error
}

// Service implements login, token verification, password change and the
// first-admin bootstrap. The two writes (password change, bootstrap) run in
// a transaction together with their audit record.
type Service struct {
	repo   repo
	tokens tokens
	tx     transactor
	audit  auditor
}

// New wires a Service over the given repo with the signing key and token
// lifetime from cfg, the transaction runner and the audit trail. cfg must
// have passed Validate.
func New(r repo, cfg Config, tx transactor, a auditor) *Service {
	return &Service{
		repo:   r,
		tokens: tokens{secret: cfg.Secret, ttl: cfg.TokenTTL, now: time.Now},
		tx:     tx,
		audit:  a,
	}
}

// LoginResult is what a successful Login hands back: the bearer token, the
// moment it stops being valid, and the signed-in user.
type LoginResult struct {
	Token     string
	ExpiresAt time.Time
	User      domain.User
}

// Login checks email + password and issues a token.
//
// Errors:
//   - apperr.ErrInvalidInput when email or password is empty.
//   - ErrInvalidCredentials when the email is unknown or the password wrong.
//   - ErrUserBlocked when the credentials are right but is_active is false.
//   - any other datastore or signing failure, propagated.
func (s *Service) Login(ctx context.Context, email, pw string) (LoginResult, error) {
	log := logger.Op(ctx, "auth.Service.Login")
	email = strings.TrimSpace(email)
	// Never log the password, not even its length.
	log.Debug("start", "email", email)
	if email == "" || pw == "" {
		return LoginResult{}, fmt.Errorf("%w: email and password are required", apperr.ErrInvalidInput)
	}

	u, err := s.repo.GetByEmail(ctx, email)
	if errors.Is(err, storage.ErrNotFound) {
		password.Check(dummyHash(), pw)
		log.Info("login failed", "email", email, "reason", "unknown email")
		return LoginResult{}, ErrInvalidCredentials
	}
	if err != nil {
		log.Error("load user failed", "email", email, "err", err)
		return LoginResult{}, err
	}
	if !password.Check(u.PasswordHash, pw) {
		log.Info("login failed", "user_id", u.ID, "reason", "wrong password")
		return LoginResult{}, ErrInvalidCredentials
	}
	if !u.IsActive {
		log.Info("login refused", "user_id", u.ID, "reason", "user blocked")
		return LoginResult{}, ErrUserBlocked
	}

	token, exp, err := s.tokens.issue(u.ID)
	if err != nil {
		log.Error("issue token failed", "user_id", u.ID, "err", err)
		return LoginResult{}, err
	}
	log.Info("login ok", "user_id", u.ID, "role", u.Role)
	return LoginResult{Token: token, ExpiresAt: exp, User: u}, nil
}

// Authenticate verifies a bearer token and returns the current state of the
// user it was issued for.
//
// Errors:
//   - ErrUnauthenticated when the token is invalid or expired, or its user no
//     longer exists or is blocked.
//   - any other datastore failure, propagated (a 500, not a 401: the token may
//     well be fine).
func (s *Service) Authenticate(ctx context.Context, token string) (domain.User, error) {
	log := logger.Op(ctx, "auth.Service.Authenticate")
	id, err := s.tokens.parse(token)
	if err != nil {
		log.Debug("token rejected", "err", err)
		return domain.User{}, ErrUnauthenticated
	}
	u, err := s.repo.GetByID(ctx, id)
	if errors.Is(err, storage.ErrNotFound) {
		log.Warn("token for missing user", "user_id", id)
		return domain.User{}, ErrUnauthenticated
	}
	if err != nil {
		log.Error("load user failed", "user_id", id, "err", err)
		return domain.User{}, err
	}
	if !u.IsActive {
		log.Debug("token for blocked user", "user_id", id)
		return domain.User{}, ErrUnauthenticated
	}
	return u, nil
}

// ChangePassword replaces the caller's own password after checking the
// current one.
//
// Errors:
//   - apperr.ErrInvalidInput when the new password breaks the length rules or
//     equals the current one.
//   - ErrWrongPassword when current does not match.
//   - any other datastore failure, propagated.
func (s *Service) ChangePassword(ctx context.Context, userID int64, current, next string) error {
	log := logger.Op(ctx, "auth.Service.ChangePassword")
	log.Debug("start", "user_id", userID)

	u, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		log.Error("load user failed", "user_id", userID, "err", err)
		return apperr.MapStorage(err, ErrUnauthenticated)
	}
	if !password.Check(u.PasswordHash, current) {
		log.Info("password change refused", "user_id", userID, "reason", "wrong current password")
		return ErrWrongPassword
	}
	if current == next {
		return fmt.Errorf("%w: new password must differ from the current one", apperr.ErrInvalidInput)
	}
	hash, err := password.Hash(next)
	if err != nil {
		return err
	}
	err = s.tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.repo.SetPasswordHash(ctx, userID, hash); err != nil {
			return err
		}
		return s.audit.Record(ctx, domain.NewAuditEntry(userID, "user.password_change", "user", userID, nil))
	})
	if err != nil {
		log.Error("store password failed", "user_id", userID, "err", err)
		return apperr.MapStorage(err, ErrUnauthenticated)
	}
	log.Info("password changed", "user_id", userID)
	return nil
}

// EnsureAdmin creates the bootstrap admin when the database has no active
// admin yet. It is idempotent: once any active admin exists it does nothing,
// so the BOOTSTRAP_ADMIN_* vars can stay in .env.
//
// With no bootstrap credentials configured and no admin present it only logs
// a warning - the server still starts (e.g. to answer /healthz), but nobody
// can sign in until an admin exists.
//
// Errors:
//   - a wrapped apperr.ErrConflict when the bootstrap email already belongs to
//     a user who is not an active admin; startup should stop rather than
//     silently leave the system without an admin.
//   - password rule violations (apperr.ErrInvalidInput) and datastore
//     failures, propagated.
func (s *Service) EnsureAdmin(ctx context.Context, b BootstrapAdmin) error {
	log := logger.Op(ctx, "auth.Service.EnsureAdmin")
	has, err := s.repo.HasActiveAdmin(ctx)
	if err != nil {
		return fmt.Errorf("auth: check admin: %w", err)
	}
	if has {
		log.Debug("active admin exists, bootstrap skipped")
		return nil
	}
	if b.Email == "" {
		log.Warn("no active admin and BOOTSTRAP_ADMIN_EMAIL is not set: nobody can sign in")
		return nil
	}

	hash, err := password.Hash(b.Password)
	if err != nil {
		return fmt.Errorf("auth: bootstrap admin: %w", err)
	}
	var u domain.User
	err = s.tx.InTx(ctx, func(ctx context.Context) error {
		var err error
		u, err = s.repo.Create(ctx, domain.User{
			FullName:     b.FullName,
			Email:        strings.TrimSpace(b.Email),
			PasswordHash: hash,
			Role:         domain.RoleAdmin,
			IsActive:     true,
		})
		if err != nil {
			return err
		}
		// Nobody is signed in yet: the system is the actor.
		return s.audit.Record(ctx, domain.AuditEntry{Action: "user.create", EntityType: "user", EntityID: &u.ID,
			Details: map[string]any{"email": u.Email, "role": string(u.Role), "bootstrap": true}})
	})
	if errors.Is(err, storage.ErrAlreadyExists) {
		return fmt.Errorf("auth: bootstrap admin: %w: %s belongs to a user who is not an active admin",
			apperr.ErrConflict, b.Email)
	}
	if err != nil {
		return fmt.Errorf("auth: bootstrap admin: %w", err)
	}
	log.Info("bootstrap admin created", "user_id", u.ID, "email", u.Email)
	return nil
}
