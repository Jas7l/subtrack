package pg

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"subtrack/internal/logger"
	"subtrack/internal/storage"
)

// Itoa is a small helper for building dynamic SQL placeholders ($1, $2, …).
func Itoa(n int) string { return strconv.Itoa(n) }

// likeEscaper backs EscapeLike: % and _ are wildcards in a LIKE pattern, and
// backslash is its default escape character.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// EscapeLike makes s match literally inside a LIKE / ILIKE pattern, so a
// search for "50%" does not match everything that starts with "50".
func EscapeLike(s string) string { return likeEscaper.Replace(s) }

// SortDir renders the ORDER BY direction the list endpoints share: ASC when the
// caller asked for oldest-first, DESC otherwise.
//
// A direction cannot be a bind parameter, so it is interpolated into the
// statement. Taking a bool rather than a string is what makes that safe by
// construction - the only two strings this can ever produce are the two
// literals below.
func SortDir(ascending bool) string {
	if ascending {
		return "ASC"
	}
	return "DESC"
}

// NewPool opens a pgx connection pool from a Config and pings the database
// to fail fast on bad configuration or unreachable hosts.
//
// Parameters:
//   - ctx - used both for dialing and for the initial ping; the ping is
//     additionally bounded to 5 seconds to keep startup deterministic.
//   - c   - connection parameters (host, credentials, pool sizing).
//
// Returns:
//   - *pgxpool.Pool - a ready-to-use pool. The caller owns Close().
//
// Errors:
//   - "postgres: parse dsn: ..."  - c.DSN() is malformed.
//   - "postgres: open pool: ..."  - pgxpool failed to create the pool.
//   - "postgres: ping: ..."       - the database could not be reached within
//     5 seconds; the pool is closed before the error is returned.
func NewPool(ctx context.Context, c Config) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(c.DSN())
	if err != nil {
		return nil, fmt.Errorf("postgres: parse dsn: %w", err)
	}
	if c.MaxConns > 0 {
		cfg.MaxConns = c.MaxConns
	}
	cfg.MaxConnLifetime = c.MaxConnLifetime
	cfg.MaxConnIdleTime = c.MaxConnIdleTime

	// Every Query/QueryRow/Exec on this pool - including those inside a
	// transaction - is logged by Tracer (tracer.go) through the request-scoped
	// logger. No Store needs a logger of its own.
	cfg.ConnConfig.Tracer = Tracer{}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: open pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}
	return pool, nil
}

// Translate maps a raw pgx error into a storage sentinel where it makes sense,
// or wraps it with operation context otherwise.
//
// Parameters:
//   - op  - short label describing the operation (e.g. "create user"), used
//     in the log line and when the error is wrapped.
//   - err - error returned by pgx; may be nil.
//
// Mapping:
//   - nil                                    → nil.
//   - pgx.ErrNoRows                          → storage.ErrNotFound (bare).
//   - SQLSTATE 23505 (unique_violation)      → *storage.ConstraintError
//     (matches storage.ErrAlreadyExists via errors.Is, carries the constraint
//     name so services can tell which uniqueness rule fired).
//   - SQLSTATE 23503 (foreign_key_violation) → wrapped
//     *storage.ForeignKeyError (matches storage.ErrNotFound: "a referenced
//     row is missing" rather than "the target row is missing", and carries
//     the key's name).
//   - SQLSTATE 23514 (check_violation)       → *storage.CheckError carrying
//     the constraint name.
//   - anything else                          → `postgres: <op>: %w` of err,
//     preserving the original via errors.Is / errors.As.
func Translate(ctx context.Context, op string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return storage.ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation
			logger.Op(ctx, "postgres."+op).Warn("unique violation",
				"constraint", pgErr.ConstraintName, "detail", pgErr.Detail)
			return &storage.ConstraintError{Constraint: pgErr.ConstraintName}
		case "23503": // foreign_key_violation
			logger.Op(ctx, "postgres."+op).Warn("foreign key violation",
				"constraint", pgErr.ConstraintName, "detail", pgErr.Detail)
			return fmt.Errorf("postgres: %s: %w", op, &storage.ForeignKeyError{Constraint: pgErr.ConstraintName})
		case "23514": // check_violation
			logger.Op(ctx, "postgres."+op).Warn("check violation",
				"constraint", pgErr.ConstraintName, "detail", pgErr.Detail)
			return &storage.CheckError{Constraint: pgErr.ConstraintName}
		}
		logger.Op(ctx, "postgres."+op).Error("db error",
			"sqlstate", pgErr.Code, "message", pgErr.Message, "detail", pgErr.Detail, "constraint", pgErr.ConstraintName)
		return fmt.Errorf("postgres: %s: %w", op, err)
	}
	logger.Op(ctx, "postgres."+op).Error("db error", "err", err.Error())
	return fmt.Errorf("postgres: %s: %w", op, err)
}
