package darkadia

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/drzero42/nexorious/internal/services/importmodel"
)

// ErrInvalidHeader signals the file is not a Darkadia export. The upload handler
// turns this into a 400 "not a Darkadia export".
var ErrInvalidHeader = errors.New("not a Darkadia export (header mismatch)")

// header is the canonical internal column layout: the 29-column required
// signature (0–28) followed by 5 optional feature-toggle columns (29–33) that
// some exports add. Rows are normalized into this layout by header name, so the
// real file's column order and any extra columns it carries do not matter.
var header = []string{
	"Name", "Added", "Loved", "Owned", "Played", "Playing", "Finished",
	"Mastered", "Dominated", "Shelved", "Rating", "Copy label", "Copy Release",
	"Copy platform", "Copy media", "Copy media other", "Copy source",
	"Copy source other", "Copy purchase date", "Copy box", "Copy box condition",
	"Copy box notes", "Copy manual", "Copy manual condition", "Copy manual notes",
	"Copy complete", "Copy complete notes", "Platforms", "Notes",
	// Optional (feature-toggle) columns — read only when present.
	"Tags", "Time played", "Review subject", "Review", "Copy notes",
}

// requiredColumnCount is the number of leading canonical columns that must be
// present (by name) for a file to be accepted as a Darkadia export.
const requiredColumnCount = 29

// Column indices (only the ones the importer reads).
const (
	colName            = 0
	colAdded           = 1
	colLoved           = 2
	colOwned           = 3
	colPlayed          = 4
	colPlaying         = 5
	colFinished        = 6
	colMastered        = 7
	colDominated       = 8
	colShelved         = 9
	colRating          = 10
	colCopyPlatform    = 13
	colCopyMedia       = 14
	colCopySource      = 16
	colCopySourceOther = 17
	colCopyPurchase    = 18
	colPlatforms       = 27
	colNotes           = 28
	colTags            = 29
	colTimePlayed      = 30
	colReviewSubject   = 31
	colReview          = 32
	colCopyNotes       = 33
)

// rawGame is one game grouped from the CSV: the named row plus its copy rows.
type rawGame struct {
	named  []string
	copies [][]string // every row (named + continuations), each normalized to the canonical layout
}

// Parse reads a Darkadia CSV and returns one consolidated Game per title.
func Parse(raw []byte) ([]importmodel.Game, error) {
	r := csv.NewReader(bytes.NewReader(raw))
	r.FieldsPerRecord = -1 // tolerate ragged rows (missing trailing columns)

	first, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	cols := buildColumnIndex(first)
	if !hasRequiredColumns(cols) {
		return nil, ErrInvalidHeader
	}

	var raws []rawGame
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read row: %w", err)
		}
		row := normalize(rec, cols)
		if row[colName] != "" {
			raws = append(raws, rawGame{named: row, copies: [][]string{row}})
			continue
		}
		if len(raws) == 0 {
			// Continuation row before any named row — malformed; skip defensively.
			continue
		}
		last := &raws[len(raws)-1]
		last.copies = append(last.copies, row)
	}

	games := make([]importmodel.Game, 0, len(raws))
	for _, rg := range raws {
		games = append(games, consolidate(rg))
	}
	return games, nil
}

// buildColumnIndex maps each header name to its position. First occurrence wins.
func buildColumnIndex(hdr []string) map[string]int {
	m := make(map[string]int, len(hdr))
	for i, name := range hdr {
		if _, ok := m[name]; !ok {
			m[name] = i
		}
	}
	return m
}

// hasRequiredColumns reports whether every required signature column is present.
func hasRequiredColumns(cols map[string]int) bool {
	for _, name := range header[:requiredColumnCount] {
		if _, ok := cols[name]; !ok {
			return false
		}
	}
	return true
}

// normalize maps a raw record into the canonical layout by header name. Absent
// columns and ragged short rows yield empty strings (ragged-row tolerance).
func normalize(rec []string, cols map[string]int) []string {
	out := make([]string, len(header))
	for canon, name := range header {
		if src, ok := cols[name]; ok && src < len(rec) {
			out[canon] = rec[src]
		}
	}
	return out
}

// platformMapping is a Nexorious platform slug plus an optional inferred
// storefront (used only as a fallback when no copy supplies one).
type platformMapping struct {
	slug     string
	inferred *string
}

// platformTable maps every Darkadia platform string in the reference export to
// a Nexorious slug. A string absent from this table is preserved in the note,
// never dropped and never a failure.
var platformTable = map[string]platformMapping{
	"PC":                         {slug: "pc-windows"},
	"Linux":                      {slug: "pc-linux"},
	"Mac":                        {slug: "mac"},
	"PlayStation 4":              {slug: "playstation-4"},
	"PlayStation 5":              {slug: "playstation-5"},
	"PlayStation 3":              {slug: "playstation-3"},
	"PlayStation Network (PS3)":  {slug: "playstation-3", inferred: ptrStr("playstation-store")},
	"PlayStation Network (Vita)": {slug: "playstation-vita", inferred: ptrStr("playstation-store")},
	"Nintendo Switch":            {slug: "nintendo-switch"},
	"Wii":                        {slug: "nintendo-wii"},
	"Xbox 360":                   {slug: "xbox-360"},
	"Xbox 360 Games Store":       {slug: "xbox-360", inferred: ptrStr("microsoft-store")},
	"Android":                    {slug: "android"},
	"PlayStation 2":              {slug: "playstation-2"},
	"PlayStation Network (PSP)":  {slug: "playstation-psp", inferred: ptrStr("playstation-store")},
}

func ptrStr(s string) *string { return &s }

// storefrontTable maps a recognized digital source (lowercased) to a Nexorious
// storefront slug. Spelling variants from the reference export are included.
var storefrontTable = map[string]string{
	"sony entertainment network": "playstation-store",
	"epic games store":           "epic-games-store",
	"epic game store":            "epic-games-store",
	"epic gamestore":             "epic-games-store",
	"epic":                       "epic-games-store",
	"gog":                        "gog",
	"humble bundle":              "humble-bundle",
	"steam":                      "steam",
	"nintendo eshop":             "nintendo-eshop",
	"origin":                     "origin-ea-app",
	"gamersgate":                 "gamersgate",
	"google play":                "google-play-store",
	"uplay":                      "uplay",
	"ubisoft club":               "uplay",
}

// effectiveSource returns the source string for a copy row: Copy source, unless
// it is the literal "Other", in which case Copy source other.
func effectiveSource(row []string) string {
	src := strings.TrimSpace(row[colCopySource])
	if strings.EqualFold(src, "Other") {
		return strings.TrimSpace(row[colCopySourceOther])
	}
	return src
}

// recognizedStorefront returns the slug for a recognized digital source. It
// tolerates extra free text after a recognized name (e.g. "Uplay (coupon …)").
func recognizedStorefront(eff string) (string, bool) {
	key := strings.ToLower(strings.TrimSpace(eff))
	if slug, ok := storefrontTable[key]; ok {
		return slug, true
	}
	// Longest recognized prefix wins, deterministically (avoids random map iteration).
	names := make([]string, 0, len(storefrontTable))
	for name := range storefrontTable {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if len(names[i]) != len(names[j]) {
			return len(names[i]) > len(names[j]) // longer first
		}
		return names[i] < names[j] // lexicographic tie-break
	})
	for _, name := range names {
		if strings.HasPrefix(key, name+" ") {
			return storefrontTable[name], true
		}
	}
	return "", false
}

// resolveStorefront applies the per-copy storefront precedence. It returns the
// storefront slug (or nil) and an optional provenance note line ("" = none).
func resolveStorefront(inferred *string, eff, media string) (*string, string) {
	if eff != "" {
		if slug, ok := recognizedStorefront(eff); ok {
			s := slug
			return &s, ""
		}
	}
	if media == "Physical" {
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

// splitAggregate splits the comma-separated aggregate Platforms list.
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

// parseDuration parses Darkadia "H:MM" playtime into hours. "148:00" → 148.0,
// "10:30" → 10.5. Empty, malformed, or non-positive → nil (no playtime).
func parseDuration(s string) *float64 {
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

// appendUnique appends s to xs unless already present (order-preserving).
func appendUnique(xs []string, s string) []string {
	for _, x := range xs {
		if x == s {
			return xs
		}
	}
	return append(xs, s)
}

func consolidate(rg rawGame) importmodel.Game {
	n := rg.named
	g := importmodel.Game{
		Title:      n[colName],
		PlayStatus: resolvePlayStatus(n),
		IsLoved:    n[colLoved] == "1",
		CreatedAt:  n[colAdded],
	}
	if r := parseRating(n[colRating]); r != nil {
		g.PersonalRating = r
	}
	for _, t := range splitAggregate(n[colTags]) {
		g.Tags = appendUnique(g.Tags, t)
	}
	if h := parseDuration(n[colTimePlayed]); h != nil {
		g.HoursPlayed = h
	}

	var noteLines []string
	addNote := func(line string) {
		if line == "" {
			return
		}
		for _, existing := range noteLines {
			if existing == line {
				return
			}
		}
		noteLines = append(noteLines, line)
	}

	if rev := strings.TrimSpace(n[colReview]); rev != "" {
		if subj := strings.TrimSpace(n[colReviewSubject]); subj != "" {
			addNote("Review — " + subj + "\n" + rev)
		} else {
			addNote("Review: " + rev)
		}
	}

	owned := map[string]bool{}
	ownedInferred := map[string]*string{}
	markOwned := func(s string) {
		m, ok := platformTable[s]
		if !ok {
			addNote("Owned on " + s + " (no Nexorious platform mapping).")
			return
		}
		owned[m.slug] = true
		if m.inferred != nil && ownedInferred[m.slug] == nil {
			ownedInferred[m.slug] = m.inferred
		}
	}
	for _, s := range splitAggregate(n[colPlatforms]) {
		markOwned(s)
	}
	for _, row := range rg.copies {
		if p := strings.TrimSpace(row[colCopyPlatform]); p != "" {
			markOwned(p)
		}
	}

	type key struct {
		platform   string
		storefront string
	}
	seen := map[key]int{}
	add := func(slug string, sf *string, date string) {
		sfKey := ""
		if sf != nil {
			sfKey = *sf
		}
		k := key{slug, sfKey}
		if idx, ok := seen[k]; ok {
			if date != "" && (g.Platforms[idx].AcquiredDate == "" || date < g.Platforms[idx].AcquiredDate) {
				g.Platforms[idx].AcquiredDate = date
			}
			return
		}
		seen[k] = len(g.Platforms)
		g.Platforms = append(g.Platforms, importmodel.Platform{Platform: slug, Storefront: sf, AcquiredDate: date})
	}

	slugHasCopy := map[string]bool{}
	for _, row := range rg.copies {
		ps := strings.TrimSpace(row[colCopyPlatform])
		if ps == "" {
			continue
		}
		m, ok := platformTable[ps]
		if !ok {
			continue // already noted via markOwned
		}
		slugHasCopy[m.slug] = true
		sf, note := resolveStorefront(m.inferred, effectiveSource(row), strings.TrimSpace(row[colCopyMedia]))
		addNote(note)
		add(m.slug, sf, strings.TrimSpace(row[colCopyPurchase]))
	}

	for _, row := range rg.copies {
		if cn := strings.TrimSpace(row[colCopyNotes]); cn != "" {
			addNote("Copy note: " + cn)
		}
	}

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

	verbatim := n[colNotes]
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

// resolvePlayStatus maps Darkadia's cumulative flags to a single Nexorious
// play_status by highest precedence (see docs/darkadia-import.md Part 2).
func resolvePlayStatus(row []string) string {
	switch {
	case row[colDominated] == "1":
		return "dominated"
	case row[colMastered] == "1":
		return "mastered"
	case row[colFinished] == "1":
		return "completed"
	case row[colShelved] == "1":
		return "dropped"
	case row[colPlaying] == "1":
		return "in_progress"
	case row[colPlayed] == "1":
		return "shelved"
	default: // Owned only (or nothing)
		return "not_started"
	}
}

// parseRating truncates a 0–5 half-step rating to a whole 1–5 star. Empty or 0
// means unrated (nil).
func parseRating(s string) *int32 {
	if s == "" {
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	v := int32(f) // truncation: 4.5 → 4
	if v <= 0 {
		return nil
	}
	return &v
}
