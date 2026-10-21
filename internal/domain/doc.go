// Package domain holds the entity types that flow through every layer of the
// service (storage, service, transport).
//
// It is deliberately dependency-free: domain imports nothing from this module
// and carries no behavior beyond plain field access and small value checks.
// Other layers depend on domain; domain never depends on them. This keeps the
// entity definitions usable as a shared vocabulary without inviting import
// cycles.
package domain
