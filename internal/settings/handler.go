package settings

import (
	"fmt"
	"net/http"
	"time"

	"subtrack/internal/apperr"
	"subtrack/internal/auth"
	"subtrack/internal/domain"
	"subtrack/internal/httpx"
)

// handler serves /api/v1/settings/notifications.
type handler struct {
	svc *Service
}

// NotificationSettingsDTO is the notification settings as the client sees
// them.
type NotificationSettingsDTO struct {
	ExpiringDays int       `json:"expiring_days" example:"7"`
	PaymentDays  int       `json:"payment_days" example:"3"`
	UpdatedAt    time.Time `json:"updated_at" example:"2026-10-23T10:00:00Z"`
	UpdatedBy    *int64    `json:"updated_by" example:"1"`
}

func toDTO(st domain.NotificationSettings) NotificationSettingsDTO {
	return NotificationSettingsDTO{ExpiringDays: st.ExpiringDays, PaymentDays: st.PaymentDays, UpdatedAt: st.UpdatedAt, UpdatedBy: st.UpdatedBy}
}

// PutNotificationSettingsRequest is the body of PUT /settings/notifications.
// Both fields are required.
type PutNotificationSettingsRequest struct {
	ExpiringDays *int `json:"expiring_days" example:"7"`
	PaymentDays  *int `json:"payment_days" example:"3"`
}

// get godoc
//
//	@Summary		Notification settings (admin)
//	@Description	How many days before the end date a subscription becomes expiring_soon (its holders and the admins are warned), and how many days before the next payment the admins are reminded.
//	@Tags			settings
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	NotificationSettingsDTO
//	@Failure		401	{object}	httpx.ErrorBody
//	@Failure		403	{object}	httpx.ErrorBody
//	@Failure		500	{object}	httpx.ErrorBody
//	@Router			/settings/notifications [get]
func (h *handler) get(w http.ResponseWriter, r *http.Request) {
	st, err := h.svc.Get(r.Context())
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	httpx.WriteOK(r.Context(), w, toDTO(st))
}

// put godoc
//
//	@Summary		Change the notification settings (admin)
//	@Description	Both values are required, each 0..365 days (0 = only on the day itself). The scheduler uses the new values from its next run.
//	@Tags			settings
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			body	body		PutNotificationSettingsRequest	true	"New values"
//	@Success		200		{object}	NotificationSettingsDTO
//	@Failure		400		{object}	httpx.ErrorBody
//	@Failure		401		{object}	httpx.ErrorBody
//	@Failure		403		{object}	httpx.ErrorBody
//	@Failure		500		{object}	httpx.ErrorBody
//	@Router			/settings/notifications [put]
func (h *handler) put(w http.ResponseWriter, r *http.Request) {
	var req PutNotificationSettingsRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	if req.ExpiringDays == nil || req.PaymentDays == nil {
		writeErr(r.Context(), w, fmt.Errorf("%w: expiring_days and payment_days are both required", apperr.ErrInvalidInput))
		return
	}
	actor, _ := auth.UserFromContext(r.Context())
	st, err := h.svc.Update(r.Context(), actor, *req.ExpiringDays, *req.PaymentDays)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	httpx.WriteOK(r.Context(), w, toDTO(st))
}
