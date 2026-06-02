package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConfirmInteractivePasswordMismatch(t *testing.T) {
	entries := []string{"first-secret", "second-secret"}
	i := 0
	read := func(string) (string, error) {
		v := entries[i]
		i++
		return v, nil
	}
	_, err := confirmInteractivePassword(read)
	if err == nil {
		t.Fatal("expected mismatch error, got nil")
	}
}

func TestConfirmInteractivePasswordMatch(t *testing.T) {
	read := func(string) (string, error) { return "supersecret", nil }
	pw, err := confirmInteractivePassword(read)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pw != "supersecret" {
		t.Fatalf("pw = %q; want supersecret", pw)
	}
}

func TestConfirmInteractivePasswordBothEmpty(t *testing.T) {
	// Two empty entries "match", so the mismatch check passes; the empty-password
	// guard must then reject them.
	read := func(string) (string, error) { return "", nil }
	_, err := confirmInteractivePassword(read)
	if err == nil || !strings.Contains(err.Error(), "password is required") {
		t.Fatalf("err = %v; want password-required error", err)
	}
}

// setupStub is a configurable httptest server for the setup command tests.
type setupStub struct {
	healthStatus  string // value for /health "status" (default "ok")
	adminStatus   int    // status code for /api/auth/setup/admin (default 201)
	adminMessage  string // {"message":...} body for 4xx admin responses
	adminLocation string // Location header for a 3xx admin response
}

func (s setupStub) server(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		status := s.healthStatus
		if status == "" {
			status = "ok"
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": status})
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

// runSetupCmd executes `setup` with the given args and piped stdin.
func runSetupCmd(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader(stdin))
	root.SetArgs(append([]string{"setup"}, args...))
	err := root.Execute()
	return out.String(), err
}

func TestSetupCreatesAdmin(t *testing.T) {
	srv := setupStub{}.server(t)
	out, err := runSetupCmd(t, "supersecret\n", "--url", srv.URL, "--username", "admin", "--password-stdin")
	if err != nil {
		t.Fatalf("setup: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, `Admin user "admin" created.`) {
		t.Fatalf("output = %q; want success message", out)
	}
}

func TestSetupAlreadyComplete(t *testing.T) {
	srv := setupStub{adminStatus: http.StatusForbidden, adminMessage: "setup already complete"}.server(t)
	_, err := runSetupCmd(t, "supersecret\n", "--url", srv.URL, "--username", "admin", "--password-stdin")
	if err == nil || !strings.Contains(err.Error(), "already") {
		t.Fatalf("err = %v; want already-complete error", err)
	}
}

func TestSetupValidationMessageSurfaced(t *testing.T) {
	srv := setupStub{adminStatus: http.StatusBadRequest, adminMessage: "password must be at least 8 characters"}.server(t)
	_, err := runSetupCmd(t, "short\n", "--url", srv.URL, "--username", "admin", "--password-stdin")
	if err == nil || !strings.Contains(err.Error(), "at least 8") {
		t.Fatalf("err = %v; want server validation message", err)
	}
}

func TestSetupRedirectToMigrate(t *testing.T) {
	srv := setupStub{adminStatus: http.StatusFound, adminLocation: "/migrate"}.server(t)
	_, err := runSetupCmd(t, "supersecret\n", "--url", srv.URL, "--username", "admin", "--password-stdin")
	if err == nil || !strings.Contains(err.Error(), "migrations are pending") {
		t.Fatalf("err = %v; want migrations-pending error", err)
	}
}

func TestSetupRedirectToDBError(t *testing.T) {
	srv := setupStub{adminStatus: http.StatusFound, adminLocation: "/db-error"}.server(t)
	_, err := runSetupCmd(t, "supersecret\n", "--url", srv.URL, "--username", "admin", "--password-stdin")
	if err == nil || !strings.Contains(err.Error(), "database is unavailable") {
		t.Fatalf("err = %v; want db-unavailable error", err)
	}
}

func TestSetupUnhealthyPreflightAborts(t *testing.T) {
	srv := setupStub{healthStatus: "needs_migration"}.server(t)
	_, err := runSetupCmd(t, "supersecret\n", "--url", srv.URL, "--username", "admin", "--password-stdin")
	if err == nil || !strings.Contains(err.Error(), "--migrate") {
		t.Fatalf("err = %v; want pending-migrations preflight error", err)
	}
}

func TestSetupMigrationFailedPreflight(t *testing.T) {
	srv := setupStub{healthStatus: "migration_failed"}.server(t)
	_, err := runSetupCmd(t, "supersecret\n", "--url", srv.URL, "--username", "admin", "--password-stdin")
	if err == nil || !strings.Contains(err.Error(), "previously failed") {
		t.Fatalf("err = %v; want previously-failed message (not 'pending')", err)
	}
}

func TestSetupMigratingPreflight(t *testing.T) {
	srv := setupStub{healthStatus: "migrating"}.server(t)
	_, err := runSetupCmd(t, "supersecret\n", "--url", srv.URL, "--username", "admin", "--password-stdin")
	if err == nil || !strings.Contains(err.Error(), "already in progress") {
		t.Fatalf("err = %v; want already-in-progress message", err)
	}
}

func TestSetupConnectionRefused(t *testing.T) {
	srv := setupStub{}.server(t)
	url := srv.URL
	srv.Close()
	_, err := runSetupCmd(t, "supersecret\n", "--url", url, "--username", "admin", "--password-stdin")
	if err == nil || !strings.Contains(err.Error(), "could not reach server") {
		t.Fatalf("err = %v; want reach-server error", err)
	}
}

func TestSetupMissingUsernameWithPasswordStdin(t *testing.T) {
	_, err := runSetupCmd(t, "supersecret\n", "--url", "http://127.0.0.1:1", "--password-stdin")
	if err == nil || !strings.Contains(err.Error(), "--username is required") {
		t.Fatalf("err = %v; want username-required error", err)
	}
}

func TestSetupNoPasswordSourceNonTTY(t *testing.T) {
	_, err := runSetupCmd(t, "", "--url", "http://127.0.0.1:1", "--username", "admin")
	if err == nil || !strings.Contains(err.Error(), "no password") {
		t.Fatalf("err = %v; want no-password error", err)
	}
}

// migrateStub serves /health (needs_migration), /api/migrate/run, and
// /api/migrate/status (which returns finalState after run is called), plus a
// 201 /api/auth/setup/admin.
type migrateStub struct {
	finalState string // "ready" or "migration_failed"
	ranMigrate bool
}

func (s *migrateStub) server(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "needs_migration"})
	})
	mux.HandleFunc("/api/migrate/run", func(w http.ResponseWriter, _ *http.Request) {
		s.ranMigrate = true
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "migration started"})
	})
	mux.HandleFunc("/api/migrate/status", func(w http.ResponseWriter, _ *http.Request) {
		state := "needs_migration"
		if s.ranMigrate {
			state = s.finalState
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"state": state, "pending_count": 0})
	})
	mux.HandleFunc("/api/auth/setup/admin", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestSetupMigrateHappyPath(t *testing.T) {
	stub := &migrateStub{finalState: "ready"}
	srv := stub.server(t)
	out, err := runSetupCmd(t, "supersecret\n", "--url", srv.URL, "--username", "admin", "--password-stdin", "--migrate")
	if err != nil {
		t.Fatalf("setup --migrate: %v\noutput: %s", err, out)
	}
	if !stub.ranMigrate {
		t.Fatal("migrations were not triggered")
	}
	if !strings.Contains(out, "Migrations complete.") || !strings.Contains(out, `Admin user "admin" created.`) {
		t.Fatalf("output = %q; want migrate + create messages", out)
	}
}

func TestSetupMigrateFailed(t *testing.T) {
	stub := &migrateStub{finalState: "migration_failed"}
	srv := stub.server(t)
	_, err := runSetupCmd(t, "supersecret\n", "--url", srv.URL, "--username", "admin", "--password-stdin", "--migrate")
	if err == nil || !strings.Contains(err.Error(), "migrations failed") {
		t.Fatalf("err = %v; want migrations-failed error", err)
	}
}

func TestSetupMigrateFailedSurfacesDetail(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "needs_migration"})
	})
	mux.HandleFunc("/api/migrate/run", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})
	mux.HandleFunc("/api/migrate/status", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"state": "migration_failed", "error": "migration 003 failed: boom",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	_, err := runSetupCmd(t, "supersecret\n", "--url", srv.URL, "--username", "admin", "--password-stdin", "--migrate")
	if err == nil || !strings.Contains(err.Error(), "migration 003 failed: boom") {
		t.Fatalf("err = %v; want surfaced server failure detail", err)
	}
}

// TestSetupMigrateWhenAlreadyMigrating covers health="migrating" + --migrate:
// the POST /api/migrate/run returns 409, which RunMigrations tolerates, and the
// command then polls to completion.
func TestSetupMigrateWhenAlreadyMigrating(t *testing.T) {
	polls := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "migrating"})
	})
	mux.HandleFunc("/api/migrate/run", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "migration already in progress"})
	})
	mux.HandleFunc("/api/migrate/status", func(w http.ResponseWriter, _ *http.Request) {
		polls++
		_ = json.NewEncoder(w).Encode(map[string]any{"state": "ready"})
	})
	mux.HandleFunc("/api/auth/setup/admin", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runSetupCmd(t, "supersecret\n", "--url", srv.URL, "--username", "admin", "--password-stdin", "--migrate")
	if err != nil {
		t.Fatalf("setup --migrate (already migrating): %v\noutput: %s", err, out)
	}
	if polls == 0 {
		t.Fatal("expected the command to poll migration status")
	}
	if !strings.Contains(out, `Admin user "admin" created.`) {
		t.Fatalf("output = %q; want success after waiting for in-progress migration", out)
	}
}
