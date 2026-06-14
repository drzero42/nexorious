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

// guard against an unused import until later tasks reference these.
var _ = errors.Is
var _ = importmodel.ErrInvalidSignature

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
