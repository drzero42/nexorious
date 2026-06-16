package csvmap

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/drzero42/nexorious/internal/services/importmodel"
)

// normKey lowercases and trims a string for case-insensitive matching.
func normKey(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// buildIndex maps each normalized header name to its column position (first wins).
func buildIndex(header []string) map[string]int {
	m := make(map[string]int, len(header))
	for i, name := range header {
		k := normKey(name)
		if _, ok := m[k]; !ok {
			m[k] = i
		}
	}
	return m
}

// cell returns the trimmed value of colName in rec, or "" if the column is
// unconfigured, absent from the header, or past the end of a ragged row.
func cell(rec []string, idx map[string]int, colName string) string {
	if colName == "" {
		return ""
	}
	i, ok := idx[normKey(colName)]
	if !ok || i >= len(rec) {
		return ""
	}
	return strings.TrimSpace(rec[i])
}

// decodeKeys returns the values a column yields under format f. Scalar: the cell
// as a single value (nil if blank). JSON-keys: the keys of a JSON object (nil for
// "", "{}", a non-object, or unparseable JSON). Object key order is undefined;
// callers that must pick one value apply an explicit precedence.
func decodeKeys(cell string, f ColumnFormat) []string {
	if cell == "" {
		return nil
	}
	if f != FormatJSONKeys {
		return []string{cell}
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(cell), &obj); err != nil {
		return nil
	}
	if len(obj) == 0 {
		return nil
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	return keys
}

// MatchesSignature reports whether every name in cfg.Signature is present in
// headers (compared normalized). A nil/empty Signature always matches — the
// generic mapping path accepts any non-empty CSV.
func MatchesSignature(headers []string, cfg Config) bool {
	if len(cfg.Signature) == 0 {
		return true
	}
	present := make(map[string]bool, len(headers))
	for _, h := range headers {
		present[normKey(h)] = true
	}
	for _, name := range cfg.Signature {
		if !present[normKey(name)] {
			return false
		}
	}
	return true
}

// Parse maps a CSV export into canonical games per cfg. On a wrong-shape file
// (failed Signature) it returns an error wrapping importmodel.ErrInvalidSignature.
func Parse(raw []byte, cfg Config) ([]importmodel.Game, error) {
	if err := validate(cfg); err != nil {
		return nil, err
	}
	records, err := ReadRecords(raw)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, io.EOF
	}

	header := records[0]
	if !MatchesSignature(header, cfg) {
		return nil, fmt.Errorf("csv does not match the expected format: %w", importmodel.ErrInvalidSignature)
	}
	idx := buildIndex(header)

	return buildGames(records[1:], idx, cfg), nil
}

// buildGames dispatches between one-row and merge-by-title grouping.
func buildGames(rows [][]string, idx map[string]int, cfg Config) []importmodel.Game {
	if cfg.Grouping.MergeByTitle {
		return buildMerged(rows, idx, cfg)
	}
	games := make([]importmodel.Game, 0, len(rows))
	for _, rec := range rows {
		if g, ok := extractGame(rec, idx, cfg); ok {
			games = append(games, g)
		}
	}
	return games
}

// platKey is the (platform, storefront) dedupe key for an ownership entry.
func platKey(p importmodel.Platform) string {
	sf := ""
	if p.Storefront != nil {
		sf = *p.Storefront
	}
	return p.Platform + "\x00" + sf
}

// buildMerged collapses rows sharing a normalized title into one game: the first
// row establishes all scalar fields; every row contributes platform entries,
// union-deduped on (platform, storefront). Output order is first-seen order.
func buildMerged(rows [][]string, idx map[string]int, cfg Config) []importmodel.Game {
	type entry struct {
		game importmodel.Game
		seen map[string]bool
	}
	var order []string
	byTitle := map[string]*entry{}
	for _, rec := range rows {
		g, ok := extractGame(rec, idx, cfg)
		if !ok {
			continue
		}
		key := normKey(g.Title)
		e, exists := byTitle[key]
		if !exists {
			e = &entry{game: g, seen: map[string]bool{}}
			for _, p := range g.Platforms {
				e.seen[platKey(p)] = true
			}
			byTitle[key] = e
			order = append(order, key)
			continue
		}
		for _, p := range g.Platforms {
			k := platKey(p)
			if e.seen[k] {
				continue
			}
			e.seen[k] = true
			e.game.Platforms = append(e.game.Platforms, p)
		}
	}
	out := make([]importmodel.Game, 0, len(order))
	for _, key := range order {
		out = append(out, byTitle[key].game)
	}
	return out
}

// extractStatus resolves the shelf-derived play_status and the wishlist flag from
// the status column (scalar or JSON-keys). A value mapped to WishlistStatus sets
// wishlisted; the remaining mapped values are play_status candidates, resolved by
// Precedence (first listed present wins), else the single candidate (scalar
// case), else Default.
func extractStatus(rec []string, idx map[string]int, cfg Config) (status string, wishlisted bool) {
	if cfg.Status.Column == nil {
		return "not_started", false
	}
	sc := cfg.Status.Column
	def := sc.Default
	if def == "" {
		def = "not_started"
	}
	present := map[string]string{} // normalized source value -> mapped play_status
	for _, v := range decodeKeys(cell(rec, idx, sc.Column), sc.Format) {
		nv := normKey(v)
		mapped, ok := sc.ValueMap[nv]
		if !ok {
			continue
		}
		if mapped == WishlistStatus {
			wishlisted = true
			continue
		}
		present[nv] = mapped
	}
	for _, p := range sc.Precedence {
		if s, ok := present[normKey(p)]; ok {
			return s, wishlisted
		}
	}
	if len(sc.Precedence) == 0 {
		for _, s := range present { // scalar case: at most one entry
			return s, wishlisted
		}
	}
	return def, wishlisted
}

// extractRating normalizes a raw rating to whole 1-5 stars per cfg.Rating.
// Returns nil when ratings are disabled, the cell is empty/invalid, or the
// result is <= 0.
func extractRating(raw string, cfg Config) *int32 {
	if cfg.Rating == nil || raw == "" {
		return nil
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil
	}
	stars := f / float64(cfg.Rating.Scale) * 5.0
	var v int32
	if cfg.Rating.Truncate {
		v = int32(math.Trunc(stars))
	} else {
		v = int32(math.Round(stars))
	}
	if v <= 0 {
		return nil
	}
	if v > 5 {
		v = 5
	}
	return &v
}

var defaultTruthy = []string{"1", "true", "yes"}

// extractLoved reports whether the loved cell matches a truthy value.
func extractLoved(rec []string, idx map[string]int, cfg Config) bool {
	if cfg.Columns.Loved == "" {
		return false
	}
	v := normKey(cell(rec, idx, cfg.Columns.Loved))
	if v == "" {
		return false
	}
	truthy := cfg.TruthyValues
	if truthy == nil {
		truthy = defaultTruthy
	}
	for _, t := range truthy {
		if normKey(t) == v {
			return true
		}
	}
	return false
}

// extractTags splits, trims, drops empties, and order-preserving dedupes the tag cell.
func extractTags(rec []string, idx map[string]int, cfg Config) []string {
	raw := cell(rec, idx, cfg.Columns.Tags)
	if raw == "" {
		return nil
	}
	sep := cfg.TagSeparator
	if sep == "" {
		sep = ","
	}
	var out []string
	seen := map[string]bool{}
	for _, p := range strings.Split(raw, sep) {
		tag := strings.TrimSpace(p)
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		out = append(out, tag)
	}
	return out
}

// extractDate normalizes a date cell to "2006-01-02". With no DateLayout (or the
// ISO layout) it accepts already-ISO input and rejects anything else. Invalid
// input yields "".
func extractDate(raw string, cfg Config) string {
	if raw == "" {
		return ""
	}
	layout := cfg.DateLayout
	if layout == "" {
		layout = "2006-01-02"
	}
	tm, err := time.Parse(layout, raw)
	if err != nil {
		return ""
	}
	return tm.Format("2006-01-02")
}

// extractHours parses decimal hours. h:mm is rejected at validation, so only the
// decimal branch is needed here.
func extractHours(rec []string, idx map[string]int, cfg Config) *float64 {
	if cfg.Duration == nil {
		return nil
	}
	raw := cell(rec, idx, cfg.Columns.HoursPlayed)
	if raw == "" {
		return nil
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil || f <= 0 {
		return nil
	}
	return &f
}

// extractIGDBID parses the configured IGDB-id cell into a positive int32. A
// missing/unconfigured column, blank, non-numeric, zero, negative, or
// out-of-int32-range value yields nil — such a row falls back to title matching
// downstream.
func extractIGDBID(rec []string, idx map[string]int, cfg Config) *int32 {
	raw := cell(rec, idx, cfg.Columns.IGDBID)
	if raw == "" {
		return nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 || n > math.MaxInt32 {
		return nil
	}
	v := int32(n) //nolint:gosec // guarded above: 0 < n <= MaxInt32
	return &v
}

// extractPlatforms builds the simple-variant ownership entry (at most one) from
// the configured platform/storefront/acquired-date columns. An empty platform
// cell or no PlatformSimple config yields no entries. Map miss = passthrough.
func extractPlatforms(rec []string, idx map[string]int, cfg Config) []importmodel.Platform {
	ps := cfg.Platform.Simple
	if ps == nil {
		return nil
	}
	pv := cell(rec, idx, ps.PlatformColumn)
	if pv == "" {
		return nil
	}
	slug := pv
	if ps.PlatformMap != nil {
		if mapped, ok := ps.PlatformMap[normKey(pv)]; ok {
			slug = mapped
		}
	}
	var sf *string
	if sv := cell(rec, idx, ps.StorefrontColumn); sv != "" {
		s := sv
		if ps.StorefrontMap != nil {
			if mapped, ok := ps.StorefrontMap[normKey(sv)]; ok {
				s = mapped
			}
		}
		sf = &s
	}
	date := extractDate(cell(rec, idx, ps.AcquiredDateColumn), cfg)
	return []importmodel.Platform{{Platform: slug, Storefront: sf, AcquiredDate: date}}
}

// extractGame builds one Game from a row, or (zero, false) if the title is empty.
func extractGame(rec []string, idx map[string]int, cfg Config) (importmodel.Game, bool) {
	title := cell(rec, idx, cfg.Columns.Title)
	if title == "" {
		return importmodel.Game{}, false
	}
	status, _ := extractStatus(rec, idx, cfg)
	g := importmodel.Game{
		Title:      title,
		PlayStatus: status,
	}
	g.IGDBID = extractIGDBID(rec, idx, cfg)
	if r := extractRating(cell(rec, idx, cfg.Columns.Rating), cfg); r != nil {
		g.PersonalRating = r
	}
	g.IsLoved = extractLoved(rec, idx, cfg)
	g.CreatedAt = extractDate(cell(rec, idx, cfg.Columns.CreatedAt), cfg)
	g.Tags = extractTags(rec, idx, cfg)
	if h := extractHours(rec, idx, cfg); h != nil {
		g.HoursPlayed = h
	}
	if n := cell(rec, idx, cfg.Notes.Column); n != "" {
		g.PersonalNotes = &n
	}
	g.Platforms = extractPlatforms(rec, idx, cfg)
	return g, true
}
