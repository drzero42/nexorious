// Tests in this package are intentionally NOT run in parallel: Init writes
// package-level globals (metricsHandler, syncTotal, syncItemsTotal), so
// concurrent Init calls would race. Do not add t.Parallel() here.
package observability_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/drzero42/nexorious/internal/config"
	"github.com/drzero42/nexorious/internal/observability"
)

func scrape(t *testing.T, h http.Handler) string {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("scrape status = %d; want 200", rec.Code)
	}
	return rec.Body.String()
}

func TestInit_EnabledExposesBusinessMetrics(t *testing.T) {
	prov, err := observability.Init(&config.Config{
		OTELServiceName:    "nexorious-test",
		OTELMetricsEnabled: true,
	}, "1.2.3-test")
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	t.Cleanup(func() { _ = prov.Shutdown(context.Background()) })

	h := observability.MetricsHandler()
	if h == nil {
		t.Fatal("MetricsHandler() = nil; want non-nil when metrics enabled")
	}

	observability.RecordSyncOutcome(context.Background(), "steam", "completed")
	observability.RecordSyncItems(context.Background(), "steam", "completed", 3)
	observability.RecordSyncItems(context.Background(), "steam", "failed", 1)

	body := scrape(t, h)

	// Exporter appends _total to the monotonic counters.
	if !strings.Contains(body, "nexorious_sync_total") {
		t.Errorf("scrape missing nexorious_sync_total:\n%s", body)
	}
	if !strings.Contains(body, `source="steam"`) || !strings.Contains(body, `status="completed"`) {
		t.Errorf("scrape missing expected sync_total labels:\n%s", body)
	}
	if !strings.Contains(body, "nexorious_sync_items_total") {
		t.Errorf("scrape missing nexorious_sync_items_total:\n%s", body)
	}
	if !strings.Contains(body, `outcome="failed"`) {
		t.Errorf("scrape missing items outcome label:\n%s", body)
	}
	// Cardinality guard: never label by user_id.
	if strings.Contains(body, "user_id=") {
		t.Errorf("scrape leaked user_id label:\n%s", body)
	}
}

func TestInit_DisabledIsNoop(t *testing.T) {
	prov, err := observability.Init(&config.Config{
		OTELServiceName:    "nexorious-test",
		OTELMetricsEnabled: false,
	}, "1.2.3-test")
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	t.Cleanup(func() { _ = prov.Shutdown(context.Background()) })

	if observability.MetricsHandler() != nil {
		t.Error("MetricsHandler() != nil; want nil when metrics disabled")
	}
	// Recording must not panic. With the no-op provider, syncTotal/syncItemsTotal
	// hold noop counters (not nil), so Add() dispatches to the no-op impl; the nil
	// guard in RecordSync* only fires if Init were never called.
	observability.RecordSyncOutcome(context.Background(), "steam", "completed")
	observability.RecordSyncItems(context.Background(), "steam", "completed", 1)
}
