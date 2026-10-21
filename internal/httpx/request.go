package httpx

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"subtrack/internal/apperr"
	"subtrack/internal/paging"
)

// PathID parses {id} from the gorilla mux route variables. Returns
// apperr.ErrInvalidInput (which handlers map to HTTP 400) when the variable is
// absent, non-numeric, or non-positive.
func PathID(r *http.Request) (int64, error) {
	v, ok := mux.Vars(r)["id"]
	if !ok {
		return 0, apperr.ErrInvalidInput
	}
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil || id <= 0 {
		return 0, apperr.ErrInvalidInput
	}
	return id, nil
}

// PageMeta is the pagination block returned alongside list payloads.
type PageMeta struct {
	Total  int64 `json:"total" example:"123"`
	Limit  int   `json:"limit" example:"50"`
	Offset int   `json:"offset" example:"0"`
}

// ParseListParams reads `?limit=`, `?offset=`, and `?order=` from the URL into
// a normalized paging.Params. Bad values silently fall back to defaults - the
// service clamps limit/offset and whitelists order anyway. `order` is only
// honored by endpoints that support sorting; other list endpoints ignore it.
func ParseListParams(r *http.Request) paging.Params {
	q := r.URL.Query()
	p := paging.Params{}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			p.Limit = n
		}
	}
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			p.Offset = n
		}
	}
	if v := q.Get("order"); v != "" {
		// Pass the raw value through; paging.Params.Normalize coerces anything
		// other than "asc" to the newest-first default, so no validation is
		// needed here.
		p.Order = paging.SortOrder(strings.ToLower(v))
	}
	return p
}

// ParseTimeRange reads `?from=` and `?to=` as RFC3339 timestamps into optional
// inclusive bounds. A missing or unparseable value yields nil for that bound
// (no filter on that side) - the repository treats nil as unbounded, so a
// malformed timestamp simply widens the window rather than erroring.
func ParseTimeRange(r *http.Request) (from, to *time.Time) {
	q := r.URL.Query()
	if v := q.Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			from = &t
		}
	}
	if v := q.Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			to = &t
		}
	}
	return from, to
}

// maxBodyBytes bounds a JSON request body. Every payload this API accepts is a
// small form; anything larger is a mistake or an attack.
const maxBodyBytes = 1 << 20

// DecodeJSON reads the request body into dst. Unknown fields are rejected so a
// mis-spelled key is a 400 the client sees, not a silently empty field. Any
// failure is returned wrapped in apperr.ErrInvalidInput.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("%w: invalid JSON body: %v", apperr.ErrInvalidInput, err)
	}
	return nil
}
