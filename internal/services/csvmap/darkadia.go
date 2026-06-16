package csvmap

// Darkadia returns the preset Config for a Darkadia CSV export (the now-defunct
// game tracker). Darkadia is a one-row-per-copy format: a game's identity and
// game-level attributes live on its first (named) row; blank-Name rows continue
// the previous game as additional copies. Status comes from cumulative
// achievement flags (Dominated > Mastered > Finished > Shelved > Playing >
// Played); ownership is consolidated from the aggregate "Platforms" column and
// per-copy "Copy platform" rows, with storefront resolved from the copy's source
// / media. See docs/darkadia-import.md. This Config re-expresses the bespoke
// internal/services/darkadia mapper as engine data (#1016) — the column-to-game
// mapping is reproduced byte-for-byte; the import is reached via the CSV card's
// Format dropdown and recorded with source "csv".
func Darkadia() Config {
	psn := "playstation-store"
	xboxStore := "microsoft-store"
	return Config{
		// The 29 required columns of a Darkadia export (0–28); optional
		// feature-toggle columns (Tags, Time played, Review…) are read when present.
		Signature: []string{
			"Name", "Added", "Loved", "Owned", "Played", "Playing", "Finished",
			"Mastered", "Dominated", "Shelved", "Rating", "Copy label", "Copy Release",
			"Copy platform", "Copy media", "Copy media other", "Copy source",
			"Copy source other", "Copy purchase date", "Copy box", "Copy box condition",
			"Copy box notes", "Copy manual", "Copy manual condition", "Copy manual notes",
			"Copy complete", "Copy complete notes", "Platforms", "Notes",
		},
		Columns: ColumnMap{
			Title:       "Name",
			Rating:      "Rating",
			CreatedAt:   "Added",
			HoursPlayed: "Time played",
			Tags:        "Tags",
			Loved:       "Loved",
		},
		TruthyValues: []string{"1"},
		Status: StatusConfig{
			Flags: &StatusFlags{
				Rules: []FlagRule{
					{Column: "Dominated", Truthy: []string{"1"}, Status: "dominated"},
					{Column: "Mastered", Truthy: []string{"1"}, Status: "mastered"},
					{Column: "Finished", Truthy: []string{"1"}, Status: "completed"},
					{Column: "Shelved", Truthy: []string{"1"}, Status: "dropped"},
					{Column: "Playing", Truthy: []string{"1"}, Status: "in_progress"},
					{Column: "Played", Truthy: []string{"1"}, Status: "shelved"},
				},
				Default: "not_started",
			},
		},
		Rating:   &RatingConfig{Scale: 5, Truncate: true},
		Duration: &DurationConfig{Format: "h:mm"},
		Grouping: GroupingConfig{
			CopyRows: &CopyRowGrouping{ContinuationColumn: "Name"},
		},
		Notes: NotesConfig{
			Column: "Notes",
			Assembly: &NoteAssembly{
				ReviewSubjectColumn: "Review subject",
				ReviewColumn:        "Review",
				CopyNoteColumn:      "Copy notes",
			},
		},
		Platform: PlatformConfig{
			Tables: &PlatformTables{
				AggregateColumn:    "Platforms",
				PlatformColumn:     "Copy platform",
				SourceColumn:       "Copy source",
				SourceOtherColumn:  "Copy source other",
				OtherSentinel:      "Other",
				MediaColumn:        "Copy media",
				MediaPhysicalValue: "Physical",
				PurchaseDateColumn: "Copy purchase date",
				// Looked up case-sensitively by the raw trimmed platform string.
				Platforms: map[string]PlatformMapping{
					"PC":                         {Slug: "pc-windows"},
					"Linux":                      {Slug: "pc-linux"},
					"Mac":                        {Slug: "mac"},
					"PlayStation 4":              {Slug: "playstation-4"},
					"PlayStation 5":              {Slug: "playstation-5"},
					"PlayStation 3":              {Slug: "playstation-3"},
					"PlayStation Network (PS3)":  {Slug: "playstation-3", InferredStorefront: &psn},
					"PlayStation Network (Vita)": {Slug: "playstation-vita", InferredStorefront: &psn},
					"Nintendo Switch":            {Slug: "nintendo-switch"},
					"Wii":                        {Slug: "nintendo-wii"},
					"Xbox 360":                   {Slug: "xbox-360"},
					"Xbox 360 Games Store":       {Slug: "xbox-360", InferredStorefront: &xboxStore},
					"Android":                    {Slug: "android"},
					"PlayStation 2":              {Slug: "playstation-2"},
					"PlayStation Network (PSP)":  {Slug: "playstation-psp", InferredStorefront: &psn},
				},
				// Looked up normalized (lowercased + trimmed); keys stay lowercased.
				Storefronts: map[string]string{
					"sony entertainment network": "playstation-store",
					"epic games store":           "epic-games-store",
					"epic game store":            "epic-games-store",
					"epic gamestore":             "epic-games-store",
					"epic":                       "epic-games-store",
					"gog":                        "gog",
					"humble bundle":              "humble-bundle",
					"steam":                      "steam",
					"nintendo eshop":             "nintendo-eshop",
					"origin":                     "origin-ea-app",
					"gamersgate":                 "gamersgate",
					"google play":                "google-play-store",
					"uplay":                      "uplay",
					"ubisoft club":               "uplay",
				},
			},
		},
	}
}
