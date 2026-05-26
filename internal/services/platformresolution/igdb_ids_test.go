package platformresolution_test

import (
	"context"
	"testing"

	"github.com/drzero42/nexorious/internal/services/platformresolution"
)

func TestIGDBPlatformIDsForExternalGame_ReturnsIDsForOwnedPlatforms(t *testing.T) {
	truncateAllTables(t)
	insertTestUser(t, testDB, "u1")
	egID := insertTestExternalGame(t, testDB, "u1", "steam", "Test Game")
	insertTestExternalGamePlatform(t, testDB, egID, "pc-windows") // igdb_platform_id = 6
	insertTestExternalGamePlatform(t, testDB, egID, "pc-linux")   // igdb_platform_id = 3

	ids, err := platformresolution.IGDBPlatformIDsForExternalGame(context.Background(), testDB, egID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !containsAll(ids, []int{3, 6}) || len(ids) != 2 {
		t.Fatalf("got %v, want [3 6] (any order)", ids)
	}
}

func TestIGDBPlatformIDsForExternalGame_MissingEGReturnsEmpty(t *testing.T) {
	truncateAllTables(t)

	ids, err := platformresolution.IGDBPlatformIDsForExternalGame(context.Background(), testDB, "does-not-exist")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("got %v, want empty", ids)
	}
}

func TestIGDBPlatformIDsForExternalGame_NullIDsAreSkipped(t *testing.T) {
	truncateAllTables(t)
	insertTestUser(t, testDB, "u2")
	egID := insertTestExternalGame(t, testDB, "u2", "physical", "Old Game")
	// Insert a synthetic platform with NULL igdb_platform_id, attach the EG to it.
	if _, err := testDB.NewRaw(
		`INSERT INTO platforms (name, display_name, igdb_platform_id, default_storefront) VALUES ('test-null-platform', 'Test Null', NULL, 'physical') ON CONFLICT DO NOTHING`,
	).Exec(context.Background()); err != nil {
		t.Fatalf("insert null-id platform: %v", err)
	}
	insertTestExternalGamePlatform(t, testDB, egID, "test-null-platform")
	insertTestExternalGamePlatform(t, testDB, egID, "playstation-2") // igdb_platform_id = 8

	ids, err := platformresolution.IGDBPlatformIDsForExternalGame(context.Background(), testDB, egID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 1 || ids[0] != 8 {
		t.Fatalf("got %v, want [8] only (NULL id skipped)", ids)
	}
}

// containsAll reports whether haystack contains every needle (order-independent).
func containsAll(haystack []int, needles []int) bool {
	set := make(map[int]bool, len(haystack))
	for _, v := range haystack {
		set[v] = true
	}
	for _, n := range needles {
		if !set[n] {
			return false
		}
	}
	return true
}
