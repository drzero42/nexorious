package librarysmells

import (
	"context"
	"testing"
)

func TestDetectOrphanGame(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := seedUser(t)

	orphan := seedUserGame(t, userID, 1) // no platform, not wishlisted → flags
	clean := seedUserGame(t, userID, 2)  // has a platform → clean
	seedPlatform(t, clean, "pc-windows", "steam")
	wish := seedUserGame(t, userID, 3) // wishlisted, no platform → clean
	if _, err := testDB.NewRaw(`UPDATE user_games SET is_wishlisted = true WHERE id = ?`, wish).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	ignored := seedUserGame(t, userID, 4) // orphan but dismissed → suppressed
	ignore(t, userID, ignored, "orphan-game")

	items, err := detectOrphanGame(ctx, testDB, userID)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	got := flaggedIDs(items)
	if !got[orphan] {
		t.Error("orphan game should flag")
	}
	if got[clean] {
		t.Error("game with a platform must not flag")
	}
	if got[wish] {
		t.Error("wishlisted game with no platform must not flag")
	}
	if got[ignored] {
		t.Error("dismissed game must be suppressed")
	}
}

func TestDetectStorefrontLess(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := seedUser(t)

	flagged := seedUserGame(t, userID, 1)
	seedPlatform(t, flagged, "pc-windows", "") // storefront NULL → flags, suggests default
	clean := seedUserGame(t, userID, 2)
	seedPlatform(t, clean, "pc-windows", "steam")

	items, err := detectStorefrontLess(ctx, testDB, userID)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if !flaggedIDs(items)[flagged] {
		t.Fatal("storefront-less platform should flag")
	}
	if flaggedIDs(items)[clean] {
		t.Fatal("platform with a storefront must not flag")
	}
	// suggested_storefront comes from platforms.default_storefront (pc-windows seeds one).
	for _, it := range items {
		if it.UserGameID == flagged && it.SuggestedStorefront == nil {
			t.Error("expected a suggested storefront for pc-windows")
		}
	}
}

func TestDetectMissingOwnership(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := seedUser(t)

	flagged := seedUserGame(t, userID, 1)
	seedPlatform(t, flagged, "pc-windows", "steam") // ownership_status NULL by default → flags
	clean := seedUserGame(t, userID, 2)
	owned := seedPlatform(t, clean, "pc-windows", "steam")
	if _, err := testDB.NewRaw(`UPDATE user_game_platforms SET ownership_status = 'owned' WHERE id = ?`, owned).Exec(ctx); err != nil {
		t.Fatal(err)
	}

	items, err := detectMissingOwnership(ctx, testDB, userID)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if !flaggedIDs(items)[flagged] {
		t.Fatal("missing ownership_status should flag")
	}
	if flaggedIDs(items)[clean] {
		t.Fatal("row with ownership_status must not flag")
	}
}

func TestDetectInvalidStorefront(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := seedUser(t)

	// pc-windows + nintendo-eshop is not a valid platform_storefronts pair.
	flagged := seedUserGame(t, userID, 1)
	seedPlatform(t, flagged, "pc-windows", "nintendo-eshop")
	clean := seedUserGame(t, userID, 2)
	seedPlatform(t, clean, "pc-windows", "steam") // valid pair
	nullRow := seedUserGame(t, userID, 3)
	seedPlatform(t, nullRow, "pc-windows", "") // NULL storefront → NOT this check's concern

	items, err := detectInvalidStorefront(ctx, testDB, userID)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	got := flaggedIDs(items)
	if !got[flagged] {
		t.Error("invalid (platform, storefront) pair should flag")
	}
	if got[clean] {
		t.Error("valid pair must not flag")
	}
	if got[nullRow] {
		t.Error("NULL storefront must not flag here (covered by storefront-less)")
	}
}

func TestDetectImpossibleAcquiredDate(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := seedUser(t)

	setAcquired := func(platformID, date string) {
		if _, err := testDB.NewRaw(`UPDATE user_game_platforms SET acquired_date = ?::date WHERE id = ?`, date, platformID).Exec(ctx); err != nil {
			t.Fatal(err)
		}
	}
	setRelease := func(gameID int32, date string) {
		if _, err := testDB.NewRaw(`UPDATE games SET release_date = ?::date WHERE id = ?`, date, gameID).Exec(ctx); err != nil {
			t.Fatal(err)
		}
	}

	// future acquired date → flags
	future := seedUserGame(t, userID, 1)
	setAcquired(seedPlatform(t, future, "pc-windows", "steam"), "2099-01-01")

	// acquired before release → flags
	preRelease := seedUserGame(t, userID, 2)
	setRelease(2, "2020-01-01")
	setAcquired(seedPlatform(t, preRelease, "pc-windows", "steam"), "2019-01-01")

	// acquired after release, not future → clean
	ok := seedUserGame(t, userID, 3)
	setRelease(3, "2020-01-01")
	setAcquired(seedPlatform(t, ok, "pc-windows", "steam"), "2021-01-01")

	// acquired before "now" but game has no release date → clean (only future arm applies)
	noRelease := seedUserGame(t, userID, 4)
	setAcquired(seedPlatform(t, noRelease, "pc-windows", "steam"), "2010-01-01")

	items, err := detectImpossibleAcquiredDate(ctx, testDB, userID)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	got := flaggedIDs(items)
	if !got[future] {
		t.Error("future acquired date should flag")
	}
	if !got[preRelease] {
		t.Error("acquired-before-release should flag")
	}
	if got[ok] {
		t.Error("valid acquired date must not flag")
	}
	if got[noRelease] {
		t.Error("old date with no release_date must not flag")
	}
}
