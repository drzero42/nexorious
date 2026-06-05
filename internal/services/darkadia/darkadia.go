package darkadia

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
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

// consolidate is filled in across later tasks. For now it carries the title and
// the verbatim Notes so grouping + dialect handling can be verified.
func consolidate(rg rawGame) Game {
	g := Game{Title: rg.named[colName]}
	if notes := rg.named[colNotes]; notes != "" {
		g.PersonalNotes = &notes
	}
	return g
}
