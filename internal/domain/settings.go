package domain

import "time"

// MaxNotifyDays bounds both notification settings; it mirrors the CHECK
// constraints on notification_settings.
const MaxNotifyDays = 365

// NotificationSettings says how early the scheduler warns. There is one set
// for the whole company.
type NotificationSettings struct {
	// ExpiringDays: a subscription whose end_date is at most this many days
	// away (0 = today) becomes expiring_soon and its holders are warned.
	ExpiringDays int
	// PaymentDays: the admins are reminded of a payment at most this many
	// days before next_payment_date.
	PaymentDays int
	UpdatedAt   time.Time
	UpdatedBy   *int64 // nil until an admin changes the defaults
}
