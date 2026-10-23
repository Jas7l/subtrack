package user

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	"subtrack/internal/auth"
	"subtrack/internal/httpx"
)

// newTestRouter mounts the users routes behind a stand-in for auth.Middleware
// that takes the caller from the X-User header, so the tests exercise
// routing, role gates and JSON handling without JWTs.
func newTestRouter(t *testing.T, f fixture) http.Handler {
	t.Helper()
	r := mux.NewRouter()
	r.NotFoundHandler = http.HandlerFunc(httpx.RouteNotFound)
	protected := r.PathPrefix("/api/v1").Subrouter()
	protected.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, _ := strconv.ParseInt(r.Header.Get("X-User"), 10, 64)
			u, err := f.repo.GetByID(r.Context(), id)
			if err != nil {
				t.Fatalf("unknown X-User %q", r.Header.Get("X-User"))
			}
			next.ServeHTTP(w, r.WithContext(auth.WithUser(r.Context(), u)))
		})
	})
	RegisterRoutes(protected, f.svc)
	return r
}

func TestHTTPUsers(t *testing.T) {
	f := newFixture(t)
	h := newTestRouter(t, f)
	admin, mgr, emp := f.admin.ID, f.mgr.ID, f.emp.ID
	path := func(id int64) string { return "/api/v1/users/" + strconv.FormatInt(id, 10) }

	tests := []struct {
		name         string
		as           int64
		method, path string
		body         string
		want         int
	}{
		{"list as admin", admin, "GET", "/api/v1/users", "", 200},
		{"list as manager", mgr, "GET", "/api/v1/users?active=true&role=employee", "", 200},
		{"list as employee", emp, "GET", "/api/v1/users", "", 403},
		{"list bad filter", admin, "GET", "/api/v1/users?active=maybe", "", 400},
		{"list bad role filter", admin, "GET", "/api/v1/users?role=boss", "", 400},
		{"get self", emp, "GET", path(emp), "", 200},
		{"get hidden", emp, "GET", path(mgr), "", 404},
		{"create as manager", mgr, "POST", "/api/v1/users",
			`{"full_name":"X","email":"x@corp.ru","password":"password1","role":"employee"}`, 403},
		{"create", admin, "POST", "/api/v1/users",
			`{"full_name":"X","email":"x@corp.ru","password":"password1","role":"employee","department_id":1}`, 201},
		{"create duplicate", admin, "POST", "/api/v1/users",
			`{"full_name":"X","email":"X@corp.ru","password":"password1","role":"employee"}`, 409},
		{"create unknown department", admin, "POST", "/api/v1/users",
			`{"full_name":"Y","email":"y@corp.ru","password":"password1","role":"employee","department_id":9}`, 400},
		{"create unknown field", admin, "POST", "/api/v1/users", `{"name":"X"}`, 400},
		{"patch as employee", emp, "PATCH", path(emp), `{"full_name":"Me"}`, 403},
		{"patch empty", admin, "PATCH", path(emp), `{}`, 400},
		{"patch block self", admin, "PATCH", path(admin), `{"is_active":false}`, 409},
		{"patch missing", admin, "PATCH", path(999), `{"full_name":"X"}`, 404},
		{"reset password", admin, "PUT", path(emp) + "/password", `{"password":"temporary1"}`, 204},
		{"reset password as manager", mgr, "PUT", path(emp) + "/password", `{"password":"temporary1"}`, 403},
		{"delete is not a thing", admin, "DELETE", path(emp), "", 405},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
		req.Header.Set("X-User", strconv.FormatInt(tt.as, 10))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != tt.want {
			t.Errorf("%s: status %d, want %d (body %s)", tt.name, rec.Code, tt.want, rec.Body)
		}
		if strings.Contains(rec.Body.String(), "password_hash") {
			t.Errorf("%s: response leaks the password hash", tt.name)
		}
	}

	// "department_id": null detaches; an absent key leaves it alone.
	patch := func(body string) {
		req := httptest.NewRequest("PATCH", path(f.emp2.ID), strings.NewReader(body))
		req.Header.Set("X-User", strconv.FormatInt(admin, 10))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("patch %s: status %d, body %s", body, rec.Code, rec.Body)
		}
	}
	patch(`{"full_name":"Renamed"}`)
	if f.repo.users[f.emp2.ID].DepartmentID == nil {
		t.Error("absent department_id cleared the department")
	}
	patch(`{"department_id":null}`)
	if f.repo.users[f.emp2.ID].DepartmentID != nil {
		t.Error("department_id: null did not clear the department")
	}
}
