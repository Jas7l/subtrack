package department

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"subtrack/internal/domain"
	"subtrack/internal/pg"
)

// Store reads and writes subtrack.departments. The pool is borrowed, not
// owned. Every method runs on pg.Conn(ctx, pool), so it joins a transaction
// started by pg.TxRunner (its own transactions become savepoints).
type Store struct {
	pool *pgxpool.Pool
}

// NewStore constructs a Store over the given pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// selectDepartment reads a department together with its head's name and the
// number of its active people, in the order scanDepartment expects.
const selectDepartment = `
	SELECT d.id, d.name, d.head_id, h.full_name,
	       (SELECT COUNT(*) FROM subtrack.users u WHERE u.department_id = d.id AND u.is_active),
	       d.created_at, d.updated_at
	FROM subtrack.departments d
	LEFT JOIN subtrack.users h ON h.id = d.head_id`

// queryRower is what both the pool and a transaction offer for single-row
// reads, so a write can re-read its row inside its own transaction.
type queryRower interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func scanDepartment(row pgx.Row) (domain.Department, error) {
	var d domain.Department
	err := row.Scan(&d.ID, &d.Name, &d.HeadID, &d.HeadName, &d.EmployeeCount, &d.CreatedAt, &d.UpdatedAt)
	return d, err
}

func getDepartment(ctx context.Context, q queryRower, id int64) (domain.Department, error) {
	return scanDepartment(q.QueryRow(ctx, selectDepartment+` WHERE d.id = $1`, id))
}

// GetByID returns one department.
//
// Errors:
//   - storage.ErrNotFound when no row has that id.
//   - any other db error wrapped as "postgres: get department: ...".
func (s *Store) GetByID(ctx context.Context, id int64) (domain.Department, error) {
	d, err := getDepartment(ctx, pg.Conn(ctx, s.pool), id)
	if err != nil {
		return domain.Department{}, pg.Translate(ctx, "get department", err)
	}
	return d, nil
}

// List returns one page of departments ordered by name, plus the total
// number of matches. query, when not empty, matches a substring of the name
// case-insensitively.
//
// Errors:
//   - any db error wrapped as "postgres: count departments: ..." /
//     "list departments ...".
func (s *Store) List(ctx context.Context, query string, limit, offset int) ([]domain.Department, int64, error) {
	where, args := "", []any{}
	if query != "" {
		where = ` WHERE d.name ILIKE $1`
		args = append(args, "%"+pg.EscapeLike(query)+"%")
	}

	var total int64
	if err := pg.Conn(ctx, s.pool).QueryRow(ctx, `SELECT COUNT(*) FROM subtrack.departments d`+where, args...).Scan(&total); err != nil {
		return nil, 0, pg.Translate(ctx, "count departments", err)
	}

	args = append(args, limit, offset)
	q := selectDepartment + where +
		` ORDER BY d.name, d.id LIMIT $` + pg.Itoa(len(args)-1) + ` OFFSET $` + pg.Itoa(len(args))
	rows, err := pg.Conn(ctx, s.pool).Query(ctx, q, args...)
	if err != nil {
		return nil, 0, pg.Translate(ctx, "list departments", err)
	}
	defer rows.Close()

	out := make([]domain.Department, 0, limit)
	for rows.Next() {
		d, err := scanDepartment(rows)
		if err != nil {
			return nil, 0, pg.Translate(ctx, "list departments scan", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, pg.Translate(ctx, "list departments rows", err)
	}
	return out, total, nil
}

// errHeadGone is returned when the new head is no longer an active manager
// by the time the department is written (blocked or demoted meanwhile).
var errHeadGone = errors.New("department store: head is missing, blocked or not a manager")

// Create inserts a department and, when HeadID is set, moves the head into it
// - one transaction, so the department never exists with a head who works
// elsewhere. Returns the stored row. The service has already checked that the
// head is an active manager; lockHead checks it again under lock.
//
// Errors:
//   - errHeadGone when the head stopped being an active manager meanwhile.
//   - *storage.ConstraintError "departments_name_key" when the name is taken
//     (case-insensitively).
//   - *storage.ConstraintError "departments_head_id_key" when the head already
//     heads another department.
//   - wrapped storage.ErrNotFound when HeadID names no user.
//   - any other db error wrapped as "postgres: create department: ...".
func (s *Store) Create(ctx context.Context, d domain.Department) (domain.Department, error) {
	tx, err := pg.Conn(ctx, s.pool).Begin(ctx)
	if err != nil {
		return domain.Department{}, pg.Translate(ctx, "begin create department", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if d.HeadID != nil {
		if err := lockHead(ctx, tx, *d.HeadID); err != nil {
			return domain.Department{}, err
		}
	}
	var id int64
	err = tx.QueryRow(ctx,
		`INSERT INTO subtrack.departments (name, head_id) VALUES ($1, $2) RETURNING id`,
		d.Name, d.HeadID).Scan(&id)
	if err != nil {
		return domain.Department{}, pg.Translate(ctx, "create department", err)
	}
	if d.HeadID != nil {
		if err := moveHead(ctx, tx, id, *d.HeadID); err != nil {
			return domain.Department{}, pg.Translate(ctx, "create department move head", err)
		}
	}
	out, err := getDepartment(ctx, tx, id)
	if err != nil {
		return domain.Department{}, pg.Translate(ctx, "create department reread", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Department{}, pg.Translate(ctx, "commit create department", err)
	}
	return out, nil
}

// Update applies a partial patch and returns the row afterwards. A new head
// is moved into the department in the same transaction; the previous head, if
// any, stays in the department as an ordinary manager.
//
// Errors:
//   - errHeadGone when the new head stopped being an active manager
//     meanwhile.
//   - storage.ErrNotFound when no row has that id.
//   - the same constraint errors as Create.
//   - any other db error wrapped as "postgres: update department: ...".
func (s *Store) Update(ctx context.Context, id int64, p domain.DepartmentPatch) (domain.Department, error) {
	sets := make([]string, 0, 3)
	args := make([]any, 0, 3)
	add := func(col string, v any) {
		args = append(args, v)
		sets = append(sets, col+" = $"+pg.Itoa(len(args)))
	}
	if p.Name != nil {
		add("name", *p.Name)
	}
	if p.HeadID.Set {
		add("head_id", p.HeadID.Value)
	}
	sets = append(sets, "updated_at = NOW()")
	args = append(args, id)
	q := `UPDATE subtrack.departments SET ` + strings.Join(sets, ", ") +
		` WHERE id = $` + pg.Itoa(len(args)) + ` RETURNING id`

	tx, err := pg.Conn(ctx, s.pool).Begin(ctx)
	if err != nil {
		return domain.Department{}, pg.Translate(ctx, "begin update department", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if p.HeadID.Set && p.HeadID.Value != nil {
		if err := lockHead(ctx, tx, *p.HeadID.Value); err != nil {
			return domain.Department{}, err
		}
	}
	if err := tx.QueryRow(ctx, q, args...).Scan(&id); err != nil {
		return domain.Department{}, pg.Translate(ctx, "update department", err)
	}
	if p.HeadID.Set && p.HeadID.Value != nil {
		if err := moveHead(ctx, tx, id, *p.HeadID.Value); err != nil {
			return domain.Department{}, pg.Translate(ctx, "update department move head", err)
		}
	}
	out, err := getDepartment(ctx, tx, id)
	if err != nil {
		return domain.Department{}, pg.Translate(ctx, "update department reread", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Department{}, pg.Translate(ctx, "commit update department", err)
	}
	return out, nil
}

// lockHead locks the user who is to head a department and checks, under that
// lock, that they are still an active manager: blocking or demoting them at
// the same moment either waits for the appointment (and then leaves the
// department without a head, as usual) or makes it fail. The user is locked
// before the department, the order user.Store.Update takes them in.
func lockHead(ctx context.Context, tx pgx.Tx, headID int64) error {
	var one int
	err := tx.QueryRow(ctx, `
		SELECT 1 FROM subtrack.users WHERE id = $1 AND is_active AND role = 'manager'
		FOR UPDATE`, headID).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return errHeadGone
	}
	if err != nil {
		return pg.Translate(ctx, "lock head", err)
	}
	return nil
}

// moveHead puts the head into the department they now lead. They cannot be
// head of the department they leave: the unique index on head_id already
// stopped that.
func moveHead(ctx context.Context, tx pgx.Tx, deptID, headID int64) error {
	_, err := tx.Exec(ctx,
		`UPDATE subtrack.users SET department_id = $1, updated_at = NOW()
		 WHERE id = $2 AND department_id IS DISTINCT FROM $1`, deptID, headID)
	return err
}

// Delete removes a department, but only an empty one: with no active people
// and no live subscriptions. When it is not empty nothing is written and the
// counts say why. Blocked users who still point at it are detached by the
// foreign key (ON DELETE SET NULL), as are past requests.
//
// Errors:
//   - storage.ErrNotFound when no row has that id.
//   - any other db error wrapped as "postgres: delete department: ...".
func (s *Store) Delete(ctx context.Context, id int64) (users, subscriptions int64, err error) {
	tx, err := pg.Conn(ctx, s.pool).Begin(ctx)
	if err != nil {
		return 0, 0, pg.Translate(ctx, "begin delete department", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Locking the row makes a concurrent "add a user to this department" wait
	// for the decision instead of slipping in after the count.
	var locked int64
	if err := tx.QueryRow(ctx, `SELECT id FROM subtrack.departments WHERE id = $1 FOR UPDATE`, id).Scan(&locked); err != nil {
		return 0, 0, pg.Translate(ctx, "delete department lock", err)
	}
	const count = `
	SELECT (SELECT COUNT(*) FROM subtrack.users WHERE department_id = $1 AND is_active),
	       (SELECT COUNT(*) FROM subtrack.subscriptions WHERE department_id = $1 AND deleted_at IS NULL)`
	if err := tx.QueryRow(ctx, count, id).Scan(&users, &subscriptions); err != nil {
		return 0, 0, pg.Translate(ctx, "delete department count", err)
	}
	if users > 0 || subscriptions > 0 {
		return users, subscriptions, nil
	}
	if _, err := tx.Exec(ctx, `DELETE FROM subtrack.departments WHERE id = $1`, id); err != nil {
		return 0, 0, pg.Translate(ctx, "delete department", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, 0, pg.Translate(ctx, "commit delete department", err)
	}
	return 0, 0, nil
}
