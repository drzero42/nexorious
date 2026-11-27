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

// TestAddPaths_EmptySlugs guards #1501 on every path that inserts platform
// rows: an empty storefront is stored as no storefront (NULL), and a missing
// or empty platform is ErrUnprocessable. Before, the empty slug skipped
// validation and the insert failed the foreign key as a 500.
func TestAddPaths_EmptySlugs(t *testing.T) {
	ctx := context.Background()
	const gameID = 1106

	// Each add path inserts in onto a fresh user game and returns that game's id.
	addPaths := map[string]func(t *testing.T, u string, in PlatformInput) (string, error){
		"Acquire": func(t *testing.T, u string, in PlatformInput) (string, error) {
			seedGame(t, gameID, "Empty Slugs")
			res, err := Acquire(ctx, testDB, AcquireParams{UserID: u, GameID: gameID, Mode: ModeCreate, Platforms: []PlatformInput{in}})
			return res.UserGameID, err
		},
		"AddPlatform": func(t *testing.T, u string, in PlatformInput) (string, error) {
			ugID := seedUserGame(t, u, gameID)
			_, err := AddPlatform(ctx, testDB, AddPlatformParams{UserID: u, UserGameID: ugID, Platform: in})
			return ugID, err
		},
		"AddPlatformBulk": func(t *testing.T, u string, in PlatformInput) (string, error) {
			ugID := seedUserGame(t, u, gameID)
			_, err := AddPlatformBulk(ctx, testDB, BulkAddPlatformParams{UserID: u, UserGameIDs: []string{ugID}, Platform: in})
			return ugID, err
		},
		"MoveToLibrary": func(t *testing.T, u string, in PlatformInput) (string, error) {
			seedGame(t, gameID, "Empty Slugs")
			var ugID string
			if err := testDB.NewRaw(
				`INSERT INTO user_games (id, user_id, game_id, is_wishlisted, play_status, created_at, updated_at)
				 VALUES (gen_random_uuid(), ?, ?, true, 'not_started', now(), now()) RETURNING id`, u, gameID,
			).Scan(ctx, &ugID); err != nil {
				t.Fatal(err)
			}
			_, err := MoveToLibrary(ctx, testDB, MoveParams{UserID: u, UserGameID: ugID, Platforms: []PlatformInput{in}})
			return ugID, err
		},
	}

	for name, add := range addPaths {
		t.Run(name+"/empty storefront is stored as NULL", func(t *testing.T) {
			truncateAllTables(t)
			u := seedUser(t)
			ugID, err := add(t, u, PlatformInput{Platform: strptr("pc-windows"), Storefront: strptr("")})
			if err != nil {
				t.Fatalf("add: %v", err)
			}
			var total, nullStorefront int
			if err := testDB.NewRaw(
				`SELECT count(*), count(*) FILTER (WHERE storefront IS NULL) FROM user_game_platforms WHERE user_game_id = ?`, ugID,
			).Scan(ctx, &total, &nullStorefront); err != nil {
				t.Fatal(err)
			}
			if total != 1 || nullStorefront != 1 {
				t.Fatalf("rows = %d, with NULL storefront = %d; want 1 and 1", total, nullStorefront)
			}
		})
		for _, platform := range []*string{nil, strptr("")} {
			label := "nil"
			if platform != nil {
				label = "empty"
			}
			t.Run(name+"/"+label+" platform is ErrUnprocessable", func(t *testing.T) {
				truncateAllTables(t)
				u := seedUser(t)
				_, err := add(t, u, PlatformInput{Platform: platform, Storefront: strptr("steam")})
				if !errors.Is(err, ErrUnprocessable) {
					t.Fatalf("expected ErrUnprocessable, got %v", err)
				}
			})
		}
	}
}
