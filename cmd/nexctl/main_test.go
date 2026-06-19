package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootCmd_Structure(t *testing.T) {
	root := newRootCmd()
	if root.Use != "nexctl" {
		t.Errorf("root.Use = %q, want nexctl", root.Use)
	}
	for _, f := range []string{"profile", "json", "quiet", "yes"} {
		if root.PersistentFlags().Lookup(f) == nil {
			t.Errorf("expected persistent flag --%s", f)
		}
	}
	want := map[string]bool{"version": false, "account": false, "profile": false, "game": false, "tag": false, "pool": false, "sync": false, "job": false, "import": false, "export": false, "backup": false, "admin": false, "config": false, "login": false, "logout": false}
	for _, sub := range root.Commands() {
		if _, ok := want[sub.Name()]; ok {
			want[sub.Name()] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("expected subcommand %q", name)
		}
	}
}

func TestVersionCmd(t *testing.T) {
	withTestVersion(t)
	// Point config resolution at an empty dir so the command can't read a real
	// developer profile; with no profile the server line is just "unavailable".
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	root := newRootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"version"})
	if err := root.Execute(); err != nil {
		t.Fatalf("version: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "nexctl ") || !strings.Contains(got, "9.9.9-test") || !strings.Contains(got, "cafef00d") {
		t.Errorf("version output = %q", got)
	}
}
