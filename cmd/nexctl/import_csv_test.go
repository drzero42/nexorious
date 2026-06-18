package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// sampleCSVInspect returns a representative CSVInspect JSON payload.
func sampleCSVInspect() map[string]any {
	return map[string]any{
		"headers":   []string{"Name", "Platform", "Score", "Status"},
		"row_count": 42,
		"columns":   []any{},
		"suggested_mapping": map[string]any{
			"columns": map[string]any{
				"title":    "Name",
				"platform": "Platform",
				"rating":   "Score",
			},
			"status": map[string]any{
				"column":    "Status",
				"value_map": map[string]string{},
			},
			"rating_scale":   10,
			"merge_by_title": false,
		},
		"presets": []map[string]any{
			{"slug": "grouvee", "name": "Grouvee"},
		},
		"detected": map[string]any{"slug": "grouvee", "name": "Grouvee"},
	}
}

func TestImportCSVInspect(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/import/csv/inspect", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		_ = readMultipartFile(t, r) // consume the upload
		if err := json.NewEncoder(w).Encode(sampleCSVInspect()); err != nil {
			t.Errorf("encode response: %v", err)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	file := writeTempFile(t, "games.csv", "Name,Platform,Score,Status\nPortal,PC,9,Beat")
	out, err := runImport(t, srv.URL, "import", "csv", file, "--inspect")
	if err != nil {
		t.Fatalf("import csv --inspect: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Name") {
		t.Errorf("output missing header 'Name': %q", out)
	}
	if !strings.Contains(out, "grouvee") {
		t.Errorf("output missing detected preset 'grouvee': %q", out)
	}
	if !strings.Contains(out, "Rows: 42") {
		t.Errorf("output missing row count: %q", out)
	}
}

func TestImportCSVInspectJSON(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/import/csv/inspect", func(w http.ResponseWriter, r *http.Request) {
		_ = readMultipartFile(t, r)
		if err := json.NewEncoder(w).Encode(sampleCSVInspect()); err != nil {
			t.Errorf("encode response: %v", err)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	file := writeTempFile(t, "games.csv", "Name,Platform,Score,Status\nPortal,PC,9,Beat")
	out, err := runImport(t, srv.URL, "import", "csv", file, "--inspect", "--json")
	if err != nil {
		t.Fatalf("import csv --inspect --json: %v\n%s", err, out)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	headers, _ := got["headers"].([]any)
	if len(headers) == 0 || headers[0] != "Name" {
		t.Errorf("JSON headers = %v", headers)
	}
}

func TestImportCSVPreset(t *testing.T) {
	mux := http.NewServeMux()
	var gotFormat, gotMapping string
	mux.HandleFunc("/api/import/csv", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		_ = readMultipartFile(t, r)
		gotFormat = r.FormValue("format")
		gotMapping = r.FormValue("mapping")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"job_id": "j-csv-1", "source": "csv", "status": "pending", "total_items": 5,
		}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	file := writeTempFile(t, "grouvee.csv", "Name,Platform\nPortal,PC")
	out, err := runImport(t, srv.URL, "import", "csv", file, "--preset", "grouvee")
	if err != nil {
		t.Fatalf("import csv --preset grouvee: %v\n%s", err, out)
	}
	if gotFormat != "grouvee" {
		t.Errorf("form field format = %q, want %q", gotFormat, "grouvee")
	}
	if gotMapping != "" {
		t.Errorf("form field mapping should be absent for preset, got %q", gotMapping)
	}
	if !strings.Contains(out, "j-csv-1") {
		t.Errorf("output missing job id: %q", out)
	}
}

func TestImportCSVManualMapping(t *testing.T) {
	mux := http.NewServeMux()
	var gotMappingRaw string
	mux.HandleFunc("/api/import/csv", func(w http.ResponseWriter, r *http.Request) {
		_ = readMultipartFile(t, r)
		gotMappingRaw = r.FormValue("mapping")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"job_id": "j-csv-2", "source": "csv", "status": "pending", "total_items": 3,
		}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	file := writeTempFile(t, "games.csv", "Name,Platform,Score,Status\nPortal,PC,8,Beat")
	out, err := runImport(t, srv.URL, "import", "csv", file,
		"--title-col", "Name",
		"--status-col", "Status",
		"--status-map", "Beat=completed",
		"--rating-col", "Score",
		"--rating-scale", "10",
		"--merge-by-title",
	)
	if err != nil {
		t.Fatalf("import csv manual: %v\n%s", err, out)
	}

	// Decode and verify the mapping the server received.
	var m map[string]any
	if err := json.Unmarshal([]byte(gotMappingRaw), &m); err != nil {
		t.Fatalf("mapping is not JSON: %v\n%s", err, gotMappingRaw)
	}

	cols, _ := m["columns"].(map[string]any)
	if cols["title"] != "Name" {
		t.Errorf("columns.title = %v, want Name", cols["title"])
	}
	// Empty columns must be absent.
	if _, ok := cols["platform"]; ok {
		t.Errorf("columns.platform should be absent when not set")
	}

	if cols["rating"] != "Score" {
		t.Errorf("columns.rating = %v, want Score", cols["rating"])
	}

	status, _ := m["status"].(map[string]any)
	if status["column"] != "Status" {
		t.Errorf("status.column = %v, want Status", status["column"])
	}
	vm, _ := status["value_map"].(map[string]any)
	if vm["Beat"] != "completed" {
		t.Errorf("status.value_map[Beat] = %v, want completed", vm["Beat"])
	}

	// rating_scale arrives as float64 after JSON round-trip.
	if m["rating_scale"] != float64(10) {
		t.Errorf("rating_scale = %v, want 10", m["rating_scale"])
	}

	if m["merge_by_title"] != true {
		t.Errorf("merge_by_title = %v, want true", m["merge_by_title"])
	}

	if !strings.Contains(out, "j-csv-2") {
		t.Errorf("output missing job id: %q", out)
	}
}

func TestImportCSVManualNoTitleCol(t *testing.T) {
	var uploaded bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/import/csv", func(http.ResponseWriter, *http.Request) { uploaded = true })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	file := writeTempFile(t, "games.csv", "Name,Platform\nPortal,PC")
	_, err := runImport(t, srv.URL, "import", "csv", file, "--notes-col", "Notes")
	if err == nil {
		t.Fatal("expected error when --title-col is missing, got nil")
	}
	if !strings.Contains(err.Error(), "--title-col") {
		t.Errorf("error should mention --title-col: %v", err)
	}
	if uploaded {
		t.Error("must not upload when --title-col is absent")
	}
}

func TestImportCSVInvalidRatingScale(t *testing.T) {
	var uploaded bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/import/csv", func(http.ResponseWriter, *http.Request) { uploaded = true })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	file := writeTempFile(t, "games.csv", "Name,Score\nPortal,8")
	_, err := runImport(t, srv.URL, "import", "csv", file,
		"--title-col", "Name", "--rating-col", "Score", "--rating-scale", "7")
	if err == nil {
		t.Fatal("expected error for an invalid rating scale, got nil")
	}
	if !strings.Contains(err.Error(), "rating-scale") {
		t.Errorf("error should mention rating-scale: %v", err)
	}
	if uploaded {
		t.Error("must not upload on an invalid rating scale")
	}
}

func TestImportCSVStatusMapMissingEquals(t *testing.T) {
	var uploaded bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/import/csv", func(http.ResponseWriter, *http.Request) { uploaded = true })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	file := writeTempFile(t, "games.csv", "Name,Status\nPortal,Beat")
	_, err := runImport(t, srv.URL, "import", "csv", file,
		"--title-col", "Name", "--status-col", "Status", "--status-map", "Beat")
	if err == nil {
		t.Fatal("expected error for a --status-map entry without '=', got nil")
	}
	if !strings.Contains(err.Error(), "raw=canonical") {
		t.Errorf("error should mention raw=canonical: %v", err)
	}
	if uploaded {
		t.Error("must not upload on a malformed --status-map")
	}
}

func TestImportCSVStatusMapRequiresStatusCol(t *testing.T) {
	var uploaded bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/import/csv", func(http.ResponseWriter, *http.Request) { uploaded = true })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	file := writeTempFile(t, "games.csv", "Name,Status\nPortal,Beat")
	_, err := runImport(t, srv.URL, "import", "csv", file,
		"--title-col", "Name", "--status-map", "Beat=completed")
	if err == nil {
		t.Fatal("expected error for --status-map without --status-col, got nil")
	}
	if !strings.Contains(err.Error(), "--status-col") {
		t.Errorf("error should mention --status-col: %v", err)
	}
	if uploaded {
		t.Error("must not upload when --status-map lacks --status-col")
	}
}

func TestImportCSVPresetAndManualMutualExclusion(t *testing.T) {
	var uploaded bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/import/csv", func(http.ResponseWriter, *http.Request) { uploaded = true })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	file := writeTempFile(t, "games.csv", "Name,Platform\nPortal,PC")
	_, err := runImport(t, srv.URL, "import", "csv", file, "--preset", "grouvee", "--title-col", "Name")
	if err == nil {
		t.Fatal("expected mutual-exclusion error, got nil")
	}
	if !strings.Contains(err.Error(), "--preset") {
		t.Errorf("error should mention --preset: %v", err)
	}
	if uploaded {
		t.Error("must not upload on mutual-exclusion error")
	}
}
