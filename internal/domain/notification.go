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
	// NotifyReleaseCreated tells whoever decides that a holder asks to give a
	// licence back.
	NotifyReleaseCreated NotificationType = "release_created"
	// NotifyReleaseApproved tells the holder that the licence is revoked: by
	// a decision on the request, or another way while it waited.
	NotifyReleaseApproved NotificationType = "release_approved"
	NotifyReleaseRejected NotificationType = "release_rejected"
)

// NotificationEntity is the kind of record a notification points at. The
// set mirrors the CHECK constraint on notifications.entity_type.
type NotificationEntity string

const (
	EntityRequest      NotificationEntity = "request"
	EntitySubscription NotificationEntity = "subscription"
	EntityTransaction  NotificationEntity = "transaction"
	EntityRelease      NotificationEntity = "release_request"
)

// Notification is an in-app message to one user. EntityType and EntityID
// name the record it is about; both are zero when it points nowhere.
type Notification struct {
	ID         int64
	UserID     int64
	Type       NotificationType
	Title      string
	Body       string
	EntityType NotificationEntity
	EntityID   int64
	IsRead     bool
	CreatedAt  time.Time
}
