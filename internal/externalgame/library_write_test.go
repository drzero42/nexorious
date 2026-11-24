package externalgame

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

func TestWriteToLibrary_FailureRollsBackWithTheCaller(t *testing.T) {
	truncateAllTables(t)
	userID := seedUser(t)
	egID := seedExternalGame(t, userID, extGame{resolved: 100, platforms: true})
	ctx := context.Background()

	err := testDB.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := WriteToLibrary(ctx, tx, userID, egID); err != nil {
			return err
		}
		return context.Canceled // the caller fails after the write
	})
	if err == nil {
		t.Fatal("expected the transaction to fail")
	}
	if n := userGameCount(t, userID, 100); n != 0 {
		t.Errorf("user games after rollback: want 0, got %d", n)
	}
}

// The refusals callers branch on: the sync worker skips or fails its item,
// a rematch or unskip leaves the game out of the library write.
func TestWriteToLibrary_Refusals(t *testing.T) {
	truncateAllTables(t)
	userID := seedUser(t)
	otherUser := seedUser(t)

	cases := []struct {
		name string
		id   string
		want error
	}{
		{"unknown id", uuid.NewString(), ErrNotFound},
		{"another user's game", seedExternalGame(t, otherUser, extGame{resolved: 100, platforms: true}), ErrNotFound},
		// Skipped is checked before the match, so a skipped game with a match
		// and platforms is still refused as skipped.
		{"skipped", seedExternalGame(t, userID, extGame{skipped: true, resolved: 101, platforms: true}), ErrSkipped},
		{"unmatched", seedExternalGame(t, userID, extGame{platforms: true}), ErrUnmatched},
		{"no platform rows", seedExternalGame(t, userID, extGame{resolved: 102}), ErrNoPlatforms},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := WriteToLibrary(context.Background(), testDB, userID, tc.id)
			if !errors.Is(err, tc.want) {
				t.Fatalf("want %v, got %v", tc.want, err)
			}
		})
	}
	var n int
	if err := testDB.NewRaw(`SELECT COUNT(*) FROM user_games`).Scan(context.Background(), &n); err != nil {
		t.Fatalf("count user games: %v", err)
	}
	if n != 0 {
		t.Errorf("user games after refusals: want 0, got %d", n)
	}
}
