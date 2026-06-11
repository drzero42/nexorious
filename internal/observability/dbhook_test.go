package observability

import (
	"context"
	"database/sql"
	"testing"

	"github.com/uptrace/bun"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// collectDBErrors returns the summed value of nexorious_db_errors across all
// attribute sets, or 0 if the instrument has not recorded anything.
func collectDBErrors(t *testing.T, reader sdkmetric.Reader) int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}
	var total int64
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "nexorious_db_errors" {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok {
				t.Fatalf("nexorious_db_errors is %T, want Sum[int64]", m.Data)
			}
			for _, dp := range sum.DataPoints {
				total += dp.Value
			}
		}
	}
	return total
}

func TestDBErrorHook(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	initInstruments(mp)

	hook := NewDBErrorHook()
	ctx := context.Background()

	// A real, non-ErrNoRows error must increment the counter.
	e := &bun.QueryEvent{Query: "SELECT 1", Err: sql.ErrConnDone}
	hook.AfterQuery(ctx, e)
	if got := collectDBErrors(t, reader); got != 1 {
		t.Fatalf("after real error: got %d, want 1", got)
	}

	// sql.ErrNoRows is "not found", not a failure — must NOT increment.
	noRows := &bun.QueryEvent{Query: "SELECT 1", Err: sql.ErrNoRows}
	hook.AfterQuery(ctx, noRows)
	if got := collectDBErrors(t, reader); got != 1 {
		t.Fatalf("after ErrNoRows: got %d, want 1 (unchanged)", got)
	}

	// A successful query (no error) must NOT increment.
	ok := &bun.QueryEvent{Query: "SELECT 1", Err: nil}
	hook.AfterQuery(ctx, ok)
	if got := collectDBErrors(t, reader); got != 1 {
		t.Fatalf("after success: got %d, want 1 (unchanged)", got)
	}
}
