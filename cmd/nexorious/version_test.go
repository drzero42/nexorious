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
