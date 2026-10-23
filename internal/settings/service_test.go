package settings

import (
	"context"
	"errors"
	"testing"
	"time"

	"subtrack/internal/apperr"
	"subtrack/internal/domain"
	"subtrack/internal/testkit"
)

type fakeRepo struct {
	st domain.NotificationSettings
}

func (f *fakeRepo) Get(context.Context) (domain.NotificationSettings, error) { return f.st, nil }

func (f *fakeRepo) Update(_ context.Context, st domain.NotificationSettings, actorID int64) (domain.NotificationSettings, error) {
	st.UpdatedBy, st.UpdatedAt = &actorID, time.Now()
	f.st = st
	return st, nil
}

func TestUpdate(t *testing.T) {
	repo := &fakeRepo{st: domain.NotificationSettings{ExpiringDays: 7, PaymentDays: 3}}
	trail := &testkit.Audit{}
	svc := New(repo, &testkit.Tx{}, trail)
	admin := domain.User{ID: 1, Role: domain.RoleAdmin}

	out, err := svc.Update(context.Background(), admin, 14, 0)
	if err != nil {
		t.Fatal(err)
	}
	if out.ExpiringDays != 14 || out.PaymentDays != 0 || out.UpdatedBy == nil || *out.UpdatedBy != 1 {
		t.Errorf("Update = %+v", out)
	}
	e := trail.Last()
	if e.Action != "settings.update" || e.EntityType != "settings" || e.EntityID != nil || *e.ActorID != 1 ||
		e.Details["expiring_days"] != 14 || e.Details["payment_days"] != 0 {
		t.Errorf("audit = %+v", e)
	}
}

func TestUpdateRejectsOutOfRange(t *testing.T) {
	repo := &fakeRepo{st: domain.NotificationSettings{ExpiringDays: 7, PaymentDays: 3}}
	trail := &testkit.Audit{}
	svc := New(repo, &testkit.Tx{}, trail)
	for _, c := range [][2]int{{-1, 3}, {7, -1}, {366, 3}, {7, 366}} {
		_, err := svc.Update(context.Background(), domain.User{ID: 1}, c[0], c[1])
		if !errors.Is(err, ErrInvalidDays) || !errors.Is(err, apperr.ErrInvalidInput) {
			t.Errorf("Update(%d, %d) err = %v, want ErrInvalidDays", c[0], c[1], err)
		}
	}
	if repo.st.ExpiringDays != 7 || len(trail.Entries) != 0 {
		t.Errorf("a rejected update changed something: %+v, audit %d", repo.st, len(trail.Entries))
	}
}

func TestUpdateFailsWithAudit(t *testing.T) {
	trail := &testkit.Audit{Err: errors.New("audit down")}
	svc := New(&fakeRepo{}, &testkit.Tx{}, trail)
	if _, err := svc.Update(context.Background(), domain.User{ID: 1}, 7, 3); err == nil {
		t.Error("want the audit failure to fail the update")
	}
}
