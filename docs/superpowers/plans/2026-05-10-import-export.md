# Import & Export Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement nexorious JSON import, JSON/CSV export, and download endpoints — completing Phase 3.

**Architecture:** Fan-out import (one worker task per game via `import_item` task type), single-task export (`export_json` / `export_csv`), download handler serving completed export files with 24h expiry. All endpoints JWT-protected, wired through the existing `worker.Pool` and `Job`/`JobItem` models.

**Tech Stack:** Go, Echo v5, Bun ORM, testcontainers-go, existing worker pool

---

### Task 1: Import Handler — Validation & Job Creation

**Files:**
- Create: `internal/api/import.go`
- Test: `internal/api/import_test.go`
- Modify: `internal/api/router.go` (add import routes)

- [ ] **Step 1: Write test helpers and first failing test — reject missing file**

Create `internal/api/import_test.go`:

```go
package api_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	bunmigrate "github.com/uptrace/bun/migrate"

	"github.com/drzero42/nexorious-go/internal/api"
	"github.com/drzero42/nexorious-go/internal/config"
	"github.com/drzero42/nexorious-go/internal/db/migrations"
	"github.com/drzero42/nexorious-go/internal/migrate"
	"github.com/drzero42/nexorious-go/internal/worker"
)

func setupImportTestDB(t *testing.T) *bun.DB {
	t.Helper()
	ctx := context.Background()
	ctr, err := tcpostgres.Run(ctx,
		"postgres:18-alpine",
		tcpostgres.WithDatabase("nexorious_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
		),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = ctr.Terminate(ctx) })

	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(connStr)))
	db := bun.NewDB(sqldb, pgdialect.New())
	t.Cleanup(func() { _ = db.Close() })

	migrator := bunmigrate.NewMigrator(db, migrations.Migrations)
	if err := migrator.Init(ctx); err != nil {
		t.Fatalf("migrator init: %v", err)
	}
	if _, err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func importTestCfg() *config.Config {
	return &config.Config{
		SecretKey:                "test-secret-key-at-least-32-bytes!",
		AccessTokenExpireMinutes: 15,
		RefreshTokenExpireDays:   30,
		Port:                     8000,
		StoragePath:              "/tmp/nexorious_test_storage",
	}
}

func importTestEcho(t *testing.T, db *bun.DB, cfg *config.Config, pool *worker.Pool) interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
} {
	t.Helper()
	m := migrate.NewMigratorForTest(migrate.AppStateReady)
	return api.New(cfg, m, db, "", nil, pool)
}

func TestImportNexorious_NoFile(t *testing.T) {
	db := setupImportTestDB(t)
	cfg := importTestCfg()
	pool := worker.NewPool(db)

	insertAuthTestUser(t, db, "user1", "admin", "password123", true, false)
	token := issueTestToken(t, cfg, "user1")

	handler := importTestEcho(t, db, cfg, pool)

	req := httptest.NewRequest(http.MethodPost, "/api/import/nexorious", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}
```

Note: `issueTestToken` and `insertAuthTestUser` are already in `auth_test.go` in the same `api_test` package, so they're available here.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/... -run TestImportNexorious_NoFile -v`
Expected: FAIL — `ImportHandler` not defined, route not registered.

- [ ] **Step 3: Create the ImportHandler struct and stub HandleImportNexorious**

Create `internal/api/import.go`:

```go
package api

import (
	"net/http"

	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious-go/internal/worker"
)

// ImportHandler handles import-related endpoints.
type ImportHandler struct {
	db   *bun.DB
	pool *worker.Pool
}

// NewImportHandler returns a new ImportHandler.
func NewImportHandler(db *bun.DB, pool *worker.Pool) *ImportHandler {
	return &ImportHandler{db: db, pool: pool}
}

// HandleImportNexorious handles POST /api/import/nexorious.
func (h *ImportHandler) HandleImportNexorious(c *echo.Context) error {
	return echo.NewHTTPError(http.StatusBadRequest, "no file uploaded")
}
```

- [ ] **Step 4: Wire the import route in router.go**

In `internal/api/router.go`, inside the `if db != nil` block (after the job-items group), add:

```go
		// Import routes (all JWT-protected)
		imh := NewImportHandler(db, pool)
		importGroup := e.Group("/api/import", auth.JWTMiddleware(cfg.SecretKey, db))
		importGroup.POST("/nexorious", imh.HandleImportNexorious)
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/api/... -run TestImportNexorious_NoFile -v`
Expected: PASS

- [ ] **Step 6: Write failing tests for validation cases**

Add to `internal/api/import_test.go`:

```go
func validExportJSON(games []map[string]any) []byte {
	if games == nil {
		games = []map[string]any{
			{
				"igdb_id":    1942,
				"title":      "The Witcher 3",
				"play_status": "completed",
				"platforms":  []any{},
				"tags":       []any{},
			},
		}
	}
	data := map[string]any{
		"export_version": "1.2",
		"export_date":    "2026-05-10T12:00:00Z",
		"user_id":        "abc",
		"total_games":    len(games),
		"total_wishlist": 0,
		"games":          games,
		"wishlist":       []any{},
	}
	b, _ := json.Marshal(data)
	return b
}

func TestImportNexorious_InvalidJSON(t *testing.T) {
	db := setupImportTestDB(t)
	cfg := importTestCfg()
	pool := worker.NewPool(db)
	insertAuthTestUser(t, db, "user1", "admin", "password123", true, false)
	token := issueTestToken(t, cfg, "user1")
	handler := importTestEcho(t, db, cfg, pool)

	rec := postMultipartFile(t, handler, "/api/import/nexorious", token, "file", "export.json", []byte("not json"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestImportNexorious_WrongVersion(t *testing.T) {
	db := setupImportTestDB(t)
	cfg := importTestCfg()
	pool := worker.NewPool(db)
	insertAuthTestUser(t, db, "user1", "admin", "password123", true, false)
	token := issueTestToken(t, cfg, "user1")
	handler := importTestEcho(t, db, cfg, pool)

	data := map[string]any{
		"export_version": "1.0",
		"games":          []any{{"igdb_id": 1, "title": "Game"}},
	}
	b, _ := json.Marshal(data)
	rec := postMultipartFile(t, handler, "/api/import/nexorious", token, "file", "export.json", b)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestImportNexorious_EmptyGames(t *testing.T) {
	db := setupImportTestDB(t)
	cfg := importTestCfg()
	pool := worker.NewPool(db)
	insertAuthTestUser(t, db, "user1", "admin", "password123", true, false)
	token := issueTestToken(t, cfg, "user1")
	handler := importTestEcho(t, db, cfg, pool)

	data := map[string]any{
		"export_version": "1.2",
		"games":          []any{},
	}
	b, _ := json.Marshal(data)
	rec := postMultipartFile(t, handler, "/api/import/nexorious", token, "file", "export.json", b)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestImportNexorious_Success(t *testing.T) {
	db := setupImportTestDB(t)
	cfg := importTestCfg()
	pool := worker.NewPool(db)
	insertAuthTestUser(t, db, "user1", "admin", "password123", true, false)
	token := issueTestToken(t, cfg, "user1")
	handler := importTestEcho(t, db, cfg, pool)

	rec := postMultipartFile(t, handler, "/api/import/nexorious", token, "file", "export.json", validExportJSON(nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["job_id"] == nil {
		t.Fatal("expected job_id in response")
	}
	if resp["total_items"].(float64) != 1 {
		t.Fatalf("expected total_items=1, got %v", resp["total_items"])
	}
}

// postMultipartFile creates a multipart/form-data request with a file field.
func postMultipartFile(t *testing.T, handler interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, path, token, fieldName, fileName string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile(fieldName, fileName)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("Write: %v", err)
	}
	w.Close()

	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}
```

Add `"bytes"`, `"mime/multipart"` to the imports.

- [ ] **Step 7: Run tests to verify they fail**

Run: `go test ./internal/api/... -run TestImportNexorious -v`
Expected: FAIL — handler only returns 400 unconditionally.

- [ ] **Step 8: Implement full HandleImportNexorious**

Replace the body of `HandleImportNexorious` in `internal/api/import.go`:

```go
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious-go/internal/auth"
	"github.com/drzero42/nexorious-go/internal/db/models"
	"github.com/drzero42/nexorious-go/internal/worker"
)

type ImportHandler struct {
	db   *bun.DB
	pool *worker.Pool
}

func NewImportHandler(db *bun.DB, pool *worker.Pool) *ImportHandler {
	return &ImportHandler{db: db, pool: pool}
}

// nexoriousExportFile represents the top-level structure of a nexorious export.
type nexoriousExportFile struct {
	ExportVersion string           `json:"export_version"`
	Games         []json.RawMessage `json:"games"`
}

// nexoriousGameEntry holds the minimal fields we need from each game for job item creation.
type nexoriousGameEntry struct {
	IGDBID int32  `json:"igdb_id"`
	Title  string `json:"title"`
}

func (h *ImportHandler) HandleImportNexorious(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	// Read the uploaded file.
	file, _, err := c.Request().FormFile("file")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "no file uploaded")
	}
	defer file.Close()

	// Limit to 50 MB.
	limited := io.LimitReader(file, 50*1024*1024+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "failed to read file")
	}
	if len(body) > 50*1024*1024 {
		return echo.NewHTTPError(http.StatusRequestEntityTooLarge, "file exceeds 50 MB limit")
	}

	// Parse JSON.
	var export nexoriousExportFile
	if err := json.Unmarshal(body, &export); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON")
	}

	// Validate export_version.
	if export.ExportVersion != "1.2" {
		return echo.NewHTTPError(http.StatusBadRequest, "Unsupported export version. Only version 1.2 is supported.")
	}

	// Validate games array.
	if len(export.Games) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "no games in export file")
	}

	ctx := context.Background()

	// Check for active nexorious import.
	var activeCount int
	activeCount, err = h.db.NewSelect().TableExpr("jobs").
		Where("user_id = ?", userID).
		Where("job_type = ?", models.JobTypeImport).
		Where("source = ?", models.JobSourceNexorious).
		Where("status IN (?)", bun.In([]string{models.JobStatusPending, models.JobStatusProcessing})).
		Count(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to check active imports")
	}
	if activeCount > 0 {
		return echo.NewHTTPError(http.StatusConflict, "an import is already in progress")
	}

	// Create job.
	jobID := uuid.NewString()
	job := &models.Job{
		ID:         jobID,
		UserID:     userID,
		JobType:    models.JobTypeImport,
		Source:     models.JobSourceNexorious,
		Status:     models.JobStatusPending,
		Priority:   models.JobPriorityHigh,
		TotalItems: len(export.Games),
	}
	_, err = h.db.NewInsert().Model(job).Exec(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create import job")
	}

	// Create job items and enqueue tasks.
	for i, rawGame := range export.Games {
		var entry nexoriousGameEntry
		_ = json.Unmarshal(rawGame, &entry)

		itemKey := fmt.Sprintf("game_%d", i)
		if entry.IGDBID > 0 {
			itemKey = fmt.Sprintf("igdb_%d", entry.IGDBID)
		}
		sourceTitle := entry.Title
		if sourceTitle == "" {
			sourceTitle = fmt.Sprintf("Game %d", i)
		}

		metadata, _ := json.Marshal(map[string]any{
			"item_type": "game",
			"data":      rawGame,
		})

		itemID := uuid.NewString()
		item := &models.JobItem{
			ID:             itemID,
			JobID:          jobID,
			UserID:         userID,
			ItemKey:        itemKey,
			SourceTitle:    sourceTitle,
			SourceMetadata: metadata,
			Status:         models.JobItemStatusPending,
			Result:         json.RawMessage("{}"),
			IGDBCandidates: json.RawMessage("[]"),
		}
		_, err = h.db.NewInsert().Model(item).Exec(ctx)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to create job items")
		}

		_ = h.pool.Submit(ctx, "import_item", map[string]string{
			"job_item_id": itemID,
		}, 5)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"job_id":      jobID,
		"source":      "nexorious",
		"status":      "pending",
		"message":     fmt.Sprintf("Import job created. Processing %d games.", len(export.Games)),
		"total_items": len(export.Games),
	})
}
```

- [ ] **Step 9: Run all import tests**

Run: `go test ./internal/api/... -run TestImportNexorious -v`
Expected: All PASS

- [ ] **Step 10: Write and run conflict test**

Add to `internal/api/import_test.go`:

```go
func TestImportNexorious_Conflict(t *testing.T) {
	db := setupImportTestDB(t)
	cfg := importTestCfg()
	pool := worker.NewPool(db)
	insertAuthTestUser(t, db, "user1", "admin", "password123", true, false)
	token := issueTestToken(t, cfg, "user1")
	handler := importTestEcho(t, db, cfg, pool)

	// First import — should succeed.
	rec := postMultipartFile(t, handler, "/api/import/nexorious", token, "file", "export.json", validExportJSON(nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("first import: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Second import — should conflict.
	rec = postMultipartFile(t, handler, "/api/import/nexorious", token, "file", "export.json", validExportJSON(nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}
```

Run: `go test ./internal/api/... -run TestImportNexorious_Conflict -v`
Expected: PASS

- [ ] **Step 11: Commit**

```bash
git add internal/api/import.go internal/api/import_test.go internal/api/router.go
git commit -m "feat: add nexorious import handler with validation and job creation"
```

---

### Task 2: Import Item Worker Task

**Files:**
- Create: `internal/worker/tasks/import_item.go`
- Test: `internal/worker/tasks/import_item_test.go`
- Modify: `cmd/nexorious/main.go` (register task handler)

- [ ] **Step 1: Create the tasks directory and write the first failing test**

Create `internal/worker/tasks/import_item_test.go`:

```go
package tasks_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	bunmigrate "github.com/uptrace/bun/migrate"

	"github.com/drzero42/nexorious-go/internal/db/migrations"
	"github.com/drzero42/nexorious-go/internal/db/models"
	"github.com/drzero42/nexorious-go/internal/worker/tasks"
)

func setupTasksTestDB(t *testing.T) *bun.DB {
	t.Helper()
	ctx := context.Background()
	ctr, err := tcpostgres.Run(ctx,
		"postgres:18-alpine",
		tcpostgres.WithDatabase("nexorious_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
		),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = ctr.Terminate(ctx) })

	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(connStr)))
	db := bun.NewDB(sqldb, pgdialect.New())
	t.Cleanup(func() { _ = db.Close() })

	migrator := bunmigrate.NewMigrator(db, migrations.Migrations)
	if err := migrator.Init(ctx); err != nil {
		t.Fatalf("migrator init: %v", err)
	}
	if _, err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func insertTestUser(t *testing.T, db *bun.DB, userID string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		"INSERT INTO users (id, username, password_hash, is_active, is_admin) VALUES (?, ?, ?, ?, ?)",
		userID, "testuser", "$2a$12$fakehashfakehashfakehashfakehashfakehashfakehashfake", true, false,
	)
	if err != nil {
		t.Fatalf("insertTestUser: %v", err)
	}
}

func insertTestJob(t *testing.T, db *bun.DB, jobID, userID string, totalItems int) {
	t.Helper()
	job := &models.Job{
		ID:         jobID,
		UserID:     userID,
		JobType:    models.JobTypeImport,
		Source:     models.JobSourceNexorious,
		Status:     models.JobStatusPending,
		Priority:   models.JobPriorityHigh,
		TotalItems: totalItems,
	}
	_, err := db.NewInsert().Model(job).Exec(context.Background())
	if err != nil {
		t.Fatalf("insertTestJob: %v", err)
	}
}

func insertTestJobItem(t *testing.T, db *bun.DB, jobID, userID string, gameData map[string]any) string {
	t.Helper()
	itemID := uuid.NewString()
	metadata, _ := json.Marshal(map[string]any{
		"item_type": "game",
		"data":      gameData,
	})
	item := &models.JobItem{
		ID:             itemID,
		JobID:          jobID,
		UserID:         userID,
		ItemKey:        "igdb_1942",
		SourceTitle:    "The Witcher 3",
		SourceMetadata: metadata,
		Status:         models.JobItemStatusPending,
		Result:         json.RawMessage("{}"),
		IGDBCandidates: json.RawMessage("[]"),
	}
	_, err := db.NewInsert().Model(item).Exec(context.Background())
	if err != nil {
		t.Fatalf("insertTestJobItem: %v", err)
	}
	return itemID
}

func TestImportItem_BasicGame(t *testing.T) {
	db := setupTasksTestDB(t)
	ctx := context.Background()
	userID := "user1"
	jobID := uuid.NewString()

	insertTestUser(t, db, userID)
	insertTestJob(t, db, jobID, userID, 1)

	gameData := map[string]any{
		"igdb_id":         1942,
		"title":           "The Witcher 3: Wild Hunt",
		"release_year":    2015,
		"play_status":     "completed",
		"personal_rating": 9,
		"is_loved":        true,
		"hours_played":    120.0,
		"personal_notes":  "Best RPG ever",
		"platforms":       []any{},
		"tags":            []any{},
	}
	itemID := insertTestJobItem(t, db, jobID, userID, gameData)

	handler := tasks.NewImportItemHandler(db)

	task := &models.PendingTask{
		ID:       uuid.NewString(),
		TaskType: "import_item",
		Payload:  mustMarshal(t, map[string]string{"job_item_id": itemID}),
	}
	err := handler(ctx, task)
	if err != nil {
		t.Fatalf("import_item failed: %v", err)
	}

	// Verify job item is completed.
	var ji models.JobItem
	err = db.NewSelect().Model(&ji).Where("id = ?", itemID).Scan(ctx)
	if err != nil {
		t.Fatalf("failed to read job item: %v", err)
	}
	if ji.Status != models.JobItemStatusCompleted {
		t.Fatalf("expected status completed, got %s", ji.Status)
	}

	// Verify game was created.
	var game models.Game
	err = db.NewSelect().Model(&game).Where("id = ?", 1942).Scan(ctx)
	if err != nil {
		t.Fatalf("game not created: %v", err)
	}
	if game.Title != "The Witcher 3: Wild Hunt" {
		t.Fatalf("expected title 'The Witcher 3: Wild Hunt', got %q", game.Title)
	}

	// Verify user_game was created.
	var ug models.UserGame
	err = db.NewSelect().Model(&ug).Where("user_id = ? AND game_id = ?", userID, 1942).Scan(ctx)
	if err != nil {
		t.Fatalf("user_game not created: %v", err)
	}
	if *ug.PlayStatus != "completed" {
		t.Fatalf("expected play_status=completed, got %v", *ug.PlayStatus)
	}
}

func TestImportItem_MissingIGDBID(t *testing.T) {
	db := setupTasksTestDB(t)
	ctx := context.Background()
	userID := "user1"
	jobID := uuid.NewString()

	insertTestUser(t, db, userID)
	insertTestJob(t, db, jobID, userID, 1)

	gameData := map[string]any{
		"title":      "No IGDB Game",
		"platforms":  []any{},
		"tags":       []any{},
	}
	itemID := insertTestJobItem(t, db, jobID, userID, gameData)

	handler := tasks.NewImportItemHandler(db)
	task := &models.PendingTask{
		ID:       uuid.NewString(),
		TaskType: "import_item",
		Payload:  mustMarshal(t, map[string]string{"job_item_id": itemID}),
	}
	err := handler(ctx, task)
	if err != nil {
		t.Fatalf("import_item should not return error: %v", err)
	}

	var ji models.JobItem
	err = db.NewSelect().Model(&ji).Where("id = ?", itemID).Scan(ctx)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if ji.Status != models.JobItemStatusFailed {
		t.Fatalf("expected failed, got %s", ji.Status)
	}
}

func TestImportItem_DuplicateGame(t *testing.T) {
	db := setupTasksTestDB(t)
	ctx := context.Background()
	userID := "user1"
	jobID := uuid.NewString()

	insertTestUser(t, db, userID)
	insertTestJob(t, db, jobID, userID, 1)

	gameData := map[string]any{
		"igdb_id":    1942,
		"title":      "The Witcher 3",
		"play_status": "completed",
		"platforms":  []any{},
		"tags":       []any{},
	}

	// First import.
	itemID1 := insertTestJobItem(t, db, jobID, userID, gameData)
	handler := tasks.NewImportItemHandler(db)
	task1 := &models.PendingTask{
		ID:       uuid.NewString(),
		TaskType: "import_item",
		Payload:  mustMarshal(t, map[string]string{"job_item_id": itemID1}),
	}
	if err := handler(ctx, task1); err != nil {
		t.Fatalf("first import: %v", err)
	}

	// Second import — same game, same user.
	itemID2 := insertTestJobItem(t, db, jobID, userID, gameData)
	task2 := &models.PendingTask{
		ID:       uuid.NewString(),
		TaskType: "import_item",
		Payload:  mustMarshal(t, map[string]string{"job_item_id": itemID2}),
	}
	if err := handler(ctx, task2); err != nil {
		t.Fatalf("second import: %v", err)
	}

	var ji models.JobItem
	_ = db.NewSelect().Model(&ji).Where("id = ?", itemID2).Scan(ctx)
	if ji.Status != models.JobItemStatusCompleted {
		t.Fatalf("expected completed, got %s", ji.Status)
	}
	// Check result has already_exists.
	var result map[string]any
	_ = json.Unmarshal(ji.Result, &result)
	if result["already_exists"] != true {
		t.Fatalf("expected already_exists=true in result, got %v", result)
	}
}

func mustMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/worker/tasks/... -run TestImportItem -v`
Expected: FAIL — package doesn't exist yet.

- [ ] **Step 3: Implement the import_item task handler**

Create `internal/worker/tasks/import_item.go`:

```go
package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious-go/internal/db/models"
)

// importGameData mirrors the game object from the nexorious export format.
type importGameData struct {
	IGDBID         int32                  `json:"igdb_id"`
	Title          string                 `json:"title"`
	ReleaseYear    *int                   `json:"release_year"`
	PlayStatus     *string                `json:"play_status"`
	PersonalRating *float64               `json:"personal_rating"`
	IsLoved        bool                   `json:"is_loved"`
	HoursPlayed    *float64               `json:"hours_played"`
	PersonalNotes  *string                `json:"personal_notes"`
	Platforms      []importPlatformEntry  `json:"platforms"`
	Tags           []importTagEntry       `json:"tags"`
	CreatedAt      *time.Time             `json:"created_at"`
	UpdatedAt      *time.Time             `json:"updated_at"`
}

type importPlatformEntry struct {
	PlatformID      string   `json:"platform_id"`
	PlatformName    string   `json:"platform_name"`
	StorefrontID    string   `json:"storefront_id"`
	StorefrontName  string   `json:"storefront_name"`
	StoreGameID     *string  `json:"store_game_id"`
	StoreUrl        *string  `json:"store_url"`
	IsAvailable     bool     `json:"is_available"`
	HoursPlayed     *float64 `json:"hours_played"`
	OwnershipStatus *string  `json:"ownership_status"`
	AcquiredDate    *string  `json:"acquired_date"`
}

type importTagEntry struct {
	Name  string  `json:"name"`
	Color *string `json:"color"`
}

// NewImportItemHandler returns a TaskHandler that processes a single import item.
func NewImportItemHandler(db *bun.DB) func(ctx context.Context, task *models.PendingTask) error {
	return func(ctx context.Context, task *models.PendingTask) error {
		var payload struct {
			JobItemID string `json:"job_item_id"`
		}
		if err := json.Unmarshal(task.Payload, &payload); err != nil {
			return fmt.Errorf("import_item: unmarshal payload: %w", err)
		}

		var item models.JobItem
		err := db.NewSelect().Model(&item).Where("id = ?", payload.JobItemID).Scan(ctx)
		if err != nil {
			return fmt.Errorf("import_item: load job item: %w", err)
		}

		userID := item.UserID
		jobID := item.JobID

		// Extract game data from source_metadata.
		var meta struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(item.SourceMetadata, &meta); err != nil {
			markItemFailed(ctx, db, item.ID, "invalid source_metadata")
			return nil
		}

		var game importGameData
		if err := json.Unmarshal(meta.Data, &game); err != nil {
			markItemFailed(ctx, db, item.ID, "invalid game data in source_metadata")
			return nil
		}

		// Validate igdb_id.
		if game.IGDBID == 0 {
			markItemFailed(ctx, db, item.ID, "missing igdb_id")
			checkJobCompletion(ctx, db, jobID)
			return nil
		}

		// Upsert game — only fill null fields on existing games.
		var existing models.Game
		err = db.NewSelect().Model(&existing).Where("id = ?", game.IGDBID).Scan(ctx)
		if errors.Is(err, sql.ErrNoRows) {
			newGame := models.Game{
				ID:    game.IGDBID,
				Title: game.Title,
			}
			if game.ReleaseYear != nil {
				rd := time.Date(*game.ReleaseYear, 1, 1, 0, 0, 0, 0, time.UTC)
				newGame.ReleaseDate = &rd
			}
			_, err = db.NewInsert().Model(&newGame).Exec(ctx)
			if err != nil {
				markItemFailed(ctx, db, item.ID, fmt.Sprintf("game insert: %v", err))
				checkJobCompletion(ctx, db, jobID)
				return nil
			}
		} else if err != nil {
			markItemFailed(ctx, db, item.ID, fmt.Sprintf("game lookup: %v", err))
			checkJobCompletion(ctx, db, jobID)
			return nil
		}
		// If game already exists, we don't overwrite — richer IGDB data is preserved.

		// Check for existing user_game (idempotent).
		var existingUG models.UserGame
		err = db.NewSelect().Model(&existingUG).
			Where("user_id = ? AND game_id = ?", userID, game.IGDBID).Scan(ctx)
		if err == nil {
			// Already exists — mark completed with already_exists.
			result, _ := json.Marshal(map[string]any{
				"game_id":        game.IGDBID,
				"user_game_id":   existingUG.ID,
				"already_exists": true,
			})
			markItemCompleted(ctx, db, item.ID, result)
			checkJobCompletion(ctx, db, jobID)
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			markItemFailed(ctx, db, item.ID, fmt.Sprintf("user_game lookup: %v", err))
			checkJobCompletion(ctx, db, jobID)
			return nil
		}

		// Create UserGame.
		now := time.Now().UTC()
		createdAt := now
		updatedAt := now
		if game.CreatedAt != nil {
			createdAt = *game.CreatedAt
		}
		if game.UpdatedAt != nil {
			updatedAt = *game.UpdatedAt
		}

		var personalRating *int32
		if game.PersonalRating != nil {
			v := int32(math.Trunc(*game.PersonalRating))
			personalRating = &v
		}

		ugID := uuid.NewString()
		ug := &models.UserGame{
			ID:             ugID,
			UserID:         userID,
			GameID:         game.IGDBID,
			PlayStatus:     game.PlayStatus,
			PersonalRating: personalRating,
			IsLoved:        game.IsLoved,
			HoursPlayed:    game.HoursPlayed,
			PersonalNotes:  game.PersonalNotes,
			CreatedAt:      createdAt,
			UpdatedAt:      updatedAt,
		}
		_, err = db.NewInsert().Model(ug).Exec(ctx)
		if err != nil {
			markItemFailed(ctx, db, item.ID, fmt.Sprintf("user_game insert: %v", err))
			checkJobCompletion(ctx, db, jobID)
			return nil
		}

		// Platforms.
		for _, p := range game.Platforms {
			// Look up platform slug.
			var plat models.Platform
			err := db.NewSelect().Model(&plat).Where("name = ?", p.PlatformID).Scan(ctx)
			if err != nil {
				slog.Warn("import_item: platform not found, skipping", "platform_id", p.PlatformID)
				continue
			}

			var storefront *string
			if p.StorefrontID != "" {
				var sf models.Storefront
				err := db.NewSelect().Model(&sf).Where("name = ?", p.StorefrontID).Scan(ctx)
				if err != nil {
					slog.Warn("import_item: storefront not found, setting null", "storefront_id", p.StorefrontID)
				} else {
					storefront = &sf.Name
				}
			}

			var acquiredDate *time.Time
			if p.AcquiredDate != nil && *p.AcquiredDate != "" {
				if t, err := time.Parse("2006-01-02", *p.AcquiredDate); err == nil {
					acquiredDate = &t
				}
			}

			ugp := &models.UserGamePlatform{
				ID:              uuid.NewString(),
				UserGameID:      ugID,
				Platform:        &plat.Name,
				Storefront:      storefront,
				StoreGameID:     p.StoreGameID,
				StoreUrl:        p.StoreUrl,
				IsAvailable:     p.IsAvailable,
				HoursPlayed:     p.HoursPlayed,
				OwnershipStatus: p.OwnershipStatus,
				AcquiredDate:    acquiredDate,
			}
			if _, err := db.NewInsert().Model(ugp).Exec(ctx); err != nil {
				slog.Warn("import_item: failed to insert platform", "err", err)
			}
		}

		// Tags.
		for _, t := range game.Tags {
			if t.Name == "" {
				continue
			}
			// Find or create tag (case-insensitive).
			var tag models.Tag
			err := db.NewSelect().Model(&tag).
				Where("user_id = ?", userID).
				Where("LOWER(name) = LOWER(?)", t.Name).
				Scan(ctx)
			if errors.Is(err, sql.ErrNoRows) {
				tag = models.Tag{
					ID:     uuid.NewString(),
					UserID: userID,
					Name:   t.Name,
					Color:  t.Color,
				}
				if _, err := db.NewInsert().Model(&tag).Exec(ctx); err != nil {
					markItemFailed(ctx, db, item.ID, fmt.Sprintf("tag create: %v", err))
					checkJobCompletion(ctx, db, jobID)
					return nil
				}
			} else if err != nil {
				markItemFailed(ctx, db, item.ID, fmt.Sprintf("tag lookup: %v", err))
				checkJobCompletion(ctx, db, jobID)
				return nil
			}

			ugt := &models.UserGameTag{
				ID:         uuid.NewString(),
				UserGameID: ugID,
				TagID:      tag.ID,
			}
			if _, err := db.NewInsert().Model(ugt).Exec(ctx); err != nil {
				markItemFailed(ctx, db, item.ID, fmt.Sprintf("user_game_tag insert: %v", err))
				checkJobCompletion(ctx, db, jobID)
				return nil
			}
		}

		// Mark completed.
		result, _ := json.Marshal(map[string]any{
			"game_id":         game.IGDBID,
			"user_game_id":    ugID,
			"is_new_addition": true,
		})
		markItemCompleted(ctx, db, item.ID, result)
		checkJobCompletion(ctx, db, jobID)
		return nil
	}
}

func markItemFailed(ctx context.Context, db *bun.DB, itemID string, errMsg string) {
	now := time.Now().UTC()
	_, err := db.NewRaw(
		`UPDATE job_items SET status = ?, error_message = ?, processed_at = ? WHERE id = ?`,
		models.JobItemStatusFailed, errMsg, now, itemID,
	).Exec(ctx)
	if err != nil {
		slog.Error("import_item: failed to mark item failed", "item_id", itemID, "err", err)
	}
}

func markItemCompleted(ctx context.Context, db *bun.DB, itemID string, result json.RawMessage) {
	now := time.Now().UTC()
	_, err := db.NewRaw(
		`UPDATE job_items SET status = ?, result = ?, processed_at = ? WHERE id = ?`,
		models.JobItemStatusCompleted, string(result), now, itemID,
	).Exec(ctx)
	if err != nil {
		slog.Error("import_item: failed to mark item completed", "item_id", itemID, "err", err)
	}
}

func checkJobCompletion(ctx context.Context, db *bun.DB, jobID string) {
	var pendingCount int
	pendingCount, err := db.NewSelect().TableExpr("job_items").
		Where("job_id = ?", jobID).
		Where("status = ?", models.JobItemStatusPending).
		Count(ctx)
	if err != nil {
		slog.Error("import_item: failed to count pending items", "job_id", jobID, "err", err)
		return
	}
	if pendingCount > 0 {
		return
	}

	// All items processed. Determine final status.
	var failedCount int
	failedCount, err = db.NewSelect().TableExpr("job_items").
		Where("job_id = ?", jobID).
		Where("status = ?", models.JobItemStatusFailed).
		Count(ctx)
	if err != nil {
		slog.Error("import_item: failed to count failed items", "job_id", jobID, "err", err)
		return
	}

	now := time.Now().UTC()
	finalStatus := models.JobStatusCompleted
	if failedCount > 0 {
		finalStatus = "completed_with_errors"
	}
	_, _ = db.NewRaw(
		`UPDATE jobs SET status = ?, completed_at = ? WHERE id = ?`,
		finalStatus, now, jobID,
	).Exec(ctx)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/worker/tasks/... -run TestImportItem -v`
Expected: All PASS

- [ ] **Step 5: Register the import_item handler in main.go**

In `cmd/nexorious/main.go`, add the import:

```go
"github.com/drzero42/nexorious-go/internal/worker/tasks"
```

Replace the comment block `// Register handlers here when consumer specs are implemented:` with:

```go
pool.Register("import_item", tasks.NewImportItemHandler(db))
```

- [ ] **Step 6: Run full build**

Run: `go build ./...`
Expected: No errors.

- [ ] **Step 7: Commit**

```bash
git add internal/worker/tasks/import_item.go internal/worker/tasks/import_item_test.go cmd/nexorious/main.go
git commit -m "feat: add import_item worker task with game upsert, platforms, and tags"
```

---

### Task 3: Export Handler — JSON and CSV Job Creation

**Files:**
- Create: `internal/api/export.go`
- Test: `internal/api/export_test.go`
- Modify: `internal/api/router.go` (add export routes)

- [ ] **Step 1: Write the first failing test — JSON export with empty collection**

Create `internal/api/export_test.go`:

```go
package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/drzero42/nexorious-go/internal/worker"
)

func TestExportJSON_NoGames(t *testing.T) {
	db := setupImportTestDB(t)
	cfg := importTestCfg()
	pool := worker.NewPool(db)
	insertAuthTestUser(t, db, "user1", "admin", "password123", true, false)
	token := issueTestToken(t, cfg, "user1")
	handler := importTestEcho(t, db, cfg, pool)

	req := httptest.NewRequest(http.MethodPost, "/api/export/json", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestExportJSON_Success(t *testing.T) {
	db := setupImportTestDB(t)
	cfg := importTestCfg()
	pool := worker.NewPool(db)
	insertAuthTestUser(t, db, "user1", "admin", "password123", true, false)
	token := issueTestToken(t, cfg, "user1")
	handler := importTestEcho(t, db, cfg, pool)

	// Insert a game and user_game so the collection isn't empty.
	_, _ = db.ExecContext(t.Context(),
		"INSERT INTO games (id, title) VALUES (1, 'Test Game')")
	_, _ = db.ExecContext(t.Context(),
		"INSERT INTO user_games (id, user_id, game_id, is_loved) VALUES ('ug1', 'user1', 1, false)")

	req := httptest.NewRequest(http.MethodPost, "/api/export/json", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["job_id"] == nil {
		t.Fatal("expected job_id in response")
	}
}

func TestExportCSV_Success(t *testing.T) {
	db := setupImportTestDB(t)
	cfg := importTestCfg()
	pool := worker.NewPool(db)
	insertAuthTestUser(t, db, "user1", "admin", "password123", true, false)
	token := issueTestToken(t, cfg, "user1")
	handler := importTestEcho(t, db, cfg, pool)

	_, _ = db.ExecContext(t.Context(),
		"INSERT INTO games (id, title) VALUES (1, 'Test Game')")
	_, _ = db.ExecContext(t.Context(),
		"INSERT INTO user_games (id, user_id, game_id, is_loved) VALUES ('ug1', 'user1', 1, false)")

	req := httptest.NewRequest(http.MethodPost, "/api/export/csv", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/api/... -run TestExport -v`
Expected: FAIL — `ExportHandler` not defined.

- [ ] **Step 3: Implement export handler**

Create `internal/api/export.go`:

```go
package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious-go/internal/auth"
	"github.com/drzero42/nexorious-go/internal/config"
	"github.com/drzero42/nexorious-go/internal/db/models"
	"github.com/drzero42/nexorious-go/internal/worker"
)

// ExportHandler handles export-related endpoints.
type ExportHandler struct {
	db   *bun.DB
	pool *worker.Pool
	cfg  *config.Config
}

// NewExportHandler returns a new ExportHandler.
func NewExportHandler(db *bun.DB, pool *worker.Pool, cfg *config.Config) *ExportHandler {
	return &ExportHandler{db: db, pool: pool, cfg: cfg}
}

func (h *ExportHandler) handleExport(c *echo.Context, taskType, source string) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	ctx := context.Background()

	// Count user's games.
	count, err := h.db.NewSelect().TableExpr("user_games").
		Where("user_id = ?", userID).Count(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to count games")
	}
	if count == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "no games to export")
	}

	jobID := uuid.NewString()
	job := &models.Job{
		ID:         jobID,
		UserID:     userID,
		JobType:    models.JobTypeExport,
		Source:     source,
		Status:     models.JobStatusPending,
		Priority:   models.JobPriorityNormal,
		TotalItems: count,
	}
	_, err = h.db.NewInsert().Model(job).Exec(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create export job")
	}

	_ = h.pool.Submit(ctx, taskType, map[string]string{
		"job_id": jobID,
	}, 3)

	return c.JSON(http.StatusOK, map[string]any{
		"job_id":          jobID,
		"status":          "pending",
		"message":         fmt.Sprintf("Export job created. Exporting %d games.", count),
		"estimated_items": count,
	})
}

// HandleExportJSON handles POST /api/export/json.
func (h *ExportHandler) HandleExportJSON(c *echo.Context) error {
	return h.handleExport(c, "export_json", models.JobSourceNexorious)
}

// HandleExportCSV handles POST /api/export/csv.
func (h *ExportHandler) HandleExportCSV(c *echo.Context) error {
	return h.handleExport(c, "export_csv", models.JobSourceCSV)
}
```

- [ ] **Step 4: Wire export routes in router.go**

In `internal/api/router.go`, inside the `if db != nil` block, after the import routes:

```go
		// Export routes (all JWT-protected)
		exh := NewExportHandler(db, pool, cfg)
		exportGroup := e.Group("/api/export", auth.JWTMiddleware(cfg.SecretKey, db))
		exportGroup.POST("/json", exh.HandleExportJSON)
		exportGroup.POST("/csv", exh.HandleExportCSV)
		exportGroup.GET("/:id/download", exh.HandleDownload)
```

Note: `HandleDownload` will be a compile error temporarily — add a stub:

In `internal/api/export.go`, add:

```go
// HandleDownload handles GET /api/export/:id/download.
func (h *ExportHandler) HandleDownload(c *echo.Context) error {
	return echo.NewHTTPError(http.StatusNotImplemented, "not yet implemented")
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/api/... -run TestExport -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add internal/api/export.go internal/api/export_test.go internal/api/router.go
git commit -m "feat: add JSON and CSV export handlers with job creation"
```

---

### Task 4: Export Worker Tasks — JSON and CSV

**Files:**
- Create: `internal/worker/tasks/export.go`
- Test: `internal/worker/tasks/export_test.go`
- Modify: `cmd/nexorious/main.go` (register task handlers)

- [ ] **Step 1: Write the failing test for JSON export task**

Create `internal/worker/tasks/export_test.go`:

```go
package tasks_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious-go/internal/db/models"
	"github.com/drzero42/nexorious-go/internal/worker/tasks"
)

func insertGameAndUserGame(t *testing.T, db *bun.DB, userID string, gameID int32, title string) {
	t.Helper()
	ctx := context.Background()
	_, _ = db.NewRaw(
		"INSERT INTO games (id, title) VALUES (?, ?) ON CONFLICT DO NOTHING",
		gameID, title,
	).Exec(ctx)
	_, err := db.NewRaw(
		"INSERT INTO user_games (id, user_id, game_id, play_status, is_loved) VALUES (?, ?, ?, ?, ?)",
		uuid.NewString(), userID, gameID, "completed", false,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insertGameAndUserGame: %v", err)
	}
}

func TestExportJSON_Task(t *testing.T) {
	db := setupTasksTestDB(t)
	ctx := context.Background()
	userID := "user1"
	jobID := uuid.NewString()

	insertTestUser(t, db, userID)
	insertGameAndUserGame(t, db, userID, 1942, "The Witcher 3")

	insertTestJob(t, db, jobID, userID, 1)
	// Override to export type.
	_, _ = db.NewRaw("UPDATE jobs SET job_type = ?, source = ? WHERE id = ?",
		models.JobTypeExport, models.JobSourceNexorious, jobID).Exec(ctx)

	storagePath := t.TempDir()
	handler := tasks.NewExportJSONHandler(db, storagePath)

	task := &models.PendingTask{
		ID:       uuid.NewString(),
		TaskType: "export_json",
		Payload:  mustMarshal(t, map[string]string{"job_id": jobID}),
	}
	err := handler(ctx, task)
	if err != nil {
		t.Fatalf("export_json failed: %v", err)
	}

	// Verify job is completed with file_path set.
	var job models.Job
	db.NewSelect().Model(&job).Where("id = ?", jobID).Scan(ctx)
	if job.Status != models.JobStatusCompleted {
		t.Fatalf("expected completed, got %s", job.Status)
	}
	if job.FilePath == nil {
		t.Fatal("expected file_path to be set")
	}

	// Verify file exists and is valid JSON.
	data, err := os.ReadFile(*job.FilePath)
	if err != nil {
		t.Fatalf("read export file: %v", err)
	}
	var export map[string]any
	if err := json.Unmarshal(data, &export); err != nil {
		t.Fatalf("invalid JSON in export file: %v", err)
	}
	if export["export_version"] != "1.2" {
		t.Fatalf("expected export_version=1.2, got %v", export["export_version"])
	}
	games := export["games"].([]any)
	if len(games) != 1 {
		t.Fatalf("expected 1 game, got %d", len(games))
	}
}

func TestExportCSV_Task(t *testing.T) {
	db := setupTasksTestDB(t)
	ctx := context.Background()
	userID := "user1"
	jobID := uuid.NewString()

	insertTestUser(t, db, userID)
	insertGameAndUserGame(t, db, userID, 1942, "The Witcher 3")

	insertTestJob(t, db, jobID, userID, 1)
	_, _ = db.NewRaw("UPDATE jobs SET job_type = ?, source = ? WHERE id = ?",
		models.JobTypeExport, models.JobSourceCSV, jobID).Exec(ctx)

	storagePath := t.TempDir()
	handler := tasks.NewExportCSVHandler(db, storagePath)

	task := &models.PendingTask{
		ID:       uuid.NewString(),
		TaskType: "export_csv",
		Payload:  mustMarshal(t, map[string]string{"job_id": jobID}),
	}
	err := handler(ctx, task)
	if err != nil {
		t.Fatalf("export_csv failed: %v", err)
	}

	var job models.Job
	db.NewSelect().Model(&job).Where("id = ?", jobID).Scan(ctx)
	if job.Status != models.JobStatusCompleted {
		t.Fatalf("expected completed, got %s", job.Status)
	}
	if job.FilePath == nil {
		t.Fatal("expected file_path to be set")
	}

	data, err := os.ReadFile(*job.FilePath)
	if err != nil {
		t.Fatalf("read csv file: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 { // header + 1 data row
		t.Fatalf("expected 2 lines (header + 1 game), got %d", len(lines))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/worker/tasks/... -run TestExport -v`
Expected: FAIL — `NewExportJSONHandler` not defined.

- [ ] **Step 3: Implement export tasks**

Create `internal/worker/tasks/export.go`:

```go
package tasks

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious-go/internal/db/models"
)

// NexoriousExportData is the top-level JSON export structure.
type NexoriousExportData struct {
	ExportVersion string              `json:"export_version"`
	ExportDate    string              `json:"export_date"`
	UserID        string              `json:"user_id"`
	TotalGames    int                 `json:"total_games"`
	TotalWishlist int                 `json:"total_wishlist"`
	ExportStats   map[string]any      `json:"export_stats"`
	Games         []map[string]any    `json:"games"`
	Wishlist      []any               `json:"wishlist"`
}

// NewExportJSONHandler returns a TaskHandler that exports user games as JSON.
func NewExportJSONHandler(db *bun.DB, storagePath string) func(ctx context.Context, task *models.PendingTask) error {
	return func(ctx context.Context, task *models.PendingTask) error {
		var payload struct {
			JobID string `json:"job_id"`
		}
		if err := json.Unmarshal(task.Payload, &payload); err != nil {
			return fmt.Errorf("export_json: unmarshal payload: %w", err)
		}

		var job models.Job
		if err := db.NewSelect().Model(&job).Where("id = ?", payload.JobID).Scan(ctx); err != nil {
			return fmt.Errorf("export_json: load job: %w", err)
		}

		// Mark processing.
		now := time.Now().UTC()
		_, _ = db.NewRaw("UPDATE jobs SET status = ?, started_at = ? WHERE id = ?",
			models.JobStatusProcessing, now, job.ID).Exec(ctx)

		// Load all user games with relations.
		var userGames []models.UserGame
		err := db.NewSelect().Model(&userGames).
			Where("user_game.user_id = ?", job.UserID).
			Relation("Game").
			Relation("Platforms").
			Relation("Tags").
			Relation("Tags.Tag").
			Scan(ctx)
		if err != nil {
			markJobFailed(ctx, db, job.ID, fmt.Sprintf("query: %v", err))
			return nil
		}

		// Build export data.
		export := buildExportData(job.UserID, userGames)

		// Write to file.
		exportsDir := filepath.Join(storagePath, "exports")
		if err := os.MkdirAll(exportsDir, 0o755); err != nil {
			markJobFailed(ctx, db, job.ID, fmt.Sprintf("mkdir: %v", err))
			return nil
		}
		ts := time.Now().UTC().Format("20060102_150405")
		filePath := filepath.Join(exportsDir, fmt.Sprintf("%s_%s.json", job.UserID, ts))

		data, err := json.MarshalIndent(export, "", "  ")
		if err != nil {
			markJobFailed(ctx, db, job.ID, fmt.Sprintf("marshal: %v", err))
			return nil
		}
		if err := os.WriteFile(filePath, data, 0o644); err != nil {
			markJobFailed(ctx, db, job.ID, fmt.Sprintf("write: %v", err))
			return nil
		}

		// Update job.
		completedAt := time.Now().UTC()
		_, _ = db.NewRaw("UPDATE jobs SET status = ?, file_path = ?, completed_at = ? WHERE id = ?",
			models.JobStatusCompleted, filePath, completedAt, job.ID).Exec(ctx)

		return nil
	}
}

func buildExportData(userID string, userGames []models.UserGame) *NexoriousExportData {
	byStatus := map[string]int{}
	byPlatform := map[string]int{}
	var totalHours float64
	var ratedCount, lovedCount int

	games := make([]map[string]any, 0, len(userGames))
	for _, ug := range userGames {
		if ug.PlayStatus != nil {
			byStatus[*ug.PlayStatus]++
		}
		if ug.HoursPlayed != nil {
			totalHours += *ug.HoursPlayed
		}
		if ug.PersonalRating != nil {
			ratedCount++
		}
		if ug.IsLoved {
			lovedCount++
		}

		game := map[string]any{
			"play_status":     ug.PlayStatus,
			"personal_rating": ug.PersonalRating,
			"is_loved":        ug.IsLoved,
			"hours_played":    ug.HoursPlayed,
			"personal_notes":  ug.PersonalNotes,
			"created_at":      ug.CreatedAt.UTC().Format(time.RFC3339),
			"updated_at":      ug.UpdatedAt.UTC().Format(time.RFC3339),
		}

		if ug.Game != nil {
			game["igdb_id"] = ug.Game.ID
			game["title"] = ug.Game.Title
			if ug.Game.ReleaseDate != nil {
				game["release_year"] = ug.Game.ReleaseDate.Year()
			}
		}

		// Platforms.
		platforms := make([]map[string]any, 0, len(ug.Platforms))
		for _, p := range ug.Platforms {
			if p.Platform != nil {
				byPlatform[*p.Platform]++
			}
			pe := map[string]any{
				"platform_id":      p.Platform,
				"storefront_id":    p.Storefront,
				"store_game_id":    p.StoreGameID,
				"store_url":        p.StoreUrl,
				"is_available":     p.IsAvailable,
				"hours_played":     p.HoursPlayed,
				"ownership_status": p.OwnershipStatus,
				"acquired_date":    nil,
			}
			if p.AcquiredDate != nil {
				pe["acquired_date"] = p.AcquiredDate.Format("2006-01-02")
			}
			platforms = append(platforms, pe)
		}
		game["platforms"] = platforms

		// Tags.
		tags := make([]map[string]any, 0, len(ug.Tags))
		for _, t := range ug.Tags {
			if t.Tag != nil {
				tags = append(tags, map[string]any{
					"name":  t.Tag.Name,
					"color": t.Tag.Color,
				})
			}
		}
		game["tags"] = tags

		games = append(games, game)
	}

	return &NexoriousExportData{
		ExportVersion: "1.2",
		ExportDate:    time.Now().UTC().Format(time.RFC3339),
		UserID:        userID,
		TotalGames:    len(games),
		TotalWishlist: 0,
		ExportStats: map[string]any{
			"by_status":   byStatus,
			"by_platform": byPlatform,
			"total_hours": totalHours,
			"rated_count": ratedCount,
			"loved_count": lovedCount,
		},
		Games:    games,
		Wishlist: []any{},
	}
}

// NewExportCSVHandler returns a TaskHandler that exports user games as CSV.
func NewExportCSVHandler(db *bun.DB, storagePath string) func(ctx context.Context, task *models.PendingTask) error {
	return func(ctx context.Context, task *models.PendingTask) error {
		var payload struct {
			JobID string `json:"job_id"`
		}
		if err := json.Unmarshal(task.Payload, &payload); err != nil {
			return fmt.Errorf("export_csv: unmarshal payload: %w", err)
		}

		var job models.Job
		if err := db.NewSelect().Model(&job).Where("id = ?", payload.JobID).Scan(ctx); err != nil {
			return fmt.Errorf("export_csv: load job: %w", err)
		}

		now := time.Now().UTC()
		_, _ = db.NewRaw("UPDATE jobs SET status = ?, started_at = ? WHERE id = ?",
			models.JobStatusProcessing, now, job.ID).Exec(ctx)

		var userGames []models.UserGame
		err := db.NewSelect().Model(&userGames).
			Where("user_game.user_id = ?", job.UserID).
			Relation("Game").
			Relation("Platforms").
			Relation("Tags").
			Relation("Tags.Tag").
			Scan(ctx)
		if err != nil {
			markJobFailed(ctx, db, job.ID, fmt.Sprintf("query: %v", err))
			return nil
		}

		exportsDir := filepath.Join(storagePath, "exports")
		if err := os.MkdirAll(exportsDir, 0o755); err != nil {
			markJobFailed(ctx, db, job.ID, fmt.Sprintf("mkdir: %v", err))
			return nil
		}
		ts := time.Now().UTC().Format("20060102_150405")
		filePath := filepath.Join(exportsDir, fmt.Sprintf("%s_%s.csv", job.UserID, ts))

		f, err := os.Create(filePath)
		if err != nil {
			markJobFailed(ctx, db, job.ID, fmt.Sprintf("create file: %v", err))
			return nil
		}
		defer f.Close()

		w := csv.NewWriter(f)
		defer w.Flush()

		header := []string{"title", "igdb_id", "play_status", "personal_rating", "is_loved",
			"hours_played", "personal_notes", "platforms", "tags", "release_year", "created_at", "updated_at"}
		w.Write(header)

		for _, ug := range userGames {
			title := ""
			igdbID := ""
			releaseYear := ""
			if ug.Game != nil {
				title = ug.Game.Title
				igdbID = fmt.Sprintf("%d", ug.Game.ID)
				if ug.Game.ReleaseDate != nil {
					releaseYear = fmt.Sprintf("%d", ug.Game.ReleaseDate.Year())
				}
			}

			playStatus := ""
			if ug.PlayStatus != nil {
				playStatus = *ug.PlayStatus
			}
			personalRating := ""
			if ug.PersonalRating != nil {
				personalRating = fmt.Sprintf("%d", *ug.PersonalRating)
			}
			isLoved := "false"
			if ug.IsLoved {
				isLoved = "true"
			}
			hoursPlayed := ""
			if ug.HoursPlayed != nil {
				hoursPlayed = fmt.Sprintf("%.1f", *ug.HoursPlayed)
			}
			personalNotes := ""
			if ug.PersonalNotes != nil {
				personalNotes = *ug.PersonalNotes
			}

			var platSlugs []string
			for _, p := range ug.Platforms {
				if p.Platform != nil {
					platSlugs = append(platSlugs, *p.Platform)
				}
			}
			var tagNames []string
			for _, t := range ug.Tags {
				if t.Tag != nil {
					tagNames = append(tagNames, t.Tag.Name)
				}
			}

			row := []string{
				title, igdbID, playStatus, personalRating, isLoved,
				hoursPlayed, personalNotes,
				strings.Join(platSlugs, ";"),
				strings.Join(tagNames, ";"),
				releaseYear,
				ug.CreatedAt.UTC().Format(time.RFC3339),
				ug.UpdatedAt.UTC().Format(time.RFC3339),
			}
			w.Write(row)
		}
		w.Flush()

		completedAt := time.Now().UTC()
		_, _ = db.NewRaw("UPDATE jobs SET status = ?, file_path = ?, completed_at = ? WHERE id = ?",
			models.JobStatusCompleted, filePath, completedAt, job.ID).Exec(ctx)

		return nil
	}
}

func markJobFailed(ctx context.Context, db *bun.DB, jobID string, errMsg string) {
	now := time.Now().UTC()
	_, err := db.NewRaw(
		"UPDATE jobs SET status = ?, error_message = ?, completed_at = ? WHERE id = ?",
		models.JobStatusFailed, errMsg, now, jobID,
	).Exec(ctx)
	if err != nil {
		slog.Error("export: failed to mark job failed", "job_id", jobID, "err", err)
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/worker/tasks/... -run TestExport -v`
Expected: All PASS

- [ ] **Step 5: Register export handlers in main.go**

In `cmd/nexorious/main.go`, after the import_item registration:

```go
pool.Register("export_json", tasks.NewExportJSONHandler(db, cfg.StoragePath))
pool.Register("export_csv", tasks.NewExportCSVHandler(db, cfg.StoragePath))
```

- [ ] **Step 6: Build and verify**

Run: `go build ./...`
Expected: No errors.

- [ ] **Step 7: Commit**

```bash
git add internal/worker/tasks/export.go internal/worker/tasks/export_test.go cmd/nexorious/main.go
git commit -m "feat: add JSON and CSV export worker tasks"
```

---

### Task 5: Download Handler

**Files:**
- Modify: `internal/api/export.go` (replace stub)
- Modify: `internal/api/export_test.go` (add download tests)

- [ ] **Step 1: Write failing download tests**

Add to `internal/api/export_test.go`:

```go
func TestDownload_NotFound(t *testing.T) {
	db := setupImportTestDB(t)
	cfg := importTestCfg()
	pool := worker.NewPool(db)
	insertAuthTestUser(t, db, "user1", "admin", "password123", true, false)
	token := issueTestToken(t, cfg, "user1")
	handler := importTestEcho(t, db, cfg, pool)

	req := httptest.NewRequest(http.MethodGet, "/api/export/nonexistent/download", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestDownload_NotExportJob(t *testing.T) {
	db := setupImportTestDB(t)
	cfg := importTestCfg()
	pool := worker.NewPool(db)
	insertAuthTestUser(t, db, "user1", "admin", "password123", true, false)
	token := issueTestToken(t, cfg, "user1")
	handler := importTestEcho(t, db, cfg, pool)

	// Create an import job (not export).
	jobID := "job-import-1"
	_, _ = db.ExecContext(t.Context(),
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items)
		 VALUES (?, 'user1', 'import', 'nexorious', 'completed', 'normal', 1)`, jobID)

	req := httptest.NewRequest(http.MethodGet, "/api/export/"+jobID+"/download", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDownload_NotCompleted(t *testing.T) {
	db := setupImportTestDB(t)
	cfg := importTestCfg()
	pool := worker.NewPool(db)
	insertAuthTestUser(t, db, "user1", "admin", "password123", true, false)
	token := issueTestToken(t, cfg, "user1")
	handler := importTestEcho(t, db, cfg, pool)

	jobID := "job-export-pending"
	_, _ = db.ExecContext(t.Context(),
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items)
		 VALUES (?, 'user1', 'export', 'nexorious', 'pending', 'normal', 1)`, jobID)

	req := httptest.NewRequest(http.MethodGet, "/api/export/"+jobID+"/download", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDownload_Success(t *testing.T) {
	db := setupImportTestDB(t)
	cfg := importTestCfg()
	cfg.StoragePath = t.TempDir()
	pool := worker.NewPool(db)
	insertAuthTestUser(t, db, "user1", "admin", "password123", true, false)
	token := issueTestToken(t, cfg, "user1")
	handler := importTestEcho(t, db, cfg, pool)

	// Create a file.
	exportsDir := filepath.Join(cfg.StoragePath, "exports")
	os.MkdirAll(exportsDir, 0o755)
	filePath := filepath.Join(exportsDir, "test_export.json")
	os.WriteFile(filePath, []byte(`{"export_version":"1.2"}`), 0o644)

	now := time.Now().UTC()
	jobID := "job-export-done"
	_, _ = db.ExecContext(t.Context(),
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, file_path, completed_at)
		 VALUES (?, 'user1', 'export', 'nexorious', 'completed', 'normal', 1, ?, ?)`,
		jobID, filePath, now)

	req := httptest.NewRequest(http.MethodGet, "/api/export/"+jobID+"/download", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "json") {
		t.Fatalf("expected json content type, got %s", ct)
	}
}
```

Add `"os"`, `"path/filepath"`, `"strings"`, `"time"` to imports.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/api/... -run TestDownload -v`
Expected: FAIL — `HandleDownload` returns 501.

- [ ] **Step 3: Implement HandleDownload**

Replace the `HandleDownload` stub in `internal/api/export.go`:

```go
// HandleDownload handles GET /api/export/:id/download.
func (h *ExportHandler) HandleDownload(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	jobID := c.Param("id")
	ctx := context.Background()

	var job models.Job
	err := h.db.NewRaw("SELECT * FROM jobs WHERE id = ? AND user_id = ?", jobID, userID).
		Scan(ctx, &job)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get job")
	}

	if job.JobType != models.JobTypeExport {
		return echo.NewHTTPError(http.StatusBadRequest, "not an export job")
	}
	if job.Status != models.JobStatusCompleted {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("export not ready, current status: %s", job.Status))
	}
	if job.FilePath == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "export file path not set")
	}

	// Check file exists.
	if _, err := os.Stat(*job.FilePath); os.IsNotExist(err) {
		return echo.NewHTTPError(http.StatusGone, "export file no longer available")
	}

	// Check expiration (24 hours).
	if job.CompletedAt != nil && time.Since(*job.CompletedAt) > 24*time.Hour {
		_ = os.Remove(*job.FilePath)
		return echo.NewHTTPError(http.StatusGone, "export file has expired")
	}

	// Determine content type and filename.
	ts := time.Now().UTC().Format("20060102_150405")
	contentType := "application/json"
	filename := fmt.Sprintf("nexorious_export_%s.json", ts)
	if strings.HasSuffix(*job.FilePath, ".csv") {
		contentType = "text/csv"
		filename = fmt.Sprintf("nexorious_export_%s.csv", ts)
	}

	c.Response().Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	return c.File(*job.FilePath, contentType)
}
```

Add `"database/sql"`, `"errors"`, `"fmt"`, `"os"`, `"strings"`, `"time"` to the export.go imports.

Note: Echo v5's `c.File()` takes `(path, contentType)`. If v5 doesn't support the second arg, use `c.Attachment(*job.FilePath, filename)` or serve manually with `http.ServeFile`. Verify the exact Echo v5 API at implementation time — if `c.File` only takes `path`, use:

```go
c.Response().Header().Set("Content-Type", contentType)
c.Response().Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
return c.File(*job.FilePath)
```

- [ ] **Step 4: Run all download tests**

Run: `go test ./internal/api/... -run TestDownload -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/api/export.go internal/api/export_test.go
git commit -m "feat: add export download handler with ownership and expiry checks"
```

---

### Task 6: CleanupExports Implementation & Slumber Collection

**Files:**
- Modify: `internal/scheduler/scheduler.go` (replace CleanupExports placeholder)
- Modify: `slumber.yaml` (add import/export requests)

- [ ] **Step 1: Implement CleanupExports**

In `internal/scheduler/scheduler.go`, replace the `CleanupExports` function:

```go
// CleanupExports deletes expired export files (>24h) and marks their jobs.
func CleanupExports(ctx context.Context, db *bun.DB) {
	cutoff := time.Now().UTC().Add(-24 * time.Hour)

	var jobs []struct {
		ID       string  `bun:"id"`
		FilePath *string `bun:"file_path"`
	}
	err := db.NewRaw(`
		SELECT id, file_path FROM jobs
		WHERE job_type = 'export' AND status = 'completed'
		  AND file_path IS NOT NULL AND completed_at < ?`,
		cutoff,
	).Scan(ctx, &jobs)
	if err != nil {
		slog.Error("cleanup: failed to query expired exports", "err", err)
		return
	}

	for _, j := range jobs {
		if j.FilePath != nil {
			if err := os.Remove(*j.FilePath); err != nil && !os.IsNotExist(err) {
				slog.Warn("cleanup: failed to remove export file", "path", *j.FilePath, "err", err)
			}
		}
	}

	if len(jobs) > 0 {
		ids := make([]string, len(jobs))
		for i, j := range jobs {
			ids[i] = j.ID
		}
		_, _ = db.NewRaw(
			`UPDATE jobs SET file_path = NULL WHERE id IN (?)`,
			bun.In(ids),
		).Exec(ctx)
		slog.Info("cleanup: cleaned expired exports", "count", len(jobs))
	}
}
```

Add `"os"` to the scheduler.go imports.

- [ ] **Step 2: Build to verify**

Run: `go build ./...`
Expected: No errors.

- [ ] **Step 3: Add slumber requests**

Append to `slumber.yaml` inside the `requests:` block (alphabetical order — after existing folders):

```yaml
  export:
    name: Export
    requests:
      export_json:
        name: Export JSON
        method: POST
        url: "{{base_url}}/api/export/json"
        <<: *.authenticated

      export_csv:
        name: Export CSV
        method: POST
        url: "{{base_url}}/api/export/csv"
        <<: *.authenticated

      download:
        name: Download Export
        method: GET
        url: "{{base_url}}/api/export/{{export_job_id}}/download"
        <<: *.authenticated

  import:
    name: Import
    requests:
      nexorious:
        name: Import Nexorious
        method: POST
        url: "{{base_url}}/api/import/nexorious"
        <<: *.authenticated
        body:
          type: multipart_form
          data:
            file:
              type: file
              data: ./test_export.json
```

Note: The exact slumber YAML syntax for anchors (`<<: *.authenticated`) and file uploads may differ — verify against the existing patterns in slumber.yaml at implementation time. The key fields are the `authentication: type: bearer` block with the login token template.

- [ ] **Step 4: Verify slumber collection loads**

Run: `slumber collection` (or `slumber` and check the TUI loads)
Expected: No errors.

- [ ] **Step 5: Commit**

```bash
git add internal/scheduler/scheduler.go slumber.yaml
git commit -m "feat: implement CleanupExports and add import/export slumber requests"
```

---

### Task 7: Update retryTaskType for Import

**Files:**
- Modify: `internal/api/jobs.go`

- [ ] **Step 1: Write a test that verifies retry works for import jobs**

This is a quick behavioral verification. The existing `retryTaskType` function maps `"import"` to `"process_import_item"`, but our actual task type is `"import_item"`. Fix it.

- [ ] **Step 2: Update retryTaskType**

In `internal/api/jobs.go`, update the `retryTaskType` function:

```go
func retryTaskType(jobType string) string {
	switch jobType {
	case models.JobTypeSync:
		return "process_sync_item"
	case models.JobTypeImport:
		return "import_item"
	case models.JobTypeMetadataRefresh:
		return "metadata_refresh_process"
	default:
		return "import_item"
	}
}
```

- [ ] **Step 3: Build and run all tests**

Run: `go build ./... && go test ./...`
Expected: All pass, no build errors.

- [ ] **Step 4: Commit**

```bash
git add internal/api/jobs.go
git commit -m "fix: map import job retry to correct import_item task type"
```

---

### Task 8: Final Integration Verification

- [ ] **Step 1: Run full test suite**

Run: `go test ./... -v`
Expected: All tests pass.

- [ ] **Step 2: Run linter**

Run: `golangci-lint run`
Expected: No errors.

- [ ] **Step 3: Build binary**

Run: `make build`
Expected: Clean build.

- [ ] **Step 4: Final commit if any lint fixes were needed**

```bash
git add -A
git commit -m "chore: lint fixes for import/export"
```
