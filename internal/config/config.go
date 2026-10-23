// Package config composes per-layer configuration into a single Config that
// the cmd/server entrypoint passes around.
//
// Each layer owns its own Config type and Load function (colocated with the
// layer's package). This package only stitches them together - keep it that
// way: do not move layer-specific defaults or env names back here.
package config

import (
	"fmt"

	"subtrack/internal/auth"
	"subtrack/internal/logger"
	"subtrack/internal/pg"
	"subtrack/internal/scheduler"
	httpsrv "subtrack/internal/transport/http"
)

// Config aggregates the per-layer configurations used by the binary. It is
// constructed once at startup; layers receive only the slice they need.
type Config struct {
	HTTP      httpsrv.Config
	DB        pg.Config
	Log       logger.Config
	Auth      auth.Config
	Scheduler scheduler.Config
}

// Load reads each layer's configuration from the environment and stitches the
// results together. Returns an error wrapped as "config: <layer>: ..." if any
// layer's Load fails (typically a malformed env var).
func Load() (Config, error) {
	httpCfg, err := httpsrv.Load()
	if err != nil {
		return Config{}, fmt.Errorf("config: http: %w", err)
	}
	dbCfg, err := pg.Load()
	if err != nil {
		return Config{}, fmt.Errorf("config: db: %w", err)
	}
	logCfg, err := logger.LoadConfig()
	if err != nil {
		return Config{}, fmt.Errorf("config: log: %w", err)
	}
	authCfg, err := auth.LoadConfig()
	if err != nil {
		return Config{}, fmt.Errorf("config: auth: %w", err)
	}
	schedCfg, err := scheduler.LoadConfig()
	if err != nil {
		return Config{}, fmt.Errorf("config: scheduler: %w", err)
	}
	return Config{HTTP: httpCfg, DB: dbCfg, Log: logCfg, Auth: authCfg, Scheduler: schedCfg}, nil
}
