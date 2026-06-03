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
