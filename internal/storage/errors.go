// Package storage holds sentinel errors shared by all repository
// implementations. Repository *interfaces* live with their consumers (the
// domain services), per idiomatic Go: the caller defines the contract, the
// implementer fulfils it.
//
// Repositories return one of these sentinels (possibly wrapped) so the service
// layer can branch on storage outcomes without knowing SQL details. The service
// layer translates them into entity-specific errors (ErrUserNotFound,
// ErrConflict, …) before they reach the transport layer.
package storage

import "errors"

var (
	// ErrNotFound is returned when a requested row does not exist. Repositories
	// also return it from UPDATE/DELETE operations that affect zero rows, and
	// from FK-violation translations when the referenced parent row is missing.
	ErrNotFound = errors.New("storage: not found")

	// ErrAlreadyExists is returned when an INSERT or UPDATE would violate a
	// unique constraint (SQLSTATE 23505, unique_violation).
	ErrAlreadyExists = errors.New("storage: already exists")
)

// ConstraintError is a unique violation that remembers which constraint fired.
// It matches ErrAlreadyExists via errors.Is, so callers that only care about
// "duplicate" need nothing else; a service that has several unique rules on
// one table reads Constraint to tell them apart:
//
//	var ce *storage.ConstraintError
//	if errors.As(err, &ce) && ce.Constraint == "users_email_key" {
//		return ErrEmailTaken
//	}
type ConstraintError struct {
	Constraint string
}

func (e *ConstraintError) Error() string {
	return "storage: unique constraint violated: " + e.Constraint
}

func (e *ConstraintError) Is(target error) bool {
	return target == ErrAlreadyExists
}
