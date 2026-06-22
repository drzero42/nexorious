package usergame

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

// ClearWishlist is the exported bulk counterpart of clearWishlistOnAcquire: it
// clears is_wishlisted for each of the user's games in userGameIDs that has at
// least one platform row (i.e. is actually in the library). The EXISTS guard
// keeps it from clearing a pure wishlist entry, and makes it idempotent. It is
// the auto-fix for the "wishlisted yet owned" library smell (#1144). Returns the
// number of rows updated.
func ClearWishlist(ctx context.Context, db *bun.DB, userID string, userGameIDs []string) (int, error) {
	if len(userGameIDs) == 0 {
		return 0, nil
	}
	var updated int
	err := db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		res, err := tx.NewRaw(
			`UPDATE user_games
			 SET is_wishlisted = false, updated_at = now()
			 WHERE id IN (?)
			   AND user_id = ?
			   AND is_wishlisted
			   AND EXISTS (SELECT 1 FROM user_game_platforms WHERE user_game_id = user_games.id)`,
			bun.List(userGameIDs), userID,
		).Exec(ctx)
		if err != nil {
			return fmt.Errorf("clear wishlist: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("rows affected: %w", err)
		}
		updated = int(n)
		return nil
	})
	return updated, err
}

// clearWishlistOnAcquire promotes a wishlisted user game to a library entry by
// clearing its is_wishlisted flag, once the game has at least one platform.
//
// It is the wishlist counterpart to promoteToInProgressIfPlayed: the usergame
// operations invoke it right after inserting user_game_platforms rows. The
// EXISTS guard keeps the invariant safe — it never clears a wishlisted row that
// somehow has no platforms — and makes the call idempotent. Accepts bun.IDB so
// it runs inside a caller's transaction when one is present.
func clearWishlistOnAcquire(ctx context.Context, db bun.IDB, userGameID string) error {
	_, err := db.NewRaw(
		`UPDATE user_games
		 SET is_wishlisted = false, updated_at = now()
		 WHERE id = ?
		   AND is_wishlisted
		   AND EXISTS (SELECT 1 FROM user_game_platforms WHERE user_game_id = ?)`,
		userGameID, userGameID,
	).Exec(ctx)
	return err
}
