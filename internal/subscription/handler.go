package subscription

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"subtrack/internal/apperr"
	"subtrack/internal/auth"
	"subtrack/internal/domain"
	"subtrack/internal/httpx"
)

// handler serves /api/v1/subscriptions and /api/v1/assignments.
type handler struct {
	svc *Service
}

// SubscriptionDTO is a subscription as the client sees it.
type SubscriptionDTO struct {
	ID              int64       `json:"id" example:"12"`
	Name            string      `json:"name" example:"Jira"`
	Vendor          string      `json:"vendor" example:"Atlassian"`
	Category        string      `json:"category" example:"Разработка"`
	Type            string      `json:"type" example:"group" enums:"individual,group,enterprise"`
	Cost            string      `json:"cost" example:"15000.00"`
	Currency        string      `json:"currency" example:"RUB"`
	BillingPeriod   string      `json:"billing_period" example:"month" enums:"month,quarter,year"`
	StartDate       httpx.Date  `json:"start_date" swaggertype:"string" format:"date" example:"2026-01-01"`
	EndDate         *httpx.Date `json:"end_date" swaggertype:"string" format:"date" example:"2026-12-31"`
	NextPaymentDate *httpx.Date `json:"next_payment_date" swaggertype:"string" format:"date" example:"2026-11-01"`
	Status          string      `json:"status" example:"active" enums:"active,expiring_soon,expired,cancelled"`
	DepartmentID    *int64      `json:"department_id" example:"3"`
	DepartmentName  *string     `json:"department_name" example:"Разработка"`
	// SeatsTotal and SeatsFree are null for enterprise (unlimited).
	SeatsTotal *int      `json:"seats_total" example:"20"`
	SeatsUsed  int       `json:"seats_used" example:"14"`
	SeatsFree  *int      `json:"seats_free" example:"6"`
	CreatedBy  *int64    `json:"created_by" example:"1"`
	CreatedAt  time.Time `json:"created_at" example:"2026-10-21T10:00:00Z"`
	UpdatedAt  time.Time `json:"updated_at" example:"2026-10-21T10:00:00Z"`
}

func toDTO(s domain.Subscription) SubscriptionDTO {
	out := SubscriptionDTO{
		ID:              s.ID,
		Name:            s.Name,
		Vendor:          s.Vendor,
		Category:        s.Category,
		Type:            string(s.Type),
		Cost:            s.Cost,
		Currency:        s.Currency,
		BillingPeriod:   string(s.BillingPeriod),
		StartDate:       httpx.Date(s.StartDate),
		EndDate:         httpx.DatePtr(s.EndDate),
		NextPaymentDate: httpx.DatePtr(s.NextPaymentDate),
		Status:          string(s.Status),
		DepartmentID:    s.DepartmentID,
		DepartmentName:  s.DepartmentName,
		SeatsTotal:      s.SeatsTotal,
		SeatsUsed:       s.SeatsUsed,
		CreatedBy:       s.CreatedBy,
		CreatedAt:       s.CreatedAt,
		UpdatedAt:       s.UpdatedAt,
	}
	if s.SeatsTotal != nil {
		free := *s.SeatsTotal - s.SeatsUsed
		out.SeatsFree = &free
	}
	return out
}

// CreateSubscriptionRequest is the body of POST /subscriptions. cost may be
// sent as a number or a string; seats_total is required for group, optional
// (always 1) for individual and must be omitted for enterprise.
type CreateSubscriptionRequest struct {
	Name            string      `json:"name" example:"Jira"`
	Vendor          string      `json:"vendor,omitempty" example:"Atlassian"`
	Category        string      `json:"category,omitempty" example:"Разработка"`
	Type            string      `json:"type" example:"group" enums:"individual,group,enterprise"`
	Cost            json.Number `json:"cost" swaggertype:"string" example:"15000.00"`
	Currency        string      `json:"currency" example:"RUB"`
	BillingPeriod   string      `json:"billing_period" example:"month" enums:"month,quarter,year"`
	StartDate       httpx.Date  `json:"start_date" swaggertype:"string" format:"date" example:"2026-01-01"`
	EndDate         *httpx.Date `json:"end_date,omitempty" swaggertype:"string" format:"date" example:"2026-12-31"`
	NextPaymentDate *httpx.Date `json:"next_payment_date,omitempty" swaggertype:"string" format:"date" example:"2026-11-01"`
	DepartmentID    *int64      `json:"department_id,omitempty" example:"3"`
	SeatsTotal      *int        `json:"seats_total,omitempty" example:"20"`
}

// UpdateSubscriptionRequest is the body of PATCH /subscriptions/{id}. Omit a
// field to leave it unchanged; null clears end_date, next_payment_date or
// department_id.
type UpdateSubscriptionRequest struct {
	Name            *string            `json:"name,omitempty" example:"Jira"`
	Vendor          *string            `json:"vendor,omitempty" example:"Atlassian"`
	Category        *string            `json:"category,omitempty" example:"Разработка"`
	Type            *string            `json:"type,omitempty" example:"group" enums:"individual,group,enterprise"`
	Cost            *json.Number       `json:"cost,omitempty" swaggertype:"string" example:"15000.00"`
	Currency        *string            `json:"currency,omitempty" example:"RUB"`
	BillingPeriod   *string            `json:"billing_period,omitempty" example:"month" enums:"month,quarter,year"`
	StartDate       *httpx.Date        `json:"start_date,omitempty" swaggertype:"string" format:"date" example:"2026-01-01"`
	EndDate         httpx.OptionalDate `json:"end_date" swaggertype:"string" format:"date" extensions:"x-nullable" example:"2026-12-31"`
	NextPaymentDate httpx.OptionalDate `json:"next_payment_date" swaggertype:"string" format:"date" extensions:"x-nullable" example:"2026-11-01"`
	Status          *string            `json:"status,omitempty" example:"cancelled" enums:"active,cancelled"`
	DepartmentID    httpx.OptionalID   `json:"department_id" swaggertype:"integer" extensions:"x-nullable" example:"3"`
	SeatsTotal      *int               `json:"seats_total,omitempty" example:"25"`
}

// ListSubscriptionsResponse is the body of GET /subscriptions.
type ListSubscriptionsResponse struct {
	Items []SubscriptionDTO `json:"items"`
	httpx.PageMeta
}

// list godoc
//
//	@Summary		List subscriptions
//	@Description	Admin: all. Manager: their department's plus the ones they hold a licence of. Employee: the ones they hold a licence of. Deleted subscriptions are never listed. Sorted by name.
//	@Tags			subscriptions
//	@Produce		json
//	@Security		BearerAuth
//	@Param			department_id	query		int		false	"Only this department's"
//	@Param			category		query		string	false	"Whole category, case-insensitive"
//	@Param			status			query		string	false	"Only this status"	Enums(active, expiring_soon, expired, cancelled)
//	@Param			type			query		string	false	"Only this type"	Enums(individual, group, enterprise)
//	@Param			end_from		query		string	false	"end_date on or after (YYYY-MM-DD)"
//	@Param			end_to			query		string	false	"end_date on or before (YYYY-MM-DD)"
//	@Param			q				query		string	false	"Substring of the name or vendor"
//	@Param			free_seats		query		bool	false	"true: a seat limit with seats left over (unused seats); false: every seat taken. Enterprise matches neither"
//	@Param			limit			query		int		false	"Page size (default 50, max 200)"
//	@Param			offset			query		int		false	"Page offset"
//	@Success		200				{object}	ListSubscriptionsResponse
//	@Failure		400				{object}	httpx.ErrorBody
//	@Failure		401				{object}	httpx.ErrorBody
//	@Failure		500				{object}	httpx.ErrorBody
//	@Router			/subscriptions [get]
func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	f, err := parseFilter(r)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	params := httpx.ParseListParams(r)
	actor, _ := auth.UserFromContext(r.Context())

	items, total, err := h.svc.List(r.Context(), actor, f, params)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	params.Normalize()
	out := ListSubscriptionsResponse{
		Items:    make([]SubscriptionDTO, 0, len(items)),
		PageMeta: httpx.PageMeta{Total: total, Limit: params.Limit, Offset: params.Offset},
	}
	for _, s := range items {
		out.Items = append(out.Items, toDTO(s))
	}
	httpx.WriteOK(r.Context(), w, out)
}

// get godoc
//
//	@Summary		Get a subscription
//	@Description	Visible as in the list; anything else is a 404.
//	@Tags			subscriptions
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Subscription id"
//	@Success		200	{object}	SubscriptionDTO
//	@Failure		400	{object}	httpx.ErrorBody
//	@Failure		401	{object}	httpx.ErrorBody
//	@Failure		404	{object}	httpx.ErrorBody
//	@Failure		500	{object}	httpx.ErrorBody
//	@Router			/subscriptions/{id} [get]
func (h *handler) get(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	actor, _ := auth.UserFromContext(r.Context())
	s, err := h.svc.Get(r.Context(), actor, id)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	httpx.WriteOK(r.Context(), w, toDTO(s))
}

// create godoc
//
//	@Summary		Create a subscription (admin)
//	@Description	Starts active with no licences handed out. individual always has 1 seat; group needs seats_total; enterprise is unlimited (no seats_total). The currency must exist in the currencies table.
//	@Tags			subscriptions
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			body	body		CreateSubscriptionRequest	true	"Subscription fields"
//	@Success		201		{object}	SubscriptionDTO
//	@Failure		400		{object}	httpx.ErrorBody
//	@Failure		401		{object}	httpx.ErrorBody
//	@Failure		403		{object}	httpx.ErrorBody
//	@Failure		500		{object}	httpx.ErrorBody
//	@Router			/subscriptions [post]
func (h *handler) create(w http.ResponseWriter, r *http.Request) {
	var req CreateSubscriptionRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	actor, _ := auth.UserFromContext(r.Context())
	s, err := h.svc.Create(r.Context(), actor, domain.Subscription{
		Name:            req.Name,
		Vendor:          req.Vendor,
		Category:        req.Category,
		Type:            domain.SubscriptionType(req.Type),
		Cost:            req.Cost.String(),
		Currency:        req.Currency,
		BillingPeriod:   domain.BillingPeriod(req.BillingPeriod),
		StartDate:       time.Time(req.StartDate),
		EndDate:         datePtr(req.EndDate),
		NextPaymentDate: datePtr(req.NextPaymentDate),
		DepartmentID:    req.DepartmentID,
		SeatsTotal:      req.SeatsTotal,
	})
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	httpx.WriteCreated(r.Context(), w, toDTO(s))
}

// update godoc
//
//	@Summary		Update a subscription (admin)
//	@Description	Partial update; at least one field. status may be set to active or cancelled only (expiring_soon and expired are set by the scheduler); cancelling revokes every licence. seats_total cannot go below seats_used (409). Changing type without seats_total resets the seats: 1 for individual, unlimited for enterprise, group then needs seats_total.
//	@Tags			subscriptions
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int							true	"Subscription id"
//	@Param			body	body		UpdateSubscriptionRequest	true	"Fields to change"
//	@Success		200		{object}	SubscriptionDTO
//	@Failure		400		{object}	httpx.ErrorBody
//	@Failure		401		{object}	httpx.ErrorBody
//	@Failure		403		{object}	httpx.ErrorBody
//	@Failure		404		{object}	httpx.ErrorBody
//	@Failure		409		{object}	httpx.ErrorBody
//	@Failure		500		{object}	httpx.ErrorBody
//	@Router			/subscriptions/{id} [patch]
func (h *handler) update(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	var req UpdateSubscriptionRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	p := domain.SubscriptionPatch{
		Name:            req.Name,
		Vendor:          req.Vendor,
		Category:        req.Category,
		Currency:        req.Currency,
		EndDate:         domain.DatePatch(req.EndDate),
		NextPaymentDate: domain.DatePatch(req.NextPaymentDate),
		DepartmentID:    domain.IDPatch(req.DepartmentID),
		SeatsTotal:      req.SeatsTotal,
	}
	if req.Type != nil {
		t := domain.SubscriptionType(*req.Type)
		p.Type = &t
	}
	if req.Cost != nil {
		c := req.Cost.String()
		p.Cost = &c
	}
	if req.BillingPeriod != nil {
		bp := domain.BillingPeriod(*req.BillingPeriod)
		p.BillingPeriod = &bp
	}
	if req.StartDate != nil {
		sd := time.Time(*req.StartDate)
		p.StartDate = &sd
	}
	if req.Status != nil {
		st := domain.SubscriptionStatus(*req.Status)
		p.Status = &st
	}
	actor, _ := auth.UserFromContext(r.Context())
	s, err := h.svc.Update(r.Context(), actor, id, p)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	httpx.WriteOK(r.Context(), w, toDTO(s))
}

// delete godoc
//
//	@Summary		Delete a subscription (admin)
//	@Description	Soft delete: the row stays for payment history, but is no longer listed. Every licence is revoked.
//	@Tags			subscriptions
//	@Security		BearerAuth
//	@Param			id	path	int	true	"Subscription id"
//	@Success		204
//	@Failure		400	{object}	httpx.ErrorBody
//	@Failure		401	{object}	httpx.ErrorBody
//	@Failure		403	{object}	httpx.ErrorBody
//	@Failure		404	{object}	httpx.ErrorBody
//	@Failure		500	{object}	httpx.ErrorBody
//	@Router			/subscriptions/{id} [delete]
func (h *handler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	actor, _ := auth.UserFromContext(r.Context())
	if err := h.svc.Delete(r.Context(), actor, id); err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	httpx.WriteNoContent(w)
}

// parseFilter reads the list filters. A malformed filter is a 400: silently
// dropping it would return a wider list than the client asked for.
func parseFilter(r *http.Request) (domain.SubscriptionFilter, error) {
	q := r.URL.Query()
	f := domain.SubscriptionFilter{Query: q.Get("q"), Category: q.Get("category")}
	var err error
	if f.DepartmentID, err = queryID(q.Get("department_id"), "department_id"); err != nil {
		return f, err
	}
	if v := q.Get("status"); v != "" {
		st := domain.SubscriptionStatus(v)
		if !st.Valid() {
			return f, fmt.Errorf("%w: status must be one of active, expiring_soon, expired, cancelled", apperr.ErrInvalidInput)
		}
		f.Status = &st
	}
	if v := q.Get("type"); v != "" {
		t := domain.SubscriptionType(v)
		if !t.Valid() {
			return f, fmt.Errorf("%w: type must be one of individual, group, enterprise", apperr.ErrInvalidInput)
		}
		f.Type = &t
	}
	if f.EndFrom, err = httpx.QueryDate(q, "end_from"); err != nil {
		return f, err
	}
	if f.EndTo, err = httpx.QueryDate(q, "end_to"); err != nil {
		return f, err
	}
	if v := q.Get("free_seats"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return f, fmt.Errorf("%w: free_seats must be true or false", apperr.ErrInvalidInput)
		}
		f.FreeSeats = &b
	}
	return f, nil
}

// queryID parses an optional positive id from a query parameter.
func queryID(v, name string) (*int64, error) {
	if v == "" {
		return nil, nil
	}
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil || id <= 0 {
		return nil, fmt.Errorf("%w: %s must be a positive integer", apperr.ErrInvalidInput, name)
	}
	return &id, nil
}

func datePtr(d *httpx.Date) *time.Time {
	if d == nil {
		return nil
	}
	t := time.Time(*d)
	return &t
}
