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

var storefrontLessCheck = Check{
	ID:          "storefront-less-platform",
	Title:       "Storefront-less platform",
	Description: "A platform entry with no storefront recorded. Physical is a real choice — NULL means unknown provenance.",
	Tier:        TierInconsistency,
	Detect:      detectStorefrontLess,
}

func detectStorefrontLess(ctx context.Context, db *bun.DB, userID string) ([]FlaggedItem, error) {
	var items []FlaggedItem
	err := db.NewRaw(
		`SELECT ug.id AS user_game_id, ug.game_id, g.title, g.cover_art_url,
		        p.id AS platform_row_id, p.platform, p.storefront,
		        pl.default_storefront AS suggested_storefront
		 FROM user_game_platforms p
		 JOIN user_games ug ON ug.id = p.user_game_id
		 JOIN games g ON g.id = ug.game_id
		 LEFT JOIN platforms pl ON pl.name = p.platform
		 WHERE ug.user_id = ?
		   AND p.storefront IS NULL
		   AND NOT EXISTS (SELECT 1 FROM smell_ignores si
		                   WHERE si.user_id = ug.user_id AND si.user_game_id = ug.id AND si.check_id = ?)
		 ORDER BY g.title`,
		userID, "storefront-less-platform",
	).Scan(ctx, &items)
	return items, err
}

var missingOwnershipCheck = Check{
	ID:          "missing-ownership-status",
	Title:       "Missing ownership status",
	Description: "A platform entry with no ownership status (owned, borrowed, …).",
	Tier:        TierInconsistency,
	Detect:      detectMissingOwnership,
}

func detectMissingOwnership(ctx context.Context, db *bun.DB, userID string) ([]FlaggedItem, error) {
	var items []FlaggedItem
	err := db.NewRaw(
		`SELECT ug.id AS user_game_id, ug.game_id, g.title, g.cover_art_url,
		        p.id AS platform_row_id, p.platform, p.storefront
		 FROM user_game_platforms p
		 JOIN user_games ug ON ug.id = p.user_game_id
		 JOIN games g ON g.id = ug.game_id
		 WHERE ug.user_id = ?
		   AND p.ownership_status IS NULL
		   AND NOT EXISTS (SELECT 1 FROM smell_ignores si
		                   WHERE si.user_id = ug.user_id AND si.user_game_id = ug.id AND si.check_id = ?)
		 ORDER BY g.title`,
		userID, "missing-ownership-status",
	).Scan(ctx, &items)
	return items, err
}

var impossibleAcquiredDateCheck = Check{
	ID:          "impossible-acquired-date",
	Title:       "Impossible acquired date",
	Description: "An acquired date in the future, or before the game was released.",
	Tier:        TierInconsistency,
	Detect:      detectImpossibleAcquiredDate,
}

func detectImpossibleAcquiredDate(ctx context.Context, db *bun.DB, userID string) ([]FlaggedItem, error) {
	var items []FlaggedItem
	err := db.NewRaw(
		`SELECT ug.id AS user_game_id, ug.game_id, g.title, g.cover_art_url,
		        p.id AS platform_row_id, p.platform, p.storefront,
		        CASE
		          WHEN p.acquired_date > now()::date THEN 'acquired date is in the future'
		          ELSE 'acquired before the game was released'
		        END AS detail
		 FROM user_game_platforms p
		 JOIN user_games ug ON ug.id = p.user_game_id
		 JOIN games g ON g.id = ug.game_id
		 WHERE ug.user_id = ?
		   AND p.acquired_date IS NOT NULL
		   AND (p.acquired_date > now()::date
		        OR (g.release_date IS NOT NULL AND p.acquired_date < g.release_date))
		   AND NOT EXISTS (SELECT 1 FROM smell_ignores si
		                   WHERE si.user_id = ug.user_id AND si.user_game_id = ug.id AND si.check_id = ?)
		 ORDER BY g.title`,
		userID, "impossible-acquired-date",
	).Scan(ctx, &items)
	return items, err
}

var invalidStorefrontCheck = Check{
	ID:          "invalid-storefront-for-platform",
	Title:       "Invalid storefront for platform",
	Description: "The platform/storefront combination is not a recognised pairing.",
	Tier:        TierInconsistency,
	Detect:      detectInvalidStorefront,
}

func detectInvalidStorefront(ctx context.Context, db *bun.DB, userID string) ([]FlaggedItem, error) {
	var items []FlaggedItem
	err := db.NewRaw(
		`SELECT ug.id AS user_game_id, ug.game_id, g.title, g.cover_art_url,
		        p.id AS platform_row_id, p.platform, p.storefront
		 FROM user_game_platforms p
		 JOIN user_games ug ON ug.id = p.user_game_id
		 JOIN games g ON g.id = ug.game_id
		 WHERE ug.user_id = ?
		   AND p.platform IS NOT NULL
		   AND p.storefront IS NOT NULL
		   AND NOT EXISTS (SELECT 1 FROM platform_storefronts ps
		                   WHERE ps.platform = p.platform AND ps.storefront = p.storefront)
		   AND NOT EXISTS (SELECT 1 FROM smell_ignores si
		                   WHERE si.user_id = ug.user_id AND si.user_game_id = ug.id AND si.check_id = ?)
		 ORDER BY g.title`,
		userID, "invalid-storefront-for-platform",
	).Scan(ctx, &items)
	return items, err
}
