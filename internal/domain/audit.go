package domain

import "time"

// AuditEntry is one change recorded in the audit trail: who did what to
// which entity. Every service writes one in the same transaction as the
// change itself, so a change is never stored without its record.
//
// Details holds what changed, as plain JSON values (strings, numbers, bools,
// nil); never passwords or their hashes.
type AuditEntry struct {
	ID         int64
	ActorID    *int64 // nil for the system (bootstrap, scheduler)
	Action     string // "<entity>.<verb>", e.g. "assignment.revoke"
	EntityType string // e.g. "subscription"
	EntityID   *int64
	Details    map[string]any
	// ActorName is read-only, filled in by reads.
	ActorName *string
	CreatedAt time.Time
}

// NewAuditEntry builds the entry for an action taken by a signed-in user.
func NewAuditEntry(actorID int64, action, entityType string, entityID int64, details map[string]any) AuditEntry {
	if details == nil {
		details = map[string]any{}
	}
	return AuditEntry{ActorID: &actorID, Action: action, EntityType: entityType, EntityID: &entityID, Details: details}
}

// AuditFilter narrows an audit list. Zero-value fields do not filter.
type AuditFilter struct {
	ActorID    *int64
	EntityType string
	EntityID   *int64
	Action     string
	// From and To bound created_at: From inclusive, To exclusive.
	From *time.Time
	To   *time.Time
}
