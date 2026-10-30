package domain

import "time"

// ReleaseStatus is where a request to give a licence back is. The set is
// closed and mirrors the CHECK constraint on release_requests.status.
//
//	pending ──approve──▶ approved   (the licence is revoked)
//	   │
//	   ├────reject───▶ rejected
//	   │
//	   └──requester cancels──▶ cancelled
type ReleaseStatus string

const (
	ReleasePending   ReleaseStatus = "pending"
	ReleaseApproved  ReleaseStatus = "approved"
	ReleaseRejected  ReleaseStatus = "rejected"
	ReleaseCancelled ReleaseStatus = "cancelled"
)

// Valid reports whether s is one of the known statuses.
func (s ReleaseStatus) Valid() bool {
	switch s {
	case ReleasePending, ReleaseApproved, ReleaseRejected, ReleaseCancelled:
		return true
	}
	return false
}

// ReleaseRequest is a licence holder asking to give the licence back. Who
// manages the subscription decides: a manager of its department or the
// admin; on approval the licence is revoked.
type ReleaseRequest struct {
	ID           int64
	AssignmentID int64
	// RequesterID is the licence holder: only the holder asks.
	RequesterID int64
	Reason      string
	Status      ReleaseStatus
	DecidedBy   *int64
	Comment     string
	DecidedAt   *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
	// The fields below are read-only, filled in by reads. DepartmentID is
	// the subscription's department now - it decides who reviews the
	// request, so a subscription moved meanwhile goes to its new department;
	// nil for a company-wide one (the admin's).
	SubscriptionID   int64
	SubscriptionName string
	DepartmentID     *int64
	DepartmentName   *string
	RequesterName    string
	DecidedByName    *string
}

// ReleaseFilter narrows a list of release requests. Zero-value fields do
// not filter.
type ReleaseFilter struct {
	Status         *ReleaseStatus
	RequesterID    *int64
	SubscriptionID *int64
	// DepartmentID is the subscription's department.
	DepartmentID *int64
	// Unattended keeps the requests nobody but the admin can decide: of
	// company-wide subscriptions, or of departments without an active
	// manager.
	Unattended bool
}

// ReleaseDecision is a reviewer's verdict on a release request.
type ReleaseDecision struct {
	ReviewerID int64
	Approve    bool
	Comment    string
}
