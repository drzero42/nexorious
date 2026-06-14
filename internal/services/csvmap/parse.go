package csvmap

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

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

// validate checks the config before any data is read. Advanced (Darkadia-era)
// slots are rejected with a descriptive error that is NOT ErrInvalidSignature.
func validate(cfg Config) error {
	if strings.TrimSpace(cfg.Columns.Title) == "" {
		return errors.New("csvmap: Columns.Title is required")
	}
	if cfg.Status.Column != nil && cfg.Status.Flags != nil {
		return errors.New("csvmap: Status.Column and Status.Flags are mutually exclusive")
	}
	if cfg.Platform.Simple != nil && cfg.Platform.Tables != nil {
		return errors.New("csvmap: Platform.Simple and Platform.Tables are mutually exclusive")
	}
	if cfg.Rating != nil {
		switch cfg.Rating.Scale {
		case 5, 10, 100:
		default:
			return fmt.Errorf("csvmap: Rating.Scale must be 5, 10, or 100, got %d", cfg.Rating.Scale)
		}
	}
	if cfg.Duration != nil {
		switch normKey(cfg.Duration.Format) {
		case "decimal":
		case "h:mm":
			return notImplemented(`Duration.Format "h:mm"`)
		default:
			return fmt.Errorf("csvmap: Duration.Format must be %q or %q, got %q", "decimal", "h:mm", cfg.Duration.Format)
		}
	}
	if cfg.Status.Flags != nil {
		return notImplemented("Status.Flags")
	}
	if cfg.Platform.Tables != nil {
		return notImplemented("Platform.Tables")
	}
	if cfg.Notes.Assembly != nil {
		return notImplemented("Notes.Assembly")
	}
	if cfg.Grouping.CopyRows != nil {
		return notImplemented("Grouping.CopyRows")
	}
	return nil
}

// notImplemented is returned for an advanced Config slot whose behaviour lands in #1016.
func notImplemented(feature string) error {
	return fmt.Errorf("csvmap: %s is not implemented yet (advanced feature, see #1016)", feature)
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
	r := csv.NewReader(bytes.NewReader(raw))
	r.FieldsPerRecord = -1 // tolerate ragged rows (missing trailing columns)

	header, err := r.Read()
	if err != nil {
		return nil, err
	}
	if !MatchesSignature(header, cfg) {
		return nil, fmt.Errorf("csv does not match the expected format: %w", importmodel.ErrInvalidSignature)
	}
	idx := buildIndex(header)

	var rows [][]string
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		rows = append(rows, rec)
	}

	games := make([]importmodel.Game, 0, len(rows))
	for _, rec := range rows {
		g, ok := extractGame(rec, idx, cfg)
		if ok {
			games = append(games, g)
		}
	}
	return games, nil
}

// extractStatus resolves play_status from the simple status column, or
// "not_started" when no status column is configured.
func extractStatus(rec []string, idx map[string]int, cfg Config) string {
	if cfg.Status.Column == nil {
		return "not_started"
	}
	sc := cfg.Status.Column
	def := sc.Default
	if def == "" {
		def = "not_started"
	}
	v := normKey(cell(rec, idx, sc.Column))
	if v == "" {
		return def
	}
	if status, ok := sc.ValueMap[v]; ok {
		return status
	}
	return def
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

// extractGame builds one Game from a row, or (zero, false) if the title is empty.
func extractGame(rec []string, idx map[string]int, cfg Config) (importmodel.Game, bool) {
	title := cell(rec, idx, cfg.Columns.Title)
	if title == "" {
		return importmodel.Game{}, false
	}
	g := importmodel.Game{
		Title:      title,
		PlayStatus: extractStatus(rec, idx, cfg),
	}
	if r := extractRating(cell(rec, idx, cfg.Columns.Rating), cfg); r != nil {
		g.PersonalRating = r
	}
	return g, true
}
