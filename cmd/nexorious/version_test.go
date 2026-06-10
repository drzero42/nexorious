package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/drzero42/nexorious/internal/services/updatecheck"
)

// releaseServer returns an httptest server mimicking the GitHub
// "latest release" endpoint, and a counter of requests received.
func releaseServer(t *testing.T, status int, body string) (*httptest.Server, *int) {
	t.Helper()
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &requests
}

func TestCheckForUpdate(t *testing.T) {
	const release = `{"tag_name":"v1.1.0","html_url":"https://github.com/drzero42/nexorious/releases/tag/v1.1.0"}`

	t.Run("newer release available", func(t *testing.T) {
		srv, _ := releaseServer(t, http.StatusOK, release)
		client := updatecheck.NewClientWithBaseURL(srv.URL)

		line, failed := checkForUpdate(context.Background(), client, "1.0.0")
		if failed {
			t.Fatalf("failed = true, want false; line = %q", line)
		}
		if !strings.Contains(line, "Update available: 1.1.0") ||
			!strings.Contains(line, "releases/tag/v1.1.0") {
			t.Errorf("line = %q, want update notice with version and URL", line)
		}
	})

	t.Run("up to date", func(t *testing.T) {
		srv, _ := releaseServer(t, http.StatusOK, release)
		client := updatecheck.NewClientWithBaseURL(srv.URL)

		line, failed := checkForUpdate(context.Background(), client, "1.1.0")
		if failed {
			t.Fatalf("failed = true, want false; line = %q", line)
		}
		if line != "You are running the latest version." {
			t.Errorf("line = %q, want latest-version message", line)
		}
	})

	t.Run("API failure", func(t *testing.T) {
		srv, _ := releaseServer(t, http.StatusInternalServerError, "")
		client := updatecheck.NewClientWithBaseURL(srv.URL)

		line, failed := checkForUpdate(context.Background(), client, "1.0.0")
		if !failed {
			t.Fatalf("failed = false, want true; line = %q", line)
		}
		if !strings.Contains(line, "update check failed") {
			t.Errorf("line = %q, want it to start with 'update check failed'", line)
		}
	})
}

// runVersionCmd executes `nexorious version <args...>` with version/commit
// overridden, returning stdout and stderr separately.
func runVersionCmd(t *testing.T, ver string, args ...string) (stdout, stderr string) {
	t.Helper()
	prevVersion, prevCommit := version, commit
	version, commit = ver, "deadbeef"
	t.Cleanup(func() {
		version = prevVersion
		commit = prevCommit
	})

	root := newRootCmd()
	var out, errOut strings.Builder
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(append([]string{"version"}, args...))
	if err := root.Execute(); err != nil {
		t.Fatalf("execute version: %v", err)
	}
	return out.String(), errOut.String()
}

// pointClientAt overrides the command's client factory at the test server.
func pointClientAt(t *testing.T, srv *httptest.Server) {
	t.Helper()
	prev := newUpdateCheckClient
	newUpdateCheckClient = func() *updatecheck.Client {
		return updatecheck.NewClientWithBaseURL(srv.URL)
	}
	t.Cleanup(func() { newUpdateCheckClient = prev })
}

func TestVersionCmd_UpdateCheck(t *testing.T) {
	const release = `{"tag_name":"v1.1.0","html_url":"https://github.com/drzero42/nexorious/releases/tag/v1.1.0"}`

	t.Run("reports available update", func(t *testing.T) {
		srv, _ := releaseServer(t, http.StatusOK, release)
		pointClientAt(t, srv)
		t.Setenv("UPDATE_CHECK_ENABLED", "true")

		stdout, stderr := runVersionCmd(t, "1.0.0")
		if !strings.Contains(stdout, "nexorious 1.0.0 (deadbeef)") {
			t.Errorf("stdout = %q, want version line first", stdout)
		}
		if !strings.Contains(stdout, "Update available: 1.1.0") {
			t.Errorf("stdout = %q, want update notice", stdout)
		}
		if stderr != "" {
			t.Errorf("stderr = %q, want empty", stderr)
		}
	})

	t.Run("failure goes to stderr, exit 0", func(t *testing.T) {
		srv, _ := releaseServer(t, http.StatusInternalServerError, "")
		pointClientAt(t, srv)
		t.Setenv("UPDATE_CHECK_ENABLED", "true")

		stdout, stderr := runVersionCmd(t, "1.0.0")
		if !strings.Contains(stdout, "nexorious 1.0.0") {
			t.Errorf("stdout = %q, want version line", stdout)
		}
		if strings.Contains(stdout, "update check failed") {
			t.Errorf("stdout = %q, failure note must not be on stdout", stdout)
		}
		if !strings.Contains(stderr, "update check failed") {
			t.Errorf("stderr = %q, want failure note", stderr)
		}
	})

	t.Run("--no-check skips the request", func(t *testing.T) {
		srv, requests := releaseServer(t, http.StatusOK, release)
		pointClientAt(t, srv)

		stdout, stderr := runVersionCmd(t, "1.0.0", "--no-check")
		if *requests != 0 {
			t.Errorf("requests = %d, want 0", *requests)
		}
		if want := "nexorious 1.0.0 (deadbeef)\n"; stdout != want {
			t.Errorf("stdout = %q, want only the version line %q", stdout, want)
		}
		if stderr != "" {
			t.Errorf("stderr = %q, want empty", stderr)
		}
	})

	t.Run("UPDATE_CHECK_ENABLED=false skips the request", func(t *testing.T) {
		srv, requests := releaseServer(t, http.StatusOK, release)
		pointClientAt(t, srv)
		t.Setenv("UPDATE_CHECK_ENABLED", "false")

		stdout, _ := runVersionCmd(t, "1.0.0")
		if *requests != 0 {
			t.Errorf("requests = %d, want 0", *requests)
		}
		if want := "nexorious 1.0.0 (deadbeef)\n"; stdout != want {
			t.Errorf("stdout = %q, want only the version line %q", stdout, want)
		}
	})

	t.Run("dev build skips the request", func(t *testing.T) {
		srv, requests := releaseServer(t, http.StatusOK, release)
		pointClientAt(t, srv)

		stdout, _ := runVersionCmd(t, "dev")
		if *requests != 0 {
			t.Errorf("requests = %d, want 0", *requests)
		}
		if want := "nexorious dev (deadbeef)\n"; stdout != want {
			t.Errorf("stdout = %q, want only the version line %q", stdout, want)
		}
	})
}
