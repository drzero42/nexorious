package main

import (
	"bytes"
	"context"
	"database/sql"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"

	"github.com/drzero42/nexorious-go/internal/migrate"
)

// fnName returns the fully-qualified name of the function pointed at by v.
// Used to verify that the root command's RunE delegates to runServe.
func fnName(v any) string {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Func {
		return ""
	}
	return runtime.FuncForPC(rv.Pointer()).Name()
}

// TestRootCmd_StructureAndSubcommands confirms the cobra tree exposes
// serve, migrate, migrate status, and version, that --config is a persistent
// flag, and that bare invocation routes to runServe via the root RunE.
func TestRootCmd_StructureAndSubcommands(t *testing.T) {
	root := newRootCmd()

	if root.Use != "nexorious" {
		t.Errorf("root.Use = %q, want %q", root.Use, "nexorious")
	}

	// --config must be on persistent flags so subcommands inherit it.
	if root.PersistentFlags().Lookup("config") == nil {
		t.Error("expected --config to be a persistent flag on root")
	}

	// Bare `./nexorious` must run serve via root RunE for backwards compat.
	if root.RunE == nil {
		t.Fatal("root.RunE must be set so bare invocation defaults to serve")
	}
	got := fnName(root.RunE)
	want := fnName(runServe)
	if got != want {
		t.Errorf("root.RunE = %q, want %q (so bare ./nexorious runs serve)", got, want)
	}

	wantSubcommands := map[string]bool{
		"serve":   false,
		"migrate": false,
		"version": false,
	}
	for _, sub := range root.Commands() {
		if _, ok := wantSubcommands[sub.Name()]; ok {
			wantSubcommands[sub.Name()] = true
		}
	}
	for name, found := range wantSubcommands {
		if !found {
			t.Errorf("expected subcommand %q to be registered on root", name)
		}
	}

	// migrate must have a `status` child.
	var migrateCmd, statusCmd *struct{}
	for _, sub := range root.Commands() {
		if sub.Name() != "migrate" {
			continue
		}
		migrateCmd = &struct{}{}
		for _, child := range sub.Commands() {
			if child.Name() == "status" {
				statusCmd = &struct{}{}
			}
		}
	}
	if migrateCmd == nil {
		t.Fatal("expected `migrate` subcommand")
	}
	if statusCmd == nil {
		t.Error("expected `migrate status` child subcommand")
	}
}

// TestVersionCmd_PrintsBuildVersion runs `nexorious version` through the cobra
// dispatcher and confirms it emits the injected version/commit values.
func TestVersionCmd_PrintsBuildVersion(t *testing.T) {
	prevVersion, prevCommit := version, commit
	version = "1.2.3-test"
	commit = "deadbeef"
	t.Cleanup(func() {
		version = prevVersion
		commit = prevCommit
	})

	root := newRootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute version: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "1.2.3-test") || !strings.Contains(got, "deadbeef") {
		t.Errorf("version output = %q, want it to contain build version and commit", got)
	}
	if !strings.HasPrefix(got, "nexorious ") {
		t.Errorf("version output = %q, want it to start with 'nexorious '", got)
	}
}

// TestHelp_MentionsAllSubcommands verifies the root help text lists every
// subcommand a user is expected to discover.
func TestHelp_MentionsAllSubcommands(t *testing.T) {
	root := newRootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute --help: %v", err)
	}
	help := buf.String()
	for _, name := range []string{"serve", "migrate", "version"} {
		if !strings.Contains(help, name) {
			t.Errorf("help output missing subcommand %q. Got:\n%s", name, help)
		}
	}
}

// startPostgresContainer launches a postgres container and returns a bun.DB
// pointing at it. The container is terminated and the DB is closed via t.Cleanup.
func startPostgresContainer(t *testing.T) *bun.DB {
	t.Helper()
	ctx := context.Background()
	ctr, err := postgres.Run(ctx,
		"postgres:18-alpine",
		postgres.WithDatabase("nexorious_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2),
		),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = ctr.Terminate(ctx) })

	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(connStr)))
	db := bun.NewDB(sqldb, pgdialect.New())
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// TestMigrator_Status_ReadOnly drives the Migrator.Status helper that backs
// `nexorious migrate status` and confirms it never applies migrations.
//
// We exercise Status directly rather than the cobra dispatcher because the
// dispatcher reads the database URL from .env / DATABASE_URL, and threading
// the testcontainer's ephemeral URL into a subprocess just to verify the
// helper output adds a lot of plumbing without exercising anything new.
func TestMigrator_Status_ReadOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping container-backed test in -short mode")
	}
	db := startPostgresContainer(t)
	ctx := context.Background()

	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	if m.State() != migrate.AppStateNeedsMigration {
		t.Fatalf("precondition: expected NeedsMigration on fresh DB, got %v", m.State())
	}

	pending, current, err := m.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if pending == 0 {
		t.Error("expected at least one pending migration on a fresh DB")
	}
	if current != "none" {
		t.Errorf("current = %q on fresh DB, want %q", current, "none")
	}

	// Status must NOT have applied any migrations.
	if m.State() != migrate.AppStateNeedsMigration {
		t.Errorf("Status mutated migrator state: got %v, want NeedsMigration", m.State())
	}
	pendingAfter, _, err := m.Status(ctx)
	if err != nil {
		t.Fatalf("Status (second call): %v", err)
	}
	if pendingAfter != pending {
		t.Errorf("pending changed between Status calls: %d -> %d (Status should be read-only)", pending, pendingAfter)
	}

	// Now apply migrations and confirm Status reports zero pending + a current name.
	if err := m.RunMigrations(ctx); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	pending2, current2, err := m.Status(ctx)
	if err != nil {
		t.Fatalf("Status after migrate: %v", err)
	}
	if pending2 != 0 {
		t.Errorf("pending after migrate = %d, want 0", pending2)
	}
	if current2 == "none" || current2 == "" {
		t.Errorf("current after migrate = %q, want a migration name", current2)
	}
}
