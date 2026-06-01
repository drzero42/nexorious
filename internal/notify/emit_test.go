package notify

import (
	"context"
	"testing"

	"github.com/google/uuid"
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
