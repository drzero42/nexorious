# Backup & Restore Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement full backup/restore with scheduled backups, admin endpoints, setup-time restore, and maintenance mode middleware — completing Phase 3 of the Go port.

**Architecture:** A new `internal/backup` package encapsulates pg_dump/psql tool detection and all backup/restore logic. A `BackupHandler` in `internal/api` exposes admin endpoints. Restore is an orchestration operation that shuts down workers/scheduler, swaps the DB, and re-initializes via callbacks from `main.go`. A `DBHolder` (atomic pointer) in `main.go` keeps the shared DB reference current across restores. Maintenance mode middleware blocks requests during restore.

**Tech Stack:** Go stdlib (`os/exec`, `archive/tar`, `compress/gzip`, `crypto/sha256`), Bun ORM, Echo v5, gocron v2, testcontainers-go.

---

**Reference spec:** `docs/superpowers/specs/2026-05-10-backup-restore-design.md`

**Existing code patterns to follow:**
- Handler structs with `New*Handler` constructors (see `internal/api/export.go`, `internal/api/setup.go`)
- `*echo.Context` (pointer) in Echo v5 handler signatures
- `*bun.DB` for database access, raw SQL via `db.NewRaw()`
- `internal/worker.Pool` for background tasks
- `internal/scheduler.Scheduler` with gocron v2 jobs
- Tests use `testing` stdlib + `testcontainers-go` for real PostgreSQL
- `internal/migrate.AppState` enum for app state machine
- Router gates in `internal/api/router.go` (Gate 1/2/3 pattern)

---

## File Structure

| File | Action | Responsibility |
|------|--------|---------------|
| `internal/backup/tools.go` | **Create** | pg_dump/psql availability detection, exec wrappers for running pg_dump and psql |
| `internal/backup/tools_test.go` | **Create** | Tests for tool detection and exec wrappers |
| `internal/backup/service.go` | **Create** | BackupService: create, list, validate, delete, restore, retention |
| `internal/backup/service_test.go` | **Create** | Integration tests for backup service (testcontainers) |
| `internal/backup/manifest.go` | **Create** | Manifest struct, JSON marshaling, version constants |
| `internal/middleware/maintenance.go` | **Create** | Maintenance mode flag + Echo middleware |
| `internal/middleware/maintenance_test.go` | **Create** | Tests for maintenance middleware |
| `internal/api/backup.go` | **Create** | BackupHandler: admin backup endpoints + setup restore |
| `internal/api/backup_test.go` | **Create** | Handler integration tests |
| `internal/db/models/backup_config.go` | **Create** | BackupConfig Bun model |
| `internal/api/router.go` | **Modify** | Add backup routes, maintenance middleware, update health endpoint, update gates |
| `cmd/nexorious/main.go` | **Modify** | Add DBHolder, ReconnectDB, RebuildPoolAndScheduler callbacks, pass to router |
| `internal/scheduler/scheduler.go` | **Modify** | Add scheduled backup job, config rebuild method |
| `slumber.yaml` | **Modify** | Add backup admin request collection |

---

### Task 1: Manifest Types & Constants

**Files:**
- Create: `internal/backup/manifest.go`

- [ ] **Step 1: Create the manifest package with types**

```go
// internal/backup/manifest.go
package backup

import "time"

const (
	ManifestVersion    = 1
	MaxManifestVersion = 1
)

// Manifest represents the manifest.json inside a backup archive.
type Manifest struct {
	Version          int       `json:"version"`
	CreatedAt        time.Time `json:"created_at"`
	AppVersion       string    `json:"app_version"`
	MigrationVersion string    `json:"migration_version"`
	BackupType       string    `json:"backup_type"`
	DatabaseFile     string    `json:"database_file"`
	DatabaseSizeBytes int64    `json:"database_size_bytes"`
	DatabaseChecksum string    `json:"database_checksum"`
	CoverArtCount    int       `json:"cover_art_count"`
	CoverArtSizeBytes int64    `json:"cover_art_size_bytes"`
	CoverArtChecksum string    `json:"cover_art_checksum"`
	StatsUsers       int       `json:"stats_users"`
	StatsGames       int       `json:"stats_games"`
	StatsTags        int       `json:"stats_tags"`
}

// BackupInfo is the summary returned by ListBackups.
type BackupInfo struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	BackupType string   `json:"backup_type"`
	SizeBytes int64     `json:"size_bytes"`
	Stats     struct {
		Users int `json:"users"`
		Games int `json:"games"`
		Tags  int `json:"tags"`
	} `json:"stats"`
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/backup/...`
Expected: success (no errors)

- [ ] **Step 3: Commit**

```bash
git add internal/backup/manifest.go
git commit -m "feat(backup): add manifest types and constants"
```

---

### Task 2: Tool Detection & Exec Wrappers

**Files:**
- Create: `internal/backup/tools.go`
- Create: `internal/backup/tools_test.go`

- [ ] **Step 1: Write failing tests for tool detection**

```go
// internal/backup/tools_test.go
package backup

import "testing"

func TestCheckTools_SetsAvailability(t *testing.T) {
	CheckTools()
	// On a dev machine with PostgreSQL installed, both should be true.
	// On CI without PostgreSQL client tools, both will be false.
	// We just verify the functions don't panic and return booleans.
	_ = PgDumpAvailable()
	_ = PsqlAvailable()
}

func TestParseDatabaseURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want DBConnParams
	}{
		{
			name: "standard postgres URL",
			url:  "postgres://user:pass@localhost:5432/mydb?sslmode=disable",
			want: DBConnParams{Host: "localhost", Port: "5432", User: "user", Password: "pass", DBName: "mydb"},
		},
		{
			name: "postgresql scheme",
			url:  "postgresql://admin:secret@db.example.com:5433/proddb",
			want: DBConnParams{Host: "db.example.com", Port: "5433", User: "admin", Password: "secret", DBName: "proddb"},
		},
		{
			name: "default port",
			url:  "postgres://user:pass@localhost/mydb",
			want: DBConnParams{Host: "localhost", Port: "5432", User: "user", Password: "pass", DBName: "mydb"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDatabaseURL(tt.url)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Host != tt.want.Host || got.Port != tt.want.Port || got.User != tt.want.User || got.Password != tt.want.Password || got.DBName != tt.want.DBName {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/backup/... -run TestParseDatabaseURL -v`
Expected: FAIL — functions not defined

- [ ] **Step 3: Implement tool detection and URL parsing**

```go
// internal/backup/tools.go
package backup

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"time"
)

var (
	pgDumpAvailable bool
	psqlAvailable   bool
)

// CheckTools probes for pg_dump and psql on the PATH.
// Call once at startup; results are cached for the process lifetime.
func CheckTools() {
	_, err := exec.LookPath("pg_dump")
	pgDumpAvailable = err == nil
	_, err = exec.LookPath("psql")
	psqlAvailable = err == nil
}

// PgDumpAvailable returns true if pg_dump was found at startup.
func PgDumpAvailable() bool { return pgDumpAvailable }

// PsqlAvailable returns true if psql was found at startup.
func PsqlAvailable() bool { return psqlAvailable }

// DBConnParams holds parsed PostgreSQL connection parameters.
type DBConnParams struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
}

// ParseDatabaseURL extracts connection params from a postgres:// URL.
func ParseDatabaseURL(databaseURL string) (DBConnParams, error) {
	u, err := url.Parse(databaseURL)
	if err != nil {
		return DBConnParams{}, fmt.Errorf("parse database URL: %w", err)
	}
	port := u.Port()
	if port == "" {
		port = "5432"
	}
	password, _ := u.User.Password()
	dbName := ""
	if len(u.Path) > 1 {
		dbName = u.Path[1:] // strip leading /
	}
	return DBConnParams{
		Host:     u.Hostname(),
		Port:     port,
		User:     u.User.Username(),
		Password: password,
		DBName:   dbName,
	}, nil
}

// RunPgDump executes pg_dump and writes the output to outputPath.
// Timeout: 300 seconds.
func RunPgDump(conn DBConnParams, outputPath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "pg_dump",
		"--format=plain",
		"--no-owner",
		"--no-acl",
		"--host="+conn.Host,
		"--port="+conn.Port,
		"--username="+conn.User,
		"--dbname="+conn.DBName,
		"--file="+outputPath,
	)
	cmd.Env = append(cmd.Environ(), "PGPASSWORD="+conn.Password)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pg_dump failed: %w\noutput: %s", err, output)
	}
	return nil
}

// RunPsqlFile executes psql with a SQL file as input.
// Timeout: 300 seconds.
func RunPsqlFile(conn DBConnParams, sqlFilePath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "psql",
		"--host="+conn.Host,
		"--port="+conn.Port,
		"--username="+conn.User,
		"--dbname="+conn.DBName,
		"--file="+sqlFilePath,
	)
	cmd.Env = append(cmd.Environ(), "PGPASSWORD="+conn.Password)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("psql restore failed: %w\noutput: %s", err, output)
	}
	return nil
}

// RunPsqlCommand executes a single psql command string.
// Timeout: 60 seconds.
func RunPsqlCommand(conn DBConnParams, command string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "psql",
		"--host="+conn.Host,
		"--port="+conn.Port,
		"--username="+conn.User,
		"--dbname="+conn.DBName,
		"--command="+command,
	)
	cmd.Env = append(cmd.Environ(), "PGPASSWORD="+conn.Password)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("psql command failed: %w\noutput: %s", err, output)
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/backup/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/backup/tools.go internal/backup/tools_test.go
git commit -m "feat(backup): add pg_dump/psql detection and exec wrappers"
```

---

### Task 3: BackupConfig Bun Model

**Files:**
- Create: `internal/db/models/backup_config.go`

- [ ] **Step 1: Create the BackupConfig model**

```go
// internal/db/models/backup_config.go
package models

import (
	"time"

	"github.com/uptrace/bun"
)

// BackupConfig is the singleton (id=1) backup schedule configuration.
type BackupConfig struct {
	bun.BaseModel `bun:"table:backup_config"`

	ID             int       `bun:"id,pk"              json:"id"`
	ScheduleCron   string    `bun:"schedule_cron,notnull" json:"schedule_cron"`
	RetentionMode  string    `bun:"retention_mode,notnull" json:"retention_mode"`
	RetentionValue int       `bun:"retention_value,notnull" json:"retention_value"`
	CreatedAt      time.Time `bun:"created_at,notnull" json:"created_at"`
	UpdatedAt      time.Time `bun:"updated_at,notnull" json:"updated_at"`
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/db/models/...`
Expected: success

- [ ] **Step 3: Commit**

```bash
git add internal/db/models/backup_config.go
git commit -m "feat(backup): add BackupConfig Bun model"
```

---

### Task 4: Maintenance Mode Middleware

**Files:**
- Create: `internal/middleware/maintenance.go`
- Create: `internal/middleware/maintenance_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/middleware/maintenance_test.go
package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
)

func TestMaintenanceMode_DefaultOff(t *testing.T) {
	if IsMaintenanceMode() {
		t.Error("maintenance mode should be off by default")
	}
}

func TestMaintenanceMode_Toggle(t *testing.T) {
	SetMaintenanceMode(true)
	if !IsMaintenanceMode() {
		t.Error("expected maintenance mode on")
	}
	SetMaintenanceMode(false)
	if IsMaintenanceMode() {
		t.Error("expected maintenance mode off")
	}
}

func TestMaintenanceMiddleware_BlocksWhenActive(t *testing.T) {
	SetMaintenanceMode(true)
	defer SetMaintenanceMode(false)

	e := echo.New()
	e.Use(MaintenanceMiddleware())
	e.GET("/api/games", func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/games", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rec.Code)
	}
}

func TestMaintenanceMiddleware_AllowsHealth(t *testing.T) {
	SetMaintenanceMode(true)
	defer SetMaintenanceMode(false)

	e := echo.New()
	e.Use(MaintenanceMiddleware())
	e.GET("/health", func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestMaintenanceMiddleware_AllowsBackupEndpoints(t *testing.T) {
	SetMaintenanceMode(true)
	defer SetMaintenanceMode(false)

	e := echo.New()
	e.Use(MaintenanceMiddleware())
	e.GET("/api/admin/backups", func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/admin/backups", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestMaintenanceMiddleware_AllowsAuthMe(t *testing.T) {
	SetMaintenanceMode(true)
	defer SetMaintenanceMode(false)

	e := echo.New()
	e.Use(MaintenanceMiddleware())
	e.GET("/api/auth/me", func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestMaintenanceMiddleware_PassesThroughWhenInactive(t *testing.T) {
	SetMaintenanceMode(false)

	e := echo.New()
	e.Use(MaintenanceMiddleware())
	e.GET("/api/games", func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/games", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/middleware/... -v`
Expected: FAIL — package doesn't exist yet

- [ ] **Step 3: Implement maintenance mode**

```go
// internal/middleware/maintenance.go
package middleware

import (
	"net/http"
	"strings"
	"sync"

	"github.com/labstack/echo/v5"
)

var (
	mu              sync.RWMutex
	maintenanceMode bool
)

// SetMaintenanceMode enables or disables maintenance mode.
func SetMaintenanceMode(enabled bool) {
	mu.Lock()
	defer mu.Unlock()
	maintenanceMode = enabled
}

// IsMaintenanceMode returns whether maintenance mode is active.
func IsMaintenanceMode() bool {
	mu.RLock()
	defer mu.RUnlock()
	return maintenanceMode
}

// MaintenanceMiddleware returns Echo middleware that blocks requests
// during maintenance mode, except for allowed paths.
func MaintenanceMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if !IsMaintenanceMode() {
				return next(c)
			}

			path := c.Request().URL.Path

			// Allowed during maintenance:
			// - GET /health
			// - GET|POST|DELETE /api/admin/backups/*
			// - GET /api/auth/me
			if path == "/health" ||
				strings.HasPrefix(path, "/api/admin/backups") ||
				path == "/api/auth/me" {
				return next(c)
			}

			return c.JSON(http.StatusServiceUnavailable, map[string]any{
				"error":            "Service Unavailable",
				"detail":           "Restore in progress",
				"maintenance_mode": true,
			})
		}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/middleware/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/middleware/maintenance.go internal/middleware/maintenance_test.go
git commit -m "feat(backup): add maintenance mode middleware"
```

---

### Task 5: Backup Service — Create & List

**Files:**
- Create: `internal/backup/service.go`
- Create: `internal/backup/service_test.go`

This task implements `CreateBackup`, `ListBackups`, `GetBackupPath`, `DeleteBackup`, and the archive creation/extraction logic. The restore logic is in Task 6.

- [ ] **Step 1: Write failing test for CreateBackup**

The test needs a real PostgreSQL via testcontainers. Create the test file with a helper:

```go
// internal/backup/service_test.go
package backup_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"

	"github.com/drzero42/nexorious-go/internal/backup"
)

func setupTestDB(t *testing.T) (*bun.DB, string) {
	t.Helper()
	ctx := context.Background()

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "postgres:16-alpine",
			ExposedPorts: []string{"5432/tcp"},
			Env: map[string]string{
				"POSTGRES_USER":     "test",
				"POSTGRES_PASSWORD": "test",
				"POSTGRES_DB":       "testdb",
			},
			WaitingFor: wait.ForListeningPort("5432/tcp").WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("start container: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	host, _ := container.Host(ctx)
	port, _ := container.MappedPort(ctx, "5432")
	dsn := "postgres://test:test@" + host + ":" + port.Port() + "/testdb?sslmode=disable"

	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	db := bun.NewDB(sqldb, pgdialect.New())
	t.Cleanup(func() { _ = db.Close() })

	// Create minimal schema for stats queries
	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS users (id TEXT PRIMARY KEY);
		CREATE TABLE IF NOT EXISTS games (id SERIAL PRIMARY KEY);
		CREATE TABLE IF NOT EXISTS tags (id TEXT PRIMARY KEY);
		CREATE TABLE IF NOT EXISTS schema_migrations (version BIGINT NOT NULL);
		INSERT INTO schema_migrations (version) VALUES (20260503000001);
	`)
	if err != nil {
		t.Fatalf("create schema: %v", err)
	}

	return db, dsn
}

func TestCreateBackup(t *testing.T) {
	if !backup.PgDumpAvailable() {
		t.Skip("pg_dump not available")
	}

	db, dsn := setupTestDB(t)
	backupDir := t.TempDir()
	storageDir := t.TempDir()

	// Create a cover_art directory with a test file
	coverDir := filepath.Join(storageDir, "cover_art")
	if err := os.MkdirAll(coverDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(coverDir, "test.jpg"), []byte("fake image"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := backup.NewService(db, dsn, backupDir, storageDir, "0.1.0")

	id, err := svc.CreateBackup("manual")
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}

	if id == "" {
		t.Fatal("expected non-empty backup ID")
	}

	// Verify archive exists
	archivePath := svc.GetBackupPath(id)
	if _, err := os.Stat(archivePath); err != nil {
		t.Fatalf("archive not found at %s: %v", archivePath, err)
	}
}

func TestListBackups(t *testing.T) {
	if !backup.PgDumpAvailable() {
		t.Skip("pg_dump not available")
	}

	db, dsn := setupTestDB(t)
	backupDir := t.TempDir()
	storageDir := t.TempDir()
	os.MkdirAll(filepath.Join(storageDir, "cover_art"), 0o755)

	svc := backup.NewService(db, dsn, backupDir, storageDir, "0.1.0")

	_, err := svc.CreateBackup("manual")
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}

	backups, err := svc.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(backups) != 1 {
		t.Fatalf("expected 1 backup, got %d", len(backups))
	}
	if backups[0].BackupType != "manual" {
		t.Errorf("expected backup_type 'manual', got %q", backups[0].BackupType)
	}
}

func TestDeleteBackup(t *testing.T) {
	if !backup.PgDumpAvailable() {
		t.Skip("pg_dump not available")
	}

	db, dsn := setupTestDB(t)
	backupDir := t.TempDir()
	storageDir := t.TempDir()
	os.MkdirAll(filepath.Join(storageDir, "cover_art"), 0o755)

	svc := backup.NewService(db, dsn, backupDir, storageDir, "0.1.0")

	id, err := svc.CreateBackup("manual")
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}

	if err := svc.DeleteBackup(id); err != nil {
		t.Fatalf("DeleteBackup: %v", err)
	}

	if _, err := os.Stat(svc.GetBackupPath(id)); !os.IsNotExist(err) {
		t.Error("archive should have been deleted")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/backup/... -run TestCreateBackup -v`
Expected: FAIL — `NewService` not defined

- [ ] **Step 3: Implement the backup service (create, list, delete)**

This is the largest implementation step. Create `internal/backup/service.go`:

```go
// internal/backup/service.go
package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/uptrace/bun"
)

// Service handles backup create/list/delete/validate/restore operations.
type Service struct {
	db          *bun.DB
	databaseURL string
	backupPath  string
	storagePath string
	appVersion  string
	mu          sync.Mutex
}

// NewService creates a new backup service.
func NewService(db *bun.DB, databaseURL, backupPath, storagePath, appVersion string) *Service {
	return &Service{
		db:          db,
		databaseURL: databaseURL,
		backupPath:  backupPath,
		storagePath: storagePath,
		appVersion:  appVersion,
	}
}

// SetDB updates the database connection (used after restore reconnect).
func (s *Service) SetDB(db *bun.DB) {
	s.db = db
}

// CreateBackup creates a backup archive and returns the backup ID.
// backupType is "manual", "scheduled", or "pre_restore".
func (s *Service) CreateBackup(backupType string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	conn, err := ParseDatabaseURL(s.databaseURL)
	if err != nil {
		return "", fmt.Errorf("create backup: %w", err)
	}

	// 1. Generate backup ID
	id := fmt.Sprintf("backup-%s", time.Now().UTC().Format("20060102-150405"))

	// 2. Create temp directory
	tmpDir, err := os.MkdirTemp("", "nexorious-backup-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	backupDir := filepath.Join(tmpDir, id)
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", fmt.Errorf("create backup dir: %w", err)
	}

	// 3. Run pg_dump
	dbSQLPath := filepath.Join(backupDir, "database.sql")
	if err := RunPgDump(conn, dbSQLPath); err != nil {
		return "", fmt.Errorf("pg_dump: %w", err)
	}

	// 4. Copy cover_art directory
	coverArtSrc := filepath.Join(s.storagePath, "cover_art")
	coverArtDst := filepath.Join(backupDir, "cover_art")
	coverArtCount, coverArtSize, err := copyDir(coverArtSrc, coverArtDst)
	if err != nil {
		return "", fmt.Errorf("copy cover art: %w", err)
	}

	// 5. Query stats
	ctx := context.Background()
	var statsUsers, statsGames, statsTags int
	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&statsUsers)
	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM games").Scan(&statsGames)
	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tags").Scan(&statsTags)

	// 6. Read migration version
	var migrationVersion int64
	_ = s.db.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&migrationVersion)

	// 7. Compute checksums and sizes
	dbChecksum, dbSize := checksumFile(dbSQLPath)
	coverArtChecksum := checksumDir(coverArtDst)

	// 8. Write manifest
	manifest := Manifest{
		Version:           ManifestVersion,
		CreatedAt:         time.Now().UTC(),
		AppVersion:        s.appVersion,
		MigrationVersion:  fmt.Sprintf("%d", migrationVersion),
		BackupType:        backupType,
		DatabaseFile:      "database.sql",
		DatabaseSizeBytes: dbSize,
		DatabaseChecksum:  "sha256:" + dbChecksum,
		CoverArtCount:     coverArtCount,
		CoverArtSizeBytes: coverArtSize,
		CoverArtChecksum:  "sha256:" + coverArtChecksum,
		StatsUsers:        statsUsers,
		StatsGames:        statsGames,
		StatsTags:         statsTags,
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(backupDir, "manifest.json"), manifestBytes, 0o644); err != nil {
		return "", fmt.Errorf("write manifest: %w", err)
	}

	// 9. Create .tar.gz archive
	if err := os.MkdirAll(s.backupPath, 0o755); err != nil {
		return "", fmt.Errorf("create backup path: %w", err)
	}
	archivePath := filepath.Join(s.backupPath, id+".tar.gz")
	if err := createTarGz(archivePath, tmpDir, id); err != nil {
		return "", fmt.Errorf("create archive: %w", err)
	}

	slog.Info("backup created", "id", id, "type", backupType, "path", archivePath)
	return id, nil
}

// ListBackups returns all backups sorted by created_at descending.
func (s *Service) ListBackups() ([]BackupInfo, error) {
	pattern := filepath.Join(s.backupPath, "backup-*.tar.gz")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob backups: %w", err)
	}

	var backups []BackupInfo
	for _, archivePath := range matches {
		manifest, err := readManifestFromArchive(archivePath)
		if err != nil {
			slog.Warn("skipping invalid backup archive", "path", archivePath, "err", err)
			continue
		}
		info, _ := os.Stat(archivePath)
		bi := BackupInfo{
			ID:         strings.TrimSuffix(filepath.Base(archivePath), ".tar.gz"),
			CreatedAt:  manifest.CreatedAt,
			BackupType: manifest.BackupType,
			SizeBytes:  info.Size(),
		}
		bi.Stats.Users = manifest.StatsUsers
		bi.Stats.Games = manifest.StatsGames
		bi.Stats.Tags = manifest.StatsTags
		backups = append(backups, bi)
	}

	sort.Slice(backups, func(i, j int) bool {
		return backups[i].CreatedAt.After(backups[j].CreatedAt)
	})

	return backups, nil
}

// GetBackupPath returns the full filesystem path for a backup archive.
func (s *Service) GetBackupPath(backupID string) string {
	return filepath.Join(s.backupPath, backupID+".tar.gz")
}

// DeleteBackup removes a backup archive file.
func (s *Service) DeleteBackup(backupID string) error {
	path := s.GetBackupPath(backupID)
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("delete backup %s: %w", backupID, err)
	}
	slog.Info("backup deleted", "id", backupID)
	return nil
}

// ValidateArchive opens an archive, reads the manifest, checks database.sql exists,
// and optionally verifies SHA-256 checksums. Also validates manifest version and
// migration version against the running binary.
func (s *Service) ValidateArchive(archivePath string, verifyChecksums bool, maxMigrationVersion string) (*Manifest, error) {
	manifest, err := readManifestFromArchive(archivePath)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	if manifest.Version > MaxManifestVersion {
		return nil, fmt.Errorf("unknown manifest version %d (max supported: %d)", manifest.Version, MaxManifestVersion)
	}

	if maxMigrationVersion != "" && manifest.MigrationVersion > maxMigrationVersion {
		return nil, fmt.Errorf(
			"Backup was created by a newer version of Nexorious (migration %s). This binary only supports up to migration %s. Upgrade before restoring.",
			manifest.MigrationVersion, maxMigrationVersion,
		)
	}

	if verifyChecksums {
		if err := verifyArchiveChecksums(archivePath, manifest); err != nil {
			return nil, fmt.Errorf("checksum verification failed: %w", err)
		}
	}

	return manifest, nil
}

// ApplyRetention deletes backups exceeding the retention policy.
func (s *Service) ApplyRetention(retentionMode string, retentionValue int) error {
	backups, err := s.ListBackups()
	if err != nil {
		return err
	}

	now := time.Now()

	// Always clean pre-restore backups older than 7 days
	for _, b := range backups {
		if b.BackupType == "pre_restore" && now.Sub(b.CreatedAt) > 7*24*time.Hour {
			if err := s.DeleteBackup(b.ID); err != nil {
				slog.Warn("retention: failed to delete old pre-restore backup", "id", b.ID, "err", err)
			}
		}
	}

	// Re-list after pre-restore cleanup
	backups, err = s.ListBackups()
	if err != nil {
		return err
	}

	switch retentionMode {
	case "days":
		cutoff := now.AddDate(0, 0, -retentionValue)
		for _, b := range backups {
			if b.BackupType != "pre_restore" && b.CreatedAt.Before(cutoff) {
				if err := s.DeleteBackup(b.ID); err != nil {
					slog.Warn("retention: failed to delete old backup", "id", b.ID, "err", err)
				}
			}
		}
	case "count":
		// backups is already sorted newest-first
		nonPreRestore := 0
		for _, b := range backups {
			if b.BackupType == "pre_restore" {
				continue
			}
			nonPreRestore++
			if nonPreRestore > retentionValue {
				if err := s.DeleteBackup(b.ID); err != nil {
					slog.Warn("retention: failed to delete excess backup", "id", b.ID, "err", err)
				}
			}
		}
	}

	return nil
}

// --- Helper functions ---

func checksumFile(path string) (string, int64) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0
	}
	defer f.Close()

	h := sha256.New()
	size, _ := io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), size
}

func checksumDir(dir string) string {
	h := sha256.New()
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, _ = io.Copy(h, f)
		return nil
	})
	return hex.EncodeToString(h.Sum(nil))
}

func copyDir(src, dst string) (fileCount int, totalSize int64, err error) {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return 0, 0, err
	}

	// If source doesn't exist, return empty dir
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return 0, 0, nil
	}

	err = filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relPath, _ := filepath.Rel(src, path)
		dstPath := filepath.Join(dst, relPath)

		if d.IsDir() {
			return os.MkdirAll(dstPath, 0o755)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fileCount++
		totalSize += int64(len(data))
		return os.WriteFile(dstPath, data, 0o644)
	})
	return fileCount, totalSize, err
}

func createTarGz(archivePath, baseDir, dirName string) error {
	f, err := os.Create(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	srcDir := filepath.Join(baseDir, dirName)
	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relPath, _ := filepath.Rel(baseDir, path)

		info, err := d.Info()
		if err != nil {
			return err
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = relPath

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(tw, file)
		return err
	})
}

func readManifestFromArchive(archivePath string) (*Manifest, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("manifest.json not found in archive")
		}
		if err != nil {
			return nil, err
		}
		if filepath.Base(hdr.Name) == "manifest.json" {
			var m Manifest
			if err := json.NewDecoder(tr).Decode(&m); err != nil {
				return nil, fmt.Errorf("decode manifest: %w", err)
			}
			return &m, nil
		}
	}
}

func verifyArchiveChecksums(archivePath string, manifest *Manifest) error {
	// Extract to temp, verify checksums, clean up
	tmpDir, err := os.MkdirTemp("", "nexorious-verify-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	if err := extractTarGz(archivePath, tmpDir); err != nil {
		return fmt.Errorf("extract for verification: %w", err)
	}

	// Find the extracted backup directory
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return fmt.Errorf("empty archive")
	}
	backupDir := filepath.Join(tmpDir, entries[0].Name())

	// Verify database.sql checksum
	dbChecksum, _ := checksumFile(filepath.Join(backupDir, "database.sql"))
	expectedDB := strings.TrimPrefix(manifest.DatabaseChecksum, "sha256:")
	if dbChecksum != expectedDB {
		return fmt.Errorf("database.sql checksum mismatch: got %s, expected %s", dbChecksum, expectedDB)
	}

	// Verify cover_art checksum
	coverChecksum := checksumDir(filepath.Join(backupDir, "cover_art"))
	expectedCover := strings.TrimPrefix(manifest.CoverArtChecksum, "sha256:")
	if coverChecksum != expectedCover {
		return fmt.Errorf("cover_art checksum mismatch: got %s, expected %s", coverChecksum, expectedCover)
	}

	return nil
}

func extractTarGz(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		target := filepath.Join(destDir, hdr.Name)

		// Prevent path traversal
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destDir)) {
			return fmt.Errorf("invalid tar path: %s", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			outFile, err := os.Create(target)
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()
		}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/backup/... -v -timeout 120s`
Expected: PASS (tests that need pg_dump will skip if unavailable)

- [ ] **Step 5: Commit**

```bash
git add internal/backup/service.go internal/backup/service_test.go
git commit -m "feat(backup): implement backup service — create, list, delete, validate, retention"
```

---

### Task 6: Backup Service — Restore Logic

**Files:**
- Modify: `internal/backup/service.go` — add `RestoreBackup`, `RestoreFromUpload`

- [ ] **Step 1: Write failing test for RestoreBackup**

Add to `internal/backup/service_test.go`:

```go
func TestRestoreBackup(t *testing.T) {
	if !backup.PgDumpAvailable() || !backup.PsqlAvailable() {
		t.Skip("pg_dump or psql not available")
	}

	db, dsn := setupTestDB(t)
	backupDir := t.TempDir()
	storageDir := t.TempDir()
	os.MkdirAll(filepath.Join(storageDir, "cover_art"), 0o755)

	svc := backup.NewService(db, dsn, backupDir, storageDir, "0.1.0")

	// Create a backup
	id, err := svc.CreateBackup("manual")
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}

	// Insert some data that will be wiped by restore
	ctx := context.Background()
	_, _ = db.ExecContext(ctx, "INSERT INTO users (id) VALUES ('will-be-restored')")

	// Restore (skip pre-restore for simplicity in test)
	restoreOpts := backup.RestoreOpts{
		SkipPreRestore:  true,
		SetMaintenance:  func(bool) {},
		ShutdownPool:    func() {},
		StopScheduler:   func() {},
		CloseDB:         func() error { return nil },
		ReconnectDB:     func() (*bun.DB, error) { return db, nil },
		RebuildServices: func(*bun.DB) error { return nil },
		ReinitMigrator:  func() error { return nil },
		SetAppState:     func(s string) {},
		MaxMigration:    "99999999999999",
	}

	if err := svc.RestoreBackup(id, restoreOpts); err != nil {
		t.Fatalf("RestoreBackup: %v", err)
	}

	// Verify the data was restored (the 'will-be-restored' row should be gone
	// since the backup was taken before it was inserted)
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE id = 'will-be-restored'").Scan(&count)
	if err != nil {
		t.Fatalf("query after restore: %v", err)
	}
	if count != 0 {
		t.Error("expected 'will-be-restored' row to be absent after restore")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/backup/... -run TestRestoreBackup -v -timeout 120s`
Expected: FAIL — `RestoreOpts` and `RestoreBackup` not defined

- [ ] **Step 3: Implement RestoreBackup and RestoreFromUpload**

Add to `internal/backup/service.go`:

```go
// RestoreOpts holds callbacks for restore orchestration.
// These are provided by main.go since restore needs to coordinate
// across the entire application.
type RestoreOpts struct {
	SkipPreRestore  bool
	SetMaintenance  func(bool)
	ShutdownPool    func()
	StopScheduler   func()
	CloseDB         func() error
	ReconnectDB     func() (*bun.DB, error)
	RebuildServices func(db *bun.DB) error
	ReinitMigrator  func() error
	SetAppState     func(state string)
	MaxMigration    string
}

// RestoreBackup restores from a stored backup archive.
func (s *Service) RestoreBackup(backupID string, opts RestoreOpts) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	archivePath := s.GetBackupPath(backupID)
	return s.doRestore(archivePath, backupID, opts)
}

// RestoreFromUpload validates an uploaded archive, moves it to the backup dir,
// then restores it.
func (s *Service) RestoreFromUpload(uploadedPath string, opts RestoreOpts) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Validate
	manifest, err := s.ValidateArchive(uploadedPath, true, opts.MaxMigration)
	if err != nil {
		return fmt.Errorf("validate uploaded archive: %w", err)
	}

	// Generate an ID and move to backup dir
	id := fmt.Sprintf("backup-%s", time.Now().UTC().Format("20060102-150405"))
	destPath := filepath.Join(s.backupPath, id+".tar.gz")
	if err := os.MkdirAll(s.backupPath, 0o755); err != nil {
		return fmt.Errorf("create backup dir: %w", err)
	}
	if err := os.Rename(uploadedPath, destPath); err != nil {
		// Rename may fail across filesystems; fall back to copy
		if err := copyFile(uploadedPath, destPath); err != nil {
			return fmt.Errorf("move uploaded archive: %w", err)
		}
		os.Remove(uploadedPath)
	}
	_ = manifest // used for validation only

	return s.doRestore(destPath, id, opts)
}

func (s *Service) doRestore(archivePath, backupID string, opts RestoreOpts) error {
	conn, err := ParseDatabaseURL(s.databaseURL)
	if err != nil {
		return fmt.Errorf("restore: parse DB URL: %w", err)
	}

	// Validate archive
	_, err = s.ValidateArchive(archivePath, false, opts.MaxMigration)
	if err != nil {
		return fmt.Errorf("restore: validate: %w", err)
	}

	// 1. Set maintenance mode
	opts.SetMaintenance(true)

	// 2. Shut down worker pool and scheduler
	opts.ShutdownPool()
	opts.StopScheduler()

	// 3. Create pre-restore backup (unless skipped)
	var preRestoreID string
	if !opts.SkipPreRestore && PgDumpAvailable() {
		s.mu.Unlock() // unlock for CreateBackup (it takes the lock)
		pid, err := s.CreateBackup("pre_restore")
		s.mu.Lock()
		if err != nil {
			slog.Error("restore: failed to create pre-restore backup", "err", err)
			// Continue anyway — better to attempt the restore
		} else {
			preRestoreID = pid
		}
	}

	// 4. Close DB pool
	if err := opts.CloseDB(); err != nil {
		slog.Error("restore: close DB", "err", err)
	}

	// Extract archive to temp
	tmpDir, err := os.MkdirTemp("", "nexorious-restore-*")
	if err != nil {
		return s.handleRestoreFailure(fmt.Errorf("create temp dir: %w", err), preRestoreID, conn, opts)
	}
	defer os.RemoveAll(tmpDir)

	if err := extractTarGz(archivePath, tmpDir); err != nil {
		return s.handleRestoreFailure(fmt.Errorf("extract archive: %w", err), preRestoreID, conn, opts)
	}

	// Find extracted backup directory
	entries, err := os.ReadDir(tmpDir)
	if err != nil || len(entries) == 0 {
		return s.handleRestoreFailure(fmt.Errorf("empty or unreadable archive"), preRestoreID, conn, opts)
	}
	extractedDir := filepath.Join(tmpDir, entries[0].Name())

	// 5. Terminate other DB connections
	terminateCmd := fmt.Sprintf(
		"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '%s' AND pid <> pg_backend_pid();",
		conn.DBName,
	)
	if err := RunPsqlCommand(conn, terminateCmd); err != nil {
		return s.handleRestoreFailure(fmt.Errorf("terminate connections: %w", err), preRestoreID, conn, opts)
	}

	// 6. Drop and recreate schema
	if err := RunPsqlCommand(conn, "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"); err != nil {
		return s.handleRestoreFailure(fmt.Errorf("drop/recreate schema: %w", err), preRestoreID, conn, opts)
	}

	// 7. Restore database
	sqlFile := filepath.Join(extractedDir, "database.sql")
	if err := RunPsqlFile(conn, sqlFile); err != nil {
		return s.handleRestoreFailure(fmt.Errorf("psql restore: %w", err), preRestoreID, conn, opts)
	}

	// 8. Restore cover art
	coverArtSrc := filepath.Join(extractedDir, "cover_art")
	coverArtDst := filepath.Join(s.storagePath, "cover_art")
	if err := os.RemoveAll(coverArtDst); err != nil {
		slog.Warn("restore: failed to remove old cover_art", "err", err)
	}
	if _, err := os.Stat(coverArtSrc); err == nil {
		if _, _, err := copyDir(coverArtSrc, coverArtDst); err != nil {
			return s.handleRestoreFailure(fmt.Errorf("restore cover art: %w", err), preRestoreID, conn, opts)
		}
	}

	// 9. Reconnect DB
	newDB, err := opts.ReconnectDB()
	if err != nil {
		return s.handleRestoreFailure(fmt.Errorf("reconnect DB: %w", err), preRestoreID, conn, opts)
	}
	s.db = newDB

	// 10. Rebuild pool and scheduler
	if err := opts.RebuildServices(newDB); err != nil {
		slog.Error("restore: rebuild services", "err", err)
	}

	// 11. Re-init migrator
	if err := opts.ReinitMigrator(); err != nil {
		slog.Error("restore: reinit migrator", "err", err)
	}

	// 12. Clear maintenance mode
	opts.SetMaintenance(false)

	slog.Info("restore completed", "backup_id", backupID)
	return nil
}

func (s *Service) handleRestoreFailure(originalErr error, preRestoreID string, conn DBConnParams, opts RestoreOpts) error {
	slog.Error("restore failed", "err", originalErr)

	if preRestoreID == "" {
		// No pre-restore backup — unrecoverable
		slog.Error("restore failed with no pre-restore backup — database may be inconsistent. Manual intervention required.",
			"err", originalErr)
		opts.SetAppState("db_unavailable")
		// Leave maintenance mode ON
		return originalErr
	}

	// Attempt rollback via pre-restore backup
	slog.Warn("attempting rollback to pre-restore backup", "pre_restore_id", preRestoreID)

	// We need to attempt the raw restore steps (5-8) again with the pre-restore archive
	archivePath := s.GetBackupPath(preRestoreID)
	tmpDir, err := os.MkdirTemp("", "nexorious-rollback-*")
	if err != nil {
		slog.Error("rollback failed: create temp dir", "err", err, "original_err", originalErr)
		opts.SetAppState("db_unavailable")
		return fmt.Errorf("restore failed AND rollback failed. Original: %w. Rollback: %v", originalErr, err)
	}
	defer os.RemoveAll(tmpDir)

	if err := extractTarGz(archivePath, tmpDir); err != nil {
		slog.Error("rollback failed: extract archive", "err", err, "original_err", originalErr)
		opts.SetAppState("db_unavailable")
		return fmt.Errorf("restore failed AND rollback failed. Original: %w. Rollback: %v", originalErr, err)
	}

	entries, _ := os.ReadDir(tmpDir)
	if len(entries) == 0 {
		opts.SetAppState("db_unavailable")
		return fmt.Errorf("restore failed AND rollback failed (empty archive). Original: %w", originalErr)
	}
	extractedDir := filepath.Join(tmpDir, entries[0].Name())

	// Re-run steps 5-8 with pre-restore backup
	terminateCmd := fmt.Sprintf(
		"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '%s' AND pid <> pg_backend_pid();",
		conn.DBName,
	)
	_ = RunPsqlCommand(conn, terminateCmd)
	_ = RunPsqlCommand(conn, "DROP SCHEMA public CASCADE; CREATE SCHEMA public;")

	sqlFile := filepath.Join(extractedDir, "database.sql")
	if err := RunPsqlFile(conn, sqlFile); err != nil {
		slog.Error("FATAL: rollback restore also failed", "err", err, "original_err", originalErr,
			"pre_restore_path", archivePath)
		opts.SetAppState("db_unavailable")
		return fmt.Errorf("restore failed AND rollback failed. Original: %w. Rollback: %v. Pre-restore backup at: %s",
			originalErr, err, archivePath)
	}

	// Rollback succeeded — restore cover art from pre-restore
	coverArtSrc := filepath.Join(extractedDir, "cover_art")
	coverArtDst := filepath.Join(s.storagePath, "cover_art")
	_ = os.RemoveAll(coverArtDst)
	if _, err := os.Stat(coverArtSrc); err == nil {
		_, _, _ = copyDir(coverArtSrc, coverArtDst)
	}

	// Reconnect and rebuild
	newDB, err := opts.ReconnectDB()
	if err != nil {
		slog.Error("rollback: reconnect DB failed", "err", err)
		opts.SetAppState("db_unavailable")
		return fmt.Errorf("restore failed, rollback DB restored but reconnect failed. Original: %w", originalErr)
	}
	s.db = newDB
	_ = opts.RebuildServices(newDB)
	_ = opts.ReinitMigrator()

	opts.SetMaintenance(false)
	slog.Warn("restore failed but successfully rolled back", "original_err", originalErr, "pre_restore_id", preRestoreID)
	return fmt.Errorf("restore failed: %w (rolled back to pre-restore backup)", originalErr)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/backup/... -v -timeout 120s`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/backup/service.go internal/backup/service_test.go
git commit -m "feat(backup): implement restore with rollback on failure"
```

---

### Task 7: DBHolder & Main.go Wiring

**Files:**
- Modify: `cmd/nexorious/main.go`

This task refactors `main.go` to use a `DBHolder` (atomic pointer), provide `ReconnectDB` and `RebuildPoolAndScheduler` callbacks, initialize `backup.Service`, and call `CheckTools()` at startup.

- [ ] **Step 1: Add DBHolder and refactor main.go**

Add above `main()`:

```go
import (
	// ... existing imports ...
	"sync/atomic"

	"github.com/drzero42/nexorious-go/internal/backup"
)

// DBHolder wraps an atomic pointer to *bun.DB so all handlers
// see the current connection even after a restore reconnect.
type DBHolder struct {
	p atomic.Pointer[bun.DB]
}

func (h *DBHolder) Get() *bun.DB { return h.p.Load() }
func (h *DBHolder) Set(db *bun.DB) { h.p.Store(db) }
```

Inside `main()`, after creating `db`:

```go
dbHolder := &DBHolder{}
dbHolder.Set(db)

// Tool detection for backup/restore
backup.CheckTools()
if backup.PgDumpAvailable() {
	slog.Info("pg_dump available — backups enabled")
} else {
	slog.Warn("pg_dump not found — backup creation disabled")
}
if backup.PsqlAvailable() {
	slog.Info("psql available — restore enabled")
} else {
	slog.Warn("psql not found — restore disabled")
}

// Backup service
backupSvc := backup.NewService(db, resolvedDatabaseURL, cfg.BackupPath, cfg.StoragePath, version)

// ReconnectDB callback
reconnectDB := func() (*bun.DB, error) {
	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(resolvedDatabaseURL)))
	newDB := bun.NewDB(sqldb, pgdialect.New())
	newDB.SetMaxOpenConns(25)
	newDB.SetMaxIdleConns(5)
	dbHolder.Set(newDB)
	backupSvc.SetDB(newDB)
	return newDB, nil
}
```

Update the `api.New` call to pass `backupSvc` (will require router signature change in Task 9).

- [ ] **Step 2: Verify it compiles**

Run: `go build ./cmd/nexorious/...`
Expected: success (may need router signature updates first — coordinate with Task 9)

- [ ] **Step 3: Commit**

```bash
git add cmd/nexorious/main.go
git commit -m "feat(backup): add DBHolder, tool detection, and backup service wiring in main"
```

---

### Task 8: Backup Handler — Admin Endpoints

**Files:**
- Create: `internal/api/backup.go`

- [ ] **Step 1: Write failing test for list endpoint**

Create `internal/api/backup_test.go` with at minimum a test that the handler struct exists and can be constructed:

```go
// internal/api/backup_test.go
package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/drzero42/nexorious-go/internal/backup"
)

func TestHandleListBackups_EmptyList(t *testing.T) {
	backupDir := t.TempDir()
	svc := backup.NewService(nil, "", backupDir, "", "0.1.0")
	h := NewBackupHandler(svc, nil)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/backups", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.HandleListBackups(&c); err != nil {
		t.Fatalf("HandleListBackups: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/... -run TestHandleListBackups -v`
Expected: FAIL — `NewBackupHandler` not defined

- [ ] **Step 3: Implement BackupHandler**

```go
// internal/api/backup.go
package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious-go/internal/backup"
	"github.com/drzero42/nexorious-go/internal/db/models"
)

// RestoreCallbacks holds the callbacks needed for restore orchestration.
// Provided by main.go at wiring time.
type RestoreCallbacks struct {
	SetMaintenance  func(bool)
	ShutdownPool    func()
	StopScheduler   func()
	CloseDB         func() error
	ReconnectDB     func() (*bun.DB, error)
	RebuildServices func(db *bun.DB) error
	ReinitMigrator  func() error
	SetAppState     func(state string)
	MaxMigration    string
}

// BackupHandler handles admin backup endpoints.
type BackupHandler struct {
	svc       *backup.Service
	callbacks *RestoreCallbacks
}

// NewBackupHandler returns a new BackupHandler.
func NewBackupHandler(svc *backup.Service, callbacks *RestoreCallbacks) *BackupHandler {
	return &BackupHandler{svc: svc, callbacks: callbacks}
}

// HandleGetConfig handles GET /api/admin/backups/config.
func (h *BackupHandler) HandleGetConfig(c *echo.Context) error {
	db := c.Get("db").(*bun.DB)
	var cfg models.BackupConfig
	err := db.NewSelect().Model(&cfg).Where("id = 1").Scan(c.Request().Context())
	if err != nil {
		slog.Error("backup: get config", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to read backup config")
	}
	return c.JSON(http.StatusOK, map[string]any{
		"schedule_cron":   cfg.ScheduleCron,
		"retention_mode":  cfg.RetentionMode,
		"retention_value": cfg.RetentionValue,
	})
}

// HandleUpdateConfig handles PUT /api/admin/backups/config.
func (h *BackupHandler) HandleUpdateConfig(c *echo.Context) error {
	var req struct {
		ScheduleCron   string `json:"schedule_cron"`
		RetentionMode  string `json:"retention_mode"`
		RetentionValue int    `json:"retention_value"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.RetentionMode != "days" && req.RetentionMode != "count" {
		return echo.NewHTTPError(http.StatusBadRequest, "retention_mode must be 'days' or 'count'")
	}
	if req.RetentionValue < 1 {
		return echo.NewHTTPError(http.StatusBadRequest, "retention_value must be >= 1")
	}

	db := c.Get("db").(*bun.DB)
	_, err := db.NewUpdate().Model(&models.BackupConfig{
		ID:             1,
		ScheduleCron:   req.ScheduleCron,
		RetentionMode:  req.RetentionMode,
		RetentionValue: req.RetentionValue,
	}).Column("schedule_cron", "retention_mode", "retention_value", "updated_at").
		Where("id = 1").Exec(c.Request().Context())
	if err != nil {
		slog.Error("backup: update config", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update backup config")
	}

	// TODO: rebuild gocron job (Task 10)

	return c.JSON(http.StatusOK, map[string]any{
		"schedule_cron":   req.ScheduleCron,
		"retention_mode":  req.RetentionMode,
		"retention_value": req.RetentionValue,
	})
}

// HandleListBackups handles GET /api/admin/backups.
func (h *BackupHandler) HandleListBackups(c *echo.Context) error {
	backups, err := h.svc.ListBackups()
	if err != nil {
		slog.Error("backup: list", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list backups")
	}
	if backups == nil {
		backups = []backup.BackupInfo{}
	}
	return c.JSON(http.StatusOK, map[string]any{
		"backups": backups,
		"total":   len(backups),
	})
}

// HandleCreateBackup handles POST /api/admin/backups.
func (h *BackupHandler) HandleCreateBackup(c *echo.Context) error {
	if !backup.PgDumpAvailable() {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "pg_dump is not available on this system. Install PostgreSQL client tools to enable backups.",
		})
	}

	id, err := h.svc.CreateBackup("manual")
	if err != nil {
		if strings.Contains(err.Error(), "already in progress") {
			return c.JSON(http.StatusConflict, map[string]string{
				"error": "A backup or restore operation is already in progress",
			})
		}
		slog.Error("backup: create", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create backup")
	}

	// Apply retention after successful backup
	db := c.Get("db").(*bun.DB)
	var cfg models.BackupConfig
	if err := db.NewSelect().Model(&cfg).Where("id = 1").Scan(c.Request().Context()); err == nil {
		if err := h.svc.ApplyRetention(cfg.RetentionMode, cfg.RetentionValue); err != nil {
			slog.Warn("backup: retention cleanup failed", "err", err)
		}
	}

	return c.JSON(http.StatusOK, map[string]string{
		"backup_id": id,
		"message":   "Backup created successfully",
	})
}

// HandleDeleteBackup handles DELETE /api/admin/backups/:id.
func (h *BackupHandler) HandleDeleteBackup(c *echo.Context) error {
	id := c.PathParam("id")
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "backup id required")
	}
	if err := h.svc.DeleteBackup(id); err != nil {
		if os.IsNotExist(err) {
			return echo.NewHTTPError(http.StatusNotFound, "backup not found")
		}
		slog.Error("backup: delete", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete backup")
	}
	return c.NoContent(http.StatusNoContent)
}

// HandleDownloadBackup handles GET /api/admin/backups/:id/download.
func (h *BackupHandler) HandleDownloadBackup(c *echo.Context) error {
	id := c.PathParam("id")
	path := h.svc.GetBackupPath(id)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return echo.NewHTTPError(http.StatusNotFound, "backup not found")
	}
	return c.Attachment(path, filepath.Base(path))
}

// HandleRestore handles POST /api/admin/backups/:id/restore.
func (h *BackupHandler) HandleRestore(c *echo.Context) error {
	if !backup.PsqlAvailable() {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "psql is not available on this system. Install PostgreSQL client tools to enable restore.",
		})
	}

	var req struct {
		Confirm bool `json:"confirm"`
	}
	if err := c.Bind(&req); err != nil || !req.Confirm {
		return echo.NewHTTPError(http.StatusBadRequest, "must confirm restore with {\"confirm\": true}")
	}

	id := c.PathParam("id")
	path := h.svc.GetBackupPath(id)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return echo.NewHTTPError(http.StatusNotFound, "backup not found")
	}

	opts := h.makeRestoreOpts(false)
	if err := h.svc.RestoreBackup(id, opts); err != nil {
		slog.Error("backup: restore failed", "err", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Restore failed: %v", err),
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"success": true,
		"message": fmt.Sprintf("Restore completed from: %s. All sessions have been cleared — please log in again.", id),
	})
}

// HandleRestoreUpload handles POST /api/admin/backups/restore/upload.
func (h *BackupHandler) HandleRestoreUpload(c *echo.Context) error {
	if !backup.PsqlAvailable() {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "psql is not available on this system. Install PostgreSQL client tools to enable restore.",
		})
	}

	file, err := c.FormFile("file")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "file upload required")
	}

	// Max 2 GB
	if file.Size > 2*1024*1024*1024 {
		return echo.NewHTTPError(http.StatusBadRequest, "file too large (max 2 GB)")
	}

	src, err := file.Open()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to open uploaded file")
	}
	defer src.Close()

	// Save to temp file
	tmpFile, err := os.CreateTemp("", "nexorious-upload-*.tar.gz")
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create temp file")
	}
	tmpPath := tmpFile.Name()
	if _, err := tmpFile.ReadFrom(src); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to save uploaded file")
	}
	tmpFile.Close()

	opts := h.makeRestoreOpts(false)
	if err := h.svc.RestoreFromUpload(tmpPath, opts); err != nil {
		os.Remove(tmpPath)
		slog.Error("backup: restore from upload failed", "err", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Restore failed: %v", err),
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"success": true,
		"message": "Restore completed. All sessions have been cleared — please log in again.",
	})
}

// HandleSetupRestore handles POST /api/auth/setup/restore.
func (h *BackupHandler) HandleSetupRestore(c *echo.Context) error {
	if !backup.PsqlAvailable() {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "psql is not available on this system. Install PostgreSQL client tools to enable restore.",
		})
	}

	// Check if any user exists
	db := c.Get("db").(*bun.DB)
	var count int
	if err := db.QueryRowContext(c.Request().Context(), "SELECT COUNT(*) FROM users").Scan(&count); err == nil && count > 0 {
		return echo.NewHTTPError(http.StatusForbidden, "setup already complete — users exist")
	}

	file, err := c.FormFile("file")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "file upload required")
	}

	if file.Size > 2*1024*1024*1024 {
		return echo.NewHTTPError(http.StatusBadRequest, "file too large (max 2 GB)")
	}

	src, err := file.Open()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to open uploaded file")
	}
	defer src.Close()

	tmpFile, err := os.CreateTemp("", "nexorious-setup-restore-*.tar.gz")
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create temp file")
	}
	tmpPath := tmpFile.Name()
	if _, err := tmpFile.ReadFrom(src); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to save uploaded file")
	}
	tmpFile.Close()

	opts := h.makeRestoreOpts(true) // skip pre-restore for setup
	if err := h.svc.RestoreFromUpload(tmpPath, opts); err != nil {
		os.Remove(tmpPath)
		slog.Error("backup: setup restore failed", "err", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Restore failed: %v", err),
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"success": true,
		"message": "Backup restored successfully. Please log in with your restored credentials.",
	})
}

func (h *BackupHandler) makeRestoreOpts(skipPreRestore bool) backup.RestoreOpts {
	if h.callbacks == nil {
		// Test mode — no-op callbacks
		return backup.RestoreOpts{
			SkipPreRestore:  skipPreRestore,
			SetMaintenance:  func(bool) {},
			ShutdownPool:    func() {},
			StopScheduler:   func() {},
			CloseDB:         func() error { return nil },
			ReconnectDB:     func() (*bun.DB, error) { return nil, nil },
			RebuildServices: func(*bun.DB) error { return nil },
			ReinitMigrator:  func() error { return nil },
			SetAppState:     func(string) {},
		}
	}
	return backup.RestoreOpts{
		SkipPreRestore:  skipPreRestore,
		SetMaintenance:  h.callbacks.SetMaintenance,
		ShutdownPool:    h.callbacks.ShutdownPool,
		StopScheduler:   h.callbacks.StopScheduler,
		CloseDB:         h.callbacks.CloseDB,
		ReconnectDB:     h.callbacks.ReconnectDB,
		RebuildServices: h.callbacks.RebuildServices,
		ReinitMigrator:  h.callbacks.ReinitMigrator,
		SetAppState:     h.callbacks.SetAppState,
		MaxMigration:    h.callbacks.MaxMigration,
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/api/... -run TestHandleListBackups -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/api/backup.go internal/api/backup_test.go
git commit -m "feat(backup): add admin backup HTTP handlers"
```

---

### Task 9: Router Wiring — Routes, Gates, Health Endpoint, Maintenance Middleware

**Files:**
- Modify: `internal/api/router.go`

- [ ] **Step 1: Update router imports and function signatures**

Add imports for `backup`, `middleware` packages. Update `registerRoutes` to accept `*backup.Service` and `*RestoreCallbacks`. Add the maintenance middleware after Gate 3 (inside the Ready state). Update the health endpoint to include `backup_available`. Wire backup routes. Replace the setup restore placeholder.

Key changes to `registerRoutes`:

1. **Signature change:** Add `backupSvc *backup.Service, restoreCallbacks *RestoreCallbacks` parameters
2. **Maintenance middleware** after Gate 3:
```go
// Gate 4: Maintenance mode — blocks most requests during restore
e.Use(maint.MaintenanceMiddleware())
```
3. **Health endpoint** update:
```go
return c.JSON(http.StatusOK, map[string]any{
	"status":           status,
	"igdb_configured":  igdbConfigured,
	"backup_available": backup.PgDumpAvailable() && backup.PsqlAvailable(),
})
```
4. **Backup routes** inside the `if db != nil` block:
```go
bh := NewBackupHandler(backupSvc, restoreCallbacks)

// Admin backup routes (JWT + admin)
admin := e.Group("", auth.JWTMiddleware(cfg.SecretKey, db), auth.AdminMiddleware())
adminBackups := admin.Group("/api/admin/backups")
adminBackups.GET("/config", bh.HandleGetConfig)
adminBackups.PUT("/config", bh.HandleUpdateConfig)
adminBackups.GET("", bh.HandleListBackups)
adminBackups.POST("", bh.HandleCreateBackup)
adminBackups.DELETE("/:id", bh.HandleDeleteBackup)
adminBackups.GET("/:id/download", bh.HandleDownloadBackup)
adminBackups.POST("/:id/restore", bh.HandleRestore)
adminBackups.POST("/restore/upload", bh.HandleRestoreUpload)
```
5. **Replace setup restore placeholder:**
```go
e.POST("/api/auth/setup/restore", bh.HandleSetupRestore)
```
6. **Update gates** 1/2/3 to allow `/api/admin/backups` through during maintenance mode (the maintenance middleware itself handles the 503)

- [ ] **Step 2: Update `api.New` signature** to accept the new parameters and pass them through

- [ ] **Step 3: Update `main.go`** to pass the new parameters to `api.New`

- [ ] **Step 4: Verify it compiles**

Run: `go build ./...`
Expected: success

- [ ] **Step 5: Run existing router tests**

Run: `go test ./internal/api/... -run TestAppState -v`
Expected: PASS (existing tests still work)

- [ ] **Step 6: Commit**

```bash
git add internal/api/router.go cmd/nexorious/main.go
git commit -m "feat(backup): wire backup routes, maintenance middleware, and health endpoint"
```

---

### Task 10: Scheduler Integration — Scheduled Backups

**Files:**
- Modify: `internal/scheduler/scheduler.go`

- [ ] **Step 1: Add backup service and backup job to scheduler**

Update `Scheduler` struct to hold a `*backup.Service` reference. Update `NewScheduler` to accept it. In `Start()`, read `backup_config` and register a gocron job if `schedule_cron` is non-empty and `pg_dump` is available. Add a `RebuildBackupJob` method for when config is updated via the API.

```go
type Scheduler struct {
	db        *bun.DB
	pool      *worker.Pool
	backupSvc *backup.Service
	scheduler gocron.Scheduler
	backupJob gocron.Job
}

func NewScheduler(db *bun.DB, pool *worker.Pool, backupSvc *backup.Service) *Scheduler {
	return &Scheduler{db: db, pool: pool, backupSvc: backupSvc}
}
```

In `Start()`, after existing jobs:

```go
// Scheduled backup job
if s.backupSvc != nil {
	var cfg models.BackupConfig
	err := s.db.NewSelect().Model(&cfg).Where("id = 1").Scan(ctx)
	if err != nil {
		slog.Warn("scheduler: could not read backup_config", "err", err)
	} else if cfg.ScheduleCron != "" && backup.PgDumpAvailable() {
		s.registerBackupJob(ctx, cfg.ScheduleCron, cfg.RetentionMode, cfg.RetentionValue)
	} else if !backup.PgDumpAvailable() {
		slog.Warn("scheduler: pg_dump not available — skipping scheduled backup job")
	}
}
```

Add method:

```go
func (s *Scheduler) registerBackupJob(ctx context.Context, cron, retentionMode string, retentionValue int) {
	job, err := s.scheduler.NewJob(
		gocron.CronJob(cron, false),
		gocron.NewTask(func() {
			id, err := s.backupSvc.CreateBackup("scheduled")
			if err != nil {
				slog.Error("scheduled backup failed", "err", err)
				return
			}
			slog.Info("scheduled backup created", "id", id)
			if err := s.backupSvc.ApplyRetention(retentionMode, retentionValue); err != nil {
				slog.Warn("scheduled backup retention cleanup failed", "err", err)
			}
		}),
	)
	if err != nil {
		slog.Error("scheduler: failed to register backup job", "err", err)
		return
	}
	s.backupJob = job
	slog.Info("scheduler: backup job registered", "cron", cron)
}

// RebuildBackupJob removes the existing backup job and registers a new one.
func (s *Scheduler) RebuildBackupJob(ctx context.Context, cron, retentionMode string, retentionValue int) {
	if s.backupJob != nil {
		s.scheduler.RemoveJob(s.backupJob.ID())
		s.backupJob = nil
	}
	if cron != "" && backup.PgDumpAvailable() {
		s.registerBackupJob(ctx, cron, retentionMode, retentionValue)
	}
}
```

- [ ] **Step 2: Update NewScheduler call site in main.go**

```go
sched = scheduler.NewScheduler(db, pool, backupSvc)
```

- [ ] **Step 3: Verify it compiles**

Run: `go build ./...`
Expected: success

- [ ] **Step 4: Run scheduler tests**

Run: `go test ./internal/scheduler/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/scheduler/scheduler.go cmd/nexorious/main.go
git commit -m "feat(backup): add scheduled backup job to scheduler"
```

---

### Task 11: Slumber Collection

**Files:**
- Modify: `slumber.yaml`

- [ ] **Step 1: Add backup endpoints to slumber.yaml**

Add an `admin/backups` folder with all 8 backup endpoints. Each admin endpoint needs the JWT bearer auth block. Use the existing pattern from other admin endpoints in the file.

Add these requests:
- `GET /api/admin/backups/config` (admin JWT)
- `PUT /api/admin/backups/config` (admin JWT, JSON body)
- `GET /api/admin/backups` (admin JWT)
- `POST /api/admin/backups` (admin JWT)
- `DELETE /api/admin/backups/:id` (admin JWT)
- `GET /api/admin/backups/:id/download` (admin JWT)
- `POST /api/admin/backups/:id/restore` (admin JWT, JSON body `{"confirm": true}`)
- `POST /api/admin/backups/restore/upload` (admin JWT, multipart file)

- [ ] **Step 2: Verify slumber collection loads**

Run: `slumber collection`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add slumber.yaml
git commit -m "feat(backup): add backup admin endpoints to slumber collection"
```

---

### Task 12: Integration Smoke Test & Final Verification

**Files:** None new — verification only.

- [ ] **Step 1: Build everything**

Run: `go build ./...`
Expected: success, zero errors

- [ ] **Step 2: Run all Go tests**

Run: `go test ./... -timeout 300s`
Expected: PASS

- [ ] **Step 3: Run linter**

Run: `golangci-lint run`
Expected: zero errors

- [ ] **Step 4: Final commit if any fixups needed**

```bash
git add -A
git commit -m "fix(backup): address lint and test issues"
```
