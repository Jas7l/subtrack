package auth

import (
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// issuer is written into every token's "iss" claim and required on parse, so a
// token signed with the same secret by some other tool is still refused.
const issuer = "subtrack"

// tokens issues and verifies the HS256 JWTs the API authenticates with.
//
// A token carries only the user id ("sub") and its validity window. Role and
// active flag are deliberately not in it: the middleware reloads the user on
// every request, so blocking a user or changing their role takes effect on
// their very next call rather than when the token expires.
type tokens struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time // time.Now outside tests
}

// issue signs a token for userID, valid for t.ttl from now.
func (t tokens) issue(userID int64) (string, time.Time, error) {
	now := t.now()
	exp := now.Add(t.ttl)
	claims := jwt.RegisteredClaims{
		Issuer:    issuer,
		Subject:   strconv.FormatInt(userID, 10),
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(exp),
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(t.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("auth: sign token: %w", err)
	}
	return signed, exp, nil
}

// parse verifies a token's signature, algorithm, issuer and expiry and returns
// the user id it was issued for. Any failure is an error; the caller does not
// need to tell a forged token from an expired one apart.
func (t tokens) parse(raw string) (int64, error) {
	var claims jwt.RegisteredClaims
	_, err := jwt.ParseWithClaims(raw, &claims,
		func(*jwt.Token) (any, error) { return t.secret, nil },
		// Pinning the algorithm closes the classic "alg: none" and
		// RS256-with-HMAC-key confusions.
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(issuer),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(t.now),
	)
	if err != nil {
		return 0, err
	}
	id, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("auth: bad subject %q", claims.Subject)
	}
	return id, nil
}
