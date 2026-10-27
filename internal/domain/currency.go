package domain

import "time"

// BaseCurrency is the reporting currency: every report converts to it, and
// its rate is fixed at 1 (currencies_rub_rate_check).
const BaseCurrency = "RUB"

// Currency is a currency amounts may be recorded in, with the rate to
// roubles the admin maintains by hand. Reports always use the current rate;
// no history is kept.
type Currency struct {
	Code string // ISO 4217, upper-case: "USD"
	Name string
	// RateToRUB is how many roubles one unit costs, as NUMERIC text without
	// trailing zeros ("92.5").
	RateToRUB string
	UpdatedAt time.Time
	UpdatedBy *int64
}

// CurrencyPatch is a partial update of a currency; nil fields stay as they
// are.
type CurrencyPatch struct {
	Name      *string
	RateToRUB *string
}

// HasAny reports whether the patch changes anything.
func (p CurrencyPatch) HasAny() bool {
	return p.Name != nil || p.RateToRUB != nil
}
