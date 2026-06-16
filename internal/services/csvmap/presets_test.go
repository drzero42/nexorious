package csvmap

import "testing"

func TestPresets_IncludesCompletionator(t *testing.T) {
	var found *Preset
	for i := range presetList {
		if presetList[i].Slug == "completionator" {
			found = &presetList[i]
		}
	}
	if found == nil {
		t.Fatal("expected a 'completionator' preset in the registry")
	}
	if found.DisplayName != "Completionator" {
		t.Errorf("display name = %q, want Completionator", found.DisplayName)
	}
	if found.Config.Columns.Title != "Name" {
		t.Errorf("preset Config not wired to Completionator() (title col = %q)", found.Config.Columns.Title)
	}
}

func TestPresetBySlug(t *testing.T) {
	cfg, ok := PresetBySlug("completionator")
	if !ok {
		t.Fatal("PresetBySlug(completionator) ok = false, want true")
	}
	if cfg.Columns.Title != "Name" {
		t.Errorf("returned Config is not Completionator (title col = %q)", cfg.Columns.Title)
	}
	if _, ok := PresetBySlug("nope"); ok {
		t.Error("PresetBySlug(nope) ok = true, want false")
	}
	if _, ok := PresetBySlug(""); ok {
		t.Error("PresetBySlug(empty) ok = true, want false")
	}
}

func TestPresets_ReturnsCopy(t *testing.T) {
	got := Presets()
	if len(got) != len(presetList) {
		t.Fatalf("Presets() len = %d, want %d", len(got), len(presetList))
	}
}

func TestPresets_IncludesGrouvee(t *testing.T) {
	cfg, ok := PresetBySlug("grouvee")
	if !ok {
		t.Fatal("expected a 'grouvee' preset in the registry")
	}
	if cfg.Columns.Title != "name" {
		t.Errorf("grouvee preset not wired to Grouvee() (title col = %q)", cfg.Columns.Title)
	}
	var found bool
	for _, p := range Presets() {
		if p.Slug == "grouvee" && p.DisplayName == "Grouvee" {
			found = true
		}
	}
	if !found {
		t.Error("Presets() must list grouvee with DisplayName Grouvee")
	}
}

func TestPresets_IncludesDarkadia(t *testing.T) {
	cfg, ok := PresetBySlug("darkadia")
	if !ok {
		t.Fatal("darkadia preset not registered")
	}
	if cfg.Columns.Title != "Name" {
		t.Errorf("Title column = %q, want Name", cfg.Columns.Title)
	}
	if cfg.Platform.Tables == nil {
		t.Error("darkadia preset must use Platform.Tables")
	}
	found := false
	for _, p := range Presets() {
		if p.Slug == "darkadia" && p.DisplayName == "Darkadia" {
			found = true
		}
	}
	if !found {
		t.Error("Presets() omits the darkadia entry")
	}
}
