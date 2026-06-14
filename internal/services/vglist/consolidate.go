package vglist

import (
	"slices"
	"strings"

	"github.com/drzero42/nexorious/internal/services/importmodel"
)

// platformTable maps a normalized vglist platform label (a free-form Wikidata
// English label) to a Nexorious platform slug. vglist has no fixed platform
// list, so this covers the common families and unmapped labels are preserved as
// provenance notes (never dropped). Keys are lowercased; see normalizeKey.
var platformTable = map[string]string{
	"microsoft windows": "pc-windows",
	"windows":           "pc-windows",
	"pc":                "pc-windows",
	"linux":             "pc-linux",
	"gnu/linux":         "pc-linux",
	"macos":             "mac",
	"mac os":            "mac",
	"mac os x":          "mac",
	"os x":              "mac",
	"mac":               "mac",

	"playstation":          "playstation",
	"playstation 2":        "playstation-2",
	"playstation 3":        "playstation-3",
	"playstation 4":        "playstation-4",
	"playstation 5":        "playstation-5",
	"playstation vita":     "playstation-vita",
	"playstation portable": "playstation-psp",
	"psp":                  "playstation-psp",

	"nintendo switch":                     "nintendo-switch",
	"nintendo switch 2":                   "nintendo-switch-2",
	"wii":                                 "nintendo-wii",
	"wii u":                               "nintendo-wii-u",
	"nintendo 3ds":                        "nintendo-3ds",
	"nintendo ds":                         "nintendo-ds",
	"nintendo 64":                         "nintendo-64",
	"gamecube":                            "nintendo-gamecube",
	"nintendo gamecube":                   "nintendo-gamecube",
	"super nintendo entertainment system": "nintendo-snes",
	"snes":                                "nintendo-snes",
	"nintendo entertainment system":       "nintendo-nes",
	"nes":                                 "nintendo-nes",
	"game boy":                            "nintendo-game-boy",
	"game boy advance":                    "nintendo-game-boy-advance",
	"game boy color":                      "nintendo-game-boy-color",

	"xbox":                       "xbox",
	"xbox 360":                   "xbox-360",
	"xbox one":                   "xbox-one",
	"xbox series x":              "xbox-series",
	"xbox series s":              "xbox-series",
	"xbox series x/s":            "xbox-series",
	"xbox series x and series s": "xbox-series",
	"xbox series x|s":            "xbox-series",

	"android": "android",
	"ios":     "ios",

	"sega genesis":    "sega-genesis",
	"sega mega drive": "sega-genesis",
	"mega drive":      "sega-genesis",
	"genesis":         "sega-genesis",
	"dreamcast":       "sega-dreamcast",
	"sega dreamcast":  "sega-dreamcast",
}

// storeMapping is a Nexorious storefront slug plus how it attaches to a platform.
// compatible is the set of platform slugs the storefront can plausibly sit on.
// defaultPlatform is synthesized when no compatible game platform is present;
// "" means none (console stores), so an unmatched store becomes a note instead.
type storeMapping struct {
	storefront      string
	defaultPlatform string
	compatible      []string
}

var pcPlatforms = []string{"pc-windows", "pc-linux", "mac"}

// storeTable maps a normalized vglist store name (an arbitrary admin-entered
// string) to its mapping. Keys are lowercased; see normalizeKey.
var storeTable = map[string]storeMapping{
	"steam":             {storefront: "steam", defaultPlatform: "pc-windows", compatible: pcPlatforms},
	"gog":               {storefront: "gog", defaultPlatform: "pc-windows", compatible: pcPlatforms},
	"gog.com":           {storefront: "gog", defaultPlatform: "pc-windows", compatible: pcPlatforms},
	"good old games":    {storefront: "gog", defaultPlatform: "pc-windows", compatible: pcPlatforms},
	"epic games store":  {storefront: "epic-games-store", defaultPlatform: "pc-windows", compatible: pcPlatforms},
	"epic games":        {storefront: "epic-games-store", defaultPlatform: "pc-windows", compatible: pcPlatforms},
	"epic":              {storefront: "epic-games-store", defaultPlatform: "pc-windows", compatible: pcPlatforms},
	"humble":            {storefront: "humble-bundle", defaultPlatform: "pc-windows", compatible: pcPlatforms},
	"humble store":      {storefront: "humble-bundle", defaultPlatform: "pc-windows", compatible: pcPlatforms},
	"humble bundle":     {storefront: "humble-bundle", defaultPlatform: "pc-windows", compatible: pcPlatforms},
	"itch.io":           {storefront: "itch-io", defaultPlatform: "pc-windows", compatible: pcPlatforms},
	"itch":              {storefront: "itch-io", defaultPlatform: "pc-windows", compatible: pcPlatforms},
	"origin":            {storefront: "origin-ea-app", defaultPlatform: "pc-windows", compatible: []string{"pc-windows"}},
	"ea app":            {storefront: "origin-ea-app", defaultPlatform: "pc-windows", compatible: []string{"pc-windows"}},
	"ea desktop":        {storefront: "origin-ea-app", defaultPlatform: "pc-windows", compatible: []string{"pc-windows"}},
	"uplay":             {storefront: "uplay", defaultPlatform: "pc-windows", compatible: []string{"pc-windows"}},
	"ubisoft connect":   {storefront: "uplay", defaultPlatform: "pc-windows", compatible: []string{"pc-windows"}},
	"ubisoft store":     {storefront: "uplay", defaultPlatform: "pc-windows", compatible: []string{"pc-windows"}},
	"gamersgate":        {storefront: "gamersgate", defaultPlatform: "pc-windows", compatible: []string{"pc-windows"}},
	"google play":       {storefront: "google-play-store", defaultPlatform: "android", compatible: []string{"android"}},
	"google play store": {storefront: "google-play-store", defaultPlatform: "android", compatible: []string{"android"}},
	"app store":         {storefront: "apple-app-store", defaultPlatform: "ios", compatible: []string{"ios", "mac"}},
	"apple app store":   {storefront: "apple-app-store", defaultPlatform: "ios", compatible: []string{"ios", "mac"}},

	// Console stores: no default platform — attach only to a compatible game
	// platform, else preserve as a note.
	"playstation store":   {storefront: "playstation-store", compatible: []string{"playstation-3", "playstation-4", "playstation-5", "playstation-vita", "playstation-psp"}},
	"playstation network": {storefront: "playstation-store", compatible: []string{"playstation-3", "playstation-4", "playstation-5", "playstation-vita", "playstation-psp"}},
	"psn":                 {storefront: "playstation-store", compatible: []string{"playstation-3", "playstation-4", "playstation-5", "playstation-vita", "playstation-psp"}},
	"nintendo eshop":      {storefront: "nintendo-eshop", compatible: []string{"nintendo-switch", "nintendo-switch-2", "nintendo-wii-u", "nintendo-3ds"}},
	"eshop":               {storefront: "nintendo-eshop", compatible: []string{"nintendo-switch", "nintendo-switch-2", "nintendo-wii-u", "nintendo-3ds"}},
	"microsoft store":     {storefront: "microsoft-store", compatible: []string{"xbox-360", "xbox-one", "xbox-series", "pc-windows"}},
	"xbox store":          {storefront: "microsoft-store", compatible: []string{"xbox-360", "xbox-one", "xbox-series", "pc-windows"}},
	"xbox games store":    {storefront: "microsoft-store", compatible: []string{"xbox-360", "xbox-one", "xbox-series", "pc-windows"}},
}

// normalizeKey lowercases and trims a label for case-insensitive table lookup.
func normalizeKey(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// consolidate turns a vglist entry's two unpaired sets (platforms, stores) into
// a deduped slice of (platform, storefront) ownership entries, recording
// unmapped/unmatched values as provenance notes. See docs/vglist-import.md.
func consolidate(e exportEntry, notes *noteBuilder) []importmodel.Platform {
	// 1. Map game platforms to slugs (order-preserving, deduped).
	var gamePlatforms []string
	gameSet := map[string]bool{}
	for _, p := range e.Platforms {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			continue
		}
		slug, ok := platformTable[normalizeKey(name)]
		if !ok {
			notes.add("Owned on " + name + " (no Nexorious platform mapping).")
			continue
		}
		if !gameSet[slug] {
			gameSet[slug] = true
			gamePlatforms = append(gamePlatforms, slug)
		}
	}

	type pkey struct{ platform, storefront string }
	var out []importmodel.Platform
	seen := map[pkey]bool{}
	hasStore := map[string]bool{}
	add := func(platform string, storefront *string) {
		sf := ""
		if storefront != nil {
			sf = *storefront
		}
		if seen[pkey{platform, sf}] {
			return
		}
		seen[pkey{platform, sf}] = true
		out = append(out, importmodel.Platform{Platform: platform, Storefront: storefront})
	}

	// 2. Attach each mapped store to a platform.
	for _, s := range e.Stores {
		name := strings.TrimSpace(s.Name)
		if name == "" {
			continue
		}
		sm, ok := storeTable[normalizeKey(name)]
		if !ok {
			notes.add("Store: " + name + " (no Nexorious storefront mapping).")
			continue
		}
		sf := sm.storefront
		matched := false
		for _, gp := range gamePlatforms {
			if slices.Contains(sm.compatible, gp) {
				add(gp, &sf)
				hasStore[gp] = true
				matched = true
			}
		}
		if matched {
			continue
		}
		if sm.defaultPlatform != "" {
			add(sm.defaultPlatform, &sf)
			hasStore[sm.defaultPlatform] = true
			continue
		}
		notes.add("Store: " + name + " (no compatible platform to attach).")
	}

	// 3. Emit a bare entry for every game platform that received no store.
	for _, gp := range gamePlatforms {
		if !hasStore[gp] {
			add(gp, nil)
		}
	}
	return out
}

// noteBuilder accumulates de-duplicated provenance lines and assembles the final
// personal_notes (verbatim comments first, then the provenance lines).
type noteBuilder struct {
	lines []string
	seen  map[string]bool
}

func (b *noteBuilder) add(line string) {
	if line == "" {
		return
	}
	if b.seen == nil {
		b.seen = map[string]bool{}
	}
	if b.seen[line] {
		return
	}
	b.seen[line] = true
	b.lines = append(b.lines, line)
}

// finalize joins verbatim comments with the provenance lines. Returns nil when
// there is nothing to record.
func (b *noteBuilder) finalize(comments string) *string {
	var sb strings.Builder
	if c := strings.TrimSpace(comments); c != "" {
		sb.WriteString(c)
	}
	if len(b.lines) > 0 {
		if sb.Len() > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(strings.Join(b.lines, "\n"))
	}
	if sb.Len() == 0 {
		return nil
	}
	s := sb.String()
	return &s
}
