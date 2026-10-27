// Command seed fills an empty SubTrack database with demo data through the
// HTTP API, the same way a client would: it signs in as the bootstrap admin,
// creates currencies, departments, users, subscriptions, licences and past
// payments, then plays a few software requests through the approval chain.
//
// Run it once against a freshly started server:
//
//	go run ./cmd/seed
//
// The admin's credentials and the server address come from .env
// (BOOTSTRAP_ADMIN_EMAIL, BOOTSTRAP_ADMIN_PASSWORD, HTTP_ADDR) unless given
// as flags. Every demo user gets the same password (-user-password).
//
// Dates are relative to today, so the data looks fresh whenever it is
// seeded. Some of it is set up for the scheduler's next run (the first one
// runs at server startup, then every SCHEDULER_INTERVAL): a payment that is
// already due, a subscription ending within days, one that has expired.
//
// The seed is not repeatable: it refuses to run when the demo departments
// already exist. To start over, empty the tables and restart the server.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"subtrack/internal/config"
)

type client struct {
	base string
	http *http.Client
}

// do sends body as JSON and decodes the response into out (when not nil).
// Any non-2xx answer is an error carrying the server's description.
func (c *client) do(token, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, rd)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		var e struct {
			Description string `json:"description"`
		}
		_ = json.Unmarshal(raw, &e)
		return fmt.Errorf("%s %s: %d %s", method, path, resp.StatusCode, e.Description)
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}

// session is a signed-in user; its methods stop the seed on any error, since
// a half-applied step leaves nothing sensible to continue with.
type session struct {
	c     *client
	token string
}

func (c *client) login(email, password string) session {
	var out struct {
		Token string `json:"token"`
	}
	if err := c.do("", http.MethodPost, "/auth/login", map[string]string{"email": email, "password": password}, &out); err != nil {
		log.Fatalf("login as %s: %v", email, err)
	}
	return session{c: c, token: out.Token}
}

func (s session) call(method, path string, body, out any) {
	if err := s.c.do(s.token, method, path, body, out); err != nil {
		log.Fatal(err)
	}
}

// create POSTs body and returns the id of what was created.
func (s session) create(path string, body any) int64 {
	var out struct {
		ID int64 `json:"id"`
	}
	s.call(http.MethodPost, path, body, &out)
	return out.ID
}

// Demo data.

type deptSeed struct {
	name string
	// head is the email of the manager who runs it; "" leaves it without a
	// head, so its employees' requests go straight to the admin.
	head string
}

var departments = []deptSeed{
	{"Разработка", "ivanov@company.ru"},
	{"Дизайн", "sokolova@company.ru"},
	{"Маркетинг", "lebedev@company.ru"},
	{"Бухгалтерия", ""},
	{"Отдел продаж", "fedorov@company.ru"},
}

type userSeed struct {
	fullName, email, role, dept string
}

var users = []userSeed{
	{"Иванов Иван Сергеевич", "ivanov@company.ru", "manager", "Разработка"},
	{"Смирнов Алексей Викторович", "smirnov@company.ru", "employee", "Разработка"},
	{"Кузнецова Мария Андреевна", "kuznetsova@company.ru", "employee", "Разработка"},
	{"Попов Дмитрий Олегович", "popov@company.ru", "employee", "Разработка"},
	{"Васильев Сергей Николаевич", "vasiliev@company.ru", "employee", "Разработка"},
	{"Новикова Анна Павловна", "novikova@company.ru", "employee", "Разработка"},

	{"Соколова Елена Игоревна", "sokolova@company.ru", "manager", "Дизайн"},
	{"Морозов Артём Денисович", "morozov@company.ru", "employee", "Дизайн"},
	{"Волкова Ольга Михайловна", "volkova@company.ru", "employee", "Дизайн"},

	{"Лебедев Павел Александрович", "lebedev@company.ru", "manager", "Маркетинг"},
	{"Козлова Дарья Евгеньевна", "kozlova@company.ru", "employee", "Маркетинг"},
	{"Егоров Никита Романович", "egorov@company.ru", "employee", "Маркетинг"},

	{"Павлова Татьяна Владимировна", "pavlova@company.ru", "employee", "Бухгалтерия"},
	{"Семёнов Олег Петрович", "semenov@company.ru", "employee", "Бухгалтерия"},

	{"Фёдоров Андрей Станиславович", "fedorov@company.ru", "manager", "Отдел продаж"},
	{"Михайлова Ирина Юрьевна", "mikhailova@company.ru", "employee", "Отдел продаж"},
	{"Николаев Максим Ильич", "nikolaev@company.ru", "employee", "Отдел продаж"},
	{"Орлов Кирилл Вадимович", "orlov@company.ru", "employee", "Отдел продаж"},
	{"Зайцев Роман Геннадьевич", "zaitsev@company.ru", "employee", "Отдел продаж"},
}

// blocked users are deactivated at the end, after they got their licences.
var blocked = []string{"zaitsev@company.ru"}

type subSeed struct {
	name, vendor, category string
	typ                    string // individual | group | enterprise
	cost, currency         string
	period                 string // month | quarter | year
	dept                   string // "" is company-wide
	seats                  int    // group only
	start                  time.Time
	end                    *time.Time
	// overdue leaves the latest past payment date unpaid as the next payment
	// date, for the scheduler to charge.
	overdue bool
	// stopPaying ends the payment history early (with cancel: the
	// subscription is then cancelled).
	stopPaying *time.Time
	cancel     bool
	// holders are emails; "*" means every active user.
	holders []string
}

var today = func() time.Time {
	y, m, d := time.Now().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}()

// monthsAgo is the given day of the month n months before the current one.
func monthsAgo(n, day int) time.Time {
	return time.Date(today.Year(), today.Month()-time.Month(n), day, 0, 0, 0, 0, time.UTC)
}

func days(n int) time.Time { return today.AddDate(0, 0, n) }

func ptr[T any](v T) *T { return &v }

var everyone = []string{"*"}

// The subscriptions are picked so that every recommendation rule has
// something to say (see internal/analytics/recommendations.go).
var subscriptions = []subSeed{
	// Paid monthly for over a year: annual_billing. 6 of 18 seats: unused_seats.
	{name: "Jira Software", vendor: "Atlassian", category: "Управление проектами", typ: "group",
		cost: "12500.00", currency: "RUB", period: "month", dept: "Разработка", seats: 18, start: monthsAgo(16, 5),
		holders: []string{"ivanov@company.ru", "smirnov@company.ru", "kuznetsova@company.ru", "popov@company.ru", "vasiliev@company.ru", "novikova@company.ru"}},
	// Second Atlassian product: duplicate_vendor.
	{name: "Confluence", vendor: "Atlassian", category: "База знаний", typ: "group",
		cost: "6000.00", currency: "RUB", period: "month", dept: "Разработка", seats: 6, start: monthsAgo(10, 12),
		holders: []string{"ivanov@company.ru", "smirnov@company.ru", "kuznetsova@company.ru", "popov@company.ru", "vasiliev@company.ru"}},
	{name: "GitHub Team", vendor: "GitHub", category: "Разработка", typ: "group",
		cost: "144.00", currency: "USD", period: "month", dept: "Разработка", seats: 6, start: monthsAgo(14, 1),
		holders: []string{"ivanov@company.ru", "smirnov@company.ru", "kuznetsova@company.ru", "popov@company.ru", "vasiliev@company.ru", "novikova@company.ru"}},
	{name: "JetBrains All Products Pack", vendor: "JetBrains", category: "Разработка", typ: "group",
		cost: "1299.00", currency: "USD", period: "year", dept: "Разработка", seats: 5, start: monthsAgo(20, 15),
		holders: []string{"smirnov@company.ru", "kuznetsova@company.ru", "popov@company.ru", "vasiliev@company.ru"}},
	// Already due: the scheduler creates a pending payment on its next run.
	{name: "ChatGPT Team", vendor: "OpenAI", category: "ИИ-сервисы", typ: "group",
		cost: "150.00", currency: "USD", period: "month", dept: "Разработка", seats: 5,
		start: days(-3).AddDate(0, -5, 0), overdue: true,
		holders: []string{"ivanov@company.ru", "smirnov@company.ru", "novikova@company.ru"}},

	// 2 of 4 seats now; a request below adds a third.
	{name: "Figma Professional", vendor: "Figma", category: "Дизайн", typ: "group",
		cost: "60.00", currency: "USD", period: "month", dept: "Дизайн", seats: 4, start: monthsAgo(8, 20),
		holders: []string{"sokolova@company.ru", "volkova@company.ru"}},
	// Three individual subscriptions of one product: individual_to_group.
	{name: "Adobe Creative Cloud", vendor: "Adobe", category: "Дизайн", typ: "individual",
		cost: "54.99", currency: "USD", period: "month", dept: "Дизайн", start: monthsAgo(6, 3),
		holders: []string{"sokolova@company.ru"}},
	{name: "Adobe Creative Cloud", vendor: "Adobe", category: "Дизайн", typ: "individual",
		cost: "54.99", currency: "USD", period: "month", dept: "Дизайн", start: monthsAgo(5, 3),
		holders: []string{"morozov@company.ru"}},
	{name: "Adobe Creative Cloud", vendor: "Adobe", category: "Дизайн", typ: "individual",
		cost: "54.99", currency: "USD", period: "month", dept: "Дизайн", start: monthsAgo(4, 3),
		holders: []string{"volkova@company.ru"}},

	// 3 of 10 seats: unused_seats.
	{name: "Miro Business", vendor: "Miro", category: "Совместная работа", typ: "group",
		cost: "160.00", currency: "USD", period: "month", dept: "Маркетинг", seats: 10, start: monthsAgo(9, 10),
		holders: []string{"lebedev@company.ru", "kozlova@company.ru", "egorov@company.ru"}},
	// Nobody uses it: no_holders.
	{name: "Zoom Pro", vendor: "Zoom", category: "Коммуникации", typ: "group",
		cost: "41.98", currency: "EUR", period: "month", dept: "Маркетинг", seats: 3, start: monthsAgo(3, 7)},
	// Expired ten days ago, yet still held: expired_with_holders.
	{name: "Canva Pro", vendor: "Canva", category: "Дизайн", typ: "individual",
		cost: "1490.00", currency: "RUB", period: "month", dept: "Маркетинг",
		start: days(-10).AddDate(-1, 0, 1), end: ptr(days(-10)),
		holders: []string{"kozlova@company.ru"}},
	// Cancelled half a year ago.
	{name: "Trello Standard", vendor: "Atlassian", category: "Управление проектами", typ: "group",
		cost: "50.00", currency: "USD", period: "month", dept: "Маркетинг", seats: 10, start: monthsAgo(14, 18),
		stopPaying: ptr(monthsAgo(6, 18)), cancel: true},

	// Ends in four days: expiring_soon on the scheduler's next run.
	{name: "1С:Бухгалтерия ПРОФ", vendor: "1С", category: "Бухгалтерия", typ: "group",
		cost: "19800.00", currency: "RUB", period: "year", dept: "Бухгалтерия", seats: 2,
		start: days(4).AddDate(-1, 0, 1), end: ptr(days(4)),
		holders: []string{"pavlova@company.ru", "semenov@company.ru"}},
	{name: "Контур.Экстерн", vendor: "СКБ Контур", category: "Бухгалтерия", typ: "individual",
		cost: "4500.00", currency: "RUB", period: "quarter", dept: "Бухгалтерия", start: monthsAgo(13, 25),
		holders: []string{"pavlova@company.ru"}},

	// Next payment in two days: the admins get a reminder.
	{name: "amoCRM Профессиональный", vendor: "amoCRM", category: "CRM", typ: "group",
		cost: "7500.00", currency: "RUB", period: "month", dept: "Отдел продаж", seats: 6,
		start:   days(2).AddDate(0, -7, 0),
		holders: []string{"fedorov@company.ru", "mikhailova@company.ru", "nikolaev@company.ru", "orlov@company.ru", "zaitsev@company.ru"}},

	// Company-wide, used by everyone.
	{name: "Яндекс 360 для бизнеса", vendor: "Яндекс", category: "Коммуникации", typ: "enterprise",
		cost: "54000.00", currency: "RUB", period: "year", start: monthsAgo(11, 1), holders: everyone},
	// Unlimited, but only a handful of people: enterprise_low_usage.
	{name: "Slack Business+", vendor: "Slack", category: "Коммуникации", typ: "enterprise",
		cost: "850.00", currency: "USD", period: "month", start: monthsAgo(7, 14),
		holders: []string{"ivanov@company.ru", "smirnov@company.ru", "kuznetsova@company.ru", "sokolova@company.ru", "lebedev@company.ru", "fedorov@company.ru"}},
	{name: "Kaspersky Endpoint Security", vendor: "Лаборатория Касперского", category: "Безопасность", typ: "enterprise",
		cost: "120000.00", currency: "RUB", period: "year", start: monthsAgo(23, 9), holders: everyone},
}

func step(t time.Time, period string, k int) time.Time {
	switch period {
	case "quarter":
		return t.AddDate(0, 3*k, 0)
	case "year":
		return t.AddDate(k, 0, 0)
	default:
		return t.AddDate(0, k, 0)
	}
}

// schedule splits the subscription's billing dates into the past ones, paid
// by hand here, and the next payment date (nil when it would fall after the
// end, or the subscription is cancelled).
func schedule(s subSeed) (paid []time.Time, next *time.Time) {
	stop := today
	if s.stopPaying != nil && s.stopPaying.Before(stop) {
		stop = *s.stopPaying
	}
	k := 0
	for ; ; k++ {
		d := step(s.start, s.period, k)
		if !d.Before(stop) || (s.end != nil && d.After(*s.end)) {
			break
		}
		paid = append(paid, d)
	}
	if s.cancel {
		return paid, nil
	}
	if s.overdue && len(paid) > 0 {
		last := paid[len(paid)-1]
		return paid[:len(paid)-1], &last
	}
	n := step(s.start, s.period, k)
	if s.end != nil && n.After(*s.end) {
		return paid, nil
	}
	return paid, &n
}

func date(t time.Time) string { return t.Format(time.DateOnly) }

func main() {
	if _, err := config.LoadEnvFile(); err != nil {
		log.Fatal(err)
	}
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	if strings.HasPrefix(addr, ":") {
		addr = "localhost" + addr
	}
	baseURL := flag.String("url", "http://"+addr+"/api/v1", "API base URL")
	adminEmail := flag.String("email", os.Getenv("BOOTSTRAP_ADMIN_EMAIL"), "admin email")
	adminPassword := flag.String("password", os.Getenv("BOOTSTRAP_ADMIN_PASSWORD"), "admin password")
	userPassword := flag.String("user-password", "password123", "password of every demo user")
	flag.Parse()
	log.SetFlags(0)

	c := &client{base: strings.TrimRight(*baseURL, "/"), http: &http.Client{Timeout: 15 * time.Second}}
	admin := c.login(*adminEmail, *adminPassword)

	var existing struct {
		Total int64 `json:"total"`
	}
	admin.call(http.MethodGet, "/departments?q="+url.QueryEscape(departments[0].name), nil, &existing)
	if existing.Total > 0 {
		log.Fatalf("department %q already exists: the database has been seeded", departments[0].name)
	}

	log.Println("currencies")
	for _, cur := range []struct{ code, name, rate string }{
		{"USD", "Доллар США", "95.40"},
		{"EUR", "Евро", "103.10"},
		{"CNY", "Китайский юань", "13.15"},
	} {
		admin.call(http.MethodPost, "/currencies", map[string]any{"code": cur.code, "name": cur.name, "rate_to_rub": cur.rate}, nil)
	}

	log.Println("departments")
	deptIDs := map[string]int64{}
	for _, d := range departments {
		deptIDs[d.name] = admin.create("/departments", map[string]any{"name": d.name})
	}

	log.Println("users")
	userIDs := map[string]int64{}
	for _, u := range users {
		userIDs[u.email] = admin.create("/users", map[string]any{
			"full_name": u.fullName, "email": u.email, "password": *userPassword,
			"role": u.role, "department_id": deptIDs[u.dept],
		})
	}
	for _, d := range departments {
		if d.head != "" {
			admin.call(http.MethodPatch, fmt.Sprintf("/departments/%d", deptIDs[d.name]), map[string]any{"head_id": userIDs[d.head]}, nil)
		}
	}

	log.Println("subscriptions, licences and payments")
	subIDs := map[string]int64{}
	payments := 0
	for _, s := range subscriptions {
		paid, next := schedule(s)
		body := map[string]any{
			"name": s.name, "vendor": s.vendor, "category": s.category, "type": s.typ,
			"cost": s.cost, "currency": s.currency, "billing_period": s.period, "start_date": date(s.start),
		}
		if s.end != nil {
			body["end_date"] = date(*s.end)
		}
		if next != nil {
			body["next_payment_date"] = date(*next)
		}
		if s.dept != "" {
			body["department_id"] = deptIDs[s.dept]
		}
		if s.typ == "group" {
			body["seats_total"] = s.seats
		}
		id := admin.create("/subscriptions", body)
		subIDs[s.name] = id

		holders := s.holders
		if len(holders) == 1 && holders[0] == "*" {
			holders = nil
			for _, u := range users {
				holders = append(holders, u.email)
			}
		}
		for _, email := range holders {
			admin.call(http.MethodPost, "/assignments", map[string]any{"subscription_id": id, "user_id": userIDs[email]}, nil)
		}

		method := "Счёт"
		if s.currency != "RUB" {
			method = "Корпоративная карта"
		}
		for _, d := range paid {
			admin.call(http.MethodPost, "/transactions", map[string]any{
				"subscription_id": id, "amount": s.cost, "payment_date": date(d),
				"payment_method": method, "invoice_number": fmt.Sprintf("INV-%s-%03d", d.Format("200601"), id),
			}, nil)
			payments++
		}
		if s.cancel {
			admin.call(http.MethodPatch, fmt.Sprintf("/subscriptions/%d", id), map[string]any{"status": "cancelled"}, nil)
		}
	}

	log.Println("requests")
	as := map[string]session{}
	sess := func(email string) session {
		if s, ok := as[email]; ok {
			return s
		}
		s := c.login(email, *userPassword)
		as[email] = s
		return s
	}
	ask := func(email string, body map[string]any) int64 {
		if _, ok := body["currency"]; !ok {
			body["currency"] = "RUB"
		}
		return sess(email).create("/requests", body)
	}
	decide := func(who session, id int64, verdict string, body map[string]any) {
		who.call(http.MethodPost, fmt.Sprintf("/requests/%d/%s", id, verdict), body, nil)
	}

	// Approved by the head and the admin, with a new subscription.
	r := ask("popov@company.ru", map[string]any{"name": "Postman", "vendor": "Postman Inc.",
		"justification": "Тестирование REST API сервисов", "estimated_cost": "70.00", "currency": "USD"})
	decide(sess("ivanov@company.ru"), r, "approve", map[string]any{"comment": "Нужен команде бэкенда"})
	decide(admin, r, "approve", map[string]any{"comment": "Оформили командную подписку", "subscription": map[string]any{
		"category": "Разработка", "type": "group", "cost": "70.00", "currency": "USD", "billing_period": "month",
		"start_date": date(today), "next_payment_date": date(today.AddDate(0, 1, 0)), "seats_total": 3,
	}})

	// Approved: a free seat of an existing subscription.
	r = ask("morozov@company.ru", map[string]any{"name": "Figma", "vendor": "Figma",
		"justification": "Работа над макетами нового личного кабинета"})
	decide(sess("sokolova@company.ru"), r, "approve", nil)
	decide(admin, r, "approve", map[string]any{"subscription_id": subIDs["Figma Professional"]})

	// Rejected by the head.
	r = ask("novikova@company.ru", map[string]any{"name": "GitHub Copilot", "vendor": "GitHub",
		"justification": "Ускорить написание кода", "estimated_cost": "19.00", "currency": "USD"})
	decide(sess("ivanov@company.ru"), r, "reject", map[string]any{"comment": "Пока используем ChatGPT Team, вернёмся к вопросу в следующем квартале"})

	// No head in the department: straight to the admin, who rejects.
	r = ask("semenov@company.ru", map[string]any{"name": "Контур.Экстерн", "vendor": "СКБ Контур",
		"justification": "Сдача отчётности", "estimated_cost": "4500.00"})
	decide(admin, r, "reject", map[string]any{"comment": "У отдела уже есть Контур.Экстерн, лицензию выдаст Павлова Т. В."})

	// A manager's own request goes to the admin, who approves with an
	// individual subscription.
	r = ask("lebedev@company.ru", map[string]any{"name": "Tilda Business", "vendor": "Tilda",
		"justification": "Лендинги для рекламных кампаний", "estimated_cost": "12000.00"})
	decide(admin, r, "approve", map[string]any{"subscription": map[string]any{
		"category": "Маркетинг", "type": "individual", "cost": "12000.00", "currency": "RUB", "billing_period": "year",
		"start_date": date(today), "end_date": date(today.AddDate(1, 0, -1)),
	}})

	// Cancelled by the requester.
	r = ask("smirnov@company.ru", map[string]any{"name": "Docker Desktop", "vendor": "Docker",
		"justification": "Локальный запуск окружения", "estimated_cost": "9.00", "currency": "USD"})
	sess("smirnov@company.ru").call(http.MethodPost, fmt.Sprintf("/requests/%d/cancel", r), nil, nil)

	// Left pending: one waiting for the admin, two for the head.
	r = ask("egorov@company.ru", map[string]any{"name": "Notion", "vendor": "Notion Labs",
		"justification": "Контент-план и база знаний отдела", "estimated_cost": "10.00", "currency": "USD"})
	decide(sess("lebedev@company.ru"), r, "approve", map[string]any{"comment": "Поддерживаю"})
	ask("mikhailova@company.ru", map[string]any{"name": "Контур.Диадок", "vendor": "СКБ Контур",
		"justification": "Электронный документооборот с клиентами", "estimated_cost": "3000.00"})
	ask("kuznetsova@company.ru", map[string]any{"name": "DataGrip", "vendor": "JetBrains",
		"justification":  "Работа с базами данных, отдельная лицензия для аналитика",
		"estimated_cost": "229.00", "currency": "USD"})

	for _, email := range blocked {
		admin.call(http.MethodPatch, fmt.Sprintf("/users/%d", userIDs[email]), map[string]any{"is_active": false}, nil)
	}

	log.Printf("done: %d departments, %d users, %d subscriptions, %d payments, 9 requests",
		len(departments), len(users), len(subscriptions)+2, payments)
	log.Printf("every demo user signs in with password %q, e.g. ivanov@company.ru (manager), smirnov@company.ru (employee)", *userPassword)
}
