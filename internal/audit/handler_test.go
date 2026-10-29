package audit

import (
	"net/http/httptest"
	"testing"
	"time"
)

// The from/to days are the company's: in Moscow the day 2026-10-29 runs from
// 2026-10-28 21:00 UTC to 2026-10-29 21:00 UTC.
func TestParseFilterDaysInCompanyZone(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/audit?from=2026-10-29&to=2026-10-29", nil)
	f, err := parseFilter(r, loc)
	if err != nil {
		t.Fatal(err)
	}
	wantFrom := time.Date(2026, 10, 28, 21, 0, 0, 0, time.UTC)
	wantTo := time.Date(2026, 10, 29, 21, 0, 0, 0, time.UTC)
	if f.From == nil || !f.From.Equal(wantFrom) {
		t.Errorf("from = %v, want %v", f.From, wantFrom)
	}
	if f.To == nil || !f.To.Equal(wantTo) {
		t.Errorf("to = %v, want %v", f.To, wantTo)
	}
}
