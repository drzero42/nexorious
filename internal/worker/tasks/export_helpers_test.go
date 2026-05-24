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
	doc := buildJSONDoc("u1", nil)
	if doc.TotalGames != 0 {
		t.Errorf("expected 0 games, got %d", doc.TotalGames)
	}
	if doc.ExportVersion != "1.2" {
		t.Errorf("expected version 1.2, got %q", doc.ExportVersion)
	}
}

func TestBuildJSONDoc_NilTag(t *testing.T) {
	// Tag with nil pointer should be skipped.
	ug := models.UserGame{
		ID:        "ug1",
		UserID:    "u1",
		GameID:    42,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Tags: []models.UserGameTag{
			{ID: "t1", UserGameID: "ug1", TagID: "tag1", Tag: nil},
		},
	}
	doc := buildJSONDoc("u1", []models.UserGame{ug})
	if len(doc.Games) != 1 {
		t.Fatalf("expected 1 game entry")
	}
	if len(doc.Games[0].Tags) != 0 {
		t.Errorf("expected 0 tags (nil tag skipped), got %d", len(doc.Games[0].Tags))
	}
}

func TestBuildJSONDoc_NilOriginalPlatformName(t *testing.T) {
	// Platform with nil OriginalPlatformName — falls back to slug pointer.
	platform := "pc-windows"
	ug := models.UserGame{
		ID:        "ug1",
		UserID:    "u1",
		GameID:    42,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Platforms: []models.UserGamePlatform{
			{
				ID: "ugp1", UserGameID: "ug1",
				Platform:             &platform,
				OriginalPlatformName: nil,   // will fall back to slug
				Storefront:           &platform,
				OriginalStorefrontName: nil, // will fall back to slug
				CreatedAt:            time.Now(), UpdatedAt: time.Now(),
			},
		},
	}
	doc := buildJSONDoc("u1", []models.UserGame{ug})
	if len(doc.Games[0].Platforms) != 1 {
		t.Fatalf("expected 1 platform")
	}
}

func TestBuildJSONDoc_WithLovedAndRated(t *testing.T) {
	rating := int32(8)
	hours := 10.0
	status := "playing"
	platform := "pc-windows"
	ug := models.UserGame{
		ID:             "ug1",
		UserID:         "u1",
		GameID:         42,
		IsLoved:        true,
		PersonalRating: &rating,
		PlayStatus:     &status,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Platforms: []models.UserGamePlatform{
			{ID: "ugp1", UserGameID: "ug1", Platform: &platform, HoursPlayed: &hours, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		},
	}
	doc := buildJSONDoc("u1", []models.UserGame{ug})
	if doc.ExportStats.LovedCount != 1 {
		t.Errorf("expected LovedCount=1, got %d", doc.ExportStats.LovedCount)
	}
	if doc.ExportStats.RatedCount != 1 {
		t.Errorf("expected RatedCount=1, got %d", doc.ExportStats.RatedCount)
	}
	if doc.ExportStats.TotalHours != 10.0 {
		t.Errorf("expected TotalHours=10.0, got %v", doc.ExportStats.TotalHours)
	}
}

func TestBuildJSONDoc_HoursFromPlatforms(t *testing.T) {
	h1 := 10.5
	h2 := 4.5
	platform := "pc-windows"
	ug := models.UserGame{
		ID:        "ug1",
		UserID:    "u1",
		GameID:    42,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Platforms: []models.UserGamePlatform{
			{ID: "ugp1", UserGameID: "ug1", Platform: &platform, HoursPlayed: &h1, CreatedAt: time.Now(), UpdatedAt: time.Now()},
			{ID: "ugp2", UserGameID: "ug1", Platform: &platform, HoursPlayed: &h2, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		},
	}
	doc := buildJSONDoc("u1", []models.UserGame{ug})
	if doc.ExportStats.TotalHours != 15.0 {
		t.Errorf("TotalHours: want 15.0, got %v", doc.ExportStats.TotalHours)
	}
	if doc.Games[0].HoursPlayed == nil || *doc.Games[0].HoursPlayed != 15.0 {
		t.Errorf("games[0].HoursPlayed: want 15.0, got %v", doc.Games[0].HoursPlayed)
	}
}
