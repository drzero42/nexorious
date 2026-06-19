package usergame

import (
	"context"
	"errors"
	"testing"
)

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

func TestDelete_OwnedRow(t *testing.T) {
	truncateAllTables(t)
	userID := seedUser(t)
	ugID := seedUserGame(t, userID, 1001)

	if err := Delete(context.Background(), testDB, DeleteParams{UserID: userID, UserGameID: ugID}); err != nil {
		t.Fatalf("Delete own row: %v", err)
	}

	// Row must be gone.
	var count int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM user_games WHERE id = ?`, ugID).Scan(context.Background(), &count)
	if count != 0 {
		t.Fatalf("expected 0 rows after delete, got %d", count)
	}
}

func TestDelete_OtherUsersRow(t *testing.T) {
	truncateAllTables(t)
	owner := seedUser(t)
	caller := seedUser(t)
	ugID := seedUserGame(t, owner, 1002)

	err := Delete(context.Background(), testDB, DeleteParams{UserID: caller, UserGameID: ugID})
	if err == nil {
		t.Fatal("expected ErrNotFound when deleting another user's row")
	}
	if !isNotFound(err) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	// Row must still exist.
	var count int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM user_games WHERE id = ?`, ugID).Scan(context.Background(), &count)
	if count != 1 {
		t.Fatalf("expected owner's row to still exist, got %d", count)
	}
}

func TestDelete_NonExistentRow(t *testing.T) {
	truncateAllTables(t)
	userID := seedUser(t)

	err := Delete(context.Background(), testDB, DeleteParams{UserID: userID, UserGameID: "00000000-0000-0000-0000-000000000000"})
	if !isNotFound(err) {
		t.Fatalf("expected ErrNotFound for non-existent id, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// DeleteBulk
// ---------------------------------------------------------------------------

func TestDeleteBulk_OnlyOwnedRows(t *testing.T) {
	truncateAllTables(t)
	caller := seedUser(t)
	other := seedUser(t)

	ownID1 := seedUserGame(t, caller, 2001)
	ownID2 := seedUserGame(t, caller, 2002)
	otherID := seedUserGame(t, other, 2003)

	deleted, err := DeleteBulk(context.Background(), testDB, BulkDeleteParams{
		UserID:      caller,
		UserGameIDs: []string{ownID1, ownID2, otherID},
	})
	if err != nil {
		t.Fatalf("DeleteBulk: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("expected 2 deleted, got %d", deleted)
	}

	// other's row must survive.
	var count int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM user_games WHERE id = ?`, otherID).Scan(context.Background(), &count)
	if count != 1 {
		t.Fatalf("expected other user's row to survive, got %d", count)
	}
}

func TestDeleteBulk_ReturnsZeroWhenNoneOwned(t *testing.T) {
	truncateAllTables(t)
	caller := seedUser(t)
	other := seedUser(t)

	otherID := seedUserGame(t, other, 2004)

	deleted, err := DeleteBulk(context.Background(), testDB, BulkDeleteParams{
		UserID:      caller,
		UserGameIDs: []string{otherID},
	})
	if err != nil {
		t.Fatalf("DeleteBulk: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("expected 0 deleted, got %d", deleted)
	}
}

// ---------------------------------------------------------------------------
// ClearLibrary
// ---------------------------------------------------------------------------

func TestClearLibrary_RemovesAllUserRows(t *testing.T) {
	truncateAllTables(t)
	caller := seedUser(t)
	other := seedUser(t)

	seedUserGame(t, caller, 3001)
	seedUserGame(t, caller, 3002)
	otherID := seedUserGame(t, other, 3003)

	deleted, err := ClearLibrary(context.Background(), testDB, caller)
	if err != nil {
		t.Fatalf("ClearLibrary: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("expected 2 deleted, got %d", deleted)
	}

	// Caller's rows must be gone.
	var callerCount int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM user_games WHERE user_id = ?`, caller).Scan(context.Background(), &callerCount)
	if callerCount != 0 {
		t.Fatalf("expected caller's rows to be gone, got %d", callerCount)
	}

	// Other user's row must survive.
	var count int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM user_games WHERE id = ?`, otherID).Scan(context.Background(), &count)
	if count != 1 {
		t.Fatalf("expected other user's row to survive, got %d", count)
	}
}

func TestClearLibrary_ReturnsZeroWhenEmpty(t *testing.T) {
	truncateAllTables(t)
	caller := seedUser(t)

	deleted, err := ClearLibrary(context.Background(), testDB, caller)
	if err != nil {
		t.Fatalf("ClearLibrary on empty library: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("expected 0 deleted, got %d", deleted)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func isNotFound(err error) bool {
	return err != nil && errors.Is(err, ErrNotFound)
}
