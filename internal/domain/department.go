package domain

import "time"

// Department is a unit of the company. Its head is a manager who reviews the
// requests of the department's people.
//
// Invariant, kept by the user and department stores: when HeadID is set, it
// names an active manager whose DepartmentID is this department. Assigning a
// head moves them into the department; blocking, demoting or moving the head
// out leaves the department without one.
type Department struct {
	ID     int64
	Name   string
	HeadID *int64 // nil when the department has no head
	// HeadName and EmployeeCount are read-only: filled in by reads, ignored on
	// writes. EmployeeCount counts active users only.
	HeadName      *string
	EmployeeCount int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// DepartmentPatch carries the optional fields of an update on a Department.
type DepartmentPatch struct {
	Name   *string
	HeadID IDPatch
}

// HasAny reports whether p touches at least one field.
func (p DepartmentPatch) HasAny() bool {
	return p.Name != nil || p.HeadID.Set
}

// IDPatch changes a nullable reference in a patch, where a plain *int64 cannot
// tell "leave it" from "clear it":
//
//   - Set false             - leave the column as it is.
//   - Set true, Value nil   - clear it (NULL).
//   - Set true, Value != nil - point it at *Value.
type IDPatch struct {
	Set   bool
	Value *int64
}
