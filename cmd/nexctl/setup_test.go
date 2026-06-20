package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/drzero42/nexorious/internal/clicfg"
)

// okHealth answers GET /health with a Ready status, so cliauth.Preflight passes
// through without triggering a migration (used by setup-zone command tests where
// the instance is already migrated).
func okHealth(w http.ResponseWriter, _ *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
}

// runNexctl executes the given args with piped stdin and an isolated config home.
func runNexctl(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader(stdin))
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func TestConfirmInteractivePasswordMismatch(t *testing.T) {
	entries := []string{"first-secret", "second-secret"}
	i := 0
	read := func(string) (string, error) { v := entries[i]; i++; return v, nil }
	if _, err := confirmInteractivePassword(read); err == nil {
		t.Fatal("expected mismatch error, got nil")
	}
}

func TestConfirmInteractivePasswordMatch(t *testing.T) {
	read := func(string) (string, error) { return "supersecret", nil }
	pw, err := confirmInteractivePassword(read)
	if err != nil || pw != "supersecret" {
		t.Fatalf("pw=%q err=%v; want supersecret", pw, err)
	}
}

func TestConfirmInteractivePasswordBothEmpty(t *testing.T) {
	read := func(string) (string, error) { return "", nil }
	_, err := confirmInteractivePassword(read)
	if err == nil || !strings.Contains(err.Error(), "password is required") {
		t.Fatalf("err = %v; want password-required", err)
	}
}

// setupAdminStub serves /health (ok), a configurable /api/auth/setup/admin.
type setupAdminStub struct {
	adminStatus   int
	adminMessage  string
	adminLocation string
}

func (s setupAdminStub) server(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	})
	mux.HandleFunc("/api/auth/setup/admin", func(w http.ResponseWriter, _ *http.Request) {
		code := s.adminStatus
		if code == 0 {
			code = http.StatusCreated
		}
		if s.adminLocation != "" {
			w.Header().Set("Location", s.adminLocation)
		}
		w.WriteHeader(code)
		if s.adminMessage != "" {
			_ = json.NewEncoder(w).Encode(map[string]string{"message": s.adminMessage})
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestSetupAdminCreates(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	srv := setupAdminStub{}.server(t)
	out, err := runNexctl(t, "supersecret\n", "setup", "admin", "--url", srv.URL, "--username", "admin", "--password-stdin")
	if err != nil {
		t.Fatalf("setup admin: %v\n%s", err, out)
	}
	if !strings.Contains(out, `Admin user "admin" created.`) {
		t.Fatalf("out = %q", out)
	}
}

func TestSetupAdminAlreadyComplete(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	srv := setupAdminStub{adminStatus: http.StatusForbidden, adminMessage: "setup already complete"}.server(t)
	_, err := runNexctl(t, "supersecret\n", "setup", "admin", "--url", srv.URL, "--username", "admin", "--password-stdin")
	if err == nil || !strings.Contains(err.Error(), "already") {
		t.Fatalf("err = %v; want already-complete", err)
	}
}

func TestSetupAdminMissingUsernameWithPasswordStdin(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	_, err := runNexctl(t, "supersecret\n", "setup", "admin", "--url", "http://127.0.0.1:1", "--password-stdin")
	if err == nil || !strings.Contains(err.Error(), "--username is required") {
		t.Fatalf("err = %v; want username-required", err)
	}
}

func TestSetupAdminNoPasswordSourceNonTTY(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	_, err := runNexctl(t, "", "setup", "admin", "--url", "http://127.0.0.1:1", "--username", "admin")
	if err == nil || !strings.Contains(err.Error(), "no password") {
		t.Fatalf("err = %v; want no-password", err)
	}
}

// loginStub serves admin-setup plus the full login bootstrap for --login.
func loginStub(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	})
	mux.HandleFunc("/api/auth/setup/admin", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusCreated) })
	mux.HandleFunc("/api/auth/login", func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "session_id", Value: "sess-xyz"})
		_ = json.NewEncoder(w).Encode(map[string]string{"username": "admin"})
	})
	mux.HandleFunc("/api/auth/api-keys", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "key-1", "key": "nxr_minted"})
	})
	mux.HandleFunc("/api/auth/logout", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestSetupAdminLoginStoresKey(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	srv := loginStub(t)
	out, err := runNexctl(t, "supersecret\n", "setup", "admin", "--url", srv.URL, "--username", "admin", "--password-stdin", "--login")
	if err != nil {
		t.Fatalf("setup admin --login: %v\n%s", err, out)
	}
	if !strings.Contains(out, `Admin user "admin" created.`) || !strings.Contains(out, "Logged in to") {
		t.Fatalf("out = %q", out)
	}
	cfg, err := clicfg.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	p, ok := cfg.CurrentProfile()
	if !ok || p.Key != "nxr_minted" || p.KeyID != "key-1" {
		t.Fatalf("profile = %+v ok=%v", p, ok)
	}
}

func TestSetupBackupsList(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	mux := http.NewServeMux()
	mux.HandleFunc("/health", okHealth)
	mux.HandleFunc("/api/auth/setup/backups", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"backups":[{"filename":"b.tar.gz","size_bytes":2048,"mtime":"2026-06-20T09:30:15Z","restorable":true}]}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runNexctl(t, "", "setup", "backups", "--url", srv.URL)
	if err != nil {
		t.Fatalf("setup backups: %v\n%s", err, out)
	}
	if !strings.Contains(out, "b.tar.gz") || !strings.Contains(out, "FILENAME") {
		t.Fatalf("out = %q", out)
	}
}

func TestSetupBackupsQuiet(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	mux := http.NewServeMux()
	mux.HandleFunc("/health", okHealth)
	mux.HandleFunc("/api/auth/setup/backups", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"backups":[{"filename":"b.tar.gz","size_bytes":2048,"restorable":true}]}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runNexctl(t, "", "setup", "backups", "--url", srv.URL, "-q")
	if err != nil {
		t.Fatalf("setup backups -q: %v\n%s", err, out)
	}
	if strings.TrimSpace(out) != "b.tar.gz" {
		t.Fatalf("out = %q; want bare filename", out)
	}
}

func TestSetupRestoreRequiresExactlyOne(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	_, err := runNexctl(t, "y\n", "setup", "restore", "--url", "http://127.0.0.1:1")
	if err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("err = %v; want exactly-one error (neither name nor --file)", err)
	}
	_, err = runNexctl(t, "y\n", "setup", "restore", "x.tar.gz", "--file", "/tmp/x.tar.gz", "--url", "http://127.0.0.1:1")
	if err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("err = %v; want exactly-one error (both)", err)
	}
}

func TestSetupRestoreFromDiskConfirmed(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	called := false
	mux := http.NewServeMux()
	mux.HandleFunc("/health", okHealth)
	mux.HandleFunc("/api/auth/setup/restore/disk", func(w http.ResponseWriter, _ *http.Request) {
		called = true
		_, _ = w.Write([]byte(`{"success":true}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runNexctl(t, "", "setup", "restore", "b.tar.gz", "--url", srv.URL, "-y")
	if err != nil {
		t.Fatalf("setup restore: %v\n%s", err, out)
	}
	if !called {
		t.Fatal("restore/disk endpoint was not called")
	}
	if !strings.Contains(out, "Backup restored") {
		t.Fatalf("out = %q", out)
	}
}

// TestSetupRestoreMigratesFreshInstance covers the disaster-recovery flow on an
// un-migrated instance: the command's preflight runs pending migrations (bringing
// the instance to Ready so the setup zone is reachable) before the restore. Without
// this, the app-state gate blocks /api/auth/setup/restore with an opaque 503.
func TestSetupRestoreMigratesFreshInstance(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	migrated, restored := false, false
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "needs_migration"})
	})
	mux.HandleFunc("/api/migrate/run", func(w http.ResponseWriter, _ *http.Request) {
		migrated = true
		w.WriteHeader(http.StatusAccepted)
	})
	mux.HandleFunc("/api/migrate/status", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"state": "ready"})
	})
	mux.HandleFunc("/api/auth/setup/restore/disk", func(w http.ResponseWriter, _ *http.Request) {
		restored = true
		_, _ = w.Write([]byte(`{"success":true}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runNexctl(t, "", "setup", "restore", "b.tar.gz", "--url", srv.URL, "-y")
	if err != nil {
		t.Fatalf("setup restore (fresh instance): %v\n%s", err, out)
	}
	if !migrated {
		t.Fatal("preflight did not run migrations on the un-migrated instance")
	}
	if !restored {
		t.Fatal("restore endpoint was not reached after migrating to Ready")
	}
}

func TestSetupRestoreAborted(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	called := false
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/setup/restore/disk", func(w http.ResponseWriter, _ *http.Request) {
		called = true
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	// "n\n" declines the confirm prompt.
	out, err := runNexctl(t, "n\n", "setup", "restore", "b.tar.gz", "--url", srv.URL)
	if err != nil {
		t.Fatalf("setup restore (declined): %v\n%s", err, out)
	}
	if called {
		t.Fatal("restore must not call the server after the user declines")
	}
	if !strings.Contains(out, "Aborted.") {
		t.Fatalf("out = %q; want Aborted.", out)
	}
}
