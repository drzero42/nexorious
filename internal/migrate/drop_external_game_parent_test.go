package migrate_test

import (
	"context"
	"testing"

	bunmigrate "github.com/uptrace/bun/migrate"

	"github.com/drzero42/nexorious/internal/db/migrations"
)

const dropParentMigration = "20261126000001"

// The migration that drops external_games.parent_id (#1450) gives a skipped
// child without a match the match of its skipped parent, so the two stay
// editions of each other, and changes no other row.
func TestDropExternalGameParentMigration(t *testing.T) {
	resetPublicSchema(t)
	ctx := context.Background()
	db := makeBunDB(t, testDSN)

	mg := bunmigrate.NewMigrator(db, migrationsBefore(dropParentMigration))
	if err := mg.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := mg.Migrate(ctx); err != nil {
		t.Fatalf("migrate before dropping parent_id: %v", err)
	}

	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`INSERT INTO users (id, username, password_hash) VALUES ('u1', 'u1', 'x')`)
	exec(`INSERT INTO games (id, title) VALUES (10, 'Ten'), (20, 'Twenty')`)
	// game inserts one External game; match 0 is unmatched, parent "" none.
	game := func(id, parent string, match int, skipped bool) {
		var m, p any
		if match != 0 {
			m = match
		}
		if parent != "" {
			p = parent
		}
		exec(`INSERT INTO external_games (id, user_id, storefront, external_id, title, resolved_igdb_id, is_skipped, parent_id)
		      VALUES (?, 'u1', 'steam', ?, 'Game', ?, ?, ?)`, id, id, m, skipped, p)
	}
	// The case the copy is for: skipped parent with a match, skipped child without.
	game("p-skip", "", 10, true)
	game("c-skip", "p-skip", 0, true)
	// Not copied: a child that is not skipped and never got the match.
	game("p-match", "", 20, false)
	game("c-unmatched", "p-match", 0, false)
	// Not copied: the parent is skipped without a match.
	game("p-skip-none", "", 0, true)
	game("c-skip-none", "p-skip-none", 0, true)
	// Not copied: the child already has a match of its own.
	game("p-skip-2", "", 10, true)
	game("c-skip-own", "p-skip-2", 20, true)
	// Not copied: no parent at all.
	game("lone", "", 0, true)

	type row struct {
		ID             string `bun:"id"`
		ResolvedIGDBID *int32 `bun:"resolved_igdb_id"`
		IsSkipped      bool   `bun:"is_skipped"`
	}
	read := func() map[string]row {
		t.Helper()
		var rows []row
		if err := db.NewRaw(`SELECT id, resolved_igdb_id, is_skipped FROM external_games`).Scan(ctx, &rows); err != nil {
			t.Fatal(err)
		}
		out := map[string]row{}
		for _, r := range rows {
			out[r.ID] = r
		}
		return out
	}
	before := read()

	if _, err := bunmigrate.NewMigrator(db, migrations.Migrations).Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	after := read()

	for id, b := range before {
		a := after[id]
		if id == "c-skip" {
			if a.ResolvedIGDBID == nil || *a.ResolvedIGDBID != 10 || !a.IsSkipped {
				t.Errorf("c-skip = %+v, want skipped with its parent's match 10", a)
			}
			continue
		}
		if !sameMatch(a.ResolvedIGDBID, b.ResolvedIGDBID) || a.IsSkipped != b.IsSkipped {
			t.Errorf("%s changed: %+v -> %+v", id, b, a)
		}
	}

	var hasColumn bool
	if err := db.NewRaw(`SELECT EXISTS (SELECT 1 FROM information_schema.columns
		WHERE table_name = 'external_games' AND column_name = 'parent_id')`).Scan(ctx, &hasColumn); err != nil {
		t.Fatal(err)
	}
	if hasColumn {
		t.Error("external_games.parent_id still exists")
	}
}

func sameMatch(a, b *int32) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
