package domain

import "time"

// SubscriptionType says how a subscription's licences are counted. The set is
// closed and mirrors the CHECK constraint on subscriptions.type.
type SubscriptionType string

const (
	// SubIndividual is one licence for one person: SeatsTotal is always 1.
	SubIndividual SubscriptionType = "individual"
	// SubGroup is a pack of SeatsTotal seats.
	SubGroup SubscriptionType = "group"
	// SubEnterprise is unlimited: SeatsTotal is nil.
	SubEnterprise SubscriptionType = "enterprise"
)

// Valid reports whether t is one of the known types.
func (t SubscriptionType) Valid() bool {
	switch t {
	case SubIndividual, SubGroup, SubEnterprise:
		return true
	}
	return false
}

// BillingPeriod is how often a subscription is paid for.
type BillingPeriod string

const (
	BillingMonth   BillingPeriod = "month"
	BillingQuarter BillingPeriod = "quarter"
	BillingYear    BillingPeriod = "year"
)

// Valid reports whether p is one of the known periods.
func (p BillingPeriod) Valid() bool {
	switch p {
	case BillingMonth, BillingQuarter, BillingYear:
		return true
	}
	return false
}

// Months is how many months the period lasts: 1, 3 or 12.
func (p BillingPeriod) Months() int {
	switch p {
	case BillingQuarter:
		return 3
	case BillingYear:
		return 12
	}
	return 1
}

// Next is the payment day one period after d (a calendar day at midnight
// UTC). Past the end of a shorter month the day is clamped, and stays so:
// 31.01 → 28.02 → 28.03.
func (p BillingPeriod) Next(d time.Time) time.Time {
	y, m, day := d.Date()
	first := time.Date(y, m+time.Month(p.Months()), 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(0, 1, -1).Day()
	return time.Date(first.Year(), first.Month(), min(day, last), 0, 0, 0, 0, time.UTC)
}

// SubscriptionStatus is where a subscription is in its life. active and
// cancelled are set by people; expiring_soon and expired are set by the
// scheduler from end_date.
type SubscriptionStatus string

const (
	SubActive       SubscriptionStatus = "active"
	SubExpiringSoon SubscriptionStatus = "expiring_soon"
	SubExpired      SubscriptionStatus = "expired"
	SubCancelled    SubscriptionStatus = "cancelled"
)

// Valid reports whether s is one of the known statuses.
func (s SubscriptionStatus) Valid() bool {
	switch s {
	case SubActive, SubExpiringSoon, SubExpired, SubCancelled:
		return true
	}
	return false
}

// Open reports whether licences of a subscription in this status may still be
// handed out.
func (s SubscriptionStatus) Open() bool {
	return s == SubActive || s == SubExpiringSoon
}

// Subscription is one paid SaaS product or licence pack in the company's
// registry. Subscriptions are soft-deleted, so payments and past assignments
// keep pointing at a real row.
//
// SeatsUsed counts the active assignments. It is kept by the store in the
// same transaction that assigns or revokes a licence and is never written
// from a request; the database refuses SeatsUsed > SeatsTotal.
type Subscription struct {
	ID       int64
	Name     string
	Vendor   string
	Category string
	Type     SubscriptionType
	// Cost is the price per billing period as a decimal string ("1500.00"),
	// so money never passes through a float.
	Cost            string
	Currency        string
	BillingPeriod   BillingPeriod
	StartDate       time.Time
	EndDate         *time.Time
	NextPaymentDate *time.Time
	Status          SubscriptionStatus
	DepartmentID    *int64 // nil for a company-wide subscription
	SeatsTotal      *int   // nil for enterprise (unlimited)
	SeatsUsed       int
	CreatedBy       *int64
	// DepartmentName is read-only: filled in by reads, ignored on writes.
	DepartmentName *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// SubscriptionFilter narrows a subscription list. Zero-value fields do not
// filter.
type SubscriptionFilter struct {
	DepartmentID *int64
	// Category matches the whole category case-insensitively.
	Category string
	Status   *SubscriptionStatus
	Type     *SubscriptionType
	// EndFrom and EndTo bound end_date, both inclusive. A subscription
	// without an end date never matches a bound.
	EndFrom *time.Time
	EndTo   *time.Time
	// Query matches a substring of the name or the vendor, case-insensitively.
	Query string
	// FreeSeats true keeps the subscriptions with a seat limit and seats
	// left over; false keeps the ones with every seat taken. Enterprise
	// subscriptions have no limit and match neither.
	FreeSeats *bool
}

// SubscriptionScope limits what a caller may see: the subscriptions of
// DepartmentID plus those HolderID has an active licence for. A nil scope
// means everything (the admin); a scope with both fields nil means nothing.
type SubscriptionScope struct {
	DepartmentID *int64
	HolderID     *int64
}

// SubscriptionPatch carries the optional fields of an update on a
// Subscription. SeatsUsed is not here: only assignments move it.
type SubscriptionPatch struct {
	Name            *string
	Vendor          *string
	Category        *string
	Type            *SubscriptionType
	Cost            *string
	Currency        *string
	BillingPeriod   *BillingPeriod
	StartDate       *time.Time
	EndDate         DatePatch
	NextPaymentDate DatePatch
	Status          *SubscriptionStatus
	DepartmentID    IDPatch
	SeatsTotal      *int
}

// HasAny reports whether p touches at least one field.
func (p SubscriptionPatch) HasAny() bool {
	return p.Name != nil || p.Vendor != nil || p.Category != nil || p.Type != nil || p.Cost != nil ||
		p.Currency != nil || p.BillingPeriod != nil || p.StartDate != nil || p.EndDate.Set ||
		p.NextPaymentDate.Set || p.Status != nil || p.DepartmentID.Set || p.SeatsTotal != nil
}

// DatePatch changes a nullable date in a patch; see IDPatch for the meaning
// of Set and Value.
type DatePatch struct {
	Set   bool
	Value *time.Time
}

// AssignmentStatus is whether a licence is still held.
type AssignmentStatus string

const (
	AssignmentActive  AssignmentStatus = "active"
	AssignmentRevoked AssignmentStatus = "revoked"
)

// Valid reports whether s is one of the known statuses.
func (s AssignmentStatus) Valid() bool {
	return s == AssignmentActive || s == AssignmentRevoked
}

// Assignment is one licence of a subscription handed to one user. Revoking
// keeps the row as history; the same user may be assigned again later, as a
// new row.
type Assignment struct {
	ID             int64
	SubscriptionID int64
	UserID         int64
	Status         AssignmentStatus
	AssignedBy     *int64
	AssignedAt     time.Time
	RevokedBy      *int64 // nil while active, or when revoked by the system
	RevokedAt      *time.Time
	// The fields below are read-only, filled in by reads: the subscription's
	// name and owning department, and the holder's name.
	SubscriptionName         string
	SubscriptionDepartmentID *int64
	UserName                 string
}

// AssignmentFilter narrows an assignment list. Zero-value fields do not
// filter.
type AssignmentFilter struct {
	SubscriptionID *int64
	UserID         *int64
	Status         *AssignmentStatus
}
