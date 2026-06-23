package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/drzero42/nexorious/internal/api"
	"github.com/drzero42/nexorious/internal/changelog"
	"github.com/drzero42/nexorious/internal/db/models"
)

// newChangelogCtx builds an echo.Context with user_id set, reading query params
// from the target URL. It bypasses AuthMiddleware so the handler can be called
// directly.
func newChangelogCtx(method, target, userID string) (*echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(method, target, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user_id", userID)
	return c, rec
}

// testEntries is a small synthetic changelog injected into tests that need a
// non-empty source. Versions are ordered newest-first (as a real CHANGELOG.md
// would be), so 0.90.0 > 0.17.1 > 0.10.0.
var testEntries = []changelog.Entry{
	{Version: "0.90.0", Date: "2026-06-20", Groups: []changelog.Group{{Title: "Features", Items: []string{"new thing"}}}},
	{Version: "0.17.1", Date: "2026-01-01", Groups: []changelog.Group{{Title: "Bug Fixes", Items: []string{"fixed a bug"}}}},
	{Version: "0.10.0", Date: "2025-06-01", Groups: []changelog.Group{{Title: "Features", Items: []string{"initial feature"}}}},
}

func availableSource() ([]changelog.Entry, bool) {
	return testEntries, true
}

func unavailableSource() ([]changelog.Entry, bool) {
	return nil, false
}

// insertUserSettings seeds a user_settings row with the given last_seen version.
func insertUserSettings(t *testing.T, userID, lastSeen string) {
	t.Helper()
	_, err := testDB.ExecContext(context.Background(),
		`INSERT INTO user_settings (user_id, deal_region, last_seen_changelog_version, created_at, updated_at)
		 VALUES (?, 'us', ?, now(), now())`,
		userID, lastSeen,
	)
	if err != nil {
		t.Fatalf("insertUserSettings: %v", err)
	}
}

// queryLastSeen reads last_seen_changelog_version from the DB for userID.
// Returns nil if no row exists or the column is NULL.
func queryLastSeen(t *testing.T, userID string) *string {
	t.Helper()
	var s models.UserSettings
	_ = testDB.NewSelect().Model(&s).Where("user_id = ?", userID).Scan(context.Background())
	return s.LastSeenChangelogVersion
}

// TestChangelog_UnavailableEmbed verifies that when the changelog embed is
// unavailable (placeholder), HandleGet returns available:false and does NOT
// write last_seen.
func TestChangelog_UnavailableEmbed(t *testing.T) {
	truncateAllTables(t)
	const userID = "cl-u1"
	insertAuthTestUser(t, testDB, userID, "cl-user1", "pass", true, false)

	h := api.NewChangelogHandler(testDB, "0.90.0")
	h.Source = unavailableSource

	c, rec := newChangelogCtx(http.MethodGet, "/api/changelog", userID)
	if err := h.HandleGet(c); err != nil {
		t.Fatalf("HandleGet: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["available"] != false {
		t.Errorf("available: want false, got %v", body["available"])
	}
	if body["current"] != "0.90.0" {
		t.Errorf("current: want 0.90.0, got %v", body["current"])
	}

	// last_seen must NOT have been written.
	if got := queryLastSeen(t, userID); got != nil {
		t.Errorf("last_seen should remain nil, got %q", *got)
	}
}

// TestChangelogUnseen_BaselineCapturedWhenNull verifies that when last_seen is
// null (first call), HandleUnseen captures the baseline (last_seen = current)
// and returns has_unseen:false.
func TestChangelogUnseen_BaselineCapturedWhenNull(t *testing.T) {
	truncateAllTables(t)
	const userID = "cl-u2"
	insertAuthTestUser(t, testDB, userID, "cl-user2", "pass", true, false)

	h := api.NewChangelogHandler(testDB, "0.90.0")
	h.Source = availableSource

	c, rec := newChangelogCtx(http.MethodGet, "/api/changelog/unseen", userID)
	if err := h.HandleUnseen(c); err != nil {
		t.Fatalf("HandleUnseen: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["has_unseen"] != false {
		t.Errorf("has_unseen: want false, got %v", body["has_unseen"])
	}

	// Baseline must have been captured.
	got := queryLastSeen(t, userID)
	if got == nil {
		t.Fatal("last_seen should be set, got nil")
	} else if *got != "0.90.0" {
		t.Errorf("last_seen: want 0.90.0, got %q", *got)
	}
}

// TestChangelogUnseen_DevVersionNoCapture verifies that a "dev" build does not
// capture a baseline and always returns has_unseen:false.
func TestChangelogUnseen_DevVersionNoCapture(t *testing.T) {
	truncateAllTables(t)
	const userID = "cl-u3"
	insertAuthTestUser(t, testDB, userID, "cl-user3", "pass", true, false)

	h := api.NewChangelogHandler(testDB, "dev")
	h.Source = availableSource

	c, rec := newChangelogCtx(http.MethodGet, "/api/changelog/unseen", userID)
	if err := h.HandleUnseen(c); err != nil {
		t.Fatalf("HandleUnseen: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["has_unseen"] != false {
		t.Errorf("has_unseen: want false, got %v", body["has_unseen"])
	}

	// No baseline should have been written.
	if got := queryLastSeen(t, userID); got != nil {
		t.Errorf("last_seen should remain nil for dev build, got %q", *got)
	}
}

// TestChangelogUnseen_HasUnseenWhenCurrentNewer verifies that when last_seen is
// older than the current version, has_unseen is true and last_seen is unchanged.
func TestChangelogUnseen_HasUnseenWhenCurrentNewer(t *testing.T) {
	truncateAllTables(t)
	const userID = "cl-u4"
	insertAuthTestUser(t, testDB, userID, "cl-user4", "pass", true, false)
	insertUserSettings(t, userID, "0.17.1")

	h := api.NewChangelogHandler(testDB, "0.90.0")
	h.Source = availableSource

	c, rec := newChangelogCtx(http.MethodGet, "/api/changelog/unseen", userID)
	if err := h.HandleUnseen(c); err != nil {
		t.Fatalf("HandleUnseen: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["has_unseen"] != true {
		t.Errorf("has_unseen: want true, got %v", body["has_unseen"])
	}

	// last_seen must NOT advance — unseen never moves the marker.
	got := queryLastSeen(t, userID)
	if got == nil {
		t.Fatal("last_seen should still exist, got nil")
	} else if *got != "0.17.1" {
		t.Errorf("last_seen: want unchanged 0.17.1, got %q", *got)
	}
}

// TestChangelog_SinceParamIsPureRead verifies that GET /api/changelog?since=X
// is a pure read — it does not advance last_seen.
func TestChangelog_SinceParamIsPureRead(t *testing.T) {
	truncateAllTables(t)
	const userID = "cl-u5"
	insertAuthTestUser(t, testDB, userID, "cl-user5", "pass", true, false)
	insertUserSettings(t, userID, "0.17.1")

	h := api.NewChangelogHandler(testDB, "0.90.0")
	h.Source = availableSource

	c, rec := newChangelogCtx(http.MethodGet, "/api/changelog?since=0.10.0", userID)
	if err := h.HandleGet(c); err != nil {
		t.Fatalf("HandleGet: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["available"] != true {
		t.Errorf("available: want true, got %v", body["available"])
	}

	// last_seen must remain "0.17.1" — since= is a pure read.
	got := queryLastSeen(t, userID)
	if got == nil {
		t.Fatal("last_seen should still exist, got nil")
	} else if *got != "0.17.1" {
		t.Errorf("last_seen: want 0.17.1 unchanged, got %q", *got)
	}
}

// TestChangelog_DefaultMarksSeen verifies that a default GET /api/changelog
// (no params) advances last_seen to the current version.
func TestChangelog_DefaultMarksSeen(t *testing.T) {
	truncateAllTables(t)
	const userID = "cl-u6"
	insertAuthTestUser(t, testDB, userID, "cl-user6", "pass", true, false)

	h := api.NewChangelogHandler(testDB, "0.90.0")
	h.Source = availableSource

	c, rec := newChangelogCtx(http.MethodGet, "/api/changelog", userID)
	if err := h.HandleGet(c); err != nil {
		t.Fatalf("HandleGet: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["available"] != true {
		t.Errorf("available: want true, got %v", body["available"])
	}

	// last_seen should have been advanced to the current version.
	got := queryLastSeen(t, userID)
	if got == nil {
		t.Fatal("last_seen should be set after default GET, got nil")
	} else if *got != "0.90.0" {
		t.Errorf("last_seen: want 0.90.0, got %q", *got)
	}
}

// TestChangelog_EntriesNeverNull verifies that the entries field in the JSON
// response is always a JSON array, never null. This is critical for web clients
// that map over .entries.length.
func TestChangelog_EntriesNeverNull(t *testing.T) {
	truncateAllTables(t)
	const userID = "cl-u7"
	insertAuthTestUser(t, testDB, userID, "cl-user7", "pass", true, false)

	h := api.NewChangelogHandler(testDB, "0.90.0")
	h.Source = availableSource

	// Request with last_seen=NULL (default since-last mode).
	// Newer(...) returns nil when lastSeen is empty, which used to serialize
	// as null. Verify it's an array instead.
	c, rec := newChangelogCtx(http.MethodGet, "/api/changelog", userID)
	if err := h.HandleGet(c); err != nil {
		t.Fatalf("HandleGet: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Entries must be present and be a JSON array, not null.
	entries, ok := body["entries"]
	if !ok {
		t.Fatal("entries field missing from response")
	}
	if entries == nil {
		t.Fatal("entries field is null; must be a JSON array")
	}
	entriesArr, isArr := entries.([]any)
	if !isArr {
		t.Fatalf("entries must be a JSON array, got %T", entries)
	}
	// Since last_seen is NULL and this is a since-last query, the result is
	// empty (no new releases before baseline capture), but it must be [] not null.
	if len(entriesArr) != 0 {
		t.Errorf("entries: expected empty array for null last_seen, got %d items", len(entriesArr))
	}
}
