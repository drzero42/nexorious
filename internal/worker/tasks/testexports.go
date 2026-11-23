package tasks

import (
	"context"

	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
)

// UpsertPlatformsForTest exposes upsertPlatforms for in-package white-box tests.
func UpsertPlatformsForTest(ctx context.Context, db *bun.DB, egID string, platforms []string, playtimeHours float64, achievementsUnlocked, achievementsTotal *int) {
	upsertPlatforms(ctx, db, egID, platforms, playtimeHours, achievementsUnlocked, achievementsTotal)
}

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

// ApplyImportedPoolsForTest exposes applyImportedPools for cross-package tests.
func ApplyImportedPoolsForTest(ctx context.Context, db *bun.DB, jobID, userID string) error {
	return applyImportedPools(ctx, db, jobID, userID)
}

// ErrChildRepointedForTest exposes errChildRepointed for cross-package tests.
var ErrChildRepointedForTest = errChildRepointed

// DecideChildWaitForTest exposes decideChildWait for cross-package tests. It
// returns the action as "proceed", "wait" or "failed".
func DecideChildWaitForTest(ctx context.Context, db *bun.DB, itemID, jobID, gameID, parentID string) (string, error) {
	res, err := decideChildWait(ctx, db, itemID, jobID, gameID, parentID)
	if err != nil {
		return "", err
	}
	switch res.action {
	case childWait:
		return "wait", nil
	case childFailed:
		return "failed", nil
	default:
		return "proceed", nil
	}
}

// RepairChildSkipsForTest exposes repairChildSkips for cross-package tests.
func RepairChildSkipsForTest(ctx context.Context, db *bun.DB, userID, storefront string) error {
	return repairChildSkips(ctx, db, userID, storefront)
}
