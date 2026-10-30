package subscription

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"subtrack/internal/domain"
	"subtrack/internal/pg"
	"subtrack/internal/storage"
)

// errLicenceGone is CreateRelease's re-check under lock failing: the licence
// was revoked (or is not the requester's) by the time the request is
// written. The service maps it like its own check.
var errLicenceGone = errors.New("subscription store: licence is no longer active")

// selectRelease reads a release request with its licence's subscription and
// that subscription's department now, and the names of the requester and
// of whoever decided it, in the order scanRelease expects. A deleted
// subscription still has its row, so its requests keep the name.
const selectRelease = `
	SELECT r.id, r.assignment_id, r.requester_id, u.full_name, r.reason, r.status,
	       r.decided_by, db.full_name, r.comment, r.decided_at, r.created_at, r.updated_at,
	       a.subscription_id, s.name, s.department_id, d.name
	FROM subtrack.release_requests r
	JOIN subtrack.subscription_assignments a ON a.id = r.assignment_id
	JOIN subtrack.subscriptions s ON s.id = a.subscription_id
	JOIN subtrack.users u ON u.id = r.requester_id
	LEFT JOIN subtrack.departments d ON d.id = s.department_id
	LEFT JOIN subtrack.users db ON db.id = r.decided_by`

func scanRelease(row pgx.Row) (domain.ReleaseRequest, error) {
	var r domain.ReleaseRequest
	var status string
	err := row.Scan(&r.ID, &r.AssignmentID, &r.RequesterID, &r.RequesterName, &r.Reason, &status,
		&r.DecidedBy, &r.DecidedByName, &r.Comment, &r.DecidedAt, &r.CreatedAt, &r.UpdatedAt,
		&r.SubscriptionID, &r.SubscriptionName, &r.DepartmentID, &r.DepartmentName)
	r.Status = domain.ReleaseStatus(status)
	return r, err
}

func getRelease(ctx context.Context, q queryRower, id int64) (domain.ReleaseRequest, error) {
	return scanRelease(q.QueryRow(ctx, selectRelease+` WHERE r.id = $1`, id))
}

// GetRelease returns one release request.
//
// Errors:
//   - storage.ErrNotFound when no row has that id.
//   - any other db error wrapped as "postgres: get release request: ...".
func (s *Store) GetRelease(ctx context.Context, id int64) (domain.ReleaseRequest, error) {
	r, err := getRelease(ctx, pg.Conn(ctx, s.pool), id)
	if err != nil {
		return domain.ReleaseRequest{}, pg.Translate(ctx, "get release request", err)
	}
	return r, nil
}

// LockRelease returns one release request with its subscription locked
// first and then the request itself, as a decision needs them: until the
// surrounding transaction ends nobody revokes the licence, moves the
// subscription to another department or decides the request meanwhile, and
// the request read is its latest version. Call it inside pg.TxRunner.InTx.
//
// Errors: as GetRelease.
func (s *Store) LockRelease(ctx context.Context, id int64) (domain.ReleaseRequest, error) {
	db := pg.Conn(ctx, s.pool)
	var subID int64
	err := db.QueryRow(ctx, `
		SELECT s.id FROM subtrack.subscriptions s
		JOIN subtrack.subscription_assignments a ON a.subscription_id = s.id
		JOIN subtrack.release_requests r ON r.assignment_id = a.id
		WHERE r.id = $1
		FOR UPDATE OF s`, id).Scan(&subID)
	if err != nil {
		return domain.ReleaseRequest{}, pg.Translate(ctx, "lock release request subscription", err)
	}
	// A statement of its own: a wait for the lock above re-checks only the
	// locked row, this one reads everything as it is now.
	r, err := scanRelease(db.QueryRow(ctx, selectRelease+` WHERE r.id = $1 FOR UPDATE OF r`, id))
	if err != nil {
		return domain.ReleaseRequest{}, pg.Translate(ctx, "lock release request", err)
	}
	return r, nil
}

// ListReleases returns one page of release requests matching f and visible
// under sc (nil: all; the department is the subscription's, the holder the
// requester), newest first, plus the total number of matches.
//
// Errors:
//   - any db error wrapped as "postgres: count release requests: ..." /
//     "list release requests ...".
func (s *Store) ListReleases(ctx context.Context, f domain.ReleaseFilter, sc *domain.SubscriptionScope, limit, offset int) ([]domain.ReleaseRequest, int64, error) {
	var b whereBuilder
	b.scope(sc, "s.department_id", "r.requester_id = ?")
	if f.Status != nil {
		b.add("r.status = ?", string(*f.Status))
	}
	if f.RequesterID != nil {
		b.add("r.requester_id = ?", *f.RequesterID)
	}
	if f.SubscriptionID != nil {
		b.add("a.subscription_id = ?", *f.SubscriptionID)
	}
	if f.DepartmentID != nil {
		b.add("s.department_id = ?", *f.DepartmentID)
	}
	if f.Unattended {
		b.conds = append(b.conds, `(s.department_id IS NULL OR NOT EXISTS (
			SELECT 1 FROM subtrack.users m
			WHERE m.department_id = s.department_id AND m.role = 'manager' AND m.is_active))`)
	}
	where := b.sql()

	db := pg.Conn(ctx, s.pool)
	var total int64
	err := db.QueryRow(ctx, `
		SELECT COUNT(*) FROM subtrack.release_requests r
		JOIN subtrack.subscription_assignments a ON a.id = r.assignment_id
		JOIN subtrack.subscriptions s ON s.id = a.subscription_id`+where, b.args...).Scan(&total)
	if err != nil {
		return nil, 0, pg.Translate(ctx, "count release requests", err)
	}

	args := append(b.args, limit, offset)
	q := selectRelease + where +
		` ORDER BY r.created_at DESC, r.id DESC LIMIT $` + pg.Itoa(len(args)-1) + ` OFFSET $` + pg.Itoa(len(args))
	rows, err := db.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, pg.Translate(ctx, "list release requests", err)
	}
	defer rows.Close()

	out := make([]domain.ReleaseRequest, 0, limit)
	for rows.Next() {
		r, err := scanRelease(rows)
		if err != nil {
			return nil, 0, pg.Translate(ctx, "list release requests scan", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, pg.Translate(ctx, "list release requests rows", err)
	}
	return out, total, nil
}

// CreateRelease files r - a pending request of r.RequesterID to give back
// the licence r.AssignmentID - and returns the stored row. Under a share
// lock on the subscription, which every revoke takes exclusively, it
// re-checks what the service checked before: the licence is still active
// and still the requester's. So a request never waits for a licence revoked
// meanwhile, and a revoke never misses a request filed meanwhile.
//
// Errors:
//   - errLicenceGone when that re-check fails.
//   - *storage.ConstraintError "release_requests_pending_key" when a request
//     for this licence already waits.
//   - any other db error wrapped as "postgres: create release request ...".
func (s *Store) CreateRelease(ctx context.Context, r domain.ReleaseRequest) (domain.ReleaseRequest, error) {
	tx, err := pg.Conn(ctx, s.pool).Begin(ctx)
	if err != nil {
		return domain.ReleaseRequest{}, pg.Translate(ctx, "begin create release request", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var subID int64
	err = tx.QueryRow(ctx, `
		SELECT s.id FROM subtrack.subscriptions s
		JOIN subtrack.subscription_assignments a ON a.subscription_id = s.id
		WHERE a.id = $1
		FOR SHARE OF s`, r.AssignmentID).Scan(&subID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ReleaseRequest{}, errLicenceGone
	}
	if err != nil {
		return domain.ReleaseRequest{}, pg.Translate(ctx, "create release request lock subscription", err)
	}
	// Read after the lock, in a statement of its own: a revoke that held the
	// subscription has committed by now and is seen.
	var active bool
	err = tx.QueryRow(ctx, `
		SELECT status = 'active' AND user_id = $2 FROM subtrack.subscription_assignments WHERE id = $1`,
		r.AssignmentID, r.RequesterID).Scan(&active)
	if err != nil {
		return domain.ReleaseRequest{}, pg.Translate(ctx, "create release request check licence", err)
	}
	if !active {
		return domain.ReleaseRequest{}, errLicenceGone
	}

	var id int64
	err = tx.QueryRow(ctx, `
		INSERT INTO subtrack.release_requests (assignment_id, requester_id, reason)
		VALUES ($1, $2, $3) RETURNING id`, r.AssignmentID, r.RequesterID, r.Reason).Scan(&id)
	if err != nil {
		return domain.ReleaseRequest{}, pg.Translate(ctx, "create release request", err)
	}
	out, err := getRelease(ctx, tx, id)
	if err != nil {
		return domain.ReleaseRequest{}, pg.Translate(ctx, "create release request reread", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.ReleaseRequest{}, pg.Translate(ctx, "commit create release request", err)
	}
	return out, nil
}

// DecideRelease records d on a request that is still pending: approved or
// rejected, by whom, with what comment. Revoking the licence on approval is
// the caller's, in the same transaction.
//
// Errors:
//   - storage.ErrNotFound when no pending request has that id.
//   - any other db error wrapped as "postgres: decide release request: ...".
func (s *Store) DecideRelease(ctx context.Context, id int64, d domain.ReleaseDecision) (domain.ReleaseRequest, error) {
	next := domain.ReleaseRejected
	if d.Approve {
		next = domain.ReleaseApproved
	}
	tag, err := pg.Conn(ctx, s.pool).Exec(ctx, `
		UPDATE subtrack.release_requests
		SET status = $2, decided_by = $3, comment = $4, decided_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND status = 'pending'`,
		id, string(next), d.ReviewerID, d.Comment)
	if err != nil {
		return domain.ReleaseRequest{}, pg.Translate(ctx, "decide release request", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ReleaseRequest{}, storage.ErrNotFound
	}
	return s.GetRelease(ctx, id)
}

// CancelRelease withdraws a pending request on behalf of its requester.
//
// Errors:
//   - storage.ErrNotFound when requesterID has no pending request with that
//     id.
//   - any other db error wrapped as "postgres: cancel release request: ...".
func (s *Store) CancelRelease(ctx context.Context, id, requesterID int64) (domain.ReleaseRequest, error) {
	tag, err := pg.Conn(ctx, s.pool).Exec(ctx, `
		UPDATE subtrack.release_requests SET status = 'cancelled', updated_at = NOW()
		WHERE id = $1 AND requester_id = $2 AND status = 'pending'`, id, requesterID)
	if err != nil {
		return domain.ReleaseRequest{}, pg.Translate(ctx, "cancel release request", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ReleaseRequest{}, storage.ErrNotFound
	}
	return s.GetRelease(ctx, id)
}

// CloseReleases approves, on behalf of actorID, the pending requests of
// subscription subID whose licence has been revoked another way (revoked
// directly, the subscription cancelled or deleted), and returns them as they
// stand afterwards. Call it in the transaction that revoked, which still
// holds the subscription: no request is filed or decided meanwhile.
//
// Errors:
//   - any db error wrapped as "postgres: close release requests ...".
func (s *Store) CloseReleases(ctx context.Context, subID, actorID int64) ([]domain.ReleaseRequest, error) {
	db := pg.Conn(ctx, s.pool)
	rows, err := db.Query(ctx, `
		UPDATE subtrack.release_requests r
		SET status = 'approved', decided_by = $2, decided_at = NOW(), updated_at = NOW()
		FROM subtrack.subscription_assignments a
		WHERE a.id = r.assignment_id AND a.subscription_id = $1 AND a.status = 'revoked' AND r.status = 'pending'
		RETURNING r.id`, subID, actorID)
	if err != nil {
		return nil, pg.Translate(ctx, "close release requests", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return nil, pg.Translate(ctx, "close release requests rows", err)
	}
	out := make([]domain.ReleaseRequest, 0, len(ids))
	for _, id := range ids {
		r, err := getRelease(ctx, db, id)
		if err != nil {
			return nil, pg.Translate(ctx, "close release requests reread", err)
		}
		out = append(out, r)
	}
	return out, nil
}
