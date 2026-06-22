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
