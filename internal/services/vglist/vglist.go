// Package vglist maps a vglist "Export Library" JSON file into the canonical,
// source-neutral importmodel.Game shape consumed by the shared import pipeline.
// It is the only vglist-specific code; everything after Parse (IGDB matching,
// pending_review, finalise/merge) is the generic pipeline. The full format and
// mapping spec is docs/vglist-import.md.
package vglist

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/drzero42/nexorious/internal/services/importmodel"
)

// ErrInvalidExport signals the file is not a vglist library export. It wraps the
// shared importmodel.ErrInvalidSignature so the generic upload handler can turn
// a wrong-file upload into a 400 without knowing the per-source sentinel.
var ErrInvalidExport = fmt.Errorf("not a vglist export: %w", importmodel.ErrInvalidSignature)

// namedRef is a vglist {id, name} reference (a platform or a store).
type namedRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// exportEntry is one library entry in a vglist export (a GamePurchase). Nullable
// columns are pointers; comments and replay_count are never null in vglist.
type exportEntry struct {
	Game struct {
		ID         int64  `json:"id"`
		Name       string `json:"name"`
		WikidataID *int64 `json:"wikidata_id"`
	} `json:"game"`
	HoursPlayed      *float64   `json:"hours_played"`
	CompletionStatus *string    `json:"completion_status"`
	Rating           *int32     `json:"rating"`
	StartDate        *string    `json:"start_date"`
	CompletionDate   *string    `json:"completion_date"`
	Comments         string     `json:"comments"`
	ReplayCount      int        `json:"replay_count"`
	Platforms        []namedRef `json:"platforms"`
	Stores           []namedRef `json:"stores"`
}

// Parse reads a vglist JSON export and returns one Game per library entry.
func Parse(raw []byte) ([]importmodel.Game, error) {
	if trimmed := bytes.TrimSpace(raw); len(trimmed) == 0 || trimmed[0] != '[' {
		return nil, ErrInvalidExport
	}
	var entries []exportEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, ErrInvalidExport
	}
	// Signature: a real export is an array of entries each with a game name.
	// An empty array is a valid (empty) library. A non-empty array where no
	// entry has a game name is the wrong file.
	if len(entries) > 0 {
		named := false
		for _, e := range entries {
			if strings.TrimSpace(e.Game.Name) != "" {
				named = true
				break
			}
		}
		if !named {
			return nil, ErrInvalidExport
		}
	}

	games := make([]importmodel.Game, 0, len(entries))
	for _, e := range entries {
		if strings.TrimSpace(e.Game.Name) == "" {
			continue // skip nameless entries defensively (vglist never emits one)
		}
		games = append(games, mapEntry(e))
	}
	return games, nil
}

// mapEntry maps one vglist entry to a canonical Game.
func mapEntry(e exportEntry) importmodel.Game {
	g := importmodel.Game{
		Title:          strings.TrimSpace(e.Game.Name),
		PlayStatus:     mapCompletionStatus(deref(e.CompletionStatus)),
		PersonalRating: mapRating(e.Rating),
	}
	if e.HoursPlayed != nil && *e.HoursPlayed > 0 {
		h := *e.HoursPlayed
		g.HoursPlayed = &h
	}

	notes := &noteBuilder{}
	g.Platforms = consolidate(e, notes)

	if d := strings.TrimSpace(deref(e.StartDate)); d != "" {
		notes.add("Started: " + d + ".")
	}
	if d := strings.TrimSpace(deref(e.CompletionDate)); d != "" {
		notes.add("Completed: " + d + ".")
	}
	if e.ReplayCount > 0 {
		notes.add(fmt.Sprintf("Replayed %d time(s).", e.ReplayCount))
	}

	g.PersonalNotes = notes.finalize(e.Comments)
	return g
}

// mapCompletionStatus maps a vglist completion_status to a Nexorious play_status.
// Unknown or empty values fall back to not_started. See docs/vglist-import.md.
func mapCompletionStatus(s string) string {
	switch strings.TrimSpace(s) {
	case "in_progress":
		return "in_progress"
	case "paused":
		return "shelved"
	case "dropped":
		return "dropped"
	case "completed":
		return "completed"
	case "fully_completed":
		return "mastered"
	case "unplayed", "not_applicable", "":
		return "not_started"
	default:
		return "not_started"
	}
}

// mapRating collapses vglist's integer 0–100 rating into whole 1–5 stars:
// round(rating/20), clamped to a minimum of 1 star for any positive rating.
// 0 or nil means unrated.
func mapRating(r *int32) *int32 {
	if r == nil || *r <= 0 {
		return nil
	}
	v := int32(math.Round(float64(*r) / 20.0))
	v = max(1, min(v, 5))
	return &v
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
