package usergame

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/drzero42/nexorious/internal/db/models"
)

func i32ptr(i int32) *int32 { return &i }

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

func TestAddPlatform_ConflictOnDuplicate(t *testing.T) {
	truncateAllTables(t)
	u := seedUser(t)
	ugID := seedUserGame(t, u, 500)
	in := PlatformInput{Platform: strptr("pc-windows"), Storefront: strptr("steam")}
	if _, err := AddPlatform(context.Background(), testDB, AddPlatformParams{UserID: u, UserGameID: ugID, Platform: in}); err != nil {
		t.Fatal(err)
	}
	_, err := AddPlatform(context.Background(), testDB, AddPlatformParams{UserID: u, UserGameID: ugID, Platform: in})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestAddPlatform_NotFoundForOtherUser(t *testing.T) {
	truncateAllTables(t)
	owner := seedUser(t)
	other := seedUser(t)
	ugID := seedUserGame(t, owner, 501)
	_, err := AddPlatform(context.Background(), testDB, AddPlatformParams{UserID: other, UserGameID: ugID, Platform: PlatformInput{Platform: strptr("pc-windows"), Storefront: strptr("gog")}})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestMoveToLibrary_RequiresWishlisted(t *testing.T) {
	truncateAllTables(t)
	u := seedUser(t)
	ugID := seedUserGame(t, u, 502) // not wishlisted
	_, err := MoveToLibrary(context.Background(), testDB, MoveParams{UserID: u, UserGameID: ugID, Platforms: []PlatformInput{{Platform: strptr("pc-windows"), Storefront: strptr("steam")}}})
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
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

func TestAcquire_PersistsAndRefreshesAchievements(t *testing.T) {
	truncateAllTables(t)
	u := seedUser(t)
	seedGame(t, 700, "Hades II")

	two, one := 2, 1
	res, err := Acquire(context.Background(), testDB, AcquireParams{
		UserID: u, GameID: 700, Mode: ModeUpsert,
		Platforms: []PlatformInput{{
			Platform: strptr("pc-windows"), Storefront: strptr("steam"),
			HoursPlayed: fptr(3), SyncFromSource: true,
			AchievementsUnlocked: &one, AchievementsTotal: &two,
		}},
	})
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}

	var gotUnlocked, gotTotal *int
	if err := testDB.NewRaw(
		`SELECT achievements_unlocked, achievements_total FROM user_game_platforms WHERE user_game_id = ?`,
		res.UserGameID,
	).Scan(context.Background(), &gotUnlocked, &gotTotal); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if gotUnlocked == nil || *gotUnlocked != 1 || gotTotal == nil || *gotTotal != 2 {
		t.Fatalf("after insert: got %v/%v, want 1/2", gotUnlocked, gotTotal)
	}

	// Re-sync with nil achievements (success=false this round) must preserve counts.
	if _, err := Acquire(context.Background(), testDB, AcquireParams{
		UserID: u, GameID: 700, Mode: ModeUpsert,
		Platforms: []PlatformInput{{
			Platform: strptr("pc-windows"), Storefront: strptr("steam"),
			HoursPlayed: fptr(4), SyncFromSource: true,
			AchievementsUnlocked: nil, AchievementsTotal: nil,
		}},
	}); err != nil {
		t.Fatalf("re-acquire: %v", err)
	}
	if err := testDB.NewRaw(
		`SELECT achievements_unlocked, achievements_total FROM user_game_platforms WHERE user_game_id = ?`,
		res.UserGameID,
	).Scan(context.Background(), &gotUnlocked, &gotTotal); err != nil {
		t.Fatalf("scan after re-acquire: %v", err)
	}
	if gotUnlocked == nil || *gotUnlocked != 1 || gotTotal == nil || *gotTotal != 2 {
		t.Fatalf("after nil re-sync: got %v/%v, want preserved 1/2", gotUnlocked, gotTotal)
	}
}

// ModeImport persists the caller-supplied meta and timestamps on a fresh insert,
// then on a re-acquire (conflict) leaves the existing row — meta AND updated_at —
// fully intact while still merging the new platform. This is the invariant the
// import workers rely on now that row-creation routes through Acquire (#1068).
func TestAcquire_ModeImportPersistsMetaAndPreservesOnConflict(t *testing.T) {
	truncateAllTables(t)
	u := seedUser(t)
	seedGame(t, 500, "Outer Wilds")

	created := time.Date(2023, 1, 15, 10, 0, 0, 0, time.UTC)
	updated := time.Date(2023, 6, 20, 12, 0, 0, 0, time.UTC)

	// Fresh insert: meta + timestamps land on the new row.
	r1, err := Acquire(context.Background(), testDB, AcquireParams{
		UserID: u, GameID: 500, Mode: ModeImport,
		PlayStatus: strptr("completed"), PersonalRating: i32ptr(5),
		IsLoved: true, PersonalNotes: strptr("masterpiece"),
		CreatedAt: &created, UpdatedAt: &updated,
	})
	if err != nil {
		t.Fatalf("import create: %v", err)
	}
	if !r1.Created {
		t.Fatal("expected Created=true on fresh insert")
	}
	ug := fetchUG(t, r1.UserGameID)
	if ug.PlayStatus == nil || *ug.PlayStatus != "completed" {
		t.Errorf("play_status = %v, want completed", ug.PlayStatus)
	}
	if ug.PersonalRating == nil || *ug.PersonalRating != 5 || !ug.IsLoved ||
		ug.PersonalNotes == nil || *ug.PersonalNotes != "masterpiece" {
		t.Errorf("meta not persisted: %+v", ug)
	}
	if !ug.CreatedAt.Equal(created) || !ug.UpdatedAt.Equal(updated) {
		t.Errorf("timestamps = (%v, %v), want (%v, %v)", ug.CreatedAt, ug.UpdatedAt, created, updated)
	}

	// Re-acquire (conflict): existing meta + updated_at preserved; platform merged.
	r2, err := Acquire(context.Background(), testDB, AcquireParams{
		UserID: u, GameID: 500, Mode: ModeImport,
		PlayStatus: strptr("not_started"), PersonalRating: i32ptr(1),
		IsLoved: false, PersonalNotes: strptr("changed"),
		CreatedAt: &created, UpdatedAt: &created, // different updated_at; must be ignored
		Platforms: []PlatformInput{{Platform: strptr("pc-windows"), Storefront: strptr("steam")}},
	})
	if err != nil {
		t.Fatalf("import merge: %v", err)
	}
	if r2.Created {
		t.Error("expected Created=false on conflict")
	}
	if r2.UserGameID != r1.UserGameID {
		t.Error("conflict should return the existing user_game id")
	}
	ug = fetchUG(t, r1.UserGameID)
	if ug.PlayStatus == nil || *ug.PlayStatus != "completed" {
		t.Errorf("play_status clobbered on merge: %v", ug.PlayStatus)
	}
	if ug.PersonalRating == nil || *ug.PersonalRating != 5 || !ug.IsLoved ||
		ug.PersonalNotes == nil || *ug.PersonalNotes != "masterpiece" {
		t.Errorf("meta clobbered on merge: %+v", ug)
	}
	if !ug.UpdatedAt.Equal(updated) {
		t.Errorf("updated_at clobbered on merge: %v, want %v", ug.UpdatedAt, updated)
	}
	var n int
	if err := testDB.NewRaw(`SELECT count(*) FROM user_game_platforms WHERE user_game_id = ?`, r1.UserGameID).Scan(context.Background(), &n); err != nil {
		t.Fatalf("count platforms: %v", err)
	}
	if n != 1 {
		t.Errorf("expected platform merged, got %d platforms", n)
	}
}
