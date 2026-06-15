package csvmap

import (
	"strings"
	"unicode"
)

// SuggestedMapping is a best-effort, frontend-shaped guess of how a CSV's
// headers map to canonical fields. Its JSON shape is byte-for-byte the
// frontend CsvMapping, so the import dialog can drop it straight into its form
// state. It only seeds the dialog; the submitted mapping remains authoritative.
type SuggestedMapping struct {
	Columns struct {
		Title        string `json:"title"`
		Platform     string `json:"platform"`
		Storefront   string `json:"storefront"`
		Rating       string `json:"rating"`
		Notes        string `json:"notes"`
		AcquiredDate string `json:"acquired_date"`
		HoursPlayed  string `json:"hours_played"`
		Tags         string `json:"tags"`
		Loved        string `json:"loved"`
	} `json:"columns"`
	Status struct {
		Column   string            `json:"column"`
		ValueMap map[string]string `json:"value_map"`
	} `json:"status"`
	RatingScale  int  `json:"rating_scale"`
	MergeByTitle bool `json:"merge_by_title"`
}

// normalizeHeader lowercases and strips every non-alphanumeric rune, so
// "Date Bought" -> "datebought" and "Play-Status" -> "playstatus".
func normalizeHeader(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// fieldAliases lists, per canonical field, the normalized header aliases that
// identify it. Order is priority order for contended headers (title first).
// The matcher does an exact-normalized pass over all fields, then a
// substring-contains pass for the still-unassigned ones.
var fieldAliases = []struct {
	set     func(*SuggestedMapping, string)
	aliases []string
}{
	{func(m *SuggestedMapping, v string) { m.Columns.Title = v }, []string{"name", "title", "game", "gamename", "gametitle"}},
	{func(m *SuggestedMapping, v string) { m.Status.Column = v }, []string{"status", "playstatus", "state", "progress", "completionstatus"}},
	{func(m *SuggestedMapping, v string) { m.Columns.Platform = v }, []string{"platform", "system", "console", "device"}},
	{func(m *SuggestedMapping, v string) { m.Columns.Storefront = v }, []string{"storefront", "store", "source", "launcher", "service"}},
	{func(m *SuggestedMapping, v string) { m.Columns.HoursPlayed = v }, []string{"hoursplayed", "playtimehours", "playtime", "timeplayed", "hours", "hrs"}},
	{func(m *SuggestedMapping, v string) { m.Columns.AcquiredDate = v }, []string{"acquireddate", "dateacquired", "dateadded", "purchasedate", "acquired", "purchased", "bought", "added"}},
	{func(m *SuggestedMapping, v string) { m.Columns.Rating = v }, []string{"rating", "score", "stars"}},
	{func(m *SuggestedMapping, v string) { m.Columns.Tags = v }, []string{"tags", "tag", "labels", "label", "categories", "genres"}},
	{func(m *SuggestedMapping, v string) { m.Columns.Notes = v }, []string{"notes", "note", "review", "comment", "comments"}},
	{func(m *SuggestedMapping, v string) { m.Columns.Loved = v }, []string{"loved", "favorite", "favourite", "fav", "liked", "starred"}},
}

func matchesAlias(norm string, aliases []string, contains bool) bool {
	for _, a := range aliases {
		if norm == a {
			return true
		}
		if contains && strings.Contains(norm, a) {
			return true
		}
	}
	return false
}

// GuessColumns matches each canonical field to at most one header, header by
// header in file order. Exact-normalized matches are taken first (all fields),
// then a substring-contains pass fills the rest. A header claimed by one field
// is never reused. RatingScale defaults to 5 and MergeByTitle to true; the
// caller refines RatingScale and the status ValueMap from the data.
func GuessColumns(header []string) SuggestedMapping {
	var m SuggestedMapping
	m.MergeByTitle = true
	m.RatingScale = 5
	m.Status.ValueMap = map[string]string{}

	norms := make([]string, len(header))
	for i, h := range header {
		norms[i] = normalizeHeader(h)
	}
	claimed := make([]bool, len(header))
	assigned := make([]bool, len(fieldAliases))

	pass := func(contains bool) {
		for fi := range fieldAliases {
			if assigned[fi] {
				continue
			}
			for hi := range header {
				if claimed[hi] || norms[hi] == "" {
					continue
				}
				if matchesAlias(norms[hi], fieldAliases[fi].aliases, contains) {
					fieldAliases[fi].set(&m, header[hi])
					claimed[hi] = true
					assigned[fi] = true
					break
				}
			}
		}
	}
	pass(false)
	pass(true)

	return m
}

// GuessRatingScale maps an observed maximum rating value to a supported scale.
// A non-positive max (no numeric values seen) falls back to 5.
func GuessRatingScale(max float64) int {
	switch {
	case max <= 5:
		return 5
	case max <= 10:
		return 10
	default:
		return 100
	}
}

// statusSynonyms maps a normalized source status value to a play_status. Values
// not present here fall back to "not_started".
var statusSynonyms = map[string]string{
	"completed": "completed", "complete": "completed", "beaten": "completed",
	"finished": "completed", "done": "completed", "100": "completed", "100percent": "completed",
	"inprogress": "in_progress", "playing": "in_progress", "started": "in_progress",
	"current": "in_progress", "ongoing": "in_progress",
	"notstarted": "not_started", "backlog": "not_started", "unplayed": "not_started",
	"neverplayed": "not_started", "tobeplayed": "not_started", "tbp": "not_started", "wishlist": "not_started", // wishlisted = unplayed here; is_wishlisted flag handled separately
	"dropped": "dropped", "abandoned": "dropped", "quit": "dropped", "gaveup": "dropped",
	"shelved": "shelved", "onhold": "shelved", "hold": "shelved", "paused": "shelved", "suspended": "shelved",
	"mastered": "mastered", "platinum": "mastered", "perfected": "mastered",
	"dominated": "dominated",
	"replay":    "replay", "replaying": "replay", "revisiting": "replay",
}

// GuessStatusValueMap maps each distinct source value to a guessed play_status,
// keyed by the raw source value (matching the dialog's value-row keys). Values
// with no synonym map to "not_started".
func GuessStatusValueMap(distinct []string) map[string]string {
	out := make(map[string]string, len(distinct))
	for _, v := range distinct {
		if ps, ok := statusSynonyms[normalizeHeader(v)]; ok {
			out[v] = ps
		} else {
			out[v] = "not_started"
		}
	}
	return out
}
