package csvmap

import (
	"bytes"
	"encoding/csv"
	"io"
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

// Parse maps a CSV export into canonical games per cfg. On a wrong-shape file
// (failed Signature) it returns an error wrapping importmodel.ErrInvalidSignature.
func Parse(raw []byte, cfg Config) ([]importmodel.Game, error) {
	r := csv.NewReader(bytes.NewReader(raw))
	r.FieldsPerRecord = -1 // tolerate ragged rows (missing trailing columns)

	header, err := r.Read()
	if err != nil {
		return nil, err
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

// extractGame builds one Game from a row, or (zero, false) if the title is empty.
func extractGame(rec []string, idx map[string]int, cfg Config) (importmodel.Game, bool) {
	title := cell(rec, idx, cfg.Columns.Title)
	if title == "" {
		return importmodel.Game{}, false
	}
	return importmodel.Game{Title: title}, true
}
