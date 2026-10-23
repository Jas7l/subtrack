package subscription

import (
	"net/http"
	"time"

	"subtrack/internal/auth"
	"subtrack/internal/domain"
	"subtrack/internal/xlsx"
)

// Russian labels of the enum values, for reports.
var (
	typeLabel   = map[domain.SubscriptionType]string{domain.SubIndividual: "индивидуальная", domain.SubGroup: "групповая", domain.SubEnterprise: "корпоративная"}
	periodLabel = map[domain.BillingPeriod]string{domain.BillingMonth: "месяц", domain.BillingQuarter: "квартал", domain.BillingYear: "год"}
	statusLabel = map[domain.SubscriptionStatus]string{domain.SubActive: "активна", domain.SubExpiringSoon: "скоро закончится",
		domain.SubExpired: "истекла", domain.SubCancelled: "отменена"}
)

// export godoc
//
//	@Summary		Export subscriptions to Excel (admin)
//	@Description	Every subscription matching the same filters as the list, as an .xlsx sheet.
//	@Tags			subscriptions
//	@Produce		application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
//	@Security		BearerAuth
//	@Param			department_id	query		int		false	"Only this department"
//	@Param			category		query		string	false	"Category, case-insensitive"
//	@Param			status			query		string	false	"active | expiring_soon | expired | cancelled"
//	@Param			type			query		string	false	"individual | group | enterprise"
//	@Param			end_from		query		string	false	"end_date from, inclusive (YYYY-MM-DD)"
//	@Param			end_to			query		string	false	"end_date to, inclusive (YYYY-MM-DD)"
//	@Param			q				query		string	false	"Substring of name or vendor"
//	@Success		200				{file}		file
//	@Failure		400				{object}	httpx.ErrorBody
//	@Failure		401				{object}	httpx.ErrorBody
//	@Failure		403				{object}	httpx.ErrorBody
//	@Failure		500				{object}	httpx.ErrorBody
//	@Router			/subscriptions/export [get]
func (h *handler) export(w http.ResponseWriter, r *http.Request) {
	f, err := parseFilter(r)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	actor, _ := auth.UserFromContext(r.Context())
	subs, err := h.svc.All(r.Context(), actor, f)
	if err != nil {
		writeErr(r.Context(), w, err)
		return
	}
	sheet := xlsx.Sheet{
		Name: "Подписки",
		Header: []string{"ID", "Название", "Вендор", "Категория", "Тип", "Стоимость", "Валюта", "Период оплаты",
			"Начало", "Окончание", "Следующий платёж", "Статус", "Отдел", "Мест всего", "Занято", "Создана"},
		Rows: make([][]any, 0, len(subs)),
	}
	for _, s := range subs {
		dept := "Общая"
		if s.DepartmentName != nil {
			dept = *s.DepartmentName
		}
		sheet.Rows = append(sheet.Rows, []any{s.ID, s.Name, s.Vendor, s.Category, typeLabel[s.Type],
			xlsx.Money(s.Cost), s.Currency, periodLabel[s.BillingPeriod], xlsx.Day(s.StartDate),
			xlsx.DayPtr(s.EndDate), xlsx.DayPtr(s.NextPaymentDate), statusLabel[s.Status], dept,
			s.SeatsTotal, s.SeatsUsed, s.CreatedAt})
	}
	xlsx.Respond(r.Context(), w, xlsx.Filename("subscriptions", time.Now(), time.Local), sheet)
}
