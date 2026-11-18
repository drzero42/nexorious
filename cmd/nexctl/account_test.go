package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/drzero42/nexorious/internal/clicfg"
)

func accountTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/login", func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "session_id", Value: "sess-xyz"})
		_ = json.NewEncoder(w).Encode(map[string]string{"username": "alice"})
	})
	mux.HandleFunc("/api/auth/api-keys", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "key-1", "key": "nxr_minted"})
	})
	mux.HandleFunc("/api/auth/logout", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "ok"})
	})
	mux.HandleFunc("/api/auth/me", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"username": "alice"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestAccountLoginWritesConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("NEXORIOUS_PASSWORD", "pw")
	srv := accountTestServer(t)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"account", "login", "--url", srv.URL, "--username", "alice"})
	if err := root.Execute(); err != nil {
		t.Fatalf("login: %v\n%s", err, out.String())
	}

	cfg, _ := clicfg.Load()
	p, ok := cfg.CurrentProfile()
	if !ok || p.Key != "nxr_minted" || p.URL != srv.URL {
		t.Fatalf("stored profile = %+v ok=%v", p, ok)
	}
}

func TestAccountWhoami(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	srv := accountTestServer(t)
	seed := &clicfg.Config{}
	seed.SetProfile("default", clicfg.Profile{URL: srv.URL, Username: "alice", Key: "nxr_x", KeyID: "key-1"})
	if err := clicfg.Save(seed); err != nil {
		t.Fatalf("seed: %v", err)
	}

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"account", "whoami"})
	if err := root.Execute(); err != nil {
		t.Fatalf("whoami: %v\n%s", err, out.String())
	}
	if got := out.String(); got == "" || !bytes.Contains([]byte(got), []byte("alice")) {
		t.Fatalf("whoami output = %q", got)
	}
}

// TestTopLevelLoginAlias verifies `nexctl login` works identically to
// `nexctl account login`.
func TestTopLevelLoginAlias(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("NEXORIOUS_PASSWORD", "pw")
	srv := accountTestServer(t)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"login", "--url", srv.URL, "--username", "alice"})
	if err := root.Execute(); err != nil {
		t.Fatalf("top-level login: %v\n%s", err, out.String())
	}
	cfg, _ := clicfg.Load()
	if p, ok := cfg.CurrentProfile(); !ok || p.Key != "nxr_minted" {
		t.Fatalf("top-level login did not store key: %+v ok=%v", p, ok)
	}
}

// loginScopeServer is accountTestServer that records the scope each
// POST /api/auth/api-keys asked for.
func loginScopeServer(t *testing.T, gotScope *string, calls *int) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/login", func(w http.ResponseWriter, _ *http.Request) {
		*calls++
		http.SetCookie(w, &http.Cookie{Name: "session_id", Value: "sess-xyz"})
		_ = json.NewEncoder(w).Encode(map[string]string{"username": "alice"})
	})
	mux.HandleFunc("/api/auth/api-keys", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		*gotScope = body["scopes"]
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "key-1", "key": "nxr_minted"})
	})
	mux.HandleFunc("/api/auth/logout", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "ok"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// `account login --scope` picks the minted key's scope (#1286); without the
// flag it stays write, so existing logins are unchanged.
func TestAccountLoginScope(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"default is write", nil, "write"},
		{"read", []string{"--scope", "read"}, "read"},
		{"write", []string{"--scope", "write"}, "write"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("NEXORIOUS_PASSWORD", "pw")
			var gotScope string
			var calls int
			srv := loginScopeServer(t, &gotScope, &calls)

			root := newRootCmd()
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&out)
			root.SetArgs(append([]string{"account", "login", "--url", srv.URL, "--username", "alice"}, tc.args...))
			if err := root.Execute(); err != nil {
				t.Fatalf("login: %v\n%s", err, out.String())
			}
			if gotScope != tc.want {
				t.Errorf("minted scope = %q, want %q", gotScope, tc.want)
			}
			if !bytes.Contains(out.Bytes(), []byte(tc.want+"-scoped")) {
				t.Errorf("output does not name the %s scope: %q", tc.want, out.String())
			}
		})
	}
}

func TestAccountLoginRejectsUnknownScope(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("NEXORIOUS_PASSWORD", "pw")
	var gotScope string
	var calls int
	srv := loginScopeServer(t, &gotScope, &calls)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"account", "login", "--url", srv.URL, "--username", "alice", "--scope", "admin"})
	if err := root.Execute(); err == nil {
		t.Fatal("login --scope admin succeeded, want an error")
	}
	if calls != 0 {
		t.Errorf("server saw %d login calls, want 0 (validate before logging in)", calls)
	}
}

// `--profile agent account login|logout` targets only the agent profile: the
// current profile must stay put, or everyday commands would silently run on
// the agent's (read) key (#1286).
func TestAccountLoginLogoutWithProfileKeepsCurrent(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("NEXORIOUS_PASSWORD", "pw")
	srv := accountTestServer(t)
	seed := &clicfg.Config{}
	seed.SetProfile("default", clicfg.Profile{URL: srv.URL, Username: "alice", Key: "nxr_main", KeyID: "key-main"})
	if err := clicfg.Save(seed); err != nil {
		t.Fatalf("seed: %v", err)
	}

	for _, args := range [][]string{
		{"--profile", "agent", "account", "login", "--url", srv.URL, "--username", "alice", "--scope", "read"},
		{"--profile", "agent", "account", "logout"},
	} {
		root := newRootCmd()
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs(args)
		if err := root.Execute(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out.String())
		}
		cfg, _ := clicfg.Load()
		if cfg.CurrentName() != "default" {
			t.Fatalf("after %v: current profile = %q, want default", args, cfg.CurrentName())
		}
		if p, _ := cfg.ProfileNamed("default"); p.Key != "nxr_main" {
			t.Fatalf("after %v: default profile key = %q, want untouched", args, p.Key)
		}
	}
}

// The first login on a fresh config still makes its profile current.
func TestAccountLoginFirstProfileBecomesCurrent(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("NEXORIOUS_PASSWORD", "pw")
	srv := accountTestServer(t)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"--profile", "work", "account", "login", "--url", srv.URL, "--username", "alice"})
	if err := root.Execute(); err != nil {
		t.Fatalf("login: %v\n%s", err, out.String())
	}
	cfg, _ := clicfg.Load()
	if cfg.CurrentName() != "work" {
		t.Fatalf("current profile = %q, want work", cfg.CurrentName())
	}
}
