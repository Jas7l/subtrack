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
