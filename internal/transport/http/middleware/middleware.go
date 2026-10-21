// Package middleware contains generic, reusable HTTP middlewares: CORS,
// request id, panic recovery, and structured request logging.
//
// Middleware order in the chain matters. The order cmd/server builds is:
//
//	CORS -> StripTrailingSlash -> RequestID -> Logging -> Recovery -> router
//
// CORS outermost so preflight requests are answered before anything else runs;
// RequestID before Logging so every log line carries it; Recovery inside
// Logging so a panic still produces a completion line with status 500.
//
// Request and response bodies are deliberately NOT logged: POST /auth/login
// carries a plaintext password and its response carries a token, and a log is
// the last place either should end up. The pgx tracer already logs every
// statement with its bound arguments at DEBUG, which is where the data shows.
package middleware

import (
	"context"
	"crypto/rand"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"subtrack/internal/httpx"
	"subtrack/internal/logger"
)

const headerRequestID = "X-Request-ID"

type ctxKey int

const ctxKeyRequestID ctxKey = iota

// StripTrailingSlash collapses any path with a trailing slash (other than "/"
// itself) to its bare form before routing - so /api/v1/users and
// /api/v1/users/ resolve to the same handler. Done in-place rather than via a
// 301 redirect so non-GET verbs are not silently turned into GETs.
func StripTrailingSlash(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p := r.URL.Path; len(p) > 1 && strings.HasSuffix(p, "/") {
			r.URL.Path = strings.TrimRight(p, "/")
		}
		next.ServeHTTP(w, r)
	})
}

// RequestID assigns or propagates a request id and exposes it on the response
// via X-Request-ID. Downstream code can read it with RequestIDFromContext.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(headerRequestID)
		if id == "" {
			id = rand.Text()
		}
		w.Header().Set(headerRequestID, id)
		ctx := context.WithValue(r.Context(), ctxKeyRequestID, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestIDFromContext returns the request id attached by the RequestID
// middleware, or "" if none.
func RequestIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyRequestID).(string); ok {
		return v
	}
	return ""
}

// CORSPolicy is the subset of httpsrv.CORSConfig the CORS middleware needs.
// Declared here so this package does not import the transport/http config
// package.
type CORSPolicy struct {
	AllowedOrigins []string
	AllowedMethods []string
	AllowedHeaders []string
	ExposedHeaders []string
	MaxAge         time.Duration
}

// CORS enforces a Cross-Origin Resource Sharing policy. Apply it as the
// outermost middleware so preflight OPTIONS requests are answered before the
// router or any auth logic runs.
//
// Behaviour:
//   - Requests without an Origin header pass straight through.
//   - "*" in AllowedOrigins matches any origin. Credentials (cookies) are not
//     used - the client sends its JWT in the Authorization header - so "*" is
//     a valid allow value.
//   - Preflight (OPTIONS with Access-Control-Request-Method) returns 204 with
//     the negotiated headers and never reaches the next handler. A
//     non-matching origin gets a 204 without allow-* headers, which the
//     browser treats as a refusal.
func CORS(p CORSPolicy) func(http.Handler) http.Handler {
	allowAny := false
	originSet := make(map[string]struct{}, len(p.AllowedOrigins))
	for _, o := range p.AllowedOrigins {
		if o == "*" {
			allowAny = true
			continue
		}
		originSet[strings.ToLower(o)] = struct{}{}
	}
	allowMethods := strings.Join(p.AllowedMethods, ", ")
	allowHeaders := strings.Join(p.AllowedHeaders, ", ")
	exposeHeaders := strings.Join(p.ExposedHeaders, ", ")
	maxAge := ""
	if p.MaxAge > 0 {
		maxAge = strconv.Itoa(int(p.MaxAge.Seconds()))
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Add("Vary", "Origin")

			allowOrigin := ""
			if _, ok := originSet[strings.ToLower(origin)]; ok {
				allowOrigin = origin
			} else if allowAny {
				allowOrigin = "*"
			}
			if allowOrigin != "" {
				w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
				if exposeHeaders != "" {
					w.Header().Set("Access-Control-Expose-Headers", exposeHeaders)
				}
			}

			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				w.Header().Add("Vary", "Access-Control-Request-Method")
				w.Header().Add("Vary", "Access-Control-Request-Headers")
				if allowOrigin != "" {
					if allowMethods != "" {
						w.Header().Set("Access-Control-Allow-Methods", allowMethods)
					}
					if allowHeaders != "" {
						w.Header().Set("Access-Control-Allow-Headers", allowHeaders)
					}
					if maxAge != "" {
						w.Header().Set("Access-Control-Max-Age", maxAge)
					}
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// Recovery catches panics from downstream handlers, logs the stack through the
// request-scoped logger, and returns 500. Without this, a panic kills the
// request's goroutine and the client sees a dropped connection.
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				logger.FromContext(r.Context()).Error("panic recovered",
					"err", rec,
					"stack", string(debug.Stack()),
				)
				httpx.WriteJSON(r.Context(), w, http.StatusInternalServerError,
					httpx.ErrorBody{Description: "internal error"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// Logging emits one structured line per request on completion and injects a
// request-scoped logger (request_id, method, path, query) into the context, so
// every service and SQL line below it is correlated.
//
// The completion line carries status and duration, plus the internal error a
// handler answered with (via httpx.ErrorSlot) - so a 500 explains itself on
// the line an operator greps. 5xx log at ERROR, everything else at INFO.
func Logging(base *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqLogger := base.With(
				"request_id", RequestIDFromContext(r.Context()),
				"method", r.Method,
				"path", r.URL.Path,
			)
			if r.URL.RawQuery != "" {
				reqLogger = reqLogger.With("query", r.URL.RawQuery)
			}
			reqLogger.Debug("http request start",
				"remote_addr", r.RemoteAddr,
				"user_agent", r.UserAgent(),
				"content_length", r.ContentLength,
			)

			start := time.Now()
			ctx := logger.WithContext(r.Context(), reqLogger)
			ctx, errSlot := httpx.WithErrorSlot(ctx)
			rw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

			defer func() {
				attrs := []any{
					"status", rw.status,
					"duration_ms", time.Since(start).Milliseconds(),
				}
				if err, _, ok := errSlot.Recorded(); ok {
					attrs = append(attrs, "err", err.Error())
				}
				if rw.status >= http.StatusInternalServerError {
					reqLogger.Error("http request", attrs...)
					return
				}
				reqLogger.Info("http request", attrs...)
			}()

			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

// statusRecorder remembers the status code a handler wrote, so Logging can
// report it. A handler that never calls WriteHeader answered 200.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.wroteHeader {
		r.status, r.wroteHeader = code, true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	r.wroteHeader = true
	return r.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the underlying writer (Flush,
// deadlines), which a wrapper otherwise hides.
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }
