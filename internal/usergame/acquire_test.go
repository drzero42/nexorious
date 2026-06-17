package usergame

import (
	"context"
	"errors"
	"testing"

	"github.com/drzero42/nexorious/internal/db/models"
)

func strptr(s string) *string { return &s }
func fptr(f float64) *float64 { return &f }

// fetchStatus / fetchWishlist / countPlatforms small helpers
func fetchUG(t *testing.T, id string) models.UserGame {
	t.Helper()
	var ug models.UserGame
	if err := testDB.NewSelect().Model(&ug).Where("id = ?", id).Scan(context.Background()); err != nil {
		t.Fatalf("fetch ug: %v", err)
	}
	return ug
}

func TestAcquire_CreateInsertsPlatformsClearsWishlistPromotes(t *testing.T) {
	truncateAllTables(t)
	u := seedUser(t)
	seedGame(t, 100, "Celeste")
	// Pre-existing wishlist row to prove clear-on-acquire.
	_, err := testDB.NewRaw(`INSERT INTO user_games (id, user_id, game_id, is_wishlisted, play_status, created_at, updated_at)
		VALUES (gen_random_uuid(), ?, 100, true, 'not_started', now(), now())`, u).Exec(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	res, err := Acquire(context.Background(), testDB, AcquireParams{
		UserID: u, GameID: 100, Mode: ModeUpsert,
		Platforms: []PlatformInput{{Platform: strptr("pc-windows"), Storefront: strptr("steam"), HoursPlayed: fptr(3)}},
	})
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	ug := fetchUG(t, res.UserGameID)
	if ug.IsWishlisted {
		t.Error("wishlist should be cleared")
	}
	if ug.PlayStatus == nil || *ug.PlayStatus != "in_progress" {
		t.Errorf("expected promote to in_progress, got %v", ug.PlayStatus)
	}
}

func TestAcquire_ModeCreateConflictsOnExisting(t *testing.T) {
	truncateAllTables(t)
	u := seedUser(t)
	seedUserGame(t, u, 200)
	_, err := Acquire(context.Background(), testDB, AcquireParams{UserID: u, GameID: 200, Mode: ModeCreate})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestAcquire_UpsertIsIdempotent(t *testing.T) {
	truncateAllTables(t)
	u := seedUser(t)
	seedGame(t, 300, "Hades")
	p := AcquireParams{UserID: u, GameID: 300, Mode: ModeUpsert,
		Platforms: []PlatformInput{{Platform: strptr("pc-windows"), Storefront: strptr("steam"), HoursPlayed: fptr(5)}}}
	r1, err := Acquire(context.Background(), testDB, p)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := Acquire(context.Background(), testDB, p)
	if err != nil {
		t.Fatal(err)
	}
	if r1.UserGameID != r2.UserGameID {
		t.Error("upsert should return the same user_game")
	}
	var n int
	if err := testDB.NewRaw(`SELECT count(*) FROM user_game_platforms WHERE user_game_id = ?`, r1.UserGameID).Scan(context.Background(), &n); err != nil {
		t.Fatalf("count platforms: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 platform after idempotent re-acquire, got %d", n)
	}
}

func TestAcquire_MergeKeepsMaxHoursAndUpgradesOwnership(t *testing.T) {
	truncateAllTables(t)
	u := seedUser(t)
	seedGame(t, 400, "Tunic")
	_, err := Acquire(context.Background(), testDB, AcquireParams{UserID: u, GameID: 400, Mode: ModeUpsert,
		Platforms: []PlatformInput{{Platform: strptr("pc-windows"), Storefront: strptr("steam"), HoursPlayed: fptr(10), OwnershipStatus: strptr("subscription")}}})
	if err != nil {
		t.Fatal(err)
	}
	// Re-acquire with lower hours, higher ownership.
	res, err := Acquire(context.Background(), testDB, AcquireParams{UserID: u, GameID: 400, Mode: ModeUpsert,
		Platforms: []PlatformInput{{Platform: strptr("pc-windows"), Storefront: strptr("steam"), HoursPlayed: fptr(2), OwnershipStatus: strptr("owned")}}})
	if err != nil {
		t.Fatal(err)
	}
	var hours float64
	var owner string
	if err := testDB.NewRaw(`SELECT hours_played, ownership_status FROM user_game_platforms WHERE user_game_id = ?`, res.UserGameID).Scan(context.Background(), &hours, &owner); err != nil {
		t.Fatalf("fetch platform: %v", err)
	}
	if hours != 10 {
		t.Errorf("expected max hours 10, got %v", hours)
	}
	if owner != "owned" {
		t.Errorf("expected ownership upgraded to owned, got %v", owner)
	}
	if len(res.PlatformChanges) != 1 || !res.PlatformChanges[0].OwnershipUpgraded {
		t.Errorf("expected OwnershipUpgraded change, got %+v", res.PlatformChanges)
	}
}
