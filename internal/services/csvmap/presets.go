package csvmap

// Preset is a named CSV source whose mapping is baked in as a Config plus a
// header signature. Manual format selection (this issue) lists these; auto-detect
// (#1015) will match an upload's header against each Config's Signature.
type Preset struct {
	Slug        string // stable id used on the wire (e.g. "completionator")
	DisplayName string // shown in the import dialog
	Config      Config
}

// presetList is the registry of known CSV source presets.
var presetList = []Preset{
	{Slug: "completionator", DisplayName: "Completionator", Config: Completionator()},
}

// Presets returns the registered presets (a copy; callers must not mutate the registry).
func Presets() []Preset {
	out := make([]Preset, len(presetList))
	copy(out, presetList)
	return out
}

// PresetBySlug returns the Config for a preset slug. ok is false for an unknown
// or empty slug.
func PresetBySlug(slug string) (Config, bool) {
	for i := range presetList {
		if presetList[i].Slug == slug {
			return presetList[i].Config, true
		}
	}
	return Config{}, false
}
