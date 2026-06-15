package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// postCSVImport posts a multipart request with a "file" field and, when mapping
// is non-empty, a "mapping" form field. Used by the /api/import/csv tests.
func postCSVImport(t *testing.T, handler interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, path, filename string, fileContent []byte, mapping, sessionID string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if fileContent != nil {
		fw, err := mw.CreateFormFile("file", filename)
		if err != nil {
			t.Fatalf("createFormFile: %v", err)
		}
		if _, err := fw.Write(fileContent); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}
	if mapping != "" {
		if err := mw.WriteField("mapping", mapping); err != nil {
			t.Fatalf("write mapping: %v", err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
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

func TestImportCSVInspect_ReturnsHeadersDistinctAndCount(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))
	_, token := setupTagUser(t, testDB, e, "csv-inspect")

	csvData := []byte("Name,Status\nCeleste,Beaten\nHades,Playing\nTunic,Beaten\nDanger, \n")
	rec := postMultipartFile(t, e, "/api/import/csv/inspect", "lib.csv", csvData, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}

	var resp struct {
		Headers  []string `json:"headers"`
		RowCount int      `json:"row_count"`
		Columns  []struct {
			Name              string   `json:"name"`
			DistinctValues    []string `json:"distinct_values"`
			DistinctTruncated bool     `json:"distinct_truncated"`
		} `json:"columns"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Headers) != 2 || resp.Headers[0] != "Name" || resp.Headers[1] != "Status" {
		t.Fatalf("headers = %v", resp.Headers)
	}
	if resp.RowCount != 4 {
		t.Errorf("row_count = %d, want 4", resp.RowCount)
	}
	var statusCol []string
	for _, c := range resp.Columns {
		if c.Name == "Status" {
			statusCol = c.DistinctValues
		}
	}
	if len(statusCol) != 2 {
		t.Errorf("Status distinct = %v, want 2 (Beaten, Playing)", statusCol)
	}
}

func TestImportCSVInspect_CapsDistinctValues(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))
	_, token := setupTagUser(t, testDB, e, "csv-cap")

	var b strings.Builder
	b.WriteString("Name,Tag\n")
	for i := 0; i < 60; i++ {
		fmt.Fprintf(&b, "Game %d,tag-%d\n", i, i)
	}
	rec := postMultipartFile(t, e, "/api/import/csv/inspect", "lib.csv", []byte(b.String()), token)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}
	var resp struct {
		Columns []struct {
			Name              string   `json:"name"`
			DistinctValues    []string `json:"distinct_values"`
			DistinctTruncated bool     `json:"distinct_truncated"`
		} `json:"columns"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, c := range resp.Columns {
		if c.Name == "Tag" {
			if len(c.DistinctValues) != 50 || !c.DistinctTruncated {
				t.Errorf("Tag: len=%d truncated=%v, want 50 / true", len(c.DistinctValues), c.DistinctTruncated)
			}
		}
	}
}

func TestImportCSVInspect_RefusesWhenIGDBNotConfigured(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(false))
	_, token := setupTagUser(t, testDB, e, "csv-noigdb")

	rec := postMultipartFile(t, e, "/api/import/csv/inspect", "lib.csv", []byte("Name\nCeleste\n"), token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestImportCSVInspect_HeaderlessEmpty(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))
	_, token := setupTagUser(t, testDB, e, "csv-empty")

	rec := postMultipartFile(t, e, "/api/import/csv/inspect", "lib.csv", []byte(""), token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// validMapping returns a mapping JSON wiring Name→title and Status→status with
// the given value map.
func validMapping(t *testing.T, valueMap map[string]string) string {
	t.Helper()
	m := map[string]any{
		"columns": map[string]string{
			"title": "Name", "platform": "", "storefront": "", "rating": "",
			"notes": "", "acquired_date": "", "hours_played": "", "tags": "", "loved": "",
		},
		"status":         map[string]any{"column": "Status", "value_map": valueMap},
		"rating_scale":   5,
		"merge_by_title": true,
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal mapping: %v", err)
	}
	return string(b)
}

func TestImportCSV_CreatesJobAndItems(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))
	_, token := setupTagUser(t, testDB, e, "csv-success")

	csvData := []byte("Name,Status\nCeleste,Beaten\nHades,Playing\n")
	mapping := validMapping(t, map[string]string{"Beaten": "completed", "Playing": "in_progress"})
	rec := postCSVImport(t, e, "/api/import/csv", "lib.csv", csvData, mapping, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}
	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	jobID, _ := resp["job_id"].(string)
	if jobID == "" {
		t.Fatalf("empty job_id")
	}
	if resp["source"] != "csv" {
		t.Errorf("source = %v, want csv", resp["source"])
	}
	if tot, _ := resp["total_items"].(float64); int(tot) != 2 {
		t.Errorf("total_items = %v, want 2", resp["total_items"])
	}

	ctx := context.Background()
	var itemCount int
	if err := testDB.NewRaw(`SELECT COUNT(*) FROM job_items WHERE job_id = ?`, jobID).Scan(ctx, &itemCount); err != nil {
		t.Fatalf("count job_items: %v", err)
	}
	if itemCount != 2 {
		t.Errorf("job_items = %d, want 2", itemCount)
	}
}

func TestImportCSV_Conflict(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))
	_, token := setupTagUser(t, testDB, e, "csv-conflict")

	csvData := []byte("Name,Status\nCeleste,Beaten\n")
	mapping := validMapping(t, map[string]string{"Beaten": "completed"})
	if rec := postCSVImport(t, e, "/api/import/csv", "lib.csv", csvData, mapping, token); rec.Code != http.StatusOK {
		t.Fatalf("first import status = %d, want 200; body: %s", rec.Code, rec.Body)
	}
	rec := postCSVImport(t, e, "/api/import/csv", "lib.csv", csvData, mapping, token)
	if rec.Code != http.StatusConflict {
		t.Fatalf("second import status = %d, want 409", rec.Code)
	}
}

func TestImportCSV_RefusesWhenIGDBNotConfigured(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(false))
	_, token := setupTagUser(t, testDB, e, "csv-imp-noigdb")

	rec := postCSVImport(t, e, "/api/import/csv", "lib.csv", []byte("Name\nCeleste\n"), validMapping(t, nil), token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestImportCSV_MissingTitleMapping(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))
	_, token := setupTagUser(t, testDB, e, "csv-notitle")

	mapping := `{"columns":{"title":""},"status":{"column":"","value_map":{}},"rating_scale":5,"merge_by_title":true}`
	rec := postCSVImport(t, e, "/api/import/csv", "lib.csv", []byte("Name\nCeleste\n"), mapping, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestImportCSV_NoDataRows(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))
	_, token := setupTagUser(t, testDB, e, "csv-norows")

	rec := postCSVImport(t, e, "/api/import/csv", "lib.csv", []byte("Name,Status\n"), validMapping(t, nil), token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (no games)", rec.Code)
	}
}
