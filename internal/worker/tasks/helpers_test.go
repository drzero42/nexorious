package tasks

// Internal tests for unexported helpers in the tasks package.
// These use package tasks (not tasks_test) to access unexported symbols.

import (
	"testing"

	"github.com/drzero42/nexorious/internal/services/igdb"
)

// ---------------------------------------------------------------------------
// parseFlexibleDate
// ---------------------------------------------------------------------------

func TestParseFlexibleDate(t *testing.T) {
	rfc3339 := "2024-01-15T00:00:00Z"
	dateOnly := "2022-06-30"
	unparseable := "not-a-date"

	tests := []struct {
		name  string
		input *string
		// wantNil is true when the result must be nil; otherwise wantY/M/D give
		// the expected parsed date components.
		wantNil             bool
		wantY, wantM, wantD int
	}{
		{name: "nil", input: nil, wantNil: true},
		{name: "rfc3339", input: &rfc3339, wantY: 2024, wantM: 1, wantD: 15},
		{name: "date_only", input: &dateOnly, wantY: 2022, wantM: 6, wantD: 30},
		{name: "unparseable", input: &unparseable, wantNil: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := parseFlexibleDate(tc.input)
			if tc.wantNil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}
			if result == nil {
				t.Fatalf("expected non-nil result for %q", *tc.input)
			}
			if result.Year() != tc.wantY || int(result.Month()) != tc.wantM || result.Day() != tc.wantD {
				t.Errorf("unexpected date: got %v, want %d-%02d-%02d", result, tc.wantY, tc.wantM, tc.wantD)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ownershipRank
// ---------------------------------------------------------------------------

func TestOwnershipRank(t *testing.T) {
	cases := []struct {
		status string
		want   int
	}{
		{"owned", 4},
		{"borrowed", 3},
		{"rented", 3},
		{"subscription", 2},
		{"no_longer_owned", 1},
		{"unknown_status", 0},
		{"", 0},
	}
	for _, tc := range cases {
		if got := ownershipRank(tc.status); got != tc.want {
			t.Errorf("ownershipRank(%q) = %d, want %d", tc.status, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// igdbMetadataToGame
// ---------------------------------------------------------------------------

func TestIgdbMetadataToGame_AllFields(t *testing.T) {
	desc := "A great RPG"
	genre := "RPG"
	dev := "CD Projekt"
	pub := "GOG"
	releaseDate := "2015-05-19"
	coverURL := "https://example.com/cover.jpg"
	rating := 93.5
	ratingCount := int32(50000)
	ttbMain := 50.0
	ttbExtra := 80.0
	ttbComp := 200.0
	gameModes := "Single-player"
	themes := "Fantasy"
	pp := "Third person"

	md := &igdb.GameMetadata{
		IgdbID:                     1942,
		IgdbSlug:                   "the-witcher-3",
		Title:                      "The Witcher 3: Wild Hunt",
		Description:                &desc,
		Genre:                      &genre,
		Developer:                  &dev,
		Publisher:                  &pub,
		ReleaseDate:                &releaseDate,
		CoverArtURL:                &coverURL,
		RatingAverage:              &rating,
		RatingCount:                &ratingCount,
		HowlongtobeatMain:          &ttbMain,
		HowlongtobeatExtra:         &ttbExtra,
		HowlongtobeatCompletionist: &ttbComp,
		PlatformIDs:                []int{6, 48},
		PlatformNames:              []string{"PC (Microsoft Windows)", "PlayStation 4"},
		GameModes:                  &gameModes,
		Themes:                     &themes,
		PlayerPerspectives:         &pp,
	}

	game := igdbMetadataToGame(md)

	if game.ID != 1942 {
		t.Errorf("ID: expected 1942, got %d", game.ID)
	}
	if game.Title != "The Witcher 3: Wild Hunt" {
		t.Errorf("Title mismatch")
	}
	if game.Description == nil || *game.Description != desc {
		t.Errorf("Description mismatch")
	}
	if game.ReleaseDate == nil {
		t.Error("expected ReleaseDate to be set")
	} else {
		if game.ReleaseDate.Year() != 2015 || game.ReleaseDate.Month() != 5 || game.ReleaseDate.Day() != 19 {
			t.Errorf("ReleaseDate mismatch: %v", game.ReleaseDate)
		}
	}
	if game.IgdbPlatformIds == nil {
		t.Error("expected IgdbPlatformIds to be set")
	}
	if game.IgdbPlatformNames == nil {
		t.Error("expected IgdbPlatformNames to be set")
	}
}

func TestIgdbMetadataToGame_MinimalFields(t *testing.T) {
	md := &igdb.GameMetadata{
		IgdbID: 42,
		Title:  "Minimal Game",
	}
	game := igdbMetadataToGame(md)
	if game.ID != 42 || game.Title != "Minimal Game" {
		t.Errorf("unexpected game: %+v", game)
	}
	if game.ReleaseDate != nil {
		t.Error("expected nil ReleaseDate for game without release date")
	}
	if game.IgdbPlatformIds != nil {
		t.Error("expected nil IgdbPlatformIds for game without platforms")
	}
}

func TestIgdbMetadataToGame_InvalidReleaseDate(t *testing.T) {
	bad := "not-a-date"
	md := &igdb.GameMetadata{
		IgdbID:      99,
		Title:       "Bad Date Game",
		ReleaseDate: &bad,
	}
	game := igdbMetadataToGame(md)
	if game.ReleaseDate != nil {
		t.Error("expected nil ReleaseDate for invalid date string")
	}
}
