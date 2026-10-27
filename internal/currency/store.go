package currency

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"subtrack/internal/domain"
	"subtrack/internal/pg"
	"subtrack/internal/storage"
)

// errInUse is what Delete reports when subscriptions, requests or payments
// still point at the currency (a foreign key refuses the delete).
var errInUse = errors.New("currency store: the currency is in use")

// Store reads and writes subtrack.currencies (created by migration 0003).
// The pool is borrowed, not owned. Every method runs on pg.Conn(ctx, pool),
// so it joins a transaction started by pg.TxRunner.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore constructs a Store over the given pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// selectCurrency reads a currency in the order scanCurrency expects; the
// rate comes without trailing zeros ("92.5", not "92.500000").
const selectCurrency = `
	SELECT code, name, trim_scale(rate_to_rub)::text, updated_at, updated_by
	FROM subtrack.currencies`

func scanCurrency(row pgx.Row) (domain.Currency, error) {
	var c domain.Currency
	err := row.Scan(&c.Code, &c.Name, &c.RateToRUB, &c.UpdatedAt, &c.UpdatedBy)
	return c, err
}

// List returns every currency, the rouble first, then by code.
//
// Errors:
//   - any db error wrapped as "postgres: list currencies ...".
func (s *Store) List(ctx context.Context) ([]domain.Currency, error) {
	rows, err := pg.Conn(ctx, s.pool).Query(ctx, selectCurrency+` ORDER BY code <> 'RUB', code`)
	if err != nil {
		return nil, pg.Translate(ctx, "list currencies", err)
	}
	defer rows.Close()
	var out []domain.Currency
	for rows.Next() {
		c, err := scanCurrency(rows)
		if err != nil {
			return nil, pg.Translate(ctx, "list currencies scan", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, pg.Translate(ctx, "list currencies rows", err)
	}
	return out, nil
}

// GetByCode returns one currency.
//
// Errors:
//   - storage.ErrNotFound when no currency has that code.
//   - any other db error wrapped as "postgres: get currency: ...".
func (s *Store) GetByCode(ctx context.Context, code string) (domain.Currency, error) {
	c, err := scanCurrency(pg.Conn(ctx, s.pool).QueryRow(ctx, selectCurrency+` WHERE code = $1`, code))
	if err != nil {
		return domain.Currency{}, pg.Translate(ctx, "get currency", err)
	}
	return c, nil
}

// GetForUpdate returns one currency and locks it until the surrounding
// transaction ends, so the audit trail names the rate that was really
// replaced. Call it inside pg.TxRunner.InTx.
//
// The lock is FOR NO KEY UPDATE, the one an UPDATE of non-key columns takes:
// unlike FOR UPDATE it does not hold up subscriptions and payments being
// written in this currency meanwhile (their foreign keys take KEY SHARE).
//
// Errors: as GetByCode.
func (s *Store) GetForUpdate(ctx context.Context, code string) (domain.Currency, error) {
	c, err := scanCurrency(pg.Conn(ctx, s.pool).QueryRow(ctx, selectCurrency+` WHERE code = $1 FOR NO KEY UPDATE`, code))
	if err != nil {
		return domain.Currency{}, pg.Translate(ctx, "get currency for update", err)
	}
	return c, nil
}

// Create inserts a currency on behalf of actorID and returns the stored row.
//
// Errors:
//   - *storage.ConstraintError "currencies_pkey" when the code is taken.
//   - *storage.CheckError for a value the schema refuses.
//   - any other db error wrapped as "postgres: create currency: ...".
func (s *Store) Create(ctx context.Context, c domain.Currency, actorID int64) (domain.Currency, error) {
	out, err := scanCurrency(pg.Conn(ctx, s.pool).QueryRow(ctx, `
		INSERT INTO subtrack.currencies (code, name, rate_to_rub, updated_by)
		VALUES ($1, $2, CAST($3::text AS NUMERIC), $4)
		RETURNING code, name, trim_scale(rate_to_rub)::text, updated_at, updated_by`,
		c.Code, c.Name, c.RateToRUB, actorID))
	if err != nil {
		return domain.Currency{}, pg.Translate(ctx, "create currency", err)
	}
	return out, nil
}

// Update writes the name and the rate of c on behalf of actorID and returns
// the row afterwards.
//
// Errors:
//   - storage.ErrNotFound when no currency has that code.
//   - *storage.CheckError for a value the schema refuses (a rate of RUB
//     other than 1, a rate not above 0).
//   - any other db error wrapped as "postgres: update currency: ...".
func (s *Store) Update(ctx context.Context, c domain.Currency, actorID int64) (domain.Currency, error) {
	out, err := scanCurrency(pg.Conn(ctx, s.pool).QueryRow(ctx, `
		UPDATE subtrack.currencies
		SET name = $2, rate_to_rub = CAST($3::text AS NUMERIC), updated_by = $4, updated_at = NOW()
		WHERE code = $1
		RETURNING code, name, trim_scale(rate_to_rub)::text, updated_at, updated_by`,
		c.Code, c.Name, c.RateToRUB, actorID))
	if err != nil {
		return domain.Currency{}, pg.Translate(ctx, "update currency", err)
	}
	return out, nil
}

// Delete removes a currency nothing refers to.
//
// Errors:
//   - storage.ErrNotFound when no currency has that code.
//   - errInUse when subscriptions, requests or payments use it.
//   - any other db error wrapped as "postgres: delete currency: ...".
func (s *Store) Delete(ctx context.Context, code string) error {
	tag, err := pg.Conn(ctx, s.pool).Exec(ctx, `DELETE FROM subtrack.currencies WHERE code = $1`, code)
	if err != nil {
		err = pg.Translate(ctx, "delete currency", err)
		// On a DELETE the only foreign key that can fire is a referencing
		// row: Translate reports it as a wrapped ErrNotFound.
		if errors.Is(err, storage.ErrNotFound) {
			return errInUse
		}
		return err
	}
	if tag.RowsAffected() == 0 {
		return storage.ErrNotFound
	}
	return nil
}
