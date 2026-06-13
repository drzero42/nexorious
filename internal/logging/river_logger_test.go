package logging

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
)

// TestRiverLogger_RemapsJobIDKey verifies that River's internal job_id attribute
// (an int64 == jobs' River id) is emitted under river_job_id, matching the
// logging-conventions contract where job_id is reserved for the jobs.id UUID.
func TestRiverLogger_RemapsJobIDKey(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(NewContextHandler(
		slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))))
	defer slog.SetDefault(prev)

	rl := RiverLogger()
	rl.LogAttrs(context.Background(), slog.LevelWarn, "river: panic recovery",
		slog.Int64("job_id", 4242), slog.String("kind", "sync_steam"))

	m := decode(t, &buf)
	if got, ok := m[KeyRiverJobID]; !ok || got != float64(4242) {
		t.Errorf("river_job_id = %v (ok=%v), want 4242", got, ok)
	}
	if _, ok := m[KeyJobID]; ok {
		t.Errorf("job_id key should be remapped away; got %v", m[KeyJobID])
	}
	if m["kind"] != "sync_steam" {
		t.Errorf("kind = %v, want sync_steam (unrelated attrs untouched)", m["kind"])
	}
}

// TestRiverLogger_RemapsJobIDInWithAttrs verifies the rename also applies to
// attributes bound via WithAttrs (logger.With), not only per-record attrs.
func TestRiverLogger_RemapsJobIDInWithAttrs(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(NewContextHandler(
		slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))))
	defer slog.SetDefault(prev)

	rl := RiverLogger().With(slog.Int64("job_id", 99))
	rl.WarnContext(context.Background(), "river: something")

	m := decode(t, &buf)
	if got, ok := m[KeyRiverJobID]; !ok || got != float64(99) {
		t.Errorf("river_job_id = %v (ok=%v), want 99", got, ok)
	}
	if _, ok := m[KeyJobID]; ok {
		t.Errorf("job_id key should be remapped away; got %v", m[KeyJobID])
	}
}
