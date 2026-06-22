package librarysmells

import (
	"context"

	"github.com/uptrace/bun"
)

var orphanGameCheck = Check{
	ID:          "orphan-game",
	Title:       "Orphan game",
	Description: "A game in your library with no platform or storefront recorded.",
	Tier:        TierInconsistency,
	Detect:      detectOrphanGame,
}

// detectOrphanGame flags non-wishlisted games with zero platform rows. A
// wishlisted game legitimately has no platforms, so it is excluded.
func detectOrphanGame(ctx context.Context, db *bun.DB, userID string) ([]FlaggedItem, error) {
	var items []FlaggedItem
	err := db.NewRaw(
		`SELECT ug.id AS user_game_id, ug.game_id, g.title, g.cover_art_url
		 FROM user_games ug
		 JOIN games g ON g.id = ug.game_id
		 WHERE ug.user_id = ?
		   AND ug.is_wishlisted = false
		   AND NOT EXISTS (SELECT 1 FROM user_game_platforms p WHERE p.user_game_id = ug.id)
		   AND NOT EXISTS (SELECT 1 FROM smell_ignores si
		                   WHERE si.user_id = ug.user_id AND si.user_game_id = ug.id AND si.check_id = ?)
		 ORDER BY g.title`,
		userID, "orphan-game",
	).Scan(ctx, &items)
	return items, err
}
