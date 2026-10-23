package transaction

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"subtrack/internal/domain"
	"subtrack/internal/paging"
	"subtrack/internal/pg"
)

// Store reads and writes subtrack.transactions. The pool is borrowed, not
// owned. Every method runs on pg.Conn(ctx, pool), so it joins a transaction
// started by pg.TxRunner.
//
// The pending payments the scheduler creates are written by
// scheduler.Store; this store serves the admin.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore constructs a Store over the given pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// selectTransaction reads a payment with its subscription's name (deleted
// subscriptions included), in the order scanTransaction expects. Amount is
// read as text so it never passes through a float.
const selectTransaction = `
	SELECT t.id, t.subscription_id, s.name, t.amount::text, t.currency, t.payment_date, t.status,
	       t.payment_method, t.invoice_number, t.paid_at, t.created_at, t.updated_at
	FROM subtrack.transactions t
	JOIN subtrack.subscriptions s ON s.id = t.subscription_id`

func scanTransaction(row pgx.Row) (domain.Transaction, error) {
	var t domain.Transaction
	var status string
	err := row.Scan(&t.ID, &t.SubscriptionID, &t.SubscriptionName, &t.Amount, &t.Currency, &t.PaymentDate,
		&status, &t.PaymentMethod, &t.InvoiceNumber, &t.PaidAt, &t.CreatedAt, &t.UpdatedAt)
	t.Status = domain.TransactionStatus(status)
	return t, err
}

// GetByID returns one payment.
//
// Errors:
//   - storage.ErrNotFound when no row has that id.
//   - any other db error wrapped as "postgres: get transaction: ...".
func (s *Store) GetByID(ctx context.Context, id int64) (domain.Transaction, error) {
	t, err := scanTransaction(pg.Conn(ctx, s.pool).QueryRow(ctx, selectTransaction+` WHERE t.id = $1`, id))
	if err != nil {
		return domain.Transaction{}, pg.Translate(ctx, "get transaction", err)
	}
	return t, nil
}

// GetForUpdate returns one payment and locks it until the surrounding
// transaction ends, so a status change is decided on a status nobody else
// can change meanwhile. Call it inside pg.TxRunner.InTx.
//
// Errors: as GetByID.
func (s *Store) GetForUpdate(ctx context.Context, id int64) (domain.Transaction, error) {
	t, err := scanTransaction(pg.Conn(ctx, s.pool).QueryRow(ctx, selectTransaction+` WHERE t.id = $1 FOR UPDATE OF t`, id))
	if err != nil {
		return domain.Transaction{}, pg.Translate(ctx, "get transaction for update", err)
	}
	return t, nil
}

// List returns one page of payments matching f, by payment date (newest
// first, or oldest first for paging.SortAsc), plus the total number of
// matches.
//
// Errors:
//   - any db error wrapped as "postgres: count transactions: ..." /
//     "list transactions ...".
func (s *Store) List(ctx context.Context, f domain.TransactionFilter, order paging.SortOrder, limit, offset int) ([]domain.Transaction, int64, error) {
	conds := make([]string, 0, 4)
	args := make([]any, 0, 6)
	add := func(cond string, v any) {
		args = append(args, v)
		conds = append(conds, strings.ReplaceAll(cond, "?", "$"+pg.Itoa(len(args))))
	}
	if f.SubscriptionID != nil {
		add("t.subscription_id = ?", *f.SubscriptionID)
	}
	if f.Status != nil {
		add("t.status = ?", string(*f.Status))
	}
	if f.From != nil {
		add("t.payment_date >= ?::date", *f.From)
	}
	if f.To != nil {
		add("t.payment_date <= ?::date", *f.To)
	}
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}

	db := pg.Conn(ctx, s.pool)
	var total int64
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM subtrack.transactions t`+where, args...).Scan(&total); err != nil {
		return nil, 0, pg.Translate(ctx, "count transactions", err)
	}

	dir := " DESC"
	if order == paging.SortAsc {
		dir = " ASC"
	}
	args = append(args, limit, offset)
	q := selectTransaction + where + ` ORDER BY t.payment_date` + dir + `, t.id` + dir +
		` LIMIT $` + pg.Itoa(len(args)-1) + ` OFFSET $` + pg.Itoa(len(args))
	rows, err := db.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, pg.Translate(ctx, "list transactions", err)
	}
	defer rows.Close()

	out := make([]domain.Transaction, 0, limit)
	for rows.Next() {
		t, err := scanTransaction(rows)
		if err != nil {
			return nil, 0, pg.Translate(ctx, "list transactions scan", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, pg.Translate(ctx, "list transactions rows", err)
	}
	return out, total, nil
}

// Create stores a payment entered by the admin; it is paid at once. ID,
// PaidAt, CreatedAt and UpdatedAt are ignored.
//
// Errors:
//   - *storage.ConstraintError (transactions_subscription_date_key) when the
//     subscription already has a payment on that date.
//   - wrapped storage.ErrNotFound when the subscription or currency does not
//     exist.
//   - any other db error wrapped as "postgres: create transaction: ...".
func (s *Store) Create(ctx context.Context, t domain.Transaction) (domain.Transaction, error) {
	db := pg.Conn(ctx, s.pool)
	var id int64
	err := db.QueryRow(ctx, `
		INSERT INTO subtrack.transactions
		    (subscription_id, amount, currency, payment_date, status, payment_method, invoice_number, paid_at)
		VALUES ($1, CAST($2::text AS NUMERIC), $3, $4, 'paid', $5, $6, NOW())
		RETURNING id`,
		t.SubscriptionID, t.Amount, t.Currency, t.PaymentDate, t.PaymentMethod, t.InvoiceNumber).Scan(&id)
	if err != nil {
		return domain.Transaction{}, pg.Translate(ctx, "create transaction", err)
	}
	return s.GetByID(ctx, id)
}

// SetStatus moves a payment the caller has locked to status, setting paid_at
// for paid. A non-empty method or invoice replaces the stored one.
//
// Errors:
//   - storage.ErrNotFound when no row has that id.
//   - any other db error wrapped as "postgres: set transaction status: ...".
func (s *Store) SetStatus(ctx context.Context, id int64, status domain.TransactionStatus, method, invoice string) (domain.Transaction, error) {
	tag, err := pg.Conn(ctx, s.pool).Exec(ctx, `
		UPDATE subtrack.transactions SET
		    status = $2,
		    paid_at = CASE WHEN $2 = 'paid' THEN NOW() END,
		    payment_method = COALESCE(NULLIF($3, ''), payment_method),
		    invoice_number = COALESCE(NULLIF($4, ''), invoice_number),
		    updated_at = NOW()
		WHERE id = $1`, id, string(status), method, invoice)
	if err != nil {
		return domain.Transaction{}, pg.Translate(ctx, "set transaction status", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.Transaction{}, pg.Translate(ctx, "set transaction status", pgx.ErrNoRows)
	}
	return s.GetByID(ctx, id)
}
