package api_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/usergame"
)

// TestClearWishlistOnAcquire exercises the ClearWishlistOnAcquire helper
// directly against the shared test database.
func TestClearWishlistOnAcquire(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// Seed a user.
	userID := "u-wl-" + uuid.NewString()[:8]
	insertAuthTestUser(t, testDB, userID, "wl-"+userID[:8], "pass123", true, false)

	// Seed a game.
	gameID := insertTestGame(t, testDB, "Hades-Wishlist")

	// Insert a wishlisted user_game with no platforms.
	ugID := uuid.NewString()
	_, err := testDB.NewRaw(
		`INSERT INTO user_games (id, user_id, game_id, is_loved, is_wishlisted, created_at, updated_at)
		 VALUES (?, ?, ?, false, true, ?, ?)`,
		ugID, userID, gameID, now, now,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert user_game: %v", err)
	}

	// No platforms yet: helper must NOT clear the flag (EXISTS guard).
	if err := usergame.ClearWishlistOnAcquire(ctx, testDB, ugID); err != nil {
		t.Fatalf("ClearWishlistOnAcquire (no platforms): %v", err)
	}
	if got := wishlistFlag(t, ugID); got != true {
		t.Fatalf("expected still wishlisted with no platforms, got is_wishlisted=%v", got)
	}

	// Attach a platform, then the helper must clear the flag.
	// platform must reference a seeded platforms row; 'pc-windows' always exists.
	_, err = testDB.NewRaw(
		`INSERT INTO user_game_platforms (id, user_game_id, platform, is_available, created_at, updated_at)
		 VALUES (?, ?, 'pc-windows', true, ?, ?)`,
		uuid.NewString(), ugID, now, now,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert user_game_platform: %v", err)
	}
	if err := usergame.ClearWishlistOnAcquire(ctx, testDB, ugID); err != nil {
		t.Fatalf("ClearWishlistOnAcquire (with platform): %v", err)
	}
	if got := wishlistFlag(t, ugID); got != false {
		t.Fatalf("expected promoted (is_wishlisted=false), got is_wishlisted=%v", got)
	}

	// Idempotent: a second call is a no-op and does not error.
	if err := usergame.ClearWishlistOnAcquire(ctx, testDB, ugID); err != nil {
		t.Fatalf("ClearWishlistOnAcquire (idempotent): %v", err)
	}
	if got := wishlistFlag(t, ugID); got != false {
		t.Fatalf("expected still false after idempotent call, got is_wishlisted=%v", got)
	}
}

// wishlistFlag loads the is_wishlisted column for the given user_game id.
func wishlistFlag(t *testing.T, ugID string) bool {
	t.Helper()
	var ug models.UserGame
	if err := testDB.NewSelect().Model(&ug).Where("id = ?", ugID).Scan(context.Background()); err != nil {
		t.Fatalf("load user_game: %v", err)
	}
	return ug.IsWishlisted
}
