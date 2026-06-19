package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func withTestVersion(t *testing.T) {
	t.Helper()
	prevV, prevC := version, commit
	version, commit = "9.9.9-test", "cafef00d"
	t.Cleanup(func() { version, commit = prevV, prevC })
}

// versionServer returns an httptest server answering GET /api/version with the
// given build, recording the Authorization header it saw.
func versionServer(t *testing.T, gotAuth *string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/version", func(w http.ResponseWriter, r *http.Request) {
		if gotAuth != nil {
			*gotAuth = r.Header.Get("Authorization")
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"version": "1.0.5-srv",
			"commit":  "deadbeef",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestRunVersion_TextWithServer(t *testing.T) {
	withTestVersion(t)
	var gotAuth string
	srv := versionServer(t, &gotAuth)

	var buf bytes.Buffer
	if err := runVersion(&buf, srv.URL, "nxr_key", false); err != nil {
		t.Fatalf("runVersion: %v", err)
	}
	got := buf.String()
	for _, want := range []string{"nexctl", "9.9.9-test", "cafef00d", "server", "1.0.5-srv", "deadbeef"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
	if gotAuth != "Bearer nxr_key" {
		t.Errorf("sent auth = %q, want Bearer nxr_key", gotAuth)
	}
}

func TestRunVersion_JSONWithServer(t *testing.T) {
	withTestVersion(t)
	srv := versionServer(t, nil)

	var buf bytes.Buffer
	if err := runVersion(&buf, srv.URL, "", true); err != nil {
		t.Fatalf("runVersion: %v", err)
	}
	var out struct {
		Client struct{ Version, Commit string }
		Server struct {
			Version, Commit string
			Error           string
		}
	}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("decode json: %v\n%s", err, buf.String())
	}
	if out.Client.Version != "9.9.9-test" || out.Client.Commit != "cafef00d" {
		t.Errorf("client = %+v", out.Client)
	}
	if out.Server.Version != "1.0.5-srv" || out.Server.Commit != "deadbeef" {
		t.Errorf("server = %+v", out.Server)
	}
	if out.Server.Error != "" {
		t.Errorf("unexpected server error %q", out.Server.Error)
	}
}

func TestRunVersion_ServerUnreachableTextExitsZero(t *testing.T) {
	withTestVersion(t)
	// Stand up a server then close it so the URL is valid but refuses connections.
	srv := versionServer(t, nil)
	url := srv.URL
	srv.Close()

	var buf bytes.Buffer
	if err := runVersion(&buf, url, "", false); err != nil {
		t.Fatalf("runVersion must not return an error when the server is down: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "9.9.9-test") {
		t.Errorf("client version missing:\n%s", got)
	}
	if !strings.Contains(got, "server") || !strings.Contains(got, "unavailable") {
		t.Errorf("expected server unavailable note:\n%s", got)
	}
}

func TestRunVersion_NoProfileConfigured(t *testing.T) {
	withTestVersion(t)

	var buf bytes.Buffer
	if err := runVersion(&buf, "", "", true); err != nil {
		t.Fatalf("runVersion: %v", err)
	}
	var out struct {
		Client struct{ Version string }
		Server struct{ Error string }
	}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("decode json: %v\n%s", err, buf.String())
	}
	if out.Client.Version != "9.9.9-test" {
		t.Errorf("client version = %q", out.Client.Version)
	}
	if !strings.Contains(out.Server.Error, "no server configured") {
		t.Errorf("server error = %q, want no-server-configured", out.Server.Error)
	}
}
