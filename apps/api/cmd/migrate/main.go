// Command migrate applies or inspects database migrations.
//
//	go run ./cmd/migrate up       apply all pending migrations
//	go run ./cmd/migrate status   list migrations and whether they are applied
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"marketingops/api/internal/config"
	"marketingops/api/internal/database"
	"marketingops/api/internal/migrate"
	"marketingops/api/migrations"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 1 || (args[0] != "up" && args[0] != "status") {
		return fmt.Errorf("usage: migrate <up|status>")
	}

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	if !cfg.DatabaseConfigured() {
		return fmt.Errorf("DATABASE_URL is not set; configure it in apps/api/.env first")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	all, err := migrate.Load(migrations.FS, ".")
	if err != nil {
		return err
	}

	pool, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	switch args[0] {
	case "up":
		applied, err := migrate.Up(ctx, pool, all, logger)
		if err != nil {
			return err
		}
		fmt.Printf("done: %d migration(s) applied, %d total known\n", applied, len(all))
	case "status":
		rows, err := migrate.Status(ctx, pool, all)
		if err != nil {
			return err
		}
		for _, row := range rows {
			state := "pending"
			if row.Applied {
				state = "applied " + row.AppliedAt.UTC().Format(time.RFC3339)
			}
			fmt.Printf("%04d  %-30s  %s\n", row.Version, row.Name, state)
		}
	}
	return nil
}
