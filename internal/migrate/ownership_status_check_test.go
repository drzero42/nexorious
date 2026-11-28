package migrate_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/uptrace/bun"
	bunmigrate "github.com/uptrace/bun/migrate"

	"github.com/drzero42/nexorious/internal/db/migrations"
	"github.com/drzero42/nexorious/internal/enum"
)

const ownershipStatusCheckMigration = "20261128000002"

// ownershipFixtures inserts user u1 and returns helpers that insert a user
// game platform (each on its own user game, as platforms are unique per user
// game) and an External game with the given ownership_status.
func ownershipFixtures(t *testing.T, ctx context.Context, db *bun.DB) (insertPlatform, insertExternal func(id string, status any) error) {
	t.Helper()
	if _, err := db.ExecContext(ctx, `INSERT INTO users (id, username, password_hash) VALUES ('u1', 'u1', 'x')`); err != nil {
		t.Fatal(err)
	}
	gameID := 0
	insertPlatform = func(id string, status any) error {
		gameID++
		if _, err := db.ExecContext(ctx, `INSERT INTO games (id, title) VALUES (?, 'Game')`, gameID); err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO user_games (id, user_id, game_id) VALUES (?, 'u1', ?)`, id, gameID); err != nil {
			return err
		}
		_, err := db.ExecContext(ctx, `INSERT INTO user_game_platforms (id, user_game_id, platform, ownership_status)
			VALUES (?, ?, 'pc-windows', ?)`, id, id, status)
		return err
	}
	insertExternal = func(id string, status any) error {
		_, err := db.ExecContext(ctx, `INSERT INTO external_games (id, user_id, storefront, external_id, title, ownership_status)
			VALUES (?, 'u1', 'steam', ?, 'Game', ?)`, id, id, status)
		return err
	}
	return insertPlatform, insertExternal
}

// The migration that adds the ownership_status CHECK constraints (#1500)
// clears every value outside enum.OwnershipStatus on both tables and changes
// no other row.
func TestOwnershipStatusCheckMigration(t *testing.T) {
	resetPublicSchema(t)
	ctx := context.Background()
	db := makeBunDB(t, testDSN)

	mg := bunmigrate.NewMigrator(db, migrationsBefore(ownershipStatusCheckMigration))
	if err := mg.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := mg.Migrate(ctx); err != nil {
		t.Fatalf("migrate before the ownership status check: %v", err)
	}
	insertPlatform, insertExternal := ownershipFixtures(t, ctx, db)

	// want is the value each row holds after the migration; "" means NULL.
	cases := []struct{ id, before, want string }{
		{"owned", "owned", "owned"},
		{"borrowed", "borrowed", "borrowed"},
		{"no-longer", "no_longer_owned", "no_longer_owned"},
		{"null", "", ""},
		{"empty", "''", ""},
		{"mis-cased", "Owned", ""},
		{"bogus", "stolen", ""},
	}
	for _, c := range cases {
		var v any
		switch c.before {
		case "":
		case "''":
			v = ""
		default:
			v = c.before
		}
		if err := insertPlatform(c.id, v); err != nil {
			t.Fatalf("insert platform %s: %v", c.id, err)
		}
		if err := insertExternal(c.id, v); err != nil {
			t.Fatalf("insert External game %s: %v", c.id, err)
		}
	}

	if _, err := bunmigrate.NewMigrator(db, migrations.Migrations).Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	for _, table := range []string{"user_game_platforms", "external_games"} {
		type row struct {
			ID     string         `bun:"id"`
			Status sql.NullString `bun:"ownership_status"`
		}
		var rows []row
		if err := db.NewRaw(`SELECT id, ownership_status FROM `+table).Scan(ctx, &rows); err != nil {
			t.Fatal(err)
		}
		got := map[string]sql.NullString{}
		for _, r := range rows {
			got[r.ID] = r.Status
		}
		for _, c := range cases {
			want := sql.NullString{String: c.want, Valid: c.want != ""}
			if got[c.id] != want {
				t.Errorf("%s %s: ownership_status = %+v, want %+v", table, c.id, got[c.id], want)
			}
		}
	}
}

// The CHECK constraints accept exactly NULL and the values of
// enum.OwnershipStatus: a status added to the enum without a migration that
// widens the constraints fails here.
func TestOwnershipStatusCheckMatchesEnum(t *testing.T) {
	resetPublicSchema(t)
	ctx := context.Background()
	db := makeBunDB(t, testDSN)

	mg := bunmigrate.NewMigrator(db, migrations.Migrations)
	if err := mg.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := mg.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	insertPlatform, insertExternal := ownershipFixtures(t, ctx, db)

	for _, s := range append(enum.AllOwnershipStatuses(), "") {
		var v any = s
		if s == "" {
			v = nil
		}
		if err := insertPlatform("p-"+s, v); err != nil {
			t.Errorf("user_game_platforms.ownership_status %v rejected: %v", v, err)
		}
		if err := insertExternal("e-"+s, v); err != nil {
			t.Errorf("external_games.ownership_status %v rejected: %v", v, err)
		}
	}

	for _, bad := range []string{"", "stolen"} {
		if err := insertPlatform("p-bad-"+bad, bad); err == nil || !strings.Contains(err.Error(), "user_game_platforms_ownership_status_check") {
			t.Errorf("user_game_platforms.ownership_status %q = %v, want a user_game_platforms_ownership_status_check violation", bad, err)
		}
		if err := insertExternal("e-bad-"+bad, bad); err == nil || !strings.Contains(err.Error(), "external_games_ownership_status_check") {
			t.Errorf("external_games.ownership_status %q = %v, want an external_games_ownership_status_check violation", bad, err)
		}
	}

	// The down step removes both constraints.
	down, err := migrations.FS.ReadFile(ownershipStatusCheckMigration + "_ownership_status_check.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, string(down)); err != nil {
		t.Fatalf("down: %v", err)
	}
	if err := insertPlatform("p-bad-after-down", "stolen"); err != nil {
		t.Errorf("user_game_platforms.ownership_status 'stolen' after down: %v", err)
	}
	if err := insertExternal("e-bad-after-down", "stolen"); err != nil {
		t.Errorf("external_games.ownership_status 'stolen' after down: %v", err)
	}
}
