// Package user owns persistence of subtrack.users. The *Store here satisfies,
// structurally, the small repo interfaces declared by its consumers (the auth
// service today); the package does not import them.
package user

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"subtrack/internal/domain"
	"subtrack/internal/pg"
	"subtrack/internal/storage"
)

// userColumns is the column list every SELECT / RETURNING here reads, in the
// order scanUser expects.
const userColumns = `id, full_name, email, password_hash, role, department_id, is_active, created_at, updated_at`

// Store reads and writes subtrack.users. The pool is borrowed, not owned.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore constructs a Store over the given pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// scanUser reads one row selected with userColumns.
func scanUser(row pgx.Row) (domain.User, error) {
	var u domain.User
	var role string
	err := row.Scan(&u.ID, &u.FullName, &u.Email, &u.PasswordHash, &role,
		&u.DepartmentID, &u.IsActive, &u.CreatedAt, &u.UpdatedAt)
	u.Role = domain.Role(role)
	return u, err
}

// GetByID returns the user with the given id, active or not.
//
// Errors:
//   - storage.ErrNotFound when no row has that id.
//   - any other db error wrapped as "postgres: get user by id: ...".
func (s *Store) GetByID(ctx context.Context, id int64) (domain.User, error) {
	u, err := scanUser(s.pool.QueryRow(ctx,
		`SELECT `+userColumns+` FROM subtrack.users WHERE id = $1`, id))
	if err != nil {
		return domain.User{}, pg.Translate(ctx, "get user by id", err)
	}
	return u, nil
}

// GetByEmail returns the user whose email matches case-insensitively - the
// same comparison the users_email_key unique index enforces.
//
// Errors:
//   - storage.ErrNotFound when no row has that email.
//   - any other db error wrapped as "postgres: get user by email: ...".
func (s *Store) GetByEmail(ctx context.Context, email string) (domain.User, error) {
	u, err := scanUser(s.pool.QueryRow(ctx,
		`SELECT `+userColumns+` FROM subtrack.users WHERE lower(email) = lower($1)`, email))
	if err != nil {
		return domain.User{}, pg.Translate(ctx, "get user by email", err)
	}
	return u, nil
}

// HasActiveAdmin reports whether at least one unblocked admin exists. Startup
// uses it to decide whether the bootstrap admin must be created.
func (s *Store) HasActiveAdmin(ctx context.Context) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM subtrack.users WHERE role = 'admin' AND is_active)`).Scan(&ok)
	if err != nil {
		return false, pg.Translate(ctx, "has active admin", err)
	}
	return ok, nil
}

// Create inserts a user and returns the stored row (id and timestamps filled
// in). ID, CreatedAt and UpdatedAt on the input are ignored.
//
// Errors:
//   - *storage.ConstraintError (matches storage.ErrAlreadyExists) with
//     Constraint "users_email_key" when the email is taken.
//   - wrapped storage.ErrNotFound when DepartmentID names no department.
//   - any other db error wrapped as "postgres: create user: ...".
func (s *Store) Create(ctx context.Context, u domain.User) (domain.User, error) {
	out, err := scanUser(s.pool.QueryRow(ctx,
		`INSERT INTO subtrack.users (full_name, email, password_hash, role, department_id, is_active)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING `+userColumns,
		u.FullName, u.Email, u.PasswordHash, string(u.Role), u.DepartmentID, u.IsActive))
	if err != nil {
		return domain.User{}, pg.Translate(ctx, "create user", err)
	}
	return out, nil
}

// SetPasswordHash replaces a user's password hash.
//
// Errors:
//   - storage.ErrNotFound when no row has that id.
//   - any other db error wrapped as "postgres: set password hash: ...".
func (s *Store) SetPasswordHash(ctx context.Context, id int64, hash string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE subtrack.users SET password_hash = $2, updated_at = NOW() WHERE id = $1`, id, hash)
	if err != nil {
		return pg.Translate(ctx, "set password hash", err)
	}
	if tag.RowsAffected() == 0 {
		return storage.ErrNotFound
	}
	return nil
}
