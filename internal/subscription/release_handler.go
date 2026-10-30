package subscription

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"subtrack/internal/apperr"
	"subtrack/internal/auth"
	"subtrack/internal/domain"
	"subtrack/internal/httpx"
)

// ReleaseRequestDTO is a request to give a licence back as the client sees
// it. department_id is the subscription's department now (null: a
// company-wide subscription, the admin's to decide).
type ReleaseRequestDTO struct {
	ID               int64      `json:"id" example:"5"`
	AssignmentID     int64      `json:"assignment_id" example:"40"`
	SubscriptionID   int64      `json:"subscription_id" example:"12"`
	SubscriptionName string     `json:"subscription_name" example:"Jira"`
	DepartmentID     *int64     `json:"department_id" example:"1"`
	DepartmentName   *string    `json:"department_name" example:"Разработка"`
	RequesterID      int64      `json:"requester_id" example:"7"`
	RequesterName    string     `json:"requester_name" example:"Смирнов Алексей Петрович"`
	Reason           string     `json:"reason" example:"Перешёл в отдел тестирования, Jira больше не нужна"`
	Status           string     `json:"status" example:"pending" enums:"pending,approved,rejected,cancelled"`
	DecidedBy        *int64     `json:"decided_by" example:"2"`
	DecidedByName    *string    `json:"decided_by_name" example:"Иванов Иван Сергеевич"`
	Comment          string     `json:"comment" example:""`
	DecidedAt        *time.Time `json:"decided_at" example:"2026-10-30T12:00:00Z"`
	CreatedAt        time.Time  `json:"created_at" example:"2026-10-30T10:00:00Z"`
	UpdatedAt        time.Time  `json:"updated_at" example:"2026-10-30T12:00:00Z"`
}

func toReleaseDTO(r domain.ReleaseRequest) ReleaseRequestDTO {
	return ReleaseRequestDTO{
		ID:               r.ID,
		AssignmentID:     r.AssignmentID,
		SubscriptionID:   r.SubscriptionID,
		SubscriptionName: r.SubscriptionName,
		DepartmentID:     r.DepartmentID,
		DepartmentName:   r.DepartmentName,
		RequesterID:      r.RequesterID,
		RequesterName:    r.RequesterName,
		Reason:           r.Reason,
		Status:           string(r.Status),
		DecidedBy:        r.DecidedBy,
		DecidedByName:    r.DecidedByName,
		Comment:          r.Comment,
		DecidedAt:        r.DecidedAt,
		CreatedAt:        r.CreatedAt,
		UpdatedAt:        r.UpdatedAt,
	}
}

// CreateReleaseRequest is the body of POST /release-requests.
type CreateReleaseRequest struct {
	AssignmentID int64  `json:"assignment_id" example:"40"`
	Reason       string `json:"reason,omitempty" example:"Перешёл в отдел тестирования, Jira больше не нужна"`
}

// ApproveReleaseRequest is the body of POST /release-requests/{id}/approve.
type ApproveReleaseRequest struct {
	Comment string `json:"comment,omitempty" example:"Место отдадим новому сотруднику"`
}

// RejectReleaseRequest is the body of POST /release-requests/{id}/reject.
type RejectReleaseRequest struct {
	Comment string `json:"comment" example:"Jira нужна для отчётов по задачам до конца квартала"`
}

// ListReleaseRequestsResponse is the body of GET /release-requests.
type ListReleaseRequestsResponse struct {
	Items []ReleaseRequestDTO `json:"items"`
	httpx.PageMeta
}

// listReleases godoc
//
//	@Summary		List release requests
//	@Description	Requests to give a licence back. Admin: all. Manager: those about their department's subscriptions plus their own. Employee: their own. Newest first. inbox=true keeps the pending ones waiting for the caller: a manager's department's; for the admin only those nobody else can decide (a company-wide subscription, a department without an active manager).
//	@Tags			release-requests
//	@Produce		json
//	@Security		BearerAuth
//	@Param			status			query		string	false	"Only this status"	Enums(pending, approved, rejected, cancelled)
//	@Param			requester_id	query		int		false	"Only this requester's"
//	@Param			subscription_id	query		int		false	"Only about this subscription"
//	@Param			department_id	query		int		false	"Only about subscriptions of this department"
//	@Param			inbox			query		bool	false	"Only the pending ones waiting for the caller"
//	@Param			limit			query		int		false	"Page size (default 50, max 200)"
//	@Param			offset			query		int		false	"Page offset"
//	@Success		200				{object}	ListReleaseRequestsResponse
//	@Failure		400				{object}	httpx.ErrorBody
//	@Failure		401				{object}	httpx.ErrorBody
//	@Failure		500				{object}	httpx.ErrorBody
//	@Router			/release-requests [get]
func (h *handler) listReleases(w http.ResponseWriter, r *http.Request) {
	f, inbox, err := parseReleaseFilter(r)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	params := httpx.ParseListParams(r)
	actor, _ := auth.UserFromContext(r.Context())

	items, total, err := h.svc.ListReleases(r.Context(), actor, f, inbox, params)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	params.Normalize()
	out := ListReleaseRequestsResponse{
		Items:    make([]ReleaseRequestDTO, 0, len(items)),
		PageMeta: httpx.PageMeta{Total: total, Limit: params.Limit, Offset: params.Offset},
	}
	for _, it := range items {
		out.Items = append(out.Items, toReleaseDTO(it))
	}
	httpx.WriteOK(r.Context(), w, out)
}

// getRelease godoc
//
//	@Summary		Get a release request
//	@Description	Visible as in the list; anything else is a 404.
//	@Tags			release-requests
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Release request id"
//	@Success		200	{object}	ReleaseRequestDTO
//	@Failure		400	{object}	httpx.ErrorBody
//	@Failure		401	{object}	httpx.ErrorBody
//	@Failure		404	{object}	httpx.ErrorBody
//	@Failure		500	{object}	httpx.ErrorBody
//	@Router			/release-requests/{id} [get]
func (h *handler) getRelease(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	actor, _ := auth.UserFromContext(r.Context())
	out, err := h.svc.GetRelease(r.Context(), actor, id)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	httpx.WriteOK(r.Context(), w, toReleaseDTO(out))
}

// createRelease godoc
//
//	@Summary		Ask to give a licence back (employee, manager)
//	@Description	Only the holder, only for an active licence of a subscription they do not manage (otherwise 409: revoke it yourself), one waiting request per licence. Whoever manages the subscription decides: a manager of its department, or the admin; they are notified (the admins when the subscription is company-wide or its department has no active manager). reason is optional.
//	@Tags			release-requests
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			body	body		CreateReleaseRequest	true	"Which licence and why"
//	@Success		201		{object}	ReleaseRequestDTO
//	@Failure		400		{object}	httpx.ErrorBody
//	@Failure		401		{object}	httpx.ErrorBody
//	@Failure		403		{object}	httpx.ErrorBody
//	@Failure		404		{object}	httpx.ErrorBody
//	@Failure		409		{object}	httpx.ErrorBody
//	@Failure		500		{object}	httpx.ErrorBody
//	@Router			/release-requests [post]
func (h *handler) createRelease(w http.ResponseWriter, r *http.Request) {
	var req CreateReleaseRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	if req.AssignmentID <= 0 {
		writeErr(r.Context(), w, fmt.Errorf("%w: assignment_id is required", apperr.ErrInvalidInput))
		return
	}
	actor, _ := auth.UserFromContext(r.Context())
	out, err := h.svc.FileRelease(r.Context(), actor, req.AssignmentID, req.Reason)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	httpx.WriteCreated(r.Context(), w, toReleaseDTO(out))
}

// approveRelease godoc
//
//	@Summary		Approve a release request (manager, admin)
//	@Description	A manager of the subscription's department or the admin. Revokes the licence in the same transaction. The comment is optional.
//	@Tags			release-requests
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int						true	"Release request id"
//	@Param			body	body		ApproveReleaseRequest	true	"Comment (may be empty: {})"
//	@Success		200		{object}	ReleaseRequestDTO
//	@Failure		400		{object}	httpx.ErrorBody
//	@Failure		401		{object}	httpx.ErrorBody
//	@Failure		403		{object}	httpx.ErrorBody
//	@Failure		404		{object}	httpx.ErrorBody
//	@Failure		409		{object}	httpx.ErrorBody
//	@Failure		500		{object}	httpx.ErrorBody
//	@Router			/release-requests/{id}/approve [post]
func (h *handler) approveRelease(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	var req ApproveReleaseRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	h.decideRelease(w, r, id, domain.ReleaseDecision{Approve: true, Comment: req.Comment})
}

// rejectRelease godoc
//
//	@Summary		Reject a release request (manager, admin)
//	@Description	A manager of the subscription's department or the admin. The licence stays with its holder. A comment is required.
//	@Tags			release-requests
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int						true	"Release request id"
//	@Param			body	body		RejectReleaseRequest	true	"Why"
//	@Success		200		{object}	ReleaseRequestDTO
//	@Failure		400		{object}	httpx.ErrorBody
//	@Failure		401		{object}	httpx.ErrorBody
//	@Failure		403		{object}	httpx.ErrorBody
//	@Failure		404		{object}	httpx.ErrorBody
//	@Failure		409		{object}	httpx.ErrorBody
//	@Failure		500		{object}	httpx.ErrorBody
//	@Router			/release-requests/{id}/reject [post]
func (h *handler) rejectRelease(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	var req RejectReleaseRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	h.decideRelease(w, r, id, domain.ReleaseDecision{Comment: req.Comment})
}

func (h *handler) decideRelease(w http.ResponseWriter, r *http.Request, id int64, d domain.ReleaseDecision) {
	actor, _ := auth.UserFromContext(r.Context())
	out, err := h.svc.DecideRelease(r.Context(), actor, id, d)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	httpx.WriteOK(r.Context(), w, toReleaseDTO(out))
}

// cancelRelease godoc
//
//	@Summary		Cancel own release request
//	@Description	Only the requester, only while the request is pending. The licence stays.
//	@Tags			release-requests
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Release request id"
//	@Success		200	{object}	ReleaseRequestDTO
//	@Failure		400	{object}	httpx.ErrorBody
//	@Failure		401	{object}	httpx.ErrorBody
//	@Failure		403	{object}	httpx.ErrorBody
//	@Failure		404	{object}	httpx.ErrorBody
//	@Failure		409	{object}	httpx.ErrorBody
//	@Failure		500	{object}	httpx.ErrorBody
//	@Router			/release-requests/{id}/cancel [post]
func (h *handler) cancelRelease(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	actor, _ := auth.UserFromContext(r.Context())
	out, err := h.svc.CancelRelease(r.Context(), actor, id)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	httpx.WriteOK(r.Context(), w, toReleaseDTO(out))
}

// parseReleaseFilter reads the list filters and the inbox flag. A malformed
// one is a 400.
func parseReleaseFilter(r *http.Request) (domain.ReleaseFilter, bool, error) {
	q := r.URL.Query()
	var f domain.ReleaseFilter
	if v := q.Get("status"); v != "" {
		st := domain.ReleaseStatus(v)
		if !st.Valid() {
			return f, false, fmt.Errorf("%w: status must be pending, approved, rejected or cancelled", apperr.ErrInvalidInput)
		}
		f.Status = &st
	}
	var err error
	if f.RequesterID, err = queryID(q.Get("requester_id"), "requester_id"); err != nil {
		return f, false, err
	}
	if f.SubscriptionID, err = queryID(q.Get("subscription_id"), "subscription_id"); err != nil {
		return f, false, err
	}
	if f.DepartmentID, err = queryID(q.Get("department_id"), "department_id"); err != nil {
		return f, false, err
	}
	inbox := false
	if v := q.Get("inbox"); v != "" {
		if inbox, err = strconv.ParseBool(v); err != nil {
			return f, false, fmt.Errorf("%w: inbox must be true or false", apperr.ErrInvalidInput)
		}
	}
	return f, inbox, nil
}
