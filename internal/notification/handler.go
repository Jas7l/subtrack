package notification

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

// handler serves /api/v1/notifications.
type handler struct {
	svc *Service
}

// NotificationDTO is a notification as the client sees it.
type NotificationDTO struct {
	ID        int64     `json:"id" example:"88"`
	Type      string    `json:"type" example:"request_approved" enums:"subscription_expiring,subscription_expired,payment_due,request_created,request_approved,request_rejected"`
	Title     string    `json:"title" example:"Заявка одобрена: Figma"`
	Body      string    `json:"body" example:"Вам выдана лицензия Figma."`
	IsRead    bool      `json:"is_read" example:"false"`
	CreatedAt time.Time `json:"created_at" example:"2026-10-23T10:00:00Z"`
}

func toDTO(n domain.Notification) NotificationDTO {
	return NotificationDTO{ID: n.ID, Type: string(n.Type), Title: n.Title, Body: n.Body, IsRead: n.IsRead, CreatedAt: n.CreatedAt}
}

// ListNotificationsResponse is the body of GET /notifications.
type ListNotificationsResponse struct {
	Items []NotificationDTO `json:"items"`
	// Unread counts all unread notifications, whatever the page.
	Unread int64 `json:"unread" example:"3"`
	httpx.PageMeta
}

// MarkAllReadResponse is the body of POST /notifications/read-all.
type MarkAllReadResponse struct {
	Marked int64 `json:"marked" example:"3"`
}

// list godoc
//
//	@Summary		My notifications
//	@Description	The caller's own notifications, newest first, with the unread count.
//	@Tags			notifications
//	@Produce		json
//	@Security		BearerAuth
//	@Param			unread	query		bool	false	"true: only unread"
//	@Param			limit	query		int		false	"Page size (default 50, max 200)"
//	@Param			offset	query		int		false	"Page offset"
//	@Success		200		{object}	ListNotificationsResponse
//	@Failure		400		{object}	httpx.ErrorBody
//	@Failure		401		{object}	httpx.ErrorBody
//	@Failure		500		{object}	httpx.ErrorBody
//	@Router			/notifications [get]
func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	unreadOnly := false
	if v := r.URL.Query().Get("unread"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			writeErr(r.Context(), w, fmt.Errorf("%w: unread must be true or false", apperr.ErrInvalidInput))
			return
		}
		unreadOnly = b
	}
	params := httpx.ParseListParams(r)
	actor, _ := auth.UserFromContext(r.Context())
	items, total, unread, err := h.svc.List(r.Context(), actor, unreadOnly, params)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	params.Normalize()
	out := ListNotificationsResponse{
		Items:    make([]NotificationDTO, 0, len(items)),
		Unread:   unread,
		PageMeta: httpx.PageMeta{Total: total, Limit: params.Limit, Offset: params.Offset},
	}
	for _, n := range items {
		out.Items = append(out.Items, toDTO(n))
	}
	httpx.WriteOK(r.Context(), w, out)
}

// markRead godoc
//
//	@Summary		Mark a notification read
//	@Tags			notifications
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Notification id"
//	@Success		200	{object}	NotificationDTO
//	@Failure		400	{object}	httpx.ErrorBody
//	@Failure		401	{object}	httpx.ErrorBody
//	@Failure		404	{object}	httpx.ErrorBody
//	@Failure		500	{object}	httpx.ErrorBody
//	@Router			/notifications/{id}/read [post]
func (h *handler) markRead(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	actor, _ := auth.UserFromContext(r.Context())
	n, err := h.svc.MarkRead(r.Context(), actor, id)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	httpx.WriteOK(r.Context(), w, toDTO(n))
}

// markAllRead godoc
//
//	@Summary		Mark all my notifications read
//	@Tags			notifications
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	MarkAllReadResponse
//	@Failure		401	{object}	httpx.ErrorBody
//	@Failure		500	{object}	httpx.ErrorBody
//	@Router			/notifications/read-all [post]
func (h *handler) markAllRead(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth.UserFromContext(r.Context())
	n, err := h.svc.MarkAllRead(r.Context(), actor)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	httpx.WriteOK(r.Context(), w, MarkAllReadResponse{Marked: n})
}
