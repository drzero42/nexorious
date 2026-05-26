package platformresolution

import (
	"context"

	"github.com/uptrace/bun"
)

// IGDBPlatformIDsForExternalGame returns the IGDB numeric platform IDs for the
// platforms attached to this external_game, sorted ascending. Platforms whose
// igdb_platform_id is NULL are silently skipped. Returns an empty slice (not an
// error) if the external_game has no platforms or no resolvable IDs. Returns an
// error only on DB failure.
//
// The ORDER BY clause makes the result deterministic so the Apicalypse query
// built downstream (and the TanStack Query cache key derived from it) is stable
// across calls.
//
// Used by the IGDB sync match path (both auto-match in the worker and manual
// match via POST /api/games/search/igdb) to scope IGDB search results to
// platforms the storefront actually reports for that specific game (issue #615).
func IGDBPlatformIDsForExternalGame(ctx context.Context, db *bun.DB, externalGameID string) ([]int, error) {
	var ids []int
	err := db.NewRaw(
		`SELECT DISTINCT p.igdb_platform_id
		 FROM external_game_platforms egp
		 JOIN platforms p ON p.name = egp.platform
		 WHERE egp.external_game_id = ? AND p.igdb_platform_id IS NOT NULL
		 ORDER BY p.igdb_platform_id`,
		externalGameID,
	).Scan(ctx, &ids)
	if err != nil {
		return nil, err
	}
	return ids, nil
}
