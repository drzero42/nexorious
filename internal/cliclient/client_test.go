package cliclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestServer returns an httptest server emulating the subset of /api/auth/*
// the CLI uses, plus a record of what it received.
type captured struct {
	createCookie string
	revokeAuth   string
	revokeID     string
	logoutCookie string
	meAuth       string
}

func newTestServer(t *testing.T, cap *captured) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/api/auth/login", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["username"] != "alice" || body["password"] != "pw" {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "incorrect username or password"})
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "session_id", Value: "sess-123"})
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"username": "alice"})
	})

	mux.HandleFunc("/api/auth/api-keys", func(w http.ResponseWriter, r *http.Request) {
		if ck, _ := r.Cookie("session_id"); ck != nil {
			cap.createCookie = ck.Value
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "key-id-1", "key": "nxr_rawkey"})
	})

	mux.HandleFunc("/api/auth/api-keys/", func(w http.ResponseWriter, r *http.Request) {
		cap.revokeID = r.URL.Path[len("/api/auth/api-keys/"):]
		if ck, _ := r.Cookie("session_id"); ck != nil {
			cap.revokeAuth = "cookie:" + ck.Value
		} else {
			cap.revokeAuth = r.Header.Get("Authorization")
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("/api/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		if ck, _ := r.Cookie("session_id"); ck != nil {
			cap.logoutCookie = ck.Value
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "ok"})
	})

	mux.HandleFunc("/api/auth/me", func(w http.ResponseWriter, r *http.Request) {
		cap.meAuth = r.Header.Get("Authorization")
		if cap.meAuth != "Bearer nxr_rawkey" {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "unauthorized"})
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"username": "alice"})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestLoginReturnsSessionCookie(t *testing.T) {
	var cap captured
	srv := newTestServer(t, &cap)
	c := New(srv.URL)

	sess, err := c.Login("alice", "pw")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if sess != "sess-123" {
		t.Fatalf("session = %q, want sess-123", sess)
	}
}

func TestLoginBadCredsReturnsServerMessage(t *testing.T) {
	var cap captured
	srv := newTestServer(t, &cap)
	c := New(srv.URL)

	_, err := c.Login("alice", "wrong")
	if err == nil {
		t.Fatal("expected error for bad creds")
	}
	if got := err.Error(); got == "" || !contains(got, "incorrect username or password") {
		t.Fatalf("error = %q, want server message", got)
	}
}

func TestCreateAPIKeySendsCookieReturnsKey(t *testing.T) {
	var cap captured
	srv := newTestServer(t, &cap)
	c := New(srv.URL)

	key, id, err := c.CreateAPIKey("sess-123", "cli@host")
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if key != "nxr_rawkey" || id != "key-id-1" {
		t.Fatalf("got key=%q id=%q", key, id)
	}
	if cap.createCookie != "sess-123" {
		t.Fatalf("server saw cookie %q, want sess-123", cap.createCookie)
	}
}

func TestRevokeWithCookie(t *testing.T) {
	var cap captured
	srv := newTestServer(t, &cap)
	c := New(srv.URL)

	if err := c.RevokeAPIKeyWithCookie("sess-123", "key-id-1"); err != nil {
		t.Fatalf("RevokeAPIKeyWithCookie: %v", err)
	}
	if cap.revokeID != "key-id-1" {
		t.Fatalf("revoked id = %q", cap.revokeID)
	}
	if cap.revokeAuth != "cookie:sess-123" {
		t.Fatalf("revoke auth = %q, want cookie:sess-123", cap.revokeAuth)
	}
}

func TestRevokeWithBearer(t *testing.T) {
	var cap captured
	srv := newTestServer(t, &cap)
	c := New(srv.URL)

	if err := c.RevokeAPIKeyWithBearer("nxr_rawkey", "key-id-1"); err != nil {
		t.Fatalf("RevokeAPIKeyWithBearer: %v", err)
	}
	if cap.revokeAuth != "Bearer nxr_rawkey" {
		t.Fatalf("revoke auth = %q, want Bearer nxr_rawkey", cap.revokeAuth)
	}
}

func TestLogoutSendsCookie(t *testing.T) {
	var cap captured
	srv := newTestServer(t, &cap)
	c := New(srv.URL)

	if err := c.Logout("sess-123"); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if cap.logoutCookie != "sess-123" {
		t.Fatalf("logout cookie = %q", cap.logoutCookie)
	}
}

func TestMeReturnsUsername(t *testing.T) {
	var cap captured
	srv := newTestServer(t, &cap)
	c := New(srv.URL)

	user, err := c.Me("nxr_rawkey")
	if err != nil {
		t.Fatalf("Me: %v", err)
	}
	if user != "alice" {
		t.Fatalf("user = %q, want alice", user)
	}
}

func TestMeUnauthorized(t *testing.T) {
	var cap captured
	srv := newTestServer(t, &cap)
	c := New(srv.URL)

	_, err := c.Me("nxr_wrong")
	if err == nil {
		t.Fatal("expected error for bad key")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
