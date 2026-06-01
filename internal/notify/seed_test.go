package notify

import (
	"context"
	"testing"
)

func TestSeedDefaultSubscriptions(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	uid := insertWorkerUser(t, "seeded", false)

	if err := SeedDefaultSubscriptions(ctx, testDB, uid); err != nil {
		t.Fatal(err)
	}

	var rows []struct {
		EventType string `bun:"event_type"`
	}
	if err := testDB.NewRaw(`SELECT event_type FROM notification_subscriptions WHERE user_id = ?`, uid).Scan(ctx, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != len(DefaultSubscriptions()) {
		t.Fatalf("expected %d default subs, got %d", len(DefaultSubscriptions()), len(rows))
	}
}

func TestSeedIsIdempotent(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	uid := insertWorkerUser(t, "seeded2", false)
	if err := SeedDefaultSubscriptions(ctx, testDB, uid); err != nil {
		t.Fatal(err)
	}
	if err := SeedDefaultSubscriptions(ctx, testDB, uid); err != nil {
		t.Fatalf("second seed should not error: %v", err)
	}
}
