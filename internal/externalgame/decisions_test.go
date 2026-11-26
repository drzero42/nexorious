package externalgame

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// inLockedSet locks id's editions in a transaction and runs fn on them; fn's
// error rolls the transaction back and is returned.
func inLockedSet(t *testing.T, userID, id string, fn func(ctx context.Context, set *Editions) error) error {
	t.Helper()
	return testDB.RunInTx(context.Background(), nil, func(ctx context.Context, tx bun.Tx) error {
		set, err := LockEditions(ctx, tx, userID, id)
		if err != nil {
			return err
		}
		return fn(ctx, set)
	})
}

// libraryGame writes the matched External game id to the library and returns
// the user game it is linked to.
func libraryGame(t *testing.T, userID, id string) string {
	t.Helper()
	lw, err := WriteToLibrary(context.Background(), testDB, userID, id)
	if err != nil {
		t.Fatalf("write to library: %v", err)
	}
	return lw.Result.UserGameID
}

func userGameExists(t *testing.T, id string) bool {
	t.Helper()
	var ok bool
	if err := testDB.NewRaw(`SELECT EXISTS (SELECT 1 FROM user_games WHERE id = ?)`, id).
		Scan(context.Background(), &ok); err != nil {
		t.Fatalf("find user game: %v", err)
	}
	return ok
}

func TestLockEditions_MissingGame(t *testing.T) {
	truncateAllTables(t)
	userID := seedUser(t)
	other := seedUser(t)
	theirs := seedExternalGame(t, other, extGame{resolved: 100, platforms: true})

	for name, id := range map[string]string{"unknown id": uuid.NewString(), "another user's game": theirs} {
		t.Run(name, func(t *testing.T) {
			err := inLockedSet(t, userID, id, func(context.Context, *Editions) error { return nil })
			if !errors.Is(err, ErrNotFound) {
				t.Fatalf("want ErrNotFound, got %v", err)
			}
		})
	}
}

// Changed is the whole set, or with the option the given game alone.
func TestEditions_Changed(t *testing.T) {
	truncateAllTables(t)
	userID := seedUser(t)
	a := seedExternalGame(t, userID, extGame{resolved: 100, platforms: true})
	b := seedExternalGame(t, userID, extGame{resolved: 100, platforms: true})
	other := seedExternalGame(t, userID, extGame{resolved: 101, platforms: true}) // another match: not an edition
	set := []string{a, b}
	slices.Sort(set)

	err := inLockedSet(t, userID, b, func(_ context.Context, e *Editions) error {
		if got := e.Changed(false); !slices.Equal(got, set) {
			t.Errorf("Changed(false) = %v, want %v", got, set)
		}
		if got := e.Changed(true); !slices.Equal(got, []string{b}) {
			t.Errorf("Changed(true) = %v, want [%s]", got, b)
		}
		if slices.Contains(e.IDs(), other) {
			t.Errorf("set %v holds %s, which has another match", e.IDs(), other)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// The orphan rule: a Rematch that would leave a user game with no storefront
// link needs an orphan action, and then keeps or removes that user game.
func TestEditions_RematchOrphanRule(t *testing.T) {
	rematch := func(t *testing.T, userID, id string, igdbID int, action string) error {
		t.Helper()
		return inLockedSet(t, userID, id, func(ctx context.Context, e *Editions) error {
			_, err := e.Rematch(ctx, RematchRequest{IGDBID: igdbID, OrphanAction: action})
			return err
		})
	}

	t.Run("no action refuses and changes nothing", func(t *testing.T) {
		truncateAllTables(t)
		userID := seedUser(t)
		eg := seedExternalGame(t, userID, extGame{resolved: 100, platforms: true})
		ug := libraryGame(t, userID, eg)

		err := rematch(t, userID, eg, 200, "")
		if !errors.Is(err, ErrOrphanActionRequired) {
			t.Fatalf("want ErrOrphanActionRequired, got %v", err)
		}
		var match int32
		var links int
		if err := testDB.NewRaw(`SELECT resolved_igdb_id FROM external_games WHERE id = ?`, eg).Scan(context.Background(), &match); err != nil {
			t.Fatal(err)
		}
		if err := testDB.NewRaw(`SELECT COUNT(*) FROM user_game_platforms WHERE user_game_id = ? AND external_game_id = ?`, ug, eg).Scan(context.Background(), &links); err != nil {
			t.Fatal(err)
		}
		if match != 100 || links != 1 || !userGameExists(t, ug) {
			t.Errorf("after the refusal: match %d (want 100), links %d (want 1), user game kept %v (want true)", match, links, userGameExists(t, ug))
		}
		if n := userGameCount(t, userID, 200); n != 0 {
			t.Errorf("user games of the new match: want 0, got %d", n)
		}
	})

	for _, tc := range []struct {
		action string
		kept   bool
	}{{"keep", true}, {OrphanRemove, false}} {
		t.Run(tc.action, func(t *testing.T) {
			truncateAllTables(t)
			userID := seedUser(t)
			eg := seedExternalGame(t, userID, extGame{resolved: 100, platforms: true})
			ug := libraryGame(t, userID, eg)

			if err := rematch(t, userID, eg, 200, tc.action); err != nil {
				t.Fatalf("rematch: %v", err)
			}
			if got := userGameExists(t, ug); got != tc.kept {
				t.Errorf("old user game kept: got %v, want %v", got, tc.kept)
			}
			if n := userGameCount(t, userID, 200); n != 1 {
				t.Errorf("user games of the new match: want 1, got %d", n)
			}
		})
	}

	// Rematching onto the match the set already has rewrites the same user
	// game's links: nothing is orphaned, so nothing is asked.
	t.Run("onto the current match asks nothing", func(t *testing.T) {
		truncateAllTables(t)
		userID := seedUser(t)
		eg := seedExternalGame(t, userID, extGame{resolved: 100, platforms: true})
		ug := libraryGame(t, userID, eg)

		if err := rematch(t, userID, eg, 100, ""); err != nil {
			t.Fatalf("rematch: %v", err)
		}
		if !userGameExists(t, ug) {
			t.Error("the user game was removed")
		}
	})
}

// A matched game with no platform rows keeps its match and is not written:
// no error, so a Rematch or Unskip of it still succeeds.
func TestWriteMatched_NoPlatforms(t *testing.T) {
	truncateAllTables(t)
	userID := seedUser(t)
	eg := seedExternalGame(t, userID, extGame{resolved: 100})

	_, wrote, err := WriteMatched(context.Background(), testDB, userID, eg)
	if err != nil || wrote {
		t.Fatalf("want wrote=false and no error, got wrote=%v err=%v", wrote, err)
	}
	if n := userGameCount(t, userID, 100); n != 0 {
		t.Errorf("user games: want 0, got %d", n)
	}
}

// An id outside int32 is refused, never wrapped onto another game: 1<<32+100
// would wrap to 100, which exists.
func TestEditions_RematchIDOutsideInt32(t *testing.T) {
	truncateAllTables(t)
	userID := seedUser(t)
	seedGame(t, 100, "Horizon")
	eg := seedExternalGame(t, userID, extGame{platforms: true})

	err := inLockedSet(t, userID, eg, func(ctx context.Context, e *Editions) error {
		_, err := e.Rematch(ctx, RematchRequest{IGDBID: 1<<32 + 100})
		return err
	})
	if err == nil {
		t.Fatal("want the rematch refused, got no error")
	}
	if n := userGameCount(t, userID, 100); n != 0 {
		t.Errorf("user games of the wrapped id: want 0, got %d", n)
	}
}
