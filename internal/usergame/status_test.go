package usergame

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func boolptrUG(b bool) *bool { return &b }

func TestUpdateFields_FinishedRemovesFromPools(t *testing.T) {
	truncateAllTables(t)
	u := seedUser(t)
	ugID := seedUserGame(t, u, 700)
	// create a pool and add the game
	poolID := uuid.NewString()
	_, _ = testDB.NewRaw(`INSERT INTO pools (id, user_id, name, created_at, updated_at) VALUES (?, ?, 'P', now(), now())`, poolID, u).Exec(context.Background())
	_, _ = testDB.NewRaw(`INSERT INTO pool_games (id, pool_id, user_game_id, created_at) VALUES (gen_random_uuid(), ?, ?, now())`, poolID, ugID).Exec(context.Background())
	if err := UpdateFields(context.Background(), testDB, UpdateFieldsParams{UserID: u, UserGameID: ugID, PlayStatus: strptr("completed")}); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = testDB.NewRaw(`SELECT count(*) FROM pool_games WHERE user_game_id = ?`, ugID).Scan(context.Background(), &n)
	if n != 0 {
		t.Errorf("expected pool membership removed on finish, got %d", n)
	}
}

func TestUpdateFields_NotFound(t *testing.T) {
	truncateAllTables(t)
	u := seedUser(t)
	if err := UpdateFields(context.Background(), testDB, UpdateFieldsParams{UserID: u, UserGameID: uuid.NewString(), IsLoved: boolptrUG(true)}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
