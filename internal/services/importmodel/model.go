// Package importmodel defines the canonical, source-neutral game shape that
// every import mapper produces and the import pipeline consumes. It is a leaf
// package (no internal dependencies) so mappers and the upload handler can both
// reference it without an import cycle.
package importmodel

import "errors"

// ErrInvalidSignature is the shared sentinel a mapper returns (wrapped) when a
// file is the wrong shape for that source. The generic upload handler turns
// errors.Is(err, ErrInvalidSignature) into a 400 "not a <source> export".
var ErrInvalidSignature = errors.New("file does not match the expected source format")

// Game is the consolidated, Nexorious-shaped payload for one imported game. It
// is marshalled verbatim into job_item.source_metadata.
type Game struct {
	Title          string     `json:"title"`
	IGDBID         *int32     `json:"igdb_id,omitempty"` // when set (>0), import hydrates directly and skips title matching
	PlayStatus     string     `json:"play_status"`
	IsLoved        bool       `json:"is_loved"`
	PersonalRating *int32     `json:"personal_rating,omitempty"`
	PersonalNotes  *string    `json:"personal_notes,omitempty"`
	CreatedAt      string     `json:"created_at,omitempty"` // "2006-01-02" or ""
	Platforms      []Platform `json:"platforms"`
	Tags           []string   `json:"tags,omitempty"`
	HoursPlayed    *float64   `json:"hours_played,omitempty"`
}

// Platform is one consolidated (platform, storefront, acquired_date) ownership entry.
type Platform struct {
	Platform     string  `json:"platform"`                // Nexorious slug
	Storefront   *string `json:"storefront,omitempty"`    // slug or nil
	AcquiredDate string  `json:"acquired_date,omitempty"` // "2006-01-02" or ""
}
