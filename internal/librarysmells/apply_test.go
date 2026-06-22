package librarysmells

import (
	"context"
	"testing"
)

func playStatusOf(t *testing.T, ugID string) string {
	t.Helper()
	var s *string
	if err := testDB.NewRaw(`SELECT play_status FROM user_games WHERE id = ?`, ugID).Scan(context.Background(), &s); err != nil {
		t.Fatalf("playStatusOf: %v", err)
	}
	if s == nil {
		return ""
	}
	return *s
}

func TestApplyClearWishlist(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := seedUser(t)

	flagged := seedUserGame(t, userID, 1)
	if _, err := testDB.NewRaw(`UPDATE user_games SET is_wishlisted = true WHERE id = ?`, flagged).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	seedPlatform(t, flagged, "pc-windows", "steam")

	check, _ := Lookup("wishlisted-yet-owned")
	if !check.AutoFixable || check.Apply == nil {
		t.Fatal("wishlisted-yet-owned must be auto-fixable")
	}
	applied, skipped, err := check.Apply(ctx, testDB, userID, []string{flagged})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if applied != 1 || skipped != 0 {
		t.Fatalf("expected applied=1 skipped=0, got applied=%d skipped=%d", applied, skipped)
	}
	var wl bool
	if err := testDB.NewRaw(`SELECT is_wishlisted FROM user_games WHERE id = ?`, flagged).Scan(ctx, &wl); err != nil {
		t.Fatal(err)
	}
	if wl {
		t.Error("wishlist flag should be cleared")
	}
}

func TestApplyInProgressUntouched(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := seedUser(t)

	flagged := seedUserGame(t, userID, 1)
	setStatus(t, flagged, "in_progress")
	seedPlatform(t, flagged, "pc-windows", "steam") // 0 hours

	check, _ := Lookup("in-progress-untouched")
	if !check.AutoFixable || check.Apply == nil {
		t.Fatal("in-progress-untouched must be auto-fixable")
	}
	applied, skipped, err := check.Apply(ctx, testDB, userID, []string{flagged})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if applied != 1 || skipped != 0 {
		t.Fatalf("expected applied=1 skipped=0, got applied=%d skipped=%d", applied, skipped)
	}
	if got := playStatusOf(t, flagged); got != "not_started" {
		t.Errorf("expected not_started, got %q", got)
	}
}
