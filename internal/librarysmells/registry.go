// Package librarysmells detects data-quality issues ("library smells") in a
// user's game collection and, for a subset, applies one-click fixes. See
// docs/superpowers/specs/2026-06-22-library-smells-engine-design.md.
package librarysmells

import (
	"context"

	"github.com/uptrace/bun"
)

// Tier is the severity grouping of a check.
type Tier string

const (
	// TierInconsistency is "something is wrong" (epic Tier 1).
	TierInconsistency Tier = "inconsistency"
	// TierNudge is "you might want to update this" (epic Tier 2).
	TierNudge Tier = "nudge"
)

// FlaggedItem is one flagged game for one check, with check-specific context.
// It carries bun tags (raw-scan target) and json tags (API response).
type FlaggedItem struct {
	UserGameID  string  `bun:"user_game_id"  json:"user_game_id"`
	GameID      int32   `bun:"game_id"       json:"game_id"`
	Title       string  `bun:"title"         json:"title"`
	CoverArtURL *string `bun:"cover_art_url" json:"cover_art_url,omitempty"`

	PlatformRowID       *string `bun:"platform_row_id"      json:"platform_row_id,omitempty"`
	Platform            *string `bun:"platform"             json:"platform,omitempty"`
	Storefront          *string `bun:"storefront"           json:"storefront,omitempty"`
	SuggestedStorefront *string `bun:"suggested_storefront" json:"suggested_storefront,omitempty"`
	SuggestedStatus     *string `bun:"suggested_status"     json:"suggested_status,omitempty"`
	Detail              *string `bun:"detail"               json:"detail,omitempty"`
}

// Check is one library-smell detector and its optional one-click fix.
type Check struct {
	ID          string
	Title       string
	Description string
	Tier        Tier
	AutoFixable bool

	// Detect returns flagged items for userID, excluding rows the user has
	// dismissed via smell_ignores for this check. Read-only.
	Detect func(ctx context.Context, db *bun.DB, userID string) ([]FlaggedItem, error)

	// Apply performs the one-click fix for the given user_game IDs, routing
	// through internal/usergame. Non-nil only when AutoFixable. Returns
	// (applied, skipped). nil for deep-link-only checks.
	Apply func(ctx context.Context, db *bun.DB, userID string, userGameIDs []string) (applied, skipped int, err error)
}

// Registry returns every check in epic display order.
func Registry() []Check {
	return []Check{
		storefrontLessCheck,
		orphanGameCheck,
		wishlistedYetOwnedCheck,
		missingOwnershipCheck,
		impossibleAcquiredDateCheck,
		invalidStorefrontCheck,
		beatButNotMarkedCheck,
		playedButNotStartedCheck,
		inProgressUntouchedCheck,
		unratedAfterFinishingCheck,
	}
}

// Lookup resolves a check by its slug.
func Lookup(id string) (Check, bool) {
	for _, c := range Registry() {
		if c.ID == id {
			return c, true
		}
	}
	return Check{}, false
}
