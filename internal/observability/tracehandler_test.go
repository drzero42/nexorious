package observability_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"go.opentelemetry.io/otel/trace"

	"github.com/drzero42/nexorious/internal/observability"
)

func logLine(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("parse log line %q: %v", buf.String(), err)
	}
	return m
}

func TestTraceContextHandler_ActiveSpanAddsIDs(t *testing.T) {
	tid, _ := trace.TraceIDFromHex("0102030405060708090a0b0c0d0e0f10")
	sid, _ := trace.SpanIDFromHex("0102030405060708")
	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    tid,
		SpanID:     sid,
		TraceFlags: trace.FlagsSampled,
	}))

	var buf bytes.Buffer
	logger := slog.New(observability.NewTraceContextHandler(slog.NewJSONHandler(&buf, nil)))
	logger.InfoContext(ctx, "hello")

	m := logLine(t, &buf)
	if m["trace_id"] != "0102030405060708090a0b0c0d0e0f10" {
		t.Errorf("trace_id = %v; want 0102030405060708090a0b0c0d0e0f10", m["trace_id"])
	}
	if m["span_id"] != "0102030405060708" {
		t.Errorf("span_id = %v; want 0102030405060708", m["span_id"])
	}
}

func TestTraceContextHandler_NoSpanAddsNothing(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(observability.NewTraceContextHandler(slog.NewJSONHandler(&buf, nil)))
	logger.InfoContext(context.Background(), "hello")

	m := logLine(t, &buf)
	if _, ok := m["trace_id"]; ok {
		t.Error("trace_id present without an active span")
	}
	if _, ok := m["span_id"]; ok {
		t.Error("span_id present without an active span")
	}
}

func TestTraceContextHandler_PreservesWithAttrs(t *testing.T) {
	tid, _ := trace.TraceIDFromHex("0102030405060708090a0b0c0d0e0f10")
	sid, _ := trace.SpanIDFromHex("0102030405060708")
	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    tid,
		SpanID:     sid,
		TraceFlags: trace.FlagsSampled,
	}))

	var buf bytes.Buffer
	logger := slog.New(observability.NewTraceContextHandler(slog.NewJSONHandler(&buf, nil))).With("component", "test")
	logger.InfoContext(ctx, "hello")

	m := logLine(t, &buf)
	if m["component"] != "test" {
		t.Errorf("component = %v; want test (WithAttrs lost)", m["component"])
	}
	if m["trace_id"] != "0102030405060708090a0b0c0d0e0f10" {
		t.Errorf("trace_id = %v; want injected on a With()-derived logger", m["trace_id"])
	}
}
