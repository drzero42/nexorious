package usergame

import (
	"context"
	"database/sql"
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

func TestAcquire_SyncFromSourcePersistedCorrectly(t *testing.T) {
	truncateAllTables(t)
	u := seedUser(t)
	seedGame(t, 500, "Hollow Knight")
	seedGame(t, 501, "Disco Elysium")

	// Platform acquired with SyncFromSource=true (sync path) should persist true.
	resSync, err := Acquire(context.Background(), testDB, AcquireParams{
		UserID: u, GameID: 500, Mode: ModeUpsert,
		Platforms: []PlatformInput{{
			Platform: strptr("pc-windows"), Storefront: strptr("steam"),
			HoursPlayed: fptr(2), SyncFromSource: true,
		}},
	})
	if err != nil {
		t.Fatalf("sync acquire: %v", err)
	}
	var syncFlag bool
	if err := testDB.NewRaw(
		`SELECT sync_from_source FROM user_game_platforms WHERE user_game_id = ?`,
		resSync.UserGameID,
	).Scan(context.Background(), &syncFlag); err != nil {
		t.Fatalf("fetch sync_from_source: %v", err)
	}
	if !syncFlag {
		t.Error("sync_from_source should be true for sync-acquired platform")
	}

	// Platform acquired without SyncFromSource (REST/import path) should persist false.
	resREST, err := Acquire(context.Background(), testDB, AcquireParams{
		UserID: u, GameID: 501, Mode: ModeCreate,
		Platforms: []PlatformInput{{
			Platform: strptr("pc-windows"), Storefront: strptr("steam"),
			HoursPlayed: fptr(1),
		}},
	})
	if err != nil {
		t.Fatalf("rest acquire: %v", err)
	}
	var restFlag bool
	if err := testDB.NewRaw(
		`SELECT sync_from_source FROM user_game_platforms WHERE user_game_id = ?`,
		resREST.UserGameID,
	).Scan(context.Background(), &restFlag); err != nil {
		t.Fatalf("fetch sync_from_source (rest): %v", err)
	}
	if restFlag {
		t.Error("sync_from_source should be false for REST/import-acquired platform")
	}
}

func TestAcquire_NilHoursPlayedPersistsNull(t *testing.T) {
	truncateAllTables(t)
	u := seedUser(t)
	seedGame(t, 600, "Celeste")
	res, err := Acquire(context.Background(), testDB, AcquireParams{
		UserID: u, GameID: 600, Mode: ModeUpsert,
		Platforms: []PlatformInput{{Platform: strptr("pc-windows"), Storefront: strptr("steam"), HoursPlayed: nil}},
	})
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	var hp sql.NullFloat64
	if err := testDB.NewRaw(
		`SELECT hours_played FROM user_game_platforms WHERE user_game_id = ?`, res.UserGameID,
	).Scan(context.Background(), &hp); err != nil {
		t.Fatalf("scan hours_played: %v", err)
	}
	if hp.Valid {
		t.Errorf("expected NULL hours_played, got %v", hp.Float64)
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
