// Package testkit holds in-memory stand-ins shared by the domain packages'
// tests: a transaction runner and an audit recorder. Production code never
// imports it.
package testkit

import (
	"context"
	"sync"

	"subtrack/internal/domain"
)

// Tx satisfies every domain's transactor by running fn directly. Fakes have
// nothing to roll back, so a test that needs rollback runs against Postgres.
type Tx struct {
	mu    sync.Mutex
	Calls int
}

// InTx runs fn and counts the call.
func (t *Tx) InTx(ctx context.Context, fn func(ctx context.Context) error) error {
	t.mu.Lock()
	t.Calls++
	t.mu.Unlock()
	return fn(ctx)
}

// Audit satisfies every domain's auditor by keeping the entries in memory.
// A non-nil Err makes Record fail, to test that a failed record fails the
// change.
type Audit struct {
	mu      sync.Mutex
	Entries []domain.AuditEntry
	Err     error
}

// Record keeps e, or returns Err.
func (a *Audit) Record(_ context.Context, e domain.AuditEntry) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.Err != nil {
		return a.Err
	}
	a.Entries = append(a.Entries, e)
	return nil
}

// Actions lists the recorded actions in order.
func (a *Audit) Actions() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]string, 0, len(a.Entries))
	for _, e := range a.Entries {
		out = append(out, e.Action)
	}
	return out
}

// Last returns the most recent entry, or the zero value when there is none.
func (a *Audit) Last() domain.AuditEntry {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.Entries) == 0 {
		return domain.AuditEntry{}
	}
	return a.Entries[len(a.Entries)-1]
}
