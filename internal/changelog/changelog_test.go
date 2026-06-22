package changelog

import (
	"os"
	"strings"
	"testing"
)

func loadSample(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/sample.md")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return string(b)
}

func TestParse(t *testing.T) {
	entries := Parse(loadSample(t))
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if entries[0].Version != "0.90.0" || entries[0].Date != "2026-06-20" {
		t.Fatalf("entry0 = %q/%q", entries[0].Version, entries[0].Date)
	}
	if len(entries[0].Groups) != 2 {
		t.Fatalf("entry0 groups = %d, want 2 (Features, Bug Fixes)", len(entries[0].Groups))
	}
	if entries[0].Groups[0].Title != "Features" {
		t.Fatalf("group0 title = %q", entries[0].Groups[0].Title)
	}
	// dev noise stripped: no commit hash, no (#NNNN), no "closes", no bold markers
	item := entries[0].Groups[0].Items[1]
	if item != "add nexctl setup command" {
		t.Fatalf("item not cleaned: %q", item)
	}
	if strings.Contains(item, "#") || strings.Contains(item, "closes") || strings.Contains(item, "*") {
		t.Fatalf("residual noise in %q", item)
	}
	// scope bold stripped but text kept
	if got := entries[0].Groups[0].Items[0]; got != "db: squash migrations into a single baseline" {
		t.Fatalf("scoped item = %q", got)
	}
}

func TestNewer(t *testing.T) {
	entries := Parse(loadSample(t))
	// since 0.17.1 -> only 0.90.0
	got := Newer(entries, "0.17.1")
	if len(got) != 1 || got[0].Version != "0.90.0" {
		t.Fatalf("Newer(0.17.1) = %+v", got)
	}
	// since current/newest -> empty
	if got := Newer(entries, "0.90.0"); len(got) != 0 {
		t.Fatalf("Newer(0.90.0) should be empty, got %d", len(got))
	}
	// since older than all -> all
	if got := Newer(entries, "0.1.0"); len(got) != 2 {
		t.Fatalf("Newer(0.1.0) = %d, want 2", len(got))
	}
	// since newer than newest (downgrade case) -> empty
	if got := Newer(entries, "9.9.9"); len(got) != 0 {
		t.Fatalf("Newer(9.9.9) should be empty, got %d", len(got))
	}
}

func TestRender(t *testing.T) {
	entries := Parse(loadSample(t))
	md := Render(entries[:1])
	if !strings.Contains(md, "## 0.90.0 — 2026-06-20") {
		t.Fatalf("render missing version header:\n%s", md)
	}
	if !strings.Contains(md, "### Features") || !strings.Contains(md, "- add nexctl setup command") {
		t.Fatalf("render missing grouped item:\n%s", md)
	}
	if strings.Contains(md, "](http") {
		t.Fatalf("render still contains links:\n%s", md)
	}
}

func TestAll_PlaceholderUnavailable(t *testing.T) {
	// With only the committed .gitkeep placeholder (no real CHANGELOG.md copied
	// in), the embedded changelog is unavailable.
	if _, ok := All(); ok {
		t.Skip("a real CHANGELOG.md is present in data/; skipping placeholder check")
	}
}
