package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/migrate"
)

// TestServeCmd_MigrateFlagRegistered verifies `serve` exposes an opt-in
// --migrate bool flag (issue #941) that defaults to false so plain
// `nexorious serve` behaviour is unchanged.
func TestServeCmd_MigrateFlagRegistered(t *testing.T) {
	cmd := newServeCmd()
	f := cmd.Flags().Lookup("migrate")
	if f == nil {
		t.Fatal("expected `serve` to register a --migrate flag")
	} else {
		if f.Value.Type() != "bool" {
			t.Errorf("--migrate flag type = %q, want bool", f.Value.Type())
		}
		if f.DefValue != "false" {
			t.Errorf("--migrate default = %q, want false (opt-in)", f.DefValue)
		}
	}
}

// freshDB creates a pristine, un-migrated database in the shared container
// (the shared testDB is already fully migrated) and returns a bun.DB
// connected to it.
func freshDB(t *testing.T, name string) *bun.DB {
	t.Helper()
	ctx := context.Background()
	if _, err := testDB.ExecContext(ctx, "DROP DATABASE IF EXISTS "+name); err != nil {
		t.Fatalf("drop database %s: %v", name, err)
	}
	if _, err := testDB.ExecContext(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("create database %s: %v", name, err)
	}
	dsn := strings.Replace(testDSN, "/nexorious_test?", "/"+name+"?", 1)
	db := openBunDB(dsn)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// TestRunStartupMigrations_AppliesPendingMigrations runs the serve --migrate
// startup path against a pristine database and verifies both the application
// (bun) and River migrations are applied, leaving the schema fully Ready.
func TestRunStartupMigrations_AppliesPendingMigrations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping container-backed test in -short mode")
	}
	ctx := context.Background()
	db := freshDB(t, "serve_migrate_pending")

	migrator := migrate.NewMigrator(db)
	if err := runStartupMigrations(ctx, db, migrator, 10*time.Second); err != nil {
		t.Fatalf("runStartupMigrations: %v", err)
	}

	// Recomputing the state must now report Ready: no pending bun or River
	// migrations remain.
	if err := migrator.DetermineState(); err != nil {
		t.Fatalf("DetermineState after migration: %v", err)
	}
	if migrator.State() != migrate.AppStateReady {
		t.Errorf("state after runStartupMigrations = %v, want Ready", migrator.State())
	}

	// Spot-check that both schemas really exist.
	for _, table := range []string{"users", "river_job"} {
		var n int
		if err := db.NewRaw("SELECT count(*) FROM "+table).Scan(ctx, &n); err != nil {
			t.Errorf("expected table %s to exist after migration: %v", table, err)
		}
	}
}

// TestRunStartupMigrations_NoopWhenUpToDate verifies the flag is safe to leave
// on permanently: with no pending migrations the helper returns nil without
// touching anything.
func TestRunStartupMigrations_NoopWhenUpToDate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping container-backed test in -short mode")
	}
	ctx := context.Background()
	db := freshDB(t, "serve_migrate_uptodate")

	if err := runStartupMigrations(ctx, db, migrate.NewMigrator(db), 10*time.Second); err != nil {
		t.Fatalf("first runStartupMigrations: %v", err)
	}

	migrator := migrate.NewMigrator(db)
	if err := runStartupMigrations(ctx, db, migrator, 10*time.Second); err != nil {
		t.Fatalf("second runStartupMigrations on up-to-date DB: %v", err)
	}
	if err := migrator.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	if migrator.State() != migrate.AppStateReady {
		t.Errorf("state = %v, want Ready", migrator.State())
	}
}

// TestRunStartupMigrations_DBUnreachable verifies startup aborts with an error
// (rather than proceeding to serve) when the database stays unreachable past
// the wait deadline.
func TestRunStartupMigrations_DBUnreachable(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network-timeout test in -short mode")
	}
	// Port 1 is never listening; every ping fails fast with connection refused.
	db := openBunDB("postgres://nobody:nope@127.0.0.1:1/none?sslmode=disable")
	defer func() { _ = db.Close() }()

	err := runStartupMigrations(context.Background(), db, migrate.NewMigrator(db), 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected an error when the database is unreachable")
	}
	if !strings.Contains(err.Error(), "unreachable") {
		t.Errorf("err = %q, want it to mention the database being unreachable", err)
	}
}
