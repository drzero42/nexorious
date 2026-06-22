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

func TestDetectWishlistedYetOwned(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := seedUser(t)

	setWish := func(ugID string) {
		if _, err := testDB.NewRaw(`UPDATE user_games SET is_wishlisted = true WHERE id = ?`, ugID).Exec(ctx); err != nil {
			t.Fatal(err)
		}
	}

	flagged := seedUserGame(t, userID, 1) // wishlisted + has a platform → flags
	setWish(flagged)
	seedPlatform(t, flagged, "pc-windows", "steam")

	pureWish := seedUserGame(t, userID, 2) // wishlisted, no platform → clean
	setWish(pureWish)

	owned := seedUserGame(t, userID, 3) // owned, not wishlisted → clean
	seedPlatform(t, owned, "pc-windows", "steam")

	items, err := detectWishlistedYetOwned(ctx, testDB, userID)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	got := flaggedIDs(items)
	if !got[flagged] {
		t.Error("wishlisted-yet-owned should flag")
	}
	if got[pureWish] {
		t.Error("pure wishlist entry must not flag")
	}
	if got[owned] {
		t.Error("owned non-wishlisted game must not flag")
	}
}

// platformWithHours seeds a platform row carrying hours_played.
func platformWithHours(t *testing.T, ugID string, hours float64) {
	t.Helper()
	id := seedPlatform(t, ugID, "pc-windows", "steam")
	if _, err := testDB.NewRaw(`UPDATE user_game_platforms SET hours_played = ? WHERE id = ?`, hours, id).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func setStatus(t *testing.T, ugID, status string) {
	t.Helper()
	if _, err := testDB.NewRaw(`UPDATE user_games SET play_status = ? WHERE id = ?`, status, ugID).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func setHLTB(t *testing.T, gameID int32, hours float64) {
	t.Helper()
	if _, err := testDB.NewRaw(`UPDATE games SET howlongtobeat_main = ? WHERE id = ?`, hours, gameID).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestDetectBeatButNotMarked(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := seedUser(t)

	beaten := seedUserGame(t, userID, 1) // 12h played, HLTB 10, in_progress → flags
	setStatus(t, beaten, "in_progress")
	setHLTB(t, 1, 10)
	platformWithHours(t, beaten, 12)

	noHLTB := seedUserGame(t, userID, 2) // lots of hours, HLTB NULL → silent
	setStatus(t, noHLTB, "in_progress")
	platformWithHours(t, noHLTB, 99)

	finished := seedUserGame(t, userID, 3) // already completed → not flagged
	setStatus(t, finished, "completed")
	setHLTB(t, 3, 10)
	platformWithHours(t, finished, 12)

	items, err := detectBeatButNotMarked(ctx, testDB, userID)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	got := flaggedIDs(items)
	if !got[beaten] {
		t.Error("beaten in-progress game should flag")
	}
	if got[noHLTB] {
		t.Error("game with NULL howlongtobeat_main must stay silent")
	}
	if got[finished] {
		t.Error("already-completed game must not flag")
	}
}

func TestDetectPlayedButNotStarted_Precedence(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := seedUser(t)

	// not_started, 2h, no HLTB → played-but-not-started
	played := seedUserGame(t, userID, 1)
	setStatus(t, played, "not_started")
	platformWithHours(t, played, 2)

	// not_started, 12h, HLTB 10 → belongs to beat-but-not-marked, NOT this check
	beaten := seedUserGame(t, userID, 2)
	setStatus(t, beaten, "not_started")
	setHLTB(t, 2, 10)
	platformWithHours(t, beaten, 12)

	// not_started, 0.2h → below threshold, clean
	barely := seedUserGame(t, userID, 3)
	setStatus(t, barely, "not_started")
	platformWithHours(t, barely, 0.2)

	items, err := detectPlayedButNotStarted(ctx, testDB, userID)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	got := flaggedIDs(items)
	if !got[played] {
		t.Error("played not_started game should flag")
	}
	if got[beaten] {
		t.Error("beaten game must defer to beat-but-not-marked (precedence)")
	}
	if got[barely] {
		t.Error("under-0.5h game must not flag")
	}
}

func TestDetectInProgressUntouched(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := seedUser(t)

	untouched := seedUserGame(t, userID, 1) // in_progress, no platform hours → flags
	setStatus(t, untouched, "in_progress")
	seedPlatform(t, untouched, "pc-windows", "steam") // hours_played NULL

	touched := seedUserGame(t, userID, 2) // in_progress, 3h → clean
	setStatus(t, touched, "in_progress")
	platformWithHours(t, touched, 3)

	items, err := detectInProgressUntouched(ctx, testDB, userID)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	got := flaggedIDs(items)
	if !got[untouched] {
		t.Error("in_progress with 0 hours should flag")
	}
	if got[touched] {
		t.Error("in_progress with hours must not flag")
	}
}

func TestDetectUnratedAfterFinishing(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := seedUser(t)

	unrated := seedUserGame(t, userID, 1) // completed, no rating → flags
	setStatus(t, unrated, "completed")

	rated := seedUserGame(t, userID, 2) // completed, rated → clean
	setStatus(t, rated, "completed")
	if _, err := testDB.NewRaw(`UPDATE user_games SET personal_rating = 8 WHERE id = ?`, rated).Exec(ctx); err != nil {
		t.Fatal(err)
	}

	dropped := seedUserGame(t, userID, 3) // dropped is NOT in this check's finished set → clean
	setStatus(t, dropped, "dropped")

	items, err := detectUnratedAfterFinishing(ctx, testDB, userID)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	got := flaggedIDs(items)
	if !got[unrated] {
		t.Error("completed unrated game should flag")
	}
	if got[rated] {
		t.Error("rated game must not flag")
	}
	if got[dropped] {
		t.Error("dropped game must not flag (only completed/mastered/dominated)")
	}
}

func TestRegistryComplete(t *testing.T) {
	if len(Registry()) != 10 {
		t.Fatalf("expected 10 checks, got %d", len(Registry()))
	}
	seen := map[string]bool{}
	for _, c := range Registry() {
		if c.ID == "" || c.Title == "" || c.Description == "" {
			t.Errorf("check %q missing metadata", c.ID)
		}
		if seen[c.ID] {
			t.Errorf("duplicate check id %q", c.ID)
		}
		seen[c.ID] = true
		if c.Tier != TierInconsistency && c.Tier != TierNudge {
			t.Errorf("check %q has invalid tier %q", c.ID, c.Tier)
		}
		if c.Detect == nil {
			t.Errorf("check %q has nil Detect", c.ID)
		}
	}
}
