package usergame

import (
	"context"

	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/enum"
)

// promoteToInProgressIfPlayed flips a user game's play_status from
// 'not_started' to 'in_progress' when the game has accrued any played hours
// across its platforms.
//
// The update is idempotent and atomic: the play_status = 'not_started' guard
// guarantees we never clobber a user-chosen status such as 'completed' or
// 'shelved', and the hours check sums all of the game's platforms (not just the
// ones a given caller touched). Callers should invoke this after writing the
// hours_played values for the user game.
func promoteToInProgressIfPlayed(ctx context.Context, db bun.IDB, userGameID string) error {
	_, err := db.NewRaw(
		`UPDATE user_games
		 SET play_status = ?, updated_at = now()
		 WHERE id = ?
		   AND play_status = ?
		   AND (SELECT COALESCE(SUM(hours_played), 0)
		        FROM user_game_platforms WHERE user_game_id = ?) > 0`,
		string(enum.PlayStatusInProgress), userGameID, string(enum.PlayStatusNotStarted), userGameID,
	).Exec(ctx)
	return err
}
