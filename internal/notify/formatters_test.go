package notify

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFormatSyncFailed(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{"storefront": "steam", "error": "bad token"})
	title, body := Format(TypeSyncFailed, payload)
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
	title, body := Format(TypeSyncDiff, payload)
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
	_, body := Format(TypeSyncDiff, payload)
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
	title, body := Format("totally.unknown", json.RawMessage(`{}`))
	if title == "" || body == "" {
		t.Errorf("unknown type should yield a safe fallback, got title=%q body=%q", title, body)
	}
}
