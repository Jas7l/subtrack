// Package apperr holds the cross-cutting error kinds shared by every domain
// service, plus the storage→service translation helper.
//
// Only the kinds the transport layer classifies generically live here:
//
//   - ErrInvalidInput - validation failed at the service boundary (missing
//     required field, empty patch, bad query parameter). Transport maps it to
//     HTTP 400.
//   - ErrConflict     - storage reported a uniqueness violation. Transport
//     maps it to HTTP 409.
//
// Entity-specific sentinels (ErrUserNotFound, ErrSubscriptionNotFound, …) live
// in their own domain packages; each domain maps them to HTTP statuses in its
// own routes.go.
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
	// (e.g. a duplicate user email or department name).
	ErrConflict = errors.New("service: resource already exists")
)

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
