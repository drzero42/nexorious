package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// truncatedBody promises more bytes than it sends, so the client's body read
// fails with an unexpected EOF partway through, like a dropped connection.
func truncatedBody(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Length", "1000")
	_, _ = io.WriteString(w, "partial-")
}

// dirEntries lists the names in dir, to catch leftover .part files.
func dirEntries(t *testing.T, dir string) []string {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	var names []string
	for _, e := range ents {
		names = append(names, e.Name())
	}
	return names
}

// A backup download that fails partway must leave the previous good file at
// --out byte-for-byte intact (it used to be truncated by os.Create before the
// download even started) and leave no partial temp file behind.
func TestBackupDownloadFailureKeepsExistingFile(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/admin/backups/bk-1/download", truncatedBody)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	destPath := filepath.Join(dir, "backup.tar.gz")
	const good = "previous-good-backup"
	if err := os.WriteFile(destPath, []byte(good), 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := runBackup(t, srv.URL, "backup", "download", "bk-1", "--out", destPath)
	if err == nil {
		t.Fatalf("expected an error for a truncated download, got nil\n%s", out)
	}
	if strings.Contains(out, "downloaded to") {
		t.Errorf("reported success on a failed download: %q", out)
	}
	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(got) != good {
		t.Errorf("existing file = %q, want it unchanged (%q)", got, good)
	}
	if names := dirEntries(t, dir); len(names) != 1 {
		t.Errorf("dir = %v, want only the original file (no .part leftovers)", names)
	}
}

// An export download that fails partway must not leave a truncated file that
// looks like a valid export.
func TestExportDownloadFailureLeavesNoFile(t *testing.T) {
	prev := exportPollInterval
	exportPollInterval = 0
	t.Cleanup(func() { exportPollInterval = prev })

	mux := http.NewServeMux()
	serveExportTrigger(mux, "json", "ex-t", 1)
	serveJobCompleted(mux, "ex-t")
	mux.HandleFunc("/api/export/ex-t/download", truncatedBody)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	out, err := runExport(t, srv.URL, "export", "--out", filepath.Join(dir, "out.json"))
	if err == nil {
		t.Fatalf("expected an error for a truncated download, got nil\n%s", out)
	}
	if names := dirEntries(t, dir); len(names) != 0 {
		t.Errorf("dir = %v, want empty: no partial export and no .part leftovers", names)
	}
}

// A successful download replaces an existing file and is private (0600).
func TestWriteFileAtomicReplacesExisting(t *testing.T) {
	dir := t.TempDir()
	destPath := filepath.Join(dir, "out.json")
	if err := os.WriteFile(destPath, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(destPath, strings.NewReader("new")); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}
	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Errorf("content = %q, want %q", got, "new")
	}
	fi, err := os.Stat(destPath)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode = %o, want 600", perm)
	}
	if names := dirEntries(t, dir); len(names) != 1 {
		t.Errorf("dir = %v, want only out.json", names)
	}
}
