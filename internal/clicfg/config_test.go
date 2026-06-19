package clicfg

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPathHonorsXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg-test")
	got, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	want := "/tmp/xdg-test/nexorious/config.yaml"
	if got != want {
		t.Fatalf("Path = %q, want %q", got, want)
	}
}

func TestPathFallsBackToHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/tmp/home-test")
	got, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	want := "/tmp/home-test/.config/nexorious/config.yaml"
	if got != want {
		t.Fatalf("Path = %q, want %q", got, want)
	}
}

func TestLoadMissingFileReturnsEmpty(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Profiles == nil {
		t.Fatal("Profiles map should be initialized, not nil")
	}
	if len(cfg.Profiles) != 0 {
		t.Fatalf("expected 0 profiles, got %d", len(cfg.Profiles))
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := &Config{}
	cfg.SetProfile("default", Profile{
		URL:      "http://localhost:8000",
		Username: "alice",
		KeyName:  "cli@host",
		KeyID:    "id-123",
		Key:      "nxr_secret",
	})
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Current != "default" {
		t.Fatalf("Current = %q, want default", got.Current)
	}
	p, ok := got.CurrentProfile()
	if !ok {
		t.Fatal("CurrentProfile not found")
	}
	if p.Key != "nxr_secret" || p.KeyID != "id-123" || p.URL != "http://localhost:8000" {
		t.Fatalf("round-trip mismatch: %+v", p)
	}
}

func TestSaveUsesOwnerOnlyPerms(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := &Config{}
	cfg.SetProfile("default", Profile{Key: "nxr_secret"})
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	p, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	info, err := os.Stat(p)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("file perm = %o, want 600", perm)
	}
	dirInfo, err := os.Stat(filepath.Dir(p))
	if err != nil {
		t.Fatalf("Stat dir: %v", err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0o700 {
		t.Fatalf("dir perm = %o, want 700", perm)
	}
}

func TestProfileHelpers(t *testing.T) {
	cfg := &Config{}
	cfg.SetProfile("default", Profile{URL: "u1", Key: "k1"})
	cfg.SetProfile("work", Profile{URL: "u2", Key: "k2"})

	if got := cfg.Names(); len(got) != 2 || got[0] != "default" || got[1] != "work" {
		t.Fatalf("Names = %v, want [default work]", got)
	}
	if p, ok := cfg.ProfileNamed("work"); !ok || p.Key != "k2" {
		t.Fatalf("ProfileNamed(work) = %+v,%v", p, ok)
	}
	// "" resolves to the default profile.
	if p, ok := cfg.ProfileNamed(""); !ok || p.Key != "k1" {
		t.Fatalf("ProfileNamed(\"\") = %+v,%v, want default profile k1", p, ok)
	}
	if err := cfg.SetCurrent("missing"); err == nil {
		t.Fatal("SetCurrent(missing) should error")
	}
	if err := cfg.SetCurrent("work"); err != nil || cfg.Current != "work" {
		t.Fatalf("SetCurrent(work) = %v, Current=%q", err, cfg.Current)
	}
	cfg.RemoveProfile("work")
	if _, ok := cfg.ProfileNamed("work"); ok {
		t.Fatal("work should be removed")
	}
	if cfg.Current != "" {
		t.Fatalf("Current should reset after removing the current profile, got %q", cfg.Current)
	}
}
