package httpx

import (
	"context"
	"sync"
)

// The completion line the Logging middleware emits - one line per request, the
// line an operator actually greps - would otherwise carry only the status, so a
// failure would read as `status: 500` and nothing else, with the real error on
// a separate record logged by Internal.
//
// An ErrorSlot travels in the request context. Whichever of Error/Internal a
// handler reaches writes the error into it, and the middleware reads it when it
// closes out the request, so the completion line explains itself. Nothing else
// changes: both helpers still log their own record, and the wire is untouched.

type errSlotKey struct{}

// ErrorSlot holds the error a handler answered a request with. It is written
// once per request by Error or Internal and read once by the middleware, but
// the mutex is not ceremony: a handler that has already started streaming can
// still be writing while a client-disconnect path reads, and the race detector
// runs over the whole HTTP suite.
type ErrorSlot struct {
	mu     sync.Mutex
	err    error
	status int
}

// WithErrorSlot attaches a fresh slot to ctx and hands back both the derived
// context and the slot. The middleware keeps the pointer so reading the result
// costs nothing, and passes the context down to the handler.
func WithErrorSlot(ctx context.Context) (context.Context, *ErrorSlot) {
	s := &ErrorSlot{}
	return context.WithValue(ctx, errSlotKey{}, s), s
}

// Recorded reports the error the handler answered with, and the status it chose.
// ok is false when the request produced no error, which is the common case.
// A nil receiver reports no error, so a caller holding a slot from a context
// that never had one does not need a guard.
func (s *ErrorSlot) Recorded() (err error, status int, ok bool) {
	if s == nil {
		return nil, 0, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err, s.status, s.err != nil
}

// record stores the first error written for a request. First rather than last:
// a handler that fails, writes its response, and then trips a second error on
// the way out should be reported by the failure that determined the status, not
// by whatever happened afterwards.
func recordError(ctx context.Context, status int, err error) {
	if err == nil {
		return
	}
	s, ok := ctx.Value(errSlotKey{}).(*ErrorSlot)
	if !ok || s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err == nil {
		s.err, s.status = err, status
	}
}
