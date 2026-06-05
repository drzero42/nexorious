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
		games[i] = gameEntry{
			IgdbID: &id,
			Title:  fmt.Sprintf("Game %d", i+1),
		}
	}

	export := map[string]any{
		"export_version": "1.2",
		"games":          games,
	}

	data, err := json.Marshal(export)
	if err != nil {
		t.Fatalf("marshal export: %v", err)
	}
	return data
}

// darkadiaTestIGDB builds an *igdb.Client for Darkadia handler tests.
// When configured=true it has non-empty credentials (Configured()==true);
// when false the credentials are empty (Configured()==false).
// No network call is made at upload time.
func darkadiaTestIGDB(configured bool) *igdb.Client {
	cfg := &config.Config{}
	if configured {
		cfg.IGDBClientID = "test-id"
		cfg.IGDBClientSecret = "test-secret"
	}
	return igdb.NewClient(cfg, nil)
}

// newTestEchoConfiguredIGDB returns an Echo instance wired with the provided
// igdb.Client and a real River client. Used by Darkadia tests that need the
// handler's IGDB guard to pass.
func newTestEchoConfiguredIGDB(t *testing.T, db *bun.DB, cfg *config.Config, igdbClient *igdb.Client) interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
} {
	t.Helper()
	m := migrate.NewMigratorForTest(migrate.AppStateReady)
	rc := newTestRiverClient(t)
	return api.New(testEncrypter, cfg, m, db, "", igdbClient, nil, nil, "dev", "unknown", rc)
}

// canonicalDarkadiaCSV returns a minimal valid Darkadia CSV with the given
// game titles (one named row each, no copy rows).
func canonicalDarkadiaCSV(titles ...string) []byte {
	const hdr = `Name,Added,Loved,Owned,Played,Playing,Finished,Mastered,Dominated,Shelved,Rating,"Copy label","Copy Release","Copy platform","Copy media","Copy media other","Copy source","Copy source other","Copy purchase date","Copy box","Copy box condition","Copy box notes","Copy manual","Copy manual condition","Copy manual notes","Copy complete","Copy complete notes",Platforms,Notes`
	var buf bytes.Buffer
	buf.WriteString(hdr + "\n")
	for _, title := range titles {
		// 29 columns: Name followed by 28 empty fields.
		buf.WriteString(title + strings.Repeat(",", 28) + "\n")
	}
	return buf.Bytes()
}

// ─── Import tests ─────────────────────────────────────────────────────────────

func TestImportNexorious_NoFile(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoPool(t, testDB, cfg)

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
	e := newTestEchoPool(t, testDB, cfg)

	_, token := setupTagUser(t, testDB, e, "imp-badjson")

	rec := postMultipartFile(t, e, "/api/import/nexorious", "export.json", []byte("not valid json{{{"), token)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body)
	}
}

func TestImportNexorious_WrongVersion(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoPool(t, testDB, cfg)

	_, token := setupTagUser(t, testDB, e, "imp-wrongver")

	wrongVersion := map[string]any{
		"export_version": "1.0",
		"games":          []map[string]any{{"title": "Game 1"}},
	}
	data, _ := json.Marshal(wrongVersion)

	rec := postMultipartFile(t, e, "/api/import/nexorious", "export.json", data, token)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body)
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	wantMsg := "Unsupported export version. Only version 1.2 is supported."
	if resp["message"] != wantMsg {
		t.Fatalf("error message = %q, want %q", resp["message"], wantMsg)
	}
}

func TestImportNexorious_EmptyGames(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoPool(t, testDB, cfg)

	_, token := setupTagUser(t, testDB, e, "imp-empty")

	emptyGames := map[string]any{
		"export_version": "1.2",
		"games":          []map[string]any{},
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
	e := newTestEchoPool(t, testDB, cfg)

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
	e := newTestEchoPool(t, testDB, cfg)

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
	e := newTestEchoPool(t, testDB, cfg)

	_, token := setupTagUser(t, testDB, e, "imp-malformed")

	// Build an export where the second game entry is a JSON string, not an object.
	// The outer json.Unmarshal into []json.RawMessage succeeds (each element is a
	// valid JSON token), but json.Unmarshal into the game-fields struct fails for a
	// string value, which is the malformed-record path we want to exercise.
	data := []byte(`{"export_version":"1.2","games":[{"igdb_id":1,"title":"Good Game"},"not-an-object",{"igdb_id":3,"title":"Another Good Game"}]}`)

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

// ─── Darkadia import tests ────────────────────────────────────────────────────

func TestImportDarkadia_RefusesWhenIGDBNotConfigured(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	// newTestEchoPool passes nil as igdbClient to api.New, so the handler's
	// IGDB guard fires — which is what this test exercises.
	e := newTestEchoPool(t, testDB, cfg)

	_, token := setupTagUser(t, testDB, e, "dark-noigdb")

	csvData := canonicalDarkadiaCSV("Portal")
	rec := postMultipartFile(t, e, "/api/import/darkadia", "darkadia.csv", csvData, token)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body)
	}
	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if !strings.Contains(resp["message"], "IGDB") {
		t.Fatalf("error message %q does not mention IGDB", resp["message"])
	}
}

func TestImportDarkadia_RejectsNonDarkadiaHeader(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, darkadiaTestIGDB(true))

	_, token := setupTagUser(t, testDB, e, "dark-badheader")

	badCSV := []byte("a,b,c\n1,2,3\n")
	rec := postMultipartFile(t, e, "/api/import/darkadia", "darkadia.csv", badCSV, token)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body)
	}
	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if !strings.Contains(resp["message"], "Darkadia") {
		t.Fatalf("error message %q does not mention Darkadia", resp["message"])
	}
}

func TestImportDarkadia_CreatesJobAndItems(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, darkadiaTestIGDB(true))

	_, token := setupTagUser(t, testDB, e, "dark-success")

	csvData := canonicalDarkadiaCSV("Portal", "Half-Life 2")
	rec := postMultipartFile(t, e, "/api/import/darkadia", "darkadia.csv", csvData, token)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	jobID, _ := resp["job_id"].(string)
	if jobID == "" {
		t.Fatalf("expected non-empty job_id, got %v", resp["job_id"])
	}
	if resp["source"] != "darkadia" {
		t.Fatalf("source = %v, want darkadia", resp["source"])
	}
	totalItems, ok := resp["total_items"].(float64)
	if !ok || int(totalItems) != 2 {
		t.Fatalf("total_items = %v, want 2", resp["total_items"])
	}

	// Verify the job row in the DB.
	ctx := context.Background()
	var dbTotalItems int
	var dbSource string
	if err := testDB.NewRaw(
		`SELECT total_items, source FROM jobs WHERE id = ?`, jobID,
	).Scan(ctx, &dbTotalItems, &dbSource); err != nil {
		t.Fatalf("select job: %v", err)
	}
	if dbSource != "darkadia" {
		t.Errorf("job.source = %q, want darkadia", dbSource)
	}
	if dbTotalItems != 2 {
		t.Errorf("job.total_items = %d, want 2", dbTotalItems)
	}

	// Verify 2 job_items were created.
	var itemCount int
	if err := testDB.NewRaw(
		`SELECT COUNT(*) FROM job_items WHERE job_id = ?`, jobID,
	).Scan(ctx, &itemCount); err != nil {
		t.Fatalf("count job_items: %v", err)
	}
	if itemCount != 2 {
		t.Errorf("job_items count = %d, want 2", itemCount)
	}
}
