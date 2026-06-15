package csvmap

import (
	"reflect"
	"testing"
)

func TestGuessColumns_ExactAndContains(t *testing.T) {
	header := []string{"Game Name", "System", "Play Status", "Score", "Date Bought", "Hours Played", "Genres", "Favorite", "Store", "Comments"}
	m := GuessColumns(header)

	if m.Columns.Title != "Game Name" {
		t.Errorf("title = %q, want Game Name", m.Columns.Title)
	}
	if m.Columns.Platform != "System" {
		t.Errorf("platform = %q, want System", m.Columns.Platform)
	}
	if m.Status.Column != "Play Status" {
		t.Errorf("status column = %q, want Play Status", m.Status.Column)
	}
	if m.Columns.Rating != "Score" {
		t.Errorf("rating = %q, want Score", m.Columns.Rating)
	}
	if m.Columns.AcquiredDate != "Date Bought" {
		t.Errorf("acquired_date = %q, want Date Bought", m.Columns.AcquiredDate)
	}
	if m.Columns.HoursPlayed != "Hours Played" {
		t.Errorf("hours_played = %q, want Hours Played", m.Columns.HoursPlayed)
	}
	if m.Columns.Tags != "Genres" {
		t.Errorf("tags = %q, want Genres", m.Columns.Tags)
	}
	if m.Columns.Loved != "Favorite" {
		t.Errorf("loved = %q, want Favorite", m.Columns.Loved)
	}
	if m.Columns.Storefront != "Store" {
		t.Errorf("storefront = %q, want Store", m.Columns.Storefront)
	}
	if m.Columns.Notes != "Comments" {
		t.Errorf("notes = %q, want Comments", m.Columns.Notes)
	}
	if !m.MergeByTitle {
		t.Errorf("merge_by_title should default true")
	}
	if m.RatingScale != 5 {
		t.Errorf("rating_scale default = %d, want 5", m.RatingScale)
	}
}

func TestGuessColumns_FirstWinsAndDedup(t *testing.T) {
	header := []string{"Title", "Name"}
	m := GuessColumns(header)
	if m.Columns.Title != "Title" {
		t.Errorf("title = %q, want Title (first match wins)", m.Columns.Title)
	}
	if m.Columns.Platform == "Name" || m.Columns.Notes == "Name" {
		t.Errorf("Name should remain unclaimed, got platform=%q notes=%q", m.Columns.Platform, m.Columns.Notes)
	}
}

func TestGuessColumns_NoMatchLeavesBlank(t *testing.T) {
	m := GuessColumns([]string{"col_a", "col_b"})
	if m.Columns.Title != "" || m.Status.Column != "" || m.Columns.Rating != "" {
		t.Errorf("expected all blank, got %+v / status=%q", m.Columns, m.Status.Column)
	}
}

func TestGuessRatingScale(t *testing.T) {
	cases := []struct {
		max  float64
		want int
	}{
		{0, 5}, {3, 5}, {5, 5}, {6, 10}, {10, 10}, {42, 100}, {100, 100},
	}
	for _, c := range cases {
		if got := GuessRatingScale(c.max); got != c.want {
			t.Errorf("GuessRatingScale(%v) = %d, want %d", c.max, got, c.want)
		}
	}
}

func TestGuessStatusValueMap(t *testing.T) {
	got := GuessStatusValueMap([]string{"Beaten", "Playing", "Backlog", "Dropped", "Weird Value"})
	want := map[string]string{
		"Beaten":      "completed",
		"Playing":     "in_progress",
		"Backlog":     "not_started",
		"Dropped":     "dropped",
		"Weird Value": "not_started",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GuessStatusValueMap = %v, want %v", got, want)
	}
}

func TestGuessStatusValueMap_Empty(t *testing.T) {
	if got := GuessStatusValueMap(nil); len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

func TestGuessColumns_ContainsPassMultipleFields(t *testing.T) {
	// Headers chosen so their normalized forms only match via substring-contains,
	// not exact-normalized, and don't collide with any higher-priority field's aliases.
	// "User Rating"  → "userrating"   (contains "rating"; no title/status/platform/storefront alias)
	// "My Store"     → "mystore"      (contains "store"; no title/status/platform alias)
	// "Total Play Time" → "totalplaytime" (contains "playtime"; no title/status/platform/storefront alias)
	header := []string{"User Rating", "My Store", "Total Play Time"}
	m := GuessColumns(header)
	if m.Columns.Rating != "User Rating" {
		t.Errorf("rating = %q, want \"User Rating\"", m.Columns.Rating)
	}
	if m.Columns.Storefront != "My Store" {
		t.Errorf("storefront = %q, want \"My Store\"", m.Columns.Storefront)
	}
	if m.Columns.HoursPlayed != "Total Play Time" {
		t.Errorf("hours_played = %q, want \"Total Play Time\"", m.Columns.HoursPlayed)
	}
}
