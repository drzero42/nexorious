package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestImportResolve(t *testing.T) {
	mux := http.NewServeMux()
	var gotIgdb float64
	var hit bool
	mux.HandleFunc("/api/job-items/it-1/resolve", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		hit = true
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotIgdb, _ = body["igdb_id"].(float64)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runImport(t, srv.URL, "import", "resolve", "it-1", "--igdb-id", "42")
	if err != nil {
		t.Fatalf("import resolve: %v\n%s", err, out)
	}
	if !hit || gotIgdb != 42 {
		t.Errorf("resolve not received with igdb_id=42 (hit=%v, igdb=%v)", hit, gotIgdb)
	}
	if !strings.Contains(out, "resolved") || !strings.Contains(out, "42") {
		t.Errorf("output = %q", out)
	}
}

func TestImportResolveMissingIgdbID(t *testing.T) {
	mux := http.NewServeMux()
	var hit bool
	mux.HandleFunc("/api/job-items/it-1/resolve", func(http.ResponseWriter, *http.Request) { hit = true })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runImport(t, srv.URL, "import", "resolve", "it-1")
	if err == nil {
		t.Fatalf("expected error without --igdb-id, got nil\n%s", out)
	}
	if !strings.Contains(err.Error(), "igdb-id") {
		t.Errorf("error = %v, want it to mention --igdb-id", err)
	}
	if hit {
		t.Error("must not send a request without --igdb-id")
	}
}

func TestImportSkipYes(t *testing.T) {
	mux := http.NewServeMux()
	var hit bool
	mux.HandleFunc("/api/job-items/it-2/skip", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		hit = true
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runImport(t, srv.URL, "import", "skip", "it-2", "-y")
	if err != nil {
		t.Fatalf("import skip -y: %v\n%s", err, out)
	}
	if !hit {
		t.Error("skip request not received")
	}
	if !strings.Contains(out, "skipped it-2") {
		t.Errorf("output = %q", out)
	}
}

func TestImportSkipAbortsWithoutYes(t *testing.T) {
	mux := http.NewServeMux()
	var hit bool
	mux.HandleFunc("/api/job-items/it-2/skip", func(http.ResponseWriter, *http.Request) { hit = true })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	// runImport seeds a non-TTY stdin; with no -y the confirm must abort.
	out, err := runImport(t, srv.URL, "import", "skip", "it-2")
	if err != nil {
		t.Fatalf("import skip abort: unexpected error %v", err)
	}
	if hit {
		t.Error("skip must not be sent when not confirmed")
	}
	if !strings.Contains(out, "Aborted.") {
		t.Errorf("output = %q, want Aborted.", out)
	}
}

func TestImportReviewOffTTY(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runImport(t, srv.URL, "import", "review", "job-1")
	if err == nil {
		t.Fatalf("expected error off-TTY, got nil\n%s", out)
	}
	if !strings.Contains(err.Error(), "interactive") {
		t.Errorf("error = %v, want interactive hint", err)
	}
}
