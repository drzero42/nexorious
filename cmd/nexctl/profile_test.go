package main

import (
	"bytes"
	"testing"

	"github.com/drzero42/nexorious/internal/clicfg"
)

func TestProfileAddUseRm(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	run := func(args ...string) (string, error) {
		root := newRootCmd()
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetIn(bytes.NewReader(nil))
		root.SetArgs(args)
		err := root.Execute()
		return out.String(), err
	}

	if _, err := run("profile", "add", "work", "--url", "http://work:8000"); err != nil {
		t.Fatalf("add: %v", err)
	}
	cfg, _ := clicfg.Load()
	if p, ok := cfg.Profile("work"); !ok || p.URL != "http://work:8000" {
		t.Fatalf("work profile = %+v ok=%v", p, ok)
	}
	if cfg.CurrentName() != "work" {
		t.Fatalf("add should switch current, got %q", cfg.CurrentName())
	}

	out, err := run("-q", "profile", "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !bytes.Contains([]byte(out), []byte("work")) {
		t.Fatalf("list -q = %q", out)
	}

	if _, err := run("profile", "rm", "work", "--yes"); err != nil {
		t.Fatalf("rm: %v", err)
	}
	cfg, _ = clicfg.Load()
	if _, ok := cfg.Profile("work"); ok {
		t.Fatal("work should be gone")
	}
}
