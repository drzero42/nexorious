package logging

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/riverqueue/river/rivertype"
)

func TestWorkerMiddleware_Success(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(NewContextHandler(slog.NewJSONHandler(&buf, nil))))
	defer slog.SetDefault(prev)

	mw := NewWorkerMiddleware()
	job := &rivertype.JobRow{ID: 42, Kind: "sync_steam"}

	var sawRiverJobID string
	err := mw.Work(context.Background(), job, func(ctx context.Context) error {
		sawRiverJobID = riverJobID(ctx)
		return nil
	})
	if err != nil {
		t.Fatalf("Work returned error: %v", err)
	}
	if sawRiverJobID != "42" {
		t.Errorf("river_job_id in ctx = %q, want 42", sawRiverJobID)
	}
	m := decode(t, &buf)
	if m[KeyJobType] != "sync_steam" {
		t.Errorf("job_type = %v, want sync_steam", m[KeyJobType])
	}
	if m[KeyOutcome] != "completed" {
		t.Errorf("outcome = %v, want completed", m[KeyOutcome])
	}
	if m[KeyRiverJobID] != "42" {
		t.Errorf("river_job_id = %v, want 42", m[KeyRiverJobID])
	}
	if _, ok := m[KeyDurationMS]; !ok {
		t.Errorf("missing duration_ms")
	}
}

func TestWorkerMiddleware_Failure(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(NewContextHandler(slog.NewJSONHandler(&buf, nil))))
	defer slog.SetDefault(prev)

	mw := NewWorkerMiddleware()
	job := &rivertype.JobRow{ID: 7, Kind: "import_item"}
	want := errors.New("boom")

	err := mw.Work(context.Background(), job, func(ctx context.Context) error {
		time.Sleep(time.Millisecond)
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("Work error = %v, want it to wrap boom", err)
	}
	m := decode(t, &buf)
	if m[KeyOutcome] != "failed" {
		t.Errorf("outcome = %v, want failed", m[KeyOutcome])
	}
	if m["level"] != "WARN" {
		t.Errorf("level = %v, want WARN on failure", m["level"])
	}
}

func TestWorkerMiddleware_QuietKindLogsDebug(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(NewContextHandler(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))))
	defer slog.SetDefault(prev)

	mw := NewWorkerMiddleware("cleanup_old_jobs")

	// A quiet kind that succeeds logs its outcome at Debug.
	err := mw.Work(context.Background(), &rivertype.JobRow{ID: 1, Kind: "cleanup_old_jobs"},
		func(ctx context.Context) error { return nil })
	if err != nil {
		t.Fatalf("Work returned error: %v", err)
	}
	if m := decode(t, &buf); m["level"] != "DEBUG" || m[KeyOutcome] != "completed" {
		t.Errorf("quiet success: level=%v outcome=%v, want DEBUG/completed", m["level"], m[KeyOutcome])
	}

	// A non-quiet kind still logs success at Info.
	buf.Reset()
	if err := mw.Work(context.Background(), &rivertype.JobRow{ID: 2, Kind: "dispatch_sync"},
		func(ctx context.Context) error { return nil }); err != nil {
		t.Fatalf("Work returned error: %v", err)
	}
	if m := decode(t, &buf); m["level"] != "INFO" {
		t.Errorf("non-quiet success: level=%v, want INFO", m["level"])
	}
}
