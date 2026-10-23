package analytics

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"subtrack/internal/apperr"
	"subtrack/internal/auth"
	"subtrack/internal/domain"
	"subtrack/internal/httpx"
)

// handler serves /api/v1/analytics.
type handler struct {
	svc *Service
}

// monthLayout is how a month travels in the analytics API: "2026-10".
const monthLayout = "2006-01"

// PaymentStatsDTO is the admin's block of payment figures.
type PaymentStatsDTO struct {
	PaidThisMonthRUB string `json:"paid_this_month_rub" example:"84500.00"`
	PaidThisYearRUB  string `json:"paid_this_year_rub" example:"912000.00"`
	PendingCount     int    `json:"pending_count" example:"3"`
	PendingRUB       string `json:"pending_rub" example:"27000.00"`
	OverdueCount     int    `json:"overdue_count" example:"1"`
	FailedCount      int    `json:"failed_count" example:"0"`
}

// SummaryDTO is the dashboard's row of numbers. payments, recommendations
// and potential_savings_rub are present for the admin only.
type SummaryDTO struct {
	DepartmentID        *int64           `json:"department_id" example:"3"`
	ActiveSubscriptions int              `json:"active_subscriptions" example:"24"`
	ExpiringSoon        int              `json:"expiring_soon" example:"2"`
	Expired             int              `json:"expired" example:"1"`
	MonthlyCostRUB      string           `json:"monthly_cost_rub" example:"185000.00"`
	AnnualCostRUB       string           `json:"annual_cost_rub" example:"2220000.00"`
	SeatsTotal          int              `json:"seats_total" example:"140"`
	SeatsUsed           int              `json:"seats_used" example:"112"`
	SeatsFree           int              `json:"seats_free" example:"28"`
	UtilizationPct      *float64         `json:"utilization_pct" example:"80"`
	Enterprise          int              `json:"enterprise_subscriptions" example:"1"`
	Licences            int              `json:"licences" example:"160"`
	ActiveUsers         int              `json:"active_users" example:"95"`
	CostPerUserRUB      *string          `json:"cost_per_user_rub" example:"1947.37"`
	Payments            *PaymentStatsDTO `json:"payments,omitempty"`
	Recommendations     *int             `json:"recommendations,omitempty" example:"5"`
	PotentialSavingsRUB *string          `json:"potential_savings_rub,omitempty" example:"240000.00"`
}

// SliceDTO is one row of a breakdown by category or vendor; key is "" for
// subscriptions without one.
type SliceDTO struct {
	Key            string  `json:"key" example:"Разработка"`
	Subscriptions  int     `json:"subscriptions" example:"6"`
	MonthlyCostRUB string  `json:"monthly_cost_rub" example:"64000.00"`
	SharePct       float64 `json:"share_pct" example:"34.6"`
}

// BreakdownResponse is the body of GET /analytics/by-category and
// /analytics/by-vendor.
type BreakdownResponse struct {
	Items []SliceDTO `json:"items"`
}

// DepartmentRowDTO is one department; department_id null is the row of
// company-wide subscriptions.
type DepartmentRowDTO struct {
	DepartmentID   *int64   `json:"department_id" example:"3"`
	Name           string   `json:"name" example:"Разработка"`
	Subscriptions  int      `json:"subscriptions" example:"9"`
	MonthlyCostRUB string   `json:"monthly_cost_rub" example:"92000.00"`
	SeatsTotal     int      `json:"seats_total" example:"60"`
	SeatsUsed      int      `json:"seats_used" example:"48"`
	UtilizationPct *float64 `json:"utilization_pct" example:"80"`
	ActiveUsers    *int     `json:"active_users" example:"40"`
	CostPerUserRUB *string  `json:"cost_per_user_rub" example:"2300.00"`
}

// ByDepartmentResponse is the body of GET /analytics/by-department.
type ByDepartmentResponse struct {
	Items []DepartmentRowDTO `json:"items"`
}

// TopRowDTO is one of the most expensive subscriptions.
type TopRowDTO struct {
	SubscriptionID    int64    `json:"subscription_id" example:"12"`
	Name              string   `json:"name" example:"Jira"`
	Vendor            string   `json:"vendor" example:"Atlassian"`
	DepartmentName    *string  `json:"department_name" example:"Разработка"`
	Status            string   `json:"status" example:"active"`
	MonthlyCostRUB    string   `json:"monthly_cost_rub" example:"30000.00"`
	AnnualCostRUB     string   `json:"annual_cost_rub" example:"360000.00"`
	SeatsTotal        *int     `json:"seats_total" example:"50"`
	SeatsUsed         int      `json:"seats_used" example:"30"`
	UtilizationPct    *float64 `json:"utilization_pct" example:"60"`
	CostPerLicenceRUB *string  `json:"cost_per_licence_rub" example:"1000.00"`
}

// TopResponse is the body of GET /analytics/top-subscriptions.
type TopResponse struct {
	Items []TopRowDTO `json:"items"`
}

// SpendingMonthDTO is what was paid in one month.
type SpendingMonthDTO struct {
	Month     string `json:"month" example:"2026-10"`
	AmountRUB string `json:"amount_rub" example:"84500.00"`
	Payments  int    `json:"payments" example:"7"`
}

// SpendingResponse is the body of GET /analytics/spending.
type SpendingResponse struct {
	From     string             `json:"from" example:"2025-11"`
	To       string             `json:"to" example:"2026-10"`
	Months   []SpendingMonthDTO `json:"months"`
	TotalRUB string             `json:"total_rub" example:"912000.00"`
}

// ForecastMonthDTO is the expected spending of one month.
type ForecastMonthDTO struct {
	Month        string `json:"month" example:"2026-11"`
	ScheduledRUB string `json:"scheduled_rub" example:"150000.00"`
	Payments     int    `json:"payments" example:"12"`
	EstimatedRUB string `json:"estimated_rub" example:"5000.00"`
	TotalRUB     string `json:"total_rub" example:"155000.00"`
}

// ForecastResponse is the body of GET /analytics/forecast.
type ForecastResponse struct {
	Months []ForecastMonthDTO `json:"months"`
	// TotalRUB sums the months; RunRateRUB is what they would cost if every
	// active subscription were renewed (monthly cost × months).
	TotalRUB    string `json:"total_rub" example:"1860000.00"`
	RunRateRUB  string `json:"run_rate_rub" example:"2220000.00"`
	Unscheduled int    `json:"unscheduled" example:"1"`
}

// RecommendationDTO is one piece of advice. savings_rub_year is null when
// it depends on prices the system does not know.
type RecommendationDTO struct {
	Type            string  `json:"type" example:"unused_seats" enums:"unused_seats,no_holders,duplicate_vendor,individual_to_group,enterprise_low_usage,annual_billing,expired_with_holders"`
	SubscriptionIDs []int64 `json:"subscription_ids"`
	Title           string  `json:"title" example:"Лишние места: Jira"`
	Text            string  `json:"text" example:"Занято 30 из 50 мест Jira. Сократите 20 мест — сэкономите 120 000 ₽/год."`
	SavingsRUBYear  *string `json:"savings_rub_year" example:"120000.00"`
}

// RecommendationsResponse is the body of GET /analytics/recommendations.
type RecommendationsResponse struct {
	Items []RecommendationDTO `json:"items"`
	// TotalSavingsRUBYear sums the savings that could be computed.
	TotalSavingsRUBYear string `json:"total_savings_rub_year" example:"240000.00"`
}

// summary godoc
//
//	@Summary		Dashboard summary (admin, department head)
//	@Description	Counts, monthly and annual cost in roubles, seat usage, licences and cost per user. The admin may pick a department and also gets payment figures and the recommendations' count and savings; a head always gets their own department and no payment figures.
//	@Tags			analytics
//	@Produce		json
//	@Security		BearerAuth
//	@Param			department_id	query		int	false	"Admin: one department"
//	@Success		200				{object}	SummaryDTO
//	@Failure		400				{object}	httpx.ErrorBody
//	@Failure		401				{object}	httpx.ErrorBody
//	@Failure		403				{object}	httpx.ErrorBody
//	@Failure		500				{object}	httpx.ErrorBody
//	@Router			/analytics/summary [get]
func (h *handler) summary(w http.ResponseWriter, r *http.Request) {
	dept, err := queryID(r.URL.Query(), "department_id")
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	actor, _ := auth.UserFromContext(r.Context())
	s, err := h.svc.Summary(r.Context(), actor, dept)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	if actor.Role == domain.RoleManager {
		dept = actor.DepartmentID
	}
	out := SummaryDTO{
		DepartmentID: dept, ActiveSubscriptions: s.Active, ExpiringSoon: s.ExpiringSoon, Expired: s.Expired,
		MonthlyCostRUB: s.MonthlyCostRUB, AnnualCostRUB: s.AnnualCostRUB,
		SeatsTotal: s.SeatsTotal, SeatsUsed: s.SeatsUsed, SeatsFree: s.SeatsFree, UtilizationPct: s.Utilization,
		Enterprise: s.Enterprise, Licences: s.Licences, ActiveUsers: s.ActiveUsers, CostPerUserRUB: s.CostPerUserRUB,
		Recommendations: s.Recommendations, PotentialSavingsRUB: s.PotentialSavingsRUB,
	}
	if p := s.Payments; p != nil {
		out.Payments = &PaymentStatsDTO{PaidThisMonthRUB: p.PaidThisMonthRUB, PaidThisYearRUB: p.PaidThisYearRUB,
			PendingCount: p.PendingCount, PendingRUB: p.PendingRUB, OverdueCount: p.OverdueCount, FailedCount: p.FailedCount}
	}
	httpx.WriteOK(r.Context(), w, out)
}

// byCategory godoc
//
//	@Summary		Monthly cost by category (admin, department head)
//	@Description	Active subscriptions grouped by category (case-insensitively), most expensive first, with their share. A head gets their own department.
//	@Tags			analytics
//	@Produce		json
//	@Security		BearerAuth
//	@Param			department_id	query		int	false	"Admin: one department"
//	@Success		200				{object}	BreakdownResponse
//	@Failure		400				{object}	httpx.ErrorBody
//	@Failure		401				{object}	httpx.ErrorBody
//	@Failure		403				{object}	httpx.ErrorBody
//	@Failure		500				{object}	httpx.ErrorBody
//	@Router			/analytics/by-category [get]
func (h *handler) byCategory(w http.ResponseWriter, r *http.Request) {
	h.breakdown(w, r, h.svc.ByCategory)
}

// byVendor godoc
//
//	@Summary		Monthly cost by vendor (admin, department head)
//	@Description	Active subscriptions grouped by vendor (case-insensitively), most expensive first, with their share. A head gets their own department.
//	@Tags			analytics
//	@Produce		json
//	@Security		BearerAuth
//	@Param			department_id	query		int	false	"Admin: one department"
//	@Success		200				{object}	BreakdownResponse
//	@Failure		400				{object}	httpx.ErrorBody
//	@Failure		401				{object}	httpx.ErrorBody
//	@Failure		403				{object}	httpx.ErrorBody
//	@Failure		500				{object}	httpx.ErrorBody
//	@Router			/analytics/by-vendor [get]
func (h *handler) byVendor(w http.ResponseWriter, r *http.Request) {
	h.breakdown(w, r, h.svc.ByVendor)
}

func (h *handler) breakdown(w http.ResponseWriter, r *http.Request,
	get func(ctx context.Context, actor domain.User, dept *int64) ([]Slice, error)) {
	dept, err := queryID(r.URL.Query(), "department_id")
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	actor, _ := auth.UserFromContext(r.Context())
	items, err := get(r.Context(), actor, dept)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	out := BreakdownResponse{Items: make([]SliceDTO, 0, len(items))}
	for _, s := range items {
		out.Items = append(out.Items, SliceDTO{Key: s.Key, Subscriptions: s.Subscriptions, MonthlyCostRUB: s.MonthlyCostRUB, SharePct: s.SharePct})
	}
	httpx.WriteOK(r.Context(), w, out)
}

// byDepartment godoc
//
//	@Summary		Departments compared (admin)
//	@Description	Every department with its active subscriptions' monthly cost, seats, active users and cost per user, most expensive first; department_id null holds the company-wide subscriptions.
//	@Tags			analytics
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	ByDepartmentResponse
//	@Failure		401	{object}	httpx.ErrorBody
//	@Failure		403	{object}	httpx.ErrorBody
//	@Failure		500	{object}	httpx.ErrorBody
//	@Router			/analytics/by-department [get]
func (h *handler) byDepartment(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.ByDepartment(r.Context())
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	out := ByDepartmentResponse{Items: make([]DepartmentRowDTO, 0, len(rows))}
	for _, d := range rows {
		out.Items = append(out.Items, DepartmentRowDTO{DepartmentID: d.DepartmentID, Name: d.Name, Subscriptions: d.Subscriptions,
			MonthlyCostRUB: d.MonthlyCostRUB, SeatsTotal: d.SeatsTotal, SeatsUsed: d.SeatsUsed, UtilizationPct: d.Utilization,
			ActiveUsers: d.ActiveUsers, CostPerUserRUB: d.CostPerUserRUB})
	}
	httpx.WriteOK(r.Context(), w, out)
}

// topSubscriptions godoc
//
//	@Summary		Most expensive subscriptions (admin)
//	@Description	Active subscriptions by monthly cost, with seat usage and the monthly cost per licence handed out.
//	@Tags			analytics
//	@Produce		json
//	@Security		BearerAuth
//	@Param			department_id	query		int	false	"One department"
//	@Param			limit			query		int	false	"How many (default 10, max 50)"
//	@Success		200				{object}	TopResponse
//	@Failure		400				{object}	httpx.ErrorBody
//	@Failure		401				{object}	httpx.ErrorBody
//	@Failure		403				{object}	httpx.ErrorBody
//	@Failure		500				{object}	httpx.ErrorBody
//	@Router			/analytics/top-subscriptions [get]
func (h *handler) topSubscriptions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	dept, err := queryID(q, "department_id")
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	limit, err := queryInt(q, "limit")
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	rows, err := h.svc.Top(r.Context(), dept, limit)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	out := TopResponse{Items: make([]TopRowDTO, 0, len(rows))}
	for _, t := range rows {
		out.Items = append(out.Items, TopRowDTO{SubscriptionID: t.ID, Name: t.Name, Vendor: t.Vendor, DepartmentName: t.DepartmentName,
			Status: string(t.Status), MonthlyCostRUB: t.MonthlyCostRUB, AnnualCostRUB: t.AnnualCostRUB,
			SeatsTotal: t.SeatsTotal, SeatsUsed: t.SeatsUsed, UtilizationPct: t.Utilization, CostPerLicenceRUB: t.CostPerLicenceRUB})
	}
	httpx.WriteOK(r.Context(), w, out)
}

// spending godoc
//
//	@Summary		Paid by month (admin)
//	@Description	Paid payments summed by the month of their payment date, in roubles at the current rates; every month of the period is present. Default: the last 12 months; at most 60.
//	@Tags			analytics
//	@Produce		json
//	@Security		BearerAuth
//	@Param			department_id	query		int		false	"One department"
//	@Param			from			query		string	false	"First month, YYYY-MM"
//	@Param			to				query		string	false	"Last month, YYYY-MM (default: this month)"
//	@Success		200				{object}	SpendingResponse
//	@Failure		400				{object}	httpx.ErrorBody
//	@Failure		401				{object}	httpx.ErrorBody
//	@Failure		403				{object}	httpx.ErrorBody
//	@Failure		500				{object}	httpx.ErrorBody
//	@Router			/analytics/spending [get]
func (h *handler) spending(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	dept, err := queryID(q, "department_id")
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	from, err := queryMonth(q, "from")
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	to, err := queryMonth(q, "to")
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	rep, err := h.svc.Spending(r.Context(), dept, from, to)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	out := SpendingResponse{From: rep.From.Format(monthLayout), To: rep.To.Format(monthLayout),
		Months: make([]SpendingMonthDTO, 0, len(rep.Months)), TotalRUB: rep.TotalRUB}
	for _, m := range rep.Months {
		out.Months = append(out.Months, SpendingMonthDTO{Month: m.Month.Format(monthLayout), AmountRUB: m.AmountRUB, Payments: m.Payments})
	}
	httpx.WriteOK(r.Context(), w, out)
}

// forecast godoc
//
//	@Summary		Budget forecast (admin, department head)
//	@Description	Expected spending of the active subscriptions, month by month from this one: payments by next_payment_date and billing period up to the end date, plus the monthly cost of subscriptions without a next_payment_date (estimated). run_rate_rub is the cost if everything were renewed. A head gets their own department.
//	@Tags			analytics
//	@Produce		json
//	@Security		BearerAuth
//	@Param			department_id	query		int	false	"Admin: one department"
//	@Param			months			query		int	false	"How many months (default 12, max 36)"
//	@Success		200				{object}	ForecastResponse
//	@Failure		400				{object}	httpx.ErrorBody
//	@Failure		401				{object}	httpx.ErrorBody
//	@Failure		403				{object}	httpx.ErrorBody
//	@Failure		500				{object}	httpx.ErrorBody
//	@Router			/analytics/forecast [get]
func (h *handler) forecast(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	dept, err := queryID(q, "department_id")
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	months, err := queryInt(q, "months")
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	actor, _ := auth.UserFromContext(r.Context())
	f, err := h.svc.Forecast(r.Context(), actor, dept, months)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	out := ForecastResponse{Months: make([]ForecastMonthDTO, 0, len(f.Months)), TotalRUB: f.TotalRUB,
		RunRateRUB: f.RunRateRUB, Unscheduled: f.Unscheduled}
	for _, m := range f.Months {
		out.Months = append(out.Months, ForecastMonthDTO{Month: m.Month.Format(monthLayout), ScheduledRUB: m.ScheduledRUB,
			Payments: m.Payments, EstimatedRUB: m.EstimatedRUB, TotalRUB: m.TotalRUB})
	}
	httpx.WriteOK(r.Context(), w, out)
}

// recommendations godoc
//
//	@Summary		Savings recommendations (admin)
//	@Description	Advice from the rules: unused seats, nobody using a subscription, several subscriptions of one vendor, individual copies of one product, an unlimited licence used by few, monthly billing for over a year, an expired subscription with licences still out. Sorted by yearly saving; advice whose saving cannot be computed comes last with savings_rub_year null.
//	@Tags			analytics
//	@Produce		json
//	@Security		BearerAuth
//	@Param			department_id	query		int	false	"One department"
//	@Success		200				{object}	RecommendationsResponse
//	@Failure		400				{object}	httpx.ErrorBody
//	@Failure		401				{object}	httpx.ErrorBody
//	@Failure		403				{object}	httpx.ErrorBody
//	@Failure		500				{object}	httpx.ErrorBody
//	@Router			/analytics/recommendations [get]
func (h *handler) recommendations(w http.ResponseWriter, r *http.Request) {
	dept, err := queryID(r.URL.Query(), "department_id")
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	recs, total, err := h.svc.Recommendations(r.Context(), dept)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	out := RecommendationsResponse{Items: make([]RecommendationDTO, 0, len(recs)), TotalSavingsRUBYear: total}
	for _, x := range recs {
		out.Items = append(out.Items, RecommendationDTO{Type: string(x.Type), SubscriptionIDs: x.SubscriptionIDs,
			Title: x.Title, Text: x.Text, SavingsRUBYear: x.SavingsRUB})
	}
	httpx.WriteOK(r.Context(), w, out)
}

// queryID parses an optional positive id from a query parameter.
func queryID(q url.Values, name string) (*int64, error) {
	v := q.Get(name)
	if v == "" {
		return nil, nil
	}
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil || id <= 0 {
		return nil, fmt.Errorf("%w: %s must be a positive integer", apperr.ErrInvalidInput, name)
	}
	return &id, nil
}

// queryInt parses an optional positive integer; missing is 0.
func queryInt(q url.Values, name string) (int, error) {
	v := q.Get(name)
	if v == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%w: %s must be a positive integer", apperr.ErrInvalidInput, name)
	}
	return n, nil
}

// queryMonth parses an optional "YYYY-MM".
func queryMonth(q url.Values, name string) (*time.Time, error) {
	v := q.Get(name)
	if v == "" {
		return nil, nil
	}
	t, err := time.Parse(monthLayout, v)
	if err != nil {
		return nil, fmt.Errorf("%w: %s must be a month like 2026-10", apperr.ErrInvalidInput, name)
	}
	return &t, nil
}
