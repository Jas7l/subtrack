package subscription

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	"subtrack/internal/auth"
	"subtrack/internal/httpx"
)

// newTestRouter mounts the routes behind a stand-in for auth.Middleware that
// takes the caller from the X-User header.
func newTestRouter(t *testing.T, f fixture) http.Handler {
	t.Helper()
	r := mux.NewRouter()
	r.NotFoundHandler = http.HandlerFunc(httpx.RouteNotFound)
	protected := r.PathPrefix("/api/v1").Subrouter()
	protected.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, _ := strconv.ParseInt(r.Header.Get("X-User"), 10, 64)
			u, ok := f.repo.users[id]
			if !ok {
				t.Fatalf("unknown X-User %q", r.Header.Get("X-User"))
			}
			next.ServeHTTP(w, r.WithContext(auth.WithUser(r.Context(), u)))
		})
	})
	RegisterRoutes(protected, f.svc)
	return r
}

func TestHTTPSubscriptions(t *testing.T) {
	f := newFixture(t)
	h := newTestRouter(t, f)
	admin, mgr, emp := f.admin.ID, f.mgr.ID, f.emp.ID

	do := func(as int64, method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(body))
		req.Header.Set("X-User", strconv.FormatInt(as, 10))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	// A number and a string are both fine for cost; dates are YYYY-MM-DD.
	rec := do(admin, "POST", "/subscriptions",
		`{"name":"Jira","vendor":"Atlassian","category":"Разработка","type":"group","cost":15000.5,
		  "currency":"RUB","billing_period":"month","start_date":"2026-01-01","end_date":"2026-12-31",
		  "department_id":1,"seats_total":2}`)
	if rec.Code != 201 {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	var created SubscriptionDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rec.Body.String(), `"start_date":"2026-01-01"`) || created.SeatsFree == nil || *created.SeatsFree != 2 {
		t.Errorf("create body: %s", rec.Body)
	}
	sub := "/subscriptions/" + strconv.FormatInt(created.ID, 10)

	tests := []struct {
		name         string
		as           int64
		method, path string
		body         string
		want         int
	}{
		{"create as manager", mgr, "POST", "/subscriptions", `{}`, 403},
		{"export as manager", mgr, "GET", "/subscriptions/export", "", 403},
		{"export bad status", admin, "GET", "/subscriptions/export?status=x", "", 400},
		{"export", admin, "GET", "/subscriptions/export?status=active", "", 200},
		{"create bad date", admin, "POST", "/subscriptions",
			`{"name":"X","type":"enterprise","cost":"1","currency":"RUB","billing_period":"year","start_date":"01.01.2026"}`, 400},
		{"create bad cost", admin, "POST", "/subscriptions",
			`{"name":"X","type":"enterprise","cost":"free","currency":"RUB","billing_period":"year","start_date":"2026-01-01"}`, 400},
		{"create string cost", admin, "POST", "/subscriptions",
			`{"name":"Slack","type":"enterprise","cost":"99.90","currency":"usd","billing_period":"year","start_date":"2026-01-01"}`, 201},
		{"list as employee", emp, "GET", "/subscriptions", "", 200},
		{"list filters", admin, "GET", "/subscriptions?status=active&type=group&end_from=2026-06-01&end_to=2027-01-01&category=разработка", "", 200},
		{"list bad status", admin, "GET", "/subscriptions?status=paused", "", 400},
		{"list bad date", admin, "GET", "/subscriptions?end_to=tomorrow", "", 400},
		{"list free seats", admin, "GET", "/subscriptions?free_seats=true", "", 200},
		{"list bad free seats", admin, "GET", "/subscriptions?free_seats=maybe", "", 400},
		{"get as manager", mgr, "GET", sub, "", 200},
		{"get hidden", emp, "GET", sub, "", 404},
		{"patch as manager", mgr, "PATCH", sub, `{"name":"X"}`, 403},
		{"patch unknown field", admin, "PATCH", sub, `{"seats_used":0}`, 400},
		{"assign as employee", emp, "POST", "/assignments", `{"subscription_id":` + strconv.FormatInt(created.ID, 10) + `,"user_id":3}`, 403},
		{"assign missing ids", mgr, "POST", "/assignments", `{}`, 400},
		{"assign", mgr, "POST", "/assignments", `{"subscription_id":` + strconv.FormatInt(created.ID, 10) + `,"user_id":3}`, 201},
		{"assign twice", mgr, "POST", "/assignments", `{"subscription_id":` + strconv.FormatInt(created.ID, 10) + `,"user_id":3}`, 409},
		{"get as holder", emp, "GET", sub, "", 200},
		{"seats down to used", admin, "PATCH", sub, `{"seats_total":1}`, 200},
		{"seats below used", admin, "PATCH", sub, `{"seats_total":0}`, 400},
		{"assign over the limit", admin, "POST", "/assignments", `{"subscription_id":` + strconv.FormatInt(created.ID, 10) + `,"user_id":4}`, 409},
		{"list licences as employee", emp, "GET", "/assignments?status=active", "", 200},
		{"list licences bad status", emp, "GET", "/assignments?status=gone", "", 400},
		{"get licence", emp, "GET", "/assignments/1", "", 200},
		{"get licence of other", f.out.ID, "GET", "/assignments/1", "", 404},
		{"revoke as holder", emp, "POST", "/assignments/1/revoke", "", 403},
		{"ask without licence", emp, "POST", "/release-requests", `{}`, 400},
		{"ask unknown field", emp, "POST", "/release-requests", `{"assignment_id":1,"why":"x"}`, 400},
		{"ask as admin", admin, "POST", "/release-requests", `{"assignment_id":1}`, 403},
		{"ask for another's", mgr, "POST", "/release-requests", `{"assignment_id":1}`, 403},
		{"ask to give it back", emp, "POST", "/release-requests", `{"assignment_id":1,"reason":"Больше не нужна"}`, 201},
		{"ask twice", emp, "POST", "/release-requests", `{"assignment_id":1}`, 409},
		{"list releases", emp, "GET", "/release-requests?status=pending", "", 200},
		{"list releases bad status", emp, "GET", "/release-requests?status=waiting", "", 400},
		{"list releases bad inbox", mgr, "GET", "/release-requests?inbox=maybe", "", 400},
		{"inbox", mgr, "GET", "/release-requests?inbox=true", "", 200},
		{"get release", emp, "GET", "/release-requests/1", "", 200},
		{"get release of other", f.out.ID, "GET", "/release-requests/1", "", 404},
		{"approve as employee", emp, "POST", "/release-requests/1/approve", `{}`, 403},
		{"approve without body", mgr, "POST", "/release-requests/1/approve", "", 400},
		{"reject without comment", mgr, "POST", "/release-requests/1/reject", `{}`, 400},
		{"cancel as reviewer", mgr, "POST", "/release-requests/1/cancel", "", 403},
		{"approve", mgr, "POST", "/release-requests/1/approve", `{"comment":"Ок"}`, 200},
		{"approve twice", mgr, "POST", "/release-requests/1/approve", `{}`, 409},
		{"cancel decided", emp, "POST", "/release-requests/1/cancel", "", 409},
		{"revoke twice", mgr, "POST", "/assignments/1/revoke", "", 409},
		{"revoke with GET", emp, "GET", "/assignments/1/revoke", "", 405},
		{"release with GET", emp, "GET", "/release-requests/1/approve", "", 405},
		{"clear end date", admin, "PATCH", sub, `{"end_date":null}`, 200},
		{"cancel", admin, "PATCH", sub, `{"status":"cancelled"}`, 200},
		{"assign to cancelled", admin, "POST", "/assignments", `{"subscription_id":` + strconv.FormatInt(created.ID, 10) + `,"user_id":4}`, 409},
		{"delete as manager", mgr, "DELETE", sub, "", 403},
		{"delete", admin, "DELETE", sub, "", 204},
		{"delete twice", admin, "DELETE", sub, "", 404},
	}
	for _, tt := range tests {
		rec := do(tt.as, tt.method, tt.path, tt.body)
		if rec.Code != tt.want {
			t.Errorf("%s: status %d, want %d (body %s)", tt.name, rec.Code, tt.want, rec.Body)
		}
	}

	got, _ := f.repo.GetByID(t.Context(), 2)
	if got.Cost != "99.90" || got.Currency != "USD" {
		t.Errorf("string cost / lower-case currency: %+v", got)
	}
}
