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
)

// ErrInvalidHeader signals the file is not a Darkadia export. The upload handler
// turns this into a 400 "not a Darkadia export".
var ErrInvalidHeader = errors.New("not a Darkadia export (header mismatch)")

// header is the canonical 29-column Darkadia header, by value (quoting is
// incidental in the real export — only space-containing names are quoted).
var header = []string{
	"Name", "Added", "Loved", "Owned", "Played", "Playing", "Finished",
	"Mastered", "Dominated", "Shelved", "Rating", "Copy label", "Copy Release",
	"Copy platform", "Copy media", "Copy media other", "Copy source",
	"Copy source other", "Copy purchase date", "Copy box", "Copy box condition",
	"Copy box notes", "Copy manual", "Copy manual condition", "Copy manual notes",
	"Copy complete", "Copy complete notes", "Platforms", "Notes",
}

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
)

// Game is the consolidated, Nexorious-shaped payload for one Darkadia game. It
// is marshalled verbatim into job_item.source_metadata.
type Game struct {
	Title          string     `json:"title"`
	PlayStatus     string     `json:"play_status"`
	IsLoved        bool       `json:"is_loved"`
	PersonalRating *int32     `json:"personal_rating,omitempty"`
	PersonalNotes  *string    `json:"personal_notes,omitempty"`
	CreatedAt      string     `json:"created_at,omitempty"` // "2006-01-02" or ""
	Platforms      []Platform `json:"platforms"`
}

// Platform is one consolidated (platform, storefront, acquired_date) ownership entry.
type Platform struct {
	Platform     string  `json:"platform"`                // Nexorious slug
	Storefront   *string `json:"storefront,omitempty"`    // slug or nil
	AcquiredDate string  `json:"acquired_date,omitempty"` // "2006-01-02" or ""
}

// rawGame is one game grouped from the CSV: the named row plus its copy rows.
type rawGame struct {
	named  []string
	copies [][]string // every row (named + continuations), each padded to 29 fields
}

// Parse reads a Darkadia CSV and returns one consolidated Game per title.
func Parse(raw []byte) ([]Game, error) {
	r := csv.NewReader(bytes.NewReader(raw))
	r.FieldsPerRecord = -1 // tolerate ragged rows (missing trailing columns)

	first, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	if !headerMatches(first) {
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
		row := pad(rec, len(header))
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

	games := make([]Game, 0, len(raws))
	for _, rg := range raws {
		games = append(games, consolidate(rg))
	}
	return games, nil
}

func headerMatches(got []string) bool {
	if len(got) != len(header) {
		return false
	}
	for i := range header {
		if got[i] != header[i] {
			return false
		}
	}
	return true
}

// pad returns row extended to n fields with empty strings (ragged-row tolerance).
func pad(row []string, n int) []string {
	if len(row) >= n {
		return row
	}
	out := make([]string, n)
	copy(out, row)
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
	for name, slug := range storefrontTable {
		if strings.HasPrefix(key, name+" ") {
			return slug, true
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

func consolidate(rg rawGame) Game {
	n := rg.named
	g := Game{
		Title:      n[colName],
		PlayStatus: resolvePlayStatus(n),
		IsLoved:    n[colLoved] == "1",
		CreatedAt:  n[colAdded],
	}
	if r := parseRating(n[colRating]); r != nil {
		g.PersonalRating = r
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
		g.Platforms = append(g.Platforms, Platform{Platform: slug, Storefront: sf, AcquiredDate: date})
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
