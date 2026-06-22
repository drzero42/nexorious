package usergame

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/drzero42/nexorious/internal/db/models"
)

func TestClearWishlist(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := seedUser(t)
	other := seedUser(t)

	// wishlisted + has a platform → cleared
	owned := seedUserGame(t, userID, 101)
	setWishlisted(t, owned, true)
	addPlatformRow(t, owned, "pc-windows", "steam")

	// wishlisted, no platform → NOT cleared (still a pure wishlist entry)
	pureWish := seedUserGame(t, userID, 102)
	setWishlisted(t, pureWish, true)

	// wishlisted + platform but other user → NOT cleared (ownership scope)
	foreign := seedUserGame(t, other, 103)
	setWishlisted(t, foreign, true)
	addPlatformRow(t, foreign, "pc-windows", "steam")

	n, err := ClearWishlist(ctx, testDB, userID, []string{owned, pureWish, foreign})
	if err != nil {
		t.Fatalf("ClearWishlist: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 cleared, got %d", n)
	}
	if isWishlisted(t, owned) {
		t.Error("owned game should have been cleared")
	}
	if !isWishlisted(t, pureWish) {
		t.Error("pure-wishlist game must stay wishlisted")
	}
	if !isWishlisted(t, foreign) {
		t.Error("other user's game must not be touched")
	}

	// Idempotent: a second call clears nothing.
	n2, err := ClearWishlist(ctx, testDB, userID, []string{owned})
	if err != nil {
		t.Fatalf("ClearWishlist (2nd): %v", err)
	}
	if n2 != 0 {
		t.Fatalf("expected 0 on idempotent call, got %d", n2)
	}
}

// TestClearWishlistOnAcquire exercises the clearWishlistOnAcquire helper
// directly against the shared test database.
func TestClearWishlistOnAcquire(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// Seed a user and game.
	userID := seedUser(t)
	seedGame(t, 9001, "Hades-Wishlist")

	// Insert a wishlisted user_game with no platforms.
	ugID := uuid.NewString()
	_, err := testDB.NewRaw(
		`INSERT INTO user_games (id, user_id, game_id, is_loved, is_wishlisted, created_at, updated_at)
		 VALUES (?, ?, ?, false, true, ?, ?)`,
		ugID, userID, int32(9001), now, now,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert user_game: %v", err)
	}

	// No platforms yet: helper must NOT clear the flag (EXISTS guard).
	if err := clearWishlistOnAcquire(ctx, testDB, ugID); err != nil {
		t.Fatalf("clearWishlistOnAcquire (no platforms): %v", err)
	}
	if got := loadWishlistFlag(t, ugID); got != true {
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
	if err := clearWishlistOnAcquire(ctx, testDB, ugID); err != nil {
		t.Fatalf("clearWishlistOnAcquire (with platform): %v", err)
	}
	if got := loadWishlistFlag(t, ugID); got != false {
		t.Fatalf("expected promoted (is_wishlisted=false), got is_wishlisted=%v", got)
	}

	// Idempotent: a second call is a no-op and does not error.
	if err := clearWishlistOnAcquire(ctx, testDB, ugID); err != nil {
		t.Fatalf("clearWishlistOnAcquire (idempotent): %v", err)
	}
	if got := loadWishlistFlag(t, ugID); got != false {
		t.Fatalf("expected still false after idempotent call, got is_wishlisted=%v", got)
	}
}

// setWishlisted sets the is_wishlisted flag for the given user_game id.
func setWishlisted(t *testing.T, ugID string, v bool) {
	t.Helper()
	if _, err := testDB.NewRaw(`UPDATE user_games SET is_wishlisted = ? WHERE id = ?`, v, ugID).Exec(context.Background()); err != nil {
		t.Fatalf("setWishlisted: %v", err)
	}
}

// isWishlisted returns the is_wishlisted flag for the given user_game id.
func isWishlisted(t *testing.T, ugID string) bool {
	t.Helper()
	var v bool
	if err := testDB.NewRaw(`SELECT is_wishlisted FROM user_games WHERE id = ?`, ugID).Scan(context.Background(), &v); err != nil {
		t.Fatalf("isWishlisted: %v", err)
	}
	return v
}

// addPlatformRow inserts a user_game_platforms row for the given user_game id.
func addPlatformRow(t *testing.T, ugID, platform, storefront string) {
	t.Helper()
	if _, err := testDB.NewRaw(
		`INSERT INTO user_game_platforms (id, user_game_id, platform, storefront) VALUES (?, ?, ?, ?)`,
		uuid.NewString(), ugID, platform, storefront).Exec(context.Background()); err != nil {
		t.Fatalf("addPlatformRow: %v", err)
	}
}

// loadWishlistFlag loads the is_wishlisted column for the given user_game id.
func loadWishlistFlag(t *testing.T, ugID string) bool {
	t.Helper()
	var ug models.UserGame
	if err := testDB.NewSelect().Model(&ug).Where("id = ?", ugID).Scan(context.Background()); err != nil {
		t.Fatalf("load user_game: %v", err)
	}
	return ug.IsWishlisted
}
