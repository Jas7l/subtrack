// Command server is the entrypoint for the SubTrack backend. It loads
// configuration, opens the Postgres pool, mounts the domain routes (via
// internal/server), and starts the HTTP server with graceful shutdown on
// SIGINT / SIGTERM.
//
// Configuration comes from environment variables; a .env file in the working
// directory (or the file named by ENV_FILE) is read first, without overriding
// variables that are already set.
//
// The database schema is not created here: migrations/ holds plain SQL files
// that are applied by hand (see README). If the required DB_* vars are not
// configured, JWT_SECRET is missing, or the database is unreachable, the
// server refuses to start.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"

	"subtrack/internal/config"
	"subtrack/internal/httpx"
	"subtrack/internal/logger"
	"subtrack/internal/pg"
	"subtrack/internal/server"
)

// Version is stamped at build time via -ldflags "-X 'main.Version=...'".
var Version = "dev"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

// run is the real entrypoint, separated from main so it can return errors
// instead of os.Exit-ing at every failure point. It blocks until either the
// server errors out or a SIGINT/SIGTERM triggers graceful shutdown.
func run() error {
	envFile, err := config.LoadEnvFile()
	if err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := logger.New(cfg.Log)
	slog.SetDefault(log)
	log.Info("subtrack starting", "version", Version)
	if envFile != "" {
		log.Info("env file loaded", "path", envFile)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := cfg.DB.Validate(); err != nil {
		return err
	}
	if err := cfg.Auth.Validate(); err != nil {
		return err
	}
	pool, err := pg.NewPool(ctx, cfg.DB)
	if err != nil {
		return err
	}
	defer pool.Close()
	log.Info("postgres pool ready", "host", cfg.DB.Host, "db", cfg.DB.Database)

	r := mux.NewRouter()
	// /healthz answers 200 while the database is reachable and 503 otherwise,
	// so "is the server up?" and "can it do anything?" are one check.
	r.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		pingCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := pool.Ping(pingCtx); err != nil {
			httpx.Error(r.Context(), w, http.StatusServiceUnavailable, errors.New("database unreachable"))
			return
		}
		httpx.WriteOK(r.Context(), w, map[string]string{"status": "ok", "version": Version})
	}).Methods("GET")

	if err := server.Mount(ctx, r, pool, cfg.Auth); err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              cfg.HTTP.Addr,
		Handler:           chain(r, cfg.HTTP, log),
		ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("http server starting", "addr", cfg.HTTP.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case err := <-errCh:
		return err
	}

	// HTTP drains first; the pool closes last via the defer above, so in-flight
	// requests still have connections to finish on.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
