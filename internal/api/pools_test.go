package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// mustUnmarshal decodes a JSON response body into v, failing the test on error.
func mustUnmarshal(t *testing.T, rec *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), v); err != nil {
		t.Fatalf("unmarshal (%d): %v — body: %s", rec.Code, err, rec.Body.String())
	}
}

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

func TestPoolCRUD(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	_, token := setupUserGamesUser(t, testDB, e, "crud")

	t.Run("create requires name", func(t *testing.T) {
		rec := postJSONAuth(t, e, "/api/pools", map[string]any{}, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("create and list", func(t *testing.T) {
		rec := postJSONAuth(t, e, "/api/pools", map[string]any{"name": "Backlog"}, token)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		var created struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Position int    `json:"position"`
		}
		mustUnmarshal(t, rec, &created)
		if created.ID == "" || created.Name != "Backlog" {
			t.Fatalf("unexpected create response: %+v", created)
		}

		rec2 := postJSONAuth(t, e, "/api/pools", map[string]any{"name": "Co-op"}, token)
		var created2 struct {
			Position int `json:"position"`
		}
		mustUnmarshal(t, rec2, &created2)
		if created2.Position <= created.Position {
			t.Fatalf("expected appended position > %d, got %d", created.Position, created2.Position)
		}

		listRec := getAuth(t, e, "/api/pools", token)
		var list []struct {
			Name           string `json:"name"`
			HasFilter      bool   `json:"has_filter"`
			QueueCount     int    `json:"queue_count"`
			CandidateCount int    `json:"candidate_count"`
		}
		mustUnmarshal(t, listRec, &list)
		if len(list) != 2 {
			t.Fatalf("expected 2 pools, got %d", len(list))
		}
	})

	t.Run("duplicate name conflicts", func(t *testing.T) {
		postJSONAuth(t, e, "/api/pools", map[string]any{"name": "Unique"}, token)
		rec := postJSONAuth(t, e, "/api/pools", map[string]any{"name": "Unique"}, token)
		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("empty card rejected", func(t *testing.T) {
		rec := postJSONAuth(t, e, "/api/pools", map[string]any{
			"name":   "BadFilter",
			"filter": map[string]any{"filters": []any{map[string]any{}}},
		}, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for empty card, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("unknown filter key rejected", func(t *testing.T) {
		rec := postJSONAuth(t, e, "/api/pools", map[string]any{
			"name":   "BadKey",
			"filter": map[string]any{"filters": []any{map[string]any{"nope": "x"}}},
		}, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for unknown key, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("empty filters coerced to null", func(t *testing.T) {
		rec := postJSONAuth(t, e, "/api/pools", map[string]any{
			"name":   "ManualPool",
			"filter": map[string]any{"filters": []any{}},
		}, token)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		var created struct {
			ID        string `json:"id"`
			HasFilter bool   `json:"has_filter"`
		}
		mustUnmarshal(t, rec, &created)
		if created.HasFilter {
			t.Fatalf("expected has_filter=false for empty filters")
		}
	})

	t.Run("update and delete", func(t *testing.T) {
		rec := postJSONAuth(t, e, "/api/pools", map[string]any{"name": "ToEdit"}, token)
		var created struct {
			ID string `json:"id"`
		}
		mustUnmarshal(t, rec, &created)

		newName := "Edited"
		upd := putJSONAuth(t, e, "/api/pools/"+created.ID, map[string]any{"name": newName}, token)
		if upd.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", upd.Code, upd.Body.String())
		}

		del := deleteAuth(t, e, "/api/pools/"+created.ID, token)
		if del.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d: %s", del.Code, del.Body.String())
		}

		del2 := deleteAuth(t, e, "/api/pools/"+created.ID, token)
		if del2.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", del2.Code, del2.Body.String())
		}
	})

	t.Run("reorder renumbers contiguous", func(t *testing.T) {
		truncateAllTables(t)
		_, tok := setupUserGamesUser(t, testDB, e, "reorder")
		var ids []string
		for _, n := range []string{"P1", "P2", "P3"} {
			rec := postJSONAuth(t, e, "/api/pools", map[string]any{"name": n}, tok)
			var c struct {
				ID string `json:"id"`
			}
			mustUnmarshal(t, rec, &c)
			ids = append(ids, c.ID)
		}
		reordered := []string{ids[2], ids[1], ids[0]}
		rec := postJSONAuth(t, e, "/api/pools/reorder", map[string]any{"ids": reordered}, tok)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
		}
		listRec := getAuth(t, e, "/api/pools", tok)
		var list []struct {
			ID       string `json:"id"`
			Position int    `json:"position"`
		}
		mustUnmarshal(t, listRec, &list)
		for i, want := range reordered {
			if list[i].ID != want {
				t.Fatalf("position %d: expected %s, got %s", i, want, list[i].ID)
			}
			if list[i].Position != i {
				t.Fatalf("expected contiguous position %d, got %d", i, list[i].Position)
			}
		}
	})
}

func TestPoolMembershipAndQueue(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID, token := setupUserGamesUser(t, testDB, e, "queue")

	poolRec := postJSONAuth(t, e, "/api/pools", map[string]any{"name": "Queue Pool"}, token)
	var pool struct {
		ID string `json:"id"`
	}
	mustUnmarshal(t, poolRec, &pool)

	var ugIDs []string
	for i, title := range []string{"G1", "G2", "G3"} {
		gid := insertTestGame(t, testDB, title)
		ugID := fmt.Sprintf("ug-q-%d", i)
		insertTestUserGame(t, testDB, ugID, userID, int(gid))
		ugIDs = append(ugIDs, ugID)
	}

	t.Run("add lands as candidate, idempotent", func(t *testing.T) {
		for _, ugID := range ugIDs {
			rec := postJSONAuth(t, e, "/api/pools/"+pool.ID+"/games",
				map[string]any{"user_game_id": ugID}, token)
			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
			}
		}
		rec := postJSONAuth(t, e, "/api/pools/"+pool.ID+"/games",
			map[string]any{"user_game_id": ugIDs[0]}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected idempotent 200, got %d: %s", rec.Code, rec.Body.String())
		}

		detail := getAuth(t, e, "/api/pools/"+pool.ID, token)
		var d struct {
			Queue      []map[string]any `json:"queue"`
			Candidates []map[string]any `json:"candidates"`
		}
		mustUnmarshal(t, detail, &d)
		if len(d.Candidates) != 3 || len(d.Queue) != 0 {
			t.Fatalf("expected 3 candidates / 0 queued, got %d / %d", len(d.Candidates), len(d.Queue))
		}
	})

	t.Run("add rejects non-existent user_game", func(t *testing.T) {
		rec := postJSONAuth(t, e, "/api/pools/"+pool.ID+"/games",
			map[string]any{"user_game_id": "does-not-exist"}, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("queue promote, reorder, demote in one PUT", func(t *testing.T) {
		rec := putJSONAuth(t, e, "/api/pools/"+pool.ID+"/queue",
			map[string]any{"ids": []string{ugIDs[0], ugIDs[1]}}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		detail := getAuth(t, e, "/api/pools/"+pool.ID, token)
		var d struct {
			Queue []struct {
				ID string `json:"id"`
			} `json:"queue"`
			Candidates []struct {
				ID string `json:"id"`
			} `json:"candidates"`
		}
		mustUnmarshal(t, detail, &d)
		if len(d.Queue) != 2 || d.Queue[0].ID != ugIDs[0] || d.Queue[1].ID != ugIDs[1] {
			t.Fatalf("unexpected queue: %+v", d.Queue)
		}
		if len(d.Candidates) != 1 || d.Candidates[0].ID != ugIDs[2] {
			t.Fatalf("unexpected candidates: %+v", d.Candidates)
		}

		rec2 := putJSONAuth(t, e, "/api/pools/"+pool.ID+"/queue",
			map[string]any{"ids": []string{ugIDs[1]}}, token)
		if rec2.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec2.Code, rec2.Body.String())
		}
		detail2 := getAuth(t, e, "/api/pools/"+pool.ID, token)
		var d2 struct {
			Queue []struct {
				ID string `json:"id"`
			} `json:"queue"`
		}
		mustUnmarshal(t, detail2, &d2)
		if len(d2.Queue) != 1 || d2.Queue[0].ID != ugIDs[1] {
			t.Fatalf("expected only G2 queued, got %+v", d2.Queue)
		}
	})

	t.Run("queue rejects a non-member id", func(t *testing.T) {
		gid := insertTestGame(t, testDB, "Outsider")
		insertTestUserGame(t, testDB, "ug-outsider", userID, int(gid))
		rec := putJSONAuth(t, e, "/api/pools/"+pool.ID+"/queue",
			map[string]any{"ids": []string{"ug-outsider"}}, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for non-member, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("remove membership", func(t *testing.T) {
		rec := deleteAuth(t, e, "/api/pools/"+pool.ID+"/games/"+ugIDs[2], token)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
		}
		rec2 := deleteAuth(t, e, "/api/pools/"+pool.ID+"/games/"+ugIDs[2], token)
		if rec2.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec2.Code, rec2.Body.String())
		}
	})
}

func TestPoolSuggestionView(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID, token := setupUserGamesUser(t, testDB, e, "suggest")

	rpg1 := insertTestGameWithGenre(t, testDB, "RPG One", "Role-playing (RPG)")
	rpg2 := insertTestGameWithGenre(t, testDB, "RPG Two", "Role-playing (RPG)")
	shooter := insertTestGameWithGenre(t, testDB, "Shooter", "Shooter")
	insertTestUserGame(t, testDB, "ug-rpg1", userID, int(rpg1))
	insertTestUserGame(t, testDB, "ug-rpg2", userID, int(rpg2))
	insertTestUserGame(t, testDB, "ug-shooter", userID, int(shooter))

	rpgDone := insertTestGameWithGenre(t, testDB, "RPG Done", "Role-playing (RPG)")
	insertTestUserGame(t, testDB, "ug-rpgdone", userID, int(rpgDone))
	if _, err := testDB.ExecContext(context.Background(),
		`UPDATE user_games SET play_status = 'completed' WHERE id = 'ug-rpgdone'`); err != nil {
		t.Fatalf("set completed: %v", err)
	}

	poolRec := postJSONAuth(t, e, "/api/pools", map[string]any{
		"name": "RPG Pool",
		"filter": map[string]any{
			"filters": []any{map[string]any{"genre": []string{"Role-playing (RPG)"}}},
		},
	}, token)
	var pool struct {
		ID string `json:"id"`
	}
	mustUnmarshal(t, poolRec, &pool)
	postJSONAuth(t, e, "/api/pools/"+pool.ID+"/games", map[string]any{"user_game_id": "ug-rpg1"}, token)

	rec := getAuth(t, e, "/api/user-games?pool="+pool.ID, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		UserGames []struct {
			ID             string  `json:"id"`
			PoolMembership *string `json:"pool_membership"`
			Game           struct {
				Title string `json:"title"`
			} `json:"game"`
		} `json:"user_games"`
		Total int `json:"total"`
	}
	mustUnmarshal(t, rec, &resp)

	got := map[string]*string{}
	for _, ug := range resp.UserGames {
		got[ug.ID] = ug.PoolMembership
	}
	if _, ok := got["ug-shooter"]; ok {
		t.Fatalf("shooter should not match RPG pool filter")
	}
	if _, ok := got["ug-rpgdone"]; ok {
		t.Fatalf("finished RPG must be excluded from suggestions")
	}
	if v, ok := got["ug-rpg1"]; !ok || v == nil || *v != "candidate" {
		t.Fatalf("ug-rpg1 should be a candidate member, got %v", got["ug-rpg1"])
	}
	if v, ok := got["ug-rpg2"]; !ok || v != nil {
		t.Fatalf("ug-rpg2 should match with null membership (a suggestion), got %v", got["ug-rpg2"])
	}
}

func TestPoolSuggestionNullFilterEmpty(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID, token := setupUserGamesUser(t, testDB, e, "nullfilter")
	gid := insertTestGame(t, testDB, "Lonely")
	insertTestUserGame(t, testDB, "ug-lonely", userID, int(gid))

	poolRec := postJSONAuth(t, e, "/api/pools", map[string]any{"name": "Manual"}, token)
	var pool struct {
		ID string `json:"id"`
	}
	mustUnmarshal(t, poolRec, &pool)

	rec := getAuth(t, e, "/api/user-games?pool="+pool.ID, token)
	var resp struct {
		Total int `json:"total"`
	}
	mustUnmarshal(t, rec, &resp)
	if resp.Total != 0 {
		t.Fatalf("expected empty result for NULL-filter pool, got total=%d", resp.Total)
	}
}
