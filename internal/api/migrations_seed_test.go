package api_test

import (
	"context"
	"testing"

	"github.com/drzero42/nexorious/internal/db/models"
)

// TestPlatformsSeed_IGDBPlatformIDsPopulated verifies the baseline migration
// seeds platforms.igdb_platform_id for every known platform. This is the data
// half of the IGDB platform-filter feature (issue #615) — if these values are
// NULL, the filter silently does nothing for affected platforms.
func TestPlatformsSeed_IGDBPlatformIDsPopulated(t *testing.T) {
	tests := []struct {
		name       string
		wantIGDBID int32
	}{
		{"pc-windows", 6},
		{"mac", 14},
		{"pc-linux", 3},
		{"playstation-5", 167},
		{"playstation-4", 48},
		{"playstation-3", 9},
		{"playstation-vita", 46},
		{"playstation-psp", 38},
		{"xbox-series", 169},
		{"xbox-one", 49},
		{"xbox-360", 12},
		{"nintendo-switch", 130},
		{"nintendo-wii", 5},
		{"ios", 39},
		{"android", 34},
		{"playstation-2", 8},
		{"playstation", 7},
		{"nintendo-wii-u", 41},
		{"nintendo-switch-2", 508},
	}

	// Register the m2m join model so that bun can resolve the Platform<->Storefront
	// relation when constructing the model query. Without this, bun panics when
	// the Platform struct's m2m tag is processed.
	testDB.RegisterModel((*models.PlatformStorefront)(nil))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var p models.Platform
			if err := testDB.NewSelect().Model(&p).Where("\"platform\".name = ?", tt.name).Scan(context.Background()); err != nil {
				t.Fatalf("query platform %q: %v", tt.name, err)
			}
			if p.IgdbPlatformID == nil {
				t.Fatalf("platform %q has NULL igdb_platform_id; want %d", tt.name, tt.wantIGDBID)
			}
			if *p.IgdbPlatformID != tt.wantIGDBID {
				t.Fatalf("platform %q igdb_platform_id = %d; want %d", tt.name, *p.IgdbPlatformID, tt.wantIGDBID)
			}
		})
	}
}
