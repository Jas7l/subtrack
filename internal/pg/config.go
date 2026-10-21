// Package pg holds the shared Postgres plumbing every domain repository
// depends on: connection config, pool construction, the pgx→storage error
// translation helper, and small SQL-building utilities. Per-entity repository
// SQL lives in the domain packages; this package only owns the parts they all
// share.
package pg

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Schema is the Postgres schema every table of the service lives in, created
// by migration 0001. The DSN pins search_path to it, so repository SQL may
// still qualify table names explicitly (subtrack.users) for readability.
const Schema = "subtrack"

// Config carries the discrete connection parameters for a Postgres backend.
// We deliberately avoid a single DSN field - a structured config is easier to
// validate, override per env, and pass around than a string blob.
type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
	SSLMode  string // disable | require | verify-ca | verify-full

	MaxConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
}

// Load reads DB_* env vars into a Config, applying defaults for anything
// unset. If a required value is empty, Validate() reports it and the server
// refuses to start.
//
// Recognised env vars:
//   - DB_HOST               (string, required)
//   - DB_PORT               (int, default 5432)
//   - DB_USER               (string, required)
//   - DB_PASSWORD           (string, required; URL-escaped into the DSN)
//   - DB_NAME               (string, required)
//   - DB_SSLMODE            (string, default "disable")
//   - DB_MAX_CONNS          (int32, default 10)
//   - DB_MAX_CONN_LIFETIME  (time.Duration, default 30m)
//   - DB_MAX_CONN_IDLE_TIME (time.Duration, default 5m)
//
// Returns an error if any present value is not parseable as the expected
// type; missing values silently fall back to defaults.
func Load() (Config, error) {
	c := Config{
		Host:            os.Getenv("DB_HOST"),
		Port:            5432,
		User:            os.Getenv("DB_USER"),
		Password:        os.Getenv("DB_PASSWORD"),
		Database:        os.Getenv("DB_NAME"),
		SSLMode:         envOr("DB_SSLMODE", "disable"),
		MaxConns:        10,
		MaxConnLifetime: 30 * time.Minute,
		MaxConnIdleTime: 5 * time.Minute,
	}
	if err := envInt("DB_PORT", &c.Port); err != nil {
		return Config{}, err
	}
	if err := envInt32("DB_MAX_CONNS", &c.MaxConns); err != nil {
		return Config{}, err
	}
	if err := envDuration("DB_MAX_CONN_LIFETIME", &c.MaxConnLifetime); err != nil {
		return Config{}, err
	}
	if err := envDuration("DB_MAX_CONN_IDLE_TIME", &c.MaxConnIdleTime); err != nil {
		return Config{}, err
	}
	return c, nil
}

// Validate ensures every required DB_* variable is set. Returns an error of
// the form
//
//	"postgres: missing required env vars: DB_USER, DB_NAME"
//
// listing every unset variable, or nil if all required values are present.
func (c Config) Validate() error {
	var missing []string
	if c.Host == "" {
		missing = append(missing, "DB_HOST")
	}
	if c.User == "" {
		missing = append(missing, "DB_USER")
	}
	if c.Password == "" {
		missing = append(missing, "DB_PASSWORD")
	}
	if c.Database == "" {
		missing = append(missing, "DB_NAME")
	}
	if len(missing) > 0 {
		return fmt.Errorf("postgres: missing required env vars: %s", strings.Join(missing, ", "))
	}
	return nil
}

// DSN renders the libpq URL pgx accepts. Password is URL-escaped so values
// with special characters survive intact.
//
// The DSN pins search_path to "subtrack,public" so unqualified table
// references resolve to the service schema. public is kept as a fallback for
// extensions that may install themselves there.
func (c Config) DSN() string {
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.User, c.Password),
		Host:   fmt.Sprintf("%s:%d", c.Host, c.Port),
		Path:   "/" + c.Database,
	}
	q := u.Query()
	q.Set("search_path", Schema+",public")
	if c.SSLMode != "" {
		q.Set("sslmode", c.SSLMode)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func envOr(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

func envInt(key string, dst *int) error {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fmt.Errorf("invalid %s %q: %w", key, v, err)
	}
	*dst = n
	return nil
}

func envInt32(key string, dst *int32) error {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	n, err := strconv.ParseInt(v, 10, 32)
	if err != nil {
		return fmt.Errorf("invalid %s %q: %w", key, v, err)
	}
	*dst = int32(n)
	return nil
}

func envDuration(key string, dst *time.Duration) error {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fmt.Errorf("invalid %s %q: %w", key, v, err)
	}
	*dst = d
	return nil
}
