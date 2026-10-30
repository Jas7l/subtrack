package subscription

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"subtrack/internal/domain"
	"subtrack/internal/pg"
	"subtrack/internal/storage"
)

// Store reads and writes subtrack.subscriptions,
// subtrack.subscription_assignments and subtrack.release_requests. The pool
// is borrowed, not owned. Every method runs on pg.Conn(ctx, pool), so it
// joins a transaction started by pg.TxRunner (its own transactions become
// savepoints).
//
// Lock order. Every write that touches more than one of these rows takes the
// locks in the same order - user, then subscription, then its assignments
// and release requests - so two concurrent writes wait for each other
// instead of deadlocking. The user store follows it too when blocking a user
// revokes their licences. Whatever changes an assignment's status or a
// release request together with a licence holds the subscription row first:
// FOR UPDATE to revoke or decide, FOR SHARE to file a request; so those
// writes on one subscription never interleave.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore constructs a Store over the given pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// selectSubscription reads a live subscription with its department's name, in
// the order scanSubscription expects. Cost is read as text so it never passes
// through a float.
const selectSubscription = `
	SELECT s.id, s.name, s.vendor, s.category, s.type, s.cost::text, s.currency, s.billing_period,
	       s.start_date, s.end_date, s.next_payment_date, s.status, s.department_id, d.name,
	       s.seats_total, s.seats_used, s.created_by, s.created_at, s.updated_at
	FROM subtrack.subscriptions s
	LEFT JOIN subtrack.departments d ON d.id = s.department_id`

// queryRower is what both the pool and a transaction offer for single-row
// reads, so a write can re-read its row inside its own transaction.
type queryRower interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func scanSubscription(row pgx.Row) (domain.Subscription, error) {
	var s domain.Subscription
	var typ, period, status string
	err := row.Scan(&s.ID, &s.Name, &s.Vendor, &s.Category, &typ, &s.Cost, &s.Currency, &period,
		&s.StartDate, &s.EndDate, &s.NextPaymentDate, &status, &s.DepartmentID, &s.DepartmentName,
		&s.SeatsTotal, &s.SeatsUsed, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt)
	s.Type = domain.SubscriptionType(typ)
	s.BillingPeriod = domain.BillingPeriod(period)
	s.Status = domain.SubscriptionStatus(status)
	return s, err
}

func getSubscription(ctx context.Context, q queryRower, id int64) (domain.Subscription, error) {
	return scanSubscription(q.QueryRow(ctx, selectSubscription+` WHERE s.id = $1 AND s.deleted_at IS NULL`, id))
}

// GetByID returns one live (not deleted) subscription.
//
// Errors:
//   - storage.ErrNotFound when no live row has that id.
//   - any other db error wrapped as "postgres: get subscription: ...".
func (s *Store) GetByID(ctx context.Context, id int64) (domain.Subscription, error) {
	sub, err := getSubscription(ctx, pg.Conn(ctx, s.pool), id)
	if err != nil {
		return domain.Subscription{}, pg.Translate(ctx, "get subscription", err)
	}
	return sub, nil
}

// GetForUpdate returns one live subscription and locks it until the
// surrounding transaction ends, so an edit written back whole cannot undo
// what the scheduler changed meanwhile. Call it inside pg.TxRunner.InTx.
//
// Errors: as GetByID.
func (s *Store) GetForUpdate(ctx context.Context, id int64) (domain.Subscription, error) {
	sub, err := scanSubscription(pg.Conn(ctx, s.pool).QueryRow(ctx,
		selectSubscription+` WHERE s.id = $1 AND s.deleted_at IS NULL FOR UPDATE OF s`, id))
	if err != nil {
		return domain.Subscription{}, pg.Translate(ctx, "get subscription for update", err)
	}
	return sub, nil
}

// whereBuilder collects SQL conditions and their positional arguments.
type whereBuilder struct {
	conds []string
	args  []any
}

// add appends one condition; every "?" in it becomes the placeholder of v.
func (b *whereBuilder) add(cond string, v any) {
	b.args = append(b.args, v)
	b.conds = append(b.conds, strings.ReplaceAll(cond, "?", "$"+pg.Itoa(len(b.args))))
}

// scope adds the visibility rule of sc: the department's subscriptions or
// those the holder has an active licence for. deptCol is the column holding
// the subscription's department; holderCond is the holder test, with "?" for
// the holder's id (a subscription list checks for an active assignment, an
// assignment list compares its own user_id).
func (b *whereBuilder) scope(sc *domain.SubscriptionScope, deptCol, holderCond string) {
	if sc == nil {
		return
	}
	var or []string
	if sc.DepartmentID != nil {
		b.args = append(b.args, *sc.DepartmentID)
		or = append(or, deptCol+" = $"+pg.Itoa(len(b.args)))
	}
	if sc.HolderID != nil {
		b.args = append(b.args, *sc.HolderID)
		or = append(or, strings.ReplaceAll(holderCond, "?", "$"+pg.Itoa(len(b.args))))
	}
	if len(or) == 0 {
		b.conds = append(b.conds, "FALSE")
		return
	}
	b.conds = append(b.conds, "("+strings.Join(or, " OR ")+")")
}

func (b *whereBuilder) sql() string {
	if len(b.conds) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(b.conds, " AND ")
}

// List returns one page of live subscriptions matching f and visible under
// sc (nil: all), ordered by name, plus the total number of matches.
//
// Errors:
//   - any db error wrapped as "postgres: count subscriptions: ..." /
//     "list subscriptions ...".
func (s *Store) List(ctx context.Context, f domain.SubscriptionFilter, sc *domain.SubscriptionScope, limit, offset int) ([]domain.Subscription, int64, error) {
	var b whereBuilder
	b.conds = append(b.conds, "s.deleted_at IS NULL")
	b.scope(sc, "s.department_id", `EXISTS (SELECT 1 FROM subtrack.subscription_assignments a
		WHERE a.subscription_id = s.id AND a.user_id = ? AND a.status = 'active')`)
	if f.DepartmentID != nil {
		b.add("s.department_id = ?", *f.DepartmentID)
	}
	if f.Category != "" {
		// ILIKE without wildcards is a case-insensitive equality that follows
		// the column's pg_c_utf8 collation; lower(?) would fold the parameter
		// under the database's "C" collation and miss Cyrillic.
		b.add("s.category ILIKE ?", pg.EscapeLike(f.Category))
	}
	if f.Status != nil {
		b.add("s.status = ?", string(*f.Status))
	}
	if f.Type != nil {
		b.add("s.type = ?", string(*f.Type))
	}
	if f.EndFrom != nil {
		b.add("s.end_date >= ?", *f.EndFrom)
	}
	if f.EndTo != nil {
		b.add("s.end_date <= ?", *f.EndTo)
	}
	if f.Query != "" {
		b.add("(s.name ILIKE ? OR s.vendor ILIKE ?)", "%"+pg.EscapeLike(f.Query)+"%")
	}
	if f.FreeSeats != nil {
		if *f.FreeSeats {
			b.conds = append(b.conds, "s.seats_used < s.seats_total")
		} else {
			b.conds = append(b.conds, "s.seats_used >= s.seats_total")
		}
	}
	where := b.sql()

	var total int64
	if err := pg.Conn(ctx, s.pool).QueryRow(ctx, `SELECT COUNT(*) FROM subtrack.subscriptions s`+where, b.args...).Scan(&total); err != nil {
		return nil, 0, pg.Translate(ctx, "count subscriptions", err)
	}

	args := append(b.args, limit, offset)
	q := selectSubscription + where +
		` ORDER BY s.name, s.id LIMIT $` + pg.Itoa(len(args)-1) + ` OFFSET $` + pg.Itoa(len(args))
	rows, err := pg.Conn(ctx, s.pool).Query(ctx, q, args...)
	if err != nil {
		return nil, 0, pg.Translate(ctx, "list subscriptions", err)
	}
	defer rows.Close()

	out := make([]domain.Subscription, 0, limit)
	for rows.Next() {
		sub, err := scanSubscription(rows)
		if err != nil {
			return nil, 0, pg.Translate(ctx, "list subscriptions scan", err)
		}
		out = append(out, sub)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, pg.Translate(ctx, "list subscriptions rows", err)
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

// Create inserts a subscription with no seats in use and returns the stored
// row. ID, SeatsUsed and the timestamps on the input are ignored.
//
// Errors:
//   - wrapped storage.ErrNotFound when the currency, department or creator
//     does not exist.
//   - *storage.CheckError for a value the schema refuses.
//   - any other db error wrapped as "postgres: create subscription: ...".
func (s *Store) Create(ctx context.Context, sub domain.Subscription) (domain.Subscription, error) {
	var id int64
	err := pg.Conn(ctx, s.pool).QueryRow(ctx, `
		INSERT INTO subtrack.subscriptions (name, vendor, category, type, cost, currency, billing_period,
		    start_date, end_date, next_payment_date, status, department_id, seats_total, created_by)
		VALUES ($1, $2, $3, $4, CAST($5::text AS NUMERIC), $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING id`,
		sub.Name, sub.Vendor, sub.Category, string(sub.Type), sub.Cost, sub.Currency, string(sub.BillingPeriod),
		sub.StartDate, sub.EndDate, sub.NextPaymentDate, string(sub.Status), sub.DepartmentID, sub.SeatsTotal,
		sub.CreatedBy).Scan(&id)
	if err != nil {
		return domain.Subscription{}, pg.Translate(ctx, "create subscription", err)
	}
	return s.GetByID(ctx, id)
}

// Update writes every editable column of sub (all but seats_used and
// created_by) and returns the row afterwards. When the new status is
// cancelled, every active licence is revoked by actorID in the same
// transaction and seats_used drops to 0.
//
// Errors:
//   - storage.ErrNotFound when no live row has that id.
//   - *storage.CheckError "subscriptions_seats_check" when seats_total would
//     fall below seats_used.
//   - wrapped storage.ErrNotFound when the currency or department does not
//     exist.
//   - any other db error wrapped as "postgres: update subscription: ...".
func (s *Store) Update(ctx context.Context, sub domain.Subscription, actorID int64) (domain.Subscription, error) {
	tx, err := pg.Conn(ctx, s.pool).Begin(ctx)
	if err != nil {
		return domain.Subscription{}, pg.Translate(ctx, "begin update subscription", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	cancelled := sub.Status == domain.SubCancelled
	var id int64
	err = tx.QueryRow(ctx, `
		UPDATE subtrack.subscriptions SET
		    name = $2, vendor = $3, category = $4, type = $5, cost = CAST($6::text AS NUMERIC),
		    currency = $7, billing_period = $8, start_date = $9, end_date = $10, next_payment_date = $11,
		    status = $12, department_id = $13, seats_total = $14,
		    seats_used = CASE WHEN $15::bool THEN 0 ELSE seats_used END,
		    updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING id`,
		sub.ID, sub.Name, sub.Vendor, sub.Category, string(sub.Type), sub.Cost,
		sub.Currency, string(sub.BillingPeriod), sub.StartDate, sub.EndDate, sub.NextPaymentDate,
		string(sub.Status), sub.DepartmentID, sub.SeatsTotal, cancelled).Scan(&id)
	if err != nil {
		return domain.Subscription{}, pg.Translate(ctx, "update subscription", err)
	}
	if cancelled {
		if err := revokeAll(ctx, tx, id, actorID); err != nil {
			return domain.Subscription{}, pg.Translate(ctx, "update subscription revoke", err)
		}
	}
	out, err := getSubscription(ctx, tx, id)
	if err != nil {
		return domain.Subscription{}, pg.Translate(ctx, "update subscription reread", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Subscription{}, pg.Translate(ctx, "commit update subscription", err)
	}
	return out, nil
}

// Delete soft-deletes a subscription and revokes its active licences on
// behalf of actorID, in one transaction.
//
// Errors:
//   - storage.ErrNotFound when no live row has that id.
//   - any other db error wrapped as "postgres: delete subscription: ...".
func (s *Store) Delete(ctx context.Context, id, actorID int64) error {
	tx, err := pg.Conn(ctx, s.pool).Begin(ctx)
	if err != nil {
		return pg.Translate(ctx, "begin delete subscription", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
		UPDATE subtrack.subscriptions SET deleted_at = NOW(), seats_used = 0, updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		return pg.Translate(ctx, "delete subscription", err)
	}
	if tag.RowsAffected() == 0 {
		return storage.ErrNotFound
	}
	if err := revokeAll(ctx, tx, id, actorID); err != nil {
		return pg.Translate(ctx, "delete subscription revoke", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return pg.Translate(ctx, "commit delete subscription", err)
	}
	return nil
}

// revokeAll revokes every active licence of a subscription. The caller has
// already locked the subscription row and set its seats_used.
func revokeAll(ctx context.Context, tx pgx.Tx, subID, actorID int64) error {
	_, err := tx.Exec(ctx, `
		UPDATE subtrack.subscription_assignments
		SET status = 'revoked', revoked_at = NOW(), revoked_by = $2
		WHERE subscription_id = $1 AND status = 'active'`, subID, actorID)
	return err
}
