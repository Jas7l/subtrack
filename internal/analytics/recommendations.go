package analytics

import (
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

	"subtrack/internal/domain"
)

// RecommendationType names a rule of the recommendation engine.
type RecommendationType string

const (
	// RecUnusedSeats: a group subscription uses less than
	// seatUsageThreshold of its seats.
	RecUnusedSeats RecommendationType = "unused_seats"
	// RecNoHolders: nobody holds a licence of a subscription older than
	// noHoldersGrace.
	RecNoHolders RecommendationType = "no_holders"
	// RecDuplicateVendor: several active subscriptions of one vendor.
	RecDuplicateVendor RecommendationType = "duplicate_vendor"
	// RecIndividualToGroup: individualToGroupMin or more individual
	// subscriptions of one product.
	RecIndividualToGroup RecommendationType = "individual_to_group"
	// RecEnterpriseLowUsage: an unlimited subscription used by fewer than
	// enterpriseLowUsage people.
	RecEnterpriseLowUsage RecommendationType = "enterprise_low_usage"
	// RecAnnualBilling: paid monthly for more than annualBillingAfter.
	RecAnnualBilling RecommendationType = "annual_billing"
	// RecExpiredWithHolders: expired, yet people still hold licences.
	RecExpiredWithHolders RecommendationType = "expired_with_holders"
)

// The thresholds of the rules.
const (
	seatUsageThreshold   = 80 // percent of seats used, below which seats are surplus
	noHoldersGrace       = 30 * 24 * time.Hour
	individualToGroupMin = 3
	enterpriseLowUsage   = 10
	annualBillingAfter   = 12 // months
)

// Recommendation is one piece of advice. SavingsRUB is the saving over the
// next year - or until the end date, when the subscription ends sooner - when
// it can be computed from the data (unused seats, nobody using it), nil when
// it depends on prices the system does not know.
type Recommendation struct {
	Type            RecommendationType
	SubscriptionIDs []int64
	Title           string
	Text            string
	SavingsRUB      *string
	savings         *big.Rat
}

// recommend runs every rule over the scope's subscriptions. The result is
// sorted by saving, largest first; advice without a computed saving
// follows, in the order of the rules above.
func recommend(subs []sub, now time.Time) []Recommendation {
	var out []Recommendation
	withSavings := func(r Recommendation, s *big.Rat) Recommendation {
		r.savings = s
		v := money(s)
		r.SavingsRUB = &v
		return r
	}

	for _, s := range subs {
		if !s.active() {
			if s.Status == domain.SubExpired && s.SeatsUsed > 0 {
				out = append(out, Recommendation{
					Type: RecExpiredWithHolders, SubscriptionIDs: []int64{s.ID},
					Title: "Истекла, но лицензии выданы: " + s.Name,
					Text: fmt.Sprintf("Подписка «%s» истекла, а лицензии остались у %d чел. Продлите её или отзовите лицензии.",
						s.Name, s.SeatsUsed),
				})
			}
			continue
		}
		ahead, span := s.costAhead(now)
		switch {
		case s.SeatsUsed == 0 && now.Sub(s.CreatedAt) >= noHoldersGrace:
			out = append(out, withSavings(Recommendation{
				Type: RecNoHolders, SubscriptionIDs: []int64{s.ID},
				Title: "Никто не пользуется: " + s.Name,
				Text: fmt.Sprintf("У подписки «%s» нет ни одной выданной лицензии. Отмените её — сэкономите %s%s.",
					s.Name, rubText(ahead), span),
			}, ahead))
		case s.Type == domain.SubGroup && s.SeatsTotal != nil && s.SeatsUsed > 0 &&
			s.SeatsUsed*100 < *s.SeatsTotal*seatUsageThreshold:
			free := *s.SeatsTotal - s.SeatsUsed
			saving := frac(ahead, free, *s.SeatsTotal)
			out = append(out, withSavings(Recommendation{
				Type: RecUnusedSeats, SubscriptionIDs: []int64{s.ID},
				Title: fmt.Sprintf("Лишние места: %s", s.Name),
				Text: fmt.Sprintf("Занято %d из %d мест %s. Сократите %d %s — сэкономите %s%s.",
					s.SeatsUsed, *s.SeatsTotal, s.Name, free, plural(free, "место", "места", "мест"), rubText(saving), span),
			}, saving))
		case s.Type == domain.SubEnterprise && s.SeatsUsed > 0 && s.SeatsUsed < enterpriseLowUsage:
			out = append(out, Recommendation{
				Type: RecEnterpriseLowUsage, SubscriptionIDs: []int64{s.ID},
				Title: "Корпоративная лицензия почти не используется: " + s.Name,
				Text: fmt.Sprintf("Безлимитной подпиской «%s» пользуются %d чел. Групповая подписка на столько мест может обойтись дешевле.",
					s.Name, s.SeatsUsed),
			})
		}
		if s.Period == domain.BillingMonth && monthsBetween(s.StartDate, now) >= annualBillingAfter {
			out = append(out, Recommendation{
				Type: RecAnnualBilling, SubscriptionIDs: []int64{s.ID},
				Title: "Перейти на годовую оплату: " + s.Name,
				Text: fmt.Sprintf("«%s» оплачивается помесячно больше года (%s/год). Годовой тариф у большинства вендоров дешевле.",
					s.Name, rubText(s.annualRUB())),
			})
		}
	}

	out = append(out, groupRule(subs, func(s sub) (string, bool) {
		return strings.TrimSpace(s.Vendor), s.Vendor != ""
	}, 2, func(name string, g []sub) (Recommendation, bool) {
		// Copies of one product are individual_to_group's business.
		distinct := map[string]bool{}
		for _, s := range g {
			distinct[strings.ToLower(strings.TrimSpace(s.Name))] = true
		}
		return Recommendation{
			Type: RecDuplicateVendor, SubscriptionIDs: ids(g),
			Title: "Несколько подписок одного вендора: " + name,
			Text: fmt.Sprintf("У вендора %s %d активных подписок (%s, всего %s/год). Объедините их в одну — обычно это дешевле и проще.",
				name, len(g), names(g), rubText(annual(g))),
		}, len(distinct) > 1
	})...)
	out = append(out, groupRule(subs, func(s sub) (string, bool) {
		return strings.TrimSpace(s.Name), s.Type == domain.SubIndividual
	}, individualToGroupMin, func(name string, g []sub) (Recommendation, bool) {
		return Recommendation{
			Type: RecIndividualToGroup, SubscriptionIDs: ids(g),
			Title: "Перейти на групповую подписку: " + name,
			Text: fmt.Sprintf("Куплено %d индивидуальных подписок «%s» (%s/год). Групповая подписка на %d мест обычно дешевле.",
				len(g), name, rubText(annual(g)), len(g)),
		}, true
	})...)

	order := map[RecommendationType]int{RecUnusedSeats: 0, RecNoHolders: 1, RecDuplicateVendor: 2, RecIndividualToGroup: 3,
		RecEnterpriseLowUsage: 4, RecAnnualBilling: 5, RecExpiredWithHolders: 6}
	sort.SliceStable(out, func(a, b int) bool {
		sa, sb := out[a].savings, out[b].savings
		switch {
		case sa != nil && sb != nil:
			return sa.Cmp(sb) > 0
		case (sa != nil) != (sb != nil):
			return sa != nil
		}
		return order[out[a].Type] < order[out[b].Type]
	})
	return out
}

// groupRule groups the active subscriptions by key, case-insensitively
// (only those for which ok), and builds one recommendation per group of at
// least minSize for which build says ok, in the order the groups were first
// met.
func groupRule(subs []sub, key func(sub) (string, bool), minSize int, build func(name string, g []sub) (Recommendation, bool)) []Recommendation {
	var order []string
	groups := map[string][]sub{}
	for _, s := range subs {
		k, ok := key(s)
		if !s.active() || !ok || k == "" {
			continue
		}
		fold := strings.ToLower(k)
		if _, seen := groups[fold]; !seen {
			order = append(order, fold)
		}
		groups[fold] = append(groups[fold], s)
	}
	var out []Recommendation
	for _, fold := range order {
		g := groups[fold]
		if len(g) >= minSize {
			name, _ := key(g[0])
			if r, ok := build(name, g); ok {
				out = append(out, r)
			}
		}
	}
	return out
}

// totalSavings sums the computed savings.
func totalSavings(recs []Recommendation) *big.Rat {
	t := zero()
	for _, r := range recs {
		if r.savings != nil {
			t = add(t, r.savings)
		}
	}
	return t
}

func ids(g []sub) []int64 {
	out := make([]int64, 0, len(g))
	for _, s := range g {
		out = append(out, s.ID)
	}
	return out
}

func names(g []sub) string {
	out := make([]string, 0, len(g))
	for _, s := range g {
		out = append(out, "«"+s.Name+"»")
	}
	return strings.Join(out, ", ")
}

func annual(g []sub) *big.Rat {
	t := zero()
	for _, s := range g {
		t = add(t, s.annualRUB())
	}
	return t
}

// costAhead is what s costs over the next year, or until its end date when
// that comes sooner (a month begun counts whole), with how a sentence names
// that span: "/год" or " до окончания".
func (s sub) costAhead(now time.Time) (*big.Rat, string) {
	if s.EndDate == nil {
		return s.annualRUB(), "/год"
	}
	n := monthsBetween(now, *s.EndDate)
	if s.EndDate.Day() != now.Day() {
		n++
	}
	if n >= 12 {
		return s.annualRUB(), "/год"
	}
	return frac(s.annualRUB(), max(n, 0), 12), " до окончания"
}

// monthsBetween counts whole months from a to b.
func monthsBetween(a, b time.Time) int {
	n := (b.Year()-a.Year())*12 + int(b.Month()) - int(a.Month())
	if b.Day() < a.Day() {
		n--
	}
	return n
}

// plural picks the Russian word form for n: 1 место, 2 места, 5 мест.
func plural(n int, one, few, many string) string {
	if n%100 >= 11 && n%100 <= 14 {
		return many
	}
	switch n % 10 {
	case 1:
		return one
	case 2, 3, 4:
		return few
	}
	return many
}
