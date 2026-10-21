package logger

import "os"

// Config carries the logger's runtime tunables. Both fields are optional;
// the zero value means "info level, auto-detected format".
//
// Fields:
//   - Level  - slog level name: "debug" | "info" | "warn" | "error".
//     Unknown values fall back to info.
//   - Format - "json" or "console". Empty / "auto" picks console when
//     stdout is a terminal, json otherwise.
type Config struct {
	Level  string // debug | info | warn | error
	Format string // "json", "console", or "" / "auto" for TTY-based detection
}

// LoadConfig reads LOG_LEVEL and LOG_FORMAT from the environment into a
// Config. Missing env vars leave the corresponding field empty (Level
// defaults to "info"). Always returns nil error today; the signature
// keeps room for future validation.
func LoadConfig() (Config, error) {
	c := Config{Level: "info"}
	if v, ok := os.LookupEnv("LOG_LEVEL"); ok {
		c.Level = v
	}
	if v, ok := os.LookupEnv("LOG_FORMAT"); ok {
		c.Format = v
	}
	return c, nil
}
