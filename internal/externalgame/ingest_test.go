package externalgame

import (
	"context"
	"testing"

	"github.com/drzero42/nexorious/internal/enum"
	"github.com/drzero42/nexorious/internal/externalgame/egtest"
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
	seedGame(t, 1001, "Horizon")
	id := egtest.Insert(t, testDB, egtest.Game{
		UserID: userID, Title: "Horizon", ResolvedIGDBID: 1001, Unavailable: true,
		SourceMetadata: map[string]string{"namespace": "ns-1"},
	})

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

// A sighting without an ownership status is stored as owned, never as an empty string,
// which the external_games_ownership_status_check constraint rejects (#1500).
func TestSeen_EmptyOwnershipStatusIsOwned(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := seedUser(t)

	for _, tc := range []struct {
		extID string
		in    enum.OwnershipStatus
		want  enum.OwnershipStatus
	}{
		{"empty", "", enum.OwnershipOwned},
		{"sub", enum.OwnershipSubscription, enum.OwnershipSubscription},
	} {
		id, _, err := Seen(ctx, testDB, Sighting{
			UserID: userID, Storefront: "steam", ExternalID: tc.extID, Title: "Game",
			OwnershipStatus: tc.in, Platforms: []string{"pc-windows"},
		})
		if err != nil {
			t.Fatalf("%s: %v", tc.extID, err)
		}
		var got enum.OwnershipStatus
		if err := testDB.NewRaw(`SELECT ownership_status FROM external_games WHERE id = ?`, id).Scan(ctx, &got); err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("%s: ownership_status = %q, want %q", tc.extID, got, tc.want)
		}
	}
}
