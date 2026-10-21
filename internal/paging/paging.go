// Package paging holds the standard pagination input shared by every List
// endpoint: a Params value plus the sort direction it may carry. It has no
// dependencies so both the service side (domain packages) and the transport
// side (httpx) can share one vocabulary without import-direction gymnastics.
package paging

// SortOrder is the direction of an `ORDER BY created_at` applied by List
// endpoints that support ordering. It is a closed set: Normalize coerces any
// value other than SortAsc to SortDesc, so a repository never sees an
// unvetted string and the transport layer can hand a raw query param straight
// through without its own validation.
type SortOrder string

const (
	// SortDesc orders newest-first (created_at DESC). It is the default when
	// the caller omits or mis-spells the order param.
	SortDesc SortOrder = "desc"
	// SortAsc orders oldest-first (created_at ASC).
	SortAsc SortOrder = "asc"
)

// Params is the standard pagination input for List endpoints. Values are
// normalized via Normalize before being handed to a repository.
type Params struct {
	Limit  int
	Offset int
	// Order selects the created_at sort direction for endpoints that honor
	// it. Endpoints that ignore it are
	// unaffected. Normalize guarantees it is one of SortAsc / SortDesc.
	Order SortOrder
}

const (
	defaultListLimit = 50
	maxListLimit     = 200
)

// Normalize clamps Limit/Offset into a sane window and pins Order to a known
// direction. Repositories should never receive a zero or negative limit, nor
// an Order outside {SortAsc, SortDesc}.
func (p *Params) Normalize() {
	if p.Limit <= 0 {
		p.Limit = defaultListLimit
	}
	if p.Limit > maxListLimit {
		p.Limit = maxListLimit
	}
	if p.Offset < 0 {
		p.Offset = 0
	}
	// Whitelist the sort direction: anything that is not an explicit ascending
	// request collapses to the newest-first default. This keeps a bad/empty
	// query param from ever reaching the SQL builder.
	if p.Order != SortAsc {
		p.Order = SortDesc
	}
}
