package user

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"subtrack/internal/apperr"
	"subtrack/internal/auth"
	"subtrack/internal/domain"
	"subtrack/internal/httpx"
	"subtrack/internal/logger"
)

// handler serves /api/v1/users.
type handler struct {
	svc *Service
}

// UserDTO is a user as the client sees it. It never carries the password
// hash.
type UserDTO struct {
	ID           int64     `json:"id" example:"7"`
	FullName     string    `json:"full_name" example:"Петров Пётр Петрович"`
	Email        string    `json:"email" example:"petrov@company.ru"`
	Role         string    `json:"role" example:"employee" enums:"admin,manager,employee"`
	DepartmentID *int64    `json:"department_id" example:"3"`
	IsActive     bool      `json:"is_active" example:"true"`
	CreatedAt    time.Time `json:"created_at" example:"2026-10-21T10:00:00Z"`
	UpdatedAt    time.Time `json:"updated_at" example:"2026-10-21T10:00:00Z"`
}

func toDTO(u domain.User) UserDTO {
	return UserDTO{
		ID:           u.ID,
		FullName:     u.FullName,
		Email:        u.Email,
		Role:         string(u.Role),
		DepartmentID: u.DepartmentID,
		IsActive:     u.IsActive,
		CreatedAt:    u.CreatedAt,
		UpdatedAt:    u.UpdatedAt,
	}
}

// CreateUserRequest is the body of POST /users.
type CreateUserRequest struct {
	FullName     string `json:"full_name" example:"Петров Пётр Петрович"`
	Email        string `json:"email" example:"petrov@company.ru"`
	Password     string `json:"password" example:"initial-password"`
	Role         string `json:"role" example:"employee" enums:"admin,manager,employee"`
	DepartmentID *int64 `json:"department_id,omitempty" example:"3"`
}

// UpdateUserRequest is the body of PATCH /users/{id}. Omit a field to leave
// it unchanged. "department_id": null removes the user from their department.
type UpdateUserRequest struct {
	FullName     *string          `json:"full_name,omitempty" example:"Петров Пётр Петрович"`
	Email        *string          `json:"email,omitempty" example:"petrov@company.ru"`
	Role         *string          `json:"role,omitempty" example:"manager" enums:"admin,manager,employee"`
	DepartmentID httpx.OptionalID `json:"department_id" swaggertype:"integer" extensions:"x-nullable" example:"3"`
	IsActive     *bool            `json:"is_active,omitempty" example:"false"`
}

// SetPasswordRequest is the body of PUT /users/{id}/password.
type SetPasswordRequest struct {
	Password string `json:"password" example:"temporary-password"`
}

// ListUsersResponse is the body of GET /users.
type ListUsersResponse struct {
	Items []UserDTO `json:"items"`
	httpx.PageMeta
}

// list godoc
//
//	@Summary		List users
//	@Description	Admin: all users. Manager: only their department (the department_id filter is ignored). Employee: 403. Sorted by full name.
//	@Tags			users
//	@Produce		json
//	@Security		BearerAuth
//	@Param			department_id	query		int		false	"Only users of this department"
//	@Param			role			query		string	false	"Only this role"	Enums(admin, manager, employee)
//	@Param			active			query		bool	false	"true: only active, false: only blocked"
//	@Param			q				query		string	false	"Substring of full name or email"
//	@Param			limit			query		int		false	"Page size (default 50, max 200)"
//	@Param			offset			query		int		false	"Page offset"
//	@Success		200				{object}	ListUsersResponse
//	@Failure		400				{object}	httpx.ErrorBody
//	@Failure		401				{object}	httpx.ErrorBody
//	@Failure		403				{object}	httpx.ErrorBody
//	@Failure		500				{object}	httpx.ErrorBody
//	@Router			/users [get]
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
	out := ListUsersResponse{
		Items:    make([]UserDTO, 0, len(items)),
		PageMeta: httpx.PageMeta{Total: total, Limit: params.Limit, Offset: params.Offset},
	}
	for _, u := range items {
		out.Items = append(out.Items, toDTO(u))
	}
	httpx.WriteOK(r.Context(), w, out)
}

// get godoc
//
//	@Summary		Get a user
//	@Description	Admin: any user. Manager: users of their department. Employee: only themselves. A user outside the caller's view is a 404.
//	@Tags			users
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"User id"
//	@Success		200	{object}	UserDTO
//	@Failure		400	{object}	httpx.ErrorBody
//	@Failure		401	{object}	httpx.ErrorBody
//	@Failure		404	{object}	httpx.ErrorBody
//	@Failure		500	{object}	httpx.ErrorBody
//	@Router			/users/{id} [get]
func (h *handler) get(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	actor, _ := auth.UserFromContext(r.Context())
	u, err := h.svc.Get(r.Context(), actor, id)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	httpx.WriteOK(r.Context(), w, toDTO(u))
}

// create godoc
//
//	@Summary		Create a user (admin)
//	@Description	The admin sets the initial password and hands it to the person. The email is unique case-insensitively (409 on a clash).
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			body	body		CreateUserRequest	true	"User fields"
//	@Success		201		{object}	UserDTO
//	@Failure		400		{object}	httpx.ErrorBody
//	@Failure		401		{object}	httpx.ErrorBody
//	@Failure		403		{object}	httpx.ErrorBody
//	@Failure		409		{object}	httpx.ErrorBody
//	@Failure		500		{object}	httpx.ErrorBody
//	@Router			/users [post]
func (h *handler) create(w http.ResponseWriter, r *http.Request) {
	var req CreateUserRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	actor, _ := auth.UserFromContext(r.Context())
	u, err := h.svc.Create(r.Context(), actor, CreateUserInput{
		FullName:     req.FullName,
		Email:        req.Email,
		Password:     req.Password,
		Role:         domain.Role(req.Role),
		DepartmentID: req.DepartmentID,
	})
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	httpx.WriteCreated(r.Context(), w, toDTO(u))
}

// update godoc
//
//	@Summary		Update a user (admin)
//	@Description	Partial update; at least one field. `"is_active": false` blocks the user (their tokens stop working at once), `true` unblocks. `"department_id": null` removes them from their department. If the user headed a department and is blocked, demoted or moved out of it, that department is left without a head. Blocking also revokes all their licences. An admin cannot block themselves or drop their own admin role (409).
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int					true	"User id"
//	@Param			body	body		UpdateUserRequest	true	"Fields to change"
//	@Success		200		{object}	UserDTO
//	@Failure		400		{object}	httpx.ErrorBody
//	@Failure		401		{object}	httpx.ErrorBody
//	@Failure		403		{object}	httpx.ErrorBody
//	@Failure		404		{object}	httpx.ErrorBody
//	@Failure		409		{object}	httpx.ErrorBody
//	@Failure		500		{object}	httpx.ErrorBody
//	@Router			/users/{id} [patch]
func (h *handler) update(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	var req UpdateUserRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	p := domain.UserPatch{
		FullName:     req.FullName,
		Email:        req.Email,
		DepartmentID: domain.IDPatch(req.DepartmentID),
		IsActive:     req.IsActive,
	}
	if req.Role != nil {
		role := domain.Role(*req.Role)
		p.Role = &role
	}
	actor, _ := auth.UserFromContext(r.Context())
	u, err := h.svc.Update(r.Context(), actor, id, p)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	httpx.WriteOK(r.Context(), w, toDTO(u))
}

// setPassword godoc
//
//	@Summary		Reset a user's password (admin)
//	@Description	Sets a new password, e.g. when the person forgot theirs. Issued tokens stay valid.
//	@Tags			users
//	@Accept			json
//	@Security		BearerAuth
//	@Param			id		path	int					true	"User id"
//	@Param			body	body	SetPasswordRequest	true	"New password"
//	@Success		204
//	@Failure		400	{object}	httpx.ErrorBody
//	@Failure		401	{object}	httpx.ErrorBody
//	@Failure		403	{object}	httpx.ErrorBody
//	@Failure		404	{object}	httpx.ErrorBody
//	@Failure		500	{object}	httpx.ErrorBody
//	@Router			/users/{id}/password [put]
func (h *handler) setPassword(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	var req SetPasswordRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	logger.Op(r.Context(), "user.handler.setPassword").Debug("request", "id", id)
	actor, _ := auth.UserFromContext(r.Context())
	if err := h.svc.SetPassword(r.Context(), actor, id, req.Password); err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	httpx.WriteNoContent(w)
}

// parseFilter reads the list filters. Unlike paging, a malformed filter is a
// 400: silently dropping it would return a wider list than the client asked
// for.
func parseFilter(r *http.Request) (domain.UserFilter, error) {
	q := r.URL.Query()
	f := domain.UserFilter{Query: q.Get("q")}
	if v := q.Get("department_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			return f, fmt.Errorf("%w: department_id must be a positive integer", apperr.ErrInvalidInput)
		}
		f.DepartmentID = &id
	}
	if v := q.Get("role"); v != "" {
		role := domain.Role(v)
		if !role.Valid() {
			return f, fmt.Errorf("%w: role must be one of admin, manager, employee", apperr.ErrInvalidInput)
		}
		f.Role = &role
	}
	if v := q.Get("active"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return f, fmt.Errorf("%w: active must be true or false", apperr.ErrInvalidInput)
		}
		f.Active = &b
	}
	return f, nil
}
