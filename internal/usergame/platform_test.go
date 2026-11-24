package usergame

import (
	"context"
	"errors"
	"testing"
)

func TestUpdatePlatform_PromotesOnHours(t *testing.T) {
	truncateAllTables(t)
	u := seedUser(t)
	ugID := seedUserGame(t, u, 600) // play_status defaults not_started
	// add a platform with 0 hours via AddPlatform
	_, _ = AddPlatform(context.Background(), testDB, AddPlatformParams{UserID: u, UserGameID: ugID, Platform: PlatformInput{Platform: strptr("pc-windows"), Storefront: strptr("steam")}})
	var pid string
	_ = testDB.NewRaw(`SELECT id FROM user_game_platforms WHERE user_game_id = ?`, ugID).Scan(context.Background(), &pid)
	if err := UpdatePlatform(context.Background(), testDB, UpdatePlatformParams{UserID: u, UserGameID: ugID, PlatformID: pid, Fields: PlatformInput{HoursPlayed: fptr(4)}}); err != nil {
		t.Fatal(err)
	}
	if ug := fetchUG(t, ugID); ug.PlayStatus == nil || *ug.PlayStatus != "in_progress" {
		t.Errorf("expected promote, got %v", ug.PlayStatus)
	}
}

func TestRemovePlatform_NotFound(t *testing.T) {
	truncateAllTables(t)
	u := seedUser(t)
	ugID := seedUserGame(t, u, 601)
	if err := RemovePlatform(context.Background(), testDB, RemovePlatformParams{UserID: u, UserGameID: ugID, PlatformID: "00000000-0000-0000-0000-000000000000"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestRemoveStorefrontPlatforms_ScopedToUserAndStorefront(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	u := seedUser(t)
	other := seedUser(t)
	ugID := seedUserGame(t, u, 610)
	otherUG := seedUserGame(t, other, 611)

	insert := func(id, ug string, storefront *string) {
		t.Helper()
		if _, err := testDB.NewRaw(
			`INSERT INTO user_game_platforms (id, user_game_id, platform, storefront) VALUES (?, ?, 'pc-windows', ?)`,
			id, ug, storefront,
		).Exec(ctx); err != nil {
			t.Fatalf("insert %s: %v", id, err)
		}
	}
	insert("ugp-steam", ugID, strptr("steam"))
	insert("ugp-gog", ugID, strptr("gog"))
	insert("ugp-none", ugID, nil)
	insert("ugp-other-steam", otherUG, strptr("steam"))

	if err := RemoveStorefrontPlatforms(ctx, testDB, u, "steam"); err != nil {
		t.Fatal(err)
	}

	var left []string
	if err := testDB.NewRaw(`SELECT id FROM user_game_platforms ORDER BY id`).Scan(ctx, &left); err != nil {
		t.Fatal(err)
	}
	want := []string{"ugp-gog", "ugp-none", "ugp-other-steam"}
	if len(left) != len(want) {
		t.Fatalf("remaining rows = %v, want %v", left, want)
	}
	for i := range want {
		if left[i] != want[i] {
			t.Fatalf("remaining rows = %v, want %v", left, want)
		}
	}
}
