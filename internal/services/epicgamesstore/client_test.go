package epicgamesstore

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func openTestSession(t *testing.T) *session {
	t.Helper()
	s, err := NewClient(t.TempDir()).openSession()
	if err != nil {
		t.Fatalf("openSession: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// sessionDirs lists the session directories left under workDir.
func sessionDirs(t *testing.T, workDir string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(workDir, sessionPrefix+"*"))
	if err != nil {
		t.Fatal(err)
	}
	return matches
}

// fakeLegendary puts a stand-in legendary on PATH. `auth --code good` writes
// user.json into $XDG_CONFIG_HOME/legendary; any other code fails.
func fakeLegendary(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake legendary is a shell script")
	}
	bin := t.TempDir()
	script := `#!/bin/sh
if [ "$1" = auth ] && [ "$3" = good ]; then
  mkdir -p "$XDG_CONFIG_HOME/legendary"
  echo '{"displayName":"Player","account_id":"acct"}' > "$XDG_CONFIG_HOME/legendary/user.json"
  exit 0
fi
echo "auth failed" >&2
exit 1
`
	if err := os.WriteFile(filepath.Join(bin, "legendary"), []byte(script), 0o700); err != nil { //nolint:gosec // test fixture must be executable
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// Files from an earlier sign-in (the legacy per-user dir, or a session a crash
// left behind) must never end up in a new sign-in's snapshot.
func TestAuthenticate_IgnoresStaleFiles(t *testing.T) {
	fakeLegendary(t)
	workDir := t.TempDir()
	for _, dir := range []string{
		filepath.Join(workDir, "user-1", "legendary"),
		filepath.Join(workDir, sessionPrefix+"crashed", "legendary"),
	} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "stale.json"), []byte("old"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	info, snapshot, err := NewClient(workDir).Authenticate(context.Background(), "good")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if info.AccountID != "acct" {
		t.Errorf("account id = %q, want acct", info.AccountID)
	}
	if _, ok := snapshot["stale.json"]; ok || len(snapshot) != 1 {
		t.Errorf("snapshot = %v, want only user.json", snapshot)
	}
}

// The sign-in's session dir holds credentials, so it goes whether or not the
// sign-in succeeded.
func TestAuthenticate_RemovesSessionDir(t *testing.T) {
	fakeLegendary(t)
	for _, code := range []string{"good", "bad"} {
		t.Run(code, func(t *testing.T) {
			workDir := t.TempDir()
			_, _, err := NewClient(workDir).Authenticate(context.Background(), code)
			if (err != nil) != (code == "bad") {
				t.Fatalf("Authenticate(%s) err = %v", code, err)
			}
			if left := sessionDirs(t, workDir); len(left) != 0 {
				t.Errorf("session dirs left behind: %v", left)
			}
		})
	}
}

// Two operations for the same user must not share files: one's restore must
// not show up in the other's capture.
func TestOpenSession_IsolatesOperations(t *testing.T) {
	c := NewClient(t.TempDir())
	a, err := c.openSession()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = a.Close() }()
	b, err := c.openSession()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()

	if err := a.RestoreSnapshot(map[string]string{"user.json": "a"}); err != nil {
		t.Fatal(err)
	}
	got, err := b.CaptureSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("second session sees the first one's files: %v", got)
	}
}

func TestSessionClose_RemovesCredentials(t *testing.T) {
	workDir := t.TempDir()
	s, err := NewClient(workDir).openSession()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RestoreSnapshot(map[string]string{"user.json": "secret"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if left := sessionDirs(t, workDir); len(left) != 0 {
		t.Errorf("session dirs left behind: %v", left)
	}
}

func TestCleanup_RemovesLegacyUserDirOnly(t *testing.T) {
	workDir := t.TempDir()
	c := NewClient(workDir)
	legacy := filepath.Join(workDir, "user-1", "legendary")
	if err := os.MkdirAll(legacy, 0o750); err != nil {
		t.Fatal(err)
	}
	running, err := c.openSession()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = running.Close() }()

	if err := c.Cleanup(context.Background(), "user-1"); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workDir, "user-1")); !os.IsNotExist(err) {
		t.Errorf("legacy dir still present (stat err %v)", err)
	}
	if _, err := os.Stat(running.dir); err != nil {
		t.Errorf("Cleanup removed a running session's dir: %v", err)
	}
}

func TestCleanup_RejectsUnsafeUserID(t *testing.T) {
	c := NewClient(t.TempDir())
	for _, id := range []string{"", ".", "..", "../x", "a/b", sessionPrefix + "x"} {
		if err := c.Cleanup(context.Background(), id); err == nil {
			t.Errorf("Cleanup(%q) = nil, want error", id)
		}
	}
}

// Tests use package epic (not epic_test) so they can verify unexported behaviour
// indirectly through the exported CaptureSnapshot / RestoreSnapshot methods.

func TestCaptureSnapshot_ExcludesLocksAndDirs(t *testing.T) {
	s := openTestSession(t)

	legendaryDir := s.legendaryDir()
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

	snapshot, err := s.CaptureSnapshot()
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
	s := openTestSession(t)

	snapshot, err := s.CaptureSnapshot()
	if err != nil {
		t.Fatalf("expected nil error for missing dir, got: %v", err)
	}
	if len(snapshot) != 0 {
		t.Errorf("expected empty snapshot, got %d entries", len(snapshot))
	}
}

func TestRestoreSnapshot_WritesFiles(t *testing.T) {
	s := openTestSession(t)

	snapshot := map[string]string{
		"user.json":              `{"displayName":"Tester","account_id":"xyz"}`,
		"config.ini":             "[Legendary]\nlog_level = warning\n",
		"metadata/SomeGame.json": `{"title":"Some Game"}`,
	}

	if err := s.RestoreSnapshot(snapshot); err != nil {
		t.Fatalf("RestoreSnapshot: %v", err)
	}

	legendaryDir := s.legendaryDir()
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
	s := openTestSession(t)

	original := map[string]string{
		"user.json":           `{"displayName":"Player","account_id":"p123"}`,
		"metadata/Game1.json": `{"title":"Game 1"}`,
		"metadata/Game2.json": `{"title":"Game 2"}`,
	}

	if err := s.RestoreSnapshot(original); err != nil {
		t.Fatalf("RestoreSnapshot: %v", err)
	}
	restored, err := s.CaptureSnapshot()
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
	s := openTestSession(t)

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
			if err := s.RestoreSnapshot(snap); err == nil {
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

// TestParseLegendaryList_SkipsDLC verifies DLC entries are filtered out. legendary
// marks a DLC by nesting metadata.mainGameItem (the main game it belongs to);
// main games carry no mainGameItem. The legacy top-level main_game_appname field
// is never emitted by legendary, so the filter must key off metadata.mainGameItem.
func TestParseLegendaryList_SkipsDLC(t *testing.T) {
	// Shape mirrors `legendary list --json`: a main game (no mainGameItem) and a
	// DLC entry (metadata.mainGameItem present and non-null).
	out := []byte(`[
	  {"app_name":"main123","app_title":"Main Game","metadata":{"namespace":"ns-main"}},
	  {"app_name":"dlc456","app_title":"Main Game - Season Pass","metadata":{"namespace":"ns-main","mainGameItem":{"id":"main123"}}}
	]`)
	entries, err := parseLegendaryList(out)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1 (DLC should be filtered out)", len(entries))
	}
	if entries[0].ExternalID != "main123" {
		t.Fatalf("kept entry ExternalID = %q, want main123 (the main game)", entries[0].ExternalID)
	}
}
