package middleware

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCORS_PreflightShortCircuits(t *testing.T) {
	reached := false
	h := CORS(CORSPolicy{
		AllowedOrigins: []string{"http://localhost:5173"},
		AllowedMethods: []string{"GET", "POST"},
		AllowedHeaders: []string{"Authorization"},
		MaxAge:         time.Minute,
	})(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true }))

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/users", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if reached {
		t.Error("preflight reached the next handler")
	}
	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Errorf("Allow-Origin = %q", got)
	}
	if got := rec.Header().Get("Access-Control-Max-Age"); got != "60" {
		t.Errorf("Max-Age = %q, want 60", got)
	}
}

func TestCORS_UnknownOriginGetsNoAllowHeader(t *testing.T) {
	h := CORS(CORSPolicy{AllowedOrigins: []string{"http://a.example"}})(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "http://evil.example")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Allow-Origin = %q, want none", got)
	}
}

func TestStripTrailingSlash(t *testing.T) {
	var seen string
	h := StripTrailingSlash(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { seen = r.URL.Path }))
	for in, want := range map[string]string{"/api/v1/users/": "/api/v1/users", "/": "/", "/a": "/a"} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, in, nil))
		if seen != want {
			t.Errorf("%q -> %q, want %q", in, seen, want)
		}
	}
}

func TestRecovery_AnswersPanicWith500(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := RequestID(Logging(log)(Recovery(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	if rec.Header().Get("X-Request-ID") == "" {
		t.Error("X-Request-ID header not set")
	}
}
