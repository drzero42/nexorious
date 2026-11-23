package dbrole_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	riverdatabasesql "github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	bunmigrate "github.com/uptrace/bun/migrate"

	"github.com/drzero42/nexorious/internal/db/migrations"
	"github.com/drzero42/nexorious/internal/dbrole"
)

// adminDSN connects as the container's POSTGRES_USER, which — exactly as in
// the bundled Compose/Helm Postgres — is the bootstrap superuser.
var adminDSN string

var dbSeq atomic.Int32

func TestMain(m *testing.M) {
	ctx := context.Background()
	ctr, err := tcpostgres.Run(ctx,
		"postgres:18-alpine",
		tcpostgres.WithDatabase("postgres_admin"),
		tcpostgres.WithUsername("nexorious"),
		tcpostgres.WithPassword("admin-pw"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
		),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start postgres container: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = ctr.Terminate(ctx) }()

	adminDSN, err = ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get connection string: %v\n", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// dsn rewrites adminDSN to connect as user/password to database.
func dsn(t *testing.T, user, password, database string) string {
	t.Helper()
	u, err := url.Parse(adminDSN)
	if err != nil {
		t.Fatalf("parse admin DSN: %v", err)
	}
	u.User = url.UserPassword(user, password)
	u.Path = "/" + database
	return u.String()
}

func open(t *testing.T, connStr string) *bun.DB {
	t.Helper()
	db := bun.NewDB(sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(connStr))), pgdialect.New())
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// migrate applies the full schema (Bun + River) to db.
func migrate(t *testing.T, db *bun.DB) {
	t.Helper()
	ctx := context.Background()
	m := bunmigrate.NewMigrator(db, migrations.Migrations)
	if err := m.Init(ctx); err != nil {
		t.Fatalf("migrator init: %v", err)
	}
	if _, err := m.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	rm, err := rivermigrate.New(riverdatabasesql.New(db.DB), nil)
	if err != nil {
		t.Fatalf("river migrator: %v", err)
	}
	if _, err := rm.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		t.Fatalf("river migrate: %v", err)
	}
}

// existingInstall creates a fresh database migrated by the superuser — the
// state of every Compose/Helm install before #1357 — and returns its name and
// an admin handle connected to it.
func existingInstall(t *testing.T) (string, *bun.DB) {
	t.Helper()
	name := fmt.Sprintf("install_%d", dbSeq.Add(1))
	root := open(t, adminDSN)
	if _, err := root.Exec("CREATE DATABASE ?", bun.Ident(name)); err != nil {
		t.Fatalf("create database: %v", err)
	}
	admin := open(t, dsn(t, "nexorious", "admin-pw", name))
	migrate(t, admin)
	return name, admin
}

// TestEnsureAppRole_TakesOverExistingInstall is the upgrade path: a database
// fully owned by the bootstrap superuser is handed to a new plain role, and
// that role can then do everything the app needs (run, migrate, restore) but
// cannot reach the database host.
func TestEnsureAppRole_TakesOverExistingInstall(t *testing.T) {
	ctx := context.Background()
	name, admin := existingInstall(t)
	// Quote and apostrophe exercise identifier and literal escaping.
	role, pw := `app"role_1`, `p'w"1:@/`

	res, err := dbrole.EnsureAppRole(ctx, admin, role, pw, name)
	if err != nil {
		t.Fatalf("EnsureAppRole: %v", err)
	}
	if res.Outcome != dbrole.OutcomeEnsured || !res.Created || res.Transferred == 0 {
		t.Fatalf("first run = %+v, want ensured, created, objects transferred", res)
	}

	var notOwned []string
	if err := admin.NewRaw(`
SELECT c.relname FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
 WHERE n.nspname = 'public' AND pg_get_userbyid(c.relowner) <> ?
UNION ALL
SELECT p.proname FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
 WHERE n.nspname = 'public' AND pg_get_userbyid(p.proowner) <> ?
UNION ALL
SELECT t.typname FROM pg_type t JOIN pg_namespace n ON n.oid = t.typnamespace
 WHERE n.nspname = 'public' AND pg_get_userbyid(t.typowner) <> ?
UNION ALL
SELECT 'schema public' FROM pg_namespace WHERE nspname = 'public' AND pg_get_userbyid(nspowner) <> ?
UNION ALL
SELECT 'database' FROM pg_database WHERE datname = ? AND pg_get_userbyid(datdba) <> ?`,
		role, role, role, role, name, role).Scan(ctx, &notOwned); err != nil {
		t.Fatalf("list objects not owned by role: %v", err)
	}
	if len(notOwned) > 0 {
		t.Fatalf("objects still not owned by %s: %v", role, notOwned)
	}

	app := open(t, dsn(t, role, pw, name))
	check, err := dbrole.CheckConnectedRole(ctx, app)
	if err != nil {
		t.Fatalf("CheckConnectedRole: %v", err)
	}
	if check.Role != role || check.Privileged() {
		t.Fatalf("app role check = %+v, want unprivileged %s", check, role)
	}

	// Normal app work on tables and River's tables/functions it now owns.
	if _, err := app.ExecContext(ctx, `ALTER TABLE users ADD COLUMN dbrole_probe int; ALTER TABLE users DROP COLUMN dbrole_probe`); err != nil {
		t.Fatalf("app role cannot alter its tables: %v", err)
	}

	// The point of the fix: plain SQL can no longer run programs on the host.
	if _, err := app.ExecContext(ctx, `COPY (SELECT 1) TO PROGRAM 'true'`); err == nil {
		t.Fatal("COPY … TO PROGRAM succeeded as the app role")
	}

	// Restore path (backup.Service): drop and recreate the schema, then
	// migrate from scratch, all as the app role.
	if _, err := app.ExecContext(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		t.Fatalf("app role cannot reset the schema for a restore: %v", err)
	}
	migrate(t, app)
}

func TestEnsureAppRole_IsIdempotentAndRotatesPassword(t *testing.T) {
	ctx := context.Background()
	name, admin := existingInstall(t)
	role := "app_role_2"

	if _, err := dbrole.EnsureAppRole(ctx, admin, role, "old", name); err != nil {
		t.Fatalf("first run: %v", err)
	}
	res, err := dbrole.EnsureAppRole(ctx, admin, role, "new", name)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if res.Outcome != dbrole.OutcomeEnsured || res.Created || res.Transferred != 0 {
		t.Fatalf("second run = %+v, want ensured, not created, nothing transferred", res)
	}
	if err := open(t, dsn(t, role, "new", name)).PingContext(ctx); err != nil {
		t.Fatalf("login with rotated password: %v", err)
	}
	if err := open(t, dsn(t, role, "old", name)).PingContext(ctx); err == nil {
		t.Fatal("old password still works after rotation")
	}

	// Objects the superuser creates later (e.g. an operator migrating by hand)
	// are picked up on the next run.
	if _, err := admin.ExecContext(ctx, `CREATE TABLE later_table (id int)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	res, err = dbrole.EnsureAppRole(ctx, admin, role, "new", name)
	if err != nil {
		t.Fatalf("third run: %v", err)
	}
	if res.Transferred != 1 {
		t.Fatalf("third run transferred %d objects, want 1", res.Transferred)
	}
}

func TestEnsureAppRole_SkipsPrivilegedRoles(t *testing.T) {
	ctx := context.Background()
	name, admin := existingInstall(t)

	res, err := dbrole.EnsureAppRole(ctx, admin, "nexorious", "x", name)
	if err != nil {
		t.Fatalf("same role: %v", err)
	}
	if res.Outcome != dbrole.OutcomeSkippedSameRole {
		t.Fatalf("same role outcome = %v, want OutcomeSkippedSameRole", res.Outcome)
	}

	if _, err := admin.ExecContext(ctx, `CREATE ROLE super_app LOGIN SUPERUSER`); err != nil {
		t.Fatalf("create superuser role: %v", err)
	}
	res, err = dbrole.EnsureAppRole(ctx, admin, "super_app", "x", name)
	if err != nil {
		t.Fatalf("superuser role: %v", err)
	}
	if res.Outcome != dbrole.OutcomeSkippedPrivileged {
		t.Fatalf("superuser role outcome = %v, want OutcomeSkippedPrivileged", res.Outcome)
	}
	var owner string
	if err := admin.NewRaw(`SELECT pg_get_userbyid(datdba) FROM pg_database WHERE datname = ?`, name).Scan(ctx, &owner); err != nil {
		t.Fatalf("read database owner: %v", err)
	}
	if owner != "nexorious" {
		t.Fatalf("database owner = %s after skip, want it untouched", owner)
	}
}

func TestCheckConnectedRole(t *testing.T) {
	ctx := context.Background()
	name, admin := existingInstall(t)

	check, err := dbrole.CheckConnectedRole(ctx, admin)
	if err != nil {
		t.Fatalf("CheckConnectedRole: %v", err)
	}
	if !check.Superuser || !check.Privileged() {
		t.Fatalf("superuser check = %+v, want privileged superuser", check)
	}

	// A non-superuser that can run programs on the host is just as dangerous.
	if _, err := admin.ExecContext(ctx, `CREATE ROLE exec_app LOGIN PASSWORD 'x'; GRANT pg_execute_server_program TO exec_app`); err != nil {
		t.Fatalf("create exec role: %v", err)
	}
	check, err = dbrole.CheckConnectedRole(ctx, open(t, dsn(t, "exec_app", "x", name)))
	if err != nil {
		t.Fatalf("CheckConnectedRole: %v", err)
	}
	if check.Superuser || !check.ServerAccess || !check.Privileged() {
		t.Fatalf("exec role check = %+v, want privileged via server access", check)
	}
	if !strings.Contains(check.Role, "exec_app") {
		t.Fatalf("role = %q, want exec_app", check.Role)
	}
}
