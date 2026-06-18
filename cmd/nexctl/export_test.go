package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// runExport drives newRootCmd with the given args, pre-seeded against srvURL.
// Returns stdout+stderr combined and any execution error.
func runExport(t *testing.T, srvURL string, args ...string) (string, error) {
	t.Helper()
	seedProfile(t, srvURL)
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

// serveExportTrigger registers POST /api/export/<format> returning jobID with
// the given estimated item count. It also records whether the endpoint was hit.
func serveExportTrigger(mux *http.ServeMux, format, jobID string, estimated int) *bool {
	hit := false
	mux.HandleFunc("/api/export/"+format, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			return
		}
		hit = true
		_ = json.NewEncoder(w).Encode(map[string]any{
			"job_id":          jobID,
			"status":          "pending",
			"message":         "export queued",
			"estimated_items": estimated,
		})
	})
	return &hit
}

// serveJobCompleted registers GET /api/jobs/<id> returning a completed terminal job.
func serveJobCompleted(mux *http.ServeMux, jobID string) {
	mux.HandleFunc("/api/jobs/"+jobID, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": jobID, "status": "completed", "is_terminal": true,
			"job_type": "export", "source": "", "priority": "normal",
			"total_items": 5, "created_at": "2026-06-18T00:00:00Z",
		})
	})
}

// serveDownload registers GET /api/export/<id>/download returning the given bytes.
func serveDownload(mux *http.ServeMux, jobID string, data []byte) {
	mux.HandleFunc("/api/export/"+jobID+"/download", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(data)
	})
}

func TestExportNoWait(t *testing.T) {
	mux := http.NewServeMux()
	hit := serveExportTrigger(mux, "json", "ex-1", 5)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runExport(t, srv.URL, "export", "--no-wait")
	if err != nil {
		t.Fatalf("export --no-wait: %v\n%s", err, out)
	}
	if !*hit {
		t.Error("expected POST /api/export/json to be called")
	}
	if !strings.Contains(out, "ex-1") {
		t.Errorf("output missing job id: %q", out)
	}
}

func TestExportNoWaitQuiet(t *testing.T) {
	mux := http.NewServeMux()
	serveExportTrigger(mux, "json", "ex-q", 3)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runExport(t, srv.URL, "export", "--no-wait", "-q")
	if err != nil {
		t.Fatalf("export --no-wait -q: %v\n%s", err, out)
	}
	if strings.TrimSpace(out) != "ex-q" {
		t.Errorf("quiet output = %q, want bare job id", out)
	}
}

func TestExportOutNoWaitMutualExclusion(t *testing.T) {
	mux := http.NewServeMux()
	var triggered bool
	mux.HandleFunc("/api/export/json", func(http.ResponseWriter, *http.Request) { triggered = true })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	_, err := runExport(t, srv.URL, "export", "--no-wait", "--out", "x.json")
	if err == nil {
		t.Fatal("expected mutual-exclusion error, got nil")
	}
	if !strings.Contains(err.Error(), "--no-wait") {
		t.Errorf("error = %v, want it to mention --no-wait", err)
	}
	if triggered {
		t.Error("must not trigger an export on the mutual-exclusion error")
	}
}

func TestExportNoWaitJSON(t *testing.T) {
	mux := http.NewServeMux()
	serveExportTrigger(mux, "json", "ex-2", 7)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runExport(t, srv.URL, "export", "--no-wait", "--json")
	if err != nil {
		t.Fatalf("export --no-wait --json: %v\n%s", err, out)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out)
	}
	if got["job_id"] != "ex-2" {
		t.Errorf("job_id = %v, want ex-2", got["job_id"])
	}
}

func TestExportWaitOutFile(t *testing.T) {
	prev := exportPollInterval
	exportPollInterval = 0
	t.Cleanup(func() { exportPollInterval = prev })

	const payload = `{"games":[]}`
	mux := http.NewServeMux()
	serveExportTrigger(mux, "json", "ex-3", 3)
	serveJobCompleted(mux, "ex-3")
	serveDownload(mux, "ex-3", []byte(payload))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	destPath := filepath.Join(t.TempDir(), "out.json")
	out, err := runExport(t, srv.URL, "export", "--out", destPath)
	if err != nil {
		t.Fatalf("export --out: %v\n%s", err, out)
	}
	if !strings.Contains(out, "exported to") || !strings.Contains(out, destPath) {
		t.Errorf("output = %q, want 'exported to <path>'", out)
	}
	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(got) != payload {
		t.Errorf("file content = %q, want %q", got, payload)
	}
}

func TestExportWaitOutStdout(t *testing.T) {
	prev := exportPollInterval
	exportPollInterval = 0
	t.Cleanup(func() { exportPollInterval = prev })

	const payload = `{"games":[]}`
	mux := http.NewServeMux()
	serveExportTrigger(mux, "json", "ex-4", 2)
	serveJobCompleted(mux, "ex-4")
	serveDownload(mux, "ex-4", []byte(payload))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runExport(t, srv.URL, "export", "--out", "-")
	if err != nil {
		t.Fatalf("export --out -: %v\n%s", err, out)
	}
	if out != payload {
		t.Errorf("stdout = %q, want %q", out, payload)
	}
}

func TestExportWaitFailedJob(t *testing.T) {
	prev := exportPollInterval
	exportPollInterval = 0
	t.Cleanup(func() { exportPollInterval = prev })

	mux := http.NewServeMux()
	serveExportTrigger(mux, "json", "ex-5", 0)
	mux.HandleFunc("/api/jobs/ex-5", func(w http.ResponseWriter, _ *http.Request) {
		errMsg := "boom"
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "ex-5", "status": "failed", "is_terminal": true,
			"error_message": errMsg,
			"job_type":      "export", "source": "", "priority": "normal",
			"total_items": 0, "created_at": "2026-06-18T00:00:00Z",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	_, err := runExport(t, srv.URL, "export")
	if err == nil {
		t.Fatal("expected error for failed job, got nil")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error = %v, want it to mention 'boom'", err)
	}
}

func TestExportFormatCSV(t *testing.T) {
	mux := http.NewServeMux()
	hit := serveExportTrigger(mux, "csv", "ex-6", 4)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runExport(t, srv.URL, "export", "--format", "csv", "--no-wait")
	if err != nil {
		t.Fatalf("export --format csv: %v\n%s", err, out)
	}
	if !*hit {
		t.Error("expected POST /api/export/csv to be called")
	}
}

func TestExportInvalidFormat(t *testing.T) {
	// No server needed — validation fires before any network call.
	mux := http.NewServeMux()
	var triggered bool
	mux.HandleFunc("/api/export/xml", func(http.ResponseWriter, *http.Request) { triggered = true })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	_, err := runExport(t, srv.URL, "export", "--format", "xml")
	if err == nil {
		t.Fatal("expected error for invalid format, got nil")
	}
	if !strings.Contains(err.Error(), "xml") {
		t.Errorf("error = %v, want it to mention 'xml'", err)
	}
	if triggered {
		t.Error("must not make a network call for an invalid format")
	}
}

func TestExportDefaultFilename(t *testing.T) {
	// Verify that when --out is empty the downloaded file is written to the
	// current directory as nexorious_export_<job-id>.<ext>.
	prev := exportPollInterval
	exportPollInterval = 0
	t.Cleanup(func() { exportPollInterval = prev })

	const payload = `{"games":[]}`
	mux := http.NewServeMux()
	serveExportTrigger(mux, "json", "ex-7", 1)
	serveJobCompleted(mux, "ex-7")
	serveDownload(mux, "ex-7", []byte(payload))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	prev2, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev2) })

	out, err := runExport(t, srv.URL, "export")
	if err != nil {
		t.Fatalf("export default filename: %v\n%s", err, out)
	}
	expected := "nexorious_export_ex-7.json"
	if !strings.Contains(out, expected) {
		t.Errorf("output = %q, want path to contain %q", out, expected)
	}
	got, err := os.ReadFile(filepath.Join(dir, expected))
	if err != nil {
		t.Fatalf("read default file: %v", err)
	}
	if string(got) != payload {
		t.Errorf("default file content = %q, want %q", got, payload)
	}
}

// Ensure the package-level var is the right type (compile-time check).
var _ time.Duration = exportPollInterval
