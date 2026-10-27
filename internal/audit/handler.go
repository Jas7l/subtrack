package audit

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"subtrack/internal/apperr"
	"subtrack/internal/domain"
	"subtrack/internal/httpx"
)

// handler serves /api/v1/audit.
type handler struct {
	svc *Service
}

// EntryDTO is one audit record as the client sees it.
type EntryDTO struct {
	ID         int64          `json:"id" example:"301"`
	ActorID    *int64         `json:"actor_id" example:"1"`
	ActorName  *string        `json:"actor_name" example:"Иванов Иван Иванович"`
	Action     string         `json:"action" example:"assignment.revoke"`
	EntityType string         `json:"entity_type" example:"assignment"`
	EntityID   *int64         `json:"entity_id" example:"40"`
	Details    map[string]any `json:"details" swaggertype:"object"`
	CreatedAt  time.Time      `json:"created_at" example:"2026-10-23T10:00:00Z"`
}

// ListAuditResponse is the body of GET /audit.
type ListAuditResponse struct {
	Items []EntryDTO `json:"items"`
	httpx.PageMeta
}

// list godoc
//
//	@Summary		Audit trail (admin)
//	@Description	Who changed what, newest first. actor_id null means the system. Actions are "<entity>.<verb>", e.g. user.update, subscription.delete, assignment.revoke, request.approve.
//	@Tags			audit
//	@Produce		json
//	@Security		BearerAuth
//	@Param			actor_id	query		int		false	"Only this user's actions"
//	@Param			entity_type	query		string	false	"Only this entity type"	Enums(user, department, subscription, assignment, request, transaction, settings, currency)
//	@Param			entity_id	query		int		false	"Only this entity (with entity_type)"
//	@Param			action		query		string	false	"Only this action"
//	@Param			from		query		string	false	"From this day (YYYY-MM-DD), inclusive"
//	@Param			to			query		string	false	"To this day (YYYY-MM-DD), inclusive"
//	@Param			limit		query		int		false	"Page size (default 50, max 200)"
//	@Param			offset		query		int		false	"Page offset"
//	@Success		200			{object}	ListAuditResponse
//	@Failure		400			{object}	httpx.ErrorBody
//	@Failure		401			{object}	httpx.ErrorBody
//	@Failure		403			{object}	httpx.ErrorBody
//	@Failure		500			{object}	httpx.ErrorBody
//	@Router			/audit [get]
func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	f, err := parseFilter(r)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	params := httpx.ParseListParams(r)
	items, total, err := h.svc.List(r.Context(), f, params)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	params.Normalize()
	out := ListAuditResponse{
		Items:    make([]EntryDTO, 0, len(items)),
		PageMeta: httpx.PageMeta{Total: total, Limit: params.Limit, Offset: params.Offset},
	}
	for _, e := range items {
		out.Items = append(out.Items, EntryDTO{
			ID: e.ID, ActorID: e.ActorID, ActorName: e.ActorName, Action: e.Action,
			EntityType: e.EntityType, EntityID: e.EntityID, Details: e.Details, CreatedAt: e.CreatedAt,
		})
	}
	httpx.WriteOK(r.Context(), w, out)
}

// parseFilter reads the list filters. A malformed filter is a 400.
func parseFilter(r *http.Request) (domain.AuditFilter, error) {
	q := r.URL.Query()
	f := domain.AuditFilter{EntityType: q.Get("entity_type"), Action: q.Get("action")}
	for _, p := range []struct {
		key string
		dst **int64
	}{{"actor_id", &f.ActorID}, {"entity_id", &f.EntityID}} {
		v := q.Get(p.key)
		if v == "" {
			continue
		}
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			return f, fmt.Errorf("%w: %s must be a positive integer", apperr.ErrInvalidInput, p.key)
		}
		*p.dst = &id
	}
	var err error
	if f.From, err = httpx.QueryDate(q, "from"); err != nil {
		return f, err
	}
	if f.To, err = httpx.QueryDate(q, "to"); err != nil {
		return f, err
	}
	if f.To != nil {
		// "to" names a whole day; the store's bound is exclusive.
		next := f.To.AddDate(0, 0, 1)
		f.To = &next
	}
	return f, nil
}
