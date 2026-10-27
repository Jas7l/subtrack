package analytics

import (
	"net/http"
	"strconv"
	"strings"

	"subtrack/internal/auth"
	"subtrack/internal/domain"
	"subtrack/internal/xlsx"
)

var recLabel = map[RecommendationType]string{
	RecUnusedSeats: "Лишние места", RecNoHolders: "Никто не пользуется", RecDuplicateVendor: "Один вендор",
	RecIndividualToGroup: "Индивидуальные → групповая", RecEnterpriseLowUsage: "Корпоративная почти не используется",
	RecAnnualBilling: "Годовая оплата", RecExpiredWithHolders: "Истекла, лицензии выданы",
}

// export godoc
//
//	@Summary		Export the analytics to Excel (admin)
//	@Description	One workbook: summary, cost by category, by vendor and (for the whole company) by department, the 20 most expensive subscriptions, paid by month for the last 12 months, a 12-month forecast, the recommendations and the currency rates every rouble amount was converted at.
//	@Tags			analytics
//	@Produce		application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
//	@Security		BearerAuth
//	@Param			department_id	query		int	false	"One department"
//	@Success		200				{file}		file
//	@Failure		400				{object}	httpx.ErrorBody
//	@Failure		401				{object}	httpx.ErrorBody
//	@Failure		403				{object}	httpx.ErrorBody
//	@Failure		500				{object}	httpx.ErrorBody
//	@Router			/analytics/export [get]
func (h *handler) export(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	dept, err := queryID(r.URL.Query(), "department_id")
	if err != nil {
		writeErr(ctx, w, err)
		return
	}
	actor, _ := auth.UserFromContext(ctx)
	sum, err := h.svc.Summary(ctx, actor, dept)
	if err != nil {
		writeErr(ctx, w, err)
		return
	}
	cats, err := h.svc.ByCategory(ctx, actor, dept)
	if err != nil {
		writeErr(ctx, w, err)
		return
	}
	vendors, err := h.svc.ByVendor(ctx, actor, dept)
	if err != nil {
		writeErr(ctx, w, err)
		return
	}
	tops, err := h.svc.Top(ctx, dept, 20)
	if err != nil {
		writeErr(ctx, w, err)
		return
	}
	spent, err := h.svc.Spending(ctx, dept, nil, nil)
	if err != nil {
		writeErr(ctx, w, err)
		return
	}
	fc, err := h.svc.Forecast(ctx, actor, dept, DefaultForecastMonths)
	if err != nil {
		writeErr(ctx, w, err)
		return
	}
	recs, savings, err := h.svc.Recommendations(ctx, dept)
	if err != nil {
		writeErr(ctx, w, err)
		return
	}
	rates, err := h.svc.Rates(ctx)
	if err != nil {
		writeErr(ctx, w, err)
		return
	}

	sheets := []xlsx.Sheet{summarySheet(sum), sliceSheet("Категории", "Категория", cats), sliceSheet("Вендоры", "Вендор", vendors)}
	if dept == nil {
		rows, err := h.svc.ByDepartment(ctx)
		if err != nil {
			writeErr(ctx, w, err)
			return
		}
		sh := xlsx.Sheet{Name: "Отделы", Header: []string{"Отдел", "Подписок", "В месяц, ₽", "Мест всего", "Занято",
			"Занятость, %", "Сотрудников", "На сотрудника в месяц, ₽"}}
		for _, d := range rows {
			var perUser any
			if d.CostPerUserRUB != nil {
				perUser = xlsx.Money(*d.CostPerUserRUB)
			}
			sh.Rows = append(sh.Rows, []any{d.Name, d.Subscriptions, xlsx.Money(d.MonthlyCostRUB), d.SeatsTotal, d.SeatsUsed,
				d.Utilization, d.ActiveUsers, perUser})
		}
		sheets = append(sheets, sh)
	}

	topSh := xlsx.Sheet{Name: "Самые дорогие", Header: []string{"ID", "Подписка", "Вендор", "Отдел", "В месяц, ₽", "В год, ₽",
		"Мест", "Занято", "Занятость, %", "Лицензия в месяц, ₽"}}
	for _, t := range tops {
		var perLic any
		if t.CostPerLicenceRUB != nil {
			perLic = xlsx.Money(*t.CostPerLicenceRUB)
		}
		topSh.Rows = append(topSh.Rows, []any{t.ID, t.Name, t.Vendor, t.DepartmentName, xlsx.Money(t.MonthlyCostRUB),
			xlsx.Money(t.AnnualCostRUB), t.SeatsTotal, t.SeatsUsed, t.Utilization, perLic})
	}
	spentSh := xlsx.Sheet{Name: "Оплачено по месяцам", Header: []string{"Месяц", "Оплачено, ₽", "Платежей"}}
	for _, m := range spent.Months {
		spentSh.Rows = append(spentSh.Rows, []any{m.Month.Format(monthLayout), xlsx.Money(m.AmountRUB), m.Payments})
	}
	spentSh.Rows = append(spentSh.Rows, []any{"Итого", xlsx.Money(spent.TotalRUB)})
	fcSh := xlsx.Sheet{Name: "Прогноз", Header: []string{"Месяц", "По графику, ₽", "Платежей", "Оценка без графика, ₽", "Всего, ₽"}}
	for _, m := range fc.Months {
		fcSh.Rows = append(fcSh.Rows, []any{m.Month.Format(monthLayout), xlsx.Money(m.ScheduledRUB), m.Payments,
			xlsx.Money(m.EstimatedRUB), xlsx.Money(m.TotalRUB)})
	}
	fcSh.Rows = append(fcSh.Rows, []any{"Итого", nil, nil, nil, xlsx.Money(fc.TotalRUB)},
		[]any{"Если всё продлевать", nil, nil, nil, xlsx.Money(fc.RunRateRUB)})
	recSh := xlsx.Sheet{Name: "Рекомендации", Header: []string{"Тип", "Рекомендация", "Подробно", "Экономия в год, ₽", "ID подписок"}}
	for _, x := range recs {
		var s any
		if x.SavingsRUB != nil {
			s = xlsx.Money(*x.SavingsRUB)
		}
		ids := make([]string, 0, len(x.SubscriptionIDs))
		for _, id := range x.SubscriptionIDs {
			ids = append(ids, strconv.FormatInt(id, 10))
		}
		recSh.Rows = append(recSh.Rows, []any{recLabel[x.Type], x.Title, x.Text, s, strings.Join(ids, ", ")})
	}
	recSh.Rows = append(recSh.Rows, []any{"Итого", nil, nil, xlsx.Money(savings)})

	sheets = append(sheets, topSh, spentSh, fcSh, recSh, ratesSheet(rates))
	xlsx.Respond(ctx, w, xlsx.Filename("analytics", h.svc.now(), h.svc.loc), sheets...)
}

func summarySheet(s Summary) xlsx.Sheet {
	sh := xlsx.Sheet{Name: "Сводка", Header: []string{"Показатель", "Значение"}}
	add := func(k string, v any) { sh.Rows = append(sh.Rows, []any{k, v}) }
	add("Активных подписок", s.Active)
	add("Из них скоро закончатся", s.ExpiringSoon)
	add("Истекших", s.Expired)
	add("Стоимость в месяц, ₽", xlsx.Money(s.MonthlyCostRUB))
	add("Стоимость в год, ₽", xlsx.Money(s.AnnualCostRUB))
	add("Мест всего", s.SeatsTotal)
	add("Мест занято", s.SeatsUsed)
	add("Мест свободно", s.SeatsFree)
	add("Занятость мест, %", s.Utilization)
	add("Корпоративных (безлимитных) подписок", s.Enterprise)
	add("Выдано лицензий", s.Licences)
	add("Активных сотрудников", s.ActiveUsers)
	if s.CostPerUserRUB != nil {
		add("На сотрудника в месяц, ₽", xlsx.Money(*s.CostPerUserRUB))
	}
	if p := s.Payments; p != nil {
		add("Оплачено в этом месяце, ₽", xlsx.Money(p.PaidThisMonthRUB))
		add("Оплачено в этом году, ₽", xlsx.Money(p.PaidThisYearRUB))
		add("Ожидают оплаты, шт.", p.PendingCount)
		add("Ожидают оплаты, ₽", xlsx.Money(p.PendingRUB))
		add("Из них просрочено, шт.", p.OverdueCount)
		add("Не прошли, шт.", p.FailedCount)
	}
	if s.Recommendations != nil {
		add("Рекомендаций", *s.Recommendations)
		add("Возможная экономия в год, ₽", xlsx.Money(*s.PotentialSavingsRUB))
	}
	return sh
}

// ratesSheet lists the rates the rouble amounts of the workbook were
// converted at. A rate is written as a plain number: it may carry up to 6
// decimals, which the money format would round to 2.
func ratesSheet(rates []domain.Currency) xlsx.Sheet {
	sh := xlsx.Sheet{Name: "Курсы", Header: []string{"Валюта", "Название", "Курс к рублю", "Курс обновлён"}}
	for _, c := range rates {
		var rate any = c.RateToRUB
		if n, err := strconv.ParseFloat(c.RateToRUB, 64); err == nil {
			rate = n
		}
		sh.Rows = append(sh.Rows, []any{c.Code, c.Name, rate, c.UpdatedAt})
	}
	return sh
}

func sliceSheet(name, key string, items []Slice) xlsx.Sheet {
	sh := xlsx.Sheet{Name: name, Header: []string{key, "Подписок", "В месяц, ₽", "Доля, %"}}
	for _, s := range items {
		k := s.Key
		if k == "" {
			k = "(не указано)"
		}
		sh.Rows = append(sh.Rows, []any{k, s.Subscriptions, xlsx.Money(s.MonthlyCostRUB), s.SharePct})
	}
	return sh
}
