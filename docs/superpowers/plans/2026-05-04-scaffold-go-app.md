# Scaffold Go App — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Produce a compilable Go binary (`./nexorious`) that starts up, prints a startup log line, and responds to `GET /health` with `200 OK {"status":"ok"}`.

**Architecture:** `cmd/nexorious/main.go` wires config → Echo server → routes; config lives in `internal/config/config.go`; the single health handler lives inline in `internal/api/router.go`. No database, no workers, no auth — pure skeleton.

**Tech Stack:** Go 1.25, Echo v4, caarlos0/env v11 for config

---

## File Map

| Path | Status | Responsibility |
|---|---|---|
| `go.mod` | Create | Module declaration, Go version |
| `go.sum` | Generated | Dependency checksums (created by `go mod tidy`) |
| `cmd/nexorious/main.go` | Create | Entry point: parse config, create Echo, register routes, start server |
| `internal/config/config.go` | Create | `Config` struct with `caarlos0/env` tags; `Load()` func |
| `internal/api/router.go` | Create | Echo setup, middleware, `GET /health` handler, `New()` func |
| `internal/api/router_test.go` | Create | Integration test for health endpoint |
| `Makefile` | Create | `build`, `test` targets |

---

## Task 1: Go module + dependencies

**Files:**
- Create: `go.mod`

- [ ] **Step 1: Initialise the module**

Run from repo root (inside `devenv shell`):

```bash
go mod init github.com/drzero42/nexorious-go
```

Expected output: creates `go.mod` with `module github.com/drzero42/nexorious-go` and `go 1.25`

- [ ] **Step 2: Add dependencies**

```bash
go get github.com/labstack/echo/v4@latest
go get github.com/labstack/gommon@latest
go get github.com/caarlos0/env/v11@latest
```

- [ ] **Step 3: Tidy**

```bash
go mod tidy
```

Expected: `go.sum` created, no errors.

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: initialise Go module with echo and env deps"
```

---

## Task 2: Config package

**Files:**
- Create: `internal/config/config.go`

- [ ] **Step 1: Write the config struct**

Create `internal/config/config.go`:

```go
package config

import (
	"fmt"

	"github.com/caarlos0/env/v11"
)

// Config holds all application configuration parsed from environment variables.
type Config struct {
	Port     int    `env:"PORT"      envDefault:"8000"`
	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`
	Debug    bool   `env:"DEBUG"     envDefault:"false"`
}

// Load parses Config from environment variables.
// Returns an error if a required variable is missing or unparseable.
func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	return cfg, nil
}
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./internal/config/...
```

Expected: no output (success).

- [ ] **Step 3: Commit**

```bash
git add internal/config/config.go
git commit -m "feat: add config package with PORT, LOG_LEVEL, DEBUG"
```

---

## Task 3: API router + health endpoint (test first)

**Files:**
- Create: `internal/api/router.go`
- Create: `internal/api/router_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/api/router_test.go`:

```go
package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/drzero42/nexorious-go/internal/api"
	"github.com/drzero42/nexorious-go/internal/config"
)

func TestHealthEndpoint(t *testing.T) {
	cfg := &config.Config{Port: 8000, LogLevel: "info", Debug: false}
	e := api.New(cfg)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if body != `{"status":"ok"}`+"\n" {
		t.Fatalf("unexpected body: %q", body)
	}
}
```

- [ ] **Step 2: Run test to confirm it fails (package doesn't exist yet)**

```bash
go test ./internal/api/... -run TestHealthEndpoint -v
```

Expected: compile error — `api` package not found.

- [ ] **Step 3: Implement the router**

Create `internal/api/router.go`:

```go
package api

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/drzero42/nexorious-go/internal/config"
)

// New creates and configures the Echo instance with all middleware and routes.
func New(cfg *config.Config) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	// Middleware
	e.Use(middleware.Recover())
	e.Use(middleware.Logger())

	// Routes
	registerRoutes(e)

	return e
}

func registerRoutes(e *echo.Echo) {
	e.GET("/health", handleHealth)
}

// handleHealth returns 200 OK with a JSON body.
// GET /health
func handleHealth(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
```

- [ ] **Step 4: Run test to confirm it passes**

```bash
go test ./internal/api/... -run TestHealthEndpoint -v
```

Expected output:
```
=== RUN   TestHealthEndpoint
--- PASS: TestHealthEndpoint (0.00s)
PASS
ok  	github.com/drzero42/nexorious-go/internal/api
```

- [ ] **Step 5: Commit**

```bash
git add internal/api/router.go internal/api/router_test.go
git commit -m "feat: add Echo router with GET /health endpoint and test"
```

---

## Task 4: Entry point

**Files:**
- Create: `cmd/nexorious/main.go`

- [ ] **Step 1: Write main.go**

Create `cmd/nexorious/main.go`:

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/drzero42/nexorious-go/internal/api"
	"github.com/drzero42/nexorious-go/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	e := api.New(cfg)

	// Start server in a goroutine so we can listen for shutdown signals.
	addr := fmt.Sprintf(":%d", cfg.Port)
	go func() {
		log.Printf("nexorious starting on %s", addr)
		if err := e.Start(addr); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	// Wait for interrupt/term signal then shut down gracefully.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	log.Println("shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := e.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("shutdown error: %v", err)
	}
	log.Println("shutdown complete")
}
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./cmd/nexorious/...
```

Expected: no output (success).

- [ ] **Step 3: Commit**

```bash
git add cmd/nexorious/main.go
git commit -m "feat: add main entry point with graceful shutdown"
```

---

## Task 5: Makefile

**Files:**
- Create: `Makefile`

- [ ] **Step 1: Write the Makefile**

Create `Makefile`:

```makefile
.PHONY: all build test

all: build

build:
	go build ./cmd/nexorious

test:
	go test ./...
```

- [ ] **Step 2: Verify build target works**

```bash
make build
```

Expected: creates `./nexorious` binary.

- [ ] **Step 3: Verify test target works**

```bash
make test
```

Expected:
```
ok  	github.com/drzero42/nexorious-go/internal/api
```

- [ ] **Step 4: Commit**

```bash
git add Makefile
git commit -m "chore: add Makefile with build and test targets"
```

---

## Task 6: Smoke test the running binary

This task has no code changes — it verifies the binary works end-to-end.

- [ ] **Step 1: Start the binary**

```bash
./nexorious &
SERVER_PID=$!
sleep 0.5
```

Expected log line: `nexorious starting on :8000`

- [ ] **Step 2: Hit the health endpoint**

```bash
curl -s http://localhost:8000/health
```

Expected:
```json
{"status":"ok"}
```

- [ ] **Step 3: Kill the server**

```bash
kill $SERVER_PID
```

Expected log line: `shutdown complete`

- [ ] **Step 4: Clean up the binary**

```bash
rm -f nexorious
```

---

## Task 7: .gitignore

**Files:**
- Modify: `.gitignore`

- [ ] **Step 1: Check existing .gitignore**

```bash
cat .gitignore 2>/dev/null || echo "(no .gitignore yet)"
```

- [ ] **Step 2: Ensure the following entries are present** (append or create as needed)

```gitignore
# Go binary
/nexorious

# Frontend build output (populated by make frontend)
ui/dist/

# devenv
.devenv/
.devenv.flake.nix
```

- [ ] **Step 3: Commit**

```bash
git add .gitignore
git commit -m "chore: add .gitignore for Go binary and frontend dist"
```

---

## Self-Review

**Spec coverage (Phase 1 scaffold slice — "starts up, health endpoint returns 200 OK"):**
- [x] Project scaffolding: `go.mod`, directory structure, Makefile → Tasks 1, 5
- [x] Config (`caarlos0/env`) → Task 2
- [x] Echo HTTP server + middleware stack → Task 3
- [x] Health endpoint `GET /health` → Task 3
- [x] Graceful shutdown → Task 4
- [x] Binary compiles and smoke-tests → Task 6
- [x] .gitignore → Task 7

**Deliberately deferred (later Phase 1 work per spec):**
- Database / pgxpool / migrations
- Migration UI, auth, SPA embedding, static files
- Workers, scheduler, all domain APIs
