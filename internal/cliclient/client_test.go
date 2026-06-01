package cliclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
	if got := err.Error(); got == "" || !strings.Contains(got, "incorrect username or password") {
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
	if !strings.Contains(err.Error(), "unauthorized") {
		t.Fatalf("error = %q, want server message", err.Error())
	}
}

// errServer returns a server that answers every request with the given status
// and an Echo-style {"message":...} body, for exercising client error paths.
func errServer(t *testing.T, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "boom"})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestCreateAPIKeyServerError(t *testing.T) {
	c := New(errServer(t, http.StatusInternalServerError).URL)
	_, _, err := c.CreateAPIKey("sess-123", "cli@host")
	if err == nil {
		t.Fatal("expected error on 500")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error = %q, want server message", err.Error())
	}
}

func TestRevokeWithBearerServerError(t *testing.T) {
	// A non-204 (here 404) must surface as an error: the key was not found.
	c := New(errServer(t, http.StatusNotFound).URL)
	if err := c.RevokeAPIKeyWithBearer("nxr_rawkey", "missing"); err == nil {
		t.Fatal("expected error on 404 revoke")
	}
}

func TestLogoutServerError(t *testing.T) {
	c := New(errServer(t, http.StatusInternalServerError).URL)
	if err := c.Logout("sess-123"); err == nil {
		t.Fatal("expected error on 500 logout")
	}
}

func TestListAPIKeys(t *testing.T) {
	var gotAuth string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/api-keys", func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":"k1","name":"laptop","scopes":"write","last_used_at":null,"created_at":"2026-01-01T00:00:00Z","expires_at":null}]`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	keys, err := New(srv.URL).ListAPIKeys("nxr_secret")
	if err != nil {
		t.Fatalf("ListAPIKeys: %v", err)
	}
	if gotAuth != "Bearer nxr_secret" {
		t.Fatalf("auth = %q, want Bearer nxr_secret", gotAuth)
	}
	if len(keys) != 1 || keys[0].ID != "k1" || keys[0].Name != "laptop" {
		t.Fatalf("keys = %+v, want one key k1/laptop", keys)
	}
	if keys[0].LastUsedAt != nil {
		t.Fatalf("LastUsedAt = %v, want nil", keys[0].LastUsedAt)
	}
}

func TestCreateAPIKeyWithBearer(t *testing.T) {
	var gotAuth string
	var gotBody map[string]string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/api-keys", func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"k2","name":"ci","scopes":"read","key":"nxr_rawkey","created_at":"2026-01-01T00:00:00Z","expires_at":null}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	key, err := New(srv.URL).CreateAPIKeyWithBearer("nxr_secret", "ci", "read", nil)
	if err != nil {
		t.Fatalf("CreateAPIKeyWithBearer: %v", err)
	}
	if gotAuth != "Bearer nxr_secret" {
		t.Fatalf("auth = %q, want Bearer nxr_secret", gotAuth)
	}
	if _, ok := gotBody["expires_at"]; ok {
		t.Fatalf("expires_at should be omitted when nil, got body %+v", gotBody)
	}
	if gotBody["name"] != "ci" || gotBody["scopes"] != "read" {
		t.Fatalf("body = %+v, want name=ci scopes=read", gotBody)
	}
	if key.Key != "nxr_rawkey" || key.ID != "k2" {
		t.Fatalf("key = %+v, want raw key nxr_rawkey id k2", key)
	}
}

func TestCreateAPIKeyWithBearerSendsExpiry(t *testing.T) {
	var gotBody map[string]string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/api-keys", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"k3","name":"temp","scopes":"write","key":"nxr_x","created_at":"2026-01-01T00:00:00Z","expires_at":"2027-01-01T00:00:00Z"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	exp := "2027-01-01T00:00:00Z"
	if _, err := New(srv.URL).CreateAPIKeyWithBearer("nxr_secret", "temp", "write", &exp); err != nil {
		t.Fatalf("CreateAPIKeyWithBearer: %v", err)
	}
	if gotBody["expires_at"] != exp {
		t.Fatalf("expires_at = %q, want %q", gotBody["expires_at"], exp)
	}
}
