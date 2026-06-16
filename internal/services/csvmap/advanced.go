package csvmap

import (
	"sort"
	"strconv"
	"strings"

	"github.com/drzero42/nexorious/internal/services/importmodel"
)

// extractStatusFlags resolves play_status from ordered boolean-flag columns: the
// first rule (in order) whose column holds a truthy value wins; otherwise Default
// ("not_started" when Default is empty). Replaces darkadia.resolvePlayStatus.
func extractStatusFlags(rec []string, idx map[string]int, sf *StatusFlags) string {
	for _, rule := range sf.Rules {
		v := normKey(cell(rec, idx, rule.Column))
		if v == "" {
			continue
		}
		for _, t := range rule.Truthy {
			if normKey(t) == v {
				return rule.Status
			}
		}
	}
	if sf.Default != "" {
		return sf.Default
	}
	return "not_started"
}

// parseHMM parses Darkadia "H:MM" playtime into hours. "148:00" -> 148.0,
// "10:30" -> 10.5. Empty, malformed, or non-positive -> nil. Replaces
// darkadia.parseDuration.
func parseHMM(s string) *float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return nil
	}
	h, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	m, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil {
		return nil
	}
	v := float64(h) + float64(m)/60.0
	if v <= 0 {
		return nil
	}
	return &v
}

// splitAggregate splits a comma-separated owned-platform list, trimming and
// dropping empties. Ported from darkadia.splitAggregate.
func splitAggregate(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// effectiveSource returns the source string for a copy row: SourceColumn, unless
// it equals OtherSentinel (case-insensitive), in which case SourceOtherColumn.
func effectiveSource(pt *PlatformTables, rec []string, idx map[string]int) string {
	src := cell(rec, idx, pt.SourceColumn)
	if pt.OtherSentinel != "" && strings.EqualFold(src, pt.OtherSentinel) {
		return cell(rec, idx, pt.SourceOtherColumn)
	}
	return src
}

// recognizedStorefront returns the storefront slug for a recognized digital
// source (normalized exact match, else longest recognized prefix followed by a
// space). Ported from darkadia.recognizedStorefront.
func recognizedStorefront(pt *PlatformTables, eff string) (string, bool) {
	k := normKey(eff)
	if slug, ok := pt.Storefronts[k]; ok {
		return slug, true
	}
	names := make([]string, 0, len(pt.Storefronts))
	for name := range pt.Storefronts {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if len(names[i]) != len(names[j]) {
			return len(names[i]) > len(names[j]) // longer first
		}
		return names[i] < names[j] // lexicographic tie-break
	})
	for _, name := range names {
		if strings.HasPrefix(k, name+" ") {
			return pt.Storefronts[name], true
		}
	}
	return "", false
}

// resolveStorefront applies the per-copy storefront precedence: recognized
// source -> physical (with provenance note) -> unrecognized source (note only)
// -> inferred -> none. Ported from darkadia.resolveStorefront.
func resolveStorefront(pt *PlatformTables, inferred *string, eff, media string) (*string, string) {
	if eff != "" {
		if slug, ok := recognizedStorefront(pt, eff); ok {
			s := slug
			return &s, ""
		}
	}
	if pt.MediaPhysicalValue != "" && media == pt.MediaPhysicalValue {
		s := "physical"
		note := ""
		if eff != "" {
			note = "Purchased physically from " + eff + "."
		}
		return &s, note
	}
	if eff != "" {
		return nil, "Purchased from " + eff + "."
	}
	if inferred != nil {
		s := *inferred
		return &s, ""
	}
	return nil, ""
}

// buildGrouped implements blank-continuation grouping (Grouping.CopyRows): a row
// whose ContinuationColumn is non-blank starts a new game; each following blank
// row is a copy of the current game. Each group is consolidated via the platform
// tables / note assembly. Replaces darkadia.Parse's grouping loop.
func buildGrouped(rows [][]string, idx map[string]int, cfg Config) []importmodel.Game {
	contCol := cfg.Grouping.CopyRows.ContinuationColumn
	type group struct {
		named  []string
		copies [][]string
	}
	var groups []group
	for _, rec := range rows {
		if cell(rec, idx, contCol) != "" {
			groups = append(groups, group{named: rec, copies: [][]string{rec}})
			continue
		}
		if len(groups) == 0 {
			continue // continuation before any named row — malformed; skip defensively
		}
		g := &groups[len(groups)-1]
		g.copies = append(g.copies, rec)
	}
	out := make([]importmodel.Game, 0, len(groups))
	for _, grp := range groups {
		out = append(out, consolidateGroup(grp.named, grp.copies, idx, cfg))
	}
	return out
}

// consolidateGroup builds one Game from a named row plus its copy rows, applying
// the platform tables, storefront precedence, (platform, storefront) dedup
// (earliest acquired date kept), and ordered note assembly. Ported from
// darkadia.consolidate.
func consolidateGroup(named []string, copies [][]string, idx map[string]int, cfg Config) importmodel.Game {
	pt := cfg.Platform.Tables
	g := importmodel.Game{
		Title:      cell(named, idx, cfg.Columns.Title),
		PlayStatus: extractStatusFlags(named, idx, cfg.Status.Flags),
		IsLoved:    extractLoved(named, idx, cfg),
		CreatedAt:  extractDate(cell(named, idx, cfg.Columns.CreatedAt), cfg),
	}
	if r := extractRating(cell(named, idx, cfg.Columns.Rating), cfg); r != nil {
		g.PersonalRating = r
	}
	g.Tags = extractTags(named, idx, cfg)
	if h := extractHours(named, idx, cfg); h != nil {
		g.HoursPlayed = h
	}

	var noteLines []string
	addNote := func(line string) {
		if line == "" {
			return
		}
		for _, e := range noteLines {
			if e == line {
				return
			}
		}
		noteLines = append(noteLines, line)
	}

	// (1) Review line.
	if a := cfg.Notes.Assembly; a != nil {
		if rev := cell(named, idx, a.ReviewColumn); rev != "" {
			if subj := cell(named, idx, a.ReviewSubjectColumn); subj != "" {
				addNote("Review — " + subj + "\n" + rev)
			} else {
				addNote("Review: " + rev)
			}
		}
	}

	// (2) Owned set from aggregate + per-copy platform strings (unmapped -> note).
	owned := map[string]bool{}
	ownedInferred := map[string]*string{}
	markOwned := func(s string) {
		m, ok := pt.Platforms[s]
		if !ok {
			addNote("Owned on " + s + " (no Nexorious platform mapping).")
			return
		}
		owned[m.Slug] = true
		if m.InferredStorefront != nil && ownedInferred[m.Slug] == nil {
			ownedInferred[m.Slug] = m.InferredStorefront
		}
	}
	for _, s := range splitAggregate(cell(named, idx, pt.AggregateColumn)) {
		markOwned(s)
	}
	for _, row := range copies {
		if p := cell(row, idx, pt.PlatformColumn); p != "" {
			markOwned(p)
		}
	}

	// Dedup on (platform, storefront), keeping the earliest acquired date.
	type key struct{ platform, storefront string }
	seen := map[key]int{}
	add := func(slug string, sfp *string, date string) {
		sfKey := ""
		if sfp != nil {
			sfKey = *sfp
		}
		k := key{slug, sfKey}
		if i, ok := seen[k]; ok {
			if date != "" && (g.Platforms[i].AcquiredDate == "" || date < g.Platforms[i].AcquiredDate) {
				g.Platforms[i].AcquiredDate = date
			}
			return
		}
		seen[k] = len(g.Platforms)
		g.Platforms = append(g.Platforms, importmodel.Platform{Platform: slug, Storefront: sfp, AcquiredDate: date})
	}

	// (3) Per-copy storefront resolution + provenance notes.
	slugHasCopy := map[string]bool{}
	for _, row := range copies {
		ps := cell(row, idx, pt.PlatformColumn)
		if ps == "" {
			continue
		}
		m, ok := pt.Platforms[ps]
		if !ok {
			continue // already noted via markOwned
		}
		slugHasCopy[m.Slug] = true
		sfp, note := resolveStorefront(pt, m.InferredStorefront, effectiveSource(pt, row, idx), cell(row, idx, pt.MediaColumn))
		addNote(note)
		add(m.Slug, sfp, cell(row, idx, pt.PurchaseDateColumn))
	}

	// (4) Copy notes.
	if a := cfg.Notes.Assembly; a != nil && a.CopyNoteColumn != "" {
		for _, row := range copies {
			if cn := cell(row, idx, a.CopyNoteColumn); cn != "" {
				addNote("Copy note: " + cn)
			}
		}
	}

	// Owned-but-no-copy slugs, in sorted order, with any inferred storefront.
	slugs := make([]string, 0, len(owned))
	for s := range owned {
		slugs = append(slugs, s)
	}
	sort.Strings(slugs)
	for _, slug := range slugs {
		if slugHasCopy[slug] {
			continue
		}
		add(slug, ownedInferred[slug], "")
	}

	// Verbatim notes column + assembled note lines.
	verbatim := cell(named, idx, cfg.Notes.Column)
	var b strings.Builder
	if verbatim != "" {
		b.WriteString(verbatim)
	}
	if len(noteLines) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(strings.Join(noteLines, "\n"))
	}
	if b.Len() > 0 {
		s := b.String()
		g.PersonalNotes = &s
	}
	return g
}
