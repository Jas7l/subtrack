package httpx

import (
	"encoding/json"
	"testing"
)

func TestOptionalID(t *testing.T) {
	type body struct {
		ID OptionalID `json:"id"`
	}
	tests := []struct {
		json    string
		set     bool
		value   *int64
		wantErr bool
	}{
		{`{}`, false, nil, false},
		{`{"id":null}`, true, nil, false},
		{`{"id":7}`, true, ptr(int64(7)), false},
		{`{"id":"7"}`, true, nil, true},
	}
	for _, tt := range tests {
		var b body
		err := json.Unmarshal([]byte(tt.json), &b)
		if (err != nil) != tt.wantErr {
			t.Errorf("%s: err = %v, wantErr %v", tt.json, err, tt.wantErr)
			continue
		}
		if tt.wantErr {
			continue
		}
		if b.ID.Set != tt.set {
			t.Errorf("%s: Set = %v, want %v", tt.json, b.ID.Set, tt.set)
		}
		if (b.ID.Value == nil) != (tt.value == nil) || (b.ID.Value != nil && *b.ID.Value != *tt.value) {
			t.Errorf("%s: Value = %v, want %v", tt.json, b.ID.Value, tt.value)
		}
	}
}

func ptr[T any](v T) *T { return &v }
