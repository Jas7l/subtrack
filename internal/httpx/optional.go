package httpx

import (
	"bytes"
	"encoding/json"
)

// OptionalID is a PATCH body field holding a nullable id. encoding/json
// decodes both a missing key and an explicit null into a nil *int64, so a
// plain pointer cannot tell "leave it" from "clear it"; OptionalID can:
//
//   - key absent      → Set false.
//   - "key": null     → Set true, Value nil.
//   - "key": 5        → Set true, Value 5.
//
// Its layout matches domain.IDPatch, so a handler converts with
// domain.IDPatch(req.Field). Use it with `swaggertype:"integer"` so the docs
// show a number rather than an object.
type OptionalID struct {
	Set   bool
	Value *int64
}

// UnmarshalJSON is only called when the key is present, which is what makes
// Set mean "the client sent this field".
func (o *OptionalID) UnmarshalJSON(b []byte) error {
	o.Set = true
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		o.Value = nil
		return nil
	}
	var v int64
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	o.Value = &v
	return nil
}
