package api

import "testing"

func TestBuildCSVConfig_FullMapping(t *testing.T) {
	var m csvMapping
	m.Columns.Title = "Game Name"
	m.Columns.Platform = "System"
	m.Columns.Storefront = "Store"
	m.Columns.AcquiredDate = "Bought"
	m.Columns.Rating = "Score"
	m.Columns.Notes = "Comment"
	m.Columns.HoursPlayed = "Hours"
	m.Columns.Tags = "Labels"
	m.Columns.Loved = "Fav"
	m.Status.Column = "State"
	m.Status.ValueMap = map[string]string{"Beaten": "completed"}
	m.RatingScale = 10
	m.MergeByTitle = true

	cfg, err := buildCSVConfig(m)
	if err != nil {
		t.Fatalf("buildCSVConfig: %v", err)
	}
	if cfg.Columns.Title != "Game Name" || cfg.Columns.Rating != "Score" || cfg.Columns.HoursPlayed != "Hours" || cfg.Columns.Tags != "Labels" || cfg.Columns.Loved != "Fav" {
		t.Errorf("scalar columns not mapped: %+v", cfg.Columns)
	}
	if cfg.Notes.Column != "Comment" {
		t.Errorf("notes column = %q, want Comment", cfg.Notes.Column)
	}
	if cfg.Platform.Simple == nil || cfg.Platform.Simple.PlatformColumn != "System" ||
		cfg.Platform.Simple.StorefrontColumn != "Store" || cfg.Platform.Simple.AcquiredDateColumn != "Bought" {
		t.Errorf("platform-simple not mapped: %+v", cfg.Platform.Simple)
	}
	if cfg.Platform.Simple.PlatformMap != nil || cfg.Platform.Simple.StorefrontMap != nil {
		t.Errorf("platform/storefront maps should be nil (passthrough)")
	}
	if cfg.Status.Column == nil || cfg.Status.Column.Column != "State" || cfg.Status.Column.Default != "not_started" {
		t.Fatalf("status column not mapped: %+v", cfg.Status.Column)
	}
	if got, ok := cfg.Status.Column.ValueMap["beaten"]; !ok || got != "completed" {
		t.Errorf("value-map key not lowercased / wrong: %+v", cfg.Status.Column.ValueMap)
	}
	if cfg.Rating == nil || cfg.Rating.Scale != 10 || cfg.Rating.Truncate {
		t.Errorf("rating not mapped: %+v", cfg.Rating)
	}
	if cfg.Duration == nil || cfg.Duration.Format != "decimal" {
		t.Errorf("duration not mapped: %+v", cfg.Duration)
	}
	if !cfg.Grouping.MergeByTitle {
		t.Errorf("merge-by-title not set")
	}
}

func TestBuildCSVConfig_OptionalsOmitted(t *testing.T) {
	var m csvMapping
	m.Columns.Title = "Title"

	cfg, err := buildCSVConfig(m)
	if err != nil {
		t.Fatalf("buildCSVConfig: %v", err)
	}
	if cfg.Platform.Simple != nil {
		t.Errorf("platform should be nil when no platform column")
	}
	if cfg.Status.Column != nil {
		t.Errorf("status should be nil when no status column")
	}
	if cfg.Rating != nil {
		t.Errorf("rating should be nil when no rating column")
	}
	if cfg.Duration != nil {
		t.Errorf("duration should be nil when no hours column")
	}
	if cfg.Notes.Column != "" {
		t.Errorf("notes should be empty when no notes column")
	}
}

func TestBuildCSVConfig_EmptyTitle(t *testing.T) {
	if _, err := buildCSVConfig(csvMapping{}); err == nil {
		t.Fatal("expected an error for empty title")
	}
}

func TestBuildCSVConfig_BadRatingScale(t *testing.T) {
	var m csvMapping
	m.Columns.Title = "Title"
	m.Columns.Rating = "Score"
	m.RatingScale = 7
	if _, err := buildCSVConfig(m); err == nil {
		t.Fatal("expected an error for an invalid rating scale")
	}
}

func TestBuildCSVConfig_IGDBIDColumn(t *testing.T) {
	var m csvMapping
	m.Columns.Title = "title"
	m.Columns.IGDBID = "igdb_id"

	cfg, err := buildCSVConfig(m)
	if err != nil {
		t.Fatalf("buildCSVConfig: %v", err)
	}
	if cfg.Columns.IGDBID != "igdb_id" {
		t.Errorf("cfg.Columns.IGDBID = %q, want %q", cfg.Columns.IGDBID, "igdb_id")
	}
}

func TestBuildCSVConfig_IGDBIDOmittedWhenUnmapped(t *testing.T) {
	var m csvMapping
	m.Columns.Title = "title"

	cfg, err := buildCSVConfig(m)
	if err != nil {
		t.Fatalf("buildCSVConfig: %v", err)
	}
	if cfg.Columns.IGDBID != "" {
		t.Errorf("cfg.Columns.IGDBID = %q, want empty", cfg.Columns.IGDBID)
	}
}
