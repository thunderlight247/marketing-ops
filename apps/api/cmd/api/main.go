// Command api runs the Marketing Operations HTTP API.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"marketingops/api/internal/config"
	"marketingops/api/internal/database"
	"marketingops/api/internal/health"
	"marketingops/api/internal/httpapi"
	"marketingops/api/internal/store"
)

// version is overridden at build time:
//
//	go build -ldflags "-X main.version=1.2.3" ./cmd/api
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)
	logger.Info("starting api", "version", version, "config", cfg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Leave dbPinger as a nil interface when no database is configured.
	var dbPinger health.Pinger
	var workspaceStore httpapi.WorkspaceStore
	if cfg.DatabaseConfigured() {
		pool, err := database.NewPool(ctx, cfg.DatabaseURL)
		if err != nil {
			return err
		}
		defer pool.Close()
		dbPinger = pool
		workspaceStore = store.New(pool)
	} else {
		logger.Warn("DATABASE_URL is not set; running without a database")
	}

	healthService := health.NewService(health.Options{
		Service:     "marketing-ops-api",
		Version:     version,
		Environment: string(cfg.Env),
		Database:    dbPinger,
		Timeout:     cfg.HealthTimeout,
		Logger:      logger,
	})

	srv := &http.Server{
		Addr: cfg.Addr(),
		Handler: httpapi.NewRouter(httpapi.Deps{
			Health:         healthService,
			Logger:         logger,
			AllowedOrigins: cfg.AllowedOrigins,
			Workspaces:     workspaceStore,
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return fmt.Errorf("server error: %w", err)
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}
	logger.Info("server stopped")
	return nil
}
