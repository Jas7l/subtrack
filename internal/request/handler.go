package request

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

// handler serves /api/v1/requests.
type handler struct {
	svc *Service
}

// RequestDTO is a request as the client sees it.
type RequestDTO struct {
	ID               int64      `json:"id" example:"15"`
	RequesterID      int64      `json:"requester_id" example:"7"`
	RequesterName    string     `json:"requester_name" example:"Петров Пётр Петрович"`
	DepartmentID     *int64     `json:"department_id" example:"3"`
	DepartmentName   *string    `json:"department_name" example:"Разработка"`
	Name             string     `json:"name" example:"Figma"`
	Vendor           string     `json:"vendor" example:"Figma Inc."`
	Justification    string     `json:"justification" example:"Нужна для макетов нового интерфейса"`
	EstimatedCost    *string    `json:"estimated_cost" example:"1500.00"`
	Currency         string     `json:"currency" example:"RUB"`
	SubscriptionID   *int64     `json:"subscription_id" example:"12"`
	Status           string     `json:"status" example:"pending_manager" enums:"pending_manager,rejected_by_manager,pending_admin,rejected_by_admin,approved,cancelled"`
	ManagerID        *int64     `json:"manager_id" example:"5"`
	ManagerComment   string     `json:"manager_comment" example:"Согласовано"`
	ManagerDecidedAt *time.Time `json:"manager_decided_at" example:"2026-10-23T12:00:00Z"`
	AdminID          *int64     `json:"admin_id" example:"1"`
	AdminComment     string     `json:"admin_comment" example:""`
	AdminDecidedAt   *time.Time `json:"admin_decided_at" example:"2026-10-24T09:00:00Z"`
	CreatedAt        time.Time  `json:"created_at" example:"2026-10-23T10:00:00Z"`
	UpdatedAt        time.Time  `json:"updated_at" example:"2026-10-23T10:00:00Z"`
}

func toDTO(r domain.Request) RequestDTO {
	return RequestDTO{
		ID:               r.ID,
		RequesterID:      r.RequesterID,
		RequesterName:    r.RequesterName,
		DepartmentID:     r.DepartmentID,
		DepartmentName:   r.DepartmentName,
		Name:             r.Name,
		Vendor:           r.Vendor,
		Justification:    r.Justification,
		EstimatedCost:    r.EstimatedCost,
		Currency:         r.Currency,
		SubscriptionID:   r.SubscriptionID,
		Status:           string(r.Status),
		ManagerID:        r.ManagerID,
		ManagerComment:   r.ManagerComment,
		ManagerDecidedAt: r.ManagerDecidedAt,
		AdminID:          r.AdminID,
		AdminComment:     r.AdminComment,
		AdminDecidedAt:   r.AdminDecidedAt,
		CreatedAt:        r.CreatedAt,
		UpdatedAt:        r.UpdatedAt,
	}
}

// CreateRequestRequest is the body of POST /requests.
type CreateRequestRequest struct {
	Name          string       `json:"name" example:"Figma"`
	Vendor        string       `json:"vendor,omitempty" example:"Figma Inc."`
	Justification string       `json:"justification" example:"Нужна для макетов нового интерфейса"`
	EstimatedCost *json.Number `json:"estimated_cost,omitempty" swaggertype:"string" example:"1500.00"`
	Currency      string       `json:"currency,omitempty" example:"RUB"`
}

// NewSubscriptionBody is a subscription the admin creates while approving.
// name and vendor default to the request's; department_id defaults to the
// request's department (null: company-wide).
type NewSubscriptionBody struct {
	Name            string           `json:"name,omitempty" example:"Figma"`
	Vendor          string           `json:"vendor,omitempty" example:"Figma Inc."`
	Category        string           `json:"category,omitempty" example:"Дизайн"`
	Type            string           `json:"type" example:"group" enums:"individual,group,enterprise"`
	Cost            json.Number      `json:"cost" swaggertype:"string" example:"15000.00"`
	Currency        string           `json:"currency" example:"RUB"`
	BillingPeriod   string           `json:"billing_period" example:"month" enums:"month,quarter,year"`
	StartDate       httpx.Date       `json:"start_date" swaggertype:"string" format:"date" example:"2026-11-01"`
	EndDate         *httpx.Date      `json:"end_date,omitempty" swaggertype:"string" format:"date" example:"2027-10-31"`
	NextPaymentDate *httpx.Date      `json:"next_payment_date,omitempty" swaggertype:"string" format:"date" example:"2026-12-01"`
	DepartmentID    httpx.OptionalID `json:"department_id" swaggertype:"integer" extensions:"x-nullable" example:"3"`
	SeatsTotal      *int             `json:"seats_total,omitempty" example:"5"`
}

// ApproveRequest is the body of POST /requests/{id}/approve. The department
// head sends at most a comment. The admin also picks the subscription the
// requester gets a licence of: exactly one of subscription_id (an existing
// one) and subscription (a new one).
type ApproveRequest struct {
	Comment        string               `json:"comment,omitempty" example:"Согласовано"`
	SubscriptionID *int64               `json:"subscription_id,omitempty" example:"12"`
	Subscription   *NewSubscriptionBody `json:"subscription,omitempty"`
}

// RejectRequest is the body of POST /requests/{id}/reject.
type RejectRequest struct {
	Comment string `json:"comment" example:"У отдела уже есть Canva, используйте её"`
}

// ListRequestsResponse is the body of GET /requests.
type ListRequestsResponse struct {
	Items []RequestDTO `json:"items"`
	httpx.PageMeta
}

// list godoc
//
//	@Summary		List requests
//	@Description	Admin: all. Manager: their department's plus their own (incoming ones: ?status=pending_manager). Employee: their own. Newest first.
//	@Tags			requests
//	@Produce		json
//	@Security		BearerAuth
//	@Param			status			query		string	false	"Only this status"	Enums(pending_manager, rejected_by_manager, pending_admin, rejected_by_admin, approved, cancelled)
//	@Param			requester_id	query		int		false	"Only this requester's"
//	@Param			department_id	query		int		false	"Only this department's"
//	@Param			limit			query		int		false	"Page size (default 50, max 200)"
//	@Param			offset			query		int		false	"Page offset"
//	@Success		200				{object}	ListRequestsResponse
//	@Failure		400				{object}	httpx.ErrorBody
//	@Failure		401				{object}	httpx.ErrorBody
//	@Failure		500				{object}	httpx.ErrorBody
//	@Router			/requests [get]
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
	out := ListRequestsResponse{
		Items:    make([]RequestDTO, 0, len(items)),
		PageMeta: httpx.PageMeta{Total: total, Limit: params.Limit, Offset: params.Offset},
	}
	for _, x := range items {
		out.Items = append(out.Items, toDTO(x))
	}
	httpx.WriteOK(r.Context(), w, out)
}

// get godoc
//
//	@Summary		Get a request
//	@Description	Visible as in the list; anything else is a 404.
//	@Tags			requests
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Request id"
//	@Success		200	{object}	RequestDTO
//	@Failure		400	{object}	httpx.ErrorBody
//	@Failure		401	{object}	httpx.ErrorBody
//	@Failure		404	{object}	httpx.ErrorBody
//	@Failure		500	{object}	httpx.ErrorBody
//	@Router			/requests/{id} [get]
func (h *handler) get(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	actor, _ := auth.UserFromContext(r.Context())
	out, err := h.svc.Get(r.Context(), actor, id)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	httpx.WriteOK(r.Context(), w, toDTO(out))
}

// create godoc
//
//	@Summary		Request a subscription (employee, manager)
//	@Description	An employee's request goes to the head of their department, or straight to the admin when there is no head; a manager's goes to the admin. name and justification are required; currency defaults to RUB.
//	@Tags			requests
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			body	body		CreateRequestRequest	true	"What is needed and why"
//	@Success		201		{object}	RequestDTO
//	@Failure		400		{object}	httpx.ErrorBody
//	@Failure		401		{object}	httpx.ErrorBody
//	@Failure		403		{object}	httpx.ErrorBody
//	@Failure		500		{object}	httpx.ErrorBody
//	@Router			/requests [post]
func (h *handler) create(w http.ResponseWriter, r *http.Request) {
	var req CreateRequestRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	in := CreateRequestInput{
		Name:          req.Name,
		Vendor:        req.Vendor,
		Justification: req.Justification,
		Currency:      req.Currency,
	}
	if req.EstimatedCost != nil {
		c := req.EstimatedCost.String()
		in.EstimatedCost = &c
	}
	actor, _ := auth.UserFromContext(r.Context())
	out, err := h.svc.Create(r.Context(), actor, in)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	httpx.WriteCreated(r.Context(), w, toDTO(out))
}

// approve godoc
//
//	@Summary		Approve a request (department head, admin)
//	@Description	The department head moves a pending_manager request to pending_admin. The admin approves a pending_admin request (or a pending_manager one when the department has no head) and picks the subscription: an existing one (subscription_id) or a new one (subscription), and the requester gets a licence of it - all in one transaction. If the licence cannot be handed out (no free seats, cancelled subscription, the requester already holds it) the answer is 409 and the request stays pending.
//	@Tags			requests
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int				true	"Request id"
//	@Param			body	body		ApproveRequest	true	"Comment and, for the admin, the subscription"
//	@Success		200		{object}	RequestDTO
//	@Failure		400		{object}	httpx.ErrorBody
//	@Failure		401		{object}	httpx.ErrorBody
//	@Failure		403		{object}	httpx.ErrorBody
//	@Failure		404		{object}	httpx.ErrorBody
//	@Failure		409		{object}	httpx.ErrorBody
//	@Failure		500		{object}	httpx.ErrorBody
//	@Router			/requests/{id}/approve [post]
func (h *handler) approve(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	var req ApproveRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	v := Verdict{Approve: true, Comment: req.Comment, SubscriptionID: req.SubscriptionID}
	if s := req.Subscription; s != nil {
		v.NewSubscription = &NewSubscription{
			Subscription: domain.Subscription{
				Name:            s.Name,
				Vendor:          s.Vendor,
				Category:        s.Category,
				Type:            domain.SubscriptionType(s.Type),
				Cost:            s.Cost.String(),
				Currency:        s.Currency,
				BillingPeriod:   domain.BillingPeriod(s.BillingPeriod),
				StartDate:       time.Time(s.StartDate),
				EndDate:         datePtr(s.EndDate),
				NextPaymentDate: datePtr(s.NextPaymentDate),
				SeatsTotal:      s.SeatsTotal,
			},
			DepartmentID: domain.IDPatch(s.DepartmentID),
		}
	}
	h.decide(w, r, id, v)
}

// reject godoc
//
//	@Summary		Reject a request (department head, admin)
//	@Description	The department head rejects at pending_manager, the admin at pending_admin (or at pending_manager when the department has no head). A comment is required.
//	@Tags			requests
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int				true	"Request id"
//	@Param			body	body		RejectRequest	true	"Why"
//	@Success		200		{object}	RequestDTO
//	@Failure		400		{object}	httpx.ErrorBody
//	@Failure		401		{object}	httpx.ErrorBody
//	@Failure		403		{object}	httpx.ErrorBody
//	@Failure		404		{object}	httpx.ErrorBody
//	@Failure		409		{object}	httpx.ErrorBody
//	@Failure		500		{object}	httpx.ErrorBody
//	@Router			/requests/{id}/reject [post]
func (h *handler) reject(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	var req RejectRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	h.decide(w, r, id, Verdict{Comment: req.Comment})
}

func (h *handler) decide(w http.ResponseWriter, r *http.Request, id int64, v Verdict) {
	actor, _ := auth.UserFromContext(r.Context())
	out, err := h.svc.Decide(r.Context(), actor, id, v)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	httpx.WriteOK(r.Context(), w, toDTO(out))
}

// cancel godoc
//
//	@Summary		Cancel own request
//	@Description	Only the requester, only while the request is pending.
//	@Tags			requests
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Request id"
//	@Success		200	{object}	RequestDTO
//	@Failure		400	{object}	httpx.ErrorBody
//	@Failure		401	{object}	httpx.ErrorBody
//	@Failure		403	{object}	httpx.ErrorBody
//	@Failure		404	{object}	httpx.ErrorBody
//	@Failure		409	{object}	httpx.ErrorBody
//	@Failure		500	{object}	httpx.ErrorBody
//	@Router			/requests/{id}/cancel [post]
func (h *handler) cancel(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	actor, _ := auth.UserFromContext(r.Context())
	out, err := h.svc.Cancel(r.Context(), actor, id)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	httpx.WriteOK(r.Context(), w, toDTO(out))
}

// parseFilter reads the list filters. A malformed filter is a 400.
func parseFilter(r *http.Request) (domain.RequestFilter, error) {
	q := r.URL.Query()
	var f domain.RequestFilter
	if v := q.Get("status"); v != "" {
		st := domain.RequestStatus(v)
		if !st.Valid() {
			return f, fmt.Errorf("%w: unknown status %q", apperr.ErrInvalidInput, v)
		}
		f.Status = &st
	}
	var err error
	if f.RequesterID, err = queryID(q.Get("requester_id"), "requester_id"); err != nil {
		return f, err
	}
	if f.DepartmentID, err = queryID(q.Get("department_id"), "department_id"); err != nil {
		return f, err
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
