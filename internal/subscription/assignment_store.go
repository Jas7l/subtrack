package subscription

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"subtrack/internal/domain"
	"subtrack/internal/pg"
	"subtrack/internal/storage"
)

// The service vets an assignment before calling Assign, but another request
// can block the user or close the subscription in between. Assign reports
// those races with these errors; the service maps them like its own checks.
var (
	errAssigneeGone     = errors.New("subscription store: assignee is missing or blocked")
	errSubscriptionGone = errors.New("subscription store: subscription is missing, deleted or closed")
)

// selectAssignment reads an assignment with the subscription's name and
// department, the holder's name, the names of whoever handed it out and
// revoked it, and the holder's release request that still waits (at most one,
// release_requests_pending_key), in the order scanAssignment expects.
const selectAssignment = `
	SELECT a.id, a.subscription_id, s.name, s.department_id, a.user_id, u.full_name,
	       a.status, a.assigned_by, ab.full_name, a.assigned_at, a.revoked_by, rb.full_name, a.revoked_at, rr.id
	FROM subtrack.subscription_assignments a
	JOIN subtrack.subscriptions s ON s.id = a.subscription_id
	JOIN subtrack.users u ON u.id = a.user_id
	LEFT JOIN subtrack.users ab ON ab.id = a.assigned_by
	LEFT JOIN subtrack.users rb ON rb.id = a.revoked_by
	LEFT JOIN subtrack.release_requests rr ON rr.assignment_id = a.id AND rr.status = 'pending'`

func scanAssignment(row pgx.Row) (domain.Assignment, error) {
	var a domain.Assignment
	var status string
	err := row.Scan(&a.ID, &a.SubscriptionID, &a.SubscriptionName, &a.SubscriptionDepartmentID,
		&a.UserID, &a.UserName, &status, &a.AssignedBy, &a.AssignedByName, &a.AssignedAt,
		&a.RevokedBy, &a.RevokedByName, &a.RevokedAt, &a.ReleaseRequestID)
	a.Status = domain.AssignmentStatus(status)
	return a, err
}

func getAssignment(ctx context.Context, q queryRower, id int64) (domain.Assignment, error) {
	return scanAssignment(q.QueryRow(ctx, selectAssignment+` WHERE a.id = $1`, id))
}

// GetAssignment returns one assignment, active or revoked.
//
// Errors:
//   - storage.ErrNotFound when no row has that id.
//   - any other db error wrapped as "postgres: get assignment: ...".
func (s *Store) GetAssignment(ctx context.Context, id int64) (domain.Assignment, error) {
	a, err := getAssignment(ctx, pg.Conn(ctx, s.pool), id)
	if err != nil {
		return domain.Assignment{}, pg.Translate(ctx, "get assignment", err)
	}
	return a, nil
}

// HasActiveAssignment reports whether userID holds an active licence of
// subID.
func (s *Store) HasActiveAssignment(ctx context.Context, subID, userID int64) (bool, error) {
	var ok bool
	err := pg.Conn(ctx, s.pool).QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM subtrack.subscription_assignments
		               WHERE subscription_id = $1 AND user_id = $2 AND status = 'active')`,
		subID, userID).Scan(&ok)
	if err != nil {
		return false, pg.Translate(ctx, "has active assignment", err)
	}
	return ok, nil
}

// ListAssignments returns one page of assignments matching f and visible
// under sc (nil: all), newest first, plus the total number of matches.
//
// Errors:
//   - any db error wrapped as "postgres: count assignments: ..." /
//     "list assignments ...".
func (s *Store) ListAssignments(ctx context.Context, f domain.AssignmentFilter, sc *domain.SubscriptionScope, limit, offset int) ([]domain.Assignment, int64, error) {
	var b whereBuilder
	b.scope(sc, "s.department_id", "a.user_id = ?")
	if f.SubscriptionID != nil {
		b.add("a.subscription_id = ?", *f.SubscriptionID)
	}
	if f.UserID != nil {
		b.add("a.user_id = ?", *f.UserID)
	}
	if f.Status != nil {
		b.add("a.status = ?", string(*f.Status))
	}
	where := b.sql()

	var total int64
	err := pg.Conn(ctx, s.pool).QueryRow(ctx, `
		SELECT COUNT(*) FROM subtrack.subscription_assignments a
		JOIN subtrack.subscriptions s ON s.id = a.subscription_id`+where, b.args...).Scan(&total)
	if err != nil {
		return nil, 0, pg.Translate(ctx, "count assignments", err)
	}

	args := append(b.args, limit, offset)
	q := selectAssignment + where +
		` ORDER BY a.assigned_at DESC, a.id DESC LIMIT $` + pg.Itoa(len(args)-1) + ` OFFSET $` + pg.Itoa(len(args))
	rows, err := pg.Conn(ctx, s.pool).Query(ctx, q, args...)
	if err != nil {
		return nil, 0, pg.Translate(ctx, "list assignments", err)
	}
	defer rows.Close()

	out := make([]domain.Assignment, 0, limit)
	for rows.Next() {
		a, err := scanAssignment(rows)
		if err != nil {
			return nil, 0, pg.Translate(ctx, "list assignments scan", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, pg.Translate(ctx, "list assignments rows", err)
	}
	return out, total, nil
}

// Assign gives userID a licence of subID on behalf of actorID and takes one
// seat, in one transaction. It re-checks under lock what the service checked
// before: the user is still active and the subscription still live and open.
//
// Errors:
//   - errAssigneeGone / errSubscriptionGone when that re-check fails.
//   - *storage.CheckError "subscriptions_seats_check" when no seat is free.
//   - *storage.ConstraintError "subscription_assignments_active_key" when
//     the user already holds a licence of it.
//   - any other db error wrapped as "postgres: assign ...".
func (s *Store) Assign(ctx context.Context, subID, userID, actorID int64) (domain.Assignment, error) {
	tx, err := pg.Conn(ctx, s.pool).Begin(ctx)
	if err != nil {
		return domain.Assignment{}, pg.Translate(ctx, "begin assign", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// FOR SHARE waits for a concurrent block of this user and then sees it.
	var one int
	err = tx.QueryRow(ctx,
		`SELECT 1 FROM subtrack.users WHERE id = $1 AND is_active FOR SHARE`, userID).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Assignment{}, errAssigneeGone
	}
	if err != nil {
		return domain.Assignment{}, pg.Translate(ctx, "assign lock user", err)
	}

	err = tx.QueryRow(ctx, `
		UPDATE subtrack.subscriptions SET seats_used = seats_used + 1, updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL AND status IN ('active', 'expiring_soon')
		RETURNING id`, subID).Scan(&subID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Assignment{}, errSubscriptionGone
	}
	if err != nil {
		return domain.Assignment{}, pg.Translate(ctx, "assign take seat", err)
	}

	var id int64
	err = tx.QueryRow(ctx, `
		INSERT INTO subtrack.subscription_assignments (subscription_id, user_id, assigned_by)
		VALUES ($1, $2, $3) RETURNING id`, subID, userID, actorID).Scan(&id)
	if err != nil {
		return domain.Assignment{}, pg.Translate(ctx, "assign", err)
	}
	out, err := getAssignment(ctx, tx, id)
	if err != nil {
		return domain.Assignment{}, pg.Translate(ctx, "assign reread", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Assignment{}, pg.Translate(ctx, "commit assign", err)
	}
	return out, nil
}

// Revoke ends an active licence on behalf of actorID and frees its seat, in
// one transaction.
//
// Errors:
//   - storage.ErrNotFound when no active assignment has that id (it does not
//     exist or was revoked meanwhile).
//   - any other db error wrapped as "postgres: revoke ...".
func (s *Store) Revoke(ctx context.Context, id, actorID int64) (domain.Assignment, error) {
	tx, err := pg.Conn(ctx, s.pool).Begin(ctx)
	if err != nil {
		return domain.Assignment{}, pg.Translate(ctx, "begin revoke", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Lock the subscription before the assignment, as every other write does.
	var subID int64
	err = tx.QueryRow(ctx, `
		SELECT s.id FROM subtrack.subscriptions s
		JOIN subtrack.subscription_assignments a ON a.subscription_id = s.id
		WHERE a.id = $1
		FOR UPDATE OF s`, id).Scan(&subID)
	if err != nil {
		return domain.Assignment{}, pg.Translate(ctx, "revoke lock subscription", err)
	}
	tag, err := tx.Exec(ctx, `
		UPDATE subtrack.subscription_assignments
		SET status = 'revoked', revoked_at = NOW(), revoked_by = $2
		WHERE id = $1 AND status = 'active'`, id, actorID)
	if err != nil {
		return domain.Assignment{}, pg.Translate(ctx, "revoke", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.Assignment{}, storage.ErrNotFound
	}
	_, err = tx.Exec(ctx, `
		UPDATE subtrack.subscriptions SET seats_used = seats_used - 1, updated_at = NOW()
		WHERE id = $1`, subID)
	if err != nil {
		return domain.Assignment{}, pg.Translate(ctx, "revoke free seat", err)
	}
	out, err := getAssignment(ctx, tx, id)
	if err != nil {
		return domain.Assignment{}, pg.Translate(ctx, "revoke reread", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Assignment{}, pg.Translate(ctx, "commit revoke", err)
	}
	return out, nil
}
