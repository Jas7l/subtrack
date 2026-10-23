package transaction

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	"subtrack/internal/auth"
	"subtrack/internal/domain"
	"subtrack/internal/httpx"
)

func TestHTTPTransactions(t *testing.T) {
	f := newFixture()
	f.repo.pending(12, "2026-10-23")
	manager := domain.User{ID: 2, Role: domain.RoleManager}

	r := mux.NewRouter()
	r.NotFoundHandler = http.HandlerFunc(httpx.RouteNotFound)
	protected := r.PathPrefix("/api/v1").Subrouter()
	protected.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			u := manager
			if req.Header.Get("X-User") == "admin" {
				u = f.admin
			}
			next.ServeHTTP(w, req.WithContext(auth.WithUser(req.Context(), u)))
		})
	})
	RegisterRoutes(protected, f.svc)

	tests := []struct {
		name, as, method, path, body string
		want                         int
	}{
		{"list as manager", "manager", "GET", "/transactions", "", 403},
		{"pay as manager", "manager", "POST", "/transactions/1/pay", "", 403},
		{"list", "admin", "GET", "/transactions?status=pending&order=asc", "", 200},
		{"list bad status", "admin", "GET", "/transactions?status=new", "", 400},
		{"list bad date", "admin", "GET", "/transactions?from=23.10.2026", "", 400},
		{"list bad subscription", "admin", "GET", "/transactions?subscription_id=x", "", 400},
		{"get", "admin", "GET", "/transactions/1", "", 200},
		{"get unknown", "admin", "GET", "/transactions/9", "", 404},
		{"fail", "admin", "POST", "/transactions/1/fail", "", 200},
		{"fail again", "admin", "POST", "/transactions/1/fail", "", 409},
		{"pay without body", "admin", "POST", "/transactions/1/pay", "", 200},
		{"pay again", "admin", "POST", "/transactions/1/pay", `{"payment_method":"Карта"}`, 409},
		{"pay unknown field", "admin", "POST", "/transactions/1/pay", `{"amount":"1"}`, 400},
		{"create", "admin", "POST", "/transactions",
			`{"subscription_id":12,"amount":1500,"payment_date":"2026-09-01","invoice_number":"INV-1"}`, 201},
		{"create same date", "admin", "POST", "/transactions",
			`{"subscription_id":12,"amount":"1500.00","payment_date":"2026-09-01"}`, 409},
		{"create bad date", "admin", "POST", "/transactions",
			`{"subscription_id":12,"amount":"1","payment_date":"01.09.2026"}`, 400},
		{"create with status", "admin", "POST", "/transactions",
			`{"subscription_id":12,"amount":"1","payment_date":"2026-09-02","status":"pending"}`, 400},
		{"put", "admin", "PUT", "/transactions/1", `{}`, 405},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(tt.method, "/api/v1"+tt.path, strings.NewReader(tt.body))
		req.Header.Set("X-User", tt.as)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != tt.want {
			t.Errorf("%s: status %d, want %d (body %s)", tt.name, rec.Code, tt.want, rec.Body)
		}
	}
}
