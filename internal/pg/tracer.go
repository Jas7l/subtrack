package pg

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"subtrack/internal/logger"
)

// This file gives the repository layer its voice.
//
// Every domain's Store talks to Postgres through the shared *pgxpool.Pool, and
// none of them logs. Adding a log line per method would mean editing all of
// them and would still drift the moment someone adds a query. pgx exposes a
// tracer hook instead - one
// implementation on the pool's ConnConfig sees every Query, QueryRow and Exec
// in the process, including the ones issued inside a transaction, so the
// repository layer becomes observable in one place and cannot fall out of sync
// with the SQL it describes.
//
// Two levels, chosen so the default production log stays readable:
//
//   - DEBUG - every statement that succeeded: the SQL, its bound arguments, how
//     long it took, and what the command tag reported. This is what
//     LOG_LEVEL=debug buys, and it is the difference between "internal error"
//     and "this exact SELECT failed with these exact arguments".
//   - ERROR - every statement that failed, at a level the default config emits,
//     carrying the same SQL and arguments. Translate (pool.go) already logs the
//     SQLSTATE and the operation label; what it cannot know is the statement
//     text or what was bound to it, because by then the query is gone. The two
//     lines share request_id, so an operator reading the ERROR from Translate
//     has the statement one grep away.
//
// pgx.ErrNoRows and a cancelled context are not failures: the first is how every
// "not found" in this codebase is spelled (Translate maps it to
// storage.ErrNotFound) and the second means the client hung up. Both log at
// DEBUG so a busy service does not page anyone over a 404.

// Log-line caps. Statements here are hand-written and short, but an unbounded
// argument list would push the useful fields off the end of the line.
const (
	maxSQLLog     = 4096 // one statement's text
	maxArgLog     = 512  // one bound argument
	maxAllArgsLog = 4096 // the whole argument list
)

// queryTraceKey addresses the per-query timing record that TraceQueryStart
// stashes for TraceQueryEnd. Unexported struct key, so nothing outside this
// file can collide with it.
type queryTraceKey struct{}

// queryTrace is what a query carries between the tracer's two callbacks: pgx
// hands the SQL and arguments to Start and the outcome to End, so the statement
// text has to be carried forward to be logged alongside its result.
type queryTrace struct {
	start time.Time
	sql   string
	args  []any
}

// Tracer implements pgx.QueryTracer and pgx.ConnectTracer, logging through the
// request-scoped logger in ctx so every statement line carries the request_id
// of the HTTP request that caused it. Installed on the pool by NewPool; the
// zero value is ready to use and it holds no state, so one value serves every
// connection.
type Tracer struct{}

// Compile-time proof that the hooks pgx looks for are actually satisfied - a
// typo in a method name would otherwise silently disable tracing, since pgx
// discovers these with a type assertion rather than a required interface.
var (
	_ pgx.QueryTracer   = Tracer{}
	_ pgx.ConnectTracer = Tracer{}
)

// TraceQueryStart records the statement and the clock so TraceQueryEnd can
// report both. It runs for every query whether or not DEBUG is enabled: the
// error path needs the statement text too, and errors are not predictable in
// advance.
func (Tracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, queryTraceKey{}, &queryTrace{
		start: time.Now(),
		sql:   data.SQL,
		args:  data.Args,
	})
}

// TraceQueryEnd emits the statement line: DEBUG when the query succeeded (or
// failed only in the "no rows"/"client left" senses), ERROR when it genuinely
// failed. A missing trace record means TraceQueryStart did not run for this
// query, which should be impossible; the line is skipped rather than logged
// without timing.
func (Tracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	t, ok := ctx.Value(queryTraceKey{}).(*queryTrace)
	if !ok {
		return
	}
	elapsed := time.Since(t.start)
	log := logger.FromContext(ctx)

	// A benign outcome on a logger that is not at DEBUG has nothing to say, and
	// this is the hot path - return before formatting anything.
	if isExpected(data.Err) && !log.Enabled(ctx, slog.LevelDebug) {
		return
	}

	attrs := []any{
		"op", "postgres.query",
		"sql", condenseSQL(t.sql),
		"args", formatArgs(t.args),
		"duration_ms", durationMS(elapsed),
	}
	if tag := data.CommandTag; tag.String() != "" {
		attrs = append(attrs, "command_tag", tag.String(), "rows_affected", tag.RowsAffected())
	}

	if data.Err == nil {
		log.Debug("sql", attrs...)
		return
	}

	attrs = append(attrs, "err", data.Err.Error())
	var pgErr *pgconn.PgError
	if errors.As(data.Err, &pgErr) {
		attrs = append(attrs,
			"sqlstate", pgErr.Code,
			"pg_message", pgErr.Message,
			"pg_detail", pgErr.Detail,
			"pg_hint", pgErr.Hint,
			"constraint", pgErr.ConstraintName,
			"pg_table", pgErr.TableName,
			"pg_column", pgErr.ColumnName,
		)
	}
	switch {
	case errors.Is(data.Err, pgx.ErrNoRows):
		// Not an error at this layer: every "not found" in the service layer is
		// this, arriving through Translate as storage.ErrNotFound.
		log.Debug("sql no rows", attrs...)
	case errors.Is(data.Err, context.Canceled):
		log.Debug("sql cancelled", attrs...)
	case isExpected(data.Err):
		log.Debug("sql constraint", attrs...)
	default:
		log.Error("sql failed", attrs...)
	}
}

// isExpected reports whether a query error is one this codebase treats as a
// business outcome rather than a fault, and so must not be logged at ERROR.
//
// The two SQLSTATEs are exactly the ones Translate converts into sentinels:
// 23505 becomes storage.ErrAlreadyExists and surfaces as a 409, 23503 becomes
// storage.ErrNotFound and surfaces as a 404. Logging those at ERROR would put
// an alert-worthy line in the stream every time someone submits a duplicate
// email - the single fastest way to teach everyone to ignore the error
// log. Translate still emits its own WARN naming the constraint; this level
// only decides whether the statement text rides along at DEBUG or ERROR.
//
// pgx.ErrNoRows and a cancelled context are expected for the same reason: the
// first is how "not found" is spelled, the second means the client hung up.
func isExpected(err error) bool {
	if err == nil || errors.Is(err, pgx.ErrNoRows) || errors.Is(err, context.Canceled) {
		return true
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505", "23503": // unique_violation, foreign_key_violation
			return true
		}
	}
	return false
}

// TraceConnectStart is the no-op half of the connect hook; the interesting
// fields (outcome, elapsed time) are only known at the end, and pgx passes the
// same ctx to both.
func (Tracer) TraceConnectStart(ctx context.Context, _ pgx.TraceConnectStartData) context.Context {
	return context.WithValue(ctx, connectTraceKey{}, time.Now())
}

// connectTraceKey addresses the connect start time, kept separate from the
// query trace so a query issued on a freshly-opened connection cannot overwrite
// either record.
type connectTraceKey struct{}

// TraceConnectEnd logs pool connection establishment: DEBUG on success, ERROR
// on failure. Connection-level failures (bad credentials, exhausted server-side
// slots, TLS refusal) otherwise surface only as an opaque error on whichever
// request happened to need a new connection.
func (Tracer) TraceConnectEnd(ctx context.Context, data pgx.TraceConnectEndData) {
	log := logger.FromContext(ctx)
	attrs := []any{"op", "postgres.connect"}
	if start, ok := ctx.Value(connectTraceKey{}).(time.Time); ok {
		attrs = append(attrs, "duration_ms", durationMS(time.Since(start)))
	}
	if data.Err != nil {
		log.Error("postgres connect failed", append(attrs, "err", data.Err.Error())...)
		return
	}
	if !log.Enabled(ctx, slog.LevelDebug) {
		return
	}
	if data.Conn != nil {
		if pid := data.Conn.PgConn().PID(); pid != 0 {
			attrs = append(attrs, "backend_pid", pid)
		}
	}
	log.Debug("postgres connected", attrs...)
}

// durationMS renders an elapsed time as fractional milliseconds. Whole
// milliseconds are too coarse here: most statements in this service finish well
// under one, and a log full of `duration_ms: 0` cannot distinguish a fast query
// from a slow one.
func durationMS(d time.Duration) float64 {
	return float64(d.Microseconds()) / 1000
}

// condenseSQL collapses a statement's newlines and indentation into single
// spaces so a multi-line const lands on one log line, then caps the result.
// Structured logs are read one record per line; a raw heredoc SQL string turns
// a JSON log into something no aggregator will group.
func condenseSQL(sql string) string {
	return truncate(strings.Join(strings.Fields(sql), " "), maxSQLLog)
}

// formatArgs renders bound arguments as a bracketed, comma-separated list in
// $1..$n order, so a failing statement can be replayed by hand against the same
// values. Long individual values and long lists are both capped.
func formatArgs(args []any) string {
	if len(args) == 0 {
		return "[]"
	}
	var b strings.Builder
	b.WriteByte('[')
	for i, a := range args {
		if i > 0 {
			b.WriteString(", ")
		}
		if b.Len() >= maxAllArgsLog {
			fmt.Fprintf(&b, "…+%d more", len(args)-i)
			break
		}
		b.WriteString(formatArg(a))
	}
	b.WriteByte(']')
	return b.String()
}

// formatArg renders one bound argument. The type switch exists because the
// default %v is wrong or unreadable for the shapes this codebase actually
// binds: []byte (jsonb columns) prints as a list of byte numbers, and a nil pointer prints as an address-like <nil> with no
// hint of which optional column it belongs to.
func formatArg(a any) string {
	switch v := a.(type) {
	case nil:
		return "NULL"
	case []byte:
		return truncate(string(v), maxArgLog)
	case string:
		return truncate(v, maxArgLog)
	case time.Time:
		return v.Format(time.RFC3339Nano)
	case *time.Time:
		if v == nil {
			return "NULL"
		}
		return v.Format(time.RFC3339Nano)
	case *int64:
		if v == nil {
			return "NULL"
		}
		return fmt.Sprintf("%d", *v)
	case *string:
		if v == nil {
			return "NULL"
		}
		return truncate(*v, maxArgLog)
	default:
		return truncate(fmt.Sprintf("%v", v), maxArgLog)
	}
}

// truncate caps s at n bytes, marking any loss so a reader can tell a value
// that ends from one that was cut. Slicing on a byte boundary can split a
// multi-byte rune; that is acceptable in a log line and avoids scanning every
// argument for rune boundaries on the hot path.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…[truncated]"
}
