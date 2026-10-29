package transaction

import (
	"net/http"
	"time"

	"subtrack/internal/domain"
	"subtrack/internal/httpx"
	"subtrack/internal/xlsx"
)

var statusLabel = map[domain.TransactionStatus]string{domain.TxPending: "ожидает оплаты", domain.TxPaid: "оплачен", domain.TxFailed: "не прошёл"}

// export godoc
//
//	@Summary		Export payments to Excel (admin)
//	@Description	Every payment matching the same filters as the list, as an .xlsx sheet, by payment date (newest first; `order=asc`: oldest first).
//	@Tags			transactions
//	@Produce		application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
//	@Security		BearerAuth
//	@Param			subscription_id	query		int		false	"Only this subscription"
//	@Param			status			query		string	false	"pending | paid | failed"
//	@Param			from			query		string	false	"payment_date from, inclusive (YYYY-MM-DD)"
//	@Param			to				query		string	false	"payment_date to, inclusive (YYYY-MM-DD)"
//	@Param			order			query		string	false	"desc (default) | asc"
//	@Success		200				{file}		file
//	@Failure		400				{object}	httpx.ErrorBody
//	@Failure		401				{object}	httpx.ErrorBody
//	@Failure		403				{object}	httpx.ErrorBody
//	@Failure		500				{object}	httpx.ErrorBody
//	@Router			/transactions/export [get]
func (h *handler) export(w http.ResponseWriter, r *http.Request) {
	f, err := parseFilter(r)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	params := httpx.ParseListParams(r)
	params.Normalize()
	txs, err := h.svc.All(r.Context(), f, params.Order)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	sheet := xlsx.Sheet{
		Name: "Платежи",
		Header: []string{"ID", "Подписка", "ID подписки", "Дата платежа", "Сумма", "Валюта", "Статус",
			"Способ оплаты", "Номер счёта", "Оплачен", "Создан"},
		Rows: make([][]any, 0, len(txs)),
	}
	for _, t := range txs {
		sheet.Rows = append(sheet.Rows, []any{t.ID, t.SubscriptionName, t.SubscriptionID, xlsx.Day(t.PaymentDate),
			xlsx.Money(t.Amount), t.Currency, statusLabel[t.Status], t.PaymentMethod, t.InvoiceNumber, t.PaidAt, t.CreatedAt})
	}
	xlsx.Respond(r.Context(), w, xlsx.Filename("payments", time.Now(), h.svc.loc), h.svc.loc, sheet)
}
