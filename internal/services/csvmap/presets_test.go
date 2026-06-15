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
