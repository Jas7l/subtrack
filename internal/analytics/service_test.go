package analytics

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/xuri/excelize/v2"

	"subtrack/internal/apperr"
	"subtrack/internal/auth"
	"subtrack/internal/domain"
	"subtrack/internal/httpx"
)

// fakeRepo serves fixed subscriptions and records the scope it was asked for.
type fakeRepo struct {
	subs      []sub
	gotDept   []*int64
	spendFrom time.Time
	spendTo   time.Time
}

func (f *fakeRepo) Subscriptions(_ context.Context, deptID *int64) ([]sub, error) {
	f.gotDept = append(f.gotDept, deptID)
	var out []sub
	for _, s := range f.subs {
		if deptID == nil || (s.DepartmentID != nil && *s.DepartmentID == *deptID) {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakeRepo) Departments(context.Context) ([]dept, int, error) {
	return []dept{{ID: 1, Name: "Разработка", Users: 4}, {ID: 2, Name: "Продажи", Users: 6}}, 12, nil
}

func (f *fakeRepo) Spending(_ context.Context, from, to time.Time, _ *int64) ([]monthSpend, error) {
	f.spendFrom, f.spendTo = from, to
	return []monthSpend{{Month: day("2026-09-01"), AmountRUB: "1500.50", Payments: 2}}, nil
}

func (f *fakeRepo) Rates(context.Context) ([]domain.Currency, error) {
	return []domain.Currency{{Code: "RUB", Name: "Российский рубль", RateToRUB: "1"},
		{Code: "USD", Name: "Доллар США", RateToRUB: "92.5"}}, nil
}

func (f *fakeRepo) Payments(context.Context, time.Time, *int64) (PaymentStats, error) {
	return PaymentStats{PaidThisMonthRUB: "10.00", PendingCount: 1}, nil
}

var (
	admin    = domain.User{ID: 1, Role: domain.RoleAdmin}
	head     = domain.User{ID: 2, Role: domain.RoleManager, DepartmentID: i64(1)}
	lostHead = domain.User{ID: 3, Role: domain.RoleManager}
	employee = domain.User{ID: 4, Role: domain.RoleEmployee, DepartmentID: i64(1)}
)

func newService() (*Service, *fakeRepo) {
	repo := &fakeRepo{subs: []sub{
		mk(1, "1000", func(s *sub) { s.DepartmentID = i64(1); s.SeatsUsed = 2 }),
		mk(2, "500", func(s *sub) { s.DepartmentID = i64(2); s.SeatsUsed = 0 }),
	}}
	msk, _ := time.LoadLocation("Europe/Moscow")
	svc := New(repo, msk)
	// 2026-10-31 22:00 UTC is already November in Moscow.
	svc.now = func() time.Time { return time.Date(2026, 10, 31, 22, 0, 0, 0, time.UTC) }
	return svc, repo
}

func TestSummaryScope(t *testing.T) {
	ctx := context.Background()
	svc, repo := newService()

	s, err := svc.Summary(ctx, admin, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.Active != 2 || s.ActiveUsers != 12 || s.Payments == nil || *s.Recommendations != 2 || *s.PotentialSavingsRUB != "15600.00" {
		t.Errorf("admin summary = %+v", s)
	}
	if s, _ = svc.Summary(ctx, admin, i64(2)); s.Active != 1 || s.ActiveUsers != 6 {
		t.Errorf("admin, department 2 = %+v", s)
	}

	// A head asking for another department still gets their own, without
	// payments or recommendations.
	s, err = svc.Summary(ctx, head, i64(2))
	if err != nil {
		t.Fatal(err)
	}
	if got := repo.gotDept[len(repo.gotDept)-1]; got == nil || *got != 1 {
		t.Errorf("head scope = %v, want department 1", got)
	}
	if s.Active != 1 || s.ActiveUsers != 4 || s.Payments != nil || s.Recommendations != nil || s.PotentialSavingsRUB != nil {
		t.Errorf("head summary = %+v", s)
	}

	if _, err := svc.Summary(ctx, lostHead, nil); !errors.Is(err, ErrNoDepartment) || !errors.Is(err, auth.ErrForbidden) {
		t.Errorf("head without department: err = %v", err)
	}
	if _, err := svc.Forecast(ctx, employee, nil, 0); !errors.Is(err, auth.ErrForbidden) {
		t.Errorf("employee: err = %v", err)
	}
}

func TestForecastMonthsClamp(t *testing.T) {
	svc, _ := newService()
	f, _ := svc.Forecast(context.Background(), head, nil, 0)
	if len(f.Months) != DefaultForecastMonths || f.Months[0].Month.Format("2006-01") != "2026-11" {
		t.Errorf("default forecast: %d months from %s", len(f.Months), f.Months[0].Month)
	}
	if f, _ = svc.Forecast(context.Background(), admin, nil, 1000); len(f.Months) != MaxForecastMonths {
		t.Errorf("clamped forecast: %d months", len(f.Months))
	}
}

func TestSpending(t *testing.T) {
	ctx := context.Background()
	svc, repo := newService()
	rep, err := svc.Spending(ctx, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Today is November 1st in Moscow: the last 12 months are Dec 2025 - Nov 2026.
	if !repo.spendFrom.Equal(day("2025-12-01")) || !repo.spendTo.Equal(day("2026-11-01")) || len(rep.Months) != 12 {
		t.Errorf("default range %s..%s, %d months", repo.spendFrom, repo.spendTo, len(rep.Months))
	}
	if m := rep.Months[9]; m.Month.Format("2006-01") != "2026-09" || m.AmountRUB != "1500.50" || m.Payments != 2 {
		t.Errorf("September = %+v", m)
	}
	if rep.Months[0].AmountRUB != "0.00" || rep.TotalRUB != "1500.50" {
		t.Errorf("empty month %+v, total %s", rep.Months[0], rep.TotalRUB)
	}
	for _, c := range [][2]string{{"2026-10-01", "2026-09-01"}, {"2020-01-01", "2026-01-01"}} {
		from, to := day(c[0]), day(c[1])
		if _, err := svc.Spending(ctx, nil, &from, &to); !errors.Is(err, apperr.ErrInvalidInput) {
			t.Errorf("Spending(%s, %s): err = %v, want a bad range", c[0], c[1], err)
		}
	}
}

func TestTopLimit(t *testing.T) {
	svc, _ := newService()
	if rows, _ := svc.Top(context.Background(), nil, 1); len(rows) != 1 || rows[0].ID != 1 {
		t.Errorf("top 1 = %+v", rows)
	}
}

func TestHTTPAnalytics(t *testing.T) {
	svc, _ := newService()
	users := map[string]domain.User{"admin": admin, "head": head, "lost": lostHead, "employee": employee}
	r := mux.NewRouter()
	r.NotFoundHandler = http.HandlerFunc(httpx.RouteNotFound)
	protected := r.PathPrefix("/api/v1").Subrouter()
	protected.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(auth.WithUser(req.Context(), users[req.Header.Get("X-User")])))
		})
	})
	RegisterRoutes(protected, svc)

	tests := []struct {
		as, method, path string
		want             int
	}{
		{"employee", "GET", "/analytics/summary", 403},
		{"lost", "GET", "/analytics/summary", 403},
		{"head", "GET", "/analytics/summary", 200},
		{"head", "GET", "/analytics/by-category", 200},
		{"head", "GET", "/analytics/by-vendor", 200},
		{"head", "GET", "/analytics/forecast?months=6", 200},
		{"head", "GET", "/analytics/by-department", 403},
		{"head", "GET", "/analytics/top-subscriptions", 403},
		{"head", "GET", "/analytics/spending", 403},
		{"head", "GET", "/analytics/recommendations", 403},
		{"admin", "GET", "/analytics/summary?department_id=1", 200},
		{"admin", "GET", "/analytics/summary?department_id=x", 400},
		{"admin", "GET", "/analytics/by-department", 200},
		{"admin", "GET", "/analytics/top-subscriptions?limit=5", 200},
		{"admin", "GET", "/analytics/top-subscriptions?limit=-1", 400},
		{"admin", "GET", "/analytics/spending?from=2026-01&to=2026-10", 200},
		{"admin", "GET", "/analytics/spending?from=2026-13", 400},
		{"admin", "GET", "/analytics/spending?from=2026-10&to=2026-01", 400},
		{"admin", "GET", "/analytics/forecast?months=abc", 400},
		{"admin", "GET", "/analytics/recommendations", 200},
		{"admin", "POST", "/analytics/summary", 405},
		{"head", "GET", "/analytics/export", 403},
		{"admin", "GET", "/analytics/export?department_id=x", 400},
		{"admin", "GET", "/analytics/export", 200},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(tt.method, "/api/v1"+tt.path, nil)
		req.Header.Set("X-User", tt.as)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != tt.want {
			t.Errorf("%s %s %s: status %d, want %d (body %s)", tt.as, tt.method, tt.path, rec.Code, tt.want, rec.Body)
		}
	}
}

func TestExportWorkbook(t *testing.T) {
	svc, _ := newService()
	h := &handler{svc: svc}
	for _, c := range []struct {
		query  string
		sheets []string
	}{
		{"", []string{"Сводка", "Категории", "Вендоры", "Отделы", "Самые дорогие", "Оплачено по месяцам", "Прогноз", "Рекомендации", "Курсы"}},
		{"?department_id=1", []string{"Сводка", "Категории", "Вендоры", "Самые дорогие", "Оплачено по месяцам", "Прогноз", "Рекомендации", "Курсы"}},
	} {
		req := httptest.NewRequest("GET", "/api/v1/analytics/export"+c.query, nil)
		req = req.WithContext(auth.WithUser(req.Context(), admin))
		rec := httptest.NewRecorder()
		h.export(rec, req)
		if rec.Code != 200 {
			t.Fatalf("export%s: %d %s", c.query, rec.Code, rec.Body)
		}
		if cd := rec.Header().Get("Content-Disposition"); cd != `attachment; filename="subtrack-analytics-2026-11-01.xlsx"` {
			t.Errorf("Content-Disposition = %q", cd)
		}
		f, err := excelize.OpenReader(rec.Body)
		if err != nil {
			t.Fatal(err)
		}
		if got := f.GetSheetList(); !slices.Equal(got, c.sheets) {
			t.Errorf("export%s sheets = %v, want %v", c.query, got, c.sheets)
		}
		if v, _ := f.GetCellValue("Сводка", "A2"); v != "Активных подписок" {
			t.Errorf("summary A2 = %q", v)
		}
		rows, _ := f.GetRows("Рекомендации")
		if len(rows) < 2 || rows[len(rows)-1][0] != "Итого" {
			t.Errorf("recommendations sheet = %v", rows)
		}
		rows, _ = f.GetRows("Курсы")
		if len(rows) != 3 || rows[1][0] != "RUB" || rows[2][0] != "USD" || rows[2][2] != "92.5" {
			t.Errorf("rates sheet = %v", rows)
		}
	}
}
