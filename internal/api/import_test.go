package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/api"
	"github.com/drzero42/nexorious/internal/config"
	"github.com/drzero42/nexorious/internal/migrate"
	"github.com/drzero42/nexorious/internal/services/igdb"
)

// ─── Import test helpers ──────────────────────────────────────────────────────

// postMultipartFile posts a multipart/form-data request with a file field.
func postMultipartFile(t *testing.T, handler interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, path string, filename string, fileContent []byte, sessionID string) *httptest.ResponseRecorder {
	t.Helper()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	if fileContent != nil {
		fw, err := mw.CreateFormFile("file", filename)
		if err != nil {
			t.Fatalf("createFormFile: %v", err)
		}
		if _, err := io.Copy(fw, bytes.NewReader(fileContent)); err != nil {
			t.Fatalf("copy file: %v", err)
		}
	}

	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if sessionID != "" {
		req.AddCookie(&http.Cookie{Name: "session_id", Value: sessionID})
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// validExportJSON returns a valid nexorious export JSON with n game entries.
func validExportJSON(t *testing.T, n int) []byte {
	t.Helper()

	type gameEntry struct {
		IgdbID *int   `json:"igdb_id,omitempty"`
		Title  string `json:"title"`
	}

	games := make([]gameEntry, n)
	for i := range games {
		id := i + 1
		games[i] = gameEntry{IgdbID: &id, Title: fmt.Sprintf("Game %d", i+1)}
	}

	export := map[string]any{
		"format":  "nexorious-library",
		"version": "2.0",
		"games":   games,
	}

	data, err := json.Marshal(export)
	if err != nil {
		t.Fatalf("marshal export: %v", err)
	}
	return data
}

// testIGDBClient builds an *igdb.Client for import handler tests.
// configured=true gives non-empty credentials so Configured()==true, with no network call at upload time.
// configured=false leaves credentials empty (Configured()==false).
func testIGDBClient(configured bool) *igdb.Client {
	cfg := &config.Config{}
	if configured {
		cfg.IGDBClientID = "test-id"
		cfg.IGDBClientSecret = "test-secret"
	}
	return igdb.NewClient(cfg, nil)
}

// newTestEchoConfiguredIGDB returns an Echo instance wired with the provided
// igdb.Client and a real River client. Used by import tests that need the
// handler's IGDB guard to pass.
func newTestEchoConfiguredIGDB(t *testing.T, db *bun.DB, cfg *config.Config, igdbClient *igdb.Client) interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
} {
	t.Helper()
	m := migrate.NewMigratorForTest(migrate.AppStateReady)
	rc := newTestRiverClient(t)
	return api.New(testEncrypter, cfg, m, db, "", igdbClient, nil, nil, "dev", "unknown", nil, rc)
}

// ─── Import tests ─────────────────────────────────────────────────────────────

func TestImportNexorious_NoFile(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))

	_, token := setupTagUser(t, testDB, e, "imp-nofile")

	// Post multipart without any file field.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/import/nexorious", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: "session_id", Value: token})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body)
	}
}

func TestImportNexorious_InvalidJSON(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))

	_, token := setupTagUser(t, testDB, e, "imp-badjson")

	rec := postMultipartFile(t, e, "/api/import/nexorious", "export.json", []byte("not valid json{{{"), token)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body)
	}
}

func TestImportNexorious_WrongVersion(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))

	_, token := setupTagUser(t, testDB, e, "imp-wrongver")

	legacy := map[string]any{
		"export_version": "1.2",
		"games":          []map[string]any{{"title": "Game 1"}},
	}
	data, _ := json.Marshal(legacy)

	rec := postMultipartFile(t, e, "/api/import/nexorious", "export.json", data, token)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body)
	}
	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if !strings.Contains(resp["message"], "2.0") {
		t.Fatalf("error message = %q, want it to mention version 2.0", resp["message"])
	}
}

func TestImportNexorious_EmptyGames(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))

	_, token := setupTagUser(t, testDB, e, "imp-empty")

	emptyGames := map[string]any{
		"format":  "nexorious-library",
		"version": "2.0",
		"games":   []map[string]any{},
	}
	data, _ := json.Marshal(emptyGames)

	rec := postMultipartFile(t, e, "/api/import/nexorious", "export.json", data, token)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body)
	}
}

func TestImportNexorious_Success(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))

	_, token := setupTagUser(t, testDB, e, "imp-success")

	data := validExportJSON(t, 3)
	rec := postMultipartFile(t, e, "/api/import/nexorious", "export.json", data, token)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if resp["job_id"] == nil || resp["job_id"] == "" {
		t.Fatalf("expected non-empty job_id, got %v", resp["job_id"])
	}
	if resp["source"] != "nexorious" {
		t.Fatalf("source = %v, want nexorious", resp["source"])
	}
	if resp["status"] != "pending" {
		t.Fatalf("status = %v, want pending", resp["status"])
	}
	totalItems, ok := resp["total_items"].(float64)
	if !ok || int(totalItems) != 3 {
		t.Fatalf("total_items = %v, want 3", resp["total_items"])
	}
}

func TestImportNexorious_Conflict(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))

	_, token := setupTagUser(t, testDB, e, "imp-conflict")

	data := validExportJSON(t, 2)

	// First upload should succeed.
	rec1 := postMultipartFile(t, e, "/api/import/nexorious", "export.json", data, token)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first upload: status = %d, want 200; body: %s", rec1.Code, rec1.Body)
	}

	// Second upload should conflict because the first job is still pending.
	rec2 := postMultipartFile(t, e, "/api/import/nexorious", "export.json", data, token)
	if rec2.Code != http.StatusConflict {
		t.Fatalf("second upload: status = %d, want 409; body: %s", rec2.Code, rec2.Body)
	}
}

func TestImportNexorious_MalformedRecord(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))

	_, token := setupTagUser(t, testDB, e, "imp-malformed")

	// Build an export where the second game entry is a JSON string, not an object.
	// The outer json.Unmarshal into []json.RawMessage succeeds (each element is a
	// valid JSON token), but json.Unmarshal into the game-fields struct fails for a
	// string value, which is the malformed-record path we want to exercise.
	data := []byte(`{"format":"nexorious-library","version":"2.0","games":[{"igdb_id":1,"title":"Good Game"},"not-an-object",{"igdb_id":3,"title":"Another Good Game"}]}`)

	rec := postMultipartFile(t, e, "/api/import/nexorious", "export.json", data, token)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	// skipped_count must be 1 (the malformed record).
	skippedCount, ok := resp["skipped_count"].(float64)
	if !ok || int(skippedCount) != 1 {
		t.Fatalf("skipped_count = %v, want 1", resp["skipped_count"])
	}

	// total_items must reflect the 2 good records only.
	totalItems, ok := resp["total_items"].(float64)
	if !ok || int(totalItems) != 2 {
		t.Fatalf("total_items = %v, want 2", resp["total_items"])
	}

	// Only 2 job_items should exist in the DB.
	jobID, _ := resp["job_id"].(string)
	var count int
	if err := testDB.NewRaw(
		`SELECT COUNT(*) FROM job_items WHERE job_id = ?`, jobID,
	).Scan(context.Background(), &count); err != nil {
		t.Fatalf("count job_items: %v", err)
	}
	if count != 2 {
		t.Errorf("job_items count = %d, want 2", count)
	}
}

func TestImportNexorious_RefusesWhenIGDBNotConfigured(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	// newTestEchoPool wires a nil IGDB client, so the handler's guard fires.
	e := newTestEchoPool(t, testDB, cfg)

	_, token := setupTagUser(t, testDB, e, "imp-noigdb")

	data := validExportJSON(t, 1)
	rec := postMultipartFile(t, e, "/api/import/nexorious", "export.json", data, token)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body)
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if !strings.Contains(resp["message"], "IGDB") {
		t.Fatalf("error message = %q, want it to mention IGDB", resp["message"])
	}
}

// An unregistered source slug has no route — only registry slugs are wired in
// router.go via importsource.All() — so the upload never reaches the generic
// handler. Echo returns 405 (not 404) for an unmatched leaf under a group that
// has sibling routes; either status proves the source was not routed/imported.
func TestImportSource_UnregisteredSlugNotRouted(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))

	_, token := setupTagUser(t, testDB, e, "import-unknown")

	rec := postMultipartFile(t, e, "/api/import/grouvee", "x.csv", []byte("a,b\n1,2\n"), token)

	if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 404/405 for unregistered source; body: %s", rec.Code, rec.Body)
	}

	// And no import job was created for the bogus source.
	var jobCount int
	if err := testDB.NewRaw(
		`SELECT COUNT(*) FROM jobs WHERE source = 'grouvee'`,
	).Scan(context.Background(), &jobCount); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if jobCount != 0 {
		t.Errorf("jobs for unregistered source = %d, want 0", jobCount)
	}
}
