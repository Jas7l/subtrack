package pg

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB is what both a pool and a transaction offer. Stores run their SQL on
// Conn(ctx, pool) instead of the pool itself, so a use case that spans
// several stores (approving a request creates a subscription and hands out a
// licence) can run them all in one transaction.
type DB interface {
	Begin(ctx context.Context) (pgx.Tx, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type txKey struct{}

// Conn returns the transaction InTx put in ctx, or the pool when there is
// none. A store that begins its own transaction on it gets a savepoint
// inside the outer one: its rollback undoes only its own work, its commit
// becomes final only when the outer transaction commits.
func Conn(ctx context.Context, pool *pgxpool.Pool) DB {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}
	return pool
}

// TxRunner runs functions in one transaction. The zero value is not usable;
// build it with NewTxRunner.
type TxRunner struct {
	pool *pgxpool.Pool
}

// NewTxRunner constructs a TxRunner over the given pool.
func NewTxRunner(pool *pgxpool.Pool) *TxRunner {
	return &TxRunner{pool: pool}
}

// InTx calls fn with a context carrying a new transaction (a savepoint when
// ctx already carries one) and commits it if fn returns nil; any error, or a
// panic, rolls it back. Every store call made with that context joins the
// transaction.
func (r *TxRunner) InTx(ctx context.Context, fn func(ctx context.Context) error) error {
	tx, err := Conn(ctx, r.pool).Begin(ctx)
	if err != nil {
		return Translate(ctx, "begin", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := fn(context.WithValue(ctx, txKey{}, tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return Translate(ctx, "commit", err)
	}
	return nil
}
