package logging

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/riverqueue/river/rivertype"
)

func TestWorkerErrorHandler_HandlePanic_EmitsPanicLine(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(NewContextHandler(
		slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))))
	defer slog.SetDefault(prev)

	h := &WorkerErrorHandler{}
	job := &rivertype.JobRow{ID: 4242, Kind: "sync_steam"}

	res := h.HandlePanic(context.Background(), job, "nil map write", "goroutine 1 [running]:")

	if res != nil {
		t.Errorf("HandlePanic result = %v, want nil (default retry behavior)", res)
	}
	m := decode(t, &buf)
	if m[KeyCategory] != "panic" {
		t.Errorf("category = %v, want panic", m[KeyCategory])
	}
	if m[KeyJobType] != "sync_steam" {
		t.Errorf("job_type = %v, want sync_steam", m[KeyJobType])
	}
	if m[KeyRiverJobID] != "4242" {
		t.Errorf("river_job_id = %v, want 4242", m[KeyRiverJobID])
	}
	if m[KeyErr] != "nil map write" {
		t.Errorf("err = %v, want \"nil map write\"", m[KeyErr])
	}
	if m[KeyStack] != "goroutine 1 [running]:" {
		t.Errorf("stack = %v, want the recovered trace", m[KeyStack])
	}
	if m["level"] != "ERROR" {
		t.Errorf("level = %v, want ERROR", m["level"])
	}
}

func TestWorkerErrorHandler_HandleError_IsNoOp(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(NewContextHandler(
		slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))))
	defer slog.SetDefault(prev)

	h := &WorkerErrorHandler{}
	job := &rivertype.JobRow{ID: 1, Kind: "sync_steam"}

	res := h.HandleError(context.Background(), job, context.Canceled)

	if res != nil {
		t.Errorf("HandleError result = %v, want nil", res)
	}
	if buf.Len() != 0 {
		t.Errorf("HandleError should not log (WorkerMiddleware already logs failures); got %q", buf.String())
	}
}
