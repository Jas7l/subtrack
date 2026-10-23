package httpx

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"subtrack/internal/apperr"
)

// DateLayout is how a calendar day travels over the API: "2026-10-23".
const DateLayout = time.DateOnly

// Date is a calendar day in JSON, written and read as "2026-10-23" rather
// than a full RFC 3339 timestamp. The time inside is midnight UTC. Use it with
// `swaggertype:"string" format:"date"`.
type Date time.Time

// MarshalJSON writes the day as a JSON string.
func (d Date) MarshalJSON() ([]byte, error) {
	return []byte(`"` + time.Time(d).Format(DateLayout) + `"`), nil
}

// UnmarshalJSON reads a "YYYY-MM-DD" string.
func (d *Date) UnmarshalJSON(b []byte) error {
	t, err := parseJSONDate(b)
	if err != nil {
		return err
	}
	*d = Date(t)
	return nil
}

// DatePtr converts a nullable day for a DTO.
func DatePtr(t *time.Time) *Date {
	if t == nil {
		return nil
	}
	d := Date(*t)
	return &d
}

// OptionalDate is a PATCH body field holding a nullable day; like OptionalID
// it tells a missing key (Set false) from "key": null (Set true, Value nil).
// Its layout matches domain.DatePatch, so a handler converts with
// domain.DatePatch(req.Field). Use it with `swaggertype:"string"
// format:"date"`.
type OptionalDate struct {
	Set   bool
	Value *time.Time
}

// UnmarshalJSON is only called when the key is present.
func (o *OptionalDate) UnmarshalJSON(b []byte) error {
	o.Set = true
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		o.Value = nil
		return nil
	}
	t, err := parseJSONDate(b)
	if err != nil {
		return err
	}
	o.Value = &t
	return nil
}

func parseJSONDate(b []byte) (time.Time, error) {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return time.Time{}, fmt.Errorf("date must be a string like 2026-10-23")
	}
	t, err := time.Parse(DateLayout, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("date %q is not in the form 2026-10-23", s)
	}
	return t, nil
}

// QueryDate reads an optional "YYYY-MM-DD" query parameter. A missing value
// is nil; a malformed one is apperr.ErrInvalidInput, so a typo in a filter is
// a 400 rather than a silently wider result.
func QueryDate(q url.Values, key string) (*time.Time, error) {
	v := q.Get(key)
	if v == "" {
		return nil, nil
	}
	t, err := time.Parse(DateLayout, v)
	if err != nil {
		return nil, fmt.Errorf("%w: %s must be a date like 2026-10-23", apperr.ErrInvalidInput, key)
	}
	return &t, nil
}
