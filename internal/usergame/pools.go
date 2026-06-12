package usergame

import (
	"context"

	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/enum"
)

// RemoveFromPoolsIfFinished deletes every pool membership for a user game when
// its current play_status is in the finished set (#955). It re-reads the row's
// play_status via an EXISTS guard, so it is idempotent and safe to call
// unconditionally on any play-status write path — it deletes only when the new
// status is finished. Mirrors ClearWishlistOnAcquire: accepts bun.IDB so it
// runs inside a caller's transaction. No queue renumber — gaps are tolerated by
// the data model and the next explicit queue write renumbers contiguous.
func RemoveFromPoolsIfFinished(ctx context.Context, db bun.IDB, userGameID string) error {
	_, err := db.NewRaw(
		`DELETE FROM pool_games
		 WHERE user_game_id = ?
		   AND EXISTS (
		       SELECT 1 FROM user_games
		       WHERE id = ? AND play_status IN (?)
		   )`,
		userGameID, userGameID, bun.List(enum.FinishedPlayStatusStrings()),
	).Exec(ctx)
	return err
}
