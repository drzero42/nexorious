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

// TestProfileListJSONRedactsKey verifies `profile list --json` emits the
// non-secret fields but never the stored API key (the bearer token in
// Profile.Key must not reach stdout/pipes).
func TestProfileListJSONRedactsKey(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	seed := &clicfg.Config{}
	seed.SetProfile("work", clicfg.Profile{
		URL:      "http://work:8000",
		Username: "alice",
		KeyName:  "cli@host",
		KeyID:    "key-123",
		Key:      "nxr_supersecret_token",
	})
	if err := clicfg.Save(seed); err != nil {
		t.Fatalf("seed: %v", err)
	}

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"--json", "profile", "list"})
	if err := root.Execute(); err != nil {
		t.Fatalf("list --json: %v", err)
	}

	got := out.String()
	if bytes.Contains([]byte(got), []byte("nxr_supersecret_token")) {
		t.Fatalf("profile list --json leaked the API key: %s", got)
	}
	if bytes.Contains([]byte(got), []byte("key-123")) {
		t.Fatalf("profile list --json leaked the key id: %s", got)
	}
	for _, want := range []string{"\"name\": \"work\"", "http://work:8000", "alice", "\"current\": true"} {
		if !bytes.Contains([]byte(got), []byte(want)) {
			t.Fatalf("profile list --json missing %q in: %s", want, got)
		}
	}
}
