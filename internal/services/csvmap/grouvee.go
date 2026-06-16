package csvmap

// Grouvee returns the preset Config for a Grouvee CSV export. Grouvee exports an
// IGDB id per row (direct match, #1022). Its shelves and platforms columns are
// JSON objects whose keys carry the data, and its dates column is a JSON play-log
// (seconds_played -> hours_played, level_of_completion -> play_status override).
// See docs/grouvee-import.md.
func Grouvee() Config {
	return Config{
		Signature: []string{
			"shelves", "date_added_to_collection", "review_platform", "giantbomb_id", "igdb_id",
		},
		Columns: ColumnMap{
			Title:     "name",
			IGDBID:    "igdb_id",
			Rating:    "rating",
			CreatedAt: "date_added_to_collection",
		},
		Status: StatusConfig{
			Column: &StatusColumn{
				Column: "shelves",
				Format: FormatJSONKeys,
				ValueMap: map[string]string{
					"playing":   "in_progress",
					"played":    "completed",
					"backlog":   "not_started",
					"wish list": WishlistStatus,
				},
				Precedence: []string{"playing", "played", "backlog"},
				Default:    "not_started",
			},
		},
		PlayLog: &PlayLogConfig{
			Column:          "dates",
			SecondsField:    "seconds_played",
			CompletionField: "level_of_completion",
			CompletionMap: map[string]string{
				"main story":          "completed",
				"main story + extras": "mastered",
				"100% completion":     "dominated",
			},
		},
		Platform: PlatformConfig{
			Simple: &PlatformSimple{
				PlatformColumn: "platforms",
				PlatformFormat: FormatJSONKeys,
				PlatformMap: map[string]string{
					"pc (microsoft windows)": "pc-windows",
					"playstation 5":          "playstation-5",
					"playstation 4":          "playstation-4",
					"playstation 3":          "playstation-3",
					"playstation 2":          "playstation-2",
					"playstation":            "playstation",
					"playstation vita":       "playstation-vita",
					"xbox series x|s":        "xbox-series",
					"xbox one":               "xbox-one",
					"xbox 360":               "xbox-360",
					"xbox":                   "xbox",
					"nintendo switch":        "nintendo-switch",
					"nintendo switch 2":      "nintendo-switch-2",
					"wii":                    "nintendo-wii",
					"wii u":                  "nintendo-wii-u",
					"nintendo 64":            "nintendo-64",
					"nintendo gamecube":      "nintendo-gamecube",
					"nintendo ds":            "nintendo-ds",
					"nintendo 3ds":           "nintendo-3ds",
					"mac":                    "mac",
					"linux":                  "pc-linux",
					"ios":                    "ios",
					"android":                "android",
				},
			},
		},
		Notes: NotesConfig{
			TitleColumn: "review_title",
			Column:      "review",
		},
		Rating:   &RatingConfig{Scale: 5, Truncate: false},
		Grouping: GroupingConfig{MergeByTitle: false},
	}
}
