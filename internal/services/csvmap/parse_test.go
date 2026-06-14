package csvmap

import (
	"errors"
	"testing"

	"github.com/drzero42/nexorious/internal/services/importmodel"
)

// titleOnlyConfig is the minimal valid config: one-row grouping, just a title.
func titleOnlyConfig() Config {
	return Config{Columns: ColumnMap{Title: "Name"}}
}

func TestParse_TitleOnly_OneRowPerGame(t *testing.T) {
	csv := "Name,Other\nHalf-Life,x\nPortal,y\n"
	games, err := Parse([]byte(csv), titleOnlyConfig())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(games) != 2 {
		t.Fatalf("want 2 games, got %d", len(games))
	}
	if games[0].Title != "Half-Life" || games[1].Title != "Portal" {
		t.Fatalf("unexpected titles: %+v", games)
	}
}

func TestParse_SkipsEmptyTitleRow(t *testing.T) {
	csv := "Name\nHalf-Life\n\nPortal\n"
	games, err := Parse([]byte(csv), titleOnlyConfig())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(games) != 2 {
		t.Fatalf("want 2 games (empty-title row skipped), got %d", len(games))
	}
}

func TestParse_HeaderCaseAndWhitespaceInsensitive(t *testing.T) {
	csv := "  NAME \nHalf-Life\n"
	games, err := Parse([]byte(csv), titleOnlyConfig())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(games) != 1 || games[0].Title != "Half-Life" {
		t.Fatalf("header normalization failed: %+v", games)
	}
}

func TestParse_RequiresTitle(t *testing.T) {
	_, err := Parse([]byte("A\nx\n"), Config{})
	if err == nil {
		t.Fatal("want error when Columns.Title is empty")
	}
	if errors.Is(err, importmodel.ErrInvalidSignature) {
		t.Fatal("title error must not be ErrInvalidSignature")
	}
}

func TestParse_RejectsAdvancedSlots(t *testing.T) {
	base := func() Config { return Config{Columns: ColumnMap{Title: "Name"}} }
	cases := map[string]Config{
		"status flags": func() Config { c := base(); c.Status.Flags = &StatusFlags{}; return c }(),
		"platform tables": func() Config {
			c := base()
			c.Platform.Tables = &PlatformTables{}
			return c
		}(),
		"notes assembly": func() Config { c := base(); c.Notes.Assembly = &NoteAssembly{}; return c }(),
		"copy rows": func() Config {
			c := base()
			c.Grouping.CopyRows = &CopyRowGrouping{}
			return c
		}(),
		"h:mm duration": func() Config {
			c := base()
			c.Duration = &DurationConfig{Format: "h:mm"}
			return c
		}(),
	}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte("Name\nx\n"), cfg)
			if err == nil {
				t.Fatalf("want error for advanced slot %q", name)
			}
			if errors.Is(err, importmodel.ErrInvalidSignature) {
				t.Fatalf("advanced-slot error must not be ErrInvalidSignature (%q)", name)
			}
		})
	}
}

func TestParse_RejectsConflictingStatusVariants(t *testing.T) {
	cfg := Config{
		Columns: ColumnMap{Title: "Name"},
		Status:  StatusConfig{Column: &StatusColumn{Column: "S"}, Flags: &StatusFlags{}},
	}
	_, err := Parse([]byte("Name\nx\n"), cfg)
	if err == nil {
		t.Fatal("want error when both Status.Column and Status.Flags are set")
	}
}

func TestParse_RejectsBadRatingScale(t *testing.T) {
	cfg := Config{Columns: ColumnMap{Title: "Name"}, Rating: &RatingConfig{Scale: 7}}
	_, err := Parse([]byte("Name\nx\n"), cfg)
	if err == nil {
		t.Fatal("want error for Rating.Scale not in {5,10,100}")
	}
}

func TestMatchesSignature(t *testing.T) {
	headers := []string{"Name", "Status", "Rating"}
	if !MatchesSignature(headers, Config{Signature: []string{"name", "rating"}}) {
		t.Fatal("expected match (case-insensitive, subset)")
	}
	if MatchesSignature(headers, Config{Signature: []string{"Name", "Missing"}}) {
		t.Fatal("expected no match when a signature column is absent")
	}
	if !MatchesSignature(headers, Config{}) {
		t.Fatal("nil signature must always match")
	}
}

func TestParse_SignatureMismatchIsInvalidSignature(t *testing.T) {
	cfg := Config{Columns: ColumnMap{Title: "Name"}, Signature: []string{"Name", "Shelves"}}
	_, err := Parse([]byte("Name,Rating\nHalf-Life,5\n"), cfg)
	if err == nil {
		t.Fatal("want error on signature mismatch")
	}
	if !errors.Is(err, importmodel.ErrInvalidSignature) {
		t.Fatalf("want ErrInvalidSignature, got %v", err)
	}
}

func TestParse_Status(t *testing.T) {
	cfg := Config{
		Columns: ColumnMap{Title: "Name"},
		Status: StatusConfig{Column: &StatusColumn{
			Column:   "Shelf",
			ValueMap: map[string]string{"playing": "in_progress", "beaten": "completed"},
			Default:  "not_started",
		}},
	}
	csv := "Name,Shelf\nA,Playing\nB,Beaten\nC,Wishlist\nD,\n"
	games, err := Parse([]byte(csv), cfg)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := []string{"in_progress", "completed", "not_started", "not_started"}
	for i, w := range want {
		if games[i].PlayStatus != w {
			t.Fatalf("game %d: want status %q, got %q", i, w, games[i].PlayStatus)
		}
	}
}

func TestParse_StatusAbsentDefaultsNotStarted(t *testing.T) {
	games, err := Parse([]byte("Name\nA\n"), Config{Columns: ColumnMap{Title: "Name"}})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if games[0].PlayStatus != "not_started" {
		t.Fatalf("want not_started when no status column, got %q", games[0].PlayStatus)
	}
}

func ratingOf(t *testing.T, cfg Config, cell string) *int32 {
	t.Helper()
	games, err := Parse([]byte("Name,R\nA,"+cell+"\n"), cfg)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return games[0].PersonalRating
}

func TestParse_Rating(t *testing.T) {
	round5 := Config{Columns: ColumnMap{Title: "Name", Rating: "R"}, Rating: &RatingConfig{Scale: 5}}
	trunc5 := Config{Columns: ColumnMap{Title: "Name", Rating: "R"}, Rating: &RatingConfig{Scale: 5, Truncate: true}}
	scale10 := Config{Columns: ColumnMap{Title: "Name", Rating: "R"}, Rating: &RatingConfig{Scale: 10}}
	scale100 := Config{Columns: ColumnMap{Title: "Name", Rating: "R"}, Rating: &RatingConfig{Scale: 100}}

	if got := ratingOf(t, trunc5, "4.5"); got == nil || *got != 4 {
		t.Fatalf("truncate 4.5/5 want 4, got %v", got)
	}
	if got := ratingOf(t, round5, "4.5"); got == nil || *got != 5 {
		t.Fatalf("round 4.5/5 want 5, got %v", got)
	}
	if got := ratingOf(t, scale10, "7"); got == nil || *got != 4 {
		t.Fatalf("round 7/10 want 4 (3.5 -> 4), got %v", got)
	}
	if got := ratingOf(t, scale100, "100"); got == nil || *got != 5 {
		t.Fatalf("100/100 want 5, got %v", got)
	}
	if got := ratingOf(t, round5, "0"); got != nil {
		t.Fatalf("0 want nil, got %v", got)
	}
	if got := ratingOf(t, round5, ""); got != nil {
		t.Fatalf("empty want nil, got %v", got)
	}
	if got := ratingOf(t, round5, "garbage"); got != nil {
		t.Fatalf("invalid want nil, got %v", got)
	}
}

func TestParse_RatingIgnoredWhenConfigNil(t *testing.T) {
	cfg := Config{Columns: ColumnMap{Title: "Name", Rating: "R"}}
	if got := ratingOf(t, cfg, "5"); got != nil {
		t.Fatalf("rating must be nil when Config.Rating is nil, got %v", got)
	}
}

func TestParse_Loved(t *testing.T) {
	cfg := Config{Columns: ColumnMap{Title: "Name", Loved: "Fav"}} // default truthy {"1","true","yes"}
	games, _ := Parse([]byte("Name,Fav\nA,Yes\nB,0\nC,\n"), cfg)
	if !games[0].IsLoved {
		t.Fatal("A: Yes should be loved")
	}
	if games[1].IsLoved || games[2].IsLoved {
		t.Fatal("B/C should not be loved")
	}
}

func TestParse_LovedCustomTruthy(t *testing.T) {
	cfg := Config{Columns: ColumnMap{Title: "Name", Loved: "Fav"}, TruthyValues: []string{"★"}}
	games, _ := Parse([]byte("Name,Fav\nA,★\nB,yes\n"), cfg)
	if !games[0].IsLoved || games[1].IsLoved {
		t.Fatalf("custom truthy failed: %+v", games)
	}
}

func TestParse_Tags(t *testing.T) {
	cfg := Config{Columns: ColumnMap{Title: "Name", Tags: "T"}}
	games, _ := Parse([]byte("Name,T\nA,\"rpg, indie , rpg,\"\n"), cfg)
	want := []string{"rpg", "indie"}
	if len(games[0].Tags) != 2 || games[0].Tags[0] != want[0] || games[0].Tags[1] != want[1] {
		t.Fatalf("tags split/trim/dedupe failed: %#v", games[0].Tags)
	}
}

func TestParse_CreatedAtPassthrough(t *testing.T) {
	cfg := Config{Columns: ColumnMap{Title: "Name", CreatedAt: "Added"}}
	games, _ := Parse([]byte("Name,Added\nA,2021-05-03\nB,not-a-date\n"), cfg)
	if games[0].CreatedAt != "2021-05-03" {
		t.Fatalf("want passthrough date, got %q", games[0].CreatedAt)
	}
	if games[1].CreatedAt != "" {
		t.Fatalf("invalid date should yield empty, got %q", games[1].CreatedAt)
	}
}

func TestParse_CreatedAtCustomLayout(t *testing.T) {
	cfg := Config{Columns: ColumnMap{Title: "Name", CreatedAt: "Added"}, DateLayout: "01/02/2006"}
	games, _ := Parse([]byte("Name,Added\nA,05/03/2021\n"), cfg)
	if games[0].CreatedAt != "2021-05-03" {
		t.Fatalf("want reformatted date 2021-05-03, got %q", games[0].CreatedAt)
	}
}

func TestParse_DurationDecimal(t *testing.T) {
	cfg := Config{Columns: ColumnMap{Title: "Name", HoursPlayed: "H"}, Duration: &DurationConfig{Format: "decimal"}}
	games, _ := Parse([]byte("Name,H\nA,12.5\nB,0\nC,\n"), cfg)
	if games[0].HoursPlayed == nil || *games[0].HoursPlayed != 12.5 {
		t.Fatalf("A: want 12.5, got %v", games[0].HoursPlayed)
	}
	if games[1].HoursPlayed != nil || games[2].HoursPlayed != nil {
		t.Fatal("B/C: want nil hours")
	}
}

func TestParse_DurationIgnoredWhenConfigNil(t *testing.T) {
	cfg := Config{Columns: ColumnMap{Title: "Name", HoursPlayed: "H"}}
	games, _ := Parse([]byte("Name,H\nA,12.5\n"), cfg)
	if games[0].HoursPlayed != nil {
		t.Fatalf("hours must be nil when Config.Duration is nil, got %v", games[0].HoursPlayed)
	}
}

func TestParse_PlatformPassthrough(t *testing.T) {
	cfg := Config{
		Columns:  ColumnMap{Title: "Name"},
		Platform: PlatformConfig{Simple: &PlatformSimple{PlatformColumn: "Plat"}},
	}
	games, _ := Parse([]byte("Name,Plat\nA,SomePlatform\nB,\n"), cfg)
	if len(games[0].Platforms) != 1 || games[0].Platforms[0].Platform != "SomePlatform" {
		t.Fatalf("A: want passthrough platform, got %+v", games[0].Platforms)
	}
	if games[0].Platforms[0].Storefront != nil {
		t.Fatal("A: storefront should be nil")
	}
	if len(games[1].Platforms) != 0 {
		t.Fatalf("B: empty platform cell -> no entry, got %+v", games[1].Platforms)
	}
}

func TestParse_PlatformWithMapsStorefrontAndDate(t *testing.T) {
	cfg := Config{
		Columns: ColumnMap{Title: "Name"},
		Platform: PlatformConfig{Simple: &PlatformSimple{
			PlatformColumn:     "Plat",
			StorefrontColumn:   "Store",
			AcquiredDateColumn: "Bought",
			PlatformMap:        map[string]string{"pc": "pc-windows"},
			StorefrontMap:      map[string]string{"steam": "steam"},
		}},
	}
	games, _ := Parse([]byte("Name,Plat,Store,Bought\nA,PC,Steam,2020-01-02\n"), cfg)
	p := games[0].Platforms[0]
	if p.Platform != "pc-windows" {
		t.Fatalf("want mapped slug pc-windows, got %q", p.Platform)
	}
	if p.Storefront == nil || *p.Storefront != "steam" {
		t.Fatalf("want mapped storefront steam, got %v", p.Storefront)
	}
	if p.AcquiredDate != "2020-01-02" {
		t.Fatalf("want acquired date 2020-01-02, got %q", p.AcquiredDate)
	}
}

func TestParse_PlatformUnmappedPassesThrough(t *testing.T) {
	cfg := Config{
		Columns: ColumnMap{Title: "Name"},
		Platform: PlatformConfig{Simple: &PlatformSimple{
			PlatformColumn: "Plat",
			PlatformMap:    map[string]string{"pc": "pc-windows"},
		}},
	}
	games, _ := Parse([]byte("Name,Plat\nA,Dreamcast\n"), cfg)
	if games[0].Platforms[0].Platform != "Dreamcast" {
		t.Fatalf("unmapped value should pass through, got %q", games[0].Platforms[0].Platform)
	}
}

func TestParse_NoPlatformConfigYieldsNoEntries(t *testing.T) {
	games, _ := Parse([]byte("Name\nA\n"), Config{Columns: ColumnMap{Title: "Name"}})
	if len(games[0].Platforms) != 0 {
		t.Fatalf("want no platform entries, got %+v", games[0].Platforms)
	}
}

func mergeCfg() Config {
	return Config{
		Columns:  ColumnMap{Title: "Name", Rating: "R"},
		Rating:   &RatingConfig{Scale: 5},
		Status:   StatusConfig{Column: &StatusColumn{Column: "S", ValueMap: map[string]string{"playing": "in_progress", "done": "completed"}, Default: "not_started"}},
		Platform: PlatformConfig{Simple: &PlatformSimple{PlatformColumn: "Plat", StorefrontColumn: "Store"}},
		Grouping: GroupingConfig{MergeByTitle: true},
	}
}

func TestParse_MergeByTitle_UnionsPlatformsFirstWinsScalars(t *testing.T) {
	csv := "Name,S,R,Plat,Store\n" +
		"Hades,playing,5,pc-windows,steam\n" +
		"Hades,done,3,nintendo-switch,nintendo-eshop\n"
	games, err := Parse([]byte(csv), mergeCfg())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("want 1 merged game, got %d", len(games))
	}
	g := games[0]
	if g.PlayStatus != "in_progress" {
		t.Fatalf("first-wins status want in_progress, got %q", g.PlayStatus)
	}
	if g.PersonalRating == nil || *g.PersonalRating != 5 {
		t.Fatalf("first-wins rating want 5, got %v", g.PersonalRating)
	}
	if len(g.Platforms) != 2 {
		t.Fatalf("want 2 union platforms, got %+v", g.Platforms)
	}
}

func TestParse_MergeByTitle_DedupesIdenticalPlatform(t *testing.T) {
	csv := "Name,S,R,Plat,Store\n" +
		"Hades,playing,5,pc-windows,steam\n" +
		"Hades,playing,5,pc-windows,steam\n"
	games, _ := Parse([]byte(csv), mergeCfg())
	if len(games) != 1 || len(games[0].Platforms) != 1 {
		t.Fatalf("want 1 game with 1 deduped platform, got %d games / %+v", len(games), games[0].Platforms)
	}
}

func TestParse_OneRow_DoesNotMerge(t *testing.T) {
	cfg := mergeCfg()
	cfg.Grouping.MergeByTitle = false
	csv := "Name,S,R,Plat,Store\n" +
		"Hades,playing,5,pc-windows,steam\n" +
		"Hades,done,3,nintendo-switch,nintendo-eshop\n"
	games, _ := Parse([]byte(csv), cfg)
	if len(games) != 2 {
		t.Fatalf("one-row grouping want 2 games, got %d", len(games))
	}
}

func TestParse_RatingClampsAboveFive(t *testing.T) {
	round5 := Config{Columns: ColumnMap{Title: "Name", Rating: "R"}, Rating: &RatingConfig{Scale: 5}}
	if got := ratingOf(t, round5, "6"); got == nil || *got != 5 {
		t.Fatalf("over-scale 6/5 want clamped 5, got %v", got)
	}
}

func TestParse_RaggedRowFillsMissingColumnsAsEmpty(t *testing.T) {
	// Header has 3 columns; the data row supplies only 1. Missing trailing
	// columns must resolve to empty (their default), not panic.
	cfg := Config{
		Columns: ColumnMap{Title: "Name", Tags: "T"},
		Status:  StatusConfig{Column: &StatusColumn{Column: "S", Default: "not_started"}},
	}
	games, err := Parse([]byte("Name,S,T\nHalf-Life\n"), cfg)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(games) != 1 || games[0].PlayStatus != "not_started" || games[0].Tags != nil {
		t.Fatalf("ragged row failed: %+v", games)
	}
}
