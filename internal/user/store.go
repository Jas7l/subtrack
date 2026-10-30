package user

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"subtrack/internal/domain"
	"subtrack/internal/pg"
	"subtrack/internal/storage"
)

// userColumns is the column list every SELECT / RETURNING here reads, in the
// order scanUser expects.
const userColumns = `id, full_name, email, password_hash, role, department_id, is_active, created_at, updated_at`

// errLastAdmin is returned by Update when the change would leave no active
// admin.
var errLastAdmin = errors.New("user store: no active admin would be left")

// adminsLock is the advisory lock that changes which may take away an admin
// (blocking, a new role) wait for each other on.
const adminsLock = 7_301_001

// Store reads and writes subtrack.users. The pool is borrowed, not owned.
// Every method runs on pg.Conn(ctx, pool), so it joins a transaction started
// by pg.TxRunner (its own transactions become savepoints).
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
	u, err := scanUser(pg.Conn(ctx, s.pool).QueryRow(ctx,
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
	u, err := scanUser(pg.Conn(ctx, s.pool).QueryRow(ctx,
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
	err := pg.Conn(ctx, s.pool).QueryRow(ctx,
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
	out, err := scanUser(pg.Conn(ctx, s.pool).QueryRow(ctx,
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
	tag, err := pg.Conn(ctx, s.pool).Exec(ctx,
		`UPDATE subtrack.users SET password_hash = $2, updated_at = NOW() WHERE id = $1`, id, hash)
	if err != nil {
		return pg.Translate(ctx, "set password hash", err)
	}
	if tag.RowsAffected() == 0 {
		return storage.ErrNotFound
	}
	return nil
}

// List returns one page of users matching f, ordered by full name, plus the
// total number of matches (ignoring limit/offset).
//
// Errors:
//   - any db error wrapped as "postgres: count users: ..." / "list users ...".
func (s *Store) List(ctx context.Context, f domain.UserFilter, limit, offset int) ([]domain.User, int64, error) {
	conds := make([]string, 0, 4)
	args := make([]any, 0, 6)
	add := func(cond string, v any) {
		args = append(args, v)
		conds = append(conds, strings.ReplaceAll(cond, "?", "$"+pg.Itoa(len(args))))
	}
	if f.DepartmentID != nil {
		add("department_id = ?", *f.DepartmentID)
	}
	if f.Role != nil {
		add("role = ?", string(*f.Role))
	}
	if f.Active != nil {
		add("is_active = ?", *f.Active)
	}
	if f.Query != "" {
		add("(full_name ILIKE ? OR email ILIKE ?)", "%"+pg.EscapeLike(f.Query)+"%")
	}
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}

	var total int64
	if err := pg.Conn(ctx, s.pool).QueryRow(ctx, `SELECT COUNT(*) FROM subtrack.users`+where, args...).Scan(&total); err != nil {
		return nil, 0, pg.Translate(ctx, "count users", err)
	}

	args = append(args, limit, offset)
	q := `SELECT ` + userColumns + ` FROM subtrack.users` + where +
		` ORDER BY full_name, id LIMIT $` + pg.Itoa(len(args)-1) + ` OFFSET $` + pg.Itoa(len(args))
	rows, err := pg.Conn(ctx, s.pool).Query(ctx, q, args...)
	if err != nil {
		return nil, 0, pg.Translate(ctx, "list users", err)
	}
	defer rows.Close()

	out := make([]domain.User, 0, limit)
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, 0, pg.Translate(ctx, "list users scan", err)
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, pg.Translate(ctx, "list users rows", err)
	}
	return out, total, nil
}

// Undone counts what blocking a user took back in the same transaction; it
// is zero for any other edit.
type Undone struct {
	// LicencesRevoked is how many active licences were revoked.
	LicencesRevoked int64
	// RequestsCancelled is how many pending requests were cancelled.
	RequestsCancelled int64
	// ReleasesCancelled is how many pending requests to give a licence back
	// were cancelled (their licences are revoked anyway).
	ReleasesCancelled int64
}

// Update applies a partial patch to a user and returns the row as it stands
// afterwards. Only the fields set in p are written; updated_at is always
// bumped. The service rejects an empty patch before calling this.
//
// In the same transaction it keeps these invariants:
//   - the department-head one (see domain.Department): if the user headed a
//     department and after the update is no longer an active manager of that
//     very department, the department is left without a head;
//   - a blocked user holds no licences: blocking revokes every active
//     assignment of theirs on behalf of actorID and frees the seats;
//   - a blocked user has no pending requests: blocking cancels them, as
//     nobody could approve them any more - requests for a subscription and
//     requests to give a licence back alike;
//   - an active admin is always left: blocking someone or changing their
//     role waits on adminsLock for any other such change, then checks.
//     Two admins blocking each other at once: the second one fails.
//
// Lock order: adminsLock (taken by nothing else), the requests, then the
// user, then their subscriptions, assignments and release requests - the
// order a decision on a request takes them in (the request, then the
// requester when the licence is handed out), and the subscription store's
// (a subscription before its licences and release requests). A
// department's new head is locked the same way: the user before the
// department.
//
// Errors:
//   - errLastAdmin when no active admin would be left.
//   - storage.ErrNotFound when no row has that id.
//   - *storage.ConstraintError "users_email_key" when the new email is taken.
//   - wrapped storage.ErrNotFound when DepartmentID names no department.
//   - any other db error wrapped as "postgres: update user: ...".
func (s *Store) Update(ctx context.Context, id int64, p domain.UserPatch, actorID int64) (domain.User, Undone, error) {
	sets := make([]string, 0, 6)
	args := make([]any, 0, 6)
	add := func(col string, v any) {
		args = append(args, v)
		sets = append(sets, col+" = $"+pg.Itoa(len(args)))
	}
	if p.FullName != nil {
		add("full_name", *p.FullName)
	}
	if p.Email != nil {
		add("email", *p.Email)
	}
	if p.Role != nil {
		add("role", string(*p.Role))
	}
	if p.DepartmentID.Set {
		add("department_id", p.DepartmentID.Value)
	}
	if p.IsActive != nil {
		add("is_active", *p.IsActive)
	}
	sets = append(sets, "updated_at = NOW()")
	args = append(args, id)
	q := `UPDATE subtrack.users SET ` + strings.Join(sets, ", ") +
		` WHERE id = $` + pg.Itoa(len(args)) + ` RETURNING ` + userColumns

	tx, err := pg.Conn(ctx, s.pool).Begin(ctx)
	if err != nil {
		return domain.User{}, Undone{}, pg.Translate(ctx, "begin update user", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	mayDropAdmin := (p.IsActive != nil && !*p.IsActive) || p.Role != nil
	if mayDropAdmin {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, adminsLock); err != nil {
			return domain.User{}, Undone{}, pg.Translate(ctx, "update user lock admins", err)
		}
	}

	var undone Undone
	if p.IsActive != nil && !*p.IsActive {
		tag, err := tx.Exec(ctx, `
			UPDATE subtrack.requests SET status = 'cancelled', updated_at = NOW()
			WHERE requester_id = $1 AND status IN ('pending_manager', 'pending_admin')`, id)
		if err != nil {
			return domain.User{}, Undone{}, pg.Translate(ctx, "update user cancel requests", err)
		}
		undone.RequestsCancelled = tag.RowsAffected()
	}

	u, err := scanUser(tx.QueryRow(ctx, q, args...))
	if err != nil {
		return domain.User{}, Undone{}, pg.Translate(ctx, "update user", err)
	}

	// $3 may be NULL (no department); IS NOT DISTINCT FROM treats that as
	// "not this department", so the head is cleared.
	const clearHead = `
	UPDATE subtrack.departments SET head_id = NULL, updated_at = NOW()
	WHERE head_id = $1 AND NOT ($2::bool AND id IS NOT DISTINCT FROM $3::bigint)`
	stillHead := u.Role == domain.RoleManager && u.IsActive
	if _, err := tx.Exec(ctx, clearHead, u.ID, stillHead, u.DepartmentID); err != nil {
		return domain.User{}, Undone{}, pg.Translate(ctx, "update user clear head", err)
	}
	if !u.IsActive {
		undone.LicencesRevoked, err = revokeLicences(ctx, tx, u.ID, actorID)
		if err != nil {
			return domain.User{}, Undone{}, pg.Translate(ctx, "update user revoke licences", err)
		}
		// After revokeLicences: their subscriptions are held, so nobody
		// decides these meanwhile.
		tag, err := tx.Exec(ctx, `
			UPDATE subtrack.release_requests SET status = 'cancelled', updated_at = NOW()
			WHERE requester_id = $1 AND status = 'pending'`, u.ID)
		if err != nil {
			return domain.User{}, Undone{}, pg.Translate(ctx, "update user cancel release requests", err)
		}
		undone.ReleasesCancelled = tag.RowsAffected()
	}
	if mayDropAdmin {
		var left bool
		err := tx.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM subtrack.users WHERE role = 'admin' AND is_active)`).Scan(&left)
		if err != nil {
			return domain.User{}, Undone{}, pg.Translate(ctx, "update user admins left", err)
		}
		if !left {
			return domain.User{}, Undone{}, errLastAdmin
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.User{}, Undone{}, pg.Translate(ctx, "commit update user", err)
	}
	return u, undone, nil
}

// revokeLicences revokes every active licence of userID on behalf of actorID
// and frees the seats. The subscriptions are locked before their assignments,
// in the order the subscription store uses, so this cannot deadlock with a
// concurrent cancel or revoke. Nothing to revoke (e.g. an already blocked
// user being edited) is a no-op. It returns how many licences it revoked.
func revokeLicences(ctx context.Context, tx pgx.Tx, userID, actorID int64) (int64, error) {
	_, err := tx.Exec(ctx, `
		SELECT s.id FROM subtrack.subscriptions s
		JOIN subtrack.subscription_assignments a ON a.subscription_id = s.id
		WHERE a.user_id = $1 AND a.status = 'active'
		ORDER BY s.id
		FOR UPDATE OF s`, userID)
	if err != nil {
		return 0, err
	}
	// A user holds at most one active licence per subscription (unique index
	// subscription_assignments_active_key), so each subscription is joined
	// once and loses exactly one seat.
	tag, err := tx.Exec(ctx, `
		WITH revoked AS (
		    UPDATE subtrack.subscription_assignments
		    SET status = 'revoked', revoked_at = NOW(), revoked_by = $2
		    WHERE user_id = $1 AND status = 'active'
		    RETURNING subscription_id)
		UPDATE subtrack.subscriptions s SET seats_used = s.seats_used - 1, updated_at = NOW()
		FROM revoked r WHERE s.id = r.subscription_id`, userID, actorID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
