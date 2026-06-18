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
