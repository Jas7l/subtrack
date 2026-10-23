package domain

import "time"

// Role is a user's place in the access model. The set is closed and mirrors
// the CHECK constraint on users.role.
type Role string

const (
	// RoleAdmin manages users, departments and every subscription, and gives
	// the final approval on requests.
	RoleAdmin Role = "admin"
	// RoleManager heads one department: sees its subscriptions and people and
	// gives the first approval on their requests.
	RoleManager Role = "manager"
	// RoleEmployee sees only their own subscriptions and requests.
	RoleEmployee Role = "employee"
)

// Valid reports whether r is one of the three known roles.
func (r Role) Valid() bool {
	switch r {
	case RoleAdmin, RoleManager, RoleEmployee:
		return true
	}
	return false
}

// User is a person who can sign in. Users are never deleted: blocking sets
// IsActive to false, which keeps their history intact.
//
// PasswordHash is the bcrypt hash and never leaves the service layer; the
// transport DTOs do not carry it.
type User struct {
	ID           int64
	FullName     string
	Email        string
	PasswordHash string
	Role         Role
	DepartmentID *int64 // nil when the user belongs to no department
	IsActive     bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// UserFilter narrows a user list. Zero-value fields do not filter.
type UserFilter struct {
	DepartmentID *int64
	Role         *Role
	Active       *bool
	// Query matches a substring of the full name or the email,
	// case-insensitively.
	Query string
}

// UserPatch carries the optional fields of an update on a User. A nil pointer
// (or an unset IDPatch) leaves the column untouched. The password is not here:
// it changes through its own endpoints, never as a side effect of an edit.
type UserPatch struct {
	FullName     *string
	Email        *string
	Role         *Role
	DepartmentID IDPatch
	IsActive     *bool
}

// HasAny reports whether p touches at least one field. Services use it to
// reject empty PATCH bodies before reaching storage.
func (p UserPatch) HasAny() bool {
	return p.FullName != nil || p.Email != nil || p.Role != nil || p.DepartmentID.Set || p.IsActive != nil
}

// Fields returns the snake_case names of the fields touched by p, for logging
// which fields an update changes without dumping values.
func (p UserPatch) Fields() []string {
	fields := make([]string, 0, 5)
	if p.FullName != nil {
		fields = append(fields, "full_name")
	}
	if p.Email != nil {
		fields = append(fields, "email")
	}
	if p.Role != nil {
		fields = append(fields, "role")
	}
	if p.DepartmentID.Set {
		fields = append(fields, "department_id")
	}
	if p.IsActive != nil {
		fields = append(fields, "is_active")
	}
	return fields
}
