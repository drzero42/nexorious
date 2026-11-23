package tasks_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/drzero42/nexorious/internal/joblifecycle"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

// syncDispatchJob makes the test's River job the dispatch owner of a sync Job
// a test inserted directly (older rows default to dispatch_complete = true) and
// returns it. A Job that does not exist or is finished is left alone.
func syncDispatchJob(t *testing.T, args tasks.DispatchSyncArgs) *river.Job[tasks.DispatchSyncArgs] {
	t.Helper()
	owner := nextFakeOwner()
	if args.JobID != "" {
		if _, err := testDB.ExecContext(context.Background(),
			`UPDATE jobs SET dispatch_complete = false, river_job_id = ?
			 WHERE id = ? AND status IN ('pending', 'processing') AND river_job_id IS NULL`,
			owner, args.JobID); err != nil {
			t.Fatalf("own dispatch: %v", err)
		}
		var rid *int64
		if err := testDB.NewRaw(`SELECT river_job_id FROM jobs WHERE id = ?`, args.JobID).Scan(context.Background(), &rid); err == nil && rid != nil {
			owner = *rid
		}
	}
	return &river.Job[tasks.DispatchSyncArgs]{JobRow: &rivertype.JobRow{ID: owner, Attempt: 1, MaxAttempts: 3}, Args: args}
}

// ownItem returns the River job that owns a pending item, recording a fake
// owner first when the test inserted the item directly.
func ownItem(t *testing.T, itemID string) int64 {
	t.Helper()
	ctx := context.Background()
	if _, err := testDB.ExecContext(ctx,
		`UPDATE job_items SET river_job_id = ? WHERE id = ? AND river_job_id IS NULL AND status = 'pending'`,
		nextFakeOwner(), itemID); err != nil {
		t.Fatalf("own item: %v", err)
	}
	var rid *int64
	if err := testDB.NewRaw(`SELECT river_job_id FROM job_items WHERE id = ?`, itemID).Scan(ctx, &rid); err != nil || rid == nil {
		return nextFakeOwner() // not pending or gone: any id is refused
	}
	return *rid
}

func matchJob(t *testing.T, args tasks.IGDBMatchArgs) *river.Job[tasks.IGDBMatchArgs] {
	t.Helper()
	return riverJob(ownItem(t, args.JobItemID), 1, args)
}

func writeJob(t *testing.T, args tasks.UserGameArgs) *river.Job[tasks.UserGameArgs] {
	t.Helper()
	return riverJob(ownItem(t, args.JobItemID), 1, args)
}

// runSyncEffects runs every queued finishing-effects River job, as River would,
// and marks each completed. It returns how many it ran.
func runSyncEffects(t *testing.T, lc *joblifecycle.Module) int {
	t.Helper()
	ctx := context.Background()
	var rows []struct {
		ID          int64           `bun:"id"`
		Args        json.RawMessage `bun:"args"`
		Attempt     int             `bun:"attempt"`
		MaxAttempts int             `bun:"max_attempts"`
	}
	if err := testDB.NewRaw(
		`SELECT id, args, attempt, max_attempts FROM river_job WHERE kind = ? AND state = 'available' ORDER BY id`,
		joblifecycle.EffectsArgs{}.Kind(),
	).Scan(ctx, &rows); err != nil {
		t.Fatalf("load effects: %v", err)
	}
	w := &joblifecycle.EffectsWorker{Module: lc}
	for _, r := range rows {
		var args joblifecycle.EffectsArgs
		if err := json.Unmarshal(r.Args, &args); err != nil {
			t.Fatalf("effects args: %v", err)
		}
		if err := w.Work(ctx, &river.Job[joblifecycle.EffectsArgs]{
			JobRow: &rivertype.JobRow{ID: r.ID, Attempt: 1, MaxAttempts: r.MaxAttempts}, Args: args,
		}); err != nil {
			t.Fatalf("effects: %v", err)
		}
		if _, err := testDB.ExecContext(ctx, `UPDATE river_job SET state = 'completed', finalized_at = now() WHERE id = ?`, r.ID); err != nil {
			t.Fatal(err)
		}
	}
	return len(rows)
}

// drainSync runs the queued igdb_match and user_game_write River jobs of the
// sync pipeline until none is left, as River would, marking each completed.
func drainSync(t *testing.T, match *tasks.IGDBMatchWorker, write *tasks.UserGameWorker) {
	t.Helper()
	ctx := context.Background()
	for range 50 {
		var rows []struct {
			ID   int64           `bun:"id"`
			Kind string          `bun:"kind"`
			Args json.RawMessage `bun:"args"`
		}
		if err := testDB.NewRaw(
			`SELECT id, kind, args FROM river_job WHERE kind IN ('igdb_match', 'user_game_write') AND state = 'available' ORDER BY id`,
		).Scan(ctx, &rows); err != nil {
			t.Fatalf("load queued sync work: %v", err)
		}
		if len(rows) == 0 {
			return
		}
		for _, r := range rows {
			var err error
			switch r.Kind {
			case "igdb_match":
				var a tasks.IGDBMatchArgs
				_ = json.Unmarshal(r.Args, &a)
				err = match.Work(ctx, riverJob(r.ID, 1, a))
			default:
				var a tasks.UserGameArgs
				_ = json.Unmarshal(r.Args, &a)
				err = write.Work(ctx, riverJob(r.ID, 1, a))
			}
			if err != nil {
				t.Fatalf("%s %d: %v", r.Kind, r.ID, err)
			}
			if _, err := testDB.ExecContext(ctx, `UPDATE river_job SET state = 'completed', finalized_at = now() WHERE id = ?`, r.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	t.Fatal("sync pipeline did not drain")
}
