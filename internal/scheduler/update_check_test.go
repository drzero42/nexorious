package scheduler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/drzero42/nexorious/internal/scheduler"
	"github.com/drzero42/nexorious/internal/services/updatecheck"
)

// newGitHubStub returns an httptest server mimicking the GitHub
// releases/latest endpoint, plus a request counter.
func newGitHubStub(t *testing.T, tagName, htmlURL string) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"` + tagName + `","html_url":"` + htmlURL + `"}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func countVersionEvents(t *testing.T) int {
	t.Helper()
	var n int
	err := testDB.NewRaw(
		`SELECT count(*) FROM events WHERE type = 'admin.version.available'`,
	).Scan(context.Background(), &n)
	if err != nil {
		t.Fatalf("count events: %v", err)
	}
	return n
}

func TestCheckForUpdates_EmitsOncePerRelease(t *testing.T) {
	truncateAllTables(t)
	srv, _ := newGitHubStub(t, "v9.9.9", "https://github.com/drzero42/nexorious/releases/tag/v9.9.9")

	st := updatecheck.NewState()
	w := &scheduler.CheckForUpdatesWorker{
		DB:             testDB,
		State:          st,
		Client:         updatecheck.NewClientWithBaseURL(srv.URL),
		RunningVersion: "0.1.0",
		Enabled:        true,
	}

	// Two runs — the dedup key must collapse them into one event.
	if err := w.Work(context.Background(), nil); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if err := w.Work(context.Background(), nil); err != nil {
		t.Fatalf("Work: %v", err)
	}

	if got := countVersionEvents(t); got != 1 {
		t.Errorf("events = %d, want exactly 1 (deduped)", got)
	}
	latest, url := st.Latest()
	if latest != "9.9.9" {
		t.Errorf("state latest = %q, want 9.9.9 (v stripped)", latest)
	}
	if url != "https://github.com/drzero42/nexorious/releases/tag/v9.9.9" {
		t.Errorf("state url = %q", url)
	}

	var payload string
	if err := testDB.NewRaw(
		`SELECT payload::text FROM events WHERE type = 'admin.version.available'`,
	).Scan(context.Background(), &payload); err != nil {
		t.Fatalf("read payload: %v", err)
	}
	for _, want := range []string{`"current_version": "0.1.0"`, `"available_version": "9.9.9"`, `"release_url": "https://github.com/drzero42/nexorious/releases/tag/v9.9.9"`} {
		if !strings.Contains(payload, want) {
			t.Errorf("payload %s missing %s", payload, want)
		}
	}
}

func TestCheckForUpdates_UpToDateEmitsNothing(t *testing.T) {
	truncateAllTables(t)
	srv, _ := newGitHubStub(t, "v0.1.0", "https://github.com/drzero42/nexorious/releases/tag/v0.1.0")

	st := updatecheck.NewState()
	w := &scheduler.CheckForUpdatesWorker{
		DB:             testDB,
		State:          st,
		Client:         updatecheck.NewClientWithBaseURL(srv.URL),
		RunningVersion: "0.1.0",
		Enabled:        true,
	}
	if err := w.Work(context.Background(), nil); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if got := countVersionEvents(t); got != 0 {
		t.Errorf("events = %d, want 0", got)
	}
	if latest, _ := st.Latest(); latest != "0.1.0" {
		t.Errorf("state latest = %q, want 0.1.0 (state still updated)", latest)
	}
}

func TestCheckForUpdates_DevVersionEmitsNothing(t *testing.T) {
	truncateAllTables(t)
	srv, _ := newGitHubStub(t, "v9.9.9", "https://github.com/drzero42/nexorious/releases/tag/v9.9.9")

	st := updatecheck.NewState()
	w := &scheduler.CheckForUpdatesWorker{
		DB:             testDB,
		State:          st,
		Client:         updatecheck.NewClientWithBaseURL(srv.URL),
		RunningVersion: "dev",
		Enabled:        true,
	}
	if err := w.Work(context.Background(), nil); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if got := countVersionEvents(t); got != 0 {
		t.Errorf("events = %d, want 0 for dev build", got)
	}
}

func TestCheckForUpdates_DisabledShortCircuits(t *testing.T) {
	truncateAllTables(t)
	srv, calls := newGitHubStub(t, "v9.9.9", "https://github.com/drzero42/nexorious/releases/tag/v9.9.9")

	st := updatecheck.NewState()
	w := &scheduler.CheckForUpdatesWorker{
		DB:             testDB,
		State:          st,
		Client:         updatecheck.NewClientWithBaseURL(srv.URL),
		RunningVersion: "0.1.0",
		Enabled:        false,
	}
	if err := w.Work(context.Background(), nil); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if calls.Load() != 0 {
		t.Errorf("GitHub fetches = %d, want 0 when disabled", calls.Load())
	}
	if got := countVersionEvents(t); got != 0 {
		t.Errorf("events = %d, want 0 when disabled", got)
	}
	if latest, _ := st.Latest(); latest != "" {
		t.Errorf("state latest = %q, want empty when disabled", latest)
	}
}

func TestCheckForUpdates_FetchFailureKeepsLastGoodState(t *testing.T) {
	truncateAllTables(t)
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(failing.Close)

	st := updatecheck.NewState()
	st.Set("0.2.0", "https://github.com/drzero42/nexorious/releases/tag/v0.2.0")
	w := &scheduler.CheckForUpdatesWorker{
		DB:             testDB,
		State:          st,
		Client:         updatecheck.NewClientWithBaseURL(failing.URL),
		RunningVersion: "0.1.0",
		Enabled:        true,
	}
	if err := w.Work(context.Background(), nil); err != nil {
		t.Fatalf("Work must not error on fetch failure, got: %v", err)
	}
	if latest, _ := st.Latest(); latest != "0.2.0" {
		t.Errorf("state latest = %q, want last good value 0.2.0", latest)
	}
}

func TestCheckForUpdates_EachNewReleaseEmitsOnce(t *testing.T) {
	truncateAllTables(t)
	var tag atomic.Value
	tag.Store("v0.10.0")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"` + tag.Load().(string) + `","html_url":"https://github.com/drzero42/nexorious/releases"}`))
	}))
	t.Cleanup(srv.Close)

	st := updatecheck.NewState()
	w := &scheduler.CheckForUpdatesWorker{
		DB:             testDB,
		State:          st,
		Client:         updatecheck.NewClientWithBaseURL(srv.URL),
		RunningVersion: "0.9.0",
		Enabled:        true,
	}
	if err := w.Work(context.Background(), nil); err != nil {
		t.Fatalf("Work: %v", err)
	}
	tag.Store("v0.11.0")
	if err := w.Work(context.Background(), nil); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if got := countVersionEvents(t); got != 2 {
		t.Errorf("events = %d, want 2 (one per release)", got)
	}
}
