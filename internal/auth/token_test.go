package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var testSecret = []byte("0123456789abcdef0123456789abcdef")

func fixedClock(t time.Time) func() time.Time { return func() time.Time { return t } }

func TestTokenRoundTrip(t *testing.T) {
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	tk := tokens{secret: testSecret, ttl: time.Hour, now: fixedClock(now)}

	raw, exp, err := tk.issue(42)
	if err != nil {
		t.Fatal(err)
	}
	if !exp.Equal(now.Add(time.Hour)) {
		t.Errorf("expires at %v, want %v", exp, now.Add(time.Hour))
	}
	id, err := tk.parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if id != 42 {
		t.Errorf("id = %d, want 42", id)
	}
}

func TestTokenRejected(t *testing.T) {
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	tk := tokens{secret: testSecret, ttl: time.Hour, now: fixedClock(now)}
	valid, _, err := tk.issue(42)
	if err != nil {
		t.Fatal(err)
	}

	sign := func(method jwt.SigningMethod, key any, claims jwt.RegisteredClaims) string {
		s, err := jwt.NewWithClaims(method, claims).SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	good := jwt.RegisteredClaims{
		Issuer: issuer, Subject: "42",
		IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
	}
	noExp := good
	noExp.ExpiresAt = nil
	otherIssuer := good
	otherIssuer.Issuer = "someone-else"
	badSubject := good
	badSubject.Subject = "admin"

	tests := []struct {
		name string
		raw  string
		tk   tokens
	}{
		{"expired", valid, tokens{secret: testSecret, now: fixedClock(now.Add(2 * time.Hour))}},
		{"other secret", valid, tokens{secret: []byte(strings.Repeat("z", 32)), now: fixedClock(now)}},
		{"tampered", valid[:len(valid)-2] + "xx", tk},
		{"alg none", sign(jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType, good), tk},
		{"HS512", sign(jwt.SigningMethodHS512, testSecret, good), tk},
		{"no exp", sign(jwt.SigningMethodHS256, testSecret, noExp), tk},
		{"other issuer", sign(jwt.SigningMethodHS256, testSecret, otherIssuer), tk},
		{"non-numeric subject", sign(jwt.SigningMethodHS256, testSecret, badSubject), tk},
		{"garbage", "not.a.jwt", tk},
	}
	for _, tt := range tests {
		if id, err := tt.tk.parse(tt.raw); err == nil {
			t.Errorf("%s: accepted, id = %d", tt.name, id)
		}
	}
}
