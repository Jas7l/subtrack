package analytics

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"subtrack/internal/domain"
	"subtrack/internal/pg"
)

// Store runs the read-only queries behind the reports. The pool is
// borrowed, not owned. Amounts come back as NUMERIC text and are converted
// to roubles in Go (see money.go), except for payment history, which is
// summed in SQL.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore constructs a Store over the given pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Subscriptions returns the live subscriptions that are not cancelled
// (active, expiring_soon, expired), of one department when deptID is set,
// each with the current rate of its currency.
//
// Errors:
//   - any db error wrapped as "postgres: analytics subscriptions ...".
func (s *Store) Subscriptions(ctx context.Context, deptID *int64) ([]sub, error) {
	rows, err := pg.Conn(ctx, s.pool).Query(ctx, `
		SELECT s.id, s.name, s.vendor, s.category, s.type, s.billing_period, s.cost::text, s.currency,
		       c.rate_to_rub::text, s.start_date, s.end_date, s.next_payment_date, s.status,
		       s.department_id, d.name, s.seats_total, s.seats_used, s.created_at
		FROM subtrack.subscriptions s
		JOIN subtrack.currencies c ON c.code = s.currency
		LEFT JOIN subtrack.departments d ON d.id = s.department_id
		WHERE s.deleted_at IS NULL AND s.status <> 'cancelled'
		  AND ($1::bigint IS NULL OR s.department_id = $1)
		ORDER BY s.id`, deptID)
	if err != nil {
		return nil, pg.Translate(ctx, "analytics subscriptions", err)
	}
	defer rows.Close()
	var out []sub
	for rows.Next() {
		var x sub
		var typ, period, cost, rate, status string
		if err := rows.Scan(&x.ID, &x.Name, &x.Vendor, &x.Category, &typ, &period, &cost, &x.Currency,
			&rate, &x.StartDate, &x.EndDate, &x.NextPaymentDate, &status,
			&x.DepartmentID, &x.DepartmentName, &x.SeatsTotal, &x.SeatsUsed, &x.CreatedAt); err != nil {
			return nil, pg.Translate(ctx, "analytics subscriptions scan", err)
		}
		x.Type, x.Period, x.Status = domain.SubscriptionType(typ), domain.BillingPeriod(period), domain.SubscriptionStatus(status)
		x.Cost, x.Rate = rat(cost), rat(rate)
		out = append(out, x)
	}
	if err := rows.Err(); err != nil {
		return nil, pg.Translate(ctx, "analytics subscriptions rows", err)
	}
	return out, nil
}

// dept is a department with its number of active users.
type dept struct {
	ID    int64
	Name  string
	Users int
}

// Departments lists every department with its active users, by name, and
// the number of active users in the whole company.
//
// Errors:
//   - any db error wrapped as "postgres: analytics departments ...".
func (s *Store) Departments(ctx context.Context) ([]dept, int, error) {
	db := pg.Conn(ctx, s.pool)
	var total int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM subtrack.users WHERE is_active`).Scan(&total); err != nil {
		return nil, 0, pg.Translate(ctx, "analytics users", err)
	}
	rows, err := db.Query(ctx, `
		SELECT d.id, d.name, COUNT(u.id)
		FROM subtrack.departments d
		LEFT JOIN subtrack.users u ON u.department_id = d.id AND u.is_active
		GROUP BY d.id, d.name
		ORDER BY d.name, d.id`)
	if err != nil {
		return nil, 0, pg.Translate(ctx, "analytics departments", err)
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (dept, error) {
		var d dept
		err := row.Scan(&d.ID, &d.Name, &d.Users)
		return d, err
	})
	if err != nil {
		return nil, 0, pg.Translate(ctx, "analytics departments rows", err)
	}
	return out, total, nil
}

// monthSpend is the paid total of one calendar month.
type monthSpend struct {
	Month     time.Time // the first day
	AmountRUB string
	Payments  int
}

// Spending sums paid payments by the month of their payment date, from the
// first day of from to the last day of to's month, in roubles at the current
// rates; months without payments are absent. deptID limits it to the
// subscriptions of one department.
//
// Errors:
//   - any db error wrapped as "postgres: analytics spending ...".
func (s *Store) Spending(ctx context.Context, from, to time.Time, deptID *int64) ([]monthSpend, error) {
	rows, err := pg.Conn(ctx, s.pool).Query(ctx, `
		SELECT date_trunc('month', t.payment_date)::date AS month,
		       ROUND(SUM(t.amount * c.rate_to_rub), 2)::text, COUNT(*)
		FROM subtrack.transactions t
		JOIN subtrack.currencies c ON c.code = t.currency
		JOIN subtrack.subscriptions s ON s.id = t.subscription_id
		WHERE t.status = 'paid' AND t.payment_date >= $1::date
		  AND t.payment_date < ($2::date + INTERVAL '1 month')
		  AND ($3::bigint IS NULL OR s.department_id = $3)
		GROUP BY month ORDER BY month`, from, to, deptID)
	if err != nil {
		return nil, pg.Translate(ctx, "analytics spending", err)
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (monthSpend, error) {
		var m monthSpend
		err := row.Scan(&m.Month, &m.AmountRUB, &m.Payments)
		return m, err
	})
	if err != nil {
		return nil, pg.Translate(ctx, "analytics spending rows", err)
	}
	return out, nil
}

// PaymentStats is the state of payments for the admin's summary.
type PaymentStats struct {
	PaidThisMonthRUB string
	PaidThisYearRUB  string
	PendingCount     int
	PendingRUB       string
	// OverdueCount counts pending payments whose date has passed.
	OverdueCount int
	FailedCount  int
}

// Payments sums the payments for the summary: paid in today's month and
// year (by payment date), pending (and overdue among them) and failed.
//
// Errors:
//   - any db error wrapped as "postgres: analytics payments: ...".
func (s *Store) Payments(ctx context.Context, today time.Time, deptID *int64) (PaymentStats, error) {
	var p PaymentStats
	err := pg.Conn(ctx, s.pool).QueryRow(ctx, `
		SELECT
		  COALESCE(ROUND(SUM(t.amount * c.rate_to_rub) FILTER (WHERE t.status = 'paid'
		      AND date_trunc('month', t.payment_date) = date_trunc('month', $1::date)), 2), 0)::text,
		  COALESCE(ROUND(SUM(t.amount * c.rate_to_rub) FILTER (WHERE t.status = 'paid'
		      AND date_trunc('year', t.payment_date) = date_trunc('year', $1::date)), 2), 0)::text,
		  COUNT(*) FILTER (WHERE t.status = 'pending'),
		  COALESCE(ROUND(SUM(t.amount * c.rate_to_rub) FILTER (WHERE t.status = 'pending'), 2), 0)::text,
		  COUNT(*) FILTER (WHERE t.status = 'pending' AND t.payment_date < $1::date),
		  COUNT(*) FILTER (WHERE t.status = 'failed')
		FROM subtrack.transactions t
		JOIN subtrack.currencies c ON c.code = t.currency
		JOIN subtrack.subscriptions s ON s.id = t.subscription_id
		WHERE ($2::bigint IS NULL OR s.department_id = $2)`, today, deptID).Scan(
		&p.PaidThisMonthRUB, &p.PaidThisYearRUB, &p.PendingCount, &p.PendingRUB, &p.OverdueCount, &p.FailedCount)
	if err != nil {
		return PaymentStats{}, pg.Translate(ctx, "analytics payments", err)
	}
	return p, nil
}
