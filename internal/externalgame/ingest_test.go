package externalgame

import (
	"context"
	"testing"
)

func TestSeen_WritesAchievementsOnTheFirstPlatformOnly(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := seedUser(t)

	three, four := 3, 4
	id, _, err := Seen(ctx, testDB, Sighting{
		UserID: userID, Storefront: "steam", ExternalID: "ach-1", Title: "Achievement Test Game",
		Platforms: []string{"pc-windows", "pc-linux"}, PlaytimeHours: 5,
		AchievementsUnlocked: &three, AchievementsTotal: &four,
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		platform      string
		unlocked, tot *int
	}{
		{"pc-windows", &three, &four},
		{"pc-linux", nil, nil},
	} {
		var unlocked, total *int
		if err := testDB.NewRaw(
			`SELECT achievements_unlocked, achievements_total FROM external_game_platforms WHERE external_game_id = ? AND platform = ?`,
			id, tc.platform,
		).Scan(ctx, &unlocked, &total); err != nil {
			t.Fatalf("scan %s: %v", tc.platform, err)
		}
		if !sameIntPtr(unlocked, tc.unlocked) || !sameIntPtr(total, tc.tot) {
			t.Errorf("%s: achievements %v/%v, want %v/%v", tc.platform, unlocked, total, tc.unlocked, tc.tot)
		}
	}
}

func sameIntPtr(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// A known game seen again keeps its decision and its stored source metadata
// when the sighting carries none, and is available again.
func TestSeen_KnownGameKeepsItsMatchAndMetadata(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := seedUser(t)
	id := seedExternalGame(t, userID, extGame{resolved: 1001})
	if _, err := testDB.NewRaw(
		`UPDATE external_games SET is_available = false, source_metadata = '{"namespace":"ns-1"}' WHERE id = ?`, id,
	).Exec(ctx); err != nil {
		t.Fatal(err)
	}

	got, skipped, err := Seen(ctx, testDB, Sighting{
		UserID: userID, Storefront: "steam", ExternalID: id, Title: "Horizon", Platforms: []string{"pc-windows"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != id || skipped {
		t.Fatalf("Seen = %s, skipped %v; want %s, not skipped", got, skipped, id)
	}
	var row struct {
		Resolved  *int32 `bun:"resolved_igdb_id"`
		Available bool   `bun:"is_available"`
		Namespace string `bun:"namespace"`
	}
	if err := testDB.NewRaw(
		`SELECT resolved_igdb_id, is_available, source_metadata->>'namespace' AS namespace FROM external_games WHERE id = ?`, id,
	).Scan(ctx, &row); err != nil {
		t.Fatal(err)
	}
	if row.Resolved == nil || *row.Resolved != 1001 || !row.Available || row.Namespace != "ns-1" {
		t.Errorf("after Seen: match %v, available %v, namespace %q; want 1001, true, ns-1", row.Resolved, row.Available, row.Namespace)
	}
}

// A game with a same-title decision takes it instead of the search's match.
func TestAutoMatch_TakesAPrecedentInsteadOfTheSearchMatch(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := seedUser(t)
	seedExternalGame(t, userID, extGame{resolved: 1001}) // the precedent, same title
	id := seedExternalGame(t, userID, extGame{})

	tx, err := testDB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	wrote, err := AutoMatch(ctx, tx, id, "Horizon", 2002, "Horizon Zero Dawn")
	if err != nil {
		t.Fatal(err)
	}
	if wrote {
		t.Error("AutoMatch reported writing the search's match over a precedent")
	}
	resolved, err := MatchUnderLock(ctx, tx, id)
	if err != nil {
		t.Fatal(err)
	}
	if resolved == nil || *resolved != 1001 {
		t.Errorf("match = %v, want the precedent's 1001", resolved)
	}
}
