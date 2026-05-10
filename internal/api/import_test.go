package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/drzero42/nexorious-go/internal/worker"
)

// ─── Import test helpers ──────────────────────────────────────────────────────

// postMultipartFile posts a multipart/form-data request with a file field.
func postMultipartFile(t *testing.T, handler interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, path string, filename string, fileContent []byte, accessToken string) *httptest.ResponseRecorder {
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
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
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

// ─── Import tests ─────────────────────────────────────────────────────────────

func TestImportNexorious_NoFile(t *testing.T) {
	db := setupAuthTestDB(t)
	pool := worker.NewPool(db)
	cfg := testCfg()
	e := newTestEchoPool(t, db, cfg, pool)

	_, token := setupTagUser(t, db, e, "imp-nofile")

	// Post multipart without any file field.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/import/nexorious", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body)
	}
}

func TestImportNexorious_InvalidJSON(t *testing.T) {
	db := setupAuthTestDB(t)
	pool := worker.NewPool(db)
	cfg := testCfg()
	e := newTestEchoPool(t, db, cfg, pool)

	_, token := setupTagUser(t, db, e, "imp-badjson")

	rec := postMultipartFile(t, e, "/api/import/nexorious", "export.json", []byte("not valid json{{{"), token)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body)
	}
}

func TestImportNexorious_WrongVersion(t *testing.T) {
	db := setupAuthTestDB(t)
	pool := worker.NewPool(db)
	cfg := testCfg()
	e := newTestEchoPool(t, db, cfg, pool)

	_, token := setupTagUser(t, db, e, "imp-wrongver")

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
	db := setupAuthTestDB(t)
	pool := worker.NewPool(db)
	cfg := testCfg()
	e := newTestEchoPool(t, db, cfg, pool)

	_, token := setupTagUser(t, db, e, "imp-empty")

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
	db := setupAuthTestDB(t)
	pool := worker.NewPool(db)
	cfg := testCfg()
	e := newTestEchoPool(t, db, cfg, pool)

	_, token := setupTagUser(t, db, e, "imp-success")

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
	db := setupAuthTestDB(t)
	pool := worker.NewPool(db)
	cfg := testCfg()
	e := newTestEchoPool(t, db, cfg, pool)

	_, token := setupTagUser(t, db, e, "imp-conflict")

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
