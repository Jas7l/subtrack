// Package web serves the built React client (web/dist) from the Go server,
// so the whole app runs as one process on one origin: no Vite, no CORS.
//
// The client is a single-page app: its addresses (/subscriptions/12) exist
// only in React Router, so any path that is not a file gets index.html and
// the router draws the page. Paths under /api never do: a wrong API call
// must stay a JSON 404, not an HTML page the client cannot read.
package web

import (
	"bytes"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"subtrack/internal/httpx"
)

const index = "index.html"

// Handler serves the files of dir; see the package comment for the
// fallback rules.
//
// Caching: Vite puts a content hash into the names under assets/
// (index-D4wOLytv.js), so the browser may keep them for a year; index.html
// names the current ones and is revalidated on every load, so a new build
// shows up at once.
func Handler(dir string) http.Handler {
	return handler{files: os.DirFS(dir)}
}

type handler struct {
	files fs.FS
}

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
		httpx.RouteNotFound(w, r)
		return
	}
	// path.Clean drops "..", so a request cannot leave dir; os.DirFS
	// refuses such names as well.
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name != "" && name != index {
		if info, err := fs.Stat(h.files, name); err == nil && !info.IsDir() {
			if strings.HasPrefix(name, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			http.ServeFileFS(w, r, h.files, name)
			return
		}
		// A missing file (an asset of an older build, a typo) is a 404:
		// index.html in its place would fail as JavaScript in the browser.
		if path.Ext(name) != "" {
			httpx.RouteNotFound(w, r)
			return
		}
	}
	w.Header().Set("Cache-Control", "no-cache")
	// Served by content, not ServeFileFS: that one redirects a request for
	// /index.html to ./. The file is small and read on every load.
	page, err := fs.ReadFile(h.files, index)
	if err != nil {
		httpx.RouteNotFound(w, r)
		return
	}
	http.ServeContent(w, r, index, time.Time{}, bytes.NewReader(page))
}
