package csvmap

import "time"

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
				Column: "play_status",
				ValueMap: map[string]string{
					"not_started": "not_started",
					"in_progress": "in_progress",
					"completed":   "completed",
					"mastered":    "mastered",
					"dominated":   "dominated",
					"shelved":     "shelved",
					"dropped":     "dropped",
					"replay":      "replay",
				},
				Default: "not_started",
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
