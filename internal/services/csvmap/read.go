package csvmap

import (
	"bytes"
	"encoding/csv"
	"errors"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

// ReadRecords parses CSV bytes tolerantly and returns every record (header
// included). It (1) transcodes Windows-1252 input to UTF-8 when the bytes are
// not already valid UTF-8, (2) parses strictly with encoding/csv, and (3) only
// when strict parsing fails on a quote error AND the file is uniformly
// quote-wrapped, falls back to a de-quote split. Otherwise it returns the
// strict error rather than risk the silent corruption LazyQuotes would cause.
func ReadRecords(raw []byte) ([][]string, error) {
	text := toUTF8(raw)

	r := csv.NewReader(bytes.NewReader(text))
	r.FieldsPerRecord = -1 // tolerate ragged rows (missing trailing columns)
	records, err := r.ReadAll()
	if err == nil {
		return records, nil
	}
	if !isQuoteError(err) {
		return nil, err
	}
	if fallback, ok := dequoteSplit(text); ok {
		return fallback, nil
	}
	return nil, err
}

// toUTF8 returns raw unchanged when it is already valid UTF-8 (including pure
// ASCII); otherwise it decodes the bytes as Windows-1252 (a Latin-1 superset
// covering the smart-quote/dash range that real exports use).
func toUTF8(raw []byte) []byte {
	if utf8.Valid(raw) {
		return raw
	}
	decoded, err := charmap.Windows1252.NewDecoder().Bytes(raw)
	if err != nil {
		return raw // Windows-1252 maps every byte; this path is unreachable in practice
	}
	return decoded
}

// isQuoteError reports whether err is a csv parse error caused by malformed quoting.
func isQuoteError(err error) bool {
	var pe *csv.ParseError
	if !errors.As(err, &pe) {
		return false
	}
	return errors.Is(pe.Err, csv.ErrQuote) || errors.Is(pe.Err, csv.ErrBareQuote)
}

// dequoteSplit recovers a fully quote-wrapped file (every non-empty line
// matching ^"…"$) by stripping the outer quotes and splitting each line on the
// literal separator `","`. It returns ok=false unless every non-empty line is
// quote-wrapped AND yields the same field count — so a partially-quoted file, a
// multi-line quoted field, or a literal `","` inside a field falls through to
// the strict error instead of misaligning columns.
func dequoteSplit(text []byte) ([][]string, bool) {
	lines := strings.Split(string(text), "\n")
	records := make([][]string, 0, len(lines))
	want := -1
	for _, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		if line == "" {
			continue
		}
		if len(line) < 2 || line[0] != '"' || line[len(line)-1] != '"' {
			return nil, false
		}
		fields := strings.Split(line[1:len(line)-1], `","`)
		if want == -1 {
			want = len(fields)
		} else if len(fields) != want {
			return nil, false
		}
		records = append(records, fields)
	}
	if len(records) == 0 {
		return nil, false
	}
	return records, true
}
