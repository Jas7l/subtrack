package analytics

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"subtrack/internal/domain"
)

func TestRecommendations(t *testing.T) {
	now := day("2026-10-23")
	subs := []sub{
		// 30 of 50 seats, 300000 a year: 20 seats = 120000.
		mk(1, "25000", func(s *sub) { s.Name, s.Vendor, s.SeatsTotal, s.SeatsUsed = "Jira", "Atlassian", intPtr(50), 30 }),
		// Nobody holds it, older than 30 days: 12000 a year.
		mk(2, "1000", func(s *sub) { s.Name, s.SeatsUsed = "Miro", 0 }),
		// Nobody, but bought a week ago: no advice yet.
		mk(3, "1000", func(s *sub) { s.Name, s.SeatsUsed, s.CreatedAt = "Fresh", 0, day("2026-10-16") }),
		// Same vendor as Jira, different product, 8 of 10 used: only the vendor rule.
		mk(4, "5000", func(s *sub) { s.Name, s.Vendor, s.SeatsUsed = "Confluence", "atlassian ", 8 }),
		// Three individual Figma copies: individual_to_group, not duplicate_vendor.
		mk(5, "1500", func(s *sub) {
			s.Name, s.Vendor, s.Type, s.SeatsTotal, s.SeatsUsed = "Figma", "Figma", domain.SubIndividual, intPtr(1), 1
		}),
		mk(6, "1500", func(s *sub) {
			s.Name, s.Vendor, s.Type, s.SeatsTotal, s.SeatsUsed = "figma", "Figma", domain.SubIndividual, intPtr(1), 1
		}),
		mk(7, "1500", func(s *sub) {
			s.Name, s.Vendor, s.Type, s.SeatsTotal, s.SeatsUsed = "Figma", "Figma", domain.SubIndividual, intPtr(1), 1
		}),
		// Unlimited, 3 people.
		mk(8, "40000", func(s *sub) { s.Name, s.Type, s.SeatsTotal, s.SeatsUsed = "Office", domain.SubEnterprise, nil, 3 }),
		// Monthly for over a year, fully used.
		mk(9, "800", func(s *sub) { s.Name, s.StartDate = "Slack", day("2025-09-01") }),
		// Expired with licences out.
		mk(10, "100", func(s *sub) { s.Name, s.Status, s.SeatsUsed = "Old", domain.SubExpired, 2 }),
		// Exactly 80% used is fine.
		mk(11, "100", func(s *sub) { s.Name, s.SeatsUsed = "Fine", 8 }),
	}
	recs := recommend(subs, now)
	var got []string
	for _, r := range recs {
		got = append(got, string(r.Type)+":"+fmtIDs(r.SubscriptionIDs))
	}
	want := []string{
		"unused_seats:1", "no_holders:2", "duplicate_vendor:1,4", "individual_to_group:5,6,7",
		"enterprise_low_usage:8", "annual_billing:9", "expired_with_holders:10",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("recommendations =\n  %v\nwant\n  %v", got, want)
	}
	if r := recs[0]; *r.SavingsRUB != "120000.00" ||
		r.Text != "Занято 30 из 50 мест Jira. Сократите 20 мест — сэкономите 120 000 ₽/год." {
		t.Errorf("unused seats = %q, %s", r.Text, *r.SavingsRUB)
	}
	if r := recs[1]; *r.SavingsRUB != "12000.00" {
		t.Errorf("no holders saving = %s", *r.SavingsRUB)
	}
	for _, r := range recs[2:] {
		if r.SavingsRUB != nil {
			t.Errorf("%s should have no computed saving", r.Type)
		}
	}
	if total := money(totalSavings(recs)); total != "132000.00" {
		t.Errorf("total savings = %s", total)
	}
	if !strings.Contains(recs[3].Text, "Куплено 3 индивидуальных подписок «Figma»") {
		t.Errorf("individual text = %q", recs[3].Text)
	}
}

func TestRecommendationSavingsUntilEnd(t *testing.T) {
	now := day("2026-10-23")
	subs := []sub{
		// 12000 a year, 3 months left (2 whole and a begun one): 3000, of
		// which 8 of 10 seats are 2400.
		mk(1, "1000", func(s *sub) { s.Name, s.SeatsUsed, s.EndDate = "Jira", 2, dayPtr("2027-01-10") }),
		// Exactly 2 months left of nobody's subscription: 1000.
		mk(2, "500", func(s *sub) { s.Name, s.SeatsUsed, s.EndDate = "Miro", 0, dayPtr("2026-12-23") }),
		// Ends in over a year: a whole year, 6000.
		mk(3, "500", func(s *sub) { s.Name, s.SeatsUsed, s.EndDate = "Zoom", 0, dayPtr("2028-01-01") }),
	}
	want := map[int64]struct{ saving, span string }{
		1: {"2400.00", " до окончания."}, 2: {"1000.00", " до окончания."}, 3: {"6000.00", "/год."},
	}
	for _, r := range recommend(subs, now) {
		w := want[r.SubscriptionIDs[0]]
		if r.SavingsRUB == nil || *r.SavingsRUB != w.saving || !strings.HasSuffix(r.Text, w.span) {
			t.Errorf("%s %v: saving %v, text %q; want %s, %q", r.Type, r.SubscriptionIDs, r.SavingsRUB, r.Text, w.saving, w.span)
		}
	}
}

func fmtIDs(ids []int64) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, strconv.FormatInt(id, 10))
	}
	return strings.Join(parts, ",")
}

func TestPlural(t *testing.T) {
	for n, want := range map[int]string{1: "место", 2: "места", 5: "мест", 11: "мест", 21: "место", 24: "места", 112: "мест"} {
		if got := plural(n, "место", "места", "мест"); got != want {
			t.Errorf("plural(%d) = %s, want %s", n, got, want)
		}
	}
}

func TestMonthsBetween(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{{"2025-10-23", "2026-10-23", 12}, {"2025-10-24", "2026-10-23", 11}, {"2026-10-01", "2026-10-31", 0}}
	for _, c := range cases {
		if got := monthsBetween(day(c.a), day(c.b)); got != c.want {
			t.Errorf("monthsBetween(%s, %s) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
