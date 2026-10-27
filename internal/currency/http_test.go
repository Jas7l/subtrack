package currency

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	"subtrack/internal/auth"
	"subtrack/internal/domain"
	"subtrack/internal/httpx"
	"subtrack/internal/testkit"
)

func TestHTTPCurrencies(t *testing.T) {
	repo := newFakeRepo()
	users := map[string]domain.User{
		"admin":    admin,
		"manager":  {ID: 2, Role: domain.RoleManager},
		"employee": {ID: 3, Role: domain.RoleEmployee},
	}

	r := mux.NewRouter()
	r.NotFoundHandler = http.HandlerFunc(httpx.RouteNotFound)
	protected := r.PathPrefix("/api/v1").Subrouter()
	protected.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			u := users[req.Header.Get("X-User")]
			next.ServeHTTP(w, req.WithContext(auth.WithUser(req.Context(), u)))
		})
	})
	RegisterRoutes(protected, New(repo, &testkit.Tx{}, &testkit.Audit{}))

	tests := []struct {
		name   string
		as     string
		method string
		path   string
		body   string
		want   int
	}{
		{"list as employee", "employee", "GET", "/currencies", "", 200},
		{"get as employee", "employee", "GET", "/currencies/usd", "", 200},
		{"get unknown", "employee", "GET", "/currencies/GBP", "", 404},
		{"get too long", "employee", "GET", "/currencies/USDT", "", 404},
		{"get digits", "employee", "GET", "/currencies/US1", "", 404},
		{"create as manager", "manager", "POST", "/currencies", `{"code":"GBP","name":"Фунт","rate_to_rub":"110"}`, 403},
		{"patch as employee", "employee", "PATCH", "/currencies/USD", `{"rate_to_rub":"1"}`, 403},
		{"delete as manager", "manager", "DELETE", "/currencies/EUR", "", 403},
		{"create, rate as string", "admin", "POST", "/currencies", `{"code":"gbp","name":"Фунт","rate_to_rub":"110.25"}`, 201},
		{"create, rate as number", "admin", "POST", "/currencies", `{"code":"CNY","name":"Юань","rate_to_rub":12.5}`, 201},
		{"create taken", "admin", "POST", "/currencies", `{"code":"USD","name":"Доллар","rate_to_rub":"90"}`, 409},
		{"create bad rate", "admin", "POST", "/currencies", `{"code":"JPY","name":"Иена","rate_to_rub":"0"}`, 400},
		{"create text rate", "admin", "POST", "/currencies", `{"code":"JPY","name":"Иена","rate_to_rub":"abc"}`, 400},
		{"create unknown field", "admin", "POST", "/currencies", `{"code":"JPY","name":"Иена","rate_to_rub":"0.6","x":1}`, 400},
		{"patch rate", "admin", "PATCH", "/currencies/usd", `{"rate_to_rub":93.1}`, 200},
		{"patch empty", "admin", "PATCH", "/currencies/USD", `{}`, 400},
		{"patch RUB rate", "admin", "PATCH", "/currencies/RUB", `{"rate_to_rub":"2"}`, 409},
		{"patch RUB name", "admin", "PATCH", "/currencies/RUB", `{"name":"Рубль"}`, 200},
		{"patch unknown", "admin", "PATCH", "/currencies/JPY", `{"name":"Иена"}`, 404},
		{"delete in use", "admin", "DELETE", "/currencies/USD", "", 409},
		{"delete RUB", "admin", "DELETE", "/currencies/RUB", "", 409},
		{"delete", "admin", "DELETE", "/currencies/eur", "", 204},
		{"delete again", "admin", "DELETE", "/currencies/EUR", "", 404},
		{"put", "admin", "PUT", "/currencies/USD", `{}`, 405},
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
	if repo.items["GBP"].RateToRUB != "110.25" || repo.items["CNY"].RateToRUB != "12.5" ||
		repo.items["USD"].RateToRUB != "93.1" || repo.items["RUB"].Name != "Рубль" {
		t.Errorf("currencies after the requests = %+v", repo.items)
	}

	// The list body: RUB first, rates as strings.
	req := httptest.NewRequest("GET", "/api/v1/currencies", nil)
	req.Header.Set("X-User", "employee")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	var body ListCurrenciesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	items := body.Items
	if len(items) != 4 || items[0].Code != "RUB" || items[0].RateToRUB != "1" {
		t.Errorf("list = %+v", items)
	}
}
