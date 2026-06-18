package cliclient

import (
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// parseMultipartFile parses the multipart request body and returns the bytes
// from the "file" part, plus the form values. The caller is responsible for
// parsing the Content-Type header boundary.
func parseMultipartFile(t *testing.T, r *http.Request) ([]byte, map[string]string) {
	t.Helper()
	ct := r.Header.Get("Content-Type")
	_, params, err := mime.ParseMediaType(ct)
	if err != nil {
		t.Fatalf("parse Content-Type: %v", err)
	}
	mr := multipart.NewReader(r.Body, params["boundary"])

	var fileBytes []byte
	fields := map[string]string{}
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("next part: %v", err)
		}
		b, err := io.ReadAll(part)
		if err != nil {
			t.Fatalf("read part: %v", err)
		}
		if part.FormName() == "file" {
			fileBytes = b
		} else {
			fields[part.FormName()] = string(b)
		}
	}
	return fileBytes, fields
}

func TestListImportSources(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/import/sources", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"slug":         "vglist",
				"display_name": "vglist",
				"description":  "Migrate a vglist library.",
				"accept":       []string{".json"},
				"features":     []string{"Matches games to IGDB"},
			},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	sources, err := c.ListImportSources("k")
	if err != nil {
		t.Fatalf("ListImportSources: %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("len(sources) = %d, want 1", len(sources))
	}
	if sources[0].Slug != "vglist" || sources[0].DisplayName != "vglist" {
		t.Errorf("sources[0] = %+v", sources[0])
	}
	if len(sources[0].Accept) != 1 || sources[0].Accept[0] != ".json" {
		t.Errorf("sources[0].Accept = %v", sources[0].Accept)
	}
}

func TestListImportSources_error(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/import/sources", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"message":"unauthorized"}`, http.StatusUnauthorized)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	_, err := c.ListImportSources("bad")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error = %v, want 401", err)
	}
}

func TestImportNexorious(t *testing.T) {
	payload := []byte(`{"version":"2.1","games":[{"igdb_id":1,"title":"Half-Life"}],"pools":[]}`)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/import/nexorious", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		got, _ := parseMultipartFile(t, r)
		if string(got) != string(payload) {
			t.Errorf("file bytes mismatch: got %q, want %q", got, payload)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"job_id":        "j-1",
			"source":        "nexorious",
			"status":        "pending",
			"message":       "Import job created. Processing 1 games.",
			"total_items":   1,
			"skipped_count": 2,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	res, err := c.ImportNexorious("k", "export.json", payload)
	if err != nil {
		t.Fatalf("ImportNexorious: %v", err)
	}
	if res.JobID != "j-1" {
		t.Errorf("JobID = %q, want j-1", res.JobID)
	}
	if res.SkippedCount != 2 {
		t.Errorf("SkippedCount = %d, want 2", res.SkippedCount)
	}
	if res.TotalItems != 1 {
		t.Errorf("TotalItems = %d, want 1", res.TotalItems)
	}
}

func TestInspectCSV(t *testing.T) {
	payload := []byte("title,status\nHalf-Life,Completed")
	mux := http.NewServeMux()
	mux.HandleFunc("/api/import/csv/inspect", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		got, _ := parseMultipartFile(t, r)
		if string(got) != string(payload) {
			t.Errorf("file bytes mismatch: got %q, want %q", got, payload)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"headers":   []string{"title", "status"},
			"row_count": 1,
			"columns": []map[string]any{
				{"name": "title", "distinct_values": []string{"Half-Life"}, "distinct_truncated": false},
				{"name": "status", "distinct_values": []string{"Completed"}, "distinct_truncated": false},
			},
			"suggested_mapping": map[string]any{
				"columns":        map[string]any{"title": "title"},
				"status":         map[string]any{"column": "status", "value_map": map[string]string{}},
				"rating_scale":   0,
				"merge_by_title": false,
			},
			"presets":  []map[string]any{},
			"detected": nil,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	res, err := c.InspectCSV("k", "games.csv", payload)
	if err != nil {
		t.Fatalf("InspectCSV: %v", err)
	}
	if res.RowCount != 1 {
		t.Errorf("RowCount = %d, want 1", res.RowCount)
	}
	if len(res.Headers) != 2 || res.Headers[0] != "title" {
		t.Errorf("Headers = %v", res.Headers)
	}
	if len(res.Columns) != 2 || res.Columns[0].Name != "title" {
		t.Errorf("Columns = %v", res.Columns)
	}
	if res.SuggestedMapping.Columns.Title != "title" {
		t.Errorf("SuggestedMapping.Columns.Title = %q, want title", res.SuggestedMapping.Columns.Title)
	}
	if res.Detected != nil {
		t.Errorf("Detected = %v, want nil", res.Detected)
	}
}

func TestImportCSV_formatOnly(t *testing.T) {
	payload := []byte("title,status\nPortal,Completed")
	mux := http.NewServeMux()
	mux.HandleFunc("/api/import/csv", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		got, fields := parseMultipartFile(t, r)
		if string(got) != string(payload) {
			t.Errorf("file bytes mismatch")
		}
		if fields["format"] != "grouvee" {
			t.Errorf("format = %q, want grouvee", fields["format"])
		}
		if _, ok := fields["mapping"]; ok {
			t.Errorf("unexpected mapping field present")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"job_id":      "j-2",
			"source":      "csv",
			"status":      "processing",
			"message":     "CSV import job created. Matching 1 games.",
			"total_items": 1,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	res, err := c.ImportCSV("k", "games.csv", payload, "grouvee", nil)
	if err != nil {
		t.Fatalf("ImportCSV (format only): %v", err)
	}
	if res.JobID != "j-2" {
		t.Errorf("JobID = %q, want j-2", res.JobID)
	}
}

func TestImportCSV_mappingOnly(t *testing.T) {
	payload := []byte("title,status\nPortal,Completed")
	mappingJSON := json.RawMessage(`{"columns":{"title":"title"},"status":{"column":"status","value_map":{}},"rating_scale":0,"merge_by_title":false}`)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/import/csv", func(w http.ResponseWriter, r *http.Request) {
		_, fields := parseMultipartFile(t, r)
		if _, ok := fields["format"]; ok {
			t.Errorf("unexpected format field present")
		}
		if fields["mapping"] != string(mappingJSON) {
			t.Errorf("mapping = %q, want %q", fields["mapping"], mappingJSON)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"job_id":      "j-3",
			"source":      "csv",
			"status":      "processing",
			"message":     "CSV import job created. Matching 1 games.",
			"total_items": 1,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	res, err := c.ImportCSV("k", "games.csv", payload, "", mappingJSON)
	if err != nil {
		t.Fatalf("ImportCSV (mapping only): %v", err)
	}
	if res.JobID != "j-3" {
		t.Errorf("JobID = %q, want j-3", res.JobID)
	}
}

func TestImportSource_pathEscape(t *testing.T) {
	payload := []byte(`[{"name":"Half-Life","status":"completed"}]`)
	// "vglist" is a normal slug that should not be altered by path escaping, but
	// we verify the full path construction is correct.
	mux := http.NewServeMux()
	mux.HandleFunc("/api/import/vglist", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		got, _ := parseMultipartFile(t, r)
		if string(got) != string(payload) {
			t.Errorf("file bytes mismatch: got %q, want %q", got, payload)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"job_id":      "j-4",
			"source":      "vglist",
			"status":      "processing",
			"message":     "vglist import job created. Matching 1 games.",
			"total_items": 1,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	res, err := c.ImportSource("k", "vglist", "export.json", payload)
	if err != nil {
		t.Fatalf("ImportSource: %v", err)
	}
	if res.JobID != "j-4" {
		t.Errorf("JobID = %q, want j-4", res.JobID)
	}
	if res.Source != "vglist" {
		t.Errorf("Source = %q, want vglist", res.Source)
	}
}

func TestImportSource_nonTwoXX(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/import/vglist", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	_, err := c.ImportSource("k", "vglist", "export.json", []byte("{}"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("error = %v, want 400", err)
	}
}
