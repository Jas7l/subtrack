package password

import (
	"errors"
	"strings"
	"testing"

	"subtrack/internal/apperr"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		pw      string
		wantErr bool
	}{
		{"too short", "1234567", true},
		{"min length", "12345678", false},
		{"max length", strings.Repeat("x", MaxLength), false},
		{"over bcrypt limit", strings.Repeat("x", MaxLength+1), true},
	}
	for _, tt := range tests {
		err := Validate(tt.pw)
		if (err != nil) != tt.wantErr {
			t.Errorf("%s: err = %v, wantErr %v", tt.name, err, tt.wantErr)
		}
		if err != nil && !errors.Is(err, apperr.ErrInvalidInput) {
			t.Errorf("%s: err %v does not wrap ErrInvalidInput", tt.name, err)
		}
	}
}

func TestHashAndCheck(t *testing.T) {
	h, err := Hash("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if h == "correct horse" {
		t.Fatal("hash equals the plaintext")
	}
	if !Check(h, "correct horse") {
		t.Error("Check rejected the right password")
	}
	if Check(h, "wrong horse") {
		t.Error("Check accepted a wrong password")
	}
	if Check("not-a-bcrypt-hash", "correct horse") {
		t.Error("Check accepted a malformed hash")
	}
}
