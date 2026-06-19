package usergame

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// An invalid platform slug must surface as ErrUnprocessable (→ 422) naming the
// offending value, never as a raw foreign-key violation (→ 500). See issue #1100.
func TestAcquire_InvalidPlatformSlug(t *testing.T) {
	truncateAllTables(t)
	u := seedUser(t)
	seedGame(t, 1100, "Bad Platform")

	_, err := Acquire(context.Background(), testDB, AcquireParams{
		UserID: u, GameID: 1100, Mode: ModeCreate,
		Platforms: []PlatformInput{{Platform: strptr("PS5"), Storefront: strptr("steam")}},
	})
	if !errors.Is(err, ErrUnprocessable) {
		t.Fatalf("expected ErrUnprocessable, got %v", err)
	}
	if !strings.Contains(err.Error(), `"PS5"`) {
		t.Errorf("expected message to name the bad slug, got %q", err.Error())
	}
}

func TestAcquire_InvalidStorefrontSlug(t *testing.T) {
	truncateAllTables(t)
	u := seedUser(t)
	seedGame(t, 1101, "Bad Storefront")

	_, err := Acquire(context.Background(), testDB, AcquireParams{
		UserID: u, GameID: 1101, Mode: ModeCreate,
		Platforms: []PlatformInput{{Platform: strptr("pc-windows"), Storefront: strptr("not-a-store")}},
	})
	if !errors.Is(err, ErrUnprocessable) {
		t.Fatalf("expected ErrUnprocessable, got %v", err)
	}
	if !strings.Contains(err.Error(), `"not-a-store"`) {
		t.Errorf("expected message to name the bad storefront, got %q", err.Error())
	}
}

func TestAddPlatform_InvalidPlatformSlug(t *testing.T) {
	truncateAllTables(t)
	u := seedUser(t)
	ugID := seedUserGame(t, u, 1102)

	_, err := AddPlatform(context.Background(), testDB, AddPlatformParams{
		UserID: u, UserGameID: ugID,
		Platform: PlatformInput{Platform: strptr("PS5"), Storefront: strptr("steam")},
	})
	if !errors.Is(err, ErrUnprocessable) {
		t.Fatalf("expected ErrUnprocessable, got %v", err)
	}
}

func TestAddPlatformBulk_InvalidPlatformSlug(t *testing.T) {
	truncateAllTables(t)
	u := seedUser(t)
	ugID := seedUserGame(t, u, 1103)

	_, err := AddPlatformBulk(context.Background(), testDB, BulkAddPlatformParams{
		UserID: u, UserGameIDs: []string{ugID},
		Platform: PlatformInput{Platform: strptr("PS5")},
	})
	if !errors.Is(err, ErrUnprocessable) {
		t.Fatalf("expected ErrUnprocessable, got %v", err)
	}
}

func TestMoveToLibrary_InvalidPlatformSlug(t *testing.T) {
	truncateAllTables(t)
	u := seedUser(t)
	seedGame(t, 1104, "Wishlisted")
	var ugID string
	if err := testDB.NewRaw(
		`INSERT INTO user_games (id, user_id, game_id, is_wishlisted, play_status, created_at, updated_at)
		 VALUES (gen_random_uuid(), ?, 1104, true, 'not_started', now(), now()) RETURNING id`, u,
	).Scan(context.Background(), &ugID); err != nil {
		t.Fatal(err)
	}

	_, err := MoveToLibrary(context.Background(), testDB, MoveParams{
		UserID: u, UserGameID: ugID,
		Platforms: []PlatformInput{{Platform: strptr("PS5")}},
	})
	if !errors.Is(err, ErrUnprocessable) {
		t.Fatalf("expected ErrUnprocessable, got %v", err)
	}
}

func TestUpdatePlatform_InvalidStorefrontSlug(t *testing.T) {
	truncateAllTables(t)
	u := seedUser(t)
	ugID := seedUserGame(t, u, 1105)
	if _, err := AddPlatform(context.Background(), testDB, AddPlatformParams{
		UserID: u, UserGameID: ugID,
		Platform: PlatformInput{Platform: strptr("pc-windows"), Storefront: strptr("steam")},
	}); err != nil {
		t.Fatal(err)
	}
	var pid string
	if err := testDB.NewRaw(`SELECT id FROM user_game_platforms WHERE user_game_id = ?`, ugID).Scan(context.Background(), &pid); err != nil {
		t.Fatal(err)
	}

	err := UpdatePlatform(context.Background(), testDB, UpdatePlatformParams{
		UserID: u, UserGameID: ugID, PlatformID: pid,
		Fields: PlatformInput{Storefront: strptr("not-a-store")},
	})
	if !errors.Is(err, ErrUnprocessable) {
		t.Fatalf("expected ErrUnprocessable, got %v", err)
	}
}
