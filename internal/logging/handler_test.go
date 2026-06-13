package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"testing"
)

// newTestLogger returns a logger writing JSON into buf, wrapped by ContextHandler.
func newTestLogger(buf *bytes.Buffer) *slog.Logger {
	inner := slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	return slog.New(NewContextHandler(inner))
}

func decode(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("unmarshal log line: %v (raw=%q)", err, buf.String())
	}
	return m
}

func TestContextHandler_InjectsCorrelation(t *testing.T) {
	var buf bytes.Buffer
	log := newTestLogger(&buf)

	ctx := WithRequestID(context.Background(), "req-123")
	ctx = WithJobID(ctx, "job-456")
	ctx = WithUserID(ctx, "user-789")

	log.InfoContext(ctx, "hello")

	m := decode(t, &buf)
	if m[KeyRequestID] != "req-123" {
		t.Errorf("request_id = %v, want req-123", m[KeyRequestID])
	}
	if m[KeyJobID] != "job-456" {
		t.Errorf("job_id = %v, want job-456", m[KeyJobID])
	}
	if m[KeyUserID] != "user-789" {
		t.Errorf("user_id = %v, want user-789", m[KeyUserID])
	}
}

func TestContextHandler_OmitsAbsentValues(t *testing.T) {
	var buf bytes.Buffer
	log := newTestLogger(&buf)

	log.InfoContext(context.Background(), "hello")

	m := decode(t, &buf)
	for _, k := range []string{KeyRequestID, KeyJobID, KeyUserID} {
		if _, present := m[k]; present {
			t.Errorf("key %q should be absent when ctx has no value", k)
		}
	}
}

func TestContextHandler_PreservesWithAttrs(t *testing.T) {
	var buf bytes.Buffer
	log := newTestLogger(&buf).With(KeyJobType, "sync_steam")

	log.InfoContext(WithJobID(context.Background(), "j1"), "hello")

	m := decode(t, &buf)
	if m[KeyJobType] != "sync_steam" {
		t.Errorf("job_type = %v, want sync_steam", m[KeyJobType])
	}
	if m[KeyJobID] != "j1" {
		t.Errorf("job_id = %v, want j1", m[KeyJobID])
	}
}

// credURLError builds a wrapped *url.Error the way a failed storefront call
// produces one: the message embeds the full request URL including its
// credential-bearing query string.
func credURLError() error {
	return fmt.Errorf("sync failed: get owned games: %w", &url.Error{
		Op:  "Get",
		URL: "https://api.steampowered.com/IPlayerService/GetOwnedGames/v0001/?key=SECRET&steamid=123",
		Err: errors.New("connection refused"),
	})
}

// assertScrubbed fails if got still carries the secret or lost the URL path.
func assertScrubbed(t *testing.T, field string, got any) {
	t.Helper()
	s, ok := got.(string)
	if !ok {
		t.Fatalf("%s = %T(%v), want string", field, got, got)
	}
	if strings.Contains(s, "key=SECRET") {
		t.Errorf("%s still contains credential query: %q", field, s)
	}
	if !strings.Contains(s, "api.steampowered.com/IPlayerService/GetOwnedGames/v0001/") {
		t.Errorf("%s lost the URL host/path: %q", field, s)
	}
}

func TestContextHandler_ScrubsErrorAttr(t *testing.T) {
	var buf bytes.Buffer
	log := newTestLogger(&buf)

	log.ErrorContext(context.Background(), "library fetch failed", KeyErr, credURLError())

	assertScrubbed(t, KeyErr, decode(t, &buf)[KeyErr])
}

func TestContextHandler_ScrubsStringAttr(t *testing.T) {
	var buf bytes.Buffer
	log := newTestLogger(&buf)

	log.InfoContext(context.Background(), "hello",
		"endpoint", "https://auth.gog.com/token?client_secret=SECRETC&refresh_token=SECRETR")

	m := decode(t, &buf)
	s, _ := m["endpoint"].(string)
	if s != "https://auth.gog.com/token" {
		t.Errorf("endpoint = %q, want query stripped", s)
	}
}

func TestContextHandler_ScrubsMessage(t *testing.T) {
	var buf bytes.Buffer
	log := newTestLogger(&buf)

	log.ErrorContext(context.Background(), "fetch https://a.example/p?key=SECRET failed")

	m := decode(t, &buf)
	s, _ := m["msg"].(string)
	if s != "fetch https://a.example/p failed" {
		t.Errorf("msg = %q, want query stripped", s)
	}
}

func TestContextHandler_ScrubsPreboundAttrs(t *testing.T) {
	var buf bytes.Buffer
	log := newTestLogger(&buf).With(KeyErr, credURLError())

	log.ErrorContext(context.Background(), "hello")

	assertScrubbed(t, KeyErr, decode(t, &buf)[KeyErr])
}

func TestContextHandler_ScrubsGroupAttr(t *testing.T) {
	var buf bytes.Buffer
	log := newTestLogger(&buf)

	log.InfoContext(context.Background(), "hello",
		slog.Group("req", slog.String("u", "https://a.example/p?k=SECRET")))

	m := decode(t, &buf)
	g, ok := m["req"].(map[string]any)
	if !ok {
		t.Fatalf("req group missing: %v", m)
	}
	if g["u"] != "https://a.example/p" {
		t.Errorf("req.u = %v, want query stripped", g["u"])
	}
}

func TestContextHandler_WithGroupKeepsCorrelationFlat(t *testing.T) {
	var buf bytes.Buffer
	log := newTestLogger(&buf).WithGroup("sync")

	ctx := WithRequestID(context.Background(), "req-1")
	ctx = WithJobID(ctx, "job-2")
	log.InfoContext(ctx, "hello", "attempt", 3)

	m := decode(t, &buf)

	// Correlation keys are a flat-key contract: they must stay at the root, never
	// qualified as "sync.request_id".
	if m[KeyRequestID] != "req-1" {
		t.Errorf("request_id must be flat at root, got %v (full=%v)", m[KeyRequestID], m)
	}
	if m[KeyJobID] != "job-2" {
		t.Errorf("job_id must be flat at root, got %v (full=%v)", m[KeyJobID], m)
	}

	// The record's own attributes still belong under the open group.
	g, ok := m["sync"].(map[string]any)
	if !ok {
		t.Fatalf("expected record attrs grouped under 'sync', got %v", m)
	}
	if g["attempt"] != float64(3) {
		t.Errorf("sync.attempt = %v, want 3", g["attempt"])
	}
	if _, dup := g[KeyRequestID]; dup {
		t.Errorf("correlation key leaked into the group: %v", g)
	}
}

func TestContextHandler_LeavesCleanRecordsUntouched(t *testing.T) {
	var buf bytes.Buffer
	log := newTestLogger(&buf)

	log.ErrorContext(context.Background(), "job finished", KeyErr, errors.New("connection refused"))

	m := decode(t, &buf)
	if m["msg"] != "job finished" {
		t.Errorf("msg = %v, want unchanged", m["msg"])
	}
	if m[KeyErr] != "connection refused" {
		t.Errorf("err = %v, want unchanged error text", m[KeyErr])
	}
}
