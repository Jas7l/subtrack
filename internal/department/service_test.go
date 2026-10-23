package department

import (
	"context"
	"errors"
	"strings"
	"testing"

	"subtrack/internal/apperr"
	"subtrack/internal/domain"
	"subtrack/internal/paging"
	"subtrack/internal/storage"
)

// fakeRepo is an in-memory departments table with the two unique rules
// (name case-insensitively, head_id) and per-department "still in use"
// counts for Delete. Moving the head into the department is SQL and not
// modelled here.
type fakeRepo struct {
	depts  map[int64]domain.Department
	inUse  map[int64][2]int64 // id → {active users, subscriptions}
	nextID int64
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{depts: map[int64]domain.Department{}, inUse: map[int64][2]int64{}, nextID: 1}
}

func (f *fakeRepo) GetByID(_ context.Context, id int64) (domain.Department, error) {
	d, ok := f.depts[id]
	if !ok {
		return domain.Department{}, storage.ErrNotFound
	}
	return d, nil
}

func (f *fakeRepo) List(_ context.Context, query string, limit, offset int) ([]domain.Department, int64, error) {
	var out []domain.Department
	for id := int64(1); id < f.nextID; id++ {
		if d, ok := f.depts[id]; ok && strings.Contains(strings.ToLower(d.Name), strings.ToLower(query)) {
			out = append(out, d)
		}
	}
	return out, int64(len(out)), nil
}

func (f *fakeRepo) check(d domain.Department) error {
	for _, o := range f.depts {
		if o.ID == d.ID {
			continue
		}
		if strings.EqualFold(o.Name, d.Name) {
			return &storage.ConstraintError{Constraint: "departments_name_key"}
		}
		if d.HeadID != nil && o.HeadID != nil && *o.HeadID == *d.HeadID {
			return &storage.ConstraintError{Constraint: "departments_head_id_key"}
		}
	}
	return nil
}

func (f *fakeRepo) Create(_ context.Context, d domain.Department) (domain.Department, error) {
	if err := f.check(d); err != nil {
		return domain.Department{}, err
	}
	d.ID = f.nextID
	f.nextID++
	f.depts[d.ID] = d
	return d, nil
}

func (f *fakeRepo) Update(_ context.Context, id int64, p domain.DepartmentPatch) (domain.Department, error) {
	d, ok := f.depts[id]
	if !ok {
		return domain.Department{}, storage.ErrNotFound
	}
	if p.Name != nil {
		d.Name = *p.Name
	}
	if p.HeadID.Set {
		d.HeadID = p.HeadID.Value
	}
	if err := f.check(d); err != nil {
		return domain.Department{}, err
	}
	f.depts[id] = d
	return d, nil
}

func (f *fakeRepo) Delete(_ context.Context, id int64) (int64, int64, error) {
	if _, ok := f.depts[id]; !ok {
		return 0, 0, storage.ErrNotFound
	}
	if n := f.inUse[id]; n[0] > 0 || n[1] > 0 {
		return n[0], n[1], nil
	}
	delete(f.depts, id)
	return 0, 0, nil
}

// fakeUsers is a fixed set of users the service vets heads against.
type fakeUsers map[int64]domain.User

func (f fakeUsers) GetByID(_ context.Context, id int64) (domain.User, error) {
	u, ok := f[id]
	if !ok {
		return domain.User{}, storage.ErrNotFound
	}
	return u, nil
}

func ptr[T any](v T) *T { return &v }

var (
	admin      = domain.User{ID: 1, Role: domain.RoleAdmin, IsActive: true}
	manager    = domain.User{ID: 2, Role: domain.RoleManager, IsActive: true}
	manager2   = domain.User{ID: 3, Role: domain.RoleManager, IsActive: true}
	blockedMgr = domain.User{ID: 4, Role: domain.RoleManager, IsActive: false}
	employee   = domain.User{ID: 5, Role: domain.RoleEmployee, IsActive: true}
)

func newTestService() (*Service, *fakeRepo) {
	repo := newFakeRepo()
	users := fakeUsers{}
	for _, u := range []domain.User{admin, manager, manager2, blockedMgr, employee} {
		users[u.ID] = u
	}
	return New(repo, users), repo
}

func TestCreateAndHead(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService()

	d, err := svc.Create(ctx, CreateDepartmentInput{Name: "  Разработка ", HeadID: &manager.ID})
	if err != nil {
		t.Fatal(err)
	}
	if d.Name != "Разработка" || d.HeadID == nil || *d.HeadID != manager.ID {
		t.Errorf("created %+v", d)
	}

	tests := []struct {
		name string
		in   CreateDepartmentInput
		want error
	}{
		{"empty name", CreateDepartmentInput{Name: " "}, apperr.ErrInvalidInput},
		{"duplicate name, other case", CreateDepartmentInput{Name: "РАЗРАБОТКА"}, ErrNameTaken},
		{"head heads another department", CreateDepartmentInput{Name: "Дизайн", HeadID: &manager.ID}, ErrHeadTaken},
		{"head is an employee", CreateDepartmentInput{Name: "Дизайн", HeadID: &employee.ID}, ErrInvalidHead},
		{"head is an admin", CreateDepartmentInput{Name: "Дизайн", HeadID: &admin.ID}, ErrInvalidHead},
		{"head is blocked", CreateDepartmentInput{Name: "Дизайн", HeadID: &blockedMgr.ID}, ErrInvalidHead},
		{"head does not exist", CreateDepartmentInput{Name: "Дизайн", HeadID: ptr(int64(99))}, ErrInvalidHead},
	}
	for _, tt := range tests {
		if _, err := svc.Create(ctx, tt.in); !errors.Is(err, tt.want) {
			t.Errorf("%s: err = %v, want %v", tt.name, err, tt.want)
		}
	}
	if _, err := svc.Create(ctx, CreateDepartmentInput{Name: "Бухгалтерия"}); err != nil {
		t.Errorf("department without head: %v", err)
	}
}

func TestUpdate(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService()
	dev, _ := svc.Create(ctx, CreateDepartmentInput{Name: "Разработка", HeadID: &manager.ID})
	design, _ := svc.Create(ctx, CreateDepartmentInput{Name: "Дизайн"})

	tests := []struct {
		name  string
		id    int64
		patch domain.DepartmentPatch
		want  error
	}{
		{"empty patch", dev.ID, domain.DepartmentPatch{}, apperr.ErrInvalidInput},
		{"blank name", dev.ID, domain.DepartmentPatch{Name: ptr(" ")}, apperr.ErrInvalidInput},
		{"name taken", design.ID, domain.DepartmentPatch{Name: ptr("разработка")}, ErrNameTaken},
		{"head taken", design.ID, domain.DepartmentPatch{HeadID: domain.IDPatch{Set: true, Value: &manager.ID}}, ErrHeadTaken},
		{"head invalid", design.ID, domain.DepartmentPatch{HeadID: domain.IDPatch{Set: true, Value: &employee.ID}}, ErrInvalidHead},
		{"missing department", 99, domain.DepartmentPatch{Name: ptr("X")}, ErrDepartmentNotFound},
	}
	for _, tt := range tests {
		if _, err := svc.Update(ctx, tt.id, tt.patch); !errors.Is(err, tt.want) {
			t.Errorf("%s: err = %v, want %v", tt.name, err, tt.want)
		}
	}

	// Re-assigning the current head to their own department is a no-op, not
	// a clash with itself.
	if _, err := svc.Update(ctx, dev.ID, domain.DepartmentPatch{HeadID: domain.IDPatch{Set: true, Value: &manager.ID}}); err != nil {
		t.Errorf("same head again: %v", err)
	}
	d, err := svc.Update(ctx, dev.ID, domain.DepartmentPatch{HeadID: domain.IDPatch{Set: true}})
	if err != nil || d.HeadID != nil {
		t.Errorf("clear head: %+v, %v", d, err)
	}
	// Now free, the manager can head the other department.
	if _, err := svc.Update(ctx, design.ID, domain.DepartmentPatch{HeadID: domain.IDPatch{Set: true, Value: &manager.ID}}); err != nil {
		t.Errorf("freed head moves: %v", err)
	}
}

func TestDelete(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestService()
	busy, _ := svc.Create(ctx, CreateDepartmentInput{Name: "Busy"})
	empty, _ := svc.Create(ctx, CreateDepartmentInput{Name: "Empty"})
	repo.inUse[busy.ID] = [2]int64{3, 1}

	if err := svc.Delete(ctx, busy.ID); !errors.Is(err, ErrDepartmentNotEmpty) {
		t.Errorf("busy: err = %v, want ErrDepartmentNotEmpty", err)
	}
	if err := svc.Delete(ctx, empty.ID); err != nil {
		t.Errorf("empty: %v", err)
	}
	if err := svc.Delete(ctx, empty.ID); !errors.Is(err, ErrDepartmentNotFound) {
		t.Errorf("deleted twice: err = %v, want ErrDepartmentNotFound", err)
	}
}

func TestReadScope(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService()
	dev, _ := svc.Create(ctx, CreateDepartmentInput{Name: "Разработка"})
	other, _ := svc.Create(ctx, CreateDepartmentInput{Name: "Дизайн"})
	member := domain.User{ID: 10, Role: domain.RoleEmployee, DepartmentID: &dev.ID}
	homeless := domain.User{ID: 11, Role: domain.RoleManager}

	if _, err := svc.Get(ctx, admin, other.ID); err != nil {
		t.Errorf("admin get: %v", err)
	}
	if _, err := svc.Get(ctx, member, dev.ID); err != nil {
		t.Errorf("member gets own department: %v", err)
	}
	if _, err := svc.Get(ctx, member, other.ID); !errors.Is(err, ErrDepartmentNotFound) {
		t.Errorf("member gets other department: err = %v", err)
	}

	items, total, _ := svc.List(ctx, admin, "", paging.Params{})
	if total != 2 || len(items) != 2 {
		t.Errorf("admin list: %d items, total %d", len(items), total)
	}
	items, total, _ = svc.List(ctx, member, "", paging.Params{})
	if total != 1 || items[0].ID != dev.ID {
		t.Errorf("member list: %+v", items)
	}
	if _, total, _ := svc.List(ctx, homeless, "", paging.Params{}); total != 0 {
		t.Errorf("no department: total %d, want 0", total)
	}
}
