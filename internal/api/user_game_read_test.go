package api_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/drzero42/nexorious/internal/api"
	"github.com/drzero42/nexorious/internal/db/models"
)

func TestLoadUserGameDetail(t *testing.T) {
	// Platform has an m2m relation to Storefront via PlatformStorefront; bun
	// requires the join model to be registered before any query that touches it.
	testDB.RegisterModel((*models.PlatformStorefront)(nil))

	truncateAllTables(t)
	ctx := context.Background()

	userID := "u-read-1"
	insertAuthTestUser(t, testDB, userID, "readuser1", "pass123", true, false)
	gameID := insertTestGame(t, testDB, "Read Detail Game")
	insertTestUserGame(t, testDB, "ug-read-1", userID, int(gameID))

	platform := "pc-windows"
	storefront := "steam"
	insertTestUserGamePlatform(t, testDB, "ugp-read-1", "ug-read-1", &platform, &storefront)

	// External game wired to the platform — its presence is what makes the
	// store_url deep-link resolvable in the projection.
	insertExternalGame(t, testDB, "eg-read-1", userID, "steam", "ext-1", "Read Detail Game")
	if _, err := testDB.ExecContext(ctx,
		`UPDATE user_game_platforms SET external_game_id = ? WHERE id = ?`,
		"eg-read-1", "ugp-read-1"); err != nil {
		t.Fatalf("wire external game: %v", err)
	}

	insertTag(t, testDB, "tag-read-1", userID, "favorites", nil)
	insertUserGameTag(t, testDB, "ugt-read-1", "ug-read-1", "tag-read-1")

	t.Run("loads full relation set", func(t *testing.T) {
		ug, err := api.LoadUserGameDetail(ctx, testDB, "ug-read-1", userID)
		if err != nil {
			t.Fatalf("LoadUserGameDetail: %v", err)
		}
		if ug.Game == nil {
			t.Error("Game relation not loaded")
		}
		if len(ug.Platforms) != 1 {
			t.Fatalf("expected 1 platform, got %d", len(ug.Platforms))
		}
		p := ug.Platforms[0]
		if p.PlatformRecord == nil {
			t.Error("PlatformRecord not loaded")
		}
		if p.StorefrontRecord == nil {
			t.Error("StorefrontRecord not loaded")
		}
		if p.ExternalGame == nil {
			t.Error("ExternalGame not loaded")
		}
		if len(ug.Tags) != 1 || ug.Tags[0].Tag == nil {
			t.Errorf("Tags/Tag relation not loaded: %+v", ug.Tags)
		}
	})

	t.Run("scopes by user_id", func(t *testing.T) {
		_, err := api.LoadUserGameDetail(ctx, testDB, "ug-read-1", "u-other")
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("expected sql.ErrNoRows for another user's game, got %v", err)
		}
	})

	t.Run("by-ids loader returns the same relation set", func(t *testing.T) {
		ugs, err := api.LoadUserGameCardsByIDs(ctx, testDB, []string{"ug-read-1"})
		if err != nil {
			t.Fatalf("LoadUserGameCardsByIDs: %v", err)
		}
		if len(ugs) != 1 {
			t.Fatalf("expected 1, got %d", len(ugs))
		}
		if ugs[0].Game == nil || len(ugs[0].Platforms) != 1 || ugs[0].Platforms[0].ExternalGame == nil {
			t.Error("by-ids loader did not load full relation set")
		}
	})

	t.Run("by-ids loader with empty input", func(t *testing.T) {
		ugs, err := api.LoadUserGameCardsByIDs(ctx, testDB, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ugs) != 0 {
			t.Errorf("expected empty slice, got %d", len(ugs))
		}
	})
}
