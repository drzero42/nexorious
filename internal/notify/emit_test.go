package notify

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	riverdatabasesql "github.com/riverqueue/river/riverdriver/riverdatabasesql"
)

func TestEmitInsertsEventRow(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	SetRiverClient(nil)

	uid := insertEmitUser(t, "emit-user-1")

	Emit(ctx, testDB, EmitParams{
		Type:        TypeSyncFailed,
		Scope:       ScopeUser,
		ActorUserID: uid,
		Payload:     map[string]any{"job_id": "job-1"},
		DedupKey:    "job-1:sync.failed",
	})

	var count int
	if err := testDB.NewRaw(`SELECT COUNT(*) FROM events WHERE type = ?`, TypeSyncFailed).Scan(ctx, &count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 event row, got %d", count)
	}
}

func TestEmitDedupFiresOnce(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	SetRiverClient(nil)

	uid := insertEmitUser(t, "emit-user-2")

	for i := 0; i < 3; i++ {
		Emit(ctx, testDB, EmitParams{
			Type: TypeSyncCompleted, Scope: ScopeUser, ActorUserID: uid,
			DedupKey: "job-9:sync.completed",
		})
	}
	var count int
	if err := testDB.NewRaw(`SELECT COUNT(*) FROM events WHERE dedup_key = ?`, "job-9:sync.completed").Scan(ctx, &count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("dedup failed: expected 1 row, got %d", count)
	}
}

func TestEmitNullDedupAlwaysInserts(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	SetRiverClient(nil)

	for i := 0; i < 3; i++ {
		Emit(ctx, testDB, EmitParams{Type: TypeAdminMaintCompleted, Scope: ScopeAdmin})
	}
	var count int
	if err := testDB.NewRaw(`SELECT COUNT(*) FROM events WHERE type = ?`, TypeAdminMaintCompleted).Scan(ctx, &count); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("expected 3 repeatable rows, got %d", count)
	}
}

func insertEmitUser(t *testing.T, username string) string {
	t.Helper()
	id := uuid.NewString()
	if _, err := testDB.NewRaw(
		`INSERT INTO users (id, username, password_hash) VALUES (?, ?, 'x')`,
		id, username,
	).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	return id
}

// EmitTx writes the event and its delivery River job through the caller's
// transaction: rolled back, neither remains; committed, both do. A retry after
// a failure between the two writes can then never find the event without its
// delivery job.
func TestEmitTxFollowsTheTransaction(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	SetRiverClient(nil)
	rc, err := river.NewClient(riverdatabasesql.New(testDB.DB), &river.Config{})
	if err != nil {
		t.Fatal(err)
	}
	SetTxRiverClient(rc)
	t.Cleanup(func() { SetTxRiverClient(nil) })
	uid := insertEmitUser(t, "emit-tx-user")
	params := func(key string) EmitParams {
		return EmitParams{Type: TypeSyncFailed, Scope: ScopeUser, ActorUserID: uid, DedupKey: key}
	}
	count := func(key string) (events, deliveries int) {
		if err := testDB.NewRaw(`SELECT COUNT(*) FROM events WHERE dedup_key = ?`, key).Scan(ctx, &events); err != nil {
			t.Fatal(err)
		}
		if err := testDB.NewRaw(
			`SELECT COUNT(*) FROM river_job WHERE kind = 'notify' AND args->>'event_id' IN (SELECT id FROM events WHERE dedup_key = ?)`, key,
		).Scan(ctx, &deliveries); err != nil {
			t.Fatal(err)
		}
		return events, deliveries
	}

	tx, err := testDB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := EmitTx(ctx, tx, params("rolled-back")); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if e, d := count("rolled-back"); e != 0 || d != 0 {
		t.Errorf("rolled-back transaction left %d events, %d deliveries", e, d)
	}

	for range 2 { // the second is deduplicated: no second event, no second delivery
		tx, err = testDB.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := EmitTx(ctx, tx, params("committed")); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	if e, d := count("committed"); e != 1 || d != 1 {
		t.Errorf("committed transaction left %d events, %d deliveries; want 1, 1", e, d)
	}
}
