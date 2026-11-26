package tasks_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/joblifecycle"
	"github.com/drzero42/nexorious/internal/notify"
	"github.com/drzero42/nexorious/internal/usergame"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

// Import Jobs on the lifecycle module (#1267). Jobs are created through the
// module, as the upload handlers create them.

// createImport creates an import Job through the module, one item per
// payload. A nexorious payload is wrapped as the upload handler wraps it.
// pools, when set, adds the pools item.
func createImport(t *testing.T, lc *joblifecycle.Module, userID, source string, payloads []map[string]any, pools any) (string, []string) {
	t.Helper()
	var items []joblifecycle.NewItem
	if pools != nil {
		items = append(items, joblifecycle.NewItem{
			UserID: userID, ItemKey: tasks.PoolsItemKey, SourceTitle: "(pools)", Completed: true,
			SourceMetadata: mustMarshal(t, map[string]any{"item_type": "pools", "data": pools}),
		})
	}
	for i, p := range payloads {
		meta := mustMarshal(t, p)
		if source == models.JobSourceNexorious {
			meta = mustMarshal(t, map[string]any{"item_type": "game", "data": p})
		}
		title, _ := p["title"].(string)
		items = append(items, joblifecycle.NewItem{
			UserID: userID, ItemKey: fmt.Sprintf("game_%d", i), SourceTitle: title, SourceMetadata: meta,
		})
	}
	c, err := lc.CreateWithItems(context.Background(), joblifecycle.NewJob{
		UserID: userID, JobType: models.JobTypeImport, Source: source,
		Priority: models.JobPriorityHigh, TotalItems: len(payloads),
	}, items, nil)
	if err != nil || !c.Created {
		t.Fatalf("create import: %+v %v", c, err)
	}
	var ids []string
	if err := testDB.NewRaw(`SELECT id FROM job_items WHERE job_id = ? AND item_key <> ? ORDER BY item_key`,
		c.JobID, tasks.PoolsItemKey).Scan(context.Background(), &ids); err != nil {
		t.Fatal(err)
	}
	return c.JobID, ids
}

func importUser(t *testing.T) string {
	t.Helper()
	truncateAllTables(t)
	id := uuid.NewString()
	insertTestUser(t, testDB, id)
	return id
}

func insertGame(t *testing.T, id int32, title string) {
	t.Helper()
	if _, err := testDB.NewRaw(`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, ?, now(), now())`, id, title).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func countUserGames(t *testing.T, userID string) int {
	t.Helper()
	var n int
	if err := testDB.NewRaw(`SELECT count(*) FROM user_games WHERE user_id = ?`, userID).Scan(context.Background(), &n); err != nil {
		t.Fatal(err)
	}
	return n
}

func ownerKind(t *testing.T, itemID string) string {
	t.Helper()
	var kind string
	if err := testDB.NewRaw(`SELECT rj.kind FROM river_job rj JOIN job_items ji ON ji.river_job_id = rj.id WHERE ji.id = ?`, itemID).Scan(context.Background(), &kind); err != nil {
		t.Fatalf("owner kind: %v", err)
	}
	return kind
}

func execSQL(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := testDB.ExecContext(context.Background(), q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

// runEffectsErr runs every queued ending-effects River job and returns the
// first error. The rows stay available, as a River retry would find them.
func runEffectsErr(t *testing.T, lc *joblifecycle.Module) error {
	t.Helper()
	var rows []struct {
		ID   int64           `bun:"id"`
		Args json.RawMessage `bun:"args"`
	}
	if err := testDB.NewRaw(`SELECT id, args FROM river_job WHERE kind = ? AND state = 'available' ORDER BY id`, joblifecycle.EffectsArgs{}.Kind()).Scan(context.Background(), &rows); err != nil {
		t.Fatalf("list effects jobs: %v", err)
	}
	w := &joblifecycle.EffectsWorker{Module: lc}
	for _, r := range rows {
		var args joblifecycle.EffectsArgs
		if err := json.Unmarshal(r.Args, &args); err != nil {
			t.Fatal(err)
		}
		if err := w.Work(context.Background(), riverJob(r.ID, 1, args)); err != nil {
			return err
		}
	}
	return nil
}

// markEffectsDone records the queued effects as run, so a later runEffects
// only runs effects queued after this point.
func markEffectsDone(t *testing.T) {
	t.Helper()
	execSQL(t, `UPDATE river_job SET state = 'completed', finalized_at = now() WHERE kind = ? AND state = 'available'`, joblifecycle.EffectsArgs{}.Kind())
}

// #1303: an item interrupted by a cancelled context records nothing and is
// completed when it runs again after the restart.
func TestImportItemInterruptedThenCompleted(t *testing.T) {
	userID := importUser(t)
	lc := newTestLC(t)
	jobID, ids := createImport(t, lc, userID, models.JobSourceNexorious, []map[string]any{{"igdb_id": 501, "title": "G"}}, nil)
	insertGame(t, 501, "G")
	w := &tasks.ImportItemWorker{DB: testDB, LC: lc}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := w.Work(cancelled, ownedJob(t, ids[0], tasks.ImportItemArgs{JobItemID: ids[0]})); !errors.Is(err, context.Canceled) {
		t.Fatalf("interrupted work: err = %v, want context.Canceled", err)
	}
	if s, _ := itemStatus(t, ids[0]); s != models.JobItemStatusPending {
		t.Fatalf("interrupted item = %s, want pending", s)
	}
	if countUserGames(t, userID) != 0 {
		t.Fatal("interrupted item wrote the library")
	}

	// Restart: reclaim and recover leave the still-queued owner alone.
	if _, err := lc.ReclaimRunning(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := lc.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := w.Work(context.Background(), ownedJob(t, ids[0], tasks.ImportItemArgs{JobItemID: ids[0]})); err != nil {
		t.Fatal(err)
	}
	if s, _ := itemStatus(t, ids[0]); s != models.JobItemStatusCompleted {
		t.Fatalf("item = %s, want completed", s)
	}
	if jobStatus(t, jobID) != models.JobStatusCompleted {
		t.Fatalf("job = %s, want completed", jobStatus(t, jobID))
	}
}

// #1303: a Resolved item has its finalize River job from the same
// transaction, so an interruption cannot strand it.
func TestImportResolvedItemCompletedAfterRestart(t *testing.T) {
	userID := importUser(t)
	lc := newTestLC(t)
	jobID, ids := createImport(t, lc, userID, models.JobSourceCSV, []map[string]any{{"title": "Some Game", "platforms": []any{}}}, nil)
	insertGame(t, 77, "Some Game")

	match := &tasks.ImportMatchWorker{DB: testDB, LC: lc}
	if err := match.Work(context.Background(), ownedJob(t, ids[0], tasks.ImportMatchArgs{JobItemID: ids[0]})); err != nil {
		t.Fatal(err)
	}
	if s, _ := itemStatus(t, ids[0]); s != models.JobItemStatusPendingReview {
		t.Fatalf("item = %s, want pending_review", s)
	}
	if err := lc.Resolve(context.Background(), ids[0], 77); err != nil {
		t.Fatal(err)
	}
	if k := ownerKind(t, ids[0]); k != (tasks.ImportFinalizeArgs{}).Kind() {
		t.Fatalf("resolved item owner kind = %s, want import_finalize", k)
	}

	// The server stops before finalize runs.
	if _, err := lc.ReclaimRunning(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := lc.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	fin := &tasks.ImportFinalizeWorker{DB: testDB, LC: lc, StoragePath: t.TempDir()}
	if err := fin.Work(context.Background(), ownedJob(t, ids[0], tasks.ImportFinalizeArgs{JobItemID: ids[0]})); err != nil {
		t.Fatal(err)
	}
	if jobStatus(t, jobID) != models.JobStatusCompleted || countUserGames(t, userID) != 1 {
		t.Fatalf("job = %s, user games = %d", jobStatus(t, jobID), countUserGames(t, userID))
	}
}

// Invariant 8: the library write, its changes row and the item's completion
// commit together.
func TestImportFinalizeChangesInsertFailsLeavesNothing(t *testing.T) {
	userID := importUser(t)
	lc := newTestLC(t)
	jobID, ids := createImport(t, lc, userID, models.JobSourceCSV, []map[string]any{{"title": "G", "igdb_id": 42, "platforms": []any{map[string]any{"platform": "pc-windows"}}}}, nil)
	insertGame(t, 42, "G")
	match := &tasks.ImportMatchWorker{DB: testDB, LC: lc}
	if err := match.Work(context.Background(), ownedJob(t, ids[0], tasks.ImportMatchArgs{JobItemID: ids[0]})); err != nil {
		t.Fatal(err)
	}
	owner := itemOwner(t, ids[0])

	execSQL(t, `CREATE FUNCTION fail_changes() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected'; END $$`)
	execSQL(t, `CREATE TRIGGER fail_changes BEFORE INSERT ON changes FOR EACH ROW EXECUTE FUNCTION fail_changes()`)
	dropped := false
	drop := func() {
		if !dropped {
			execSQL(t, `DROP TRIGGER fail_changes ON changes`)
			execSQL(t, `DROP FUNCTION fail_changes()`)
			dropped = true
		}
	}
	t.Cleanup(drop)

	fin := &tasks.ImportFinalizeWorker{DB: testDB, LC: lc, StoragePath: t.TempDir()}
	if err := fin.Work(context.Background(), riverJob(owner, 1, tasks.ImportFinalizeArgs{JobItemID: ids[0]})); err == nil {
		t.Fatal("finalize with a failing changes insert returned nil")
	}
	if countUserGames(t, userID) != 0 || countAllChangeRows(t, jobID) != 0 {
		t.Fatalf("rolled-back finalize left rows: user games %d, changes %d", countUserGames(t, userID), countAllChangeRows(t, jobID))
	}
	if s, _ := itemStatus(t, ids[0]); s != models.JobItemStatusPending || itemOwner(t, ids[0]) != owner {
		t.Fatalf("item = %s, want pending with the same owner", s)
	}

	drop()
	if err := fin.Work(context.Background(), riverJob(owner, 2, tasks.ImportFinalizeArgs{JobItemID: ids[0]})); err != nil {
		t.Fatal(err)
	}
	if countUserGames(t, userID) != 1 || countChangeRows(t, jobID, "added") != 1 {
		t.Fatal("retried finalize did not write the library")
	}
}

// A second River job for an item that already completed changes nothing and
// stops before ensureGameRow (so before any IGDB call): the games row it
// would recreate stays gone.
func TestImportDuplicateRiverJobChangesNothing(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func(lc *joblifecycle.Module, itemID string, riverJobID int64) error
	}{
		{"finalize", func(lc *joblifecycle.Module, itemID string, rid int64) error {
			w := &tasks.ImportFinalizeWorker{DB: testDB, LC: lc, StoragePath: t.TempDir()}
			return w.Work(context.Background(), riverJob(rid, 1, tasks.ImportFinalizeArgs{JobItemID: itemID}))
		}},
		{"legacy", func(lc *joblifecycle.Module, itemID string, rid int64) error {
			w := &tasks.ImportItemWorker{DB: testDB, LC: lc}
			return w.Work(context.Background(), riverJob(rid, 1, tasks.ImportItemArgs{JobItemID: itemID}))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			userID := importUser(t)
			lc := newTestLC(t)
			source := models.JobSourceCSV
			if tc.name == "legacy" {
				source = models.JobSourceNexorious
			}
			jobID, ids := createImport(t, lc, userID, source, []map[string]any{{"title": "G", "igdb_id": 42, "platforms": []any{}}}, nil)
			insertGame(t, 42, "G")
			if tc.name == "finalize" {
				m := &tasks.ImportMatchWorker{DB: testDB, LC: lc}
				if err := m.Work(context.Background(), ownedJob(t, ids[0], tasks.ImportMatchArgs{JobItemID: ids[0]})); err != nil {
					t.Fatal(err)
				}
			}
			owner := itemOwner(t, ids[0])
			if err := tc.run(lc, ids[0], owner); err != nil {
				t.Fatal(err)
			}
			if jobStatus(t, jobID) != models.JobStatusCompleted {
				t.Fatal("job not completed")
			}
			execSQL(t, `DELETE FROM user_games WHERE user_id = ?`, userID)
			execSQL(t, `DELETE FROM games WHERE id = 42`)

			// The owner run again (after a crash), and another River job.
			for _, rid := range []int64{owner, owner + 1000} {
				if err := tc.run(lc, ids[0], rid); err != nil {
					t.Fatalf("duplicate run: %v", err)
				}
			}
			var games int
			if err := testDB.NewRaw(`SELECT count(*) FROM games WHERE id = 42`).Scan(context.Background(), &games); err != nil {
				t.Fatal(err)
			}
			if games != 0 || countUserGames(t, userID) != 0 || countAllChangeRows(t, jobID) != 1 {
				t.Errorf("duplicate run wrote: games %d, user games %d, changes %d", games, countUserGames(t, userID), countAllChangeRows(t, jobID))
			}
		})
	}
}

// A game already in the library whose only change is a new tag is recorded
// as updated (Acquire reports the tag links it inserted).
func TestImportNewTagOnlyIsUpdated(t *testing.T) {
	userID := importUser(t)
	lc := newTestLC(t)
	insertGame(t, 42, "G")
	pc := "pc-windows"
	if _, err := usergame.Acquire(context.Background(), testDB, usergame.AcquireParams{
		UserID: userID, GameID: 42, Mode: usergame.ModeImport,
		Platforms: []usergame.PlatformInput{{Platform: &pc}},
	}); err != nil {
		t.Fatal(err)
	}
	jobID, ids := createImport(t, lc, userID, models.JobSourceCSV, []map[string]any{{
		"title": "G", "igdb_id": 42, "platforms": []any{map[string]any{"platform": "pc-windows"}}, "tags": []string{"cozy"},
	}}, nil)
	m := &tasks.ImportMatchWorker{DB: testDB, LC: lc}
	if err := m.Work(context.Background(), ownedJob(t, ids[0], tasks.ImportMatchArgs{JobItemID: ids[0]})); err != nil {
		t.Fatal(err)
	}
	fin := &tasks.ImportFinalizeWorker{DB: testDB, LC: lc, StoragePath: t.TempDir()}
	if err := fin.Work(context.Background(), ownedJob(t, ids[0], tasks.ImportFinalizeArgs{JobItemID: ids[0]})); err != nil {
		t.Fatal(err)
	}
	if n := countChangeRows(t, jobID, "updated"); n != 1 {
		t.Errorf("updated changes = %d, want 1", n)
	}
}

// Retry sends an item back to the stage it failed in: one with its IGDB game
// to finalize, one without to match.
func TestImportRetryGoesToTheFailedStage(t *testing.T) {
	userID := importUser(t)
	lc := newTestLC(t)
	ctx := context.Background()
	jobID, ids := createImport(t, lc, userID, models.JobSourceCSV, []map[string]any{
		{"title": "Matched", "igdb_id": 42, "platforms": []any{}},
		{"title": "Unmatched", "platforms": []any{}},
	}, nil)
	m := &tasks.ImportMatchWorker{DB: testDB, LC: lc}
	if err := m.Work(ctx, ownedJob(t, ids[0], tasks.ImportMatchArgs{JobItemID: ids[0]})); err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if err := lc.EndItem(ctx, id, itemOwner(t, id), joblifecycle.ItemResult{Status: models.JobItemStatusFailed, Message: "boom"}); err != nil {
			t.Fatal(err)
		}
	}
	if jobStatus(t, jobID) != models.JobStatusCompleted {
		t.Fatal("job not completed")
	}
	if n, err := lc.Retry(ctx, jobID, nil); err != nil || n != 2 {
		t.Fatalf("retry: %d %v", n, err)
	}
	if k := ownerKind(t, ids[0]); k != (tasks.ImportFinalizeArgs{}).Kind() {
		t.Errorf("matched item retried as %s, want import_finalize", k)
	}
	if k := ownerKind(t, ids[1]); k != (tasks.ImportMatchArgs{}).Kind() {
		t.Errorf("unmatched item retried as %s, want import_match", k)
	}
}

func countEventKey(t *testing.T, jobID, typ string) int {
	t.Helper()
	var n int
	if err := testDB.NewRaw(`SELECT count(*) FROM events WHERE dedup_key = ?`, jobID+":"+typ).Scan(context.Background(), &n); err != nil {
		t.Fatal(err)
	}
	return n
}

func countPools(t *testing.T, userID string) int {
	t.Helper()
	var n int
	if err := testDB.NewRaw(`SELECT count(*) FROM pools WHERE user_id = ?`, userID).Scan(context.Background(), &n); err != nil {
		t.Fatal(err)
	}
	return n
}

// The notification goes first and a pools error is retried; a call that does
// not complete the Job sends nothing; the pools are applied on the first
// ending only, not when a retried Job completes again.
func TestImportEffectsNotificationAndPools(t *testing.T) {
	userID := importUser(t)
	lc := newTestLC(t)
	ctx := context.Background()
	insertGame(t, 501, "A")
	insertGame(t, 502, "B")
	pools := []map[string]any{{"name": "Backlog", "position": 0, "games": []map[string]any{{"igdb_id": 501}, {"igdb_id": 502}}}}
	jobID, ids := createImport(t, lc, userID, models.JobSourceNexorious, []map[string]any{
		{"igdb_id": 501, "title": "A"}, {"igdb_id": 502, "title": "B"},
	}, pools)
	w := &tasks.ImportItemWorker{DB: testDB, LC: lc}

	if err := w.Work(ctx, ownedJob(t, ids[0], tasks.ImportItemArgs{JobItemID: ids[0]})); err != nil {
		t.Fatal(err)
	}
	// Not completed yet: nothing to send.
	if err := runEffectsErr(t, lc); err != nil {
		t.Fatal(err)
	}
	if countEventKey(t, jobID, notify.TypeImportCompleted)+countEventKey(t, jobID, notify.TypeImportFailed) != 0 {
		t.Fatal("notification sent before the Job completed")
	}

	// B fails; the Job completes with one failure.
	if err := lc.EndItem(ctx, ids[1], itemOwner(t, ids[1]), joblifecycle.ItemResult{Status: models.JobItemStatusFailed, Message: "boom"}); err != nil {
		t.Fatal(err)
	}

	// The pools step fails: the notification is still sent, and the error
	// returned so River retries the effects.
	execSQL(t, `CREATE FUNCTION fail_pools() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected'; END $$`)
	execSQL(t, `CREATE TRIGGER fail_pools BEFORE INSERT ON pools FOR EACH ROW EXECUTE FUNCTION fail_pools()`)
	err := runEffectsErr(t, lc)
	execSQL(t, `DROP TRIGGER fail_pools ON pools`)
	execSQL(t, `DROP FUNCTION fail_pools()`)
	if err == nil {
		t.Fatal("pools database error was not returned")
	}
	if countEventKey(t, jobID, notify.TypeImportFailed) != 1 {
		t.Fatal("import.failed not sent before the pools step")
	}
	if err := runEffectsErr(t, lc); err != nil {
		t.Fatalf("retried effects: %v", err)
	}
	if countPools(t, userID) != 1 || countEventKey(t, jobID, notify.TypeImportFailed) != 1 {
		t.Fatalf("pools %d, import.failed %d", countPools(t, userID), countEventKey(t, jobID, notify.TypeImportFailed))
	}
	markEffectsDone(t)

	// The user deletes the pool; a Retry completes the Job again.
	execSQL(t, `DELETE FROM pools WHERE user_id = ?`, userID)
	if n, err := lc.Retry(ctx, jobID, nil); err != nil || n != 1 {
		t.Fatalf("retry: %d %v", n, err)
	}
	if err := w.Work(ctx, ownedJob(t, ids[1], tasks.ImportItemArgs{JobItemID: ids[1]})); err != nil {
		t.Fatal(err)
	}
	if jobStatus(t, jobID) != models.JobStatusCompleted {
		t.Fatal("retried job not completed")
	}
	if err := runEffectsErr(t, lc); err != nil {
		t.Fatal(err)
	}
	if countPools(t, userID) != 0 {
		t.Error("pools applied again on a second ending")
	}
	if countEventKey(t, jobID, notify.TypeImportCompleted) != 1 {
		t.Error("import.completed not sent on the second ending")
	}
}

func TestImportAllFailedSendsImportFailed(t *testing.T) {
	userID := importUser(t)
	lc := newTestLC(t)
	jobID, ids := createImport(t, lc, userID, models.JobSourceNexorious, []map[string]any{{"title": "no id"}, {"title": "no id either"}}, nil)
	w := &tasks.ImportItemWorker{DB: testDB, LC: lc}
	for _, id := range ids {
		if err := w.Work(context.Background(), ownedJob(t, id, tasks.ImportItemArgs{JobItemID: id})); err != nil {
			t.Fatal(err)
		}
	}
	runEffects(t, lc)
	if countEventKey(t, jobID, notify.TypeImportFailed) != 1 || countEventKey(t, jobID, notify.TypeImportCompleted) != 0 {
		t.Error("an import whose every item failed must send import.failed only")
	}
}

// The generic import validates platforms through Acquire: an unknown one
// fails the item at once and writes nothing.
func TestImportFinalizeUnknownPlatformFails(t *testing.T) {
	userID := importUser(t)
	lc := newTestLC(t)
	insertGame(t, 42, "G")
	_, ids := createImport(t, lc, userID, models.JobSourceCSV, []map[string]any{{"title": "G", "igdb_id": 42, "platforms": []any{map[string]any{"platform": "no-such-platform"}}}}, nil)
	m := &tasks.ImportMatchWorker{DB: testDB, LC: lc}
	if err := m.Work(context.Background(), ownedJob(t, ids[0], tasks.ImportMatchArgs{JobItemID: ids[0]})); err != nil {
		t.Fatal(err)
	}
	fin := &tasks.ImportFinalizeWorker{DB: testDB, LC: lc, StoragePath: t.TempDir()}
	if err := fin.Work(context.Background(), ownedJob(t, ids[0], tasks.ImportFinalizeArgs{JobItemID: ids[0]})); err != nil {
		t.Fatal(err)
	}
	s, msg := itemStatus(t, ids[0])
	if s != models.JobItemStatusFailed || msg == nil || !strings.Contains(*msg, "no-such-platform") {
		t.Fatalf("item = %s %v, want failed naming the platform", s, msg)
	}
	if countUserGames(t, userID) != 0 {
		t.Error("failed item left a library row")
	}
}

// The legacy worker treats only sql.ErrNoRows as an unknown platform; any
// other lookup error is returned to River.
func TestImportItemPlatformLookupErrorIsReturned(t *testing.T) {
	userID := importUser(t)
	lc := newTestLC(t)
	insertGame(t, 501, "G")
	_, ids := createImport(t, lc, userID, models.JobSourceNexorious, []map[string]any{{
		"igdb_id": 501, "title": "G", "platforms": []any{map[string]any{"platform": "pc-windows"}},
	}}, nil)
	execSQL(t, `ALTER TABLE platforms RENAME TO platforms_hidden`)
	restored := false
	restore := func() {
		if !restored {
			execSQL(t, `ALTER TABLE platforms_hidden RENAME TO platforms`)
			restored = true
		}
	}
	t.Cleanup(restore)

	w := &tasks.ImportItemWorker{DB: testDB, LC: lc}
	err := w.Work(context.Background(), ownedJob(t, ids[0], tasks.ImportItemArgs{JobItemID: ids[0]}))
	restore()
	if err == nil {
		t.Fatal("a platform lookup error was treated as an unknown platform")
	}
	if s, _ := itemStatus(t, ids[0]); s != models.JobItemStatusPending {
		t.Errorf("item = %s, want pending", s)
	}
}

// An import whose source has been removed from the registry is still on the
// module: recovered, matched, skipped and completed.
func TestImportFromRemovedSourceIsRecoveredAndCompleted(t *testing.T) {
	userID := importUser(t)
	lc := newTestLC(t)
	ctx := context.Background()
	jobID, ids := createImport(t, lc, userID, "darkadia", []map[string]any{{"title": "Old", "platforms": []any{}}}, nil)
	// Its River job is lost.
	execSQL(t, `UPDATE job_items SET river_job_id = NULL WHERE job_id = ?`, jobID)
	execSQL(t, `DELETE FROM river_job`)

	if res, err := lc.Recover(ctx); err != nil || res.Requeued != 1 {
		t.Fatalf("recover: %+v %v", res, err)
	}
	if k := ownerKind(t, ids[0]); k != (tasks.ImportMatchArgs{}).Kind() {
		t.Fatalf("recovered as %s, want import_match", k)
	}
	m := &tasks.ImportMatchWorker{DB: testDB, LC: lc}
	if err := m.Work(ctx, ownedJob(t, ids[0], tasks.ImportMatchArgs{JobItemID: ids[0]})); err != nil {
		t.Fatal(err)
	}
	if err := lc.Skip(ctx, ids[0]); err != nil {
		t.Fatal(err)
	}
	if jobStatus(t, jobID) != models.JobStatusCompleted {
		t.Fatalf("job = %s, want completed", jobStatus(t, jobID))
	}
}

// An import_match cut off by a cancelled context during its IGDB search is
// neither failed nor sent to review, even on its last attempt.
func TestImportMatchCancelledDuringSearch(t *testing.T) {
	userID := importUser(t)
	lc := newTestLC(t)
	_, ids := createImport(t, lc, userID, models.JobSourceCSV, []map[string]any{{"title": "Slow", "platforms": []any{}}}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	stop := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth2/token" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"t","expires_in":3600,"token_type":"bearer"}`))
			return
		}
		cancel() // the search is under way when the worker is cancelled
		select {
		case <-r.Context().Done():
		case <-stop:
		}
	}))
	defer srv.Close()
	defer close(stop) // runs first: release a handler whose client already left

	m := &tasks.ImportMatchWorker{DB: testDB, LC: lc, IGDBClient: newIGDBClientForTests(t, srv.URL+"/oauth2/token", srv.URL)}
	err := m.Work(ctx, riverJob(itemOwner(t, ids[0]), 5, tasks.ImportMatchArgs{JobItemID: ids[0]}))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if s, _ := itemStatus(t, ids[0]); s != models.JobItemStatusPending {
		t.Errorf("item = %s, want pending", s)
	}
}

// A half-created generic import (its upload handler died; dispatch not done)
// is failed by recovery, which sends import.failed and queues nothing for
// its items.
func TestImportHalfCreatedFailedByRecovery(t *testing.T) {
	userID := importUser(t)
	lc := newTestLC(t)
	ctx := context.Background()
	jobID, _ := createImport(t, lc, userID, models.JobSourceCSV, []map[string]any{{"title": "A"}, {"title": "B"}}, nil)
	execSQL(t, `UPDATE jobs SET dispatch_complete = false WHERE id = ?`, jobID)
	execSQL(t, `UPDATE job_items SET river_job_id = NULL WHERE job_id = ?`, jobID)
	execSQL(t, `DELETE FROM river_job`)

	if _, err := lc.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	var msg *string
	if err := testDB.NewRaw(`SELECT error_message FROM jobs WHERE id = ?`, jobID).Scan(ctx, &msg); err != nil {
		t.Fatal(err)
	}
	if jobStatus(t, jobID) != models.JobStatusFailed || msg == nil || !strings.Contains(*msg, "start the import again") {
		t.Fatalf("job = %s %v", jobStatus(t, jobID), msg)
	}
	var queued int
	if err := testDB.NewRaw(`SELECT count(*) FROM river_job WHERE kind IN ('import_match', 'import_finalize')`).Scan(ctx, &queued); err != nil {
		t.Fatal(err)
	}
	if queued != 0 {
		t.Errorf("recovery queued %d item River jobs for a Job it failed", queued)
	}
	runEffects(t, lc)
	if countEventKey(t, jobID, notify.TypeImportFailed) != 1 {
		t.Error("import.failed not sent")
	}
}

// A River job queued by an older version (no recorded owner) is adopted, not
// duplicated.
func TestImportRecoveryAdoptsOlderRiverJob(t *testing.T) {
	userID := importUser(t)
	lc := newTestLC(t)
	jobID, ids := createImport(t, lc, userID, models.JobSourceCSV, []map[string]any{{"title": "A"}}, nil)
	queued := itemOwner(t, ids[0])
	execSQL(t, `UPDATE job_items SET river_job_id = NULL WHERE job_id = ?`, jobID)

	if res, err := lc.Recover(context.Background()); err != nil || res.Requeued != 0 {
		t.Fatalf("recover: %+v %v", res, err)
	}
	if itemOwner(t, ids[0]) != queued {
		t.Error("the queued River job was not adopted")
	}
}

// Rows as the stage 2 migration leaves them, handled by recovery at startup:
// a resolved item that was stranded in 'processing' is now pending without a
// River job and is finalized; a nexorious import that was in flight is
// completed and its pools applied.
func TestImportRecoveryAfterUpgrade(t *testing.T) {
	userID := importUser(t)
	lc := newTestLC(t)
	ctx := context.Background()
	insertGame(t, 42, "G")
	insertGame(t, 501, "A")

	csvJob, csvIDs := createImport(t, lc, userID, models.JobSourceCSV, []map[string]any{{"title": "G", "platforms": []any{}}}, nil)
	pools := []map[string]any{{"name": "Backlog", "position": 0, "games": []map[string]any{{"igdb_id": 501}}}}
	nxJob, nxIDs := createImport(t, lc, userID, models.JobSourceNexorious, []map[string]any{{"igdb_id": 501, "title": "A"}}, pools)
	// What the older version left and the migration normalised: no owners,
	// no River jobs, the resolved id kept.
	execSQL(t, `UPDATE job_items SET river_job_id = NULL`)
	execSQL(t, `UPDATE job_items SET resolved_igdb_id = 42 WHERE id = ?`, csvIDs[0])
	execSQL(t, `DELETE FROM river_job`)

	if _, err := lc.ReclaimRunning(ctx); err != nil {
		t.Fatal(err)
	}
	if res, err := lc.Recover(ctx); err != nil || res.Requeued != 2 {
		t.Fatalf("recover: %+v %v", res, err)
	}
	if k := ownerKind(t, csvIDs[0]); k != (tasks.ImportFinalizeArgs{}).Kind() {
		t.Fatalf("resolved item recovered as %s, want import_finalize", k)
	}
	fin := &tasks.ImportFinalizeWorker{DB: testDB, LC: lc, StoragePath: t.TempDir()}
	if err := fin.Work(ctx, ownedJob(t, csvIDs[0], tasks.ImportFinalizeArgs{JobItemID: csvIDs[0]})); err != nil {
		t.Fatal(err)
	}
	w := &tasks.ImportItemWorker{DB: testDB, LC: lc}
	if err := w.Work(ctx, ownedJob(t, nxIDs[0], tasks.ImportItemArgs{JobItemID: nxIDs[0]})); err != nil {
		t.Fatal(err)
	}
	for _, j := range []string{csvJob, nxJob} {
		if jobStatus(t, j) != models.JobStatusCompleted {
			t.Errorf("job %s = %s, want completed", j, jobStatus(t, j))
		}
	}
	runEffects(t, lc)
	if countPools(t, userID) != 1 {
		t.Error("pools of the in-flight nexorious import not applied")
	}
}
