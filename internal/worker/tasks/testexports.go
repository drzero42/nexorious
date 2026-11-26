package tasks

import (
	"context"

	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
)

// UpsertPlatformsForTest exposes upsertPlatforms for in-package white-box tests.
func UpsertPlatformsForTest(ctx context.Context, db *bun.DB, egID string, platforms []string, playtimeHours float64, achievementsUnlocked, achievementsTotal *int) error {
	return upsertPlatforms(ctx, db, egID, platforms, playtimeHours, achievementsUnlocked, achievementsTotal)
}

// BuildJSONDocForTest exposes buildJSONDoc for cross-package tests.
func BuildJSONDocForTest(ugs []models.UserGame, pools []exportPoolJSON) exportDocJSON {
	return buildJSONDoc(ugs, pools)
}

// LoadPoolsForExportForTest exposes loadPoolsForExport for cross-package tests.
func LoadPoolsForExportForTest(ctx context.Context, db *bun.DB, userID string) ([]exportPoolJSON, error) {
	return loadPoolsForExport(ctx, db, userID)
}

// LoadUserGamesWithRelationsForTest exposes loadUserGamesWithRelations for tests.
func LoadUserGamesWithRelationsForTest(ctx context.Context, db *bun.DB, userID string) ([]models.UserGame, error) {
	return loadUserGamesWithRelations(ctx, db, userID)
}

// ApplyImportedPoolsForTest exposes applyImportedPools for cross-package tests.
func ApplyImportedPoolsForTest(ctx context.Context, db *bun.DB, jobID, userID string) error {
	return applyImportedPools(ctx, db, jobID, userID)
}

// CompleteSameTitleReviewsForTest runs the match worker's completion of
// same-title reviews for the item itemID, whose External game egID a search
// has just matched.
func CompleteSameTitleReviewsForTest(ctx context.Context, w *IGDBMatchWorker, itemID, egID string) error {
	var item models.JobItem
	if err := w.DB.NewSelect().Model(&item).Where("id = ?", itemID).Scan(ctx); err != nil {
		return err
	}
	var eg models.ExternalGame
	if err := w.DB.NewSelect().Model(&eg).Where("id = ?", egID).Scan(ctx); err != nil {
		return err
	}
	w.completeSameTitleReviews(ctx, &item, &eg)
	return nil
}
