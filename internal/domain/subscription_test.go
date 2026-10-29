package domain

import (
	"slices"
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

// TestBillingPeriodNextChain steps one date several times: a day clamped at
// the end of a shorter month stays clamped.
func TestBillingPeriodNextChain(t *testing.T) {
	cases := []struct {
		from   string
		period BillingPeriod
		want   []string
	}{
		{"2026-01-29", BillingMonth, []string{"2026-02-28", "2026-03-28", "2026-04-28"}},
		{"2026-01-30", BillingMonth, []string{"2026-02-28", "2026-03-28"}},
		{"2026-01-31", BillingMonth, []string{"2026-02-28", "2026-03-28"}},
		{"2028-01-31", BillingMonth, []string{"2028-02-29", "2028-03-29"}},
		{"2026-03-31", BillingMonth, []string{"2026-04-30", "2026-05-30"}},
		{"2026-11-30", BillingMonth, []string{"2026-12-30", "2027-01-30", "2027-02-28"}},
		{"2026-12-31", BillingMonth, []string{"2027-01-31", "2027-02-28"}},
		{"2026-11-30", BillingQuarter, []string{"2027-02-28", "2027-05-28"}},
		{"2026-08-31", BillingQuarter, []string{"2026-11-30", "2027-02-28"}},
		{"2026-12-31", BillingQuarter, []string{"2027-03-31", "2027-06-30", "2027-09-30", "2027-12-30"}},
		{"2028-02-29", BillingYear, []string{"2029-02-28", "2030-02-28", "2031-02-28", "2032-02-28"}},
		{"2026-12-31", BillingYear, []string{"2027-12-31", "2028-12-31"}},
	}
	for _, c := range cases {
		d, _ := time.Parse(time.DateOnly, c.from)
		var got []string
		for range c.want {
			d = c.period.Next(d)
			got = append(got, d.Format(time.DateOnly))
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("%q from %s: %v, want %v", c.period, c.from, got, c.want)
		}
	}
}
