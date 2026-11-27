// Package librarysmells implements Library health: it runs Checks over a
// user's collection to find data that looks wrong or out of date and, for a
// subset, applies one-click fixes (#1144).
//
// Each Check in the registry is keyed by slug and finds its flagged games with
// a read-only query anti-joined against smell_ignores (the user's ignores).
// Checks on platform rows flag each game once (EXISTS over
// user_game_platforms). A fixable check's Apply first re-validates which of the
// requested games are still flagged, then fixes them through internal/usergame.
// The API serves library health at /api/library/smells.
//
// A check must not infer a terminal status from play time: the removed
// beat-but-not-marked check (hours_played ≥ howlongtobeat_main → completed)
// misfired on co-op, replayed and idle-inflated games, and its fix took them
// out of pools (#1145).
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
	// TierSmell is "you might want to update this" (epic Tier 2). Its wire
	// value is still "nudge", the tier's old name: it is visible in the REST
	// API and MCP output, so renaming it would break clients.
	TierSmell Tier = "nudge"
)

// FlaggedItem is one flagged game for one check. Checks report one item per
// game (platform-level checks dedupe across a game's platform rows), so the
// only check-specific context is an optional human detail / suggested status.
// It carries bun tags (raw-scan target) and json tags (API response).
type FlaggedItem struct {
	UserGameID  string  `bun:"user_game_id"  json:"user_game_id"`
	GameID      int32   `bun:"game_id"       json:"game_id"`
	Title       string  `bun:"title"         json:"title"`
	CoverArtURL *string `bun:"cover_art_url" json:"cover_art_url,omitempty"`

	SuggestedStatus *string `bun:"suggested_status" json:"suggested_status,omitempty"`
	Detail          *string `bun:"detail"           json:"detail,omitempty"`
}

// Check is one rule in Library health, with its optional one-click fix.
type Check struct {
	ID          string
	Title       string
	Description string
	Tier        Tier
	AutoFixable bool

	// Detect returns this Check's flagged games for userID, leaving out games
	// the user has Ignored for this Check (smell_ignores). Read-only.
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
		playedButNotStartedCheck,
		inProgressUntouchedCheck,
		unratedAfterBeatingCheck,
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
