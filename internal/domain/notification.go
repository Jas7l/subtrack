package domain

import "time"

// NotificationType is what a notification is about. The set is closed and
// mirrors the CHECK constraint on notifications.type.
type NotificationType string

const (
	NotifySubscriptionExpiring NotificationType = "subscription_expiring"
	NotifySubscriptionExpired  NotificationType = "subscription_expired"
	NotifyPaymentDue           NotificationType = "payment_due"
	// NotifyRequestCreated tells a reviewer that a request waits for them.
	NotifyRequestCreated NotificationType = "request_created"
	// NotifyRequestApproved tells the requester about an approval, by the
	// department head (it moves on to the admin) or the final one.
	NotifyRequestApproved NotificationType = "request_approved"
	NotifyRequestRejected NotificationType = "request_rejected"
)

// Notification is an in-app message to one user.
type Notification struct {
	ID        int64
	UserID    int64
	Type      NotificationType
	Title     string
	Body      string
	IsRead    bool
	CreatedAt time.Time
}
