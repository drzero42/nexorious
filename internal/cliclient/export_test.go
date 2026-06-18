package cliclient

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTriggerExport(t *testing.T) {
	for _, format := range []string{"json", "csv"} {
		format := format
		t.Run(format, func(t *testing.T) {
			var gotMethod, gotPath string
			mux := http.NewServeMux()
			mux.HandleFunc("/api/export/"+format, func(w http.ResponseWriter, r *http.Request) {
				gotMethod = r.Method
				gotPath = r.URL.Path
				_ = json.NewEncoder(w).Encode(map[string]any{
					"job_id":          "exp-1",
					"status":          "pending",
					"message":         "export enqueued",
					"estimated_items": 42,
				})
			})
			srv := httptest.NewServer(mux)
			t.Cleanup(srv.Close)
			c := New(srv.URL)

			result, err := c.TriggerExport("k", format)
			if err != nil {
				t.Fatalf("TriggerExport(%q): %v", format, err)
			}
			if result.JobID != "exp-1" {
				t.Errorf("job_id = %q, want exp-1", result.JobID)
			}
			if result.Status != "pending" {
				t.Errorf("status = %q, want pending", result.Status)
			}
			if result.EstimatedItems != 42 {
				t.Errorf("estimated_items = %d, want 42", result.EstimatedItems)
			}
			if gotMethod != http.MethodPost {
				t.Errorf("method = %q, want POST", gotMethod)
			}
			if gotPath != "/api/export/"+format {
				t.Errorf("path = %q, want /api/export/%s", gotPath, format)
			}
		})
	}
}

func TestTriggerExportUnsupportedFormat(t *testing.T) {
	c := New("http://localhost")
	_, err := c.TriggerExport("k", "xml")
	if err == nil {
		t.Fatal("expected error for unsupported format, got nil")
	}
	want := `unsupported export format "xml"`
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestDownloadExport(t *testing.T) {
	const payload = "game1,steam\ngame2,gog\n"
	mux := http.NewServeMux()
	mux.HandleFunc("/api/export/exp-7/download", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "wrong method", http.StatusMethodNotAllowed)
			return
		}
		// DownloadExport builds its own request (not via doBearer), so it is the
		// one place a missing auth header would slip past the helper's coverage.
		if got := r.Header.Get("Authorization"); got != "Bearer k" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer k")
		}
		w.Header().Set("Content-Disposition", `attachment; filename="export.csv"`)
		_, _ = w.Write([]byte(payload))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	var buf bytes.Buffer
	if err := c.DownloadExport("k", "exp-7", &buf); err != nil {
		t.Fatalf("DownloadExport: %v", err)
	}
	if buf.String() != payload {
		t.Errorf("body = %q, want %q", buf.String(), payload)
	}
}

func TestDownloadExportNonOK(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/export/exp-missing/download", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "job not found"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	var buf bytes.Buffer
	err := c.DownloadExport("k", "exp-missing", &buf)
	if err == nil {
		t.Fatal("expected error on 404, got nil")
	}
	if buf.Len() != 0 {
		t.Errorf("expected no bytes written on error, got %d", buf.Len())
	}
}

func TestDownloadExportPathEscaping(t *testing.T) {
	const payload = "ok"
	mux := http.NewServeMux()
	// An id with a space must be %20-encoded into the request path.
	mux.HandleFunc("/api/export/exp%207/download", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(payload))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	var buf bytes.Buffer
	if err := c.DownloadExport("k", "exp 7", &buf); err != nil {
		t.Fatalf("DownloadExport: %v", err)
	}
	if buf.String() != payload {
		t.Errorf("body = %q, want %q", buf.String(), payload)
	}
}
