package notify

import (
	"context"
	"testing"
)

// nonAdminDefaultCount returns the number of default subscriptions that are
// not admin-scoped, i.e. the set a non-admin user should receive.
func nonAdminDefaultCount() int {
	n := 0
	for _, eventType := range DefaultSubscriptions() {
		if !IsAdminType(eventType) {
			n++
		}
	}
	return n
}

func TestSeedDefaultSubscriptionsAdmin(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	uid := insertWorkerUser(t, "seeded-admin", true)

	if err := SeedDefaultSubscriptions(ctx, testDB, uid, true); err != nil {
		t.Fatal(err)
	}

	var rows []struct {
		EventType string `bun:"event_type"`
	}
	if err := testDB.NewRaw(`SELECT event_type FROM notification_subscriptions WHERE user_id = ?`, uid).Scan(ctx, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != len(DefaultSubscriptions()) {
		t.Fatalf("admin: expected %d default subs, got %d", len(DefaultSubscriptions()), len(rows))
	}
}

func TestSeedDefaultSubscriptionsNonAdmin(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	uid := insertWorkerUser(t, "seeded-nonadmin", false)

	if err := SeedDefaultSubscriptions(ctx, testDB, uid, false); err != nil {
		t.Fatal(err)
	}

	var rows []struct {
		EventType string `bun:"event_type"`
	}
	if err := testDB.NewRaw(`SELECT event_type FROM notification_subscriptions WHERE user_id = ?`, uid).Scan(ctx, &rows); err != nil {
		t.Fatal(err)
	}
	want := nonAdminDefaultCount()
	if len(rows) != want {
		t.Fatalf("non-admin: expected %d default subs, got %d", want, len(rows))
	}
	for _, r := range rows {
		if IsAdminType(r.EventType) {
			t.Fatalf("non-admin should not be seeded admin-scoped type %q", r.EventType)
		}
	}
}

func TestSeedIsIdempotent(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	uid := insertWorkerUser(t, "seeded2", false)
	if err := SeedDefaultSubscriptions(ctx, testDB, uid, false); err != nil {
		t.Fatal(err)
	}
	if err := SeedDefaultSubscriptions(ctx, testDB, uid, false); err != nil {
		t.Fatalf("second seed should not error: %v", err)
	}
}
