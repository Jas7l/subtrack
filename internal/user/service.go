// Package user manages the people who can sign in: the admin creates them,
// edits them, blocks them and resets their passwords; managers see the people
// of their own department; everyone sees themselves.
//
// The *Store here also backs the auth service, which declares its own repo
// interface for it. Like every domain, this package imports auth only to learn
// who is calling and to gate routes by role.
//
// Users are never deleted. Blocking (is_active = false) keeps their history.
package user

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"

	"subtrack/internal/apperr"
	"subtrack/internal/auth"
	"subtrack/internal/domain"
	"subtrack/internal/logger"
	"subtrack/internal/paging"
	"subtrack/internal/password"
	"subtrack/internal/storage"
)

var (
	// ErrUserNotFound is returned when the user does not exist or the caller
	// is not allowed to see them; the two are deliberately indistinguishable.
	// Mapped to 404.
	ErrUserNotFound = errors.New("service: user not found")

	// ErrEmailTaken is returned when another user already has the email
	// (compared case-insensitively). Mapped to 409.
	ErrEmailTaken = errors.New("service: email is already taken")

	// ErrUnknownDepartment is returned when department_id names no
	// department. Mapped to 400: it is a bad value in the request body.
	ErrUnknownDepartment = errors.New("service: invalid input: department does not exist")

	// ErrSelfLockout is returned when an admin tries to block themselves or
	// take away their own admin role. Refusing it also guarantees that the
	// last active admin can never disappear. Mapped to 409.
	ErrSelfLockout = errors.New("service: you cannot block yourself or remove your own admin role")
)

// repo is the slice of users persistence the service needs; *Store satisfies
// it.
type repo interface {
	GetByID(ctx context.Context, id int64) (domain.User, error)
	List(ctx context.Context, f domain.UserFilter, limit, offset int) ([]domain.User, int64, error)
	Create(ctx context.Context, u domain.User) (domain.User, error)
	Update(ctx context.Context, id int64, p domain.UserPatch, actorID int64) (domain.User, error)
	SetPasswordHash(ctx context.Context, id int64, hash string) error
}

// departments is what the service needs to know about departments: whether
// one exists. *department.Store satisfies it structurally.
type departments interface {
	GetByID(ctx context.Context, id int64) (domain.Department, error)
}

// transactor runs fn in one transaction; *pg.TxRunner satisfies it.
type transactor interface {
	InTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// auditor records a change in the audit trail; *audit.Store satisfies it.
// It is called inside the change's transaction.
type auditor interface {
	Record(ctx context.Context, e domain.AuditEntry) error
}

// Service implements the users use cases. Every write runs in a transaction
// together with its audit record.
type Service struct {
	repo  repo
	depts departments
	tx    transactor
	audit auditor
}

// New wires a Service over the users repo, the departments reader, the
// transaction runner and the audit trail.
func New(r repo, d departments, tx transactor, a auditor) *Service {
	return &Service{repo: r, depts: d, tx: tx, audit: a}
}

// CreateUserInput carries the fields of a new user. Password is plaintext;
// the service hashes it.
type CreateUserInput struct {
	FullName     string
	Email        string
	Password     string
	Role         domain.Role
	DepartmentID *int64
}

// canSee reports whether actor may read u: the admin sees everyone, a
// manager sees the people of their department, everyone sees themselves.
func canSee(actor, u domain.User) bool {
	switch {
	case actor.Role == domain.RoleAdmin, actor.ID == u.ID:
		return true
	case actor.Role == domain.RoleManager:
		return actor.DepartmentID != nil && u.DepartmentID != nil && *actor.DepartmentID == *u.DepartmentID
	}
	return false
}

// Get returns one user as actor is allowed to see it.
//
// Errors:
//   - ErrUserNotFound when the user does not exist or is outside actor's view.
//   - any other datastore failure, propagated.
func (s *Service) Get(ctx context.Context, actor domain.User, id int64) (domain.User, error) {
	log := logger.Op(ctx, "user.Service.Get")
	log.Debug("start", "id", id)

	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return domain.User{}, apperr.MapStorage(err, ErrUserNotFound)
	}
	if !canSee(actor, u) {
		log.Debug("hidden from caller", "id", id)
		return domain.User{}, ErrUserNotFound
	}
	return u, nil
}

// List returns one page of users visible to actor. For a manager the
// department filter is forced to their own department (whatever f says); a
// manager without a department sees nobody.
//
// Errors:
//   - auth.ErrForbidden for an employee: they only see themselves, via
//     /auth/me or Get.
//   - any datastore failure, propagated.
func (s *Service) List(ctx context.Context, actor domain.User, f domain.UserFilter, p paging.Params) ([]domain.User, int64, error) {
	log := logger.Op(ctx, "user.Service.List")
	p.Normalize()
	switch actor.Role {
	case domain.RoleAdmin:
	case domain.RoleManager:
		if actor.DepartmentID == nil {
			return []domain.User{}, 0, nil
		}
		f.DepartmentID = actor.DepartmentID
	default:
		return nil, 0, auth.ErrForbidden
	}
	f.Query = strings.TrimSpace(f.Query)

	items, total, err := s.repo.List(ctx, f, p.Limit, p.Offset)
	if err == nil {
		log.Debug("done", "count", len(items), "total", total, "limit", p.Limit, "offset", p.Offset)
	}
	return items, total, err
}

// Create validates the input, hashes the password and inserts an active user.
//
// Errors:
//   - apperr.ErrInvalidInput for a missing name, a malformed email, an unknown
//     role or a password that breaks the length rules.
//   - ErrUnknownDepartment when DepartmentID names no department.
//   - ErrEmailTaken when the email belongs to another user.
//   - any other datastore failure, propagated.
func (s *Service) Create(ctx context.Context, actor domain.User, in CreateUserInput) (domain.User, error) {
	log := logger.Op(ctx, "user.Service.Create")
	in.FullName = strings.TrimSpace(in.FullName)
	in.Email = strings.TrimSpace(in.Email)
	log.Debug("start", "email", in.Email, "role", in.Role)

	var missing []string
	if in.FullName == "" {
		missing = append(missing, "full_name")
	}
	if in.Email == "" {
		missing = append(missing, "email")
	}
	if in.Role == "" {
		missing = append(missing, "role")
	}
	if len(missing) > 0 {
		return domain.User{}, fmt.Errorf("%w: missing required fields: %s", apperr.ErrInvalidInput, strings.Join(missing, ", "))
	}
	if err := validateEmail(in.Email); err != nil {
		return domain.User{}, err
	}
	if err := validateRole(in.Role); err != nil {
		return domain.User{}, err
	}
	if err := s.checkDepartment(ctx, in.DepartmentID); err != nil {
		return domain.User{}, err
	}
	hash, err := password.Hash(in.Password)
	if err != nil {
		return domain.User{}, err
	}

	var u domain.User
	err = s.tx.InTx(ctx, func(ctx context.Context) error {
		var err error
		u, err = s.repo.Create(ctx, domain.User{
			FullName:     in.FullName,
			Email:        in.Email,
			PasswordHash: hash,
			Role:         in.Role,
			DepartmentID: in.DepartmentID,
			IsActive:     true,
		})
		if err != nil {
			return err
		}
		return s.audit.Record(ctx, domain.NewAuditEntry(actor.ID, "user.create", "user", u.ID, map[string]any{
			"full_name": u.FullName, "email": u.Email, "role": string(u.Role), "department_id": u.DepartmentID,
		}))
	})
	if err != nil {
		return domain.User{}, mapWriteErr(err)
	}
	log.Info("user created", "id", u.ID, "role", u.Role)
	return u, nil
}

// Update applies a partial patch on behalf of actor (an admin).
//
// Side effects, done by the store in the same transaction: if the user headed
// a department and is no longer an active manager in it (blocked, demoted or
// moved), that department is left without a head; if the user is blocked,
// all their licences are revoked by actor.
//
// Errors:
//   - apperr.ErrInvalidInput for an empty patch or an invalid value.
//   - ErrSelfLockout when actor would block themselves or drop their own
//     admin role.
//   - ErrUserNotFound, ErrUnknownDepartment, ErrEmailTaken as their names say.
//   - any other datastore failure, propagated.
func (s *Service) Update(ctx context.Context, actor domain.User, id int64, p domain.UserPatch) (domain.User, error) {
	log := logger.Op(ctx, "user.Service.Update")
	log.Debug("start", "id", id, "fields", p.Fields())

	if !p.HasAny() {
		return domain.User{}, fmt.Errorf("%w: at least one field must be provided", apperr.ErrInvalidInput)
	}
	if p.FullName != nil {
		name := strings.TrimSpace(*p.FullName)
		if name == "" {
			return domain.User{}, fmt.Errorf("%w: full_name must not be empty", apperr.ErrInvalidInput)
		}
		p.FullName = &name
	}
	if p.Email != nil {
		email := strings.TrimSpace(*p.Email)
		if err := validateEmail(email); err != nil {
			return domain.User{}, err
		}
		p.Email = &email
	}
	if p.Role != nil {
		if err := validateRole(*p.Role); err != nil {
			return domain.User{}, err
		}
	}
	if actor.ID == id && ((p.IsActive != nil && !*p.IsActive) || (p.Role != nil && *p.Role != domain.RoleAdmin)) {
		return domain.User{}, ErrSelfLockout
	}
	if p.DepartmentID.Set {
		if err := s.checkDepartment(ctx, p.DepartmentID.Value); err != nil {
			return domain.User{}, err
		}
	}

	var u domain.User
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		var err error
		u, err = s.repo.Update(ctx, id, p, actor.ID)
		if err != nil {
			return err
		}
		return s.audit.Record(ctx, domain.NewAuditEntry(actor.ID, "user.update", "user", id, patchDetails(p)))
	})
	if err != nil {
		return domain.User{}, mapWriteErr(err)
	}
	log.Info("user updated", "id", u.ID, "fields", p.Fields())
	return u, nil
}

// SetPassword replaces a user's password (an admin reset; the user changes
// their own through /auth/password). Issued tokens stay valid.
//
// Errors:
//   - apperr.ErrInvalidInput when the password breaks the length rules.
//   - ErrUserNotFound when no user has that id.
//   - any other datastore failure, propagated.
func (s *Service) SetPassword(ctx context.Context, actor domain.User, id int64, pw string) error {
	log := logger.Op(ctx, "user.Service.SetPassword")
	hash, err := password.Hash(pw)
	if err != nil {
		return err
	}
	err = s.tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.repo.SetPasswordHash(ctx, id, hash); err != nil {
			return err
		}
		return s.audit.Record(ctx, domain.NewAuditEntry(actor.ID, "user.password_reset", "user", id, nil))
	})
	if err != nil {
		return apperr.MapStorage(err, ErrUserNotFound)
	}
	log.Info("password reset", "id", id)
	return nil
}

// checkDepartment returns ErrUnknownDepartment when id is set and names no
// department. Checking up front, rather than relying on the foreign key, keeps
// "no such user" and "no such department" apart.
func (s *Service) checkDepartment(ctx context.Context, id *int64) error {
	if id == nil {
		return nil
	}
	_, err := s.depts.GetByID(ctx, *id)
	if errors.Is(err, storage.ErrNotFound) {
		return ErrUnknownDepartment
	}
	return err
}

// mapWriteErr translates the storage errors of Create/Update.
func mapWriteErr(err error) error {
	var ce *storage.ConstraintError
	if errors.As(err, &ce) && ce.Constraint == "users_email_key" {
		return ErrEmailTaken
	}
	return apperr.MapStorage(err, ErrUserNotFound)
}

// validateEmail accepts a bare address (no display name, no spaces).
func validateEmail(email string) error {
	a, err := mail.ParseAddress(email)
	if err != nil || a.Address != email {
		return fmt.Errorf("%w: email %q is not a valid address", apperr.ErrInvalidInput, email)
	}
	return nil
}

func validateRole(r domain.Role) error {
	if !r.Valid() {
		return fmt.Errorf("%w: role must be one of admin, manager, employee", apperr.ErrInvalidInput)
	}
	return nil
}

// patchDetails lists the fields an update sets, with their new values, for
// the audit trail.
func patchDetails(p domain.UserPatch) map[string]any {
	d := map[string]any{}
	if p.FullName != nil {
		d["full_name"] = *p.FullName
	}
	if p.Email != nil {
		d["email"] = *p.Email
	}
	if p.Role != nil {
		d["role"] = string(*p.Role)
	}
	if p.DepartmentID.Set {
		d["department_id"] = p.DepartmentID.Value
	}
	if p.IsActive != nil {
		d["is_active"] = *p.IsActive
	}
	return d
}
