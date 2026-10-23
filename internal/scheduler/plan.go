package scheduler

import (
	"fmt"
	"time"

	"subtrack/internal/domain"
)

// eventKind is what the scheduler found about a subscription.
type eventKind string

const (
	// evExpiring: the end date is within ExpiringDays.
	evExpiring eventKind = "expiring"
	// evExpired: the end date has passed.
	evExpired eventKind = "expired"
	// evPayment: the next payment is within PaymentDays.
	evPayment eventKind = "payment"
	// evRenewed: an expiring_soon subscription whose end date was moved out
	// of the window (or cleared) goes back to active, silently.
	evRenewed eventKind = "renewed"
	// evCharge: next_payment_date has come. A pending payment is created for
	// it (and for any later dates also passed), and next_payment_date moves
	// on by the billing period.
	evCharge eventKind = "charge"
)

// maxChargesPerRun bounds how many payment dates of one subscription one run
// catches up on, so a next_payment_date typed decades in the past cannot
// create thousands of rows at once. The next run goes on from where this one
// stopped.
const maxChargesPerRun = 24

// event is one thing to do about a subscription in a run.
type event struct {
	kind eventKind
	// status is the status to set, or "" to leave it as it is.
	status domain.SubscriptionStatus
	// key identifies the event in notification_log, or "" when there is
	// nothing to notify. It names the date of the event, not of the run, so
	// each event is notified once however many runs see it, and moving the
	// date makes a new event.
	key string
	// charge: for evCharge, the payment dates to create pending payments
	// for, oldest first.
	charge []time.Time
	// next: for evCharge, the next_payment_date to store afterwards; for
	// evPayment, the payment date reminded of.
	next time.Time
}

// plan says what to do about sub today. today is a calendar day at midnight
// UTC, the way dates come out of Postgres. Cancelled and expired
// subscriptions are left alone.
//
// Payments: every next_payment_date up to today and not after the end date
// is charged (evCharge), then a reminder goes out for a payment date within
// PaymentDays - today's own charge included, so a 0-day window still
// reminds on the day. An expired subscription gets no reminder; neither does
// a payment after the end date.
func plan(sub domain.Subscription, today time.Time, st domain.NotificationSettings) []event {
	if sub.Status != domain.SubActive && sub.Status != domain.SubExpiringSoon {
		return nil
	}
	end := sub.EndDate
	var out []event

	pay := sub.NextPaymentDate
	var due []time.Time // payment dates a reminder may be about
	if pay != nil {
		e := event{kind: evCharge, next: *pay}
		for !e.next.After(today) && (end == nil || !e.next.After(*end)) && len(e.charge) < maxChargesPerRun {
			e.charge = append(e.charge, e.next)
			e.next = nextPayment(e.next, sub.BillingPeriod)
		}
		if len(e.charge) > 0 {
			out = append(out, e)
			due = append(due, e.charge[len(e.charge)-1])
		}
		due = append(due, e.next)
	}

	if end != nil && end.Before(today) {
		return append(out, event{kind: evExpired, status: domain.SubExpired, key: eventKey(sub.ID, evExpired, *end)})
	}

	switch {
	case end != nil && !end.After(today.AddDate(0, 0, st.ExpiringDays)):
		e := event{kind: evExpiring, key: eventKey(sub.ID, evExpiring, *end)}
		if sub.Status == domain.SubActive {
			e.status = domain.SubExpiringSoon
		}
		out = append(out, e)
	case sub.Status == domain.SubExpiringSoon:
		out = append(out, event{kind: evRenewed, status: domain.SubActive})
	}

	for _, d := range due {
		if !d.Before(today) && !d.After(today.AddDate(0, 0, st.PaymentDays)) && (end == nil || !d.After(*end)) {
			out = append(out, event{kind: evPayment, key: eventKey(sub.ID, evPayment, d), next: d})
		}
	}
	return out
}

// nextPayment is the payment date one billing period after d. Past the end
// of a shorter month the day is clamped: 31.01 → 28.02 → 28.03.
func nextPayment(d time.Time, p domain.BillingPeriod) time.Time {
	months := 1
	switch p {
	case domain.BillingQuarter:
		months = 3
	case domain.BillingYear:
		months = 12
	}
	y, m, day := d.Date()
	first := time.Date(y, m+time.Month(months), 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(0, 1, -1).Day()
	return time.Date(first.Year(), first.Month(), min(day, last), 0, 0, 0, 0, time.UTC)
}

// eventKey builds a notification_log key such as "sub_5_expiring_2026-10-30".
func eventKey(subID int64, kind eventKind, day time.Time) string {
	return fmt.Sprintf("sub_%d_%s_%s", subID, kind, day.Format(time.DateOnly))
}

// dayIn returns the calendar day of t in loc, as midnight UTC.
func dayIn(t time.Time, loc *time.Location) time.Time {
	y, m, d := t.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// daysUntil is how many days from today to day, both midnight UTC.
func daysUntil(today, day time.Time) int {
	return int(day.Sub(today).Hours() / 24)
}

// inDays says when a day n days away is, in Russian: "сегодня", "завтра",
// "через 3 дня".
func inDays(n int) string {
	switch n {
	case 0:
		return "сегодня"
	case 1:
		return "завтра"
	}
	word := "дней"
	if n%100 < 11 || n%100 > 14 {
		switch n % 10 {
		case 1:
			word = "день"
		case 2, 3, 4:
			word = "дня"
		}
	}
	return fmt.Sprintf("через %d %s", n, word)
}
