package main

import (
	"log/slog"
	"net/http"

	httpsrv "subtrack/internal/transport/http"
	"subtrack/internal/transport/http/middleware"
)

// chain wraps the router in the process-wide middleware stack, outermost
// first. See the middleware package doc for why the order is what it is.
func chain(h http.Handler, cfg httpsrv.Config, log *slog.Logger) http.Handler {
	h = middleware.Recovery(h)
	h = middleware.Logging(log)(h)
	h = middleware.RequestID(h)
	h = middleware.StripTrailingSlash(h)
	h = middleware.CORS(middleware.CORSPolicy{
		AllowedOrigins: cfg.CORS.AllowedOrigins,
		AllowedMethods: cfg.CORS.AllowedMethods,
		AllowedHeaders: cfg.CORS.AllowedHeaders,
		ExposedHeaders: cfg.CORS.ExposedHeaders,
		MaxAge:         cfg.CORS.MaxAge,
	})(h)
	return h
}
