package csvmap

// Completionator returns the preset Config for a Completionator CSV export.
// Completionator quote-wraps every field (with unescaped embedded quotes) and
// exports Windows-1252; ReadRecords handles both. Play-status comes from the
// single Progress Status column (the Now Playing / Backlogged flags are not
// honoured — that would need the advanced StatusFlags, #1016). Ratings are a
// 1-10 scale. See docs/completionator-import.md.
func Completionator() Config {
	return Config{
		Signature: []string{
			"Now Playing", "Backlogged", "Ownership Status",
			"Progress Status", "Added On",
		},
		Columns: ColumnMap{
			Title:     "Name",
			Rating:    "Rating",
			CreatedAt: "Added On",
			Tags:      "Tags",
		},
		Status: StatusConfig{
			Column: &StatusColumn{
				Column: "Progress Status",
				ValueMap: map[string]string{
					"finished":   "completed",
					"incomplete": "not_started",
				},
				Default: "not_started",
			},
		},
		Platform: PlatformConfig{
			Simple: &PlatformSimple{
				PlatformColumn:     "Platform",
				StorefrontColumn:   "Format",
				AcquiredDateColumn: "Acquisition Date",
				PlatformMap: map[string]string{
					"pc / windows":  "pc-windows",
					"playstation 5": "playstation-5",
				},
				StorefrontMap: map[string]string{
					"digital (steam)":       "steam",
					"digital (gog)":         "gog",
					"physical (unassigned)": "physical",
					"physical (new)":        "physical",
					"physical (used)":       "physical",
				},
			},
		},
		Rating:     &RatingConfig{Scale: 10, Truncate: false},
		DateLayout: "1/2/2006",
		Grouping:   GroupingConfig{MergeByTitle: true},
	}
}
