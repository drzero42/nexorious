package csvmap

import (
	"time"

	"github.com/drzero42/nexorious/internal/enum"
)

// NexoriousCSV returns the preset Config for Nexorious's own CSV export
// (internal/worker/tasks/export.go). Every row carries an igdb_id (the
// IGDB-keyed games.id), so matching is a direct id match (#1022) — no title
// matching, nothing in pending_review. play_status is exported verbatim as one
// of the eight canonical values, so the status ValueMap is an identity map over
// them. platforms is a semicolon-joined list of canonical platform slugs, read
// via the PlatformSeparator extension. The updated_at column has no model home
// and is dropped. See docs/nexorious-csv-import.md.
func NexoriousCSV() Config {
	return Config{
		Signature: []string{
			"play_status", "personal_rating", "is_loved", "hours_played", "personal_notes",
		},
		Columns: ColumnMap{
			Title:       "title",
			IGDBID:      "igdb_id",
			Rating:      "personal_rating",
			HoursPlayed: "hours_played",
			Tags:        "tags",
			Loved:       "is_loved",
			CreatedAt:   "created_at",
		},
		Status: StatusConfig{
			Column: &StatusColumn{
				Column:   "play_status",
				ValueMap: playStatusIdentityMap(),
				Default:  enum.PlayStatusNotStarted,
			},
		},
		Platform: PlatformConfig{
			Simple: &PlatformSimple{
				PlatformColumn:    "platforms",
				PlatformSeparator: ";",
			},
		},
		Notes:        NotesConfig{Column: "personal_notes"},
		Rating:       &RatingConfig{Scale: 5, Truncate: false},
		Duration:     &DurationConfig{Format: "decimal"},
		TagSeparator: ";",
		DateLayout:   time.RFC3339,
		Grouping:     GroupingConfig{MergeByTitle: false},
	}
}

// playStatusIdentityMap maps every play status to itself: Nexorious's own
// export writes play_status verbatim.
func playStatusIdentityMap() map[string]enum.PlayStatus {
	all := enum.AllPlayStatuses()
	m := make(map[string]enum.PlayStatus, len(all))
	for _, s := range all {
		m[s] = enum.PlayStatus(s)
	}
	return m
}
