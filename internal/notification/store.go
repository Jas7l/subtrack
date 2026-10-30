package notification

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"subtrack/internal/domain"
	"subtrack/internal/pg"
)

// Store reads and writes subtrack.notifications. The pool is borrowed, not
// owned. Every method runs on pg.Conn(ctx, pool), so notifications sent as
// part of a change are stored in its transaction - or not at all.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore constructs a Store over the given pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Send stores one notification for n.UserID. ID, IsRead and CreatedAt are
// ignored; an empty EntityType stores no entity.
//
// Errors:
//   - wrapped storage.ErrNotFound when the user does not exist.
//   - any other db error wrapped as "postgres: send notification: ...".
func (s *Store) Send(ctx context.Context, n domain.Notification) error {
	_, err := pg.Conn(ctx, s.pool).Exec(ctx, `
		INSERT INTO subtrack.notifications (user_id, type, title, body, entity_type, entity_id)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), NULLIF($6, 0))`,
		n.UserID, string(n.Type), n.Title, n.Body, string(n.EntityType), entityID(n))
	return pg.Translate(ctx, "send notification", err)
}

// SendToAdmins stores a copy of n for every active admin; n.UserID is
// ignored.
//
// Errors:
//   - any db error wrapped as "postgres: send notification to admins: ...".
func (s *Store) SendToAdmins(ctx context.Context, n domain.Notification) error {
	_, err := pg.Conn(ctx, s.pool).Exec(ctx, `
		INSERT INTO subtrack.notifications (user_id, type, title, body, entity_type, entity_id)
		SELECT id, $1, $2, $3, NULLIF($4, ''), NULLIF($5, 0) FROM subtrack.users WHERE role = 'admin' AND is_active`,
		string(n.Type), n.Title, n.Body, string(n.EntityType), entityID(n))
	return pg.Translate(ctx, "send notification to admins", err)
}

// SendToManagers stores a copy of n for every active manager of department
// deptID and returns how many were stored (0 when it has none); n.UserID is
// ignored.
//
// Errors:
//   - any db error wrapped as "postgres: send notification to managers: ...".
func (s *Store) SendToManagers(ctx context.Context, deptID int64, n domain.Notification) (int64, error) {
	tag, err := pg.Conn(ctx, s.pool).Exec(ctx, `
		INSERT INTO subtrack.notifications (user_id, type, title, body, entity_type, entity_id)
		SELECT id, $2, $3, $4, NULLIF($5, ''), NULLIF($6, 0) FROM subtrack.users
		WHERE role = 'manager' AND is_active AND department_id = $1`,
		deptID, string(n.Type), n.Title, n.Body, string(n.EntityType), entityID(n))
	if err != nil {
		return 0, pg.Translate(ctx, "send notification to managers", err)
	}
	return tag.RowsAffected(), nil
}

// entityID is n.EntityID, or 0 (stored as NULL) when n points nowhere, so
// the two columns are always set together.
func entityID(n domain.Notification) int64 {
	if n.EntityType == "" {
		return 0
	}
	return n.EntityID
}

// List returns one page of userID's notifications, newest first, plus the
// number that match and the number still unread.
//
// Errors:
//   - any db error wrapped as "postgres: count notifications: ..." /
//     "list notifications ...".
func (s *Store) List(ctx context.Context, userID int64, unreadOnly bool, limit, offset int) (items []domain.Notification, total, unread int64, err error) {
	db := pg.Conn(ctx, s.pool)
	err = db.QueryRow(ctx, `
		SELECT COUNT(*) FILTER (WHERE NOT $2::bool OR NOT is_read), COUNT(*) FILTER (WHERE NOT is_read)
		FROM subtrack.notifications WHERE user_id = $1`, userID, unreadOnly).Scan(&total, &unread)
	if err != nil {
		return nil, 0, 0, pg.Translate(ctx, "count notifications", err)
	}
	rows, err := db.Query(ctx, `
		SELECT id, user_id, type, title, body, entity_type, entity_id, is_read, created_at
		FROM subtrack.notifications
		WHERE user_id = $1 AND (NOT $2::bool OR NOT is_read)
		ORDER BY created_at DESC, id DESC LIMIT $3 OFFSET $4`, userID, unreadOnly, limit, offset)
	if err != nil {
		return nil, 0, 0, pg.Translate(ctx, "list notifications", err)
	}
	defer rows.Close()

	items = make([]domain.Notification, 0, limit)
	for rows.Next() {
		n, err := scanNotification(rows)
		if err != nil {
			return nil, 0, 0, pg.Translate(ctx, "list notifications scan", err)
		}
		items = append(items, n)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, 0, pg.Translate(ctx, "list notifications rows", err)
	}
	return items, total, unread, nil
}

func scanNotification(row pgx.Row) (domain.Notification, error) {
	var n domain.Notification
	var typ string
	var entityType *string
	var entityID *int64
	err := row.Scan(&n.ID, &n.UserID, &typ, &n.Title, &n.Body, &entityType, &entityID, &n.IsRead, &n.CreatedAt)
	n.Type = domain.NotificationType(typ)
	if entityType != nil && entityID != nil {
		n.EntityType = domain.NotificationEntity(*entityType)
		n.EntityID = *entityID
	}
	return n, err
}

// MarkRead marks one of userID's notifications read and returns it. Marking
// a read one again is fine.
//
// Errors:
//   - storage.ErrNotFound when userID has no notification with that id.
//   - any other db error wrapped as "postgres: mark notification read: ...".
func (s *Store) MarkRead(ctx context.Context, userID, id int64) (domain.Notification, error) {
	n, err := scanNotification(pg.Conn(ctx, s.pool).QueryRow(ctx, `
		UPDATE subtrack.notifications SET is_read = TRUE
		WHERE id = $1 AND user_id = $2
		RETURNING id, user_id, type, title, body, entity_type, entity_id, is_read, created_at`, id, userID))
	if err != nil {
		return domain.Notification{}, pg.Translate(ctx, "mark notification read", err)
	}
	return n, nil
}

// MarkAllRead marks every unread notification of userID read and returns how
// many there were.
func (s *Store) MarkAllRead(ctx context.Context, userID int64) (int64, error) {
	tag, err := pg.Conn(ctx, s.pool).Exec(ctx,
		`UPDATE subtrack.notifications SET is_read = TRUE WHERE user_id = $1 AND NOT is_read`, userID)
	if err != nil {
		return 0, pg.Translate(ctx, "mark all notifications read", err)
	}
	return tag.RowsAffected(), nil
}
