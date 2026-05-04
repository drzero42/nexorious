# Config, CLI Flags & Database Pool Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expand the config struct to the full spec, add CLI flags (`--version`, `--config`, `--migrate-only`), and open a verified `pgxpool` database connection on startup.

**Architecture:** Config is expanded in-place in `internal/config/config.go` with a post-parse hook that assembles `DatabaseURL` from individual `DB_*` vars when `DATABASE_URL` is absent. CLI flags are parsed in `main.go` before config loading; `--config` feeds `.env` file loading via `godotenv`. The `pgxpool` is opened in `main.go` and injected as a dependency; startup aborts if the initial ping fails.

**Tech Stack:** `caarlos0/env/v11`, `jackc/pgx/v5` (`pgxpool`), `joho/godotenv`, stdlib `flag`, stdlib `log/slog`

---

## File Map

| File | Action | Responsibility |
|---|---|---|
| `internal/config/config.go` | Modify | Full config struct + `DatabaseURL` post-parse hook |
| `internal/config/config_test.go` | Create | Unit tests for config loading and URL assembly |
| `cmd/nexorious/main.go` | Modify | CLI flags, `.env` loading, pgxpool open/ping/close |
| `go.mod` / `go.sum` | Modify (via `go get`) | Add `pgx/v5`, `godotenv` dependencies |
| `Makefile` | Modify | Add `VERSION`/`COMMIT` injection via `-ldflags`; add `sqlc` target |

---

### Task 1: Add `pgx/v5` and `godotenv` dependencies

**Files:**
- Modify: `go.mod`, `go.sum` (via `go get`)

- [ ] **Step 1: Add the dependencies**

```bash
cd /home/abo/workspace/home/nexorious-go
go get github.com/jackc/pgx/v5@latest
go get github.com/joho/godotenv@latest
```

Expected output: lines beginning `go: added github.com/jackc/pgx/v5 ...` and `go: added github.com/joho/godotenv ...`

- [ ] **Step 2: Verify the build still passes**

```bash
go build ./...
```

Expected: exits 0, no output.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add pgx/v5 and godotenv dependencies"
```

---

### Task 2: Expand Config struct

**Files:**
- Modify: `internal/config/config.go`
- Create: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/config/config_test.go`:

```go
package config_test

import (
	"os"
	"testing"
)

func TestLoad_DatabaseURLFromIndividualVars(t *testing.T) {
	// Clear DATABASE_URL so the fallback path is exercised.
	os.Unsetenv("DATABASE_URL")
	os.Setenv("DB_HOST", "db.example.com")
	os.Setenv("DB_PORT", "5433")
	os.Setenv("DB_USER", "myuser")
	os.Setenv("DB_PASSWORD", "p@ss word!")
	os.Setenv("DB_NAME", "mydb")
	// Required fields.
	os.Setenv("SECRET_KEY", "testsecretkey")
	os.Setenv("IGDB_CLIENT_ID", "testclientid")
	os.Setenv("IGDB_CLIENT_SECRET", "testclientsecret")
	t.Cleanup(func() {
		for _, k := range []string{
			"DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_NAME",
			"SECRET_KEY", "IGDB_CLIENT_ID", "IGDB_CLIENT_SECRET",
		} {
			os.Unsetenv(k)
		}
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	// Password and user must be percent-encoded; special chars in password.
	want := "postgresql://myuser:p%40ss+word%21@db.example.com:5433/mydb"
	if cfg.DatabaseURL != want {
		t.Errorf("DatabaseURL = %q; want %q", cfg.DatabaseURL, want)
	}
}

func TestLoad_DatabaseURLExplicit(t *testing.T) {
	os.Setenv("DATABASE_URL", "postgresql://override:pass@host/db")
	os.Setenv("SECRET_KEY", "testsecretkey")
	os.Setenv("IGDB_CLIENT_ID", "testclientid")
	os.Setenv("IGDB_CLIENT_SECRET", "testclientsecret")
	t.Cleanup(func() {
		for _, k := range []string{
			"DATABASE_URL", "SECRET_KEY", "IGDB_CLIENT_ID", "IGDB_CLIENT_SECRET",
		} {
			os.Unsetenv(k)
		}
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.DatabaseURL != "postgresql://override:pass@host/db" {
		t.Errorf("DatabaseURL = %q; want explicit value", cfg.DatabaseURL)
	}
}

func TestLoad_RequiredFieldsMissing(t *testing.T) {
	os.Unsetenv("SECRET_KEY")
	os.Unsetenv("IGDB_CLIENT_ID")
	os.Unsetenv("IGDB_CLIENT_SECRET")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when required fields are missing, got nil")
	}
}

func TestLoad_Defaults(t *testing.T) {
	os.Setenv("SECRET_KEY", "testsecretkey")
	os.Setenv("IGDB_CLIENT_ID", "testclientid")
	os.Setenv("IGDB_CLIENT_SECRET", "testclientsecret")
	t.Cleanup(func() {
		for _, k := range []string{
			"SECRET_KEY", "IGDB_CLIENT_ID", "IGDB_CLIENT_SECRET",
		} {
			os.Unsetenv(k)
		}
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Port != 8000 {
		t.Errorf("Port = %d; want 8000", cfg.Port)
	}
	if cfg.WorkerCount != 4 {
		t.Errorf("WorkerCount = %d; want 4", cfg.WorkerCount)
	}
	if cfg.AccessTokenExpireMinutes != 15 {
		t.Errorf("AccessTokenExpireMinutes = %d; want 15", cfg.AccessTokenExpireMinutes)
	}
	if cfg.RateLimiterBackend != "local" {
		t.Errorf("RateLimiterBackend = %q; want local", cfg.RateLimiterBackend)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/config/... -v
```

Expected: FAIL — `Load` and `Config` don't have the new fields yet. The `TestLoad_RequiredFieldsMissing` test may pass or fail depending on current state; all others should fail.

- [ ] **Step 3: Replace `internal/config/config.go` with the full implementation**

```go
package config

import (
	"fmt"
	"net/url"

	"github.com/caarlos0/env/v11"
)

// Config holds all application configuration parsed from environment variables.
type Config struct {
	// -------------------------------------------------------------------------
	// Database
	// -------------------------------------------------------------------------

	// DatabaseURL takes priority when non-empty. When absent, the URL is
	// constructed from the individual DB_* vars below.
	DatabaseURL string `env:"DATABASE_URL"`

	// Individual DB connection vars — fallback when DATABASE_URL is not set.
	// Defaults match the dev URL: postgresql://nexorious:nexorious@localhost:5432/nexorious
	DbHost     string `env:"DB_HOST"     envDefault:"localhost"`
	DbPort     int    `env:"DB_PORT"     envDefault:"5432"`
	DbUser     string `env:"DB_USER"     envDefault:"nexorious"`
	DbPassword string `env:"DB_PASSWORD" envDefault:"nexorious"`
	DbName     string `env:"DB_NAME"     envDefault:"nexorious"`

	// -------------------------------------------------------------------------
	// Security
	// -------------------------------------------------------------------------

	// SecretKey is used for JWT signing and credential encryption.
	SecretKey string `env:"SECRET_KEY,required"`

	// JWT lifetimes. Go port uses 15 min access (Python defaulted to 30).
	AccessTokenExpireMinutes int `env:"ACCESS_TOKEN_EXPIRE_MINUTES" envDefault:"15"`
	RefreshTokenExpireDays   int `env:"REFRESH_TOKEN_EXPIRE_DAYS"   envDefault:"30"`

	// -------------------------------------------------------------------------
	// IGDB
	// -------------------------------------------------------------------------

	IGDBClientID          string  `env:"IGDB_CLIENT_ID,required"`
	IGDBClientSecret      string  `env:"IGDB_CLIENT_SECRET,required"`
	IGDBAccessToken       string  `env:"IGDB_ACCESS_TOKEN"`
	IGDBRequestsPerSecond float64 `env:"IGDB_REQUESTS_PER_SECOND" envDefault:"4.0"`
	IGDBBurstCapacity     int     `env:"IGDB_BURST_CAPACITY"      envDefault:"8"`
	IGDBMaxRetries        int     `env:"IGDB_MAX_RETRIES"         envDefault:"3"`
	IGDBBackoffFactor     float64 `env:"IGDB_BACKOFF_FACTOR"      envDefault:"1.0"`

	// -------------------------------------------------------------------------
	// Steam
	// -------------------------------------------------------------------------

	SteamRequestsPerSecond float64 `env:"STEAM_REQUESTS_PER_SECOND" envDefault:"4.0"`
	SteamBurstCapacity     int     `env:"STEAM_BURST_CAPACITY"      envDefault:"10"`

	// -------------------------------------------------------------------------
	// Storage
	// -------------------------------------------------------------------------

	StoragePath    string `env:"STORAGE_PATH"     envDefault:"./storage"`
	BackupPath     string `env:"BACKUP_PATH"      envDefault:"./storage/backups"`
	TempStorageDir string `env:"TEMP_STORAGE_DIR" envDefault:"/tmp/nexorious_uploads"`

	// -------------------------------------------------------------------------
	// Application
	// -------------------------------------------------------------------------

	Port     int    `env:"PORT"      envDefault:"8000"`
	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`
	Debug    bool   `env:"DEBUG"     envDefault:"false"`

	// CORSOrigins is only needed in development; production is same-origin.
	CORSOrigins []string `env:"CORS_ORIGINS" envSeparator:","`

	// -------------------------------------------------------------------------
	// Workers
	// -------------------------------------------------------------------------

	WorkerCount int `env:"WORKER_COUNT" envDefault:"4"`

	// -------------------------------------------------------------------------
	// Scheduler
	// -------------------------------------------------------------------------

	// MetadataRefreshInterval is a Go duration string (e.g. "24h").
	// The backup schedule is stored in the backup_config table, not here.
	MetadataRefreshInterval string `env:"METADATA_REFRESH_INTERVAL" envDefault:"24h"`

	// -------------------------------------------------------------------------
	// Rate limiter
	// -------------------------------------------------------------------------

	// RateLimiterBackend selects the rate limiter implementation: "local" or "postgres".
	RateLimiterBackend string `env:"RATE_LIMITER_BACKEND" envDefault:"local"`
}

// Load parses Config from environment variables and assembles DatabaseURL
// when DATABASE_URL is not set explicitly.
func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	cfg.resolveDatabaseURL()
	return cfg, nil
}

// resolveDatabaseURL builds DatabaseURL from individual DB_* vars when
// DATABASE_URL is not set. Special characters in user/password are
// percent-encoded, matching Python's urllib.parse.quote(value, safe='').
func (c *Config) resolveDatabaseURL() {
	if c.DatabaseURL != "" {
		return
	}
	user := url.QueryEscape(c.DbUser)
	pass := url.QueryEscape(c.DbPassword)
	c.DatabaseURL = fmt.Sprintf(
		"postgresql://%s:%s@%s:%d/%s",
		user, pass, c.DbHost, c.DbPort, c.DbName,
	)
}
```

- [ ] **Step 4: Run tests and verify they pass**

```bash
go test ./internal/config/... -v
```

Expected: all four tests PASS.

- [ ] **Step 5: Verify the full build still compiles**

```bash
go build ./...
```

Expected: exits 0.

- [ ] **Step 6: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: expand Config struct with all spec fields + DatabaseURL resolution"
```

---

### Task 3: Add CLI flags and `.env` loading to `main.go`

**Files:**
- Modify: `cmd/nexorious/main.go`
- Modify: `Makefile`

The binary accepts `--version`, `--config`, and `--migrate-only` via stdlib `flag`. Version and commit are injected at build time via `-ldflags`.

- [ ] **Step 1: Update the Makefile to inject version info**

Replace the existing `Makefile` content:

```makefile
.PHONY: all frontend sqlc build test

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS  = -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT)"

all: frontend sqlc build

frontend:
	cd ui && npm install && npm run build

sqlc:
	sqlc generate

build:
	go build $(LDFLAGS) -o nexorious ./cmd/nexorious

test:
	go test ./...
```

- [ ] **Step 2: Update `cmd/nexorious/main.go`**

Replace the existing `main.go` with:

```go
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
```

**Note:** `parseSlogLevel` also exists in `internal/api/router.go`. In the next step we'll remove the duplicate from `router.go` since slog setup now lives in `main.go`.

- [ ] **Step 3: Remove `parseSlogLevel` from `internal/api/router.go` and move slog setup to accept it externally**

Replace `internal/api/router.go` with:

```go
package api

import (
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"

	"github.com/drzero42/nexorious-go/internal/config"
)

// New creates and configures the Echo instance with all middleware and routes.
// The caller is responsible for configuring the global slog logger before calling New.
func New(cfg *config.Config) *echo.Echo {
	e := echo.New()

	e.Use(middleware.Recover())
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogStatus:   true,
		LogURI:      true,
		LogMethod:   true,
		LogLatency:  true,
		HandleError: true,
		LogValuesFunc: func(c *echo.Context, v middleware.RequestLoggerValues) error {
			if v.Error != nil {
				slog.Error("request", "method", v.Method, "uri", v.URI, "status", v.Status, "latency", v.Latency, "err", v.Error)
			} else {
				slog.Info("request", "method", v.Method, "uri", v.URI, "status", v.Status, "latency", v.Latency)
			}
			return nil
		},
	}))

	registerRoutes(e)

	return e
}

func registerRoutes(e *echo.Echo) {
	e.GET("/health", handleHealth)
}

// handleHealth returns 200 OK with a JSON body.
// GET /health
func handleHealth(c *echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
```

- [ ] **Step 4: Build and run the binary to verify flags work**

```bash
go build -ldflags "-X main.version=v0.0.1-test -X main.commit=abc1234" -o /tmp/nexorious-test ./cmd/nexorious

# Test --version flag
/tmp/nexorious-test --version
```

Expected output: `nexorious v0.0.1-test (abc1234)`

```bash
# Test --help flag (stdlib provides this automatically)
/tmp/nexorious-test --help
```

Expected: usage text listing `--version`, `--config`, `--migrate-only`.

- [ ] **Step 5: Run tests to ensure nothing broke**

```bash
go test ./...
```

Expected: PASS (existing `/health` test should still pass).

- [ ] **Step 6: Commit**

```bash
git add cmd/nexorious/main.go internal/api/router.go Makefile
git commit -m "feat: add CLI flags (--version, --config, --migrate-only) and .env loading"
```

---

### Task 4: Open pgxpool connection on startup

**Files:**
- Modify: `cmd/nexorious/main.go`

This wires a `*pgxpool.Pool` into startup. The pool is closed on shutdown. If the initial ping fails, the binary exits non-zero. No handlers use the pool yet — that comes with the migrator and route handlers in later phases.

- [ ] **Step 1: Update `cmd/nexorious/main.go` to open and ping the pool**

Add the pool setup block between config loading and the HTTP server start. Replace the `main.go` content with:

```go
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

	"github.com/jackc/pgx/v5/pgxpool"
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
	if err := pool.Ping(ctx); err != nil {
		slog.Error("database ping failed", "err", err)
		os.Exit(1)
	}
	slog.Info("database connected")

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
```

- [ ] **Step 2: Build to confirm it compiles**

```bash
go build ./...
```

Expected: exits 0, no errors.

- [ ] **Step 3: Run existing tests**

```bash
go test ./...
```

Expected: PASS. (The router test doesn't require a DB connection.)

- [ ] **Step 4: Manual smoke test against a real database**

If a local PostgreSQL is running (e.g. via `devenv shell`):

```bash
# Using devenv DATABASE_URL (no SECRET_KEY/IGDB vars needed for compile; required for runtime)
export DATABASE_URL="postgresql://localhost/nexorious?sslmode=disable"
export SECRET_KEY="dev-secret"
export IGDB_CLIENT_ID="placeholder"
export IGDB_CLIENT_SECRET="placeholder"

go run ./cmd/nexorious
```

Expected log lines (JSON):
```
{"level":"INFO","msg":"database connected"}
{"level":"INFO","msg":"nexorious starting","addr":":8000","version":"dev","commit":"unknown"}
```

Press Ctrl-C. Expected:
```
{"level":"INFO","msg":"shutdown complete"}
```

If no database is available, verify the failure path:
```bash
export DATABASE_URL="postgresql://localhost:9999/doesnotexist"
go run ./cmd/nexorious
```

Expected: log line `"database ping failed"` then exit non-zero.

- [ ] **Step 5: Commit**

```bash
git add cmd/nexorious/main.go
git commit -m "feat: open and verify pgxpool database connection on startup"
```

---

## Self-Review

### Spec coverage

- ✅ Full Config struct with all fields from spec
- ✅ `DatabaseURL` post-parse hook (percent-encode user/password, match Python `urllib.parse.quote`)
- ✅ `DATABASE_URL` takes priority over individual `DB_*` vars
- ✅ `SECRET_KEY`, `IGDB_CLIENT_ID`, `IGDB_CLIENT_SECRET` marked `required`
- ✅ CLI flags: `--version`/`-v`, `--config`, `--migrate-only`
- ✅ Build-time version injection via `-ldflags` in Makefile
- ✅ `.env` file loading via `godotenv` (no-op if file absent)
- ✅ `pgxpool` opened and pinged on startup; binary exits non-zero on failure
- ✅ Pool closed on shutdown
- ✅ Makefile updated with `VERSION`/`COMMIT`, `frontend`, `sqlc` targets
- ✅ `parseSlogLevel` deduplicated (removed from `router.go`, lives in `main.go`)

### Type consistency

- `Config` fields referenced in `main.go`: `cfg.DatabaseURL`, `cfg.Port`, `cfg.LogLevel` — all defined in Task 2's config struct ✅
- `api.New(cfg)` signature unchanged — still takes `*config.Config` ✅

### Placeholder scan

No TBD/TODO/placeholder patterns in code steps. The `--migrate-only` stub logs and exits cleanly; it's not "TODO implement later" but an intentional placeholder that will be replaced in the migrator phase (Phase 1 step 2).
