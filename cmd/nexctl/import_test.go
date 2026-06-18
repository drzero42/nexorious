package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// serveImportSources registers GET /api/import/sources advertising the given slugs.
func serveImportSources(mux *http.ServeMux, slugs ...string) {
	mux.HandleFunc("/api/import/sources", func(w http.ResponseWriter, _ *http.Request) {
		sources := make([]map[string]any, len(slugs))
		for i, s := range slugs {
			sources[i] = map[string]any{
				"slug": s, "display_name": strings.ToUpper(s[:1]) + s[1:],
				"description": s + " export", "accept": []string{".json"},
			}
		}
		_ = json.NewEncoder(w).Encode(sources)
	})
}

// readMultipartFile parses the request's multipart form and returns the "file"
// part's bytes. Fails the test if absent.
func readMultipartFile(t *testing.T, r *http.Request) []byte {
	t.Helper()
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		t.Fatalf("parse multipart: %v", err)
	}
	f, _, err := r.FormFile("file")
	if err != nil {
		t.Fatalf("form file: %v", err)
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	return b
}

// writeTempFile writes content to a temp file and returns its path.
func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}

func runImport(t *testing.T, srvURL string, args ...string) (string, error) {
	t.Helper()
	seedProfile(t, srvURL)
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader(""))
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func TestImportSources(t *testing.T) {
	mux := http.NewServeMux()
	serveImportSources(mux, "vglist")
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runImport(t, srv.URL, "import", "sources")
	if err != nil {
		t.Fatalf("import sources: %v\n%s", err, out)
	}
	if !strings.Contains(out, "vglist") {
		t.Errorf("output missing slug: %q", out)
	}
	if !strings.Contains(out, "Dedicated importers") || !strings.Contains(out, "csv") {
		t.Errorf("output missing dedicated-importers footer: %q", out)
	}
}

func TestImportSourcesQuiet(t *testing.T) {
	mux := http.NewServeMux()
	serveImportSources(mux, "vglist")
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runImport(t, srv.URL, "import", "sources", "-q")
	if err != nil {
		t.Fatalf("import sources -q: %v\n%s", err, out)
	}
	if strings.TrimSpace(out) != "vglist" {
		t.Errorf("quiet output = %q, want bare slug", out)
	}
}

func TestImportSourcesJSON(t *testing.T) {
	mux := http.NewServeMux()
	serveImportSources(mux, "vglist")
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runImport(t, srv.URL, "import", "sources", "--json")
	if err != nil {
		t.Fatalf("import sources --json: %v\n%s", err, out)
	}
	var got []map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if len(got) != 1 || got[0]["slug"] != "vglist" {
		t.Errorf("json = %v", got)
	}
}

func TestImportNexorious(t *testing.T) {
	const payload = `{"version":"2.1","games":[]}`
	mux := http.NewServeMux()
	var gotFile []byte
	mux.HandleFunc("/api/import/nexorious", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		gotFile = readMultipartFile(t, r)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"job_id": "j-1", "source": "nexorious", "status": "pending",
			"total_items": 3, "skipped_count": 1,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	file := writeTempFile(t, "export.json", payload)
	out, err := runImport(t, srv.URL, "import", "nexorious", file)
	if err != nil {
		t.Fatalf("import nexorious: %v\n%s", err, out)
	}
	if string(gotFile) != payload {
		t.Errorf("uploaded file = %q, want %q", gotFile, payload)
	}
	if !strings.Contains(out, "j-1") || !strings.Contains(out, "3 games") || !strings.Contains(out, "1 skipped") {
		t.Errorf("output = %q", out)
	}
}

func TestImportNexoriousMissingFile(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runImport(t, srv.URL, "import", "nexorious", filepath.Join(t.TempDir(), "nope.json"))
	if err == nil {
		t.Fatalf("expected error for missing file, got nil\n%s", out)
	}
}

func TestImportRun(t *testing.T) {
	const payload = `[{"name":"Portal"}]`
	mux := http.NewServeMux()
	serveImportSources(mux, "vglist")
	var gotFile []byte
	mux.HandleFunc("/api/import/vglist", func(w http.ResponseWriter, r *http.Request) {
		gotFile = readMultipartFile(t, r)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"job_id": "j-9", "source": "vglist", "status": "processing", "total_items": 1,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	file := writeTempFile(t, "lib.json", payload)
	out, err := runImport(t, srv.URL, "import", "run", "vglist", file)
	if err != nil {
		t.Fatalf("import run: %v\n%s", err, out)
	}
	if string(gotFile) != payload {
		t.Errorf("uploaded file = %q", gotFile)
	}
	if !strings.Contains(out, "j-9") {
		t.Errorf("output = %q", out)
	}
}

func TestImportRunUnknownSource(t *testing.T) {
	mux := http.NewServeMux()
	serveImportSources(mux, "vglist")
	var uploaded bool
	mux.HandleFunc("/api/import/bogus", func(http.ResponseWriter, *http.Request) { uploaded = true })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	file := writeTempFile(t, "lib.json", "[]")
	out, err := runImport(t, srv.URL, "import", "run", "bogus", file)
	if err == nil {
		t.Fatalf("expected error for unknown source, got nil\n%s", out)
	}
	if !strings.Contains(err.Error(), "vglist") {
		t.Errorf("error should list valid sources: %v", err)
	}
	if uploaded {
		t.Error("must not upload for an unknown source")
	}
}
