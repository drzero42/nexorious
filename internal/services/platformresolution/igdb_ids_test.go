package platformresolution_test

import (
	"context"
	"testing"

	"github.com/drzero42/nexorious/internal/externalgame/egtest"
	"github.com/drzero42/nexorious/internal/services/platformresolution"
)

func TestIGDBPlatformIDsForExternalGame_ReturnsIDsForOwnedPlatforms(t *testing.T) {
	truncateAllTables(t)
	insertTestUser(t, testDB, "u1")
	egID := egtest.Insert(t, testDB, egtest.Game{
		UserID: "u1", Title: "Test Game",
		Platforms: []string{"pc-windows", "pc-linux"}, // igdb_platform_id 6 and 3
	})

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
	egID := egtest.Insert(t, testDB, egtest.Game{UserID: "u2", Storefront: "physical", Title: "Old Game"})
	// Insert a synthetic platform with NULL igdb_platform_id, attach the EG to it.
	if _, err := testDB.NewRaw(
		`INSERT INTO platforms (name, display_name, igdb_platform_id, default_storefront) VALUES ('test-null-platform', 'Test Null', NULL, 'physical') ON CONFLICT DO NOTHING`,
	).Exec(context.Background()); err != nil {
		t.Fatalf("insert null-id platform: %v", err)
	}
	egtest.InsertPlatform(t, testDB, egID, "test-null-platform", 0)
	egtest.InsertPlatform(t, testDB, egID, "playstation-2", 0) // igdb_platform_id = 8

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
