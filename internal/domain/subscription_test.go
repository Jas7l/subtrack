package domain

import (
	"testing"
	"time"
)

func TestBillingPeriodNext(t *testing.T) {
	cases := []struct {
		from   string
		period BillingPeriod
		want   string
	}{
		{"2026-10-23", BillingMonth, "2026-11-23"},
		{"2026-12-15", BillingMonth, "2027-01-15"},
		{"2026-01-31", BillingMonth, "2026-02-28"},
		{"2028-01-31", BillingMonth, "2028-02-29"},
		{"2026-11-30", BillingQuarter, "2027-02-28"},
		{"2026-10-23", BillingQuarter, "2027-01-23"},
		{"2028-02-29", BillingYear, "2029-02-28"},
		{"2026-10-23", "", "2026-11-23"},
	}
	for _, c := range cases {
		from, _ := time.Parse(time.DateOnly, c.from)
		if got := c.period.Next(from).Format(time.DateOnly); got != c.want {
			t.Errorf("%q.Next(%s) = %s, want %s", c.period, c.from, got, c.want)
		}
	}
}
