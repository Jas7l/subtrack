// Package department manages the company's departments and who heads them.
// The admin creates, renames and deletes departments and assigns heads; a
// manager or employee sees only their own department.
//
// A head must be an active manager; assigning one moves them into the
// department. The full invariant is described on domain.Department.
package department

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"subtrack/internal/apperr"
	"subtrack/internal/domain"
	"subtrack/internal/logger"
	"subtrack/internal/paging"
	"subtrack/internal/storage"
)

var (
	// ErrDepartmentNotFound is returned when the department does not exist or
	// is not the caller's own. Mapped to 404.
	ErrDepartmentNotFound = errors.New("service: department not found")

	// ErrNameTaken is returned when another department has the same name,
	// compared case-insensitively. Mapped to 409.
	ErrNameTaken = errors.New("service: department name is already taken")

	// ErrHeadTaken is returned when the proposed head already heads another
	// department (unique index departments_head_id_key). Mapped to 409.
	ErrHeadTaken = errors.New("service: this user already heads another department")

	// ErrInvalidHead is returned when the proposed head does not exist, is
	// blocked, or is not a manager. Mapped to 400.
	ErrInvalidHead = errors.New("service: invalid input: head must be an active user with the manager role")

	// ErrDepartmentNotEmpty is returned by Delete while the department still
	// has active people or live subscriptions. Mapped to 409.
	ErrDepartmentNotEmpty = errors.New("service: department is not empty")
)

// repo is the slice of departments persistence the service needs; *Store
// satisfies it.
type repo interface {
	GetByID(ctx context.Context, id int64) (domain.Department, error)
	List(ctx context.Context, query string, limit, offset int) ([]domain.Department, int64, error)
	Create(ctx context.Context, d domain.Department) (domain.Department, error)
	Update(ctx context.Context, id int64, p domain.DepartmentPatch) (domain.Department, error)
	Delete(ctx context.Context, id int64) (users, subscriptions int64, err error)
}

// users is what the service needs to vet a head; *user.Store satisfies it
// structurally.
type users interface {
	GetByID(ctx context.Context, id int64) (domain.User, error)
}

// transactor runs fn in one transaction; *pg.TxRunner satisfies it.
type transactor interface {
	InTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// auditor records a change in the audit trail; *audit.Store satisfies it.
// It is called inside the change's transaction.
type auditor interface {
	Record(ctx context.Context, e domain.AuditEntry) error
}

// Service implements the departments use cases. Every write runs in a
// transaction together with its audit record.
type Service struct {
	repo  repo
	users users
	tx    transactor
	audit auditor
}

// New wires a Service over the departments repo, the users reader, the
// transaction runner and the audit trail.
func New(r repo, u users, tx transactor, a auditor) *Service {
	return &Service{repo: r, users: u, tx: tx, audit: a}
}

// CreateDepartmentInput carries the fields of a new department.
type CreateDepartmentInput struct {
	Name   string
	HeadID *int64
}

// ownsDepartment reports whether actor may read department id: the admin
// reads every department, everyone else only their own.
func ownsDepartment(actor domain.User, id int64) bool {
	return actor.Role == domain.RoleAdmin || (actor.DepartmentID != nil && *actor.DepartmentID == id)
}

// Get returns one department as actor is allowed to see it.
//
// Errors:
//   - ErrDepartmentNotFound when it does not exist or is not actor's own.
//   - any other datastore failure, propagated.
func (s *Service) Get(ctx context.Context, actor domain.User, id int64) (domain.Department, error) {
	logger.Op(ctx, "department.Service.Get").Debug("start", "id", id)
	if !ownsDepartment(actor, id) {
		return domain.Department{}, ErrDepartmentNotFound
	}
	d, err := s.repo.GetByID(ctx, id)
	return d, apperr.MapStorage(err, ErrDepartmentNotFound)
}

// List returns one page of departments visible to actor: all of them for the
// admin, at most their own one for anybody else.
func (s *Service) List(ctx context.Context, actor domain.User, query string, p paging.Params) ([]domain.Department, int64, error) {
	log := logger.Op(ctx, "department.Service.List")
	p.Normalize()
	if actor.Role != domain.RoleAdmin {
		if actor.DepartmentID == nil {
			return []domain.Department{}, 0, nil
		}
		d, err := s.repo.GetByID(ctx, *actor.DepartmentID)
		if errors.Is(err, storage.ErrNotFound) {
			return []domain.Department{}, 0, nil
		}
		if err != nil {
			return nil, 0, err
		}
		return []domain.Department{d}, 1, nil
	}

	items, total, err := s.repo.List(ctx, strings.TrimSpace(query), p.Limit, p.Offset)
	if err == nil {
		log.Debug("done", "count", len(items), "total", total, "limit", p.Limit, "offset", p.Offset)
	}
	return items, total, err
}

// Create validates the input and inserts a department, moving the head into
// it when one is given.
//
// Errors:
//   - apperr.ErrInvalidInput when the name is empty.
//   - ErrInvalidHead, ErrHeadTaken, ErrNameTaken as their names say.
//   - any other datastore failure, propagated.
func (s *Service) Create(ctx context.Context, actor domain.User, in CreateDepartmentInput) (domain.Department, error) {
	log := logger.Op(ctx, "department.Service.Create")
	in.Name = strings.TrimSpace(in.Name)
	log.Debug("start", "name", in.Name)

	if in.Name == "" {
		return domain.Department{}, fmt.Errorf("%w: missing required fields: name", apperr.ErrInvalidInput)
	}
	if err := s.checkHead(ctx, in.HeadID); err != nil {
		return domain.Department{}, err
	}
	var d domain.Department
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		var err error
		d, err = s.repo.Create(ctx, domain.Department{Name: in.Name, HeadID: in.HeadID})
		if err != nil {
			return err
		}
		return s.audit.Record(ctx, domain.NewAuditEntry(actor.ID, "department.create", "department", d.ID,
			map[string]any{"name": d.Name, "head_id": d.HeadID}))
	})
	if err != nil {
		return domain.Department{}, mapWriteErr(err)
	}
	log.Info("department created", "id", d.ID, "head_id", logger.Val(d.HeadID))
	return d, nil
}

// Update renames a department and/or changes its head. A new head is moved
// into the department; "head_id": null leaves it without one.
//
// Errors:
//   - apperr.ErrInvalidInput for an empty patch or an empty name.
//   - ErrDepartmentNotFound, ErrInvalidHead, ErrHeadTaken, ErrNameTaken.
//   - any other datastore failure, propagated.
func (s *Service) Update(ctx context.Context, actor domain.User, id int64, p domain.DepartmentPatch) (domain.Department, error) {
	log := logger.Op(ctx, "department.Service.Update")
	log.Debug("start", "id", id)

	if !p.HasAny() {
		return domain.Department{}, fmt.Errorf("%w: at least one field must be provided", apperr.ErrInvalidInput)
	}
	if p.Name != nil {
		name := strings.TrimSpace(*p.Name)
		if name == "" {
			return domain.Department{}, fmt.Errorf("%w: name must not be empty", apperr.ErrInvalidInput)
		}
		p.Name = &name
	}
	if p.HeadID.Set {
		if err := s.checkHead(ctx, p.HeadID.Value); err != nil {
			return domain.Department{}, err
		}
	}
	details := map[string]any{}
	if p.Name != nil {
		details["name"] = *p.Name
	}
	if p.HeadID.Set {
		details["head_id"] = p.HeadID.Value
	}
	var d domain.Department
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		var err error
		d, err = s.repo.Update(ctx, id, p)
		if err != nil {
			return err
		}
		return s.audit.Record(ctx, domain.NewAuditEntry(actor.ID, "department.update", "department", id, details))
	})
	if err != nil {
		return domain.Department{}, mapWriteErr(err)
	}
	log.Info("department updated", "id", d.ID, "head_id", logger.Val(d.HeadID))
	return d, nil
}

// Delete removes an empty department.
//
// Errors:
//   - ErrDepartmentNotFound when no department has that id.
//   - ErrDepartmentNotEmpty (wrapped, with the counts) while it still has
//     active people or live subscriptions.
//   - any other datastore failure, propagated.
func (s *Service) Delete(ctx context.Context, actor domain.User, id int64) error {
	log := logger.Op(ctx, "department.Service.Delete")
	var users, subs int64
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		// Read the name first: the record outlives the row.
		d, err := s.repo.GetByID(ctx, id)
		if err != nil {
			return err
		}
		users, subs, err = s.repo.Delete(ctx, id)
		if err != nil || users > 0 || subs > 0 {
			return err
		}
		return s.audit.Record(ctx, domain.NewAuditEntry(actor.ID, "department.delete", "department", id,
			map[string]any{"name": d.Name}))
	})
	if err != nil {
		return apperr.MapStorage(err, ErrDepartmentNotFound)
	}
	if users > 0 || subs > 0 {
		log.Info("delete refused", "id", id, "active_users", users, "subscriptions", subs)
		return fmt.Errorf("%w: it still has %d active users and %d subscriptions; move or block them first",
			ErrDepartmentNotEmpty, users, subs)
	}
	log.Info("department deleted", "id", id)
	return nil
}

// checkHead returns ErrInvalidHead unless id is nil or names an active
// manager.
func (s *Service) checkHead(ctx context.Context, id *int64) error {
	if id == nil {
		return nil
	}
	u, err := s.users.GetByID(ctx, *id)
	if errors.Is(err, storage.ErrNotFound) {
		return ErrInvalidHead
	}
	if err != nil {
		return err
	}
	if !u.IsActive || u.Role != domain.RoleManager {
		return ErrInvalidHead
	}
	return nil
}

// mapWriteErr translates the storage errors of Create/Update.
func mapWriteErr(err error) error {
	if errors.Is(err, errHeadGone) {
		return ErrInvalidHead
	}
	var ce *storage.ConstraintError
	if errors.As(err, &ce) {
		switch ce.Constraint {
		case "departments_name_key":
			return ErrNameTaken
		case "departments_head_id_key":
			return ErrHeadTaken
		}
	}
	return apperr.MapStorage(err, ErrDepartmentNotFound)
}
