package api

import (
	"context"

	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
)

// withUserGameRelations applies the canonical set of relations for projecting a
// user-game into a card/detail response: the game, its platforms (with platform,
// storefront, and external-game records — the last drives the store_url
// deep-link), and its tags. This is the single definition of that relation set;
// every card/detail read in this package loads through it.
func withUserGameRelations(q *bun.SelectQuery) *bun.SelectQuery {
	return q.
		Relation("Game").
		Relation("Platforms", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Relation("PlatformRecord").Relation("StorefrontRecord").Relation("ExternalGame")
		}).
		Relation("Tags", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Relation("Tag")
		})
}

// LoadUserGameDetail loads a single user-game owned by userID with the canonical
// relation set. Returns sql.ErrNoRows when the game does not exist or is not the
// caller's; callers map that to a 404.
func LoadUserGameDetail(ctx context.Context, db *bun.DB, userGameID, userID string) (*models.UserGame, error) {
	var ug models.UserGame
	if err := withUserGameRelations(db.NewSelect().Model(&ug)).
		Where("user_game.id = ?", userGameID).
		Where("user_game.user_id = ?", userID).
		Scan(ctx); err != nil {
		return nil, err
	}
	return &ug, nil
}

// LoadUserGameCardsByIDs loads user-games for the given ids with the canonical
// relation set, for list/card projections. Order is not guaranteed; callers that
// need a specific order re-apply it (HandleListUserGames) or key by id (pools).
func LoadUserGameCardsByIDs(ctx context.Context, db *bun.DB, ids []string) ([]models.UserGame, error) {
	var userGames []models.UserGame
	if len(ids) == 0 {
		return userGames, nil
	}
	if err := withUserGameRelations(db.NewSelect().Model(&userGames)).
		Where("user_game.id IN (?)", bun.List(ids)).
		Scan(ctx); err != nil {
		return nil, err
	}
	return userGames, nil
}
