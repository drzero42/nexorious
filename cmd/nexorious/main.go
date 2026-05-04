package main

import (
	"context"
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
		showVersion = flag.Bool("version", false, "Print version and exit")
		configFile  = flag.String("config", "", "Path to .env file (default: .env in working directory)")
		migrateOnly = flag.Bool("migrate-only", false, "Run pending migrations then exit (for initContainers)")
	)
	flag.BoolVar(showVersion, "v", false, "Print version and exit (shorthand)")
	flag.Parse()

	if *showVersion {
		fmt.Printf("nexorious %s (%s)\n", version, commit)
		os.Exit(0)
	}

	// -------------------------------------------------------------------------
	// .env file loading
	// -------------------------------------------------------------------------
	envFile := *configFile
	if envFile == "" {
		envFile = ".env"
	}
	// godotenv.Load is a no-op when the file does not exist.
	if err := godotenv.Load(envFile); err != nil && !os.IsNotExist(err) {
		log.Fatalf("failed to load env file %q: %v", envFile, err)
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
	if *migrateOnly {
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
