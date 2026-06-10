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

	var sawJobID string
	err := mw.Work(context.Background(), job, func(ctx context.Context) error {
		sawJobID = jobID(ctx)
		return nil
	})
	if err != nil {
		t.Fatalf("Work returned error: %v", err)
	}
	if sawJobID != "42" {
		t.Errorf("job_id in ctx = %q, want 42", sawJobID)
	}
	m := decode(t, &buf)
	if m[KeyJobType] != "sync_steam" {
		t.Errorf("job_type = %v, want sync_steam", m[KeyJobType])
	}
	if m[KeyOutcome] != "completed" {
		t.Errorf("outcome = %v, want completed", m[KeyOutcome])
	}
	if m[KeyJobID] != "42" {
		t.Errorf("job_id = %v, want 42", m[KeyJobID])
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
}
