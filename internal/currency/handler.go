package currency

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"

	"subtrack/internal/auth"
	"subtrack/internal/domain"
	"subtrack/internal/httpx"
)

// handler serves /api/v1/currencies.
type handler struct {
	svc *Service
}

// CurrencyDTO is a currency as the client sees it.
type CurrencyDTO struct {
	Code      string    `json:"code" example:"USD"`
	Name      string    `json:"name" example:"Доллар США"`
	RateToRUB string    `json:"rate_to_rub" example:"92.5"`
	UpdatedAt time.Time `json:"updated_at" example:"2026-10-27T10:00:00Z"`
	UpdatedBy *int64    `json:"updated_by" example:"1"`
}

func toDTO(c domain.Currency) CurrencyDTO {
	return CurrencyDTO{Code: c.Code, Name: c.Name, RateToRUB: c.RateToRUB, UpdatedAt: c.UpdatedAt, UpdatedBy: c.UpdatedBy}
}

// CreateCurrencyRequest is the body of POST /currencies. The rate may be
// sent as a string or a number.
type CreateCurrencyRequest struct {
	Code      string      `json:"code" example:"USD"`
	Name      string      `json:"name" example:"Доллар США"`
	RateToRUB json.Number `json:"rate_to_rub" swaggertype:"string" example:"92.5"`
}

// UpdateCurrencyRequest is the body of PATCH /currencies/{code}. Omit a
// field to leave it unchanged.
type UpdateCurrencyRequest struct {
	Name      *string      `json:"name,omitempty" example:"Доллар США"`
	RateToRUB *json.Number `json:"rate_to_rub,omitempty" swaggertype:"string" example:"93.1"`
}

// ListCurrenciesResponse is the body of GET /currencies.
type ListCurrenciesResponse struct {
	Items []CurrencyDTO `json:"items"`
}

// list godoc
//
//	@Summary		List currencies
//	@Description	Every currency with its current rate to roubles, RUB first. Open to everyone signed in: forms need the list.
//	@Tags			currencies
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	ListCurrenciesResponse
//	@Failure		401	{object}	httpx.ErrorBody
//	@Failure		500	{object}	httpx.ErrorBody
//	@Router			/currencies [get]
func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List(r.Context())
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	out := ListCurrenciesResponse{Items: make([]CurrencyDTO, 0, len(items))}
	for _, c := range items {
		out.Items = append(out.Items, toDTO(c))
	}
	httpx.WriteOK(r.Context(), w, out)
}

// get godoc
//
//	@Summary	Get a currency
//	@Tags		currencies
//	@Produce	json
//	@Security	BearerAuth
//	@Param		code	path		string	true	"Currency code, any case"	example(USD)
//	@Success	200		{object}	CurrencyDTO
//	@Failure	401		{object}	httpx.ErrorBody
//	@Failure	404		{object}	httpx.ErrorBody
//	@Failure	500		{object}	httpx.ErrorBody
//	@Router		/currencies/{code} [get]
func (h *handler) get(w http.ResponseWriter, r *http.Request) {
	c, err := h.svc.Get(r.Context(), mux.Vars(r)["code"])
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	httpx.WriteOK(r.Context(), w, toDTO(c))
}

// create godoc
//
//	@Summary		Add a currency (admin)
//	@Description	The code is three Latin letters (stored upper-case), the rate is how many roubles one unit costs: above 0, at most 6 decimals.
//	@Tags			currencies
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			body	body		CreateCurrencyRequest	true	"Currency fields"
//	@Success		201		{object}	CurrencyDTO
//	@Failure		400		{object}	httpx.ErrorBody
//	@Failure		401		{object}	httpx.ErrorBody
//	@Failure		403		{object}	httpx.ErrorBody
//	@Failure		409		{object}	httpx.ErrorBody
//	@Failure		500		{object}	httpx.ErrorBody
//	@Router			/currencies [post]
func (h *handler) create(w http.ResponseWriter, r *http.Request) {
	var req CreateCurrencyRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	actor, _ := auth.UserFromContext(r.Context())
	c, err := h.svc.Create(r.Context(), actor, domain.Currency{Code: req.Code, Name: req.Name, RateToRUB: req.RateToRUB.String()})
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	httpx.WriteCreated(r.Context(), w, toDTO(c))
}

// update godoc
//
//	@Summary		Update a currency (admin)
//	@Description	Rename and/or set a new rate; at least one field. The new rate applies at once to every report. The rate of RUB is fixed at 1 (409).
//	@Tags			currencies
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			code	path		string					true	"Currency code, any case"	example(USD)
//	@Param			body	body		UpdateCurrencyRequest	true	"Fields to change"
//	@Success		200		{object}	CurrencyDTO
//	@Failure		400		{object}	httpx.ErrorBody
//	@Failure		401		{object}	httpx.ErrorBody
//	@Failure		403		{object}	httpx.ErrorBody
//	@Failure		404		{object}	httpx.ErrorBody
//	@Failure		409		{object}	httpx.ErrorBody
//	@Failure		500		{object}	httpx.ErrorBody
//	@Router			/currencies/{code} [patch]
func (h *handler) update(w http.ResponseWriter, r *http.Request) {
	var req UpdateCurrencyRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	p := domain.CurrencyPatch{Name: req.Name}
	if req.RateToRUB != nil {
		rate := req.RateToRUB.String()
		p.RateToRUB = &rate
	}
	actor, _ := auth.UserFromContext(r.Context())
	c, err := h.svc.Update(r.Context(), actor, mux.Vars(r)["code"], p)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	httpx.WriteOK(r.Context(), w, toDTO(c))
}

// delete godoc
//
//	@Summary		Delete a currency (admin)
//	@Description	Only a currency no subscription, request or payment uses (409 otherwise). RUB cannot be deleted (409).
//	@Tags			currencies
//	@Security		BearerAuth
//	@Param			code	path	string	true	"Currency code, any case"	example(USD)
//	@Success		204
//	@Failure		401	{object}	httpx.ErrorBody
//	@Failure		403	{object}	httpx.ErrorBody
//	@Failure		404	{object}	httpx.ErrorBody
//	@Failure		409	{object}	httpx.ErrorBody
//	@Failure		500	{object}	httpx.ErrorBody
//	@Router			/currencies/{code} [delete]
func (h *handler) delete(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth.UserFromContext(r.Context())
	if err := h.svc.Delete(r.Context(), actor, mux.Vars(r)["code"]); err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	httpx.WriteNoContent(w)
}
