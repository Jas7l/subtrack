package domain

import "time"

// TransactionStatus is where a payment is. The set is closed and mirrors the
// CHECK constraint on transactions.status.
type TransactionStatus string

const (
	// TxPending: due, not paid yet. The scheduler creates these.
	TxPending TransactionStatus = "pending"
	// TxPaid is final.
	TxPaid TransactionStatus = "paid"
	// TxFailed: the payment did not go through; it may still be paid later.
	TxFailed TransactionStatus = "failed"
)

// Valid reports whether s is one of the known statuses.
func (s TransactionStatus) Valid() bool {
	switch s {
	case TxPending, TxPaid, TxFailed:
		return true
	}
	return false
}

// CanBecome reports whether a payment in status s may move to next:
// pending → paid | failed, failed → paid.
func (s TransactionStatus) CanBecome(next TransactionStatus) bool {
	switch s {
	case TxPending:
		return next == TxPaid || next == TxFailed
	case TxFailed:
		return next == TxPaid
	}
	return false
}

// Transaction is one payment for a subscription. A subscription has at most
// one per payment date.
type Transaction struct {
	ID             int64
	SubscriptionID int64
	// Amount is a decimal string ("1500.00"), so money never passes through a
	// float.
	Amount        string
	Currency      string
	PaymentDate   time.Time
	Status        TransactionStatus
	PaymentMethod string
	InvoiceNumber string
	PaidAt        *time.Time // set exactly when Status is paid
	// SubscriptionName is read-only, filled in by reads.
	SubscriptionName string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// TransactionFilter narrows a transaction list. Zero-value fields do not
// filter.
type TransactionFilter struct {
	SubscriptionID *int64
	Status         *TransactionStatus
	// From and To bound payment_date, both inclusive.
	From *time.Time
	To   *time.Time
}
