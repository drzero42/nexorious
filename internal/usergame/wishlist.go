package usergame

import (
	"context"

	"github.com/uptrace/bun"
)

// ClearWishlistOnAcquire promotes a wishlisted user game to a library entry by
// clearing its is_wishlisted flag, once the game has at least one platform.
//
// It is the wishlist counterpart to PromoteToInProgressIfPlayed: callers invoke
// it right after inserting user_game_platforms rows for a user game, from every
// insert path (API create/attach/bulk, sync, import). The EXISTS
// guard keeps the invariant safe — it never clears a wishlisted row that somehow
// has no platforms — and makes the call idempotent. Accepts bun.IDB so it runs
// inside a caller's transaction when one is present.
func ClearWishlistOnAcquire(ctx context.Context, db bun.IDB, userGameID string) error {
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
