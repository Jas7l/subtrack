package auth

import (
	"fmt"
	"os"
	"time"
)

// minSecretLength is the shortest JWT_SECRET accepted, in bytes. HS256 keys
// shorter than the hash output (32 bytes) weaken the signature.
const minSecretLength = 32

// Config carries the auth layer's tunables.
//
// Fields:
//   - Secret    - HMAC key the JWTs are signed with (HS256). Required.
//   - TokenTTL  - how long an issued token stays valid.
//   - Bootstrap - the first admin, created at startup when no active admin
//     exists. Both Email and Password empty means "do not create one".
type Config struct {
	Secret    []byte
	TokenTTL  time.Duration
	Bootstrap BootstrapAdmin
}

// BootstrapAdmin describes the admin account created on an empty database.
// Users can only be created by an admin, so without it nobody could sign in.
type BootstrapAdmin struct {
	Email    string
	Password string
	FullName string
}

// LoadConfig reads JWT_* and BOOTSTRAP_ADMIN_* env vars into a Config.
//
// Recognised env vars:
//   - JWT_SECRET               (string, required, at least 32 bytes)
//   - JWT_TTL                  (time.Duration, default 12h)
//   - BOOTSTRAP_ADMIN_EMAIL    (string, optional)
//   - BOOTSTRAP_ADMIN_PASSWORD (string, optional; required with the email)
//   - BOOTSTRAP_ADMIN_NAME     (string, default "Administrator")
//
// Returns an error if JWT_TTL is present but unparseable. Required values are
// checked by Validate, so a missing secret is reported together with the
// other startup checks.
func LoadConfig() (Config, error) {
	c := Config{
		Secret:   []byte(os.Getenv("JWT_SECRET")),
		TokenTTL: 12 * time.Hour,
		Bootstrap: BootstrapAdmin{
			Email:    os.Getenv("BOOTSTRAP_ADMIN_EMAIL"),
			Password: os.Getenv("BOOTSTRAP_ADMIN_PASSWORD"),
			FullName: envOr("BOOTSTRAP_ADMIN_NAME", "Administrator"),
		},
	}
	if v := os.Getenv("JWT_TTL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("invalid JWT_TTL %q: %w", v, err)
		}
		c.TokenTTL = d
	}
	return c, nil
}

// Validate reports the first configuration problem that would make auth
// unusable or unsafe, or nil.
func (c Config) Validate() error {
	switch {
	case len(c.Secret) == 0:
		return fmt.Errorf("auth: missing required env var JWT_SECRET")
	case len(c.Secret) < minSecretLength:
		return fmt.Errorf("auth: JWT_SECRET must be at least %d bytes, got %d", minSecretLength, len(c.Secret))
	case c.TokenTTL <= 0:
		return fmt.Errorf("auth: JWT_TTL must be positive, got %s", c.TokenTTL)
	case (c.Bootstrap.Email == "") != (c.Bootstrap.Password == ""):
		return fmt.Errorf("auth: BOOTSTRAP_ADMIN_EMAIL and BOOTSTRAP_ADMIN_PASSWORD must be set together")
	}
	return nil
}

func envOr(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}
