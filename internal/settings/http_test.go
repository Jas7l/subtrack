package settings

import (
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

func TestHTTPSettings(t *testing.T) {
	repo := &fakeRepo{st: domain.NotificationSettings{ExpiringDays: 7, PaymentDays: 3}}
	admin := domain.User{ID: 1, Role: domain.RoleAdmin}
	manager := domain.User{ID: 2, Role: domain.RoleManager}

	r := mux.NewRouter()
	r.NotFoundHandler = http.HandlerFunc(httpx.RouteNotFound)
	protected := r.PathPrefix("/api/v1").Subrouter()
	protected.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			u := manager
			if req.Header.Get("X-User") == "admin" {
				u = admin
			}
			next.ServeHTTP(w, req.WithContext(auth.WithUser(req.Context(), u)))
		})
	})
	RegisterRoutes(protected, New(repo, &testkit.Tx{}, &testkit.Audit{}))

	tests := []struct {
		name   string
		as     string
		method string
		body   string
		want   int
	}{
		{"get as manager", "manager", "GET", "", 403},
		{"put as manager", "manager", "PUT", `{"expiring_days":1,"payment_days":1}`, 403},
		{"get", "admin", "GET", "", 200},
		{"put one field", "admin", "PUT", `{"expiring_days":14}`, 400},
		{"put out of range", "admin", "PUT", `{"expiring_days":14,"payment_days":400}`, 400},
		{"put unknown field", "admin", "PUT", `{"expiring_days":14,"payment_days":3,"x":1}`, 400},
		{"put", "admin", "PUT", `{"expiring_days":14,"payment_days":0}`, 200},
		{"patch", "admin", "PATCH", `{}`, 405},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(tt.method, "/api/v1/settings/notifications", strings.NewReader(tt.body))
		req.Header.Set("X-User", tt.as)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != tt.want {
			t.Errorf("%s: status %d, want %d (body %s)", tt.name, rec.Code, tt.want, rec.Body)
		}
	}
	if repo.st.ExpiringDays != 14 || repo.st.PaymentDays != 0 {
		t.Errorf("settings = %+v, want 14/0", repo.st)
	}
}
