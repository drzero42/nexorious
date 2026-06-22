// Package changelog parses the release-please CHANGELOG.md (embedded into the
// server binary) into structured per-version entries and slices it by semver.
// The repo-root CHANGELOG.md is copied into data/ only by asset-populating
// builds (make, release-artifacts.yaml, nix); test/lint builds see just the
// committed .gitkeep placeholder, so All reports unavailable and callers
// degrade gracefully.
package changelog

import (
	"embed"
	"regexp"
	"strings"

	"github.com/drzero42/nexorious/internal/services/updatecheck"
)

//go:embed all:data
var dataFS embed.FS

// Entry is one released version's notes.
type Entry struct {
	Version string  `json:"version"`
	Date    string  `json:"date"`
	Groups  []Group `json:"groups"`
}

// Group is a titled list of human-readable change descriptions.
type Group struct {
	Title string   `json:"title"`
	Items []string `json:"items"`
}

var (
	// "## [0.90.0](compare-url) (2026-06-20)" — link and date are optional.
	reHeader = regexp.MustCompile(`^## \[?([0-9]+\.[0-9]+\.[0-9]+)\]?(?:\([^)]*\))?(?:\s+\(([0-9]{4}-[0-9]{2}-[0-9]{2})\))?`)
	reGroup  = regexp.MustCompile(`^### (.+)$`)
	reItem   = regexp.MustCompile(`^\*\s+(.+)$`)

	reCloses    = regexp.MustCompile(`(?i),?\s*closes \[#\d+\]\([^)]*\)`)
	reCommitRef = regexp.MustCompile(`\s*\(\[[0-9a-f]{7,40}\]\([^)]*\)\)`)
	reIssueRef  = regexp.MustCompile(`\s*\(\[#\d+\]\([^)]*\)\)`)
	reBold      = regexp.MustCompile(`\*\*`)
)

// cleanItem strips release-please dev noise: trailing "closes #N", commit-hash
// and issue-number link refs, and bold scope markers — keeping the human text.
func cleanItem(s string) string {
	s = reCloses.ReplaceAllString(s, "")
	s = reCommitRef.ReplaceAllString(s, "")
	s = reIssueRef.ReplaceAllString(s, "")
	s = reBold.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

// Parse turns a release-please CHANGELOG.md into ordered entries (newest first,
// matching the file order). Lines before the first version header are ignored.
func Parse(md string) []Entry {
	var entries []Entry
	hasGroup := false
	for _, line := range strings.Split(md, "\n") {
		switch {
		case reHeader.MatchString(line):
			m := reHeader.FindStringSubmatch(line)
			entries = append(entries, Entry{Version: m[1], Date: m[2]})
			hasGroup = false
		case len(entries) == 0:
			// preamble before the first "## [x.y.z]" header
		case reGroup.MatchString(line):
			m := reGroup.FindStringSubmatch(line)
			i := len(entries) - 1
			entries[i].Groups = append(entries[i].Groups, Group{Title: strings.TrimSpace(m[1])})
			hasGroup = true
		case reItem.MatchString(line):
			m := reItem.FindStringSubmatch(line)
			item := cleanItem(m[1])
			if item == "" {
				continue
			}
			i := len(entries) - 1
			if !hasGroup {
				entries[i].Groups = append(entries[i].Groups, Group{Title: "Changes"})
				hasGroup = true
			}
			g := len(entries[i].Groups) - 1
			entries[i].Groups[g].Items = append(entries[i].Groups[g].Items, item)
		}
	}
	return entries
}

// Newer returns the entries strictly newer than sinceExclusive (semver). An
// empty or invalid sinceExclusive returns nothing, so callers never blast the
// full history through the since-last path without a captured baseline.
func Newer(entries []Entry, sinceExclusive string) []Entry {
	if !updatecheck.IsValidVersion(sinceExclusive) {
		return nil
	}
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		if updatecheck.Compare(e.Version, sinceExclusive) > 0 {
			out = append(out, e)
		}
	}
	return out
}

// Render produces a clean markdown slice for the web view: "## <version> —
// <date>", regrouped bullet lists, all dev-noise links removed.
func Render(entries []Entry) string {
	var b strings.Builder
	for _, e := range entries {
		b.WriteString("## ")
		b.WriteString(e.Version)
		if e.Date != "" {
			b.WriteString(" — ")
			b.WriteString(e.Date)
		}
		b.WriteString("\n\n")
		for _, g := range e.Groups {
			b.WriteString("### ")
			b.WriteString(g.Title)
			b.WriteString("\n\n")
			for _, it := range g.Items {
				b.WriteString("- ")
				b.WriteString(it)
				b.WriteString("\n")
			}
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// All parses the embedded changelog. ok is false when only the .gitkeep
// placeholder is present (test/lint builds) or the file is empty.
func All() (entries []Entry, ok bool) {
	b, err := dataFS.ReadFile("data/CHANGELOG.md")
	if err != nil {
		return nil, false
	}
	if strings.TrimSpace(string(b)) == "" {
		return nil, false
	}
	return Parse(string(b)), true
}
