package cliclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPErrorTyped(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		wantMsg string
		wantErr string
	}{
		{"message shape", http.StatusForbidden, `{"message":"insufficient scope"}`, "insufficient scope", "server returned 403: insufficient scope"},
		{"error shape", http.StatusInternalServerError, `{"error":"backup failed"}`, "backup failed", "server returned 500: backup failed"},
		{"app state gate", http.StatusServiceUnavailable, `{"app_state":"needs_migration"}`, "instance not ready (state: needs_migration)", "server returned 503: instance not ready (state: needs_migration)"},
		{"no message", http.StatusBadGateway, `not json`, "", "server returned 502"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer srv.Close()

			_, err := New(srv.URL).Me(t.Context(), "k")
			var he *HTTPError
			if !errors.As(err, &he) {
				t.Fatalf("err = %v (%T), want *HTTPError", err, err)
			}
			if he.Status != tc.status || he.Message != tc.wantMsg {
				t.Errorf("got {%d %q}, want {%d %q}", he.Status, he.Message, tc.status, tc.wantMsg)
			}
			if err.Error() != tc.wantErr {
				t.Errorf("Error() = %q, want %q", err.Error(), tc.wantErr)
			}
		})
	}
}

type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }

func TestHTTPErrorBodyReadFailureStaysWrapped(t *testing.T) {
	readErr := errors.New("connection reset")
	err := httpError(&http.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(failingReader{readErr})})

	if !errors.Is(err, readErr) {
		t.Errorf("errors.Is(err, readErr) = false; err = %v", err)
	}
	if want := "server returned 502 (failed reading body: connection reset)"; err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

func TestContextCancelAbortsInFlightRequest(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		_, err := New(srv.URL).Me(ctx, "k")
		done <- err
	}()
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("request did not abort after ctx cancel")
	}
}

type stubTransport struct {
	reqs []*http.Request
}

func (s *stubTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	s.reqs = append(s.reqs, r)
	return &http.Response{
		StatusCode: http.StatusFound,
		Header:     http.Header{"Location": []string{"/migrate"}},
		Body:       io.NopCloser(strings.NewReader("")),
		Request:    r,
	}, nil
}

func TestNewWithTransportUsesStubAndDoesNotFollowRedirects(t *testing.T) {
	rt := &stubTransport{}
	res, err := NewWithTransport("http://nexorious.internal/", rt).SetupAdmin(t.Context(), "admin", "pw")
	if err != nil {
		t.Fatalf("SetupAdmin: %v", err)
	}
	if len(rt.reqs) != 1 {
		t.Fatalf("transport saw %d requests, want 1 (redirect must not be followed)", len(rt.reqs))
	}
	if got := rt.reqs[0].URL.String(); got != "http://nexorious.internal/api/auth/setup/admin" {
		t.Errorf("request URL = %q", got)
	}
	if res.StatusCode != http.StatusFound || res.Location != "/migrate" {
		t.Errorf("result = %+v, want 302 with Location /migrate", res)
	}
}
