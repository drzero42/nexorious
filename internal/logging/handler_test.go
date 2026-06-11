package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
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
