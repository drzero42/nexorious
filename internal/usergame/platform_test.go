package usergame

import (
	"context"
	"errors"
	"testing"

	"github.com/drzero42/nexorious/internal/enum"
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

// TestUpdatePlatform_EmptyValues pins the empty-string rules (#1501): an empty
// platform, storefront or ownership status is "not provided" and leaves the
// column unchanged; only ClearStorefront clears the storefront. Writing an
// empty string would violate the foreign keys and surface as a 500.
func TestUpdatePlatform_EmptyValues(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	u := seedUser(t)
	ugID := seedUserGame(t, u, 620)
	if _, err := AddPlatform(ctx, testDB, AddPlatformParams{UserID: u, UserGameID: ugID, Platform: PlatformInput{Platform: strptr("pc-windows"), Storefront: strptr("steam")}}); err != nil {
		t.Fatal(err)
	}
	var pid string
	if err := testDB.NewRaw(`SELECT id FROM user_game_platforms WHERE user_game_id = ?`, ugID).Scan(ctx, &pid); err != nil {
		t.Fatal(err)
	}

	type platformRow struct {
		Platform        string  `bun:"platform"`
		Storefront      *string `bun:"storefront"`
		OwnershipStatus string  `bun:"ownership_status"`
	}
	load := func() platformRow {
		t.Helper()
		var row platformRow
		if err := testDB.NewRaw(`SELECT platform, storefront, ownership_status FROM user_game_platforms WHERE id = ?`, pid).Scan(ctx, &row); err != nil {
			t.Fatal(err)
		}
		return row
	}

	empty := enum.OwnershipStatus("")
	if err := UpdatePlatform(ctx, testDB, UpdatePlatformParams{UserID: u, UserGameID: ugID, PlatformID: pid, Fields: PlatformInput{
		Platform:        strptr(""),
		Storefront:      strptr(""),
		OwnershipStatus: &empty,
	}}); err != nil {
		t.Fatalf("UpdatePlatform with empty values: %v", err)
	}
	row := load()
	if row.Platform != "pc-windows" {
		t.Errorf("platform = %q, want unchanged pc-windows", row.Platform)
	}
	if row.Storefront == nil || *row.Storefront != "steam" {
		t.Errorf("storefront = %v, want unchanged steam", row.Storefront)
	}
	if row.OwnershipStatus != string(enum.OwnershipOwned) {
		t.Errorf("ownership_status = %q, want unchanged %q", row.OwnershipStatus, enum.OwnershipOwned)
	}

	if err := UpdatePlatform(ctx, testDB, UpdatePlatformParams{UserID: u, UserGameID: ugID, PlatformID: pid, Fields: PlatformInput{
		ClearStorefront: true,
	}}); err != nil {
		t.Fatalf("UpdatePlatform with ClearStorefront: %v", err)
	}
	if row := load(); row.Storefront != nil {
		t.Errorf("storefront = %q, want NULL", *row.Storefront)
	}
}
