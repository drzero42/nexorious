package notify

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFormatSyncFailed(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{"storefront": "steam", "error": "bad token"})
	title, body, _ := Format(TypeSyncFailed, payload)
	if !strings.Contains(strings.ToLower(title), "sync") {
		t.Errorf("title missing 'sync': %q", title)
	}
	if !strings.Contains(body, "steam") || !strings.Contains(body, "bad token") {
		t.Errorf("body missing detail: %q", body)
	}
}

func TestFormatSyncDiff(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{
		"added":   []map[string]any{{"title": "Hades", "platforms": []string{"Steam"}}},
		"removed": []map[string]any{{"title": "Old Game", "platforms": []string{"GOG"}}},
	})
	title, body, _ := Format(TypeSyncDiff, payload)
	if title == "" {
		t.Error("expected non-empty title")
	}
	if !strings.Contains(body, "Hades") || !strings.Contains(body, "Old Game") {
		t.Errorf("diff body missing games: %q", body)
	}
}

func TestFormatSyncDiff_EmptyPlatformsOmitsBrackets(t *testing.T) {
	// A game with no platforms must render without any bracket notation.
	payload, _ := json.Marshal(map[string]any{
		"added":   []map[string]any{{"title": "NoPlat", "platforms": []string{}}},
		"removed": []map[string]any{},
	})
	_, body, _ := Format(TypeSyncDiff, payload)
	if !strings.Contains(body, "NoPlat") {
		t.Errorf("expected body to contain 'NoPlat', got: %q", body)
	}
	if strings.Contains(body, "NoPlat []") {
		t.Errorf("expected no empty brackets for no-platform game, got: %q", body)
	}
	if strings.Contains(body, "[]") {
		t.Errorf("expected no empty brackets anywhere in body, got: %q", body)
	}
}

func TestFormatUnknownTypeIsSafe(t *testing.T) {
	title, body, _ := Format("totally.unknown", json.RawMessage(`{}`))
	if title == "" || body == "" {
		t.Errorf("unknown type should yield a safe fallback, got title=%q body=%q", title, body)
	}
}

// samplePayloads holds one representative, well-formed payload per registered
// event type. Adding a new event type to the registry without adding a sample
// here fails TestFormat_AllRegisteredTypesRoundTrip.
var samplePayloads = map[string]any{
	TypeSyncCompleted:           SyncCompletedPayload{Storefront: "Steam", JobID: "j1"},
	TypeSyncCompletedWithErrors: SyncCompletedWithErrorsPayload{Storefront: "Steam", Failed: 3, JobID: "j1"},
	TypeSyncFailed:              SyncFailedPayload{Storefront: "Steam", Error: "bad token", JobID: "j1"},
	TypeSyncAuthExpired:         SyncAuthExpiredPayload{Storefront: "Steam"},
	TypeSyncNeedsReview:         SyncNeedsReviewPayload{Storefront: "Steam", Count: 2, JobID: "j1"},
	TypeSyncDiff:                SyncDiffPayload{Added: []DiffGame{{Title: "Hades", Platforms: []string{"Steam"}}}, JobID: "j1"},
	TypeImportCompleted:         ImportCompletedPayload{JobID: "j1"},
	TypeImportFailed:            ImportFailedPayload{JobID: "j1", Failed: 2, Error: "2 item(s) failed to import"},
	TypeExportCompleted:         ExportCompletedPayload{JobID: "j1", FilePath: "/tmp/export.zip"},
	TypeExportFailed:            ExportFailedPayload{JobID: "j1", Error: "disk full"},
	TypeAdminBackupCompleted:    BackupCompletedPayload{BackupID: "b1"},
	TypeAdminBackupFailed:       BackupFailedPayload{Error: "s3 unreachable"},
	TypeAdminMaintCompleted:     MaintPayload{Action: "prune_events", Count: 5},
	TypeAdminMaintFailed:        MaintPayload{Action: "prune_events", Error: "query failed"},
}

// wantRender is the exact (title, body) each sample must produce. Locks the
// user-facing copy; intentional wording changes update this map alongside
// formatters.go. Note: this asserts render output, not which struct a Format
// case decodes into — struct identity at emit sites is enforced by the
// compiler, and a mispairing with differing JSON keys surfaces here as a
// render mismatch.
var wantRender = map[string]struct{ title, body string }{
	TypeSyncCompleted:           {"Sync completed", "Your Steam sync completed successfully."},
	TypeSyncCompletedWithErrors: {"Sync completed with errors", "Your Steam sync finished with 3 failed item(s)."},
	TypeSyncFailed:              {"Sync failed", "Your Steam sync failed: bad token"},
	TypeSyncAuthExpired:         {"Storefront needs reconnect", "Your Steam connection has expired. Open Sync settings to reconnect."},
	TypeSyncNeedsReview:         {"Sync needs review", "Your Steam sync has 2 item(s) needing review."},
	TypeSyncDiff:                {"Game library changes", "Added (1):\n  + Hades [Steam]"},
	TypeImportCompleted:         {"Import completed", "Your import finished successfully."},
	TypeImportFailed:            {"Import failed", "Your import failed: 2 item(s) failed to import"},
	TypeExportCompleted:         {"Export completed", "Your export is ready."},
	TypeExportFailed:            {"Export failed", "Your export failed: disk full"},
	TypeAdminBackupCompleted:    {"Backup completed", "A scheduled backup completed successfully."},
	TypeAdminBackupFailed:       {"Backup failed", "A scheduled backup failed: s3 unreachable"},
	TypeAdminMaintCompleted:     {"Maintenance completed", "Maintenance task completed (prune_events)."},
	TypeAdminMaintFailed:        {"Maintenance failed", "Maintenance task failed (prune_events) - query failed."},
}

func TestFormat_AllRegisteredTypesRoundTrip(t *testing.T) {
	for _, et := range Registry() {
		t.Run(et.Type, func(t *testing.T) {
			sample, ok := samplePayloads[et.Type]
			if !ok {
				t.Fatalf("no sample payload for registered type %q — add one to samplePayloads", et.Type)
			}
			want, ok := wantRender[et.Type]
			if !ok {
				t.Fatalf("no expected render for registered type %q — add one to wantRender", et.Type)
			}
			raw, err := json.Marshal(sample)
			if err != nil {
				t.Fatalf("marshal sample: %v", err)
			}
			title, body, derr := Format(et.Type, raw)
			if derr != nil {
				t.Fatalf("decode error on well-formed payload: %v", derr)
			}
			if body == "An event occurred: "+et.Type {
				t.Fatalf("type %q fell through to the generic body", et.Type)
			}
			if title != want.title || body != want.body {
				t.Fatalf("render mismatch:\n got  title=%q body=%q\n want title=%q body=%q", title, body, want.title, want.body)
			}
		})
	}
}

func TestFormat_DecodeFailureSafeFallback(t *testing.T) {
	cases := []struct {
		name      string
		eventType string
		payload   string
		wantBody  string
	}{
		{"completed", TypeSyncCompleted, `{"storefront":123}`, "Your sync completed successfully."},
		{"with_errors", TypeSyncCompletedWithErrors, `{"failed":"oops"}`, "Your library sync finished with some failed item(s)."},
		{"needs_review", TypeSyncNeedsReview, `{"count":"oops"}`, "Your library sync has item(s) needing review."},
		{"maint_completed", TypeAdminMaintCompleted, `"notobject"`, "Maintenance task completed."},
		{"sync_failed", TypeSyncFailed, `["not","an","object"]`, "Sync failed."},
		{"sync_auth_expired", TypeSyncAuthExpired, `["not","an","object"]`, "A storefront connection has expired. Open Sync settings to reconnect."},
		{"sync_diff", TypeSyncDiff, `{"added":"nope"}`, "Your game library changed."},
		{"import_failed", TypeImportFailed, `"notobject"`, "Your import failed."},
		{"export_failed", TypeExportFailed, `"notobject"`, "Your export failed."},
		{"backup_failed", TypeAdminBackupFailed, `"notobject"`, "A scheduled backup failed."},
		{"maint_failed", TypeAdminMaintFailed, `"notobject"`, "Maintenance task failed."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			title, body, err := Format(tc.eventType, json.RawMessage(tc.payload))
			if err == nil {
				t.Fatalf("expected a decode error, got nil")
			}
			if title == "" {
				t.Fatalf("title must not be empty")
			}
			if body != tc.wantBody {
				t.Fatalf("body = %q, want %q", body, tc.wantBody)
			}
		})
	}
}
