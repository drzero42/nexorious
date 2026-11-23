package tasks_test

// Tests for the library write's locking and the match guards in
// IGDBMatchWorker (#1338): a rematch that commits while a worker is running
// always wins.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/worker/tasks"
)

// rematchInTx locks egIDs FOR UPDATE in a transaction and sets their match to
// igdbID, as a rematch does, without committing. Call the returned commit.
func rematchInTx(t *testing.T, igdbID int32, egIDs ...string) (commit func()) {
	t.Helper()
	ctx := context.Background()
	famEnsureGame(t, igdbID)
	tx, err := testDB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	for _, id := range egIDs {
		if _, err := tx.NewRaw(`SELECT id FROM external_games WHERE id = ? FOR UPDATE`, id).Exec(ctx); err != nil {
			t.Fatalf("lock: %v", err)
		}
		if _, err := tx.NewRaw(`UPDATE external_games SET resolved_igdb_id = ? WHERE id = ?`, igdbID, id).Exec(ctx); err != nil {
			t.Fatalf("rematch: %v", err)
		}
	}
	done := false
	t.Cleanup(func() {
		if !done {
			_ = tx.Rollback()
		}
	})
	return func() {
		done = true
		if err := tx.Commit(); err != nil {
			t.Fatalf("commit: %v", err)
		}
	}
}

// runBlocked runs f in a goroutine, asserts it is still blocked after a short
// wait, then calls release and waits for f to return.
func runBlocked(t *testing.T, f func() error, release func()) {
	t.Helper()
	errc := make(chan error, 1)
	go func() { errc <- f() }()
	select {
	case err := <-errc:
		t.Fatalf("expected the worker to wait for the rematch's lock; it returned %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	release()
	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("worker: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("worker did not finish after the rematch committed")
	}
}

func ugCountFor(t *testing.T, userID string, igdbID int32) int {
	t.Helper()
	return famCount(t, `SELECT COUNT(*) FROM user_games WHERE user_id = ? AND game_id = ?`, userID, igdbID)
}

func TestUserGameWorker_WritesTheMatchARematchCommittedMeanwhile(t *testing.T) {
	userID := famSetup(t)
	egID := famInsertGame(t, userID, famGame{extID: "1", available: true, resolved: 100})
	jobID := famInsertJob(t, userID, "processing", true)
	itemID := famInsertItem(t, jobID, userID, egID, "pending")

	commit := rematchInTx(t, 200, egID)
	w := &tasks.UserGameWorker{DB: testDB, LC: newTestLC(t), RiverClient: newTestRiverClient(t)}
	runBlocked(t, func() error {
		return w.Work(context.Background(), writeJob(t, tasks.UserGameArgs{JobItemID: itemID}))
	}, commit)

	if n := ugCountFor(t, userID, 200); n != 1 {
		t.Errorf("user games on the rematch's match: want 1, got %d", n)
	}
	if n := ugCountFor(t, userID, 100); n != 0 {
		t.Errorf("user games on the old match: want 0, got %d", n)
	}
	if s, _ := famItemStatus(t, itemID); s != "completed" {
		t.Errorf("item: want completed, got %s", s)
	}
	if n := famCount(t, `SELECT COUNT(*) FROM external_games WHERE id = ? AND library_written_at IS NOT NULL`, egID); n != 1 {
		t.Error("library_written_at not set by the write")
	}
}

func TestIGDBMatchWorker_ChildInheritsTheMatchOfARematchInProgress(t *testing.T) {
	userID := famSetup(t)
	parentID := famInsertGame(t, userID, famGame{extID: "p", available: true, resolved: 100})
	childID := famInsertGame(t, userID, famGame{extID: "c", available: true, parent: parentID})
	jobID := famInsertJob(t, userID, "processing", true)
	itemID := famInsertItem(t, jobID, userID, childID, "pending")

	// The rematch holds the parent's lock but has not reached the child yet.
	commit := rematchInTx(t, 200, parentID)
	rc := newTestRiverClient(t)
	runBlocked(t, func() error { return famMatch(t, rc, itemID) }, commit)

	if _, eg := famGet(t, userID, "c"); eg.Resolved == nil || *eg.Resolved != 200 {
		t.Errorf("child match: want 200, got %v", eg.Resolved)
	}
}

// igdbMatchWithSearchHook runs IGDBMatchWorker for itemID against a fake IGDB
// that returns results and calls during() while the search is in flight.
func igdbMatchWithSearchHook(t *testing.T, itemID string, results []map[string]any, during func()) {
	t.Helper()
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600, "token_type": "bearer"})
	}))
	t.Cleanup(tokenSrv.Close)
	igdbSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		during()
		_ = json.NewEncoder(w).Encode(results)
	}))
	t.Cleanup(igdbSrv.Close)

	w := &tasks.IGDBMatchWorker{DB: testDB, IGDBClient: newIGDBClientForTests(t, tokenSrv.URL, igdbSrv.URL), LC: newTestLC(t)}
	if err := w.Work(context.Background(), matchJob(t, tasks.IGDBMatchArgs{JobItemID: itemID})); err != nil {
		t.Fatalf("igdb_match: %v", err)
	}
}

func setMatch(t *testing.T, egID string, igdbID int32) {
	t.Helper()
	famEnsureGame(t, igdbID)
	if _, err := testDB.NewRaw(`UPDATE external_games SET resolved_igdb_id = ? WHERE id = ?`, igdbID, egID).Exec(context.Background()); err != nil {
		t.Fatalf("set match: %v", err)
	}
}

func TestIGDBMatchWorker_ConfidentMatchAfterRematch_KeepsTheRematch(t *testing.T) {
	userID := famSetup(t)
	egID := famInsertGame(t, userID, famGame{extID: "1", title: "Horizon", available: true})
	jobID := famInsertJob(t, userID, "processing", true)
	itemID := famInsertItem(t, jobID, userID, egID, "pending")

	igdbMatchWithSearchHook(t, itemID,
		[]map[string]any{{"id": 500, "name": "Horizon", "slug": "horizon"}},
		func() { setMatch(t, egID, 200) })

	if _, eg := famGet(t, userID, "1"); eg.Resolved == nil || *eg.Resolved != 200 {
		t.Errorf("match: want the rematch's 200, got %v", eg.Resolved)
	}
	if n := famCount(t, `SELECT COUNT(*) FROM river_job WHERE kind = 'user_game_write' AND args->>'job_item_id' = ?`, itemID); n != 1 {
		t.Errorf("user_game_write jobs: want 1, got %d", n)
	}
}

func TestIGDBMatchWorker_LowConfidenceAfterRematch_GoesToTheWriteStage(t *testing.T) {
	userID := famSetup(t)
	egID := famInsertGame(t, userID, famGame{extID: "1", title: "Horizon", available: true})
	jobID := famInsertJob(t, userID, "processing", true)
	itemID := famInsertItem(t, jobID, userID, egID, "pending")

	igdbMatchWithSearchHook(t, itemID,
		[]map[string]any{{"id": 501, "name": "Something else", "slug": "x"}},
		func() { setMatch(t, egID, 200) })

	if s, _ := famItemStatus(t, itemID); s != "pending" {
		t.Errorf("item: want pending (on to the write stage), got %s", s)
	}
	if n := famCount(t, `SELECT COUNT(*) FROM river_job WHERE kind = 'user_game_write' AND args->>'job_item_id' = ?`, itemID); n != 1 {
		t.Errorf("user_game_write jobs: want 1, got %d", n)
	}
}

func TestWriteExternalGameToLibrary_FailureRollsBackWithTheCaller(t *testing.T) {
	userID := famSetup(t)
	egID := famInsertGame(t, userID, famGame{extID: "1", available: true, resolved: 100})
	ctx := context.Background()

	err := testDB.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tasks.WriteExternalGameToLibrary(ctx, tx, userID, egID); err != nil {
			return err
		}
		return context.Canceled // the caller fails after the write
	})
	if err == nil {
		t.Fatal("expected the transaction to fail")
	}
	if n := ugCountFor(t, userID, 100); n != 0 {
		t.Errorf("user games after rollback: want 0, got %d", n)
	}
}
