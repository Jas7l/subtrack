// Package apperr holds the cross-cutting error kinds shared by every domain
// service, plus the storage→service translation helper.
//
// Only the kinds the transport layer classifies generically live here:
//
//   - ErrInvalidInput - validation failed at the service boundary (missing
//     required field, empty patch, bad query parameter). Transport maps it to
//     HTTP 400.
//   - ErrConflict     - storage reported a uniqueness violation, or the
//     request breaks a rule given the current state (no free seats, already
//     revoked). Transport maps it to HTTP 409.
//   - ErrNotFound     - the thing does not exist or is out of the caller's
//     view. Transport maps it to HTTP 404.
//
// Entity-specific sentinels (ErrUserNotFound, ErrSubscriptionNotFound, …) live
// in their own domain packages; each domain maps them to HTTP statuses in its
// own routes.go. A sentinel made with New also belongs to one of these kinds,
// so a domain that receives it from a neighbour's service (an approved request
// handing out a licence) can map it without importing the neighbour.
package apperr

import (
	"errors"

	"subtrack/internal/storage"
)

var (
	// ErrInvalidInput marks a validation failure at the service boundary.
	// Wrapped messages add the specific reason; the transport layer maps
	// this to HTTP 400.
	ErrInvalidInput = errors.New("service: invalid input")

	// ErrConflict is returned when storage reports a uniqueness violation
	// (e.g. a duplicate user email or department name), and is the kind of
	// sentinels for "not allowed in the current state".
	ErrConflict = errors.New("service: resource already exists")

	// ErrNotFound is the kind of "not found" sentinels; it is never returned
	// bare.
	ErrNotFound = errors.New("service: not found")
)

// New returns a sentinel error with message msg that also matches kind
// (ErrInvalidInput, ErrConflict or ErrNotFound) under errors.Is. Each call
// gives a distinct error, so errors.Is(err, ErrNoFreeSeats) keeps telling
// sentinels apart, while errors.Is(err, apperr.ErrConflict) classifies them.
// The message is shown as is (after the "service: " prefix is stripped), not
// prefixed with the kind's.
func New(kind error, msg string) error {
	return &kindError{msg: msg, kind: kind}
}

type kindError struct {
	msg  string
	kind error
}

func (e *kindError) Error() string { return e.msg }

func (e *kindError) Is(target error) bool { return target == e.kind }

// MapStorage translates raw storage errors into the entity-specific service
// errors callers expect.
//
// Mapping:
//   - nil                      → nil.
//   - storage.ErrNotFound      → notFound (the caller-supplied sentinel).
//   - storage.ErrAlreadyExists → ErrConflict.
//   - anything else            → returned verbatim so wrapped context
//     (operation name, constraint, etc.) is preserved upstream.
func MapStorage(err error, notFound error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, storage.ErrNotFound):
		return notFound
	case errors.Is(err, storage.ErrAlreadyExists):
		return ErrConflict
	default:
		return err
	}
}
