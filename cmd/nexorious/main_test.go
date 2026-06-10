package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/drzero42/nexorious/internal/migrate"
)

// TestRootCmd_StructureAndSubcommands confirms the cobra tree exposes
// serve, migrate, migrate status, and version, and that --config is a
// persistent flag.
func TestRootCmd_StructureAndSubcommands(t *testing.T) {
	root := newRootCmd()

	if root.Use != "nexorious" {
		t.Errorf("root.Use = %q, want %q", root.Use, "nexorious")
	}

	// --config must be on persistent flags so subcommands inherit it.
	if root.PersistentFlags().Lookup("config") == nil {
		t.Error("expected --config to be a persistent flag on root")
	}

	wantSubcommands := map[string]bool{
		"serve":   false,
		"migrate": false,
		"version": false,
		"login":   false,
		"logout":  false,
		"whoami":  false,
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

// TestRootCmd_NoSubcommandPrintsHelp verifies a bare `./nexorious` prints the
// help overview and returns errNoSubcommand (so main exits non-zero) rather
// than starting the server.
func TestRootCmd_NoSubcommandPrintsHelp(t *testing.T) {
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{})

	err := root.Execute()
	if !errors.Is(err, errNoSubcommand) {
		t.Fatalf("bare invocation err = %v, want errNoSubcommand", err)
	}
	help := out.String()
	if !strings.Contains(help, "Usage:") || !strings.Contains(help, "serve") {
		t.Errorf("bare invocation should print the help overview, got:\n%s", help)
	}
}

// TestRootCmd_UnknownSubcommandErrors verifies a mistyped subcommand reports an
// "unknown command" error (with a suggestion) instead of falling through to the
// server, and that the error is not errNoSubcommand.
func TestRootCmd_UnknownSubcommandErrors(t *testing.T) {
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"serv"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected an error for an unknown subcommand")
	}
	if errors.Is(err, errNoSubcommand) {
		t.Error("unknown subcommand must not be treated as a bare invocation")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Errorf("err = %q, want it to mention 'unknown command'", err.Error())
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
	root.SetArgs([]string{"version", "--no-check"})

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
	for _, name := range []string{"serve", "migrate", "version", "login", "logout", "whoami"} {
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
