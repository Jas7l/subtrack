// Package analytics builds the reports of the dashboard: a summary, cost
// breakdowns, payment history, a budget forecast and savings
// recommendations. Every amount is in roubles at the current rate of its
// currency (amount * rate_to_rub); the math is exact (big.Rat), never float.
//
// "Active" subscriptions are active and expiring_soon ones; their monthly
// cost is the price of a period divided by its months (1, 3 or 12).
//
// Who sees what:
//   - the admin sees everything, for the whole company or one department
//     (department_id);
//   - a manager sees aggregated figures of their own department only -
//     summary, breakdowns by category and vendor, forecast - without payment
//     history, payment figures or recommendations;
//   - an employee sees nothing (the routes answer 403).
package analytics

import (
	"context"
	"fmt"
	"time"

	"subtrack/internal/apperr"
	"subtrack/internal/auth"
	"subtrack/internal/domain"
	"subtrack/internal/logger"
)

var (
	// ErrNoDepartment is returned to a manager who is in no department, so
	// has no figures to see. Mapped to 403.
	ErrNoDepartment = fmt.Errorf("%w: you are in no department", auth.ErrForbidden)

	// ErrBadRange is returned for a malformed or too long period. Mapped to
	// 400.
	ErrBadRange = apperr.New(apperr.ErrInvalidInput, "service: invalid input: bad period")
)

// Limits of the request parameters.
const (
	DefaultForecastMonths = 12
	MaxForecastMonths     = 36
	DefaultSpendingMonths = 12
	MaxSpendingMonths     = 60
	DefaultTopLimit       = 10
	MaxTopLimit           = 50
)

// repo is what the reports read; *Store satisfies it.
type repo interface {
	Subscriptions(ctx context.Context, deptID *int64) ([]sub, error)
	Departments(ctx context.Context) ([]dept, int, error)
	Spending(ctx context.Context, from, to time.Time, deptID *int64) ([]monthSpend, error)
	Payments(ctx context.Context, today time.Time, deptID *int64) (PaymentStats, error)
}

// Service builds the reports.
type Service struct {
	repo repo
	loc  *time.Location
	// now is time.Now; tests replace it.
	now func() time.Time
}

// New wires a Service. loc decides what "today" and "this month" are (the
// scheduler's zone, Europe/Moscow by default).
func New(r repo, loc *time.Location) *Service {
	return &Service{repo: r, loc: loc, now: time.Now}
}

// today is the calendar day in the company's zone, as midnight UTC.
func (s *Service) today() time.Time {
	y, m, d := s.now().In(s.loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// scope decides which department a report covers: the requested one (nil:
// the whole company) for the admin, always their own for a manager.
func scope(actor domain.User, requested *int64) (*int64, error) {
	switch actor.Role {
	case domain.RoleAdmin:
		return requested, nil
	case domain.RoleManager:
		if actor.DepartmentID == nil {
			return nil, ErrNoDepartment
		}
		return actor.DepartmentID, nil
	}
	return nil, fmt.Errorf("%w: analytics is for the admin and department heads", auth.ErrForbidden)
}

// Summary returns the dashboard's numbers for the scope. The admin's also
// carries payment figures and the number of recommendations with their
// yearly savings.
func (s *Service) Summary(ctx context.Context, actor domain.User, deptID *int64) (Summary, error) {
	log := logger.Op(ctx, "analytics.Service.Summary")
	dept, err := scope(actor, deptID)
	if err != nil {
		return Summary{}, err
	}
	subs, err := s.repo.Subscriptions(ctx, dept)
	if err != nil {
		return Summary{}, err
	}
	depts, users, err := s.repo.Departments(ctx)
	if err != nil {
		return Summary{}, err
	}
	if dept != nil {
		users = 0
		for _, d := range depts {
			if d.ID == *dept {
				users = d.Users
			}
		}
	}
	out := summarize(subs, users)
	if actor.Role == domain.RoleAdmin {
		p, err := s.repo.Payments(ctx, s.today(), dept)
		if err != nil {
			return Summary{}, err
		}
		recs := recommend(subs, s.now())
		n, savings := len(recs), money(totalSavings(recs))
		out.Payments, out.Recommendations, out.PotentialSavingsRUB = &p, &n, &savings
	}
	log.Debug("done", "department_id", logger.Val(dept), "active", out.Active)
	return out, nil
}

// ByCategory breaks the active subscriptions' monthly cost down by category.
func (s *Service) ByCategory(ctx context.Context, actor domain.User, deptID *int64) ([]Slice, error) {
	return s.breakdown(ctx, actor, deptID, func(x sub) string { return x.Category })
}

// ByVendor breaks the active subscriptions' monthly cost down by vendor.
func (s *Service) ByVendor(ctx context.Context, actor domain.User, deptID *int64) ([]Slice, error) {
	return s.breakdown(ctx, actor, deptID, func(x sub) string { return x.Vendor })
}

func (s *Service) breakdown(ctx context.Context, actor domain.User, deptID *int64, key func(sub) string) ([]Slice, error) {
	dept, err := scope(actor, deptID)
	if err != nil {
		return nil, err
	}
	subs, err := s.repo.Subscriptions(ctx, dept)
	if err != nil {
		return nil, err
	}
	return breakdown(subs, key), nil
}

// ByDepartment compares the departments (the admin only).
func (s *Service) ByDepartment(ctx context.Context) ([]DepartmentRow, error) {
	subs, err := s.repo.Subscriptions(ctx, nil)
	if err != nil {
		return nil, err
	}
	depts, _, err := s.repo.Departments(ctx)
	if err != nil {
		return nil, err
	}
	return byDepartment(subs, depts), nil
}

// Top returns the most expensive active subscriptions (the admin only);
// limit is clamped to 1..MaxTopLimit, 0 meaning DefaultTopLimit.
func (s *Service) Top(ctx context.Context, deptID *int64, limit int) ([]TopRow, error) {
	if limit <= 0 {
		limit = DefaultTopLimit
	}
	limit = min(limit, MaxTopLimit)
	subs, err := s.repo.Subscriptions(ctx, deptID)
	if err != nil {
		return nil, err
	}
	return top(subs, limit), nil
}

// SpendingReport is what was actually paid, month by month.
type SpendingReport struct {
	From, To time.Time // first days of the first and last months
	Months   []monthSpend
	TotalRUB string
}

// Spending sums paid payments by month (the admin only), every month from
// from to to present even without payments. Both are months (any day in
// them); nil to means this month, nil from means DefaultSpendingMonths back.
//
// Errors:
//   - ErrBadRange when from is after to or the period is longer than
//     MaxSpendingMonths.
//   - any datastore failure, propagated.
func (s *Service) Spending(ctx context.Context, deptID *int64, from, to *time.Time) (SpendingReport, error) {
	toM := monthOf(s.today())
	if to != nil {
		toM = monthOf(*to)
	}
	fromM := toM.AddDate(0, 1-DefaultSpendingMonths, 0)
	if from != nil {
		fromM = monthOf(*from)
	}
	n := (toM.Year()-fromM.Year())*12 + int(toM.Month()) - int(fromM.Month()) + 1
	if n < 1 || n > MaxSpendingMonths {
		return SpendingReport{}, fmt.Errorf("%w: from must not be after to, and the period at most %d months", ErrBadRange, MaxSpendingMonths)
	}
	rows, err := s.repo.Spending(ctx, fromM, toM, deptID)
	if err != nil {
		return SpendingReport{}, err
	}
	byMonth := map[time.Time]monthSpend{}
	for _, r := range rows {
		byMonth[monthOf(r.Month)] = r
	}
	out := SpendingReport{From: fromM, To: toM, Months: make([]monthSpend, 0, n)}
	total := zero()
	for i := range n {
		m := fromM.AddDate(0, i, 0)
		r, ok := byMonth[m]
		if !ok {
			r = monthSpend{Month: m, AmountRUB: "0.00"}
		}
		r.Month = m
		total = add(total, rat(r.AmountRUB))
		out.Months = append(out.Months, r)
	}
	out.TotalRUB = money(total)
	return out, nil
}

// Forecast projects the spending of the scope's active subscriptions over
// months calendar months from this one; months is clamped to
// 1..MaxForecastMonths, 0 meaning DefaultForecastMonths.
func (s *Service) Forecast(ctx context.Context, actor domain.User, deptID *int64, months int) (Forecast, error) {
	dept, err := scope(actor, deptID)
	if err != nil {
		return Forecast{}, err
	}
	if months <= 0 {
		months = DefaultForecastMonths
	}
	months = min(months, MaxForecastMonths)
	subs, err := s.repo.Subscriptions(ctx, dept)
	if err != nil {
		return Forecast{}, err
	}
	return forecast(subs, s.today(), months), nil
}

// Recommendations runs the recommendation engine (the admin only) and
// returns the advice with the sum of the yearly savings it could compute.
func (s *Service) Recommendations(ctx context.Context, deptID *int64) ([]Recommendation, string, error) {
	subs, err := s.repo.Subscriptions(ctx, deptID)
	if err != nil {
		return nil, "", err
	}
	recs := recommend(subs, s.now())
	return recs, money(totalSavings(recs)), nil
}

func monthOf(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}
