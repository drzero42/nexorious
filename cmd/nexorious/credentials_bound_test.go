package main

import (
	"context"
	"testing"
)

func seedStoredSignIn(t *testing.T, userID, storefront, ciphertext string) {
	t.Helper()
	if _, err := testDB.ExecContext(context.Background(),
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials) VALUES (gen_random_uuid(), ?, ?, 'daily', ?)`,
		userID, storefront, ciphertext,
	); err != nil {
		t.Fatalf("seed config: %v", err)
	}
}

func storedSignIn(t *testing.T, userID, storefront string) string {
	t.Helper()
	var ct string
	if err := testDB.NewRaw(`SELECT storefront_credentials FROM user_sync_configs WHERE user_id = ? AND storefront = ?`, userID, storefront).
		Scan(context.Background(), &ct); err != nil {
		t.Fatalf("read config: %v", err)
	}
	return ct
}

// A sync's refreshed sign-in replaces the stored one while it is still the one
// the sync read or last wrote, so a sync that refreshes twice keeps both.
func TestStoredCredentialsWriter_SavesWhileStillStored(t *testing.T) {
	truncateAllTables(t)
	userID := seedResetUser(t, "writer-same", "password123", false)
	seedStoredSignIn(t, userID, "gog", "loaded-ct")
	w := newStoredCredentialsWriter(testDB, userID, "gog", "loaded-ct")
	ctx := context.Background()
	if err := w.write(ctx, "refreshed-1"); err != nil {
		t.Fatal(err)
	}
	if got := storedSignIn(t, userID, "gog"); got != "refreshed-1" {
		t.Fatalf("after one refresh stored %q, want refreshed-1", got)
	}
	if err := w.write(ctx, "refreshed-2"); err != nil {
		t.Fatal(err)
	}
	if got := storedSignIn(t, userID, "gog"); got != "refreshed-2" {
		t.Errorf("after a second refresh stored %q, want refreshed-2", got)
	}
}

// A reconnect committed while the sync ran is kept: the older sign-in's
// refreshed value is dropped without an error that would fail the sync.
func TestStoredCredentialsWriter_KeepsReconnect(t *testing.T) {
	truncateAllTables(t)
	userID := seedResetUser(t, "writer-reconnect", "password123", false)
	seedStoredSignIn(t, userID, "epic-games-store", "loaded-ct")
	w := newStoredCredentialsWriter(testDB, userID, "epic-games-store", "loaded-ct")
	if _, err := testDB.ExecContext(context.Background(),
		`UPDATE user_sync_configs SET storefront_credentials = 'reconnected-ct' WHERE user_id = ?`, userID); err != nil {
		t.Fatal(err)
	}
	if err := w.write(context.Background(), "refreshed-old"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := storedSignIn(t, userID, "epic-games-store"); got != "reconnected-ct" {
		t.Errorf("stored %q, want the reconnected sign-in kept", got)
	}
}

// A disconnect committed while the sync ran stays a disconnect: the sync's
// refreshed sign-in must not bring the credentials back (#1335).
func TestStoredCredentialsWriter_KeepsDisconnect(t *testing.T) {
	truncateAllTables(t)
	userID := seedResetUser(t, "writer-disconnect", "password123", false)
	seedStoredSignIn(t, userID, "epic-games-store", "loaded-ct")
	w := newStoredCredentialsWriter(testDB, userID, "epic-games-store", "loaded-ct")
	if _, err := testDB.ExecContext(context.Background(),
		`UPDATE user_sync_configs SET storefront_credentials = NULL WHERE user_id = ?`, userID); err != nil {
		t.Fatal(err)
	}
	if err := w.write(context.Background(), "refreshed-old"); err != nil {
		t.Fatalf("write: %v", err)
	}
	var stored *string
	if err := testDB.NewRaw(`SELECT storefront_credentials FROM user_sync_configs WHERE user_id = ?`, userID).
		Scan(context.Background(), &stored); err != nil {
		t.Fatal(err)
	}
	if stored != nil {
		t.Errorf("stored %q after disconnect, want NULL", *stored)
	}
}
