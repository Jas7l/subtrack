// Package httpsrv owns HTTP-layer concerns shared across the transport
// subtree: the server's network/lifecycle configuration and its CORS policy.
//
// The package is named httpsrv (not http) to avoid colliding with the stdlib
// net/http package at call sites. Import as:
//
//	import httpsrv "subtrack/internal/transport/http"
package httpsrv

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Config carries the HTTP server's network and lifecycle parameters.
//
// Fields:
//   - Addr              - listen address in net.Listen form (e.g. ":8080").
//   - ReadHeaderTimeout - maximum time allowed to read request headers;
//     defends against slowloris-style stalls.
//   - ShutdownTimeout   - bound on graceful shutdown; in-flight requests past
//     this point are abandoned.
//   - CORS              - Cross-Origin Resource Sharing policy applied as the
//     outermost middleware, so the browser client on another port can call
//     the API.
type Config struct {
	Addr              string
	ReadHeaderTimeout time.Duration
	ShutdownTimeout   time.Duration
	CORS              CORSConfig
}

// CORSConfig is the CORS policy the middleware enforces. Lists are read from
// comma-separated env vars; "*" in AllowedOrigins matches any origin.
type CORSConfig struct {
	AllowedOrigins []string
	AllowedMethods []string
	AllowedHeaders []string
	ExposedHeaders []string
	MaxAge         time.Duration
}

// Load reads HTTP_* and CORS_* env vars into a Config, applying defaults for
// anything unset.
//
// Recognised env vars:
//   - HTTP_ADDR                (default ":8080")
//   - HTTP_READ_HEADER_TIMEOUT (time.Duration, default 5s)
//   - HTTP_SHUTDOWN_TIMEOUT    (time.Duration, default 10s)
//   - CORS_ALLOWED_ORIGINS     (default "*")
//   - CORS_ALLOWED_METHODS     (default GET,POST,PUT,PATCH,DELETE,OPTIONS)
//   - CORS_ALLOWED_HEADERS     (default Content-Type,Authorization,X-Request-ID)
//   - CORS_EXPOSED_HEADERS     (default X-Request-ID,Content-Disposition)
//   - CORS_MAX_AGE             (time.Duration, default 5m)
//
// Returns an error if a duration is present but unparseable.
func Load() (Config, error) {
	c := Config{
		Addr:              envOr("HTTP_ADDR", ":8080"),
		ReadHeaderTimeout: 5 * time.Second,
		ShutdownTimeout:   10 * time.Second,
		CORS: CORSConfig{
			AllowedOrigins: envList("CORS_ALLOWED_ORIGINS", "*"),
			AllowedMethods: envList("CORS_ALLOWED_METHODS", "GET,POST,PUT,PATCH,DELETE,OPTIONS"),
			AllowedHeaders: envList("CORS_ALLOWED_HEADERS", "Content-Type,Authorization,X-Request-ID"),
			ExposedHeaders: envList("CORS_EXPOSED_HEADERS", "X-Request-ID,Content-Disposition"),
			MaxAge:         5 * time.Minute,
		},
	}
	if err := envDuration("HTTP_READ_HEADER_TIMEOUT", &c.ReadHeaderTimeout); err != nil {
		return Config{}, err
	}
	if err := envDuration("HTTP_SHUTDOWN_TIMEOUT", &c.ShutdownTimeout); err != nil {
		return Config{}, err
	}
	if err := envDuration("CORS_MAX_AGE", &c.CORS.MaxAge); err != nil {
		return Config{}, err
	}
	return c, nil
}

func envOr(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

// envList splits a comma-separated env var, trimming blanks and dropping empty
// entries. An unset or empty variable yields the split default.
func envList(key, def string) []string {
	raw := envOr(key, def)
	out := make([]string, 0, 4)
	for _, p := range strings.Split(raw, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
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
