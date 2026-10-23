package audit

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"subtrack/internal/domain"
	"subtrack/internal/pg"
)

// Store reads and writes subtrack.audit_log. The pool is borrowed, not owned.
// Record runs on pg.Conn(ctx, pool), so it joins the transaction of the
// change it records.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore constructs a Store over the given pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Record appends one entry. Domains call it inside the transaction of the
// change (see pg.TxRunner), so a failure here rolls the change back too.
//
// Errors:
//   - any db error wrapped as "postgres: record audit: ...".
func (s *Store) Record(ctx context.Context, e domain.AuditEntry) error {
	details := e.Details
	if details == nil {
		details = map[string]any{}
	}
	_, err := pg.Conn(ctx, s.pool).Exec(ctx, `
		INSERT INTO subtrack.audit_log (actor_id, action, entity_type, entity_id, details)
		VALUES ($1, $2, $3, $4, $5)`,
		e.ActorID, e.Action, e.EntityType, e.EntityID, details)
	return pg.Translate(ctx, "record audit", err)
}

// List returns one page of entries matching f, newest first, plus the total
// number of matches.
//
// Errors:
//   - any db error wrapped as "postgres: count audit: ..." / "list audit ...".
func (s *Store) List(ctx context.Context, f domain.AuditFilter, limit, offset int) ([]domain.AuditEntry, int64, error) {
	conds := make([]string, 0, 6)
	args := make([]any, 0, 8)
	add := func(cond string, v any) {
		args = append(args, v)
		conds = append(conds, strings.ReplaceAll(cond, "?", "$"+pg.Itoa(len(args))))
	}
	if f.ActorID != nil {
		add("a.actor_id = ?", *f.ActorID)
	}
	if f.EntityType != "" {
		add("a.entity_type = ?", f.EntityType)
	}
	if f.EntityID != nil {
		add("a.entity_id = ?", *f.EntityID)
	}
	if f.Action != "" {
		add("a.action = ?", f.Action)
	}
	if f.From != nil {
		add("a.created_at >= ?", *f.From)
	}
	if f.To != nil {
		add("a.created_at < ?", *f.To)
	}
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}

	db := pg.Conn(ctx, s.pool)
	var total int64
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM subtrack.audit_log a`+where, args...).Scan(&total); err != nil {
		return nil, 0, pg.Translate(ctx, "count audit", err)
	}
	args = append(args, limit, offset)
	q := `SELECT a.id, a.actor_id, u.full_name, a.action, a.entity_type, a.entity_id, a.details, a.created_at
		FROM subtrack.audit_log a
		LEFT JOIN subtrack.users u ON u.id = a.actor_id` + where +
		` ORDER BY a.created_at DESC, a.id DESC LIMIT $` + pg.Itoa(len(args)-1) + ` OFFSET $` + pg.Itoa(len(args))
	rows, err := db.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, pg.Translate(ctx, "list audit", err)
	}
	defer rows.Close()

	out := make([]domain.AuditEntry, 0, limit)
	for rows.Next() {
		var e domain.AuditEntry
		if err := rows.Scan(&e.ID, &e.ActorID, &e.ActorName, &e.Action, &e.EntityType, &e.EntityID,
			&e.Details, &e.CreatedAt); err != nil {
			return nil, 0, pg.Translate(ctx, "list audit scan", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, pg.Translate(ctx, "list audit rows", err)
	}
	return out, total, nil
}
