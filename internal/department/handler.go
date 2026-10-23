package department

import (
	"net/http"
	"time"

	"subtrack/internal/auth"
	"subtrack/internal/domain"
	"subtrack/internal/httpx"
)

// handler serves /api/v1/departments.
type handler struct {
	svc *Service
}

// DepartmentDTO is a department as the client sees it.
type DepartmentDTO struct {
	ID            int64     `json:"id" example:"3"`
	Name          string    `json:"name" example:"Разработка"`
	HeadID        *int64    `json:"head_id" example:"5"`
	HeadName      *string   `json:"head_name" example:"Сидоров Сидор Сидорович"`
	EmployeeCount int64     `json:"employee_count" example:"12"`
	CreatedAt     time.Time `json:"created_at" example:"2026-10-21T10:00:00Z"`
	UpdatedAt     time.Time `json:"updated_at" example:"2026-10-21T10:00:00Z"`
}

func toDTO(d domain.Department) DepartmentDTO {
	return DepartmentDTO{
		ID:            d.ID,
		Name:          d.Name,
		HeadID:        d.HeadID,
		HeadName:      d.HeadName,
		EmployeeCount: d.EmployeeCount,
		CreatedAt:     d.CreatedAt,
		UpdatedAt:     d.UpdatedAt,
	}
}

// CreateDepartmentRequest is the body of POST /departments.
type CreateDepartmentRequest struct {
	Name   string `json:"name" example:"Разработка"`
	HeadID *int64 `json:"head_id,omitempty" example:"5"`
}

// UpdateDepartmentRequest is the body of PATCH /departments/{id}. Omit a field
// to leave it unchanged; "head_id": null leaves the department without a head.
type UpdateDepartmentRequest struct {
	Name   *string          `json:"name,omitempty" example:"Разработка"`
	HeadID httpx.OptionalID `json:"head_id" swaggertype:"integer" extensions:"x-nullable" example:"5"`
}

// ListDepartmentsResponse is the body of GET /departments.
type ListDepartmentsResponse struct {
	Items []DepartmentDTO `json:"items"`
	httpx.PageMeta
}

// list godoc
//
//	@Summary		List departments
//	@Description	Admin: all departments, sorted by name. Anyone else: only their own department (zero or one item).
//	@Tags			departments
//	@Produce		json
//	@Security		BearerAuth
//	@Param			q		query		string	false	"Substring of the name (admin only)"
//	@Param			limit	query		int		false	"Page size (default 50, max 200)"
//	@Param			offset	query		int		false	"Page offset"
//	@Success		200		{object}	ListDepartmentsResponse
//	@Failure		401		{object}	httpx.ErrorBody
//	@Failure		500		{object}	httpx.ErrorBody
//	@Router			/departments [get]
func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	params := httpx.ParseListParams(r)
	actor, _ := auth.UserFromContext(r.Context())
	items, total, err := h.svc.List(r.Context(), actor, r.URL.Query().Get("q"), params)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	params.Normalize()
	out := ListDepartmentsResponse{
		Items:    make([]DepartmentDTO, 0, len(items)),
		PageMeta: httpx.PageMeta{Total: total, Limit: params.Limit, Offset: params.Offset},
	}
	for _, d := range items {
		out.Items = append(out.Items, toDTO(d))
	}
	httpx.WriteOK(r.Context(), w, out)
}

// get godoc
//
//	@Summary		Get a department
//	@Description	Admin: any department. Anyone else: only their own; another department is a 404.
//	@Tags			departments
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Department id"
//	@Success		200	{object}	DepartmentDTO
//	@Failure		400	{object}	httpx.ErrorBody
//	@Failure		401	{object}	httpx.ErrorBody
//	@Failure		404	{object}	httpx.ErrorBody
//	@Failure		500	{object}	httpx.ErrorBody
//	@Router			/departments/{id} [get]
func (h *handler) get(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	actor, _ := auth.UserFromContext(r.Context())
	d, err := h.svc.Get(r.Context(), actor, id)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	httpx.WriteOK(r.Context(), w, toDTO(d))
}

// create godoc
//
//	@Summary		Create a department (admin)
//	@Description	The name is unique case-insensitively. The head, if given, must be an active manager who heads no other department; they are moved into the new department.
//	@Tags			departments
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			body	body		CreateDepartmentRequest	true	"Department fields"
//	@Success		201		{object}	DepartmentDTO
//	@Failure		400		{object}	httpx.ErrorBody
//	@Failure		401		{object}	httpx.ErrorBody
//	@Failure		403		{object}	httpx.ErrorBody
//	@Failure		409		{object}	httpx.ErrorBody
//	@Failure		500		{object}	httpx.ErrorBody
//	@Router			/departments [post]
func (h *handler) create(w http.ResponseWriter, r *http.Request) {
	var req CreateDepartmentRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	actor, _ := auth.UserFromContext(r.Context())
	d, err := h.svc.Create(r.Context(), actor, CreateDepartmentInput{Name: req.Name, HeadID: req.HeadID})
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	httpx.WriteCreated(r.Context(), w, toDTO(d))
}

// update godoc
//
//	@Summary		Update a department (admin)
//	@Description	Rename and/or change the head; at least one field. A new head must be an active manager who heads no other department and is moved into this one; the previous head stays in the department. `"head_id": null` leaves it without a head.
//	@Tags			departments
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int						true	"Department id"
//	@Param			body	body		UpdateDepartmentRequest	true	"Fields to change"
//	@Success		200		{object}	DepartmentDTO
//	@Failure		400		{object}	httpx.ErrorBody
//	@Failure		401		{object}	httpx.ErrorBody
//	@Failure		403		{object}	httpx.ErrorBody
//	@Failure		404		{object}	httpx.ErrorBody
//	@Failure		409		{object}	httpx.ErrorBody
//	@Failure		500		{object}	httpx.ErrorBody
//	@Router			/departments/{id} [patch]
func (h *handler) update(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	var req UpdateDepartmentRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	actor, _ := auth.UserFromContext(r.Context())
	d, err := h.svc.Update(r.Context(), actor, id, domain.DepartmentPatch{
		Name:   req.Name,
		HeadID: domain.IDPatch(req.HeadID),
	})
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	httpx.WriteOK(r.Context(), w, toDTO(d))
}

// delete godoc
//
//	@Summary		Delete a department (admin)
//	@Description	Only an empty department can be deleted: no active users and no subscriptions (409 otherwise, with the counts). Blocked users and past requests that point at it are detached.
//	@Tags			departments
//	@Security		BearerAuth
//	@Param			id	path	int	true	"Department id"
//	@Success		204
//	@Failure		400	{object}	httpx.ErrorBody
//	@Failure		401	{object}	httpx.ErrorBody
//	@Failure		403	{object}	httpx.ErrorBody
//	@Failure		404	{object}	httpx.ErrorBody
//	@Failure		409	{object}	httpx.ErrorBody
//	@Failure		500	{object}	httpx.ErrorBody
//	@Router			/departments/{id} [delete]
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
