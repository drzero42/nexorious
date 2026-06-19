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

func TestImportCSVInspect_MalformedCompletionatorQuotes(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))
	_, token := setupTagUser(t, testDB, e, "csv-inspect-malformed")

	// Fully quote-wrapped, with a bare-quoted title strict encoding/csv rejects.
	csvData := []byte("\"Name\",\"Platform\"\n" +
		"\"A Hat in Time\",\"PC / Windows\"\n" +
		"\"Episode 1: \"Done Running\"\",\"PC / Windows\"\n")
	rec := postMultipartFile(t, e, "/api/import/csv/inspect", "completionator.csv", csvData, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}

	var resp struct {
		RowCount int `json:"row_count"`
		Columns  []struct {
			Name           string   `json:"name"`
			DistinctValues []string `json:"distinct_values"`
		} `json:"columns"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.RowCount != 2 {
		t.Fatalf("row_count = %d, want 2", resp.RowCount)
	}
	var names []string
	for _, c := range resp.Columns {
		if c.Name == "Name" {
			names = c.DistinctValues
		}
	}
	found := false
	for _, n := range names {
		if n == `Episode 1: "Done Running"` {
			found = true
		}
	}
	if !found {
		t.Fatalf("recovered Name values = %v, want one to be `Episode 1: \"Done Running\"`", names)
	}
}

func TestImportCSVInspect_SuggestsMapping(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))
	_, token := setupTagUser(t, testDB, e, "csv-suggest")

	csvData := []byte("Name,Platform,Status,Score\nCeleste,Switch,Beaten,9\nHades,PC,Playing,8\n")
	rec := postMultipartFile(t, e, "/api/import/csv/inspect", "lib.csv", csvData, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}

	var resp struct {
		SuggestedMapping struct {
			Columns struct {
				Title    string `json:"title"`
				Platform string `json:"platform"`
				Rating   string `json:"rating"`
			} `json:"columns"`
			Status struct {
				Column   string            `json:"column"`
				ValueMap map[string]string `json:"value_map"`
			} `json:"status"`
			RatingScale  int  `json:"rating_scale"`
			MergeByTitle bool `json:"merge_by_title"`
		} `json:"suggested_mapping"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	sm := resp.SuggestedMapping
	if sm.Columns.Title != "Name" || sm.Columns.Platform != "Platform" || sm.Columns.Rating != "Score" {
		t.Errorf("columns guess wrong: %+v", sm.Columns)
	}
	if sm.Status.Column != "Status" {
		t.Errorf("status column = %q, want Status", sm.Status.Column)
	}
	if sm.Status.ValueMap["Beaten"] != "completed" || sm.Status.ValueMap["Playing"] != "in_progress" {
		t.Errorf("status value_map wrong: %+v", sm.Status.ValueMap)
	}
	if sm.RatingScale != 10 {
		t.Errorf("rating_scale = %d, want 10", sm.RatingScale)
	}
	if !sm.MergeByTitle {
		t.Errorf("merge_by_title should default true")
	}
}

const completionatorCSV = `"Name","Edition","Platform","Format","Region","Now Playing","Backlogged","Ownership Status","Progress Status","Est. Value","Amt. Paid","Tags","Box/Case","Cart/Disc","Manual","Extras","Acquisition Type","Acquisition Source","Acquisition Date","Rating","Initial Release Date","Item Release Date","Added On","Genre"
"A Hat in Time","","PC / Windows","Digital (Steam)","EU","No","Yes","Owned","Incomplete","","","","","","","","Purchase","","","10","10/5/2017","","1/17/2022","Platformer"
`

func TestImportCSVInspect_ReturnsPresets(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))
	_, token := setupTagUser(t, testDB, e, "csv-presets")

	rec := postMultipartFile(t, e, "/api/import/csv/inspect", "x.csv", []byte("Name,Status\nA,Beaten\n"), token)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}
	var resp struct {
		Presets []struct {
			Slug string `json:"slug"`
			Name string `json:"name"`
		} `json:"presets"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	found := false
	for _, p := range resp.Presets {
		if p.Slug == "completionator" && p.Name == "Completionator" {
			found = true
		}
	}
	if !found {
		t.Fatalf("presets = %+v, want one {completionator, Completionator}", resp.Presets)
	}
}

func TestImportCSVInspect_DetectsRegisteredPreset(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))
	_, token := setupTagUser(t, testDB, e, "csv-detect")

	rec := postMultipartFile(t, e, "/api/import/csv/inspect", "c.csv",
		[]byte(completionatorCSV+"\"Celeste\",,,,,Yes,No,Owned,Beaten,,,,,,,,,,2024-01-01,,,,2024-01-02,"), token)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}
	var resp struct {
		Detected *struct {
			Slug string `json:"slug"`
			Name string `json:"name"`
		} `json:"detected"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Detected == nil {
		t.Fatal("detected = nil, want the completionator preset")
	}
	if resp.Detected.Slug != "completionator" || resp.Detected.Name != "Completionator" {
		t.Fatalf("detected = %+v, want {completionator, Completionator}", resp.Detected)
	}
}

func TestImportCSVInspect_NoDetectionForUnknownHeader(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))
	_, token := setupTagUser(t, testDB, e, "csv-no-detect")

	rec := postMultipartFile(t, e, "/api/import/csv/inspect", "x.csv",
		[]byte("Title,Console\nCeleste,PC\n"), token)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}
	var resp struct {
		Detected *struct {
			Slug string `json:"slug"`
		} `json:"detected"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Detected != nil {
		t.Fatalf("detected = %+v, want nil for an unrecognised header", resp.Detected)
	}
}

func TestImportCSV_PresetFormat_UsesServerConfig(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))
	_, token := setupTagUser(t, testDB, e, "csv-preset-import")

	rec := postCSVImportFormat(t, e, "completionator.csv", []byte(completionatorCSV), "completionator", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}
	var resp struct {
		JobID string `json:"job_id"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.JobID == "" {
		t.Error("expected a job_id")
	}
}

func TestImportCSV_UnknownFormat_400(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))
	_, token := setupTagUser(t, testDB, e, "csv-bad-format")

	rec := postCSVImportFormat(t, e, "x.csv", []byte(completionatorCSV), "bogus", token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body)
	}
}

func TestImportCSV_PresetFormat_SignatureMismatch_400(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))
	_, token := setupTagUser(t, testDB, e, "csv-sig-mismatch")

	rec := postCSVImportFormat(t, e, "x.csv", []byte("Title,Console\nCeleste,PC\n"), "completionator", token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body)
	}
}

func TestImportCSV_Auto_DetectsPreset(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))
	_, token := setupTagUser(t, testDB, e, "csv-auto-preset")

	// completionatorCSV header matches the completionator signature; append a data row.
	data := []byte(completionatorCSV + "\"Celeste\",,,,,Yes,No,Owned,Beaten,,,,,,,,,,2024-01-01,,,,2024-01-02,")
	rec := postMultipartFile(t, e, "/api/import/csv", "c.csv", data, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}
	var resp struct {
		JobID string `json:"job_id"`
		Auto  *struct {
			Mode   string `json:"mode"`
			Preset *struct {
				Slug string `json:"slug"`
				Name string `json:"name"`
			} `json:"preset"`
		} `json:"auto"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.JobID == "" {
		t.Error("expected a job_id")
	}
	if resp.Auto == nil || resp.Auto.Mode != "preset" || resp.Auto.Preset == nil || resp.Auto.Preset.Slug != "completionator" {
		t.Fatalf("auto = %+v, want preset completionator", resp.Auto)
	}
}

func TestImportCSV_Auto_GuessesMapping(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))
	_, token := setupTagUser(t, testDB, e, "csv-auto-guess")

	data := []byte("Name,Status\nCeleste,Beaten\nHades,Playing\n")
	rec := postMultipartFile(t, e, "/api/import/csv", "g.csv", data, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}
	var resp struct {
		JobID string `json:"job_id"`
		Auto  *struct {
			Mode    string `json:"mode"`
			Mapping *struct {
				Columns struct {
					Title string `json:"title"`
				} `json:"columns"`
			} `json:"mapping"`
		} `json:"auto"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.JobID == "" {
		t.Error("expected a job_id")
	}
	if resp.Auto == nil || resp.Auto.Mode != "guessed" || resp.Auto.Mapping == nil || resp.Auto.Mapping.Columns.Title != "Name" {
		t.Fatalf("auto = %+v, want guessed title=Name", resp.Auto)
	}
}

func TestImportCSV_Auto_NoTitle_400(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))
	_, token := setupTagUser(t, testDB, e, "csv-auto-notitle")

	// "Console" guesses to platform, no title-like header, no preset signature.
	data := []byte("Console\nPC\nSwitch\n")
	rec := postMultipartFile(t, e, "/api/import/csv", "n.csv", data, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "no title column") {
		t.Errorf("body = %q, want it to mention 'no title column'", rec.Body.String())
	}
}

func TestImportCSV_GenericNoMapping_StillMissingMapping_400(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))
	_, token := setupTagUser(t, testDB, e, "csv-generic-nomap")

	rec := postCSVImportFormat(t, e, "x.csv", []byte("Name,Status\nCeleste,Beaten\n"), "generic", token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "missing mapping field") {
		t.Errorf("body = %q, want 'missing mapping field'", rec.Body.String())
	}
}

// postCSVImportFormat posts a CSV with a "format" form field (preset path), no mapping.
func postCSVImportFormat(t *testing.T, e interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, filename string, fileContent []byte, format, sessionID string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("createFormFile: %v", err)
	}
	if _, err := fw.Write(fileContent); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := mw.WriteField("format", format); err != nil {
		t.Fatalf("write format: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/import/csv", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if sessionID != "" {
		req.AddCookie(&http.Cookie{Name: "session_id", Value: sessionID})
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}
