package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/labstack/echo/v5"

	"github.com/drzero42/nexorious-go/internal/api"
	"github.com/drzero42/nexorious-go/internal/config"
	"github.com/drzero42/nexorious-go/internal/migrate"
)

// Injected at build time via -ldflags.
var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	// -------------------------------------------------------------------------
	// CLI flags
	// -------------------------------------------------------------------------
	var (
		showVersion bool
		configFile  string
		migrateOnly bool
	)
	flag.BoolVar(&showVersion, "version", false, "Print version and exit")
	flag.BoolVar(&showVersion, "v", false, "Print version and exit (shorthand)")
	flag.StringVar(&configFile, "config", "", "Path to .env file (default: .env in working directory)")
	flag.BoolVar(&migrateOnly, "migrate-only", false, "Run pending migrations then exit (for initContainers)")
	flag.Parse()

	if showVersion {
		fmt.Printf("nexorious %s (%s)\n", version, commit)
		os.Exit(0)
	}

	// -------------------------------------------------------------------------
	// .env file loading
	// -------------------------------------------------------------------------
	if configFile != "" {
		// User explicitly provided a config file path — fatal if it doesn't exist.
		if err := godotenv.Load(configFile); err != nil {
			log.Fatalf("failed to load env file %q: %v", configFile, err)
		}
	} else {
		// Default .env is optional — ignore missing file, fatal on other errors.
		if err := godotenv.Load(".env"); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Fatalf("failed to load .env: %v", err)
		}
	}

	// -------------------------------------------------------------------------
	// Config
	// -------------------------------------------------------------------------
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// Configure the global slog logger.
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseSlogLevel(cfg.LogLevel),
	})))

	// -------------------------------------------------------------------------
	// Database pool
	// -------------------------------------------------------------------------
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to create database pool", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	// Verify connectivity before starting anything else.
	pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
	defer pingCancel()
	if err := pool.Ping(pingCtx); err != nil {
		slog.Error("database ping failed", "err", err)
		pool.Close()
		os.Exit(1)
	}
	slog.Info("database connected")

	// -------------------------------------------------------------------------
	// Migrator
	// -------------------------------------------------------------------------
	migrator, err := migrate.NewMigrator(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to create migrator", "err", err)
		pool.Close()
		os.Exit(1)
	}
	defer func() {
		if err := migrator.Close(); err != nil {
			slog.Error("migrator close error", "err", err)
		}
	}()

	// -------------------------------------------------------------------------
	// --migrate-only mode: run migrations then exit (for initContainers).
	// -------------------------------------------------------------------------
	if migrateOnly {
		migrator.SetLogWriter(slog.NewLogLogger(slog.Default().Handler(), slog.LevelInfo).Writer())
		if err := migrator.RunMigrations(ctx); err != nil {
			slog.Error("migrate-only: migrations failed", "err", err)
			pool.Close()
			os.Exit(1)
		}
		slog.Info("migrate-only: migrations complete")
		pool.Close()
		os.Exit(0)
	}

	// -------------------------------------------------------------------------
	// HTTP server
	// -------------------------------------------------------------------------
	e := api.New(cfg, migrator)

	addr := fmt.Sprintf(":%d", cfg.Port)
	sc := echo.StartConfig{
		Address:         addr,
		GracefulTimeout: 10 * time.Second,
		HideBanner:      true,
		HidePort:        true,
	}

	shutdownCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	slog.Info("nexorious starting", "addr", addr, "version", version, "commit", commit)
	if err := sc.Start(shutdownCtx, e); err != nil {
		slog.Info("server stopped", "err", err)
	}
	slog.Info("shutdown complete")
}

// parseSlogLevel maps a LOG_LEVEL string to a slog.Level.
func parseSlogLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
