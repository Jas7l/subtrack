package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	"subtrack/internal/domain"
	"subtrack/internal/httpx"
)

// newTestRouter lays out routes the way server.Mount does - a public /api/v1
// subrouter and a protected one behind Middleware - plus an admin-only route
// for exercising RequireRole.
func newTestRouter(svc *Service) http.Handler {
	r := mux.NewRouter()
	r.NotFoundHandler = http.HandlerFunc(httpx.RouteNotFound)
	api := r.PathPrefix("/api/v1").Subrouter()
	api.NotFoundHandler = r.NotFoundHandler
	protected := api.NewRoute().Subrouter()
	protected.Use(svc.Middleware)

	RegisterRoutes(api, protected, svc)
	httpx.Register(protected, "/admin-only", map[string]http.HandlerFunc{
		"GET": RequireRole(domain.RoleAdmin)(func(w http.ResponseWriter, r *http.Request) {
			httpx.WriteNoContent(w)
		}),
	})
	return r
}

func do(t *testing.T, h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func loginToken(t *testing.T, h http.Handler, email, pw string) string {
	t.Helper()
	rec := do(t, h, "POST", "/api/v1/auth/login", "", `{"email":"`+email+`","password":"`+pw+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("login %s: status %d, body %s", email, rec.Code, rec.Body)
	}
	var res LoginResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.TokenType != "Bearer" || res.User.Email != email {
		t.Errorf("login response %+v", res)
	}
	if strings.Contains(rec.Body.String(), "password") {
		t.Errorf("login response mentions a password: %s", rec.Body)
	}
	return res.Token
}

func TestHTTPAuthFlow(t *testing.T) {
	repo := newFakeRepo()
	repo.add(t, "admin@corp.ru", "admin-password", domain.RoleAdmin, true)
	repo.add(t, "emp@corp.ru", "employee-password", domain.RoleEmployee, true)
	h := newTestRouter(newTestService(repo))

	adminToken := loginToken(t, h, "admin@corp.ru", "admin-password")
	empToken := loginToken(t, h, "emp@corp.ru", "employee-password")

	tests := []struct {
		name         string
		method, path string
		token, body  string
		want         int
	}{
		{"login wrong password", "POST", "/api/v1/auth/login", "", `{"email":"emp@corp.ru","password":"nope-nope"}`, 401},
		{"login unknown field", "POST", "/api/v1/auth/login", "", `{"login":"emp@corp.ru","password":"x"}`, 400},
		{"login bad json", "POST", "/api/v1/auth/login", "", `{`, 400},
		{"login wrong method", "GET", "/api/v1/auth/login", "", "", 405},
		{"me without token", "GET", "/api/v1/auth/me", "", "", 401},
		{"me with garbage token", "GET", "/api/v1/auth/me", "garbage", "", 401},
		{"me", "GET", "/api/v1/auth/me", empToken, "", 200},
		{"admin route as employee", "GET", "/api/v1/admin-only", empToken, "", 403},
		{"admin route as admin", "GET", "/api/v1/admin-only", adminToken, "", 204},
		{"admin route without token", "GET", "/api/v1/admin-only", "", "", 401},
		// An unknown path is a 404 whether or not the caller is signed in:
		// auth middleware only runs on matched routes.
		{"unknown path, no token", "GET", "/api/v1/nope", "", "", 404},
		{"unknown path, token", "GET", "/api/v1/nope", empToken, "", 404},
		{"change password, wrong current", "POST", "/api/v1/auth/password", empToken,
			`{"current_password":"nope-nope","new_password":"brand-new-pass"}`, 400},
		{"change password", "POST", "/api/v1/auth/password", empToken,
			`{"current_password":"employee-password","new_password":"brand-new-pass"}`, 204},
	}
	for _, tt := range tests {
		rec := do(t, h, tt.method, tt.path, tt.token, tt.body)
		if rec.Code != tt.want {
			t.Errorf("%s: status %d, want %d (body %s)", tt.name, rec.Code, tt.want, rec.Body)
		}
		if rec.Code == http.StatusUnauthorized && rec.Header().Get("WWW-Authenticate") == "" {
			t.Errorf("%s: 401 without WWW-Authenticate", tt.name)
		}
	}

	// The old token survives a password change; the new password works.
	if rec := do(t, h, "GET", "/api/v1/auth/me", empToken, ""); rec.Code != 200 {
		t.Errorf("token after password change: status %d", rec.Code)
	}
	loginToken(t, h, "emp@corp.ru", "brand-new-pass")
}

func TestHTTPBlockedUser(t *testing.T) {
	repo := newFakeRepo()
	emp := repo.add(t, "emp@corp.ru", "employee-password", domain.RoleEmployee, true)
	h := newTestRouter(newTestService(repo))
	token := loginToken(t, h, "emp@corp.ru", "employee-password")

	repo.setActive(emp.ID, false)
	if rec := do(t, h, "GET", "/api/v1/auth/me", token, ""); rec.Code != 401 {
		t.Errorf("blocked user's token: status %d, want 401", rec.Code)
	}
	rec := do(t, h, "POST", "/api/v1/auth/login", "", `{"email":"emp@corp.ru","password":"employee-password"}`)
	if rec.Code != 403 {
		t.Errorf("blocked user's login: status %d, want 403", rec.Code)
	}
}
