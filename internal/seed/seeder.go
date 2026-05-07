package seed

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SeedResult holds counts of rows inserted or updated per table.
type SeedResult struct {
	Storefronts  int
	Platforms    int
	Associations int
}

// SeedAll seeds official storefronts, platforms, and associations in a single transaction.
// Idempotent: safe to call on an already-seeded database. Custom rows (source='custom') are never touched.
func SeedAll(ctx context.Context, pool *pgxpool.Pool) (SeedResult, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return SeedResult{}, fmt.Errorf("seed: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var result SeedResult

	// Storefronts — ON CONFLICT only updates official rows.
	for _, s := range OfficialStorefronts {
		tag, err := tx.Exec(ctx, `
			INSERT INTO storefronts (name, display_name, icon_url, base_url, is_active, source, version_added)
			VALUES ($1, $2, $3, $4, $5, 'official', $6)
			ON CONFLICT (name) DO UPDATE SET
				display_name  = EXCLUDED.display_name,
				icon_url      = EXCLUDED.icon_url,
				base_url      = EXCLUDED.base_url,
				version_added = EXCLUDED.version_added,
				updated_at    = now()
			WHERE storefronts.source = 'official'`,
			s.Name, s.DisplayName, s.IconURL, s.BaseURL, s.IsActive, s.VersionAdded,
		)
		if err != nil {
			return SeedResult{}, fmt.Errorf("seed storefronts: %w", err)
		}
		result.Storefronts += int(tag.RowsAffected())
	}

	// Platforms — ON CONFLICT only updates official rows.
	for _, p := range OfficialPlatforms {
		tag, err := tx.Exec(ctx, `
			INSERT INTO platforms (name, display_name, icon_url, is_active, source, version_added, default_storefront)
			VALUES ($1, $2, $3, $4, 'official', $5, $6)
			ON CONFLICT (name) DO UPDATE SET
				display_name       = EXCLUDED.display_name,
				icon_url           = EXCLUDED.icon_url,
				default_storefront = EXCLUDED.default_storefront,
				version_added      = EXCLUDED.version_added,
				updated_at         = now()
			WHERE platforms.source = 'official'`,
			p.Name, p.DisplayName, p.IconURL, p.IsActive, p.VersionAdded, p.DefaultStorefront,
		)
		if err != nil {
			return SeedResult{}, fmt.Errorf("seed platforms: %w", err)
		}
		result.Platforms += int(tag.RowsAffected())
	}

	// Associations — idempotent via DO NOTHING.
	for _, a := range OfficialAssociations {
		tag, err := tx.Exec(ctx, `
			INSERT INTO platform_storefronts (platform, storefront)
			VALUES ($1, $2)
			ON CONFLICT (platform, storefront) DO NOTHING`,
			a.Platform, a.Storefront,
		)
		if err != nil {
			return SeedResult{}, fmt.Errorf("seed associations: %w", err)
		}
		result.Associations += int(tag.RowsAffected())
	}

	if err := tx.Commit(ctx); err != nil {
		return SeedResult{}, fmt.Errorf("seed: commit: %w", err)
	}
	return result, nil
}
