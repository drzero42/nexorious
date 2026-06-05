package tasks

// Internal tests for unexported export helpers.

import (
	"testing"
	"time"

	"github.com/drzero42/nexorious/internal/db/models"
)

// ---------------------------------------------------------------------------
// buildCSVRow
// ---------------------------------------------------------------------------

func TestBuildCSVRow_NoGame(t *testing.T) {
	ug := models.UserGame{
		ID:        "ug1",
		UserID:    "u1",
		GameID:    42,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	row := buildCSVRow(ug)
	if row[0] != "" {
		t.Errorf("expected empty title when Game is nil, got %q", row[0])
	}
}

func TestBuildCSVRow_AllNilOptionals(t *testing.T) {
	ug := models.UserGame{
		ID:        "ug1",
		UserID:    "u1",
		GameID:    42,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Game:      &models.Game{Title: "My Game", ID: 42, LastUpdated: time.Now(), CreatedAt: time.Now()},
	}
	row := buildCSVRow(ug)
	// title set, play_status empty, rating empty, etc.
	if row[0] != "My Game" {
		t.Errorf("expected 'My Game', got %q", row[0])
	}
	if row[2] != "" {
		t.Errorf("expected empty play_status, got %q", row[2])
	}
}

func TestBuildCSVRow_AllFieldsSet(t *testing.T) {
	releaseDate := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	ps := "completed"
	rating := int32(9)
	hours := 100.5
	notes := "great game"
	platform := "pc-windows"
	acquired := time.Date(2021, 6, 1, 0, 0, 0, 0, time.UTC)
	tagName := "favorites"

	ug := models.UserGame{
		ID:             "ug1",
		UserID:         "u1",
		GameID:         42,
		PlayStatus:     &ps,
		PersonalRating: &rating,
		IsLoved:        true,
		PersonalNotes:  &notes,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Game: &models.Game{
			ID: 42, Title: "Full Game",
			ReleaseDate: &releaseDate,
			LastUpdated: time.Now(), CreatedAt: time.Now(),
		},
		Platforms: []models.UserGamePlatform{
			{
				ID: "ugp1", UserGameID: "ug1",
				Platform:     &platform,
				HoursPlayed:  &hours,
				AcquiredDate: &acquired,
				CreatedAt:    time.Now(), UpdatedAt: time.Now(),
			},
		},
		Tags: []models.UserGameTag{
			{ID: "t1", UserGameID: "ug1", TagID: "tag1",
				Tag: &models.Tag{ID: "tag1", UserID: "u1", Name: tagName, CreatedAt: time.Now(), UpdatedAt: time.Now()},
			},
		},
	}

	row := buildCSVRow(ug)
	if row[0] != "Full Game" {
		t.Errorf("title: expected 'Full Game', got %q", row[0])
	}
	if row[2] != "completed" {
		t.Errorf("play_status: expected 'completed', got %q", row[2])
	}
	if row[3] != "9" {
		t.Errorf("rating: expected '9', got %q", row[3])
	}
	if row[9] != "2020" {
		t.Errorf("release_year: expected '2020', got %q", row[9])
	}
}

func TestBuildCSVRow_NilPlatformSlug(t *testing.T) {
	// Platform with nil Platform pointer — should be skipped in slugs.
	ug := models.UserGame{
		ID:        "ug1",
		UserID:    "u1",
		GameID:    42,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Game:      &models.Game{ID: 42, Title: "Game", LastUpdated: time.Now(), CreatedAt: time.Now()},
		Platforms: []models.UserGamePlatform{
			{ID: "ugp1", UserGameID: "ug1", Platform: nil, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		},
	}
	row := buildCSVRow(ug)
	// platforms field should be empty (nil slug skipped).
	if row[7] != "" {
		t.Errorf("expected empty platforms, got %q", row[7])
	}
}

func TestBuildCSVRow_NilTagEntry(t *testing.T) {
	// UserGameTag with nil Tag pointer — should be skipped.
	ug := models.UserGame{
		ID:        "ug1",
		UserID:    "u1",
		GameID:    42,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Game:      &models.Game{ID: 42, Title: "Game", LastUpdated: time.Now(), CreatedAt: time.Now()},
		Tags: []models.UserGameTag{
			{ID: "t1", UserGameID: "ug1", TagID: "tag1", Tag: nil},
		},
	}
	row := buildCSVRow(ug)
	// tags field should be empty (nil tag skipped).
	if row[8] != "" {
		t.Errorf("expected empty tags, got %q", row[8])
	}
}

// ---------------------------------------------------------------------------
// buildJSONDoc
// ---------------------------------------------------------------------------

func TestBuildJSONDoc_EmptyGames(t *testing.T) {
	doc := buildJSONDoc(nil)
	if doc.Format != "nexorious-library" {
		t.Errorf("format = %q, want nexorious-library", doc.Format)
	}
	if doc.Version != "2.0" {
		t.Errorf("version = %q, want 2.0", doc.Version)
	}
	if doc.ExportedAt == "" {
		t.Errorf("exported_at should be set")
	}
	if len(doc.Games) != 0 {
		t.Errorf("expected 0 games, got %d", len(doc.Games))
	}
}

func TestBuildJSONDoc_NilTag(t *testing.T) {
	ug := models.UserGame{
		ID: "ug1", UserID: "u1", GameID: 42,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
		Tags: []models.UserGameTag{
			{ID: "t1", UserGameID: "ug1", TagID: "tag1", Tag: nil},
		},
	}
	doc := buildJSONDoc([]models.UserGame{ug})
	if len(doc.Games) != 1 {
		t.Fatalf("expected 1 game entry")
	}
	if len(doc.Games[0].Tags) != 0 {
		t.Errorf("expected 0 tags (nil tag skipped), got %d", len(doc.Games[0].Tags))
	}
	if doc.Games[0].Title != "" {
		t.Errorf("title = %q, want empty when Game is nil", doc.Games[0].Title)
	}
}

func TestBuildJSONDoc_PlatformCanonicalKeys(t *testing.T) {
	platform := "pc-windows"
	storefront := "steam"
	ug := models.UserGame{
		ID: "ug1", UserID: "u1", GameID: 42,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
		Platforms: []models.UserGamePlatform{
			{ID: "ugp1", UserGameID: "ug1", Platform: &platform, Storefront: &storefront, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		},
	}
	doc := buildJSONDoc([]models.UserGame{ug})
	if len(doc.Games[0].Platforms) != 1 {
		t.Fatalf("expected 1 platform")
	}
	pj := doc.Games[0].Platforms[0]
	if pj.Platform == nil || *pj.Platform != platform {
		t.Errorf("platform = %v, want %q", pj.Platform, platform)
	}
	if pj.Storefront == nil || *pj.Storefront != storefront {
		t.Errorf("storefront = %v, want %q", pj.Storefront, storefront)
	}
}

func TestBuildJSONDoc_UserFieldsAndPerPlatformHours(t *testing.T) {
	rating := int32(4)
	h := 10.5
	status := "completed"
	platform := "pc-windows"
	ug := models.UserGame{
		ID: "ug1", UserID: "u1", GameID: 42,
		IsLoved: true, PersonalRating: &rating, PlayStatus: &status,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
		Platforms: []models.UserGamePlatform{
			{ID: "ugp1", UserGameID: "ug1", Platform: &platform, HoursPlayed: &h, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		},
	}
	doc := buildJSONDoc([]models.UserGame{ug})
	g := doc.Games[0]
	if g.PersonalRating == nil || *g.PersonalRating != 4 {
		t.Errorf("personal_rating = %v, want 4", g.PersonalRating)
	}
	if !g.IsLoved {
		t.Errorf("is_loved = false, want true")
	}
	if g.PlayStatus == nil || *g.PlayStatus != "completed" {
		t.Errorf("play_status = %v, want completed", g.PlayStatus)
	}
	if g.Platforms[0].HoursPlayed == nil || *g.Platforms[0].HoursPlayed != 10.5 {
		t.Errorf("platform hours = %v, want 10.5", g.Platforms[0].HoursPlayed)
	}
}
