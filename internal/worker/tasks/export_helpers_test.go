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

func TestBuildCSVRow(t *testing.T) {
	releaseDate := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	ps := "completed"
	rating := int32(9)
	hours := 100.5
	notes := "great game"
	platform := "pc-windows"
	acquired := time.Date(2021, 6, 1, 0, 0, 0, 0, time.UTC)
	tagName := "favorites"

	tests := []struct {
		name  string
		ug    models.UserGame
		check func(t *testing.T, row []string)
	}{
		{
			name: "no_game",
			ug: models.UserGame{
				ID: "ug1", UserID: "u1", GameID: 42,
				CreatedAt: time.Now(), UpdatedAt: time.Now(),
			},
			check: func(t *testing.T, row []string) {
				if row[0] != "" {
					t.Errorf("expected empty title when Game is nil, got %q", row[0])
				}
			},
		},
		{
			name: "all_nil_optionals",
			ug: models.UserGame{
				ID: "ug1", UserID: "u1", GameID: 42,
				CreatedAt: time.Now(), UpdatedAt: time.Now(),
				Game: &models.Game{Title: "My Game", ID: 42, LastUpdated: time.Now(), CreatedAt: time.Now()},
			},
			check: func(t *testing.T, row []string) {
				// title set, play_status empty.
				if row[0] != "My Game" {
					t.Errorf("expected 'My Game', got %q", row[0])
				}
				if row[2] != "" {
					t.Errorf("expected empty play_status, got %q", row[2])
				}
			},
		},
		{
			name: "all_fields_set",
			ug: models.UserGame{
				ID: "ug1", UserID: "u1", GameID: 42,
				PlayStatus: &ps, PersonalRating: &rating, IsLoved: true, PersonalNotes: &notes,
				CreatedAt: time.Now(), UpdatedAt: time.Now(),
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
			},
			check: func(t *testing.T, row []string) {
				if row[0] != "Full Game" {
					t.Errorf("title: expected 'Full Game', got %q", row[0])
				}
				if row[2] != "completed" {
					t.Errorf("play_status: expected 'completed', got %q", row[2])
				}
				if row[3] != "9" {
					t.Errorf("rating: expected '9', got %q", row[3])
				}
			},
		},
		{
			name: "nil_platform_slug",
			// Platform with nil Platform pointer — should be skipped in slugs.
			ug: models.UserGame{
				ID: "ug1", UserID: "u1", GameID: 42,
				CreatedAt: time.Now(), UpdatedAt: time.Now(),
				Game: &models.Game{ID: 42, Title: "Game", LastUpdated: time.Now(), CreatedAt: time.Now()},
				Platforms: []models.UserGamePlatform{
					{ID: "ugp1", UserGameID: "ug1", Platform: nil, CreatedAt: time.Now(), UpdatedAt: time.Now()},
				},
			},
			check: func(t *testing.T, row []string) {
				if row[7] != "" {
					t.Errorf("expected empty platforms (nil slug skipped), got %q", row[7])
				}
			},
		},
		{
			name: "nil_tag_entry",
			// UserGameTag with nil Tag pointer — should be skipped.
			ug: models.UserGame{
				ID: "ug1", UserID: "u1", GameID: 42,
				CreatedAt: time.Now(), UpdatedAt: time.Now(),
				Game: &models.Game{ID: 42, Title: "Game", LastUpdated: time.Now(), CreatedAt: time.Now()},
				Tags: []models.UserGameTag{
					{ID: "t1", UserGameID: "ug1", TagID: "tag1", Tag: nil},
				},
			},
			check: func(t *testing.T, row []string) {
				if row[8] != "" {
					t.Errorf("expected empty tags (nil tag skipped), got %q", row[8])
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.check(t, buildCSVRow(tc.ug))
		})
	}
}

// ---------------------------------------------------------------------------
// buildJSONDoc
// ---------------------------------------------------------------------------

func TestBuildJSONDoc(t *testing.T) {
	platform := "pc-windows"
	storefront := "steam"
	rating := int32(4)
	h := 10.5
	status := "completed"

	tests := []struct {
		name string
		// games is the input slice (nil exercises the empty-export case).
		games []models.UserGame
		check func(t *testing.T, doc exportDocJSON)
	}{
		{
			name:  "empty_games",
			games: nil,
			check: func(t *testing.T, doc exportDocJSON) {
				if doc.Format != "nexorious-library" {
					t.Errorf("format = %q, want nexorious-library", doc.Format)
				}
				if doc.Version != "2.1" {
					t.Errorf("version = %q, want 2.1", doc.Version)
				}
				if doc.ExportedAt == "" {
					t.Errorf("exported_at should be set")
				}
				if len(doc.Games) != 0 {
					t.Errorf("expected 0 games, got %d", len(doc.Games))
				}
			},
		},
		{
			name: "nil_tag",
			games: []models.UserGame{{
				ID: "ug1", UserID: "u1", GameID: 42,
				CreatedAt: time.Now(), UpdatedAt: time.Now(),
				Tags: []models.UserGameTag{
					{ID: "t1", UserGameID: "ug1", TagID: "tag1", Tag: nil},
				},
			}},
			check: func(t *testing.T, doc exportDocJSON) {
				if len(doc.Games) != 1 {
					t.Fatalf("expected 1 game entry")
				}
				if len(doc.Games[0].Tags) != 0 {
					t.Errorf("expected 0 tags (nil tag skipped), got %d", len(doc.Games[0].Tags))
				}
				if doc.Games[0].Title != "" {
					t.Errorf("title = %q, want empty when Game is nil", doc.Games[0].Title)
				}
			},
		},
		{
			name: "platform_canonical_keys",
			games: []models.UserGame{{
				ID: "ug1", UserID: "u1", GameID: 42,
				CreatedAt: time.Now(), UpdatedAt: time.Now(),
				Platforms: []models.UserGamePlatform{
					{ID: "ugp1", UserGameID: "ug1", Platform: &platform, Storefront: &storefront, CreatedAt: time.Now(), UpdatedAt: time.Now()},
				},
			}},
			check: func(t *testing.T, doc exportDocJSON) {
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
			},
		},
		{
			name: "user_fields_and_per_platform_hours",
			games: []models.UserGame{{
				ID: "ug1", UserID: "u1", GameID: 42,
				IsLoved: true, PersonalRating: &rating, PlayStatus: &status,
				CreatedAt: time.Now(), UpdatedAt: time.Now(),
				Platforms: []models.UserGamePlatform{
					{ID: "ugp1", UserGameID: "ug1", Platform: &platform, HoursPlayed: &h, CreatedAt: time.Now(), UpdatedAt: time.Now()},
				},
			}},
			check: func(t *testing.T, doc exportDocJSON) {
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
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.check(t, buildJSONDoc(tc.games))
		})
	}
}
