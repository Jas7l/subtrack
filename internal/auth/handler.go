package auth

import (
	"net/http"
	"time"

	"subtrack/internal/domain"
	"subtrack/internal/httpx"
	"subtrack/internal/logger"
)

// handler serves /api/v1/auth: sign-in, "who am I", and changing one's own
// password.
type handler struct {
	svc *Service
}

// LoginRequest is the body of POST /auth/login.
type LoginRequest struct {
	Email    string `json:"email" example:"admin@company.ru"`
	Password string `json:"password" example:"change-me-now"`
}

// LoginResponse is returned by a successful login. The client sends Token back
// as "Authorization: Bearer <token>" on every other request.
type LoginResponse struct {
	Token     string    `json:"token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
	TokenType string    `json:"token_type" example:"Bearer"`
	ExpiresAt time.Time `json:"expires_at" example:"2026-10-22T08:00:00Z"`
	User      MeDTO     `json:"user"`
}

// MeDTO is the signed-in user as the client sees it. It never carries the
// password hash.
type MeDTO struct {
	ID           int64     `json:"id" example:"1"`
	FullName     string    `json:"full_name" example:"Иванов Иван Иванович"`
	Email        string    `json:"email" example:"ivanov@company.ru"`
	Role         string    `json:"role" example:"employee" enums:"admin,manager,employee"`
	DepartmentID *int64    `json:"department_id" example:"3"`
	CreatedAt    time.Time `json:"created_at" example:"2026-10-21T10:00:00Z"`
}

// ChangePasswordRequest is the body of POST /auth/password.
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" example:"change-me-now"`
	NewPassword     string `json:"new_password" example:"a-better-password"`
}

func toMeDTO(u domain.User) MeDTO {
	return MeDTO{
		ID:           u.ID,
		FullName:     u.FullName,
		Email:        u.Email,
		Role:         string(u.Role),
		DepartmentID: u.DepartmentID,
		CreatedAt:    u.CreatedAt,
	}
}

// login godoc
//
//	@Summary		Sign in
//	@Description	Checks email and password and returns a JWT. An unknown email and a wrong password both answer 401 with the same message; a blocked account answers 403.
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			body	body		LoginRequest	true	"Credentials"
//	@Success		200		{object}	LoginResponse
//	@Failure		400		{object}	httpx.ErrorBody
//	@Failure		401		{object}	httpx.ErrorBody
//	@Failure		403		{object}	httpx.ErrorBody
//	@Failure		500		{object}	httpx.ErrorBody
//	@Router			/auth/login [post]
func (h *handler) login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	res, err := h.svc.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	httpx.WriteOK(r.Context(), w, LoginResponse{
		Token:     res.Token,
		TokenType: "Bearer",
		ExpiresAt: res.ExpiresAt,
		User:      toMeDTO(res.User),
	})
}

// me godoc
//
//	@Summary		Current user
//	@Description	Returns the user the bearer token belongs to.
//	@Tags			auth
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	MeDTO
//	@Failure		401	{object}	httpx.ErrorBody
//	@Router			/auth/me [get]
func (h *handler) me(w http.ResponseWriter, r *http.Request) {
	u, _ := UserFromContext(r.Context())
	httpx.WriteOK(r.Context(), w, toMeDTO(u))
}

// changePassword godoc
//
//	@Summary		Change own password
//	@Description	Replaces the caller's password after checking the current one. Issued tokens stay valid.
//	@Tags			auth
//	@Accept			json
//	@Security		BearerAuth
//	@Param			body	body	ChangePasswordRequest	true	"Current and new password"
//	@Success		204
//	@Failure		400	{object}	httpx.ErrorBody
//	@Failure		401	{object}	httpx.ErrorBody
//	@Failure		500	{object}	httpx.ErrorBody
//	@Router			/auth/password [post]
func (h *handler) changePassword(w http.ResponseWriter, r *http.Request) {
	var req ChangePasswordRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	u, _ := UserFromContext(r.Context())
	logger.Op(r.Context(), "auth.handler.changePassword").Debug("request", "user_id", u.ID)
	if err := h.svc.ChangePassword(r.Context(), u.ID, req.CurrentPassword, req.NewPassword); err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	httpx.WriteNoContent(w)
}
