package main

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gorilla/mux"

	"subtrack/internal/web"
)

// mountWeb serves the built client (web/dist) on every path the routes
// before it did not take, so the app opens at the server's own address.
// Without a build the server keeps running with the API only: in
// development the client runs on Vite (make web-dev).
func mountWeb(r *mux.Router, cfg web.Config, log *slog.Logger) {
	if cfg.Dir == "" {
		log.Info("web client off: WEB_DIR is empty")
		return
	}
	if _, err := os.Stat(filepath.Join(cfg.Dir, "index.html")); err != nil {
		log.Warn("web client not built, serving the API only (make web-build)", "dir", cfg.Dir)
		return
	}
	r.PathPrefix("/").Handler(web.Handler(cfg.Dir)).Methods(http.MethodGet, http.MethodHead)
	log.Info("web client served", "dir", cfg.Dir)
}
