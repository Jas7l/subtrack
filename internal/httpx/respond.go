// Package httpx holds the transport-layer helpers shared by every domain's
// HTTP handlers: the JSON response envelope and write helpers, the pagination
// query parsing, and the per-path route registration helper. Each domain
// package (user, subscription, request, …) owns its own handlers and maps its own
// errors to HTTP statuses using the primitives here.
package httpx

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"subtrack/internal/logger"
)

// ErrorBody is the only response wrapper. Successful responses return the
// resource (or a list payload) directly - the HTTP status carries the success
// signal, no envelope is needed. Failures return an ErrorBody so the client
// always has a stable place to find the human-readable reason.
type ErrorBody struct {
	Description string `json:"description" example:"resource not found"`
}

// WriteJSON writes body as JSON with the given status code, sets the
// Content-Type header, and logs (but does not surface) any encode error.
// Once WriteHeader is called the response is committed; encode failures
// can no longer change the status, so they are intentionally swallowed
// after being logged. ctx carries the request-scoped logger so the log
// line is correlated by request_id.
func WriteJSON(ctx context.Context, w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		logger.FromContext(ctx).Error("write response", "err", err)
	}
}

// WriteOK is the canonical 200 helper: marshals data directly as the
// response body (no envelope) at HTTP 200.
func WriteOK(ctx context.Context, w http.ResponseWriter, data any) {
	WriteJSON(ctx, w, http.StatusOK, data)
}

// WriteCreated is the 201 helper for endpoints that mint a new resource.
func WriteCreated(ctx context.Context, w http.ResponseWriter, data any) {
	WriteJSON(ctx, w, http.StatusCreated, data)
}

// WriteNoContent is the success response for endpoints that have nothing to
// return (e.g. DELETE). 204 + empty body.
func WriteNoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// Error writes an ErrorBody at the given status, using PublicMessage to strip
// internal namespace prefixes so the client never sees them. Use it for the
// 4xx cases a handler maps from its service sentinels (400 / 401 / 403 / 404 /
// 409).
//
// The error is recorded on the request's ErrorSlot so the middleware's
// completion line names it, and logged at DEBUG - a 4xx is the client being
// told it asked for something impossible, not a fault of this service, so it
// does not belong in the default production stream. Under LOG_LEVEL=debug it
// is exactly what you want when a caller reports "your API keeps 400ing me".
func Error(ctx context.Context, w http.ResponseWriter, status int, err error) {
	recordError(ctx, status, err)
	logger.FromContext(ctx).Debug("request rejected",
		"status", status, "err", err.Error(), "description", PublicMessage(err))
	WriteJSON(ctx, w, status, ErrorBody{Description: PublicMessage(err)})
}

// Internal is the 500 helper: it logs the full error with the request id and
// returns a generic message so internal detail never reaches the wire. Use it
// as the default arm of a handler's error switch.
//
// err_type carries the concrete Go type behind the message. The wire message is
// deliberately opaque and the wrapped chain often is too ("postgres: list
// subscriptions: ERROR: …"), so the type is what distinguishes a
// *pgconn.PgError from a *storage.ConstraintError that no writeErr arm claimed
// - the usual reason an error that should have been a 404 or 409 arrives here
// as a 500 instead.
func Internal(ctx context.Context, w http.ResponseWriter, err error) {
	recordError(ctx, http.StatusInternalServerError, err)
	logger.FromContext(ctx).Error("internal error",
		"err", err.Error(), "err_type", fmt.Sprintf("%T", err))
	WriteJSON(ctx, w, http.StatusInternalServerError, ErrorBody{Description: "internal error"})
}

// PublicMessage strips the "service:" / "service: invalid input:" namespace
// prefixes added by lower layers so only the human-readable tail reaches the
// wire. Internal context lives in the structured log line, not the response.
func PublicMessage(err error) string {
	msg := err.Error()
	for _, p := range []string{"service: invalid input: ", "service: "} {
		if strings.HasPrefix(msg, p) {
			msg = msg[len(p):]
			break
		}
	}
	return msg
}

// RouteNotFound and MethodNotAllowed back the router-level fallthrough
// handlers so unrouted requests follow the same envelope/headers as routed
// ones.
func RouteNotFound(w http.ResponseWriter, r *http.Request) {
	WriteJSON(r.Context(), w, http.StatusNotFound, ErrorBody{Description: "route not found"})
}

func MethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	WriteJSON(r.Context(), w, http.StatusMethodNotAllowed, ErrorBody{Description: "method not allowed"})
}
