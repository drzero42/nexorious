package migrate

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
	bunmigrate "github.com/uptrace/bun/migrate"
)

// baselineTimestamp is the bun_migrations.name (14-digit timestamp) of the
// squashed baseline migration file 20260620000001_baseline.up.sql. Bun stores
// only the timestamp in the name column — the "_baseline" suffix is the
// (unpersisted) comment. See issue #1117.
const baselineTimestamp = "20260620000001"

// v0171Manifest is the frozen, permanent set of bun_migrations.name timestamps
// written by the 23 migrations shipped through v0.17.1 — the last release on the
// incremental chain. A database whose bun_migrations contains EXACTLY this set
// (and not the baseline) is byte-for-byte the baseline schema and is safe to
// adopt. This manifest is frozen forever; never edit it (a future squash would
// add a second, separate manifest).
var v0171Manifest = map[string]struct{}{
	"20260503000001": {}, "20260531000001": {}, "20260531000002": {}, "20260601000001": {},
	"20260601000002": {}, "20260601000003": {}, "20260601000004": {}, "20260602000001": {},
	"20260602000002": {}, "20260602000003": {}, "20260604000001": {}, "20260604000002": {},
	"20260604000003": {}, "20260604000004": {}, "20260605000001": {}, "20260605000002": {},
	"20260605000003": {}, "20260608000001": {}, "20260608000002": {}, "20260608000003": {},
	"20260608000004": {}, "20260609000001": {}, "20260612000001": {},
}

type adoptDecision int

const (
	// decisionNormal: run Bun migration as usual (fresh install → baseline;
	// baseline already present → catch up any post-baseline migrations).
	decisionNormal adoptDecision = iota
	// decisionAdopt: bun_migrations is exactly the v0.17.1 manifest with no
	// baseline row — rewrite it to the single baseline row, then catch up.
	decisionAdopt
	// decisionRefuse: bun_migrations is non-empty, has no baseline row, and is
	// not the exact manifest (older than v0.17.1, partial, or unknown).
	decisionRefuse
)

// adopt rewrites a fully-migrated v0.17.1 bun_migrations (the 23 manifest rows)
// to the single baseline row, in one transaction. Callers MUST hold the Bun
// advisory lock and MUST have just re-confirmed classify()==decisionAdopt under
// that lock. The baseline row uses GroupID 1 to match a fresh-install Migrate().
func (mg *Migrator) adopt(ctx context.Context) error {
	names := make([]string, 0, len(v0171Manifest))
	for n := range v0171Manifest {
		names = append(names, n)
	}
	return mg.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewDelete().
			Model((*bunmigrate.Migration)(nil)).
			ModelTableExpr("bun_migrations").
			Where("name IN (?)", bun.List(names)).
			Exec(ctx); err != nil {
			return fmt.Errorf("adopt: delete v0.17.1 rows: %w", err)
		}
		row := &bunmigrate.Migration{Name: baselineTimestamp, GroupID: 1}
		if _, err := tx.NewInsert().
			Model(row).
			ModelTableExpr("bun_migrations").
			Exec(ctx); err != nil {
			return fmt.Errorf("adopt: insert baseline row: %w", err)
		}
		return nil
	})
}

// classify decides how to treat a database from its raw bun_migrations rows.
// applied is the result of (*bunmigrate.Migrator).AppliedMigrations — each
// .Name is the 14-digit timestamp.
func classify(applied bunmigrate.MigrationSlice) adoptDecision {
	have := make(map[string]struct{}, len(applied))
	for _, m := range applied {
		have[m.Name] = struct{}{}
	}
	if _, ok := have[baselineTimestamp]; ok {
		return decisionNormal
	}
	if len(have) == 0 {
		return decisionNormal // fresh DB → baseline is discovered-unapplied
	}
	if len(have) == len(v0171Manifest) {
		for n := range have {
			if _, ok := v0171Manifest[n]; !ok {
				return decisionRefuse
			}
		}
		return decisionAdopt
	}
	return decisionRefuse
}
