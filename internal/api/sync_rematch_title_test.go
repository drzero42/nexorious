package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// rematchTitleFixture rematches the user's External game "Gauntlet™" through
// the full router with a stub IGDB server, then returns the games row's title
// and the IGDB title the storefront list shows for the game (#1455).
func rematchTitleFixture(t *testing.T, igdbGame map[string]any, igdbID int) (rowTitle, listTitle string, idLookedUp bool) {
	t.Helper()
	srv, stub := newIGDBInferenceServer(t, igdbGame, nil)
	e := newTestEchoWithLiveIGDB(t, testDB, srv.URL)
	insertAuthTestUser(t, testDB, "u-rm-title", "rmtitle", "pass123", true, false)
	token := loginAndGetToken(t, e, "rmtitle", "pass123")
	insertExternalGame(t, testDB, "eg-rm-title", "u-rm-title", "playstation-store", "ps-1", "Gauntlet™")

	rec := postJSONAuth(t, e, "/api/sync/external-games/eg-rm-title/rematch",
		map[string]any{"igdb_id": igdbID}, token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("rematch: expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	if err := testDB.NewRaw(`SELECT title FROM games WHERE id = ?`, igdbID).Scan(context.Background(), &rowTitle); err != nil {
		t.Fatalf("read games row: %v", err)
	}
	rec = getAuth(t, e, "/api/sync/playstation-store/external-games", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var list []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list: expected 1 entry, got %d", len(list))
	}
	listTitle, _ = list[0]["igdb_title"].(string)
	stub.mu.Lock()
	defer stub.mu.Unlock()
	return rowTitle, listTitle, stub.idQueried
}

// A Rematch onto an IGDB game new to the database writes its games row with
// the IGDB title, so the storefront list shows it at once, before the
// metadata fetch (#1455).
func TestRematchExternalGame_NewIGDBGame_ListShowsIGDBTitle(t *testing.T) {
	truncateAllTables(t)
	const want = "Warhammer Age of Sigmar: Realms of Ruin – Gaunt Summoner"
	rowTitle, listTitle, _ := rematchTitleFixture(t,
		map[string]any{"id": 295728, "name": want, "slug": "gaunt-summoner"}, 295728)
	if rowTitle != want {
		t.Errorf("games.title: want %q, got %q", want, rowTitle)
	}
	if listTitle != want {
		t.Errorf("list igdb_title: want %q, got %q", want, listTitle)
	}
}

// When IGDB cannot give the title, the Rematch still goes through and the
// games row takes the storefront title until the metadata fetch replaces it.
func TestRematchExternalGame_IGDBLookupFails_FallsBackToStorefrontTitle(t *testing.T) {
	truncateAllTables(t)
	rowTitle, _, looked := rematchTitleFixture(t, nil, 295728)
	if !looked {
		t.Fatal("expected an IGDB lookup for a game new to the database")
	}
	if rowTitle != "Gauntlet™" {
		t.Errorf("games.title: want the storefront title, got %q", rowTitle)
	}
}

// A Rematch onto an IGDB game already in the database asks IGDB nothing and
// leaves its row alone.
func TestRematchExternalGame_KnownIGDBGame_NoLookup(t *testing.T) {
	truncateAllTables(t)
	if _, err := testDB.ExecContext(context.Background(),
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (295728, 'Known Title', now(), now())`); err != nil {
		t.Fatalf("insert game: %v", err)
	}
	rowTitle, listTitle, looked := rematchTitleFixture(t,
		map[string]any{"id": 295728, "name": "Other Title", "slug": "other"}, 295728)
	if looked {
		t.Error("expected no IGDB lookup for a game already in the database")
	}
	if rowTitle != "Known Title" || listTitle != "Known Title" {
		t.Errorf("want the existing title kept, got row %q, list %q", rowTitle, listTitle)
	}
}
