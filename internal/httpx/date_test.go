package httpx

import (
	"encoding/json"
	"net/url"
	"testing"
	"time"
)

func TestDate(t *testing.T) {
	var d Date
	if err := json.Unmarshal([]byte(`"2026-10-23"`), &d); err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(d)
	if string(out) != `"2026-10-23"` {
		t.Errorf("round trip: %s", out)
	}
	for _, bad := range []string{`"23.10.2026"`, `"2026-10-23T00:00:00Z"`, `20261023`} {
		if err := json.Unmarshal([]byte(bad), &d); err == nil {
			t.Errorf("%s: no error", bad)
		}
	}
}

func TestOptionalDate(t *testing.T) {
	type body struct {
		D OptionalDate `json:"d"`
	}
	day := time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		json    string
		set     bool
		value   *time.Time
		wantErr bool
	}{
		{`{}`, false, nil, false},
		{`{"d":null}`, true, nil, false},
		{`{"d":"2026-10-23"}`, true, &day, false},
		{`{"d":"tomorrow"}`, true, nil, true},
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
		if b.D.Set != tt.set {
			t.Errorf("%s: Set = %v, want %v", tt.json, b.D.Set, tt.set)
		}
		if (b.D.Value == nil) != (tt.value == nil) || (b.D.Value != nil && !b.D.Value.Equal(*tt.value)) {
			t.Errorf("%s: Value = %v, want %v", tt.json, b.D.Value, tt.value)
		}
	}
}

func TestQueryDate(t *testing.T) {
	q := url.Values{"from": {"2026-10-23"}, "bad": {"23.10.2026"}}
	if d, err := QueryDate(q, "from"); err != nil || d == nil || d.Day() != 23 {
		t.Errorf("from: %v, %v", d, err)
	}
	if d, err := QueryDate(q, "missing"); err != nil || d != nil {
		t.Errorf("missing: %v, %v", d, err)
	}
	if _, err := QueryDate(q, "bad"); err == nil {
		t.Error("bad: no error")
	}
}
