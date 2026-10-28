package subscription

import (
	"fmt"
	"net/http"
	"time"

	"subtrack/internal/apperr"
	"subtrack/internal/auth"
	"subtrack/internal/domain"
	"subtrack/internal/httpx"
)

// AssignmentDTO is a licence as the client sees it.
type AssignmentDTO struct {
	ID               int64      `json:"id" example:"40"`
	SubscriptionID   int64      `json:"subscription_id" example:"12"`
	SubscriptionName string     `json:"subscription_name" example:"Jira"`
	UserID           int64      `json:"user_id" example:"7"`
	UserName         string     `json:"user_name" example:"Петров Пётр Петрович"`
	Status           string     `json:"status" example:"active" enums:"active,revoked"`
	AssignedBy       *int64     `json:"assigned_by" example:"1"`
	AssignedByName   *string    `json:"assigned_by_name" example:"Администратор"`
	AssignedAt       time.Time  `json:"assigned_at" example:"2026-10-23T10:00:00Z"`
	RevokedBy        *int64     `json:"revoked_by" example:"1"`
	RevokedByName    *string    `json:"revoked_by_name" example:"Иванов Иван Сергеевич"`
	RevokedAt        *time.Time `json:"revoked_at" example:"2026-11-02T09:30:00Z"`
}

func toAssignmentDTO(a domain.Assignment) AssignmentDTO {
	return AssignmentDTO{
		ID:               a.ID,
		SubscriptionID:   a.SubscriptionID,
		SubscriptionName: a.SubscriptionName,
		UserID:           a.UserID,
		UserName:         a.UserName,
		Status:           string(a.Status),
		AssignedBy:       a.AssignedBy,
		AssignedByName:   a.AssignedByName,
		AssignedAt:       a.AssignedAt,
		RevokedBy:        a.RevokedBy,
		RevokedByName:    a.RevokedByName,
		RevokedAt:        a.RevokedAt,
	}
}

// AssignRequest is the body of POST /assignments.
type AssignRequest struct {
	SubscriptionID int64 `json:"subscription_id" example:"12"`
	UserID         int64 `json:"user_id" example:"7"`
}

// ListAssignmentsResponse is the body of GET /assignments.
type ListAssignmentsResponse struct {
	Items []AssignmentDTO `json:"items"`
	httpx.PageMeta
}

// listAssignments godoc
//
//	@Summary		List licences
//	@Description	Admin: all. Manager: those of their department's subscriptions plus their own. Employee: their own. Newest first.
//	@Tags			assignments
//	@Produce		json
//	@Security		BearerAuth
//	@Param			subscription_id	query		int		false	"Only this subscription's"
//	@Param			user_id			query		int		false	"Only this user's"
//	@Param			status			query		string	false	"Only this status"	Enums(active, revoked)
//	@Param			limit			query		int		false	"Page size (default 50, max 200)"
//	@Param			offset			query		int		false	"Page offset"
//	@Success		200				{object}	ListAssignmentsResponse
//	@Failure		400				{object}	httpx.ErrorBody
//	@Failure		401				{object}	httpx.ErrorBody
//	@Failure		500				{object}	httpx.ErrorBody
//	@Router			/assignments [get]
func (h *handler) listAssignments(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var f domain.AssignmentFilter
	var err error
	if f.SubscriptionID, err = queryID(q.Get("subscription_id"), "subscription_id"); err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	if f.UserID, err = queryID(q.Get("user_id"), "user_id"); err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	if v := q.Get("status"); v != "" {
		st := domain.AssignmentStatus(v)
		if !st.Valid() {
			writeErr(r.Context(), w, fmt.Errorf("%w: status must be active or revoked", apperr.ErrInvalidInput))
			return
		}
		f.Status = &st
	}
	params := httpx.ParseListParams(r)
	actor, _ := auth.UserFromContext(r.Context())

	items, total, err := h.svc.ListAssignments(r.Context(), actor, f, params)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	params.Normalize()
	out := ListAssignmentsResponse{
		Items:    make([]AssignmentDTO, 0, len(items)),
		PageMeta: httpx.PageMeta{Total: total, Limit: params.Limit, Offset: params.Offset},
	}
	for _, a := range items {
		out.Items = append(out.Items, toAssignmentDTO(a))
	}
	httpx.WriteOK(r.Context(), w, out)
}

// getAssignment godoc
//
//	@Summary		Get a licence
//	@Description	Visible as in the list; anything else is a 404.
//	@Tags			assignments
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Assignment id"
//	@Success		200	{object}	AssignmentDTO
//	@Failure		400	{object}	httpx.ErrorBody
//	@Failure		401	{object}	httpx.ErrorBody
//	@Failure		404	{object}	httpx.ErrorBody
//	@Failure		500	{object}	httpx.ErrorBody
//	@Router			/assignments/{id} [get]
func (h *handler) getAssignment(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	actor, _ := auth.UserFromContext(r.Context())
	a, err := h.svc.GetAssignment(r.Context(), actor, id)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	httpx.WriteOK(r.Context(), w, toAssignmentDTO(a))
}

// assign godoc
//
//	@Summary		Hand out a licence (admin, manager)
//	@Description	Admin: any active or expiring_soon subscription to any active user. Manager: only their department's subscriptions, only to people of their department. Takes one seat.
//	@Tags			assignments
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			body	body		AssignRequest	true	"Who gets which subscription"
//	@Success		201		{object}	AssignmentDTO
//	@Failure		400		{object}	httpx.ErrorBody
//	@Failure		401		{object}	httpx.ErrorBody
//	@Failure		403		{object}	httpx.ErrorBody
//	@Failure		404		{object}	httpx.ErrorBody
//	@Failure		409		{object}	httpx.ErrorBody
//	@Failure		500		{object}	httpx.ErrorBody
//	@Router			/assignments [post]
func (h *handler) assign(w http.ResponseWriter, r *http.Request) {
	var req AssignRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	if req.SubscriptionID <= 0 || req.UserID <= 0 {
		writeErr(r.Context(), w, fmt.Errorf("%w: subscription_id and user_id are required", apperr.ErrInvalidInput))
		return
	}
	actor, _ := auth.UserFromContext(r.Context())
	a, err := h.svc.Assign(r.Context(), actor, req.SubscriptionID, req.UserID)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	httpx.WriteCreated(r.Context(), w, toAssignmentDTO(a))
}

// revoke godoc
//
//	@Summary		Revoke a licence
//	@Description	Frees the seat; the row stays as history. Allowed to the admin, the manager of the subscription's department, and the holder (giving it back).
//	@Tags			assignments
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Assignment id"
//	@Success		200	{object}	AssignmentDTO
//	@Failure		400	{object}	httpx.ErrorBody
//	@Failure		401	{object}	httpx.ErrorBody
//	@Failure		404	{object}	httpx.ErrorBody
//	@Failure		409	{object}	httpx.ErrorBody
//	@Failure		500	{object}	httpx.ErrorBody
//	@Router			/assignments/{id}/revoke [post]
func (h *handler) revoke(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	actor, _ := auth.UserFromContext(r.Context())
	a, err := h.svc.Revoke(r.Context(), actor, id)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	httpx.WriteOK(r.Context(), w, toAssignmentDTO(a))
}
