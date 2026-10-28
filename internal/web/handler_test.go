package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// dist builds a small web/dist in a temp folder.
func dist(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"index.html":           "<!doctype html><title>SubTrack</title>",
		"favicon.svg":          "<svg></svg>",
		"assets/index-abc.js":  "console.log(1)",
		"assets/index-abc.css": "body{}",
	}
	for name, body := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// A secret next to dist must stay out of reach.
	if err := os.WriteFile(filepath.Join(filepath.Dir(dir), "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func get(h http.Handler, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func TestHandler(t *testing.T) {
	h := Handler(dist(t))
	tests := []struct {
		name, target string
		status       int
		body         string
		cache        string
	}{
		{"root is the app", "/", 200, "<title>SubTrack</title>", "no-cache"},
		{"index.html itself", "/index.html", 200, "<title>SubTrack</title>", "no-cache"},
		{"client route falls back", "/subscriptions/12", 200, "<title>SubTrack</title>", "no-cache"},
		{"client route with a query", "/transactions?tab=all", 200, "<title>SubTrack</title>", "no-cache"},
		{"hashed asset is cached", "/assets/index-abc.js", 200, "console.log(1)", "public, max-age=31536000, immutable"},
		{"other file", "/favicon.svg", 200, "<svg></svg>", ""},
		{"missing asset is 404", "/assets/index-old.js", 404, "route not found", ""},
		{"folder is the app", "/assets", 200, "<title>SubTrack</title>", "no-cache"},
		{"unknown API path stays JSON", "/api/v1/nope", 404, `"description":"route not found"`, ""},
		{"bare /api stays JSON", "/api", 404, "route not found", ""},
		{"no way out of dist", "/../secret.txt", 404, "route not found", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := get(h, tc.target)
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d (body %q)", rec.Code, tc.status, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tc.body) {
				t.Errorf("body = %q, want it to contain %q", rec.Body.String(), tc.body)
			}
			if got := rec.Header().Get("Cache-Control"); got != tc.cache {
				t.Errorf("Cache-Control = %q, want %q", got, tc.cache)
			}
		})
	}
}

func TestHandler_NoBuild(t *testing.T) {
	// dist without index.html (the client was never built): a 404, not a
	// crash.
	rec := get(Handler(t.TempDir()), "/users")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestLoadConfig(t *testing.T) {
	t.Setenv("WEB_DIR", "")
	os.Unsetenv("WEB_DIR")
	if got := LoadConfig().Dir; got != "web/dist" {
		t.Errorf("unset: Dir = %q, want web/dist", got)
	}
	t.Setenv("WEB_DIR", "")
	if got := LoadConfig().Dir; got != "" {
		t.Errorf("empty: Dir = %q, want empty (API only)", got)
	}
	t.Setenv("WEB_DIR", "/srv/subtrack")
	if got := LoadConfig().Dir; got != "/srv/subtrack" {
		t.Errorf("set: Dir = %q", got)
	}
}
