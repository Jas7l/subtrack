package scheduler

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

func TestPlan(t *testing.T) {
	today := day("2026-10-23")
	st := domain.NotificationSettings{ExpiringDays: 7, PaymentDays: 3}
	cases := []struct {
		name string
		sub  domain.Subscription
		want []event
	}{
		{
			name: "no dates",
			sub:  domain.Subscription{ID: 1, Status: domain.SubActive},
		},
		{
			name: "end far away",
			sub:  domain.Subscription{ID: 1, Status: domain.SubActive, EndDate: dayPtr("2026-10-31")},
		},
		{
			name: "end on the last day of the window",
			sub:  domain.Subscription{ID: 1, Status: domain.SubActive, EndDate: dayPtr("2026-10-30")},
			want: []event{{kind: evExpiring, status: domain.SubExpiringSoon, key: "sub_1_expiring_2026-10-30"}},
		},
		{
			name: "end today is not expired yet",
			sub:  domain.Subscription{ID: 1, Status: domain.SubActive, EndDate: dayPtr("2026-10-23")},
			want: []event{{kind: evExpiring, status: domain.SubExpiringSoon, key: "sub_1_expiring_2026-10-23"}},
		},
		{
			name: "already expiring_soon keeps its status but still has the event",
			sub:  domain.Subscription{ID: 2, Status: domain.SubExpiringSoon, EndDate: dayPtr("2026-10-25")},
			want: []event{{kind: evExpiring, key: "sub_2_expiring_2026-10-25"}},
		},
		{
			name: "end yesterday expires",
			sub:  domain.Subscription{ID: 3, Status: domain.SubExpiringSoon, EndDate: dayPtr("2026-10-22")},
			want: []event{{kind: evExpired, status: domain.SubExpired, key: "sub_3_expired_2026-10-22"}},
		},
		{
			name: "expired gets no payment reminder",
			sub: domain.Subscription{ID: 3, Status: domain.SubActive, EndDate: dayPtr("2026-10-01"),
				NextPaymentDate: dayPtr("2026-10-24")},
			want: []event{{kind: evExpired, status: domain.SubExpired, key: "sub_3_expired_2026-10-01"}},
		},
		{
			name: "expiring_soon moved away goes back to active",
			sub:  domain.Subscription{ID: 4, Status: domain.SubExpiringSoon, EndDate: dayPtr("2027-10-23")},
			want: []event{{kind: evRenewed, status: domain.SubActive}},
		},
		{
			name: "expiring_soon with the end date cleared goes back to active",
			sub:  domain.Subscription{ID: 4, Status: domain.SubExpiringSoon},
			want: []event{{kind: evRenewed, status: domain.SubActive}},
		},
		{
			name: "payment within the window",
			sub:  domain.Subscription{ID: 5, Status: domain.SubActive, NextPaymentDate: dayPtr("2026-10-26")},
			want: []event{{kind: evPayment, key: "sub_5_payment_2026-10-26"}},
		},
		{
			name: "payment today",
			sub:  domain.Subscription{ID: 5, Status: domain.SubActive, NextPaymentDate: dayPtr("2026-10-23")},
			want: []event{{kind: evPayment, key: "sub_5_payment_2026-10-23"}},
		},
		{
			name: "payment past is not reminded",
			sub:  domain.Subscription{ID: 5, Status: domain.SubActive, NextPaymentDate: dayPtr("2026-10-22")},
		},
		{
			name: "payment beyond the window",
			sub:  domain.Subscription{ID: 5, Status: domain.SubActive, NextPaymentDate: dayPtr("2026-10-27")},
		},
		{
			name: "payment after the end date is not reminded",
			sub: domain.Subscription{ID: 6, Status: domain.SubActive, EndDate: dayPtr("2026-10-24"),
				NextPaymentDate: dayPtr("2026-10-25")},
			want: []event{{kind: evExpiring, status: domain.SubExpiringSoon, key: "sub_6_expiring_2026-10-24"}},
		},
		{
			name: "expiring and payment together",
			sub: domain.Subscription{ID: 7, Status: domain.SubActive, EndDate: dayPtr("2026-10-28"),
				NextPaymentDate: dayPtr("2026-10-24")},
			want: []event{
				{kind: evExpiring, status: domain.SubExpiringSoon, key: "sub_7_expiring_2026-10-28"},
				{kind: evPayment, key: "sub_7_payment_2026-10-24"},
			},
		},
		{
			name: "cancelled is left alone",
			sub: domain.Subscription{ID: 8, Status: domain.SubCancelled, EndDate: dayPtr("2026-10-01"),
				NextPaymentDate: dayPtr("2026-10-24")},
		},
		{
			name: "expired is left alone",
			sub:  domain.Subscription{ID: 9, Status: domain.SubExpired, EndDate: dayPtr("2026-10-01")},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := plan(c.sub, today, st)
			if len(got) != len(c.want) {
				t.Fatalf("plan = %+v, want %+v", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("event %d = %+v, want %+v", i, got[i], c.want[i])
				}
			}
		})
	}
}

func TestPlanZeroDays(t *testing.T) {
	today := day("2026-10-23")
	st := domain.NotificationSettings{}
	sub := domain.Subscription{ID: 1, Status: domain.SubActive, EndDate: dayPtr("2026-10-24"), NextPaymentDate: dayPtr("2026-10-24")}
	if got := plan(sub, today, st); len(got) != 0 {
		t.Fatalf("tomorrow with 0 days: plan = %+v, want nothing", got)
	}
	sub.EndDate, sub.NextPaymentDate = dayPtr("2026-10-23"), dayPtr("2026-10-23")
	if got := plan(sub, today, st); len(got) != 2 {
		t.Fatalf("today with 0 days: plan = %+v, want expiring and payment", got)
	}
}

func TestDayIn(t *testing.T) {
	msk, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		now  time.Time
		want string
	}{
		{time.Date(2026, 10, 23, 20, 59, 0, 0, time.UTC), "2026-10-23"},
		// 21:00 UTC is midnight in Moscow (UTC+3).
		{time.Date(2026, 10, 23, 21, 0, 0, 0, time.UTC), "2026-10-24"},
	}
	for _, c := range cases {
		got := dayIn(c.now, msk)
		if got.Format(time.DateOnly) != c.want || got.Location() != time.UTC || got.Hour() != 0 {
			t.Errorf("dayIn(%s) = %s, want %s at midnight UTC", c.now, got, c.want)
		}
	}
}

func TestInDays(t *testing.T) {
	cases := map[int]string{
		0: "сегодня", 1: "завтра", 2: "через 2 дня", 4: "через 4 дня", 5: "через 5 дней",
		11: "через 11 дней", 14: "через 14 дней", 21: "через 21 день", 22: "через 22 дня", 111: "через 111 дней",
	}
	for n, want := range cases {
		if got := inDays(n); got != want {
			t.Errorf("inDays(%d) = %q, want %q", n, got, want)
		}
	}
}
