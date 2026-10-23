package settings

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"subtrack/internal/domain"
	"subtrack/internal/pg"
)

// Store reads and writes the single row of subtrack.notification_settings
// (created by migration 0010). The pool is borrowed, not owned. Every method
// runs on pg.Conn(ctx, pool), so it joins a transaction started by
// pg.TxRunner.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore constructs a Store over the given pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func scanSettings(row pgx.Row) (domain.NotificationSettings, error) {
	var st domain.NotificationSettings
	err := row.Scan(&st.ExpiringDays, &st.PaymentDays, &st.UpdatedAt, &st.UpdatedBy)
	return st, err
}

// Get returns the notification settings.
//
// Errors:
//   - storage.ErrNotFound when the row is missing (migration 0010 not
//     applied as is).
//   - any other db error wrapped as "postgres: get notification settings: ...".
func (s *Store) Get(ctx context.Context) (domain.NotificationSettings, error) {
	st, err := scanSettings(pg.Conn(ctx, s.pool).QueryRow(ctx, `
		SELECT expiring_days, payment_days, updated_at, updated_by FROM subtrack.notification_settings`))
	if err != nil {
		return domain.NotificationSettings{}, pg.Translate(ctx, "get notification settings", err)
	}
	return st, nil
}

// Update replaces both values on behalf of actorID and returns the new row.
//
// Errors:
//   - *storage.CheckError when a value is out of range.
//   - storage.ErrNotFound when the row is missing.
//   - any other db error wrapped as "postgres: update notification settings: ...".
func (s *Store) Update(ctx context.Context, st domain.NotificationSettings, actorID int64) (domain.NotificationSettings, error) {
	out, err := scanSettings(pg.Conn(ctx, s.pool).QueryRow(ctx, `
		UPDATE subtrack.notification_settings
		SET expiring_days = $1, payment_days = $2, updated_by = $3, updated_at = NOW()
		RETURNING expiring_days, payment_days, updated_at, updated_by`,
		st.ExpiringDays, st.PaymentDays, actorID))
	if err != nil {
		return domain.NotificationSettings{}, pg.Translate(ctx, "update notification settings", err)
	}
	return out, nil
}
