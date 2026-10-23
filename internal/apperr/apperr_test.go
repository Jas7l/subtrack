package apperr

import (
	"errors"
	"fmt"
	"testing"
)

func TestNew(t *testing.T) {
	a := New(ErrConflict, "service: no free seats left")
	b := New(ErrConflict, "service: no free seats left")
	wrapped := fmt.Errorf("assign: %w", a)

	if !errors.Is(wrapped, a) || !errors.Is(wrapped, ErrConflict) {
		t.Error("wrapped sentinel lost its identity or kind")
	}
	if errors.Is(a, b) {
		t.Error("two sentinels with the same text must stay distinct")
	}
	if errors.Is(a, ErrInvalidInput) || errors.Is(a, ErrNotFound) {
		t.Error("sentinel matches a kind it does not have")
	}
	if a.Error() != "service: no free seats left" {
		t.Errorf("message %q", a.Error())
	}
}
