package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"

	"github.com/drzero42/nexorious/internal/config"
	"github.com/drzero42/nexorious/internal/migrate"
)

// newMigrateCmd returns the `migrate` subcommand and its `status` child.
// `migrate` replaces the legacy `--migrate-only` flag.
func newMigrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Run pending database migrations and exit",
		Long: "Run all pending database migrations and exit. Intended for use as a\n" +
			"Kubernetes initContainer or one-shot migration step. Replaces the\n" +
			"legacy --migrate-only flag.",
		RunE: runMigrate,
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Print pending migration count and current schema version, then exit",
		Long: "Print the pending migration count, the current applied migration name,\n" +
			"and the migrator state to stdout. Does not modify the database.",
		RunE: runMigrateStatus,
	})
	return cmd
}

// openBunDB builds a *bun.DB from a postgres:// DSN. It is shared between the
// `serve`, `migrate`, and `migrate status` subcommands so connection setup
// lives in one place.
func openBunDB(dsn string) *bun.DB {
	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	return bun.NewDB(sqldb, pgdialect.New())
}

// openPgxPool builds a *pgxpool.Pool from a postgres:// DSN.
// pgx treats a host beginning with '/' as a Unix socket directory and appends
// /.s.PGSQL.<port> itself. Bun's pgdriver (and libpq) accept the full socket
// file path in the host, so DATABASE_URL may already contain the filename. We
// strip it before handing the DSN to pgxpool to avoid a doubled path like
// /run/…/postgres/.s.PGSQL.5432/.s.PGSQL.5432.
// TCP hosts are unaffected (they don't start with '/').
func openPgxPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.ParseConfig: %w", err)
	}
	host := cfg.ConnConfig.Host
	socketFile := fmt.Sprintf("/.s.PGSQL.%d", cfg.ConnConfig.Port)
	if strings.HasPrefix(host, "/") && strings.HasSuffix(host, socketFile) {
		cfg.ConnConfig.Host = strings.TrimSuffix(host, socketFile)
	}
	return pgxpool.NewWithConfig(ctx, cfg)
}

// loadEnvAndConfig resolves --config / .env, loads it into the process env,
// and returns the parsed config. Shared between the migrate subcommands.
func loadEnvAndConfig(cmd *cobra.Command) (*config.Config, error) {
	configFile, _ := cmd.Root().PersistentFlags().GetString("config")
	if configFile != "" {
		if err := godotenv.Load(configFile); err != nil {
			return nil, fmt.Errorf("load env file %q: %w", configFile, err)
		}
	} else {
		if err := godotenv.Load(".env"); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("load .env: %w", err)
		}
	}
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	return cfg, nil
}

// runMigrate runs all pending migrations, then exits. Mirrors the previous
// `--migrate-only` behaviour: retries the initial DB ping for up to 30s, then
// fails hard if the database is still unreachable.
func runMigrate(cmd *cobra.Command, _ []string) error {
	cfg, err := loadEnvAndConfig(cmd)
	if err != nil {
		return err
	}

	db := openBunDB(cfg.DatabaseURL)
	defer func() { _ = db.Close() }()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Retry the initial connection — useful when running as a Kubernetes
	// initContainer where the database pod may still be starting up.
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		err := db.PingContext(pingCtx)
		cancel()
		if err == nil {
			break
		}
		slog.Warn("migrate: waiting for database", "err", err)
		time.Sleep(2 * time.Second)
	}

	pingCtx, pingCancel := context.WithTimeout(ctx, 2*time.Second)
	if err := db.PingContext(pingCtx); err != nil {
		pingCancel()
		return fmt.Errorf("database unreachable after 30s: %w", err)
	}
	pingCancel()

	migrator := migrate.NewMigrator(db)
	if err := migrator.DetermineState(); err != nil {
		return fmt.Errorf("determine state: %w", err)
	}
	if migrator.State() == migrate.AppStateReady {
		slog.Info("migrate: no pending migrations")
		fmt.Println("No pending migrations.")
		return nil
	}

	migrator.SetLogWriter(slog.NewLogLogger(slog.Default().Handler(), slog.LevelInfo).Writer())
	if err := migrator.RunMigrations(ctx); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	slog.Info("migrate: migrations complete")
	fmt.Println("Migrations complete.")
	return nil
}

// runMigrateStatus prints the migration state without applying anything.
func runMigrateStatus(cmd *cobra.Command, _ []string) error {
	cfg, err := loadEnvAndConfig(cmd)
	if err != nil {
		return err
	}

	db := openBunDB(cfg.DatabaseURL)
	defer func() { _ = db.Close() }()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	if err := db.PingContext(pingCtx); err != nil {
		cancel()
		return fmt.Errorf("database unreachable: %w", err)
	}
	cancel()

	migrator := migrate.NewMigrator(db)
	if err := migrator.DetermineState(); err != nil {
		return fmt.Errorf("determine state: %w", err)
	}
	pending, current, err := migrator.Status(ctx)
	if err != nil {
		return fmt.Errorf("status: %w", err)
	}
	out := cmd.OutOrStdout()
	if _, err := fmt.Fprintf(out, "current_version=%s\npending=%d\nstate=%s\n", current, pending, migrator.State()); err != nil {
		return fmt.Errorf("write status: %w", err)
	}
	return nil
}
