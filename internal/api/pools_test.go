package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// insertPool inserts a pool row directly and returns its ID.
func insertPool(t *testing.T, db *bun.DB, id, userID, name string, position int) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO pools (id, user_id, name, position) VALUES (?, ?, ?, ?)`,
		id, userID, name, position,
	)
	if err != nil {
		t.Fatalf("insertPool: %v", err)
	}
}

// insertPoolGame inserts a pool_games membership row directly. position nil = Candidate.
func insertPoolGame(t *testing.T, db *bun.DB, poolID, userGameID string, position *int) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO pool_games (id, pool_id, user_game_id, position) VALUES (?, ?, ?, ?)`,
		uuid.NewString(), poolID, userGameID, position,
	)
	if err != nil {
		t.Fatalf("insertPoolGame: %v", err)
	}
}

// poolGameCount returns how many pool_games rows reference a user_game.
func poolGameCount(t *testing.T, db *bun.DB, userGameID string) int {
	t.Helper()
	n, err := db.NewSelect().Table("pool_games").
		Where("user_game_id = ?", userGameID).Count(context.Background())
	if err != nil {
		t.Fatalf("poolGameCount: %v", err)
	}
	return n
}

func TestCompletionRemovesFromPools(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID, token := setupUserGamesUser(t, testDB, e, "complete")

	t.Run("single update to finished removes from all pools", func(t *testing.T) {
		gameID := insertTestGame(t, testDB, "Done Single")
		insertTestUserGame(t, testDB, "ug-done-1", userID, int(gameID))
		insertPool(t, testDB, "pool-a", userID, "Pool A", 0)
		insertPool(t, testDB, "pool-b", userID, "Pool B", 1)
		insertPoolGame(t, testDB, "pool-a", "ug-done-1", nil)
		pos := 0
		insertPoolGame(t, testDB, "pool-b", "ug-done-1", &pos)

		if poolGameCount(t, testDB, "ug-done-1") != 2 {
			t.Fatalf("setup: expected 2 memberships")
		}

		rec := putJSONAuth(t, e, "/api/user-games/ug-done-1",
			map[string]any{"play_status": "completed"}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if got := poolGameCount(t, testDB, "ug-done-1"); got != 0 {
			t.Fatalf("expected 0 memberships after completion, got %d", got)
		}
	})

	t.Run("single update to eligible status keeps memberships", func(t *testing.T) {
		gameID := insertTestGame(t, testDB, "Still Playing")
		insertTestUserGame(t, testDB, "ug-play-1", userID, int(gameID))
		insertPool(t, testDB, "pool-c", userID, "Pool C", 2)
		insertPoolGame(t, testDB, "pool-c", "ug-play-1", nil)

		rec := putJSONAuth(t, e, "/api/user-games/ug-play-1",
			map[string]any{"play_status": "in_progress"}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if got := poolGameCount(t, testDB, "ug-play-1"); got != 1 {
			t.Fatalf("expected membership kept, got %d", got)
		}
	})

	t.Run("dropped also removes", func(t *testing.T) {
		gameID := insertTestGame(t, testDB, "Dropped Game")
		insertTestUserGame(t, testDB, "ug-drop-1", userID, int(gameID))
		insertPool(t, testDB, "pool-d", userID, "Pool D", 3)
		insertPoolGame(t, testDB, "pool-d", "ug-drop-1", nil)

		rec := putJSONAuth(t, e, "/api/user-games/ug-drop-1",
			map[string]any{"play_status": "dropped"}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if got := poolGameCount(t, testDB, "ug-drop-1"); got != 0 {
			t.Fatalf("expected 0 after dropped, got %d", got)
		}
	})

	t.Run("bulk update to finished removes from pools", func(t *testing.T) {
		g1 := insertTestGame(t, testDB, "Bulk Done 1")
		g2 := insertTestGame(t, testDB, "Bulk Done 2")
		insertTestUserGame(t, testDB, "ug-bulk-1", userID, int(g1))
		insertTestUserGame(t, testDB, "ug-bulk-2", userID, int(g2))
		insertPool(t, testDB, "pool-e", userID, "Pool E", 4)
		insertPoolGame(t, testDB, "pool-e", "ug-bulk-1", nil)
		insertPoolGame(t, testDB, "pool-e", "ug-bulk-2", nil)

		rec := putJSONAuth(t, e, "/api/user-games/bulk-update", map[string]any{
			"ids":     []string{"ug-bulk-1", "ug-bulk-2"},
			"updates": map[string]any{"play_status": "mastered"},
		}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if poolGameCount(t, testDB, "ug-bulk-1") != 0 || poolGameCount(t, testDB, "ug-bulk-2") != 0 {
			t.Fatalf("expected both removed after bulk completion")
		}
	})
}
