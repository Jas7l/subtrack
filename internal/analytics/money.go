package analytics

import (
	"math/big"
	"strings"
	"time"

	"subtrack/internal/domain"
)

// Money is exact: amounts are big.Rat values built from the NUMERIC text
// Postgres returns, and are written back as decimal strings with two
// places. Nothing passes through a float.

func zero() *big.Rat { return new(big.Rat) }

// rat parses a decimal string from the database; a malformed one (never
// expected from NUMERIC) counts as zero rather than failing a whole report.
func rat(s string) *big.Rat {
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return zero()
	}
	return r
}

func add(a, b *big.Rat) *big.Rat { return new(big.Rat).Add(a, b) }
func mul(a, b *big.Rat) *big.Rat { return new(big.Rat).Mul(a, b) }
func mulInt(a *big.Rat, n int) *big.Rat {
	return new(big.Rat).Mul(a, new(big.Rat).SetInt64(int64(n)))
}

// frac is a * num / den.
func frac(a *big.Rat, num, den int) *big.Rat {
	if den == 0 {
		return zero()
	}
	return new(big.Rat).Mul(a, big.NewRat(int64(num), int64(den)))
}

// money writes an amount the way the API returns it: "120000.00".
func money(r *big.Rat) string { return r.FloatString(2) }

// rubText writes an amount for a sentence, rounded to whole roubles with
// thin groups: "120 000 ₽".
func rubText(r *big.Rat) string {
	s := r.FloatString(0)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteRune(' ')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String() + " ₽"
	}
	return b.String() + " ₽"
}

// pct is part/whole in percent with one decimal, or nil when whole is 0.
func pct(part, whole int) *float64 {
	if whole == 0 {
		return nil
	}
	v := float64((part*1000+whole/2)/whole) / 10
	return &v
}

// sharePct is part/whole of two amounts in percent with one decimal.
func sharePct(part, whole *big.Rat) float64 {
	if whole.Sign() == 0 {
		return 0
	}
	f, _ := new(big.Rat).Quo(mulInt(part, 1000), whole).Float64()
	return float64(int64(f+0.5)) / 10
}

// sub is a subscription as the reports see it: its price already paired
// with the current rate of its currency.
type sub struct {
	ID              int64
	Name            string
	Vendor          string
	Category        string
	Type            domain.SubscriptionType
	Period          domain.BillingPeriod
	Cost            *big.Rat // per period, in Currency
	Currency        string
	Rate            *big.Rat // roubles per unit of Currency
	StartDate       time.Time
	EndDate         *time.Time
	NextPaymentDate *time.Time
	Status          domain.SubscriptionStatus
	DepartmentID    *int64
	DepartmentName  *string
	SeatsTotal      *int // nil for enterprise
	SeatsUsed       int
	CreatedAt       time.Time
}

// active reports whether the subscription is running (active or
// expiring_soon).
func (s sub) active() bool { return s.Status.Open() }

// periodRUB is the price of one billing period in roubles.
func (s sub) periodRUB() *big.Rat { return mul(s.Cost, s.Rate) }

// monthlyRUB is the price spread over a month.
func (s sub) monthlyRUB() *big.Rat { return frac(s.periodRUB(), 1, s.Period.Months()) }

// annualRUB is the price of a year.
func (s sub) annualRUB() *big.Rat { return frac(s.periodRUB(), 12, s.Period.Months()) }
