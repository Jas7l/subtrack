package analytics

import (
	"testing"
	"time"

	"subtrack/internal/domain"
)

func day(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

func dayPtr(s string) *time.Time {
	t := day(s)
	return &t
}

func intPtr(n int) *int { return &n }

func i64(n int64) *int64 { return &n }

// mk builds an active group subscription paid monthly in roubles.
func mk(id int64, cost string, opts ...func(*sub)) sub {
	s := sub{ID: id, Name: "S" + string(rune('A'+id-1)), Type: domain.SubGroup, Period: domain.BillingMonth,
		Cost: rat(cost), Currency: "RUB", Rate: rat("1"), Status: domain.SubActive,
		StartDate: day("2026-01-01"), CreatedAt: day("2026-01-01"), SeatsTotal: intPtr(10), SeatsUsed: 10}
	for _, o := range opts {
		o(&s)
	}
	return s
}

func TestMoney(t *testing.T) {
	cases := map[string]string{"0": "0 ₽", "999.4": "999 ₽", "1000": "1 000 ₽", "120000": "120 000 ₽",
		"1234567.5": "1 234 568 ₽", "-5000": "-5 000 ₽"}
	for in, want := range cases {
		if got := rubText(rat(in)); got != want {
			t.Errorf("rubText(%s) = %q, want %q", in, got, want)
		}
	}
	if got := money(frac(rat("100"), 1, 3)); got != "33.33" {
		t.Errorf("100/3 = %s", got)
	}
	if p := pct(2, 3); p == nil || *p != 66.7 {
		t.Errorf("pct(2,3) = %v", p)
	}
	if pct(1, 0) != nil {
		t.Error("pct(1,0) should be nil")
	}
	if got := sharePct(rat("1"), rat("3")); got != 33.3 {
		t.Errorf("sharePct = %v", got)
	}
}

func TestSubPrices(t *testing.T) {
	s := mk(1, "300", func(s *sub) { s.Period, s.Currency, s.Rate = domain.BillingQuarter, "USD", rat("90.5") })
	if got := money(s.periodRUB()); got != "27150.00" {
		t.Errorf("period = %s", got)
	}
	if got := money(s.monthlyRUB()); got != "9050.00" {
		t.Errorf("monthly = %s", got)
	}
	if got := money(s.annualRUB()); got != "108600.00" {
		t.Errorf("annual = %s", got)
	}
}

func TestSummarize(t *testing.T) {
	subs := []sub{
		mk(1, "1000", func(s *sub) { s.SeatsTotal, s.SeatsUsed = intPtr(10), 6 }),
		mk(2, "1200", func(s *sub) {
			s.Period = domain.BillingYear
			s.Status = domain.SubExpiringSoon
			s.SeatsTotal, s.SeatsUsed = intPtr(1), 1
		}),
		mk(3, "5000", func(s *sub) { s.Type, s.SeatsTotal, s.SeatsUsed = domain.SubEnterprise, nil, 40 }),
		mk(4, "9999", func(s *sub) { s.Status = domain.SubExpired; s.SeatsUsed = 3 }),
	}
	got := summarize(subs, 50)
	if got.Active != 3 || got.ExpiringSoon != 1 || got.Expired != 1 || got.Enterprise != 1 {
		t.Errorf("counts = %+v", got)
	}
	// 1000 + 1200/12 + 5000 = 6100.
	if got.MonthlyCostRUB != "6100.00" || got.AnnualCostRUB != "73200.00" {
		t.Errorf("cost = %s / %s", got.MonthlyCostRUB, got.AnnualCostRUB)
	}
	if got.SeatsTotal != 11 || got.SeatsUsed != 7 || got.SeatsFree != 4 || *got.Utilization != 63.6 || got.Licences != 47 {
		t.Errorf("seats = %+v, utilization %v", got, *got.Utilization)
	}
	if got.CostPerUserRUB == nil || *got.CostPerUserRUB != "122.00" {
		t.Errorf("cost per user = %v", got.CostPerUserRUB)
	}
	if empty := summarize(nil, 0); empty.MonthlyCostRUB != "0.00" || empty.Utilization != nil || empty.CostPerUserRUB != nil {
		t.Errorf("empty = %+v", empty)
	}
}

func TestBreakdown(t *testing.T) {
	subs := []sub{
		mk(1, "100", func(s *sub) { s.Category = "Дизайн" }),
		mk(2, "300", func(s *sub) { s.Category = "Разработка" }),
		mk(3, "100", func(s *sub) { s.Category = " дизайн " }),
		mk(4, "500", func(s *sub) { s.Category = "" }),
		mk(5, "9000", func(s *sub) { s.Category = "Дизайн"; s.Status = domain.SubExpired }),
	}
	got := breakdown(subs, func(s sub) string { return s.Category })
	if len(got) != 3 {
		t.Fatalf("breakdown = %+v", got)
	}
	want := []struct {
		key   string
		n     int
		cost  string
		share float64
	}{{"", 1, "500.00", 50}, {"Разработка", 1, "300.00", 30}, {"Дизайн", 2, "200.00", 20}}
	for i, w := range want {
		g := got[i]
		if g.Key != w.key || g.Subscriptions != w.n || g.MonthlyCostRUB != w.cost || g.SharePct != w.share {
			t.Errorf("row %d = %+v, want %+v", i, g, w)
		}
	}
}

func TestByDepartment(t *testing.T) {
	depts := []dept{{ID: 1, Name: "Бухгалтерия", Users: 4}, {ID: 2, Name: "Разработка", Users: 10}, {ID: 3, Name: "Пустой", Users: 0}}
	subs := []sub{
		mk(1, "1000", func(s *sub) { s.DepartmentID = i64(2); s.SeatsUsed = 5 }),
		mk(2, "200", func(s *sub) { s.DepartmentID = i64(1) }),
		mk(3, "3000"),
		mk(4, "50", func(s *sub) { s.DepartmentID = i64(99) }),
	}
	got := byDepartment(subs, depts)
	if len(got) != 4 {
		t.Fatalf("rows = %+v", got)
	}
	if got[0].DepartmentID != nil || got[0].MonthlyCostRUB != "3000.00" || got[0].ActiveUsers != nil {
		t.Errorf("company-wide row = %+v", got[0])
	}
	if *got[1].DepartmentID != 2 || *got[1].Utilization != 50 || *got[1].CostPerUserRUB != "100.00" {
		t.Errorf("Разработка = %+v", got[1])
	}
	if last := got[3]; last.Name != "Пустой" || last.Subscriptions != 0 || last.CostPerUserRUB != nil || last.Utilization != nil {
		t.Errorf("empty department = %+v", last)
	}
}

func TestTop(t *testing.T) {
	subs := []sub{
		mk(1, "100"),
		mk(2, "12000", func(s *sub) { s.Period = domain.BillingYear; s.SeatsUsed = 4 }), // 1000 a month
		mk(3, "500", func(s *sub) { s.SeatsUsed = 0 }),
		mk(4, "99999", func(s *sub) { s.Status = domain.SubExpired }),
	}
	got := top(subs, 2)
	if len(got) != 2 || got[0].ID != 2 || got[1].ID != 3 {
		t.Fatalf("top = %+v", got)
	}
	if got[0].MonthlyCostRUB != "1000.00" || *got[0].CostPerLicenceRUB != "250.00" || *got[0].Utilization != 40 {
		t.Errorf("first = %+v", got[0])
	}
	if got[1].CostPerLicenceRUB != nil {
		t.Errorf("no licences: cost per licence = %v", *got[1].CostPerLicenceRUB)
	}
}

func TestForecast(t *testing.T) {
	today := day("2026-10-23")
	subs := []sub{
		// Monthly on the 5th: October's is past, then Nov, Dec, Jan.
		mk(1, "1000", func(s *sub) { s.NextPaymentDate = dayPtr("2026-11-05") }),
		// Quarterly on the 25th: Oct 25 (from today on), Jan 25.
		mk(2, "3000", func(s *sub) { s.Period = domain.BillingQuarter; s.NextPaymentDate = dayPtr("2026-10-25") }),
		// Ends mid-December: Nov and Dec only.
		mk(3, "200", func(s *sub) { s.NextPaymentDate = dayPtr("2026-11-10"); s.EndDate = dayPtr("2026-12-15") }),
		// A date not charged yet (scheduler runs later today) is left out.
		mk(4, "70", func(s *sub) { s.NextPaymentDate = dayPtr("2026-10-20") }),
		// No schedule: 600 a year → 50 a month, estimated, until it ends in November.
		mk(5, "600", func(s *sub) { s.Period = domain.BillingYear; s.EndDate = dayPtr("2026-11-30") }),
		mk(6, "99999", func(s *sub) { s.Status = domain.SubExpired; s.NextPaymentDate = dayPtr("2026-11-01") }),
	}
	f := forecast(subs, today, 4)
	want := []struct {
		month, sched string
		n            int
		est, total   string
	}{
		{"2026-10", "3000.00", 1, "50.00", "3050.00"},
		{"2026-11", "1270.00", 3, "50.00", "1320.00"},
		{"2026-12", "1270.00", 3, "0.00", "1270.00"},
		{"2027-01", "4070.00", 3, "0.00", "4070.00"},
	}
	if len(f.Months) != len(want) {
		t.Fatalf("months = %+v", f.Months)
	}
	for i, w := range want {
		m := f.Months[i]
		if m.Month.Format("2006-01") != w.month || m.ScheduledRUB != w.sched || m.Payments != w.n ||
			m.EstimatedRUB != w.est || m.TotalRUB != w.total {
			t.Errorf("month %d = %+v, want %+v", i, m, w)
		}
	}
	if f.TotalRUB != "9710.00" || f.Unscheduled != 1 {
		t.Errorf("total = %s, unscheduled %d", f.TotalRUB, f.Unscheduled)
	}
	// Monthly costs 1000 + 1000 + 200 + 70 + 50 = 2320, × 4.
	if f.RunRateRUB != "9280.00" {
		t.Errorf("run rate = %s", f.RunRateRUB)
	}
}
