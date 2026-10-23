package request

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	"subtrack/internal/auth"
	"subtrack/internal/domain"
	"subtrack/internal/httpx"
)

func TestHTTPRequests(t *testing.T) {
	f := newFixture()
	users := map[int64]domain.User{}
	for _, u := range []domain.User{f.admin, f.head, f.mgr2, f.emp, f.emp2, f.designer} {
		users[u.ID] = u
	}
	r := mux.NewRouter()
	r.NotFoundHandler = http.HandlerFunc(httpx.RouteNotFound)
	protected := r.PathPrefix("/api/v1").Subrouter()
	protected.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			id, _ := strconv.ParseInt(req.Header.Get("X-User"), 10, 64)
			next.ServeHTTP(w, req.WithContext(auth.WithUser(req.Context(), users[id])))
		})
	})
	RegisterRoutes(protected, f.svc)

	tests := []struct {
		name         string
		as           domain.User
		method, path string
		body         string
		want         int
	}{
		{"create as admin", f.admin, "POST", "/requests", `{"name":"X","justification":"y"}`, 403},
		{"create", f.emp, "POST", "/requests", `{"name":"Figma","justification":"макеты","estimated_cost":1500}`, 201},
		{"create unknown field", f.emp, "POST", "/requests", `{"name":"X","justification":"y","status":"approved"}`, 400},
		{"list mine", f.emp, "GET", "/requests", "", 200},
		{"list inbox", f.head, "GET", "/requests?status=pending_manager", "", 200},
		{"list bad status", f.head, "GET", "/requests?status=new", "", 400},
		{"get hidden", f.designer, "GET", "/requests/1", "", 404},
		{"approve as employee", f.emp2, "POST", "/requests/1/approve", `{}`, 403},
		{"approve as non-head", f.mgr2, "POST", "/requests/1/approve", `{}`, 403},
		{"admin too early", f.admin, "POST", "/requests/1/approve", `{"subscription_id":9}`, 409},
		{"reject without comment", f.head, "POST", "/requests/1/reject", `{}`, 400},
		{"head approves", f.head, "POST", "/requests/1/approve", `{"comment":"ок"}`, 200},
		{"admin: both choices", f.admin, "POST", "/requests/1/approve",
			`{"subscription_id":9,"subscription":{"type":"individual","cost":"1","currency":"RUB","billing_period":"month","start_date":"2026-11-01"}}`, 400},
		{"admin approves new", f.admin, "POST", "/requests/1/approve",
			`{"subscription":{"type":"individual","cost":"1","currency":"RUB","billing_period":"month","start_date":"2026-11-01","department_id":null}}`, 200},
		{"cancel approved", f.emp, "POST", "/requests/1/cancel", "", 409},
		{"cancel with GET", f.emp, "GET", "/requests/1/cancel", "", 405},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(tt.method, "/api/v1"+tt.path, strings.NewReader(tt.body))
		req.Header.Set("X-User", strconv.FormatInt(tt.as.ID, 10))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != tt.want {
			t.Errorf("%s: status %d, want %d (body %s)", tt.name, rec.Code, tt.want, rec.Body)
		}
	}
	if len(f.subs.created) != 1 || f.subs.created[0].DepartmentID != nil {
		t.Errorf(`"department_id": null should make it company-wide: %+v`, f.subs.created)
	}
}
