package scheduler

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"subtrack/internal/domain"
	"subtrack/internal/pg"
)

// Store holds the scheduler's own queries over subtrack.subscriptions,
// subtrack.subscription_assignments and subtrack.notification_log. The pool is
// borrowed, not owned. Every method runs on pg.Conn(ctx, pool), so it joins
// the transaction of the run's current subscription.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore constructs a Store over the given pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// selectSub reads the fields plan needs, in the order scanSub expects.
const selectSub = `
	SELECT id, name, cost::text, currency, end_date, next_payment_date, status
	FROM subtrack.subscriptions`

func scanSub(row pgx.Row) (domain.Subscription, error) {
	var s domain.Subscription
	var status string
	err := row.Scan(&s.ID, &s.Name, &s.Cost, &s.Currency, &s.EndDate, &s.NextPaymentDate, &status)
	s.Status = domain.SubscriptionStatus(status)
	return s, err
}

// Candidates returns the ids of live subscriptions that may need something
// done today: any not cancelled or expired whose end date has come within
// expiringUntil or passed, whose next payment falls between today and
// paymentUntil, or which is expiring_soon (its end date may have moved
// away). plan decides what exactly; the list is only a cheap first cut.
//
// Errors:
//   - any db error wrapped as "postgres: scheduler candidates ...".
func (s *Store) Candidates(ctx context.Context, today, expiringUntil, paymentUntil time.Time) ([]int64, error) {
	rows, err := pg.Conn(ctx, s.pool).Query(ctx, `
		SELECT id FROM subtrack.subscriptions
		WHERE deleted_at IS NULL AND status IN ('active', 'expiring_soon')
		  AND (status = 'expiring_soon'
		       OR end_date <= $2::date
		       OR next_payment_date BETWEEN $1::date AND $3::date)
		ORDER BY id`, today, expiringUntil, paymentUntil)
	if err != nil {
		return nil, pg.Translate(ctx, "scheduler candidates", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return nil, pg.Translate(ctx, "scheduler candidates rows", err)
	}
	return ids, nil
}

// Lock reads one live subscription and locks its row until the transaction
// ends, so a concurrent cancel or edit waits for the scheduler (or the
// scheduler for it) instead of being judged on a stale read.
//
// Errors:
//   - storage.ErrNotFound when no live row has that id (deleted meanwhile).
//   - any other db error wrapped as "postgres: scheduler lock subscription: ...".
func (s *Store) Lock(ctx context.Context, id int64) (domain.Subscription, error) {
	sub, err := scanSub(pg.Conn(ctx, s.pool).QueryRow(ctx,
		selectSub+` WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, id))
	if err != nil {
		return domain.Subscription{}, pg.Translate(ctx, "scheduler lock subscription", err)
	}
	return sub, nil
}

// SetStatus changes the status of a subscription the caller has locked.
//
// Errors:
//   - any db error wrapped as "postgres: scheduler set status: ...".
func (s *Store) SetStatus(ctx context.Context, id int64, status domain.SubscriptionStatus) error {
	_, err := pg.Conn(ctx, s.pool).Exec(ctx, `
		UPDATE subtrack.subscriptions SET status = $2, updated_at = NOW() WHERE id = $1`, id, string(status))
	return pg.Translate(ctx, "scheduler set status", err)
}

// MarkSent records that the event with this key is being notified. It
// reports false when the key was already there: the event was notified
// before and must not be again.
//
// Errors:
//   - any db error wrapped as "postgres: scheduler mark sent: ...".
func (s *Store) MarkSent(ctx context.Context, key string) (bool, error) {
	tag, err := pg.Conn(ctx, s.pool).Exec(ctx,
		`INSERT INTO subtrack.notification_log (key) VALUES ($1) ON CONFLICT (key) DO NOTHING`, key)
	if err != nil {
		return false, pg.Translate(ctx, "scheduler mark sent", err)
	}
	return tag.RowsAffected() == 1, nil
}

// Holders returns the active non-admin users holding an active licence of the
// subscription. Admins are left out: they are notified as admins anyway.
//
// Errors:
//   - any db error wrapped as "postgres: scheduler holders ...".
func (s *Store) Holders(ctx context.Context, subID int64) ([]int64, error) {
	rows, err := pg.Conn(ctx, s.pool).Query(ctx, `
		SELECT u.id FROM subtrack.subscription_assignments a
		JOIN subtrack.users u ON u.id = a.user_id
		WHERE a.subscription_id = $1 AND a.status = 'active' AND u.is_active AND u.role <> 'admin'
		ORDER BY u.id`, subID)
	if err != nil {
		return nil, pg.Translate(ctx, "scheduler holders", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return nil, pg.Translate(ctx, "scheduler holders rows", err)
	}
	return ids, nil
}
