package notify

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestPruneEventsDeletesOld(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	SetRiverClient(nil)

	oldID := uuid.NewString()
	if _, err := testDB.NewRaw(
		`INSERT INTO events (id, type, scope, payload, occurred_at)
		 VALUES (?, ?, ?, '{}'::jsonb, now() - interval '100 days')`,
		oldID, TypeSyncFailed, ScopeUser,
	).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	freshID := uuid.NewString()
	if _, err := testDB.NewRaw(
		`INSERT INTO events (id, type, scope, payload, occurred_at)
		 VALUES (?, ?, ?, '{}'::jsonb, now())`,
		freshID, TypeSyncFailed, ScopeUser,
	).Exec(ctx); err != nil {
		t.Fatal(err)
	}

	PruneEvents(ctx, testDB, 90)

	// old event must be deleted
	var oldCount int
	if err := testDB.NewRaw(`SELECT COUNT(*) FROM events WHERE id = ?`, oldID).Scan(ctx, &oldCount); err != nil {
		t.Fatal(err)
	}
	if oldCount != 0 {
		t.Fatalf("old event should be deleted, still present (count=%d)", oldCount)
	}
	// fresh event must survive
	var freshCount int
	if err := testDB.NewRaw(`SELECT COUNT(*) FROM events WHERE id = ?`, freshID).Scan(ctx, &freshCount); err != nil {
		t.Fatal(err)
	}
	if freshCount != 1 {
		t.Fatalf("fresh event should survive, got count=%d", freshCount)
	}
	// prune emits a maintenance completed event
	var maintCount int
	if err := testDB.NewRaw(`SELECT COUNT(*) FROM events WHERE type = ?`, TypeAdminMaintCompleted).Scan(ctx, &maintCount); err != nil {
		t.Fatal(err)
	}
	if maintCount != 1 {
		t.Fatalf("expected 1 maintenance completed event emitted by prune, got %d", maintCount)
	}
}
