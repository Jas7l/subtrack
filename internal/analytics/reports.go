package analytics

import (
	"math/big"
	"sort"
	"strings"
	"time"

	"subtrack/internal/domain"
)

// The functions here are pure: they take the subscriptions of the scope and
// "today" and build a report, so the rules are tested without a database.

// Summary is the dashboard's row of numbers.
type Summary struct {
	Active       int // active + expiring_soon
	ExpiringSoon int
	Expired      int
	// MonthlyCostRUB and AnnualCostRUB price the active subscriptions.
	MonthlyCostRUB string
	AnnualCostRUB  string
	// Seats count the subscriptions with a seat limit (individual, group);
	// enterprise ones are unlimited and counted in Enterprise.
	SeatsTotal  int
	SeatsUsed   int
	SeatsFree   int
	Utilization *float64 // percent, nil without limited seats
	Enterprise  int
	// Licences is the number of licences handed out on active
	// subscriptions, enterprise included.
	Licences    int
	ActiveUsers int
	// CostPerUserRUB is the monthly cost per active user, nil without users.
	CostPerUserRUB *string
	// The admin only.
	Payments            *PaymentStats
	Recommendations     *int
	PotentialSavingsRUB *string
}

func summarize(subs []sub, users int) Summary {
	var out Summary
	monthly := zero()
	for _, s := range subs {
		switch s.Status {
		case domain.SubExpired:
			out.Expired++
			continue
		case domain.SubExpiringSoon:
			out.ExpiringSoon++
		}
		out.Active++
		monthly = add(monthly, s.monthlyRUB())
		out.Licences += s.SeatsUsed
		if s.SeatsTotal == nil {
			out.Enterprise++
			continue
		}
		out.SeatsTotal += *s.SeatsTotal
		out.SeatsUsed += s.SeatsUsed
	}
	out.SeatsFree = out.SeatsTotal - out.SeatsUsed
	out.Utilization = pct(out.SeatsUsed, out.SeatsTotal)
	out.MonthlyCostRUB = money(monthly)
	out.AnnualCostRUB = money(mulInt(monthly, 12))
	out.ActiveUsers = users
	if users > 0 {
		c := money(frac(monthly, 1, users))
		out.CostPerUserRUB = &c
	}
	return out
}

// Slice is one row of a breakdown: a category, a vendor.
type Slice struct {
	Key            string // "" for subscriptions without one
	Subscriptions  int
	MonthlyCostRUB string
	SharePct       float64 // of the scope's monthly cost
	monthly        *big.Rat
}

// breakdown groups the active subscriptions by key, case-insensitively
// (the first spelling met names the group), most expensive first.
func breakdown(subs []sub, key func(sub) string) []Slice {
	idx := map[string]int{}
	var out []Slice
	total := zero()
	for _, s := range subs {
		if !s.active() {
			continue
		}
		k := strings.TrimSpace(key(s))
		fold := strings.ToLower(k)
		i, ok := idx[fold]
		if !ok {
			i = len(out)
			idx[fold] = i
			out = append(out, Slice{Key: k, monthly: zero()})
		}
		out[i].Subscriptions++
		out[i].monthly = add(out[i].monthly, s.monthlyRUB())
		total = add(total, s.monthlyRUB())
	}
	for i := range out {
		out[i].MonthlyCostRUB = money(out[i].monthly)
		out[i].SharePct = sharePct(out[i].monthly, total)
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].monthly.Cmp(out[b].monthly) > 0 })
	return out
}

// DepartmentRow is one department in the breakdown by department. The row
// with a nil DepartmentID holds the company-wide subscriptions.
type DepartmentRow struct {
	DepartmentID   *int64
	Name           string
	Subscriptions  int
	MonthlyCostRUB string
	SeatsTotal     int
	SeatsUsed      int
	Utilization    *float64
	// ActiveUsers and CostPerUserRUB are nil for the company-wide row.
	ActiveUsers    *int
	CostPerUserRUB *string
	monthly        *big.Rat
}

// byDepartment lists every department (even without subscriptions) and,
// when there are any, the company-wide subscriptions; most expensive first.
func byDepartment(subs []sub, depts []dept) []DepartmentRow {
	idx := map[int64]int{}
	out := make([]DepartmentRow, 0, len(depts)+1)
	for _, d := range depts {
		idx[d.ID] = len(out)
		users := d.Users
		out = append(out, DepartmentRow{DepartmentID: &d.ID, Name: d.Name, ActiveUsers: &users, monthly: zero()})
	}
	common := -1
	for _, s := range subs {
		if !s.active() {
			continue
		}
		var i int
		if s.DepartmentID == nil {
			if common < 0 {
				common = len(out)
				out = append(out, DepartmentRow{Name: "Общие (без отдела)", monthly: zero()})
			}
			i = common
		} else {
			var ok bool
			if i, ok = idx[*s.DepartmentID]; !ok {
				continue // created after the department list was read
			}
		}
		r := &out[i]
		r.Subscriptions++
		r.monthly = add(r.monthly, s.monthlyRUB())
		if s.SeatsTotal != nil {
			r.SeatsTotal += *s.SeatsTotal
			r.SeatsUsed += s.SeatsUsed
		}
	}
	for i := range out {
		r := &out[i]
		r.MonthlyCostRUB = money(r.monthly)
		r.Utilization = pct(r.SeatsUsed, r.SeatsTotal)
		if r.ActiveUsers != nil && *r.ActiveUsers > 0 {
			c := money(frac(r.monthly, 1, *r.ActiveUsers))
			r.CostPerUserRUB = &c
		}
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].monthly.Cmp(out[b].monthly) > 0 })
	return out
}

// TopRow is one of the most expensive subscriptions.
type TopRow struct {
	ID             int64
	Name           string
	Vendor         string
	DepartmentName *string
	Status         domain.SubscriptionStatus
	MonthlyCostRUB string
	AnnualCostRUB  string
	SeatsTotal     *int
	SeatsUsed      int
	Utilization    *float64
	// CostPerLicenceRUB is the monthly cost per licence handed out, nil
	// when nobody holds one.
	CostPerLicenceRUB *string
}

// top returns the limit most expensive active subscriptions by monthly cost.
func top(subs []sub, limit int) []TopRow {
	act := make([]sub, 0, len(subs))
	for _, s := range subs {
		if s.active() {
			act = append(act, s)
		}
	}
	sort.SliceStable(act, func(a, b int) bool { return act[a].monthlyRUB().Cmp(act[b].monthlyRUB()) > 0 })
	act = act[:min(limit, len(act))]
	out := make([]TopRow, 0, len(act))
	for _, s := range act {
		r := TopRow{ID: s.ID, Name: s.Name, Vendor: s.Vendor, DepartmentName: s.DepartmentName, Status: s.Status,
			MonthlyCostRUB: money(s.monthlyRUB()), AnnualCostRUB: money(s.annualRUB()),
			SeatsTotal: s.SeatsTotal, SeatsUsed: s.SeatsUsed}
		if s.SeatsTotal != nil {
			r.Utilization = pct(s.SeatsUsed, *s.SeatsTotal)
		}
		if s.SeatsUsed > 0 {
			c := money(frac(s.monthlyRUB(), 1, s.SeatsUsed))
			r.CostPerLicenceRUB = &c
		}
		out = append(out, r)
	}
	return out
}

// ForecastMonth is the expected spending of one calendar month.
type ForecastMonth struct {
	Month time.Time // the first day
	// ScheduledRUB sums the payments due by next_payment_date and the
	// billing period; Payments counts them.
	ScheduledRUB string
	Payments     int
	// EstimatedRUB is the monthly cost of active subscriptions without a
	// next_payment_date (their schedule is unknown).
	EstimatedRUB string
	TotalRUB     string
}

// Forecast is the expected spending over the next months.
type Forecast struct {
	Months   []ForecastMonth
	TotalRUB string
	// RunRateRUB is what the same months would cost if every active
	// subscription were renewed: monthly cost × months. The difference to
	// TotalRUB is what ends with end dates.
	RunRateRUB string
	// Unscheduled counts the active subscriptions without a
	// next_payment_date, priced in EstimatedRUB.
	Unscheduled int
}

// forecast projects the active subscriptions over months calendar months
// starting with today's. A scheduled subscription pays on each
// next_payment_date stepped by its period, from today on and up to its end
// date. Dates already past are left out: the scheduler charges them as
// pending payments.
func forecast(subs []sub, today time.Time, months int) Forecast {
	first := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
	horizon := first.AddDate(0, months, 0) // exclusive
	sched := make([]*big.Rat, months)
	est := make([]*big.Rat, months)
	count := make([]int, months)
	for i := range months {
		sched[i], est[i] = zero(), zero()
	}
	monthIdx := func(d time.Time) int {
		return (d.Year()-first.Year())*12 + int(d.Month()) - int(first.Month())
	}

	var out Forecast
	runRate := zero()
	for _, s := range subs {
		if !s.active() {
			continue
		}
		runRate = add(runRate, s.monthlyRUB())
		if s.NextPaymentDate == nil {
			out.Unscheduled++
			for i := range months {
				m := first.AddDate(0, i, 0)
				if s.EndDate != nil && s.EndDate.Before(m) {
					break
				}
				est[i] = add(est[i], s.monthlyRUB())
			}
			continue
		}
		for d := *s.NextPaymentDate; d.Before(horizon); d = s.Period.Next(d) {
			if s.EndDate != nil && d.After(*s.EndDate) {
				break
			}
			if d.Before(today) {
				continue
			}
			i := monthIdx(d)
			sched[i] = add(sched[i], s.periodRUB())
			count[i]++
		}
	}
	total := zero()
	for i := range months {
		t := add(sched[i], est[i])
		total = add(total, t)
		out.Months = append(out.Months, ForecastMonth{Month: first.AddDate(0, i, 0), ScheduledRUB: money(sched[i]),
			Payments: count[i], EstimatedRUB: money(est[i]), TotalRUB: money(t)})
	}
	out.TotalRUB = money(total)
	out.RunRateRUB = money(mulInt(runRate, months))
	return out
}
