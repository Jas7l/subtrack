package scheduler

import (
	"reflect"
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
			name: "missed payments before the end are charged, then it expires",
			sub: domain.Subscription{ID: 3, Status: domain.SubActive, BillingPeriod: domain.BillingMonth,
				EndDate: dayPtr("2026-10-10"), NextPaymentDate: dayPtr("2026-09-05")},
			want: []event{
				{kind: evCharge, charge: []time.Time{day("2026-09-05"), day("2026-10-05")}, next: day("2026-11-05")},
				{kind: evExpired, status: domain.SubExpired, key: "sub_3_expired_2026-10-10"},
			},
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
			want: []event{{kind: evPayment, key: "sub_5_payment_2026-10-26", next: day("2026-10-26")}},
		},
		{
			name: "payment today is charged and reminded",
			sub: domain.Subscription{ID: 5, Status: domain.SubActive, BillingPeriod: domain.BillingQuarter,
				NextPaymentDate: dayPtr("2026-10-23")},
			want: []event{
				{kind: evCharge, charge: []time.Time{day("2026-10-23")}, next: day("2027-01-23")},
				{kind: evPayment, key: "sub_5_payment_2026-10-23", next: day("2026-10-23")},
			},
		},
		{
			name: "payment yesterday is charged, not reminded",
			sub: domain.Subscription{ID: 5, Status: domain.SubActive, BillingPeriod: domain.BillingYear,
				NextPaymentDate: dayPtr("2026-10-22")},
			want: []event{{kind: evCharge, charge: []time.Time{day("2026-10-22")}, next: day("2027-10-22")}},
		},
		{
			name: "missed months are each charged and the new date is reminded",
			sub: domain.Subscription{ID: 5, Status: domain.SubActive, BillingPeriod: domain.BillingMonth,
				NextPaymentDate: dayPtr("2026-08-25")},
			want: []event{
				{kind: evCharge, charge: []time.Time{day("2026-08-25"), day("2026-09-25")}, next: day("2026-10-25")},
				{kind: evPayment, key: "sub_5_payment_2026-10-25", next: day("2026-10-25")},
			},
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
				{kind: evPayment, key: "sub_7_payment_2026-10-24", next: day("2026-10-24")},
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
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("plan =\n  %+v\nwant\n  %+v", got, c.want)
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
	got := plan(sub, today, st)
	if len(got) != 3 || got[0].kind != evCharge || got[1].kind != evExpiring || got[2].kind != evPayment {
		t.Fatalf("today with 0 days: plan = %+v, want charge, expiring and payment", got)
	}
}

func TestPlanChargeCap(t *testing.T) {
	sub := domain.Subscription{ID: 1, Status: domain.SubActive, BillingPeriod: domain.BillingMonth,
		NextPaymentDate: dayPtr("1990-01-01")}
	got := plan(sub, day("2026-10-23"), domain.NotificationSettings{})
	if len(got) != 1 || len(got[0].charge) != maxChargesPerRun || !got[0].next.Equal(day("1992-01-01")) {
		t.Fatalf("plan = %+v, want %d charges and next 1992-01-01", got, maxChargesPerRun)
	}
}

func TestPlanChargeCapPutsOffExpiry(t *testing.T) {
	// 30 monthly payments are owed before the end date: the first run
	// charges 24 and leaves the subscription open, the second charges the
	// other 6 and expires it.
	sub := domain.Subscription{ID: 1, Status: domain.SubActive, BillingPeriod: domain.BillingMonth,
		NextPaymentDate: dayPtr("2024-01-10"), EndDate: dayPtr("2026-06-30")}
	today := day("2026-10-23")
	got := plan(sub, today, domain.NotificationSettings{})
	if len(got) != 1 || got[0].kind != evCharge || len(got[0].charge) != maxChargesPerRun {
		t.Fatalf("first run = %+v, want only %d charges", got, maxChargesPerRun)
	}

	sub.NextPaymentDate = &got[0].next
	got = plan(sub, today, domain.NotificationSettings{})
	if len(got) != 2 || len(got[0].charge) != 6 || !got[0].next.Equal(day("2026-07-10")) || got[1].kind != evExpired {
		t.Fatalf("second run = %+v, want 6 charges, then expired", got)
	}

	// Exactly the cap owed: nothing is left, so it expires in the same run.
	sub.NextPaymentDate, sub.EndDate = dayPtr("2024-07-10"), dayPtr("2026-06-30")
	got = plan(sub, today, domain.NotificationSettings{})
	if len(got) != 2 || len(got[0].charge) != maxChargesPerRun || got[1].kind != evExpired {
		t.Fatalf("exactly the cap = %+v, want %d charges, then expired", got, maxChargesPerRun)
	}
}

func TestPlanYearEnd(t *testing.T) {
	st := domain.NotificationSettings{ExpiringDays: 7, PaymentDays: 3}
	cases := []struct {
		name  string
		today string
		sub   domain.Subscription
		want  []event
	}{
		{
			name:  "charged on 31.12, next on 31.01",
			today: "2026-12-31",
			sub: domain.Subscription{ID: 1, Status: domain.SubActive, BillingPeriod: domain.BillingMonth,
				NextPaymentDate: dayPtr("2026-12-31")},
			want: []event{
				{kind: evCharge, charge: []time.Time{day("2026-12-31")}, next: day("2027-01-31")},
				{kind: evPayment, key: "sub_1_payment_2026-12-31", next: day("2026-12-31")},
			},
		},
		{
			name:  "windows reach into January",
			today: "2026-12-30",
			sub: domain.Subscription{ID: 2, Status: domain.SubActive, BillingPeriod: domain.BillingYear,
				NextPaymentDate: dayPtr("2027-01-02"), EndDate: dayPtr("2027-01-06")},
			want: []event{
				{kind: evExpiring, status: domain.SubExpiringSoon, key: "sub_2_expiring_2027-01-06"},
				{kind: evPayment, key: "sub_2_payment_2027-01-02", next: day("2027-01-02")},
			},
		},
		{
			name:  "missed run over New Year catches up",
			today: "2027-01-01",
			sub: domain.Subscription{ID: 3, Status: domain.SubActive, BillingPeriod: domain.BillingMonth,
				NextPaymentDate: dayPtr("2026-11-30")},
			want: []event{
				{kind: evCharge, charge: []time.Time{day("2026-11-30"), day("2026-12-30")}, next: day("2027-01-30")},
			},
		},
		{
			name:  "ended on 31.12",
			today: "2027-01-01",
			sub:   domain.Subscription{ID: 4, Status: domain.SubExpiringSoon, EndDate: dayPtr("2026-12-31")},
			want:  []event{{kind: evExpired, status: domain.SubExpired, key: "sub_4_expired_2026-12-31"}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := plan(c.sub, day(c.today), st)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("plan =\n  %+v\nwant\n  %+v", got, c.want)
			}
		})
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
		{time.Date(2026, 12, 31, 20, 59, 0, 0, time.UTC), "2026-12-31"},
		{time.Date(2026, 12, 31, 21, 0, 0, 0, time.UTC), "2027-01-01"},
	}
	for _, c := range cases {
		got := dayIn(c.now, msk)
		if got.Format(time.DateOnly) != c.want || got.Location() != time.UTC || got.Hour() != 0 {
			t.Errorf("dayIn(%s) = %s, want %s at midnight UTC", c.now, got, c.want)
		}
	}
}

func TestDaysUntil(t *testing.T) {
	if n := daysUntil(day("2026-12-30"), day("2027-01-02")); n != 3 {
		t.Errorf("30.12 → 02.01: %d days, want 3", n)
	}
	if n := daysUntil(day("2028-02-27"), day("2028-03-01")); n != 3 {
		t.Errorf("27.02 → 01.03 in a leap year: %d days, want 3", n)
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
