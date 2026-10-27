package request

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"subtrack/internal/domain"
	"subtrack/internal/pg"
	"subtrack/internal/storage"
)

// Store reads and writes subtrack.requests. The pool is borrowed, not owned.
// Every method runs on pg.Conn(ctx, pool), so it joins a transaction started
// by pg.TxRunner.
//
// The decision writes are conditional on the current status: a request that
// moved on meanwhile (decided by someone else, cancelled) is not written and
// the write reports storage.ErrNotFound, so the state machine cannot be
// skipped by two clicks racing each other.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore constructs a Store over the given pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// selectRequest reads a request with the names of the requester, the
// department and whoever decided it, in the order scanRequest expects.
const selectRequest = `
	SELECT r.id, r.requester_id, u.full_name, r.department_id, d.name, r.name, r.vendor, r.justification,
	       r.estimated_cost::text, r.currency, r.subscription_id, r.status,
	       r.manager_id, m.full_name, r.manager_comment, r.manager_decided_at,
	       r.admin_id, a.full_name, r.admin_comment, r.admin_decided_at, r.created_at, r.updated_at
	FROM subtrack.requests r
	JOIN subtrack.users u ON u.id = r.requester_id
	LEFT JOIN subtrack.departments d ON d.id = r.department_id
	LEFT JOIN subtrack.users m ON m.id = r.manager_id
	LEFT JOIN subtrack.users a ON a.id = r.admin_id`

func scanRequest(row pgx.Row) (domain.Request, error) {
	var r domain.Request
	var status string
	err := row.Scan(&r.ID, &r.RequesterID, &r.RequesterName, &r.DepartmentID, &r.DepartmentName,
		&r.Name, &r.Vendor, &r.Justification, &r.EstimatedCost, &r.Currency, &r.SubscriptionID, &status,
		&r.ManagerID, &r.ManagerName, &r.ManagerComment, &r.ManagerDecidedAt,
		&r.AdminID, &r.AdminName, &r.AdminComment, &r.AdminDecidedAt, &r.CreatedAt, &r.UpdatedAt)
	r.Status = domain.RequestStatus(status)
	return r, err
}

// GetByID returns one request.
//
// Errors:
//   - storage.ErrNotFound when no row has that id.
//   - any other db error wrapped as "postgres: get request: ...".
func (s *Store) GetByID(ctx context.Context, id int64) (domain.Request, error) {
	r, err := scanRequest(pg.Conn(ctx, s.pool).QueryRow(ctx, selectRequest+` WHERE r.id = $1`, id))
	if err != nil {
		return domain.Request{}, pg.Translate(ctx, "get request", err)
	}
	return r, nil
}

// GetForUpdate returns one request and locks it until the surrounding
// transaction ends, so a decision is made on a status nobody else can change
// meanwhile. Call it inside pg.TxRunner.InTx.
//
// Errors: as GetByID.
func (s *Store) GetForUpdate(ctx context.Context, id int64) (domain.Request, error) {
	r, err := scanRequest(pg.Conn(ctx, s.pool).QueryRow(ctx, selectRequest+` WHERE r.id = $1 FOR UPDATE OF r`, id))
	if err != nil {
		return domain.Request{}, pg.Translate(ctx, "get request for update", err)
	}
	return r, nil
}

// List returns one page of requests matching f and visible under sc (nil:
// all), newest first, plus the total number of matches.
//
// Errors:
//   - any db error wrapped as "postgres: count requests: ..." /
//     "list requests ...".
func (s *Store) List(ctx context.Context, f domain.RequestFilter, sc *domain.RequestScope, limit, offset int) ([]domain.Request, int64, error) {
	conds := make([]string, 0, 4)
	args := make([]any, 0, 6)
	add := func(cond string, v any) {
		args = append(args, v)
		conds = append(conds, strings.ReplaceAll(cond, "?", "$"+pg.Itoa(len(args))))
	}
	if sc != nil {
		var or []string
		if sc.DepartmentID != nil {
			args = append(args, *sc.DepartmentID)
			or = append(or, "r.department_id = $"+pg.Itoa(len(args)))
		}
		if sc.RequesterID != nil {
			args = append(args, *sc.RequesterID)
			or = append(or, "r.requester_id = $"+pg.Itoa(len(args)))
		}
		if len(or) == 0 {
			or = append(or, "FALSE")
		}
		conds = append(conds, "("+strings.Join(or, " OR ")+")")
	}
	if f.Status != nil {
		add("r.status = ?", string(*f.Status))
	}
	if f.RequesterID != nil {
		add("r.requester_id = ?", *f.RequesterID)
	}
	if f.DepartmentID != nil {
		add("r.department_id = ?", *f.DepartmentID)
	}
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}

	db := pg.Conn(ctx, s.pool)
	var total int64
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM subtrack.requests r`+where, args...).Scan(&total); err != nil {
		return nil, 0, pg.Translate(ctx, "count requests", err)
	}

	args = append(args, limit, offset)
	q := selectRequest + where +
		` ORDER BY r.created_at DESC, r.id DESC LIMIT $` + pg.Itoa(len(args)-1) + ` OFFSET $` + pg.Itoa(len(args))
	rows, err := db.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, pg.Translate(ctx, "list requests", err)
	}
	defer rows.Close()

	out := make([]domain.Request, 0, limit)
	for rows.Next() {
		r, err := scanRequest(rows)
		if err != nil {
			return nil, 0, pg.Translate(ctx, "list requests scan", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, pg.Translate(ctx, "list requests rows", err)
	}
	return out, total, nil
}

// CurrencyExists reports whether code is a known currency.
func (s *Store) CurrencyExists(ctx context.Context, code string) (bool, error) {
	var ok bool
	err := pg.Conn(ctx, s.pool).QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM subtrack.currencies WHERE code = $1)`, code).Scan(&ok)
	if err != nil {
		return false, pg.Translate(ctx, "currency exists", err)
	}
	return ok, nil
}

// Create inserts a request in the status the service chose and returns the
// stored row. Decision fields on the input are ignored.
//
// Errors:
//   - wrapped storage.ErrNotFound when the requester, department or
//     currency does not exist.
//   - any other db error wrapped as "postgres: create request: ...".
func (s *Store) Create(ctx context.Context, r domain.Request) (domain.Request, error) {
	var id int64
	err := pg.Conn(ctx, s.pool).QueryRow(ctx, `
		INSERT INTO subtrack.requests (requester_id, department_id, name, vendor, justification,
		    estimated_cost, currency, status)
		VALUES ($1, $2, $3, $4, $5, CAST($6::text AS NUMERIC), $7, $8)
		RETURNING id`,
		r.RequesterID, r.DepartmentID, r.Name, r.Vendor, r.Justification,
		r.EstimatedCost, r.Currency, string(r.Status)).Scan(&id)
	if err != nil {
		return domain.Request{}, pg.Translate(ctx, "create request", err)
	}
	return s.GetByID(ctx, id)
}

// DecideAsManager records the department head's decision on a request that
// is still pending_manager: approval moves it to pending_admin, rejection to
// rejected_by_manager.
//
// Errors:
//   - storage.ErrNotFound when no pending_manager request has that id.
//   - any other db error wrapped as "postgres: manager decision: ...".
func (s *Store) DecideAsManager(ctx context.Context, id int64, d domain.Decision) (domain.Request, error) {
	next := domain.ReqRejectedByManager
	if d.Approve {
		next = domain.ReqPendingAdmin
	}
	tag, err := pg.Conn(ctx, s.pool).Exec(ctx, `
		UPDATE subtrack.requests
		SET status = $2, manager_id = $3, manager_comment = $4, manager_decided_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND status = 'pending_manager'`,
		id, string(next), d.ReviewerID, d.Comment)
	if err != nil {
		return domain.Request{}, pg.Translate(ctx, "manager decision", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.Request{}, storage.ErrNotFound
	}
	return s.GetByID(ctx, id)
}

// DecideAsAdmin records the admin's final decision on a pending request:
// approved (with the subscription the requester got) or rejected_by_admin.
// The service has already checked that the admin may decide at this stage.
//
// Errors:
//   - storage.ErrNotFound when no pending request has that id.
//   - any other db error wrapped as "postgres: admin decision: ...".
func (s *Store) DecideAsAdmin(ctx context.Context, id int64, d domain.Decision) (domain.Request, error) {
	next := domain.ReqRejectedByAdmin
	if d.Approve {
		next = domain.ReqApproved
	}
	tag, err := pg.Conn(ctx, s.pool).Exec(ctx, `
		UPDATE subtrack.requests
		SET status = $2, admin_id = $3, admin_comment = $4, admin_decided_at = NOW(),
		    subscription_id = COALESCE($5, subscription_id), updated_at = NOW()
		WHERE id = $1 AND status IN ('pending_manager', 'pending_admin')`,
		id, string(next), d.ReviewerID, d.Comment, d.SubscriptionID)
	if err != nil {
		return domain.Request{}, pg.Translate(ctx, "admin decision", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.Request{}, storage.ErrNotFound
	}
	return s.GetByID(ctx, id)
}

// Cancel withdraws a pending request on behalf of its requester.
//
// Errors:
//   - storage.ErrNotFound when requesterID has no pending request with that
//     id.
//   - any other db error wrapped as "postgres: cancel request: ...".
func (s *Store) Cancel(ctx context.Context, id, requesterID int64) (domain.Request, error) {
	tag, err := pg.Conn(ctx, s.pool).Exec(ctx, `
		UPDATE subtrack.requests SET status = 'cancelled', updated_at = NOW()
		WHERE id = $1 AND requester_id = $2 AND status IN ('pending_manager', 'pending_admin')`,
		id, requesterID)
	if err != nil {
		return domain.Request{}, pg.Translate(ctx, "cancel request", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.Request{}, storage.ErrNotFound
	}
	return s.GetByID(ctx, id)
}
