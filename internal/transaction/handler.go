package transaction

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

// handler serves /api/v1/transactions.
type handler struct {
	svc *Service
}

// TransactionDTO is a payment as the client sees it.
type TransactionDTO struct {
	ID               int64      `json:"id" example:"40"`
	SubscriptionID   int64      `json:"subscription_id" example:"12"`
	SubscriptionName string     `json:"subscription_name" example:"Jira"`
	Amount           string     `json:"amount" example:"15000.00"`
	Currency         string     `json:"currency" example:"RUB"`
	PaymentDate      httpx.Date `json:"payment_date" swaggertype:"string" format:"date" example:"2026-11-01"`
	Status           string     `json:"status" example:"pending" enums:"pending,paid,failed"`
	PaymentMethod    string     `json:"payment_method" example:"Корпоративная карта"`
	InvoiceNumber    string     `json:"invoice_number" example:"INV-2026-114"`
	PaidAt           *time.Time `json:"paid_at" example:"2026-11-01T09:30:00Z"`
	CreatedAt        time.Time  `json:"created_at" example:"2026-11-01T00:05:00Z"`
	UpdatedAt        time.Time  `json:"updated_at" example:"2026-11-01T09:30:00Z"`
}

func toDTO(t domain.Transaction) TransactionDTO {
	return TransactionDTO{
		ID:               t.ID,
		SubscriptionID:   t.SubscriptionID,
		SubscriptionName: t.SubscriptionName,
		Amount:           t.Amount,
		Currency:         t.Currency,
		PaymentDate:      httpx.Date(t.PaymentDate),
		Status:           string(t.Status),
		PaymentMethod:    t.PaymentMethod,
		InvoiceNumber:    t.InvoiceNumber,
		PaidAt:           t.PaidAt,
		CreatedAt:        t.CreatedAt,
		UpdatedAt:        t.UpdatedAt,
	}
}

// ListTransactionsResponse is the body of GET /transactions.
type ListTransactionsResponse struct {
	Items []TransactionDTO `json:"items"`
	httpx.PageMeta
}

// CreateTransactionRequest is the body of POST /transactions. amount may be
// a JSON string or number; currency defaults to the subscription's.
type CreateTransactionRequest struct {
	SubscriptionID int64       `json:"subscription_id" example:"12"`
	Amount         json.Number `json:"amount" swaggertype:"string" example:"15000.00"`
	Currency       string      `json:"currency,omitempty" example:"RUB"`
	PaymentDate    *httpx.Date `json:"payment_date" swaggertype:"string" format:"date" example:"2026-09-01"`
	PaymentMethod  string      `json:"payment_method,omitempty" example:"Счёт"`
	InvoiceNumber  string      `json:"invoice_number,omitempty" example:"INV-2026-090"`
}

// PayTransactionRequest is the optional body of POST /transactions/{id}/pay.
type PayTransactionRequest struct {
	PaymentMethod string `json:"payment_method,omitempty" example:"Корпоративная карта"`
	InvoiceNumber string `json:"invoice_number,omitempty" example:"INV-2026-114"`
}

// list godoc
//
//	@Summary		List payments (admin)
//	@Description	By payment date, newest first (`order=asc`: oldest first). Expected payments: `status=pending&order=asc`.
//	@Tags			transactions
//	@Produce		json
//	@Security		BearerAuth
//	@Param			subscription_id	query		int		false	"Only this subscription"
//	@Param			status			query		string	false	"pending | paid | failed"
//	@Param			from			query		string	false	"payment_date from, inclusive (YYYY-MM-DD)"
//	@Param			to				query		string	false	"payment_date to, inclusive (YYYY-MM-DD)"
//	@Param			order			query		string	false	"desc (default) | asc"
//	@Param			limit			query		int		false	"Page size (default 50, max 200)"
//	@Param			offset			query		int		false	"Page offset"
//	@Success		200				{object}	ListTransactionsResponse
//	@Failure		400				{object}	httpx.ErrorBody
//	@Failure		401				{object}	httpx.ErrorBody
//	@Failure		403				{object}	httpx.ErrorBody
//	@Failure		500				{object}	httpx.ErrorBody
//	@Router			/transactions [get]
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
	out := ListTransactionsResponse{
		Items:    make([]TransactionDTO, 0, len(items)),
		PageMeta: httpx.PageMeta{Total: total, Limit: params.Limit, Offset: params.Offset},
	}
	for _, t := range items {
		out.Items = append(out.Items, toDTO(t))
	}
	httpx.WriteOK(r.Context(), w, out)
}

// get godoc
//
//	@Summary		Get a payment (admin)
//	@Tags			transactions
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Transaction id"
//	@Success		200	{object}	TransactionDTO
//	@Failure		400	{object}	httpx.ErrorBody
//	@Failure		401	{object}	httpx.ErrorBody
//	@Failure		403	{object}	httpx.ErrorBody
//	@Failure		404	{object}	httpx.ErrorBody
//	@Failure		500	{object}	httpx.ErrorBody
//	@Router			/transactions/{id} [get]
func (h *handler) get(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	t, err := h.svc.Get(r.Context(), id)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	httpx.WriteOK(r.Context(), w, toDTO(t))
}

// create godoc
//
//	@Summary		Enter a payment by hand (admin)
//	@Description	A one-off payment or a past one; it is stored as paid. Cancelled and expired subscriptions are accepted too. 409 when the subscription already has a payment on that date.
//	@Tags			transactions
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			body	body		CreateTransactionRequest	true	"The payment"
//	@Success		201		{object}	TransactionDTO
//	@Failure		400		{object}	httpx.ErrorBody
//	@Failure		401		{object}	httpx.ErrorBody
//	@Failure		403		{object}	httpx.ErrorBody
//	@Failure		409		{object}	httpx.ErrorBody
//	@Failure		500		{object}	httpx.ErrorBody
//	@Router			/transactions [post]
func (h *handler) create(w http.ResponseWriter, r *http.Request) {
	var req CreateTransactionRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	in := CreateInput{
		SubscriptionID: req.SubscriptionID,
		Amount:         req.Amount.String(),
		Currency:       req.Currency,
		PaymentMethod:  req.PaymentMethod,
		InvoiceNumber:  req.InvoiceNumber,
	}
	if req.PaymentDate != nil {
		d := time.Time(*req.PaymentDate)
		in.PaymentDate = &d
	}
	actor, _ := auth.UserFromContext(r.Context())
	t, err := h.svc.Create(r.Context(), actor, in)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	httpx.WriteCreated(r.Context(), w, toDTO(t))
}

// pay godoc
//
//	@Summary		Mark a payment paid (admin)
//	@Description	From pending or failed. The body is optional; a given payment_method or invoice_number is stored.
//	@Tags			transactions
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int						true	"Transaction id"
//	@Param			body	body		PayTransactionRequest	false	"Payment details"
//	@Success		200		{object}	TransactionDTO
//	@Failure		400		{object}	httpx.ErrorBody
//	@Failure		401		{object}	httpx.ErrorBody
//	@Failure		403		{object}	httpx.ErrorBody
//	@Failure		404		{object}	httpx.ErrorBody
//	@Failure		409		{object}	httpx.ErrorBody
//	@Failure		500		{object}	httpx.ErrorBody
//	@Router			/transactions/{id}/pay [post]
func (h *handler) pay(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	var req PayTransactionRequest
	if r.ContentLength != 0 {
		if err := httpx.DecodeJSON(w, r, &req); err != nil {
			writeErr(r.Context(), w, err)
			return
		}
	}
	actor, _ := auth.UserFromContext(r.Context())
	t, err := h.svc.Pay(r.Context(), actor, id, req.PaymentMethod, req.InvoiceNumber)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	httpx.WriteOK(r.Context(), w, toDTO(t))
}

// fail godoc
//
//	@Summary		Mark a payment failed (admin)
//	@Description	Only from pending. A failed payment can still be paid later.
//	@Tags			transactions
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Transaction id"
//	@Success		200	{object}	TransactionDTO
//	@Failure		400	{object}	httpx.ErrorBody
//	@Failure		401	{object}	httpx.ErrorBody
//	@Failure		403	{object}	httpx.ErrorBody
//	@Failure		404	{object}	httpx.ErrorBody
//	@Failure		409	{object}	httpx.ErrorBody
//	@Failure		500	{object}	httpx.ErrorBody
//	@Router			/transactions/{id}/fail [post]
func (h *handler) fail(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	actor, _ := auth.UserFromContext(r.Context())
	t, err := h.svc.Fail(r.Context(), actor, id)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	httpx.WriteOK(r.Context(), w, toDTO(t))
}

func parseFilter(r *http.Request) (domain.TransactionFilter, error) {
	q := r.URL.Query()
	var f domain.TransactionFilter
	if v := q.Get("status"); v != "" {
		st := domain.TransactionStatus(v)
		if !st.Valid() {
			return f, fmt.Errorf("%w: unknown status %q", apperr.ErrInvalidInput, v)
		}
		f.Status = &st
	}
	if v := q.Get("subscription_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			return f, fmt.Errorf("%w: subscription_id must be a positive integer", apperr.ErrInvalidInput)
		}
		f.SubscriptionID = &id
	}
	var err error
	if f.From, err = httpx.QueryDate(q, "from"); err != nil {
		return f, err
	}
	if f.To, err = httpx.QueryDate(q, "to"); err != nil {
		return f, err
	}
	return f, nil
}
