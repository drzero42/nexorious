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

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v5"

	"github.com/drzero42/nexorious-go/internal/api"
	"github.com/drzero42/nexorious-go/internal/config"
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
	// --migrate-only mode: placeholder until the migrator is implemented.
	// -------------------------------------------------------------------------
	if migrateOnly {
		slog.Info("migrate-only mode: migrator not yet implemented, exiting")
		os.Exit(0)
	}

	// -------------------------------------------------------------------------
	// HTTP server
	// -------------------------------------------------------------------------
	e := api.New(cfg)

	addr := fmt.Sprintf(":%d", cfg.Port)
	sc := echo.StartConfig{
		Address:         addr,
		GracefulTimeout: 10 * time.Second,
		HideBanner:      true,
		HidePort:        true,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	slog.Info("nexorious starting", "addr", addr, "version", version, "commit", commit)
	if err := sc.Start(ctx, e); err != nil {
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
