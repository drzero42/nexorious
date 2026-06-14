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
