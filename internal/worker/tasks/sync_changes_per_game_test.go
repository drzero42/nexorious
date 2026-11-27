package tasks_test

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/drzero42/nexorious/internal/notify"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

// A sync's change lists show a game once, whatever editions it has (#1443).

// setDecision sets an External game's match (0 = none) and skip.
func setDecision(t *testing.T, egID string, igdbID int32, skipped bool) {
	t.Helper()
	var match any
	if igdbID != 0 {
		match = igdbID
	}
	if _, err := testDB.ExecContext(context.Background(),
		`UPDATE external_games SET resolved_igdb_id = ?, is_skipped = ? WHERE id = ?`, match, skipped, egID,
	); err != nil {
		t.Fatalf("set decision: %v", err)
	}
}

// The sweep lists a game as removed only in the sync that takes its last
// available edition away; another game that counts as the same game (an
// edition, or a same-title game without a decision) still available means no
// row.
func TestDispatchSync_RemovedOncePerGame(t *testing.T) {
	cases := []struct {
		name      string
		bMatch    int32 // 0 = undecided
		bStays    bool
		wantLines int
	}{
		{name: "both editions leave", bMatch: 100, bStays: false, wantLines: 1},
		{name: "one edition stays", bMatch: 100, bStays: true, wantLines: 0},
		{name: "an undecided same-title game stays", bMatch: 0, bStays: true, wantLines: 0},
		{name: "a same-title game with another decision stays", bMatch: 200, bStays: true, wantLines: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			truncateAllTables(t)
			ctx := context.Background()
			userID, jobID := uuid.NewString(), uuid.NewString()
			insertTestUser(t, testDB, userID)
			insertTestGame(t, 100, "Hades", time.Now())
			insertTestGame(t, 200, "Hades II", time.Now())
			if _, err := testDB.ExecContext(ctx,
				`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'psn', 'pending', 'low', 0)`,
				jobID, userID,
			); err != nil {
				t.Fatalf("insert job: %v", err)
			}
			if _, err := testDB.ExecContext(ctx,
				`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'psn', 'daily')`,
				uuid.NewString(), userID,
			); err != nil {
				t.Fatalf("insert sync config: %v", err)
			}

			a := insertTestExternalGame(t, userID, "psn", "1", "Hades", "playstation-4")
			b := insertTestExternalGame(t, userID, "psn", "2", "Hades", "playstation-5")
			setDecision(t, a, 100, false)
			setDecision(t, b, tc.bMatch, false)

			fetched := []tasks.ExternalGameEntry{
				{ExternalID: "3", Title: "Kept Game", Platforms: []string{"playstation-5"}, OwnershipStatus: "owned"},
			}
			if tc.bStays {
				fetched = append(fetched, tasks.ExternalGameEntry{ExternalID: "2", Title: "Hades", Platforms: []string{"playstation-5"}, OwnershipStatus: "owned"})
			}
			w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(&fakeStorefrontAdapter{batches: [][]tasks.ExternalGameEntry{fetched}}), LC: newTestLC(t)}
			_ = w.Work(ctx, syncDispatchJob(t, tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "psn"}))

			var aAvailable bool
			if err := testDB.NewRaw(`SELECT is_available FROM external_games WHERE id = ?`, a).Scan(ctx, &aAvailable); err != nil {
				t.Fatalf("read availability: %v", err)
			}
			if aAvailable {
				t.Fatal("the leaving edition should be marked unavailable")
			}
			var n int
			if err := testDB.NewRaw(
				`SELECT COUNT(*) FROM changes WHERE job_id = ? AND change_type = 'removed' AND title = 'Hades'`, jobID,
			).Scan(ctx, &n); err != nil {
				t.Fatalf("count removed: %v", err)
			}
			if n != tc.wantLines {
				t.Errorf("removed rows for Hades = %d, want %d", n, tc.wantLines)
			}
		})
	}
}

// A Job records a skipped game once, whichever of its editions is recorded
// first; another skipped game still gets its own row.
func TestInsertSkippedChange_OncePerSetOfEditions(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID, jobID, _ := syncCompletionFixture(t, "psn", true, 0)

	a := insertTestExternalGame(t, userID, "psn", "1", "Hades", "playstation-4")
	b := insertTestExternalGame(t, userID, "psn", "2", "Hades", "playstation-5")
	c := insertTestExternalGame(t, userID, "psn", "3", "Celeste", "playstation-4")
	for _, id := range []string{a, b, c} {
		setDecision(t, id, 0, true)
	}

	for _, g := range []struct{ id, title string }{{a, "Hades"}, {b, "Hades"}, {a, "Hades"}, {c, "Celeste"}} {
		if err := tasks.InsertSkippedChange(ctx, testDB, jobID, userID, g.id, g.title); err != nil {
			t.Fatalf("InsertSkippedChange(%s): %v", g.title, err)
		}
	}

	var titles []string
	if err := testDB.NewRaw(
		`SELECT title FROM changes WHERE job_id = ? AND change_type = 'skipped' ORDER BY title`, jobID,
	).Scan(ctx, &titles); err != nil {
		t.Fatalf("read skipped rows: %v", err)
	}
	if !slices.Equal(titles, []string{"Celeste", "Hades"}) {
		t.Errorf("skipped rows = %v, want [Celeste Hades]", titles)
	}
}

// The diff notification's added line shows the library entry's title and the
// platforms of every edition the Job wrote to it; its removed line shows the
// platforms of every edition of the set.
func TestSyncDiff_MergesEditionPlatforms(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	notify.SetRiverClient(nil)

	lc := newTestLC(t)
	userID, jobID, items := syncCompletionFixture(t, "psn", true, 1)

	insertTestGame(t, 100, "Hades", time.Now())
	ugID := uuid.NewString()
	if _, err := testDB.ExecContext(ctx,
		`INSERT INTO user_games (id, user_id, game_id, created_at, updated_at) VALUES (?, ?, 100, now(), now())`, ugID, userID,
	); err != nil {
		t.Fatalf("insert user game: %v", err)
	}
	ps4 := insertTestExternalGame(t, userID, "psn", "1", "HADES (PS4)", "playstation-4")
	ps5 := insertTestExternalGame(t, userID, "psn", "2", "HADES (PS5)", "playstation-5")
	oldPS4 := insertTestExternalGame(t, userID, "psn", "3", "Old Game", "playstation-4")
	insertTestExternalGame(t, userID, "psn", "4", "Old Game", "playstation-5")
	if _, err := testDB.ExecContext(ctx, `UPDATE external_games SET is_available = false WHERE title = 'Old Game'`); err != nil {
		t.Fatalf("mark unavailable: %v", err)
	}

	insertChange := func(egID string, ug any, changeType, title string, offset int) {
		t.Helper()
		if _, err := testDB.ExecContext(ctx,
			`INSERT INTO changes (id, job_id, user_id, external_game_id, user_game_id, change_type, title, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, now() + make_interval(secs => ?))`,
			uuid.NewString(), jobID, userID, egID, ug, changeType, title, offset,
		); err != nil {
			t.Fatalf("insert %s change: %v", changeType, err)
		}
	}
	insertChange(ps4, ugID, "added", "HADES (PS4)", 0)
	insertChange(ps5, ugID, "already_in_library", "HADES (PS5)", 1)
	insertChange(oldPS4, nil, "removed", "Old Game", 2)

	endSyncItem(t, lc, items[0], "completed")
	runSyncEffects(t, lc)

	var raw string
	if err := testDB.NewRaw(`SELECT payload::text FROM events WHERE dedup_key = ?`, jobID+":sync.diff").Scan(ctx, &raw); err != nil {
		t.Fatalf("read sync.diff payload: %v", err)
	}
	var p notify.SyncDiffPayload
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	both := []string{"playstation-4", "playstation-5"}
	if len(p.Added) != 1 || p.Added[0].Title != "Hades" || !slices.Equal(p.Added[0].Platforms, both) {
		t.Errorf("added = %+v, want one line Hades %v", p.Added, both)
	}
	if len(p.Removed) != 1 || p.Removed[0].Title != "Old Game" || !slices.Equal(p.Removed[0].Platforms, both) {
		t.Errorf("removed = %+v, want one line Old Game %v", p.Removed, both)
	}
}
