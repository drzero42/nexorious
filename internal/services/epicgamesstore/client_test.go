package epicgamesstore

import (
	"os"
	"path/filepath"
	"testing"
)

// Tests use package epic (not epic_test) so they can verify unexported behaviour
// indirectly through the exported CaptureSnapshot / RestoreSnapshot methods.

func TestCaptureSnapshot_ExcludesLocksAndDirs(t *testing.T) {
	tmpDir := t.TempDir()
	c := NewClient(tmpDir)
	userID := "test-user"

	legendaryDir := filepath.Join(tmpDir, userID, "legendary")
	if err := os.MkdirAll(legendaryDir, 0o750); err != nil {
		t.Fatal(err)
	}

	// Write files that should be included and files that should be excluded.
	files := map[string]string{
		"user.json":              `{"displayName":"TestUser","account_id":"abc123"}`,
		"config.ini":             "[Legendary]\n",
		"version.json":           `{"version":"3.0.0"}`,
		"metadata/GameName.json": `{"title":"A Game"}`,
		// excluded:
		"tmp/progress.tmp":        "should be excluded",
		"manifests/game.manifest": "should be excluded",
		"legendary.lock":          "should be excluded",
	}
	for relPath, content := range files {
		fullPath := filepath.Join(legendaryDir, relPath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	snapshot, err := c.CaptureSnapshot(userID)
	if err != nil {
		t.Fatalf("CaptureSnapshot: %v", err)
	}

	for _, want := range []string{"user.json", "config.ini", "version.json", "metadata/GameName.json"} {
		if _, ok := snapshot[want]; !ok {
			t.Errorf("expected %q in snapshot", want)
		}
	}
	for _, notWant := range []string{"tmp/progress.tmp", "manifests/game.manifest", "legendary.lock"} {
		if _, ok := snapshot[notWant]; ok {
			t.Errorf("expected %q to be excluded from snapshot, but it was included", notWant)
		}
	}
}

func TestCaptureSnapshot_EmptyWhenDirMissing(t *testing.T) {
	tmpDir := t.TempDir()
	c := NewClient(tmpDir)

	snapshot, err := c.CaptureSnapshot("nonexistent-user")
	if err != nil {
		t.Fatalf("expected nil error for missing dir, got: %v", err)
	}
	if len(snapshot) != 0 {
		t.Errorf("expected empty snapshot, got %d entries", len(snapshot))
	}
}

func TestRestoreSnapshot_WritesFiles(t *testing.T) {
	tmpDir := t.TempDir()
	c := NewClient(tmpDir)
	userID := "test-user"

	snapshot := map[string]string{
		"user.json":              `{"displayName":"Tester","account_id":"xyz"}`,
		"config.ini":             "[Legendary]\nlog_level = warning\n",
		"metadata/SomeGame.json": `{"title":"Some Game"}`,
	}

	if err := c.RestoreSnapshot(userID, snapshot); err != nil {
		t.Fatalf("RestoreSnapshot: %v", err)
	}

	legendaryDir := filepath.Join(tmpDir, userID, "legendary")
	for relPath, want := range snapshot {
		got, err := os.ReadFile(filepath.Join(legendaryDir, relPath))
		if err != nil {
			t.Errorf("expected file %q to exist: %v", relPath, err)
			continue
		}
		if string(got) != want {
			t.Errorf("file %q: got %q, want %q", relPath, string(got), want)
		}
	}
}

func TestRestoreAndCapture_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	c := NewClient(tmpDir)
	userID := "test-user"

	original := map[string]string{
		"user.json":           `{"displayName":"Player","account_id":"p123"}`,
		"metadata/Game1.json": `{"title":"Game 1"}`,
		"metadata/Game2.json": `{"title":"Game 2"}`,
	}

	if err := c.RestoreSnapshot(userID, original); err != nil {
		t.Fatalf("RestoreSnapshot: %v", err)
	}
	restored, err := c.CaptureSnapshot(userID)
	if err != nil {
		t.Fatalf("CaptureSnapshot: %v", err)
	}
	for k, v := range original {
		if restored[k] != v {
			t.Errorf("round-trip %q: got %q, want %q", k, restored[k], v)
		}
	}
	if len(restored) != len(original) {
		t.Errorf("snapshot length mismatch: got %d, want %d", len(restored), len(original))
	}
}

func TestRestoreSnapshot_RejectsUnsafePaths(t *testing.T) {
	tmpDir := t.TempDir()
	c := NewClient(tmpDir)

	cases := []struct {
		name string
		key  string
	}{
		{"absolute path", "/etc/passwd"},
		{"parent traversal", "../../etc/passwd"},
		{"embedded parent", "metadata/../../escape"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snap := map[string]string{tc.key: "evil"}
			if err := c.RestoreSnapshot("u", snap); err == nil {
				t.Fatalf("expected error for unsafe path %q, got nil", tc.key)
			}
		})
	}
}

// TestParseLegendaryList_NamespaceFromMetadata verifies namespace is read from
// metadata.namespace (legendary's actual location), not a top-level key — the
// value Epic store-link resolution depends on.
func TestParseLegendaryList_NamespaceFromMetadata(t *testing.T) {
	// Shape mirrors `legendary list --json`: catalog fields nested under metadata,
	// no top-level "namespace".
	out := []byte(`[
	  {"app_name":"abc123","app_title":"A Plague Tale: Innocence","metadata":{"namespace":"ns-plague"}},
	  {"app_name":"def456","app_title":"Other Game","metadata":{"namespace":"ns-other"}}
	]`)
	entries, err := parseLegendaryList(out)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if entries[0].ExternalID != "abc123" || entries[0].Title != "A Plague Tale: Innocence" {
		t.Fatalf("unexpected entry[0]: %+v", entries[0])
	}
	if entries[0].Namespace != "ns-plague" {
		t.Fatalf("entry[0].Namespace = %q, want ns-plague (from metadata.namespace)", entries[0].Namespace)
	}
	if entries[1].Namespace != "ns-other" {
		t.Fatalf("entry[1].Namespace = %q, want ns-other", entries[1].Namespace)
	}
}
