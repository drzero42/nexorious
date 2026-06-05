package tasks

import (
	"context"

	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
)

// BuildJSONDocForTest exposes buildJSONDoc for cross-package tests.
func BuildJSONDocForTest(ugs []models.UserGame) exportDocJSON {
	return buildJSONDoc(ugs)
}

// LoadUserGamesWithRelationsForTest exposes loadUserGamesWithRelations for tests.
func LoadUserGamesWithRelationsForTest(ctx context.Context, db *bun.DB, userID string) ([]models.UserGame, error) {
	return loadUserGamesWithRelations(ctx, db, userID)
}
