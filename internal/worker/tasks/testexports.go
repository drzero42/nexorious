package tasks

import (
	"context"

	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
)

// BuildJSONDocForTest exposes buildJSONDoc for cross-package tests.
func BuildJSONDocForTest(ugs []models.UserGame, pools []exportPoolJSON) exportDocJSON {
	return buildJSONDoc(ugs, pools)
}

// LoadPoolsForExportForTest exposes loadPoolsForExport for cross-package tests.
func LoadPoolsForExportForTest(ctx context.Context, db *bun.DB, userID string) ([]exportPoolJSON, error) {
	return loadPoolsForExport(ctx, db, userID)
}

// SyncJobItemStatusCountsForTest exposes syncJobItemStatusCounts for cross-package tests.
func SyncJobItemStatusCountsForTest(ctx context.Context, db *bun.DB, jobID string) (completed, failed, skipped int64, ok bool) {
	return syncJobItemStatusCounts(ctx, db, jobID)
}

// LoadUserGamesWithRelationsForTest exposes loadUserGamesWithRelations for tests.
func LoadUserGamesWithRelationsForTest(ctx context.Context, db *bun.DB, userID string) ([]models.UserGame, error) {
	return loadUserGamesWithRelations(ctx, db, userID)
}
