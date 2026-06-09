package filter

import (
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/dbutil"
)

const (
	joinUserGamePlatforms = "LEFT JOIN user_game_platforms AS ugp ON ugp.user_game_id = ug.id"
	joinGames             = "LEFT JOIN games AS g ON g.id = ug.game_id"
)

// ApplyPlayStatus filters by user_games.play_status.
func ApplyPlayStatus(fb *FilterBuilder, status string) {
	if status == "" {
		return
	}
	fb.AddWhere(func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.Where("ug.play_status = ?", status)
	})
}

// ApplyOwnershipStatus filters by user_game_platforms.ownership_status.
func ApplyOwnershipStatus(fb *FilterBuilder, status string) {
	if status == "" {
		return
	}
	fb.AddJoin("ugp", joinUserGamePlatforms)
	fb.AddWhere(func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.Where("ugp.ownership_status = ?", status)
	})
}

// ApplyIsLoved filters by user_games.is_loved.
func ApplyIsLoved(fb *FilterBuilder, v *bool) {
	if v == nil {
		return
	}
	fb.AddWhere(func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.Where("ug.is_loved = ?", *v)
	})
}

// ApplyRatingMin filters by user_games.personal_rating >= min.
func ApplyRatingMin(fb *FilterBuilder, min *float64) {
	if min == nil {
		return
	}
	fb.AddWhere(func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.Where("ug.personal_rating >= ?", *min)
	})
}

// ApplyRatingMax filters by user_games.personal_rating <= max.
func ApplyRatingMax(fb *FilterBuilder, max *float64) {
	if max == nil {
		return
	}
	fb.AddWhere(func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.Where("ug.personal_rating <= ?", *max)
	})
}

// ApplyHasNotes filters by whether personal_notes is present.
func ApplyHasNotes(fb *FilterBuilder, v *bool) {
	if v == nil {
		return
	}
	fb.AddWhere(func(q *bun.SelectQuery) *bun.SelectQuery {
		if *v {
			return q.Where("ug.personal_notes IS NOT NULL AND ug.personal_notes != ''")
		}
		return q.Where("ug.personal_notes IS NULL OR ug.personal_notes = ''")
	})
}

// ApplyPlatform filters by platform name(s). "unknown" maps to NULL.
func ApplyPlatform(fb *FilterBuilder, platforms []string) {
	if len(platforms) == 0 {
		return
	}
	fb.AddJoin("ugp", joinUserGamePlatforms)

	hasUnknown := false
	known := make([]string, 0, len(platforms))
	for _, p := range platforms {
		if p == "unknown" {
			hasUnknown = true
		} else {
			known = append(known, p)
		}
	}

	fb.AddWhere(func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			if len(known) > 0 {
				q = q.WhereOr("ugp.platform IN (?)", bun.List(known))
			}
			if hasUnknown {
				q = q.WhereOr("ugp.platform IS NULL")
			}
			return q
		})
	})
}

// ApplyStorefront filters by storefront name(s). "unknown" maps to NULL.
func ApplyStorefront(fb *FilterBuilder, storefronts []string) {
	if len(storefronts) == 0 {
		return
	}
	fb.AddJoin("ugp", joinUserGamePlatforms)

	hasUnknown := false
	known := make([]string, 0, len(storefronts))
	for _, s := range storefronts {
		if s == "unknown" {
			hasUnknown = true
		} else {
			known = append(known, s)
		}
	}

	fb.AddWhere(func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			if len(known) > 0 {
				q = q.WhereOr("ugp.storefront IN (?)", bun.List(known))
			}
			if hasUnknown {
				q = q.WhereOr("ugp.storefront IS NULL")
			}
			return q
		})
	})
}

// ApplyGenre filters by game genre (ILIKE match).
func ApplyGenre(fb *FilterBuilder, genres []string) {
	if len(genres) == 0 {
		return
	}
	fb.AddJoin("g", joinGames)
	fb.AddWhere(func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			for _, g := range genres {
				q = q.WhereOr("g.genre ILIKE ?", dbutil.LikeContains(g))
			}
			return q
		})
	})
}

// ApplyGameMode filters by game mode (ILIKE match).
func ApplyGameMode(fb *FilterBuilder, modes []string) {
	if len(modes) == 0 {
		return
	}
	fb.AddJoin("g", joinGames)
	fb.AddWhere(func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			for _, m := range modes {
				q = q.WhereOr("g.game_modes ILIKE ?", dbutil.LikeContains(m))
			}
			return q
		})
	})
}

// ApplyTheme filters by theme (ILIKE match).
func ApplyTheme(fb *FilterBuilder, themes []string) {
	if len(themes) == 0 {
		return
	}
	fb.AddJoin("g", joinGames)
	fb.AddWhere(func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			for _, t := range themes {
				q = q.WhereOr("g.themes ILIKE ?", dbutil.LikeContains(t))
			}
			return q
		})
	})
}

// ApplyPlayerPerspective filters by player perspective (ILIKE match).
func ApplyPlayerPerspective(fb *FilterBuilder, perspectives []string) {
	if len(perspectives) == 0 {
		return
	}
	fb.AddJoin("g", joinGames)
	fb.AddWhere(func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			for _, p := range perspectives {
				q = q.WhereOr("g.player_perspectives ILIKE ?", dbutil.LikeContains(p))
			}
			return q
		})
	})
}

// ApplyTag filters by tag IDs via subquery.
func ApplyTag(fb *FilterBuilder, tagIDs []string) {
	if len(tagIDs) == 0 {
		return
	}
	fb.AddWhere(func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.Where("ug.id IN (SELECT user_game_id FROM user_game_tags WHERE tag_id IN (?))", bun.List(tagIDs))
	})
}

// ApplyWishlist filters user_games by wishlist state. With onlyWishlisted=true
// it returns only wishlisted entries; otherwise it excludes them (the default
// for the main library and bulk-selection ID lists).
func ApplyWishlist(fb *FilterBuilder, onlyWishlisted bool) {
	fb.AddWhere(func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.Where("ug.is_wishlisted = ?", onlyWishlisted)
	})
}

// ApplySearch filters by title or personal notes (ILIKE match).
func ApplySearch(fb *FilterBuilder, query string) {
	if query == "" {
		return
	}
	fb.AddJoin("g", joinGames)
	pattern := dbutil.LikeContains(query)
	fb.AddWhere(func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			q = q.WhereOr("g.title ILIKE ?", pattern)
			q = q.WhereOr("ug.personal_notes IS NOT NULL AND ug.personal_notes ILIKE ?", pattern)
			return q
		})
	})
}
