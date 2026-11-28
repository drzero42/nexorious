// Package egtest writes External game fixtures for tests in any package.
//
// It is the one place outside internal/externalgame that writes
// external_games and external_game_platforms (ADR 0009), and only test code
// may import it: the module's write guard exempts this package and fails if a
// non-test package links it. A fixture writes a row as it is, without the
// editions lock or the library write, so a test can set up any state the
// schema allows.
//
// Tests of a migration write rows into the schema as it was before that
// migration, so they keep their own SQL instead of using this package.
package egtest

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Game describes an external_games row. The zero value of each field gets the
// default named on it.
type Game struct {
	ID         string // default: a new UUID
	UserID     string // required
	Storefront string // default: steam
	ExternalID string // default: the ID
	Title      string // default: Game

	Skipped     bool
	Unavailable bool
	// ResolvedIGDBID is the match. 0 leaves the game unmatched; a games row
	// with that id must exist.
	ResolvedIGDBID int32
	// OwnershipStatus is NULL when empty.
	OwnershipStatus string
	// StoreLink is NULL when nil.
	StoreLink *string
	// CreatedAt is now() when zero. updated_at is always now().
	CreatedAt time.Time

	// Platforms gets one external_game_platforms row each, with no playtime.
	// Use InsertPlatform for a row with playtime.
	Platforms []string
}

// Insert writes g and its platform rows and returns its id.
func Insert(t testing.TB, db bun.IDB, g Game) string {
	t.Helper()
	if g.UserID == "" {
		t.Fatal("egtest.Insert: UserID is required")
	}
	if g.ID == "" {
		g.ID = uuid.NewString()
	}
	if g.Storefront == "" {
		g.Storefront = "steam"
	}
	if g.ExternalID == "" {
		g.ExternalID = g.ID
	}
	if g.Title == "" {
		g.Title = "Game"
	}
	var resolved, ownership, created any
	if g.ResolvedIGDBID != 0 {
		resolved = g.ResolvedIGDBID
	}
	if g.OwnershipStatus != "" {
		ownership = g.OwnershipStatus
	}
	if !g.CreatedAt.IsZero() {
		created = g.CreatedAt
	}
	if _, err := db.NewRaw(`
		INSERT INTO external_games (id, user_id, storefront, external_id, title,
			is_skipped, is_available, resolved_igdb_id, ownership_status, store_link, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, COALESCE(?::timestamptz, now()), now())`,
		g.ID, g.UserID, g.Storefront, g.ExternalID, g.Title,
		g.Skipped, !g.Unavailable, resolved, ownership, g.StoreLink, created,
	).Exec(context.Background()); err != nil {
		t.Fatalf("egtest.Insert %q: %v", g.Title, err)
	}
	for _, p := range g.Platforms {
		InsertPlatform(t, db, g.ID, p, 0)
	}
	return g.ID
}

// InsertPlatform writes an external_game_platforms row for the External game
// id and returns its id.
func InsertPlatform(t testing.TB, db bun.IDB, externalGameID, platform string, hoursPlayed float64) string {
	t.Helper()
	id := uuid.NewString()
	if _, err := db.NewRaw(`
		INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at)
		VALUES (?, ?, ?, ?, now())`,
		id, externalGameID, platform, hoursPlayed,
	).Exec(context.Background()); err != nil {
		t.Fatalf("egtest.InsertPlatform %q on %s: %v", platform, externalGameID, err)
	}
	return id
}
