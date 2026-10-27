package domain

import "time"

// RequestStatus is where a request is on its way through approval. The set
// is closed and mirrors the CHECK constraint on requests.status.
//
//	pending_manager ──reject──▶ rejected_by_manager
//	      │ approve
//	      ▼
//	pending_admin ────reject──▶ rejected_by_admin
//	      │ approve
//	      ▼
//	   approved
//
//	pending_manager / pending_admin ──requester cancels──▶ cancelled
type RequestStatus string

const (
	ReqPendingManager    RequestStatus = "pending_manager"
	ReqRejectedByManager RequestStatus = "rejected_by_manager"
	ReqPendingAdmin      RequestStatus = "pending_admin"
	ReqRejectedByAdmin   RequestStatus = "rejected_by_admin"
	ReqApproved          RequestStatus = "approved"
	ReqCancelled         RequestStatus = "cancelled"
)

// Valid reports whether s is one of the known statuses.
func (s RequestStatus) Valid() bool {
	switch s {
	case ReqPendingManager, ReqRejectedByManager, ReqPendingAdmin, ReqRejectedByAdmin, ReqApproved, ReqCancelled:
		return true
	}
	return false
}

// Pending reports whether the request still waits for a decision.
func (s RequestStatus) Pending() bool {
	return s == ReqPendingManager || s == ReqPendingAdmin
}

// Request is somebody asking for a software subscription. The requester
// describes what they need; on final approval the admin picks an existing
// subscription or creates one, and the requester gets a licence of it.
type Request struct {
	ID          int64
	RequesterID int64
	// DepartmentID is the requester's department when the request was made:
	// its head reviews the request even if the requester moves later.
	DepartmentID  *int64
	Name          string
	Vendor        string
	Justification string
	// EstimatedCost is a decimal string ("1500.00"), nil when not given.
	EstimatedCost *string
	Currency      string
	// SubscriptionID is filled in on approval.
	SubscriptionID   *int64
	Status           RequestStatus
	ManagerID        *int64
	ManagerComment   string
	ManagerDecidedAt *time.Time
	AdminID          *int64
	AdminComment     string
	AdminDecidedAt   *time.Time
	// RequesterName, DepartmentName, ManagerName and AdminName are
	// read-only, filled in by reads.
	RequesterName  string
	DepartmentName *string
	ManagerName    *string
	AdminName      *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// RequestFilter narrows a request list. Zero-value fields do not filter.
type RequestFilter struct {
	Status       *RequestStatus
	RequesterID  *int64
	DepartmentID *int64
}

// RequestScope limits what a caller may see: the requests of DepartmentID
// plus those RequesterID made. A nil scope means everything (the admin).
type RequestScope struct {
	DepartmentID *int64
	RequesterID  *int64
}

// Decision is a reviewer's verdict on a request.
type Decision struct {
	ReviewerID int64
	Approve    bool
	Comment    string
	// SubscriptionID is the subscription the requester got a licence of;
	// set on the admin's approval only.
	SubscriptionID *int64
}
