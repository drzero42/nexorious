package librarysmells

import (
	"context"

	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/enum"
	"github.com/drzero42/nexorious/internal/usergame"
)

// revalidate runs the check's Detect and returns the subset of userGameIDs that
// are still flagged (de-duplicated), plus the count of requested ids that are
// not flagged. This makes Apply safe against a stale client and idempotent.
func revalidate(
	ctx context.Context, db *bun.DB, userID string, userGameIDs []string,
	detect func(context.Context, *bun.DB, string) ([]FlaggedItem, error),
) (subset []string, skipped int, err error) {
	flagged, err := detect(ctx, db, userID)
	if err != nil {
		return nil, 0, err
	}
	valid := flaggedIDSet(flagged)
	seen := make(map[string]bool, len(userGameIDs))
	for _, id := range userGameIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		if valid[id] {
			subset = append(subset, id)
		}
	}
	return subset, len(seen) - len(subset), nil
}

func flaggedIDSet(items []FlaggedItem) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, it := range items {
		m[it.UserGameID] = true
	}
	return m
}

func applyStatus(
	ctx context.Context, db *bun.DB, userID string, ids []string,
	status enum.PlayStatus, detect func(context.Context, *bun.DB, string) ([]FlaggedItem, error),
) (applied, skipped int, err error) {
	subset, skipped, err := revalidate(ctx, db, userID, ids, detect)
	if err != nil {
		return 0, 0, err
	}
	if len(subset) == 0 {
		return 0, skipped, nil
	}
	n, err := usergame.SetPlayStatusBulk(ctx, db, usergame.BulkStatusParams{
		UserID:      userID,
		UserGameIDs: subset,
		PlayStatus:  string(status),
	})
	return n, skipped, err
}

func applyClearWishlist(ctx context.Context, db *bun.DB, userID string, ids []string) (applied, skipped int, err error) {
	subset, skipped, err := revalidate(ctx, db, userID, ids, detectWishlistedYetOwned)
	if err != nil {
		return 0, 0, err
	}
	if len(subset) == 0 {
		return 0, skipped, nil
	}
	n, err := usergame.ClearWishlist(ctx, db, userID, subset)
	return n, skipped, err
}

func applyPlayedButNotStarted(ctx context.Context, db *bun.DB, userID string, ids []string) (int, int, error) {
	return applyStatus(ctx, db, userID, ids, enum.PlayStatusInProgress, detectPlayedButNotStarted)
}

func applyInProgressUntouched(ctx context.Context, db *bun.DB, userID string, ids []string) (int, int, error) {
	return applyStatus(ctx, db, userID, ids, enum.PlayStatusNotStarted, detectInProgressUntouched)
}
