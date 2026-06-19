package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// runAdmin drives newRootCmd with the given args, pre-seeded against srvURL.
// Returns stdout+stderr combined and any execution error.
func runAdmin(t *testing.T, srvURL string, stdin string, args ...string) (string, error) {
	t.Helper()
	seedProfile(t, srvURL)
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader(stdin))
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

// sampleUser returns a standard admin user payload for reuse across tests.
func sampleUser(id, username string, active, admin bool) map[string]any {
	return map[string]any{
		"id":         id,
		"username":   username,
		"is_active":  active,
		"is_admin":   admin,
		"created_at": "2026-06-18T00:00:00Z",
		"updated_at": "2026-06-18T00:00:00Z",
	}
}

func TestAdminUserListTable(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/admin/users", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		_ = json.NewEncoder(w).Encode([]any{
			sampleUser("u-1", "alice", true, true),
			sampleUser("u-2", "bob", true, false),
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runAdmin(t, srv.URL, "", "admin", "user", "list")
	if err != nil {
		t.Fatalf("admin user list: %v\n%s", err, out)
	}
	if !strings.Contains(out, "u-1") || !strings.Contains(out, "u-2") {
		t.Errorf("table missing ids: %q", out)
	}
	if !strings.Contains(out, "alice") || !strings.Contains(out, "bob") {
		t.Errorf("table missing usernames: %q", out)
	}
	if !strings.Contains(out, "USERNAME") {
		t.Errorf("table missing header: %q", out)
	}
}

func TestAdminUserListQuiet(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/admin/users", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]any{
			sampleUser("u-1", "alice", true, true),
			sampleUser("u-2", "bob", true, false),
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runAdmin(t, srv.URL, "", "admin", "user", "list", "-q")
	if err != nil {
		t.Fatalf("admin user list -q: %v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 || lines[0] != "u-1" || lines[1] != "u-2" {
		t.Errorf("quiet ids = %q, want u-1 and u-2 on separate lines", out)
	}
}

func TestAdminUserShow(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/admin/users/u-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		_ = json.NewEncoder(w).Encode(sampleUser("u-1", "alice", true, true))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runAdmin(t, srv.URL, "", "admin", "user", "show", "u-1")
	if err != nil {
		t.Fatalf("admin user show: %v\n%s", err, out)
	}
	if !strings.Contains(out, "u-1") || !strings.Contains(out, "alice") {
		t.Errorf("output missing user fields: %q", out)
	}
	if !strings.Contains(out, "username") {
		t.Errorf("output missing labels: %q", out)
	}
}

func TestAdminUserCreateSendsCorrectBody(t *testing.T) {
	var gotBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/admin/users", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(sampleUser("u-new", "alice", true, true))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runAdmin(t, srv.URL, "", "admin", "user", "create", "alice", "--admin", "--password", "pw")
	if err != nil {
		t.Fatalf("admin user create: %v\n%s", err, out)
	}
	if gotBody["username"] != "alice" {
		t.Errorf("username = %v, want alice", gotBody["username"])
	}
	if gotBody["password"] != "pw" {
		t.Errorf("password = %v, want pw", gotBody["password"])
	}
	if gotBody["is_admin"] != true {
		t.Errorf("is_admin = %v, want true", gotBody["is_admin"])
	}
	if !strings.Contains(out, "u-new") {
		t.Errorf("output missing new user id: %q", out)
	}
}

func TestAdminUserEnableSendsCorrectBody(t *testing.T) {
	var gotBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/admin/users/u-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(sampleUser("u-1", "alice", true, false))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runAdmin(t, srv.URL, "", "admin", "user", "enable", "u-1")
	if err != nil {
		t.Fatalf("admin user enable: %v\n%s", err, out)
	}
	if gotBody["is_active"] != true {
		t.Errorf("is_active = %v, want true", gotBody["is_active"])
	}
	if len(gotBody) != 1 {
		t.Errorf("PUT body has %d keys, want exactly 1: %v", len(gotBody), gotBody)
	}
	if !strings.Contains(out, "enabled user u-1") {
		t.Errorf("output = %q, want confirmation", out)
	}
}

func TestAdminUserDisableSendsCorrectBody(t *testing.T) {
	var gotBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/admin/users/u-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(sampleUser("u-1", "alice", false, false))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runAdmin(t, srv.URL, "", "admin", "user", "disable", "u-1", "-y")
	if err != nil {
		t.Fatalf("admin user disable -y: %v\n%s", err, out)
	}
	if gotBody["is_active"] != false {
		t.Errorf("is_active = %v, want false", gotBody["is_active"])
	}
	if len(gotBody) != 1 {
		t.Errorf("PUT body has %d keys, want exactly 1: %v", len(gotBody), gotBody)
	}
	if !strings.Contains(out, "disabled user u-1") {
		t.Errorf("output = %q, want confirmation", out)
	}
}

func TestAdminUserDisableAbort(t *testing.T) {
	var putHit bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/admin/users/u-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			putHit = true
			_ = json.NewEncoder(w).Encode(sampleUser("u-1", "alice", false, false))
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runAdmin(t, srv.URL, "n\n", "admin", "user", "disable", "u-1")
	if err != nil {
		t.Fatalf("admin user disable (abort): %v\n%s", err, out)
	}
	if putHit {
		t.Fatal("PUT must not be sent when aborted")
	}
	if !strings.Contains(out, "Aborted.") {
		t.Errorf("output = %q, want 'Aborted.'", out)
	}
}

func TestAdminUserSetAdminGrant(t *testing.T) {
	var gotBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/admin/users/u-2", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(sampleUser("u-2", "bob", true, true))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runAdmin(t, srv.URL, "", "admin", "user", "set-admin", "u-2")
	if err != nil {
		t.Fatalf("admin user set-admin: %v\n%s", err, out)
	}
	if gotBody["is_admin"] != true {
		t.Errorf("is_admin = %v, want true", gotBody["is_admin"])
	}
	if len(gotBody) != 1 {
		t.Errorf("PUT body has %d keys, want exactly 1: %v", len(gotBody), gotBody)
	}
	if !strings.Contains(out, "granted admin") {
		t.Errorf("output = %q, want 'granted admin'", out)
	}
}

func TestAdminUserSetAdminRevoke(t *testing.T) {
	var gotBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/admin/users/u-2", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(sampleUser("u-2", "bob", true, false))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runAdmin(t, srv.URL, "", "admin", "user", "set-admin", "u-2", "--revoke")
	if err != nil {
		t.Fatalf("admin user set-admin --revoke: %v\n%s", err, out)
	}
	if gotBody["is_admin"] != false {
		t.Errorf("is_admin = %v, want false", gotBody["is_admin"])
	}
	if !strings.Contains(out, "revoked admin") {
		t.Errorf("output = %q, want 'revoked admin'", out)
	}
}

func TestAdminUserPasswdSendsCorrectBody(t *testing.T) {
	var gotBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/admin/users/u-1/password", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runAdmin(t, srv.URL, "", "admin", "user", "passwd", "u-1", "--password", "pw")
	if err != nil {
		t.Fatalf("admin user passwd: %v\n%s", err, out)
	}
	if gotBody["new_password"] != "pw" {
		t.Errorf("new_password = %v, want pw", gotBody["new_password"])
	}
	if !strings.Contains(out, "password updated") {
		t.Errorf("output = %q, want confirmation", out)
	}
}

func TestAdminUserRmWithYesFetchesImpactThenDeletes(t *testing.T) {
	var getHit, deleteHit bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/admin/users/u-1/deletion-impact", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		getHit = true
		_ = json.NewEncoder(w).Encode(map[string]any{
			"user_id": "u-1", "username": "alice",
			"total_games": 10, "total_tags": 3,
			"total_import_jobs": 1, "total_export_jobs": 2,
			"total_sync_jobs": 0, "total_sync_configs": 1,
			"total_sessions": 2, "warning": "",
		})
	})
	mux.HandleFunc("/api/auth/admin/users/u-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		deleteHit = true
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "user deleted"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runAdmin(t, srv.URL, "", "admin", "user", "rm", "u-1", "-y")
	if err != nil {
		t.Fatalf("admin user rm -y: %v\n%s", err, out)
	}
	if !getHit {
		t.Fatal("GET deletion-impact not called")
	}
	if !deleteHit {
		t.Fatal("DELETE not called")
	}
	if !strings.Contains(out, "alice") || !strings.Contains(out, "10") {
		t.Errorf("output missing impact details: %q", out)
	}
	if !strings.Contains(out, "removed user u-1") {
		t.Errorf("output = %q, want 'removed user u-1'", out)
	}
}

func TestAdminUserRmAbortSkipsDelete(t *testing.T) {
	var deleteHit bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/admin/users/u-1/deletion-impact", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"user_id": "u-1", "username": "alice",
			"total_games": 5, "total_tags": 1,
			"total_import_jobs": 0, "total_export_jobs": 0,
			"total_sync_jobs": 0, "total_sync_configs": 0,
			"total_sessions": 1, "warning": "",
		})
	})
	mux.HandleFunc("/api/auth/admin/users/u-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleteHit = true
			w.WriteHeader(http.StatusOK)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runAdmin(t, srv.URL, "n\n", "admin", "user", "rm", "u-1")
	if err != nil {
		t.Fatalf("admin user rm (abort): %v\n%s", err, out)
	}
	if deleteHit {
		t.Fatal("DELETE must not be called when aborted")
	}
	if !strings.Contains(out, "Aborted.") {
		t.Errorf("output = %q, want 'Aborted.'", out)
	}
}

func TestAdminResetWithYesPrintsCount(t *testing.T) {
	var postHit bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/admin/reset", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		postHit = true
		_ = json.NewEncoder(w).Encode(map[string]any{"deleted": 42})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runAdmin(t, srv.URL, "", "admin", "reset", "-y")
	if err != nil {
		t.Fatalf("admin reset -y: %v\n%s", err, out)
	}
	if !postHit {
		t.Fatal("POST /api/auth/admin/reset not called")
	}
	if !strings.Contains(out, "deleted 42 games") {
		t.Errorf("output = %q, want 'deleted 42 games'", out)
	}
}

func TestAdminResetAbort(t *testing.T) {
	var postHit bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/admin/reset", func(w http.ResponseWriter, _ *http.Request) {
		postHit = true
		_ = json.NewEncoder(w).Encode(map[string]any{"deleted": 0})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runAdmin(t, srv.URL, "n\n", "admin", "reset")
	if err != nil {
		t.Fatalf("admin reset (abort): %v\n%s", err, out)
	}
	if postHit {
		t.Fatal("POST must not be called when aborted")
	}
	if !strings.Contains(out, "Aborted.") {
		t.Errorf("output = %q, want 'Aborted.'", out)
	}
}

func TestAdminUserDisableSelf400Surfaces(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/admin/users/self", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "Cannot deactivate your own account"})
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	_, err := runAdmin(t, srv.URL, "", "admin", "user", "disable", "self", "-y")
	if err == nil {
		t.Fatal("expected error from 400 response")
	}
	if !strings.Contains(err.Error(), "Cannot deactivate your own account") {
		t.Errorf("error = %v, want it to mention 'Cannot deactivate your own account'", err)
	}
}

// Verify the admin command tree is wired correctly.
func TestAdminCmdStructure(t *testing.T) {
	cmd := newAdminCmd()
	if cmd.Use != "admin" {
		t.Errorf("Use = %q, want admin", cmd.Use)
	}
	subNames := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		subNames[sub.Name()] = true
	}
	for _, want := range []string{"user", "reset"} {
		if !subNames[want] {
			t.Errorf("missing subcommand %q", want)
		}
	}

	userCmd := newAdminUserCmd()
	userSubNames := make(map[string]bool)
	for _, sub := range userCmd.Commands() {
		userSubNames[sub.Name()] = true
	}
	for _, want := range []string{"list", "show", "create", "enable", "disable", "set-admin", "passwd", "rm"} {
		if !userSubNames[want] {
			t.Errorf("missing user subcommand %q", want)
		}
	}
	_ = fmt.Sprintf // keep fmt imported
}
