package main

import (
	"bytes"
	"context"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/drzero42/nexorious/internal/migrate"
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

// TestMigrator_Status_ReadOnly drives the Migrator.Status helper that backs
// `nexorious migrate status` and confirms it never applies migrations.
//
// TestMain has already run all migrations, so the shared testDB starts in the
// Ready state. We verify Status reports zero pending and a non-empty current
// version, and that calling Status twice returns stable results.
func TestMigrator_Status_ReadOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping container-backed test in -short mode")
	}
	truncateAllTables(t)
	ctx := context.Background()

	m := migrate.NewMigrator(testDB)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	// TestMain already applied all migrations, so the DB must be fully up to date.
	if m.State() != migrate.AppStateReady {
		t.Fatalf("precondition: expected Ready on migrated DB, got %v", m.State())
	}

	pending, current, err := m.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if pending != 0 {
		t.Errorf("expected 0 pending migrations on fully-migrated DB, got %d", pending)
	}
	if current == "none" || current == "" {
		t.Errorf("current = %q on migrated DB, want a migration name", current)
	}

	// Status must NOT mutate state — calling it twice must return stable results.
	if m.State() != migrate.AppStateReady {
		t.Errorf("Status mutated migrator state: got %v, want Ready", m.State())
	}
	pendingAfter, currentAfter, err := m.Status(ctx)
	if err != nil {
		t.Fatalf("Status (second call): %v", err)
	}
	if pendingAfter != pending {
		t.Errorf("pending changed between Status calls: %d -> %d (Status should be read-only)", pending, pendingAfter)
	}
	if currentAfter != current {
		t.Errorf("current changed between Status calls: %q -> %q (Status should be read-only)", current, currentAfter)
	}
}
