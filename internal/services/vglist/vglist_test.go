package vglist

import (
	"errors"
	"strings"
	"testing"

	"github.com/drzero42/nexorious/internal/services/importmodel"
)

// parseOne parses a single-entry export and returns the one game.
func parseOne(t *testing.T, body string) importmodel.Game {
	t.Helper()
	games, err := Parse([]byte("[" + body + "]"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("games = %d, want 1", len(games))
	}
	return games[0]
}

func platMap(g importmodel.Game) map[string]*string {
	m := map[string]*string{}
	for _, p := range g.Platforms {
		m[p.Platform] = p.Storefront
	}
	return m
}

func TestParse_RejectsNonArrayJSON(t *testing.T) {
	for _, body := range []string{`{"game":{"name":"X"}}`, `not json at all`, `"a string"`, ``} {
		_, err := Parse([]byte(body))
		if !errors.Is(err, importmodel.ErrInvalidSignature) {
			t.Errorf("Parse(%q) err = %v, want ErrInvalidSignature", body, err)
		}
	}
}

func TestParse_RejectsArrayWithoutGameNames(t *testing.T) {
	_, err := Parse([]byte(`[{"foo":1},{"bar":2}]`))
	if !errors.Is(err, importmodel.ErrInvalidSignature) {
		t.Errorf("err = %v, want ErrInvalidSignature", err)
	}
}

func TestParse_EmptyLibraryIsZeroGamesNoError(t *testing.T) {
	games, err := Parse([]byte(`[]`))
	if err != nil {
		t.Fatalf("Parse([]) err = %v, want nil", err)
	}
	if len(games) != 0 {
		t.Errorf("games = %d, want 0", len(games))
	}
}

// TestParse_WrapperObjectRoot covers the real vglist export shape: a top-level
// { user, games } object rather than a bare array. The entries under .games are
// mapped exactly as the array form is. Regression for #1029.
func TestParse_WrapperObjectRoot(t *testing.T) {
	games, err := Parse([]byte(`{
		"user": {"id": 2882, "username": "drzero"},
		"games": [
			{"game": {"id": 1, "name": "Half-Life 2", "wikidata_id": 193581},
			 "hours_played": 12.5, "completion_status": "completed", "rating": 95,
			 "comments": "Great game.", "replay_count": 0,
			 "platforms": [{"id": 3, "name": "Microsoft Windows"}], "stores": [{"id": 2, "name": "Steam"}]},
			{"game": {"id": 42, "name": "Some Obscure Game", "wikidata_id": null},
			 "platforms": [], "stores": []}
		]
	}`))
	if err != nil {
		t.Fatalf("Parse(wrapper) err = %v, want nil", err)
	}
	if len(games) != 2 {
		t.Fatalf("games = %d, want 2", len(games))
	}
	if games[0].Title != "Half-Life 2" || games[1].Title != "Some Obscure Game" {
		t.Errorf("titles = %q, %q", games[0].Title, games[1].Title)
	}
}

// TestParse_EmptyWrapperLibraryIsZeroGamesNoError covers an empty real export:
// the wrapper carries a present-but-empty games array.
func TestParse_EmptyWrapperLibraryIsZeroGamesNoError(t *testing.T) {
	games, err := Parse([]byte(`{"user": {"id": 1, "username": "x"}, "games": []}`))
	if err != nil {
		t.Fatalf("Parse(empty wrapper) err = %v, want nil", err)
	}
	if len(games) != 0 {
		t.Errorf("games = %d, want 0", len(games))
	}
}

// TestParse_RejectsObjectWithoutGamesKey ensures an arbitrary object without a
// games key is not mistaken for an empty vglist library.
func TestParse_RejectsObjectWithoutGamesKey(t *testing.T) {
	for _, body := range []string{`{"foo": 1}`, `{"user": {"id": 1}}`} {
		if _, err := Parse([]byte(body)); !errors.Is(err, importmodel.ErrInvalidSignature) {
			t.Errorf("Parse(%q) err = %v, want ErrInvalidSignature", body, err)
		}
	}
}

func TestParse_BasicFields(t *testing.T) {
	g := parseOne(t, `{
		"game": {"id": 1, "name": "Half-Life 2", "wikidata_id": 193581},
		"hours_played": 12.5,
		"completion_status": "completed",
		"rating": 95,
		"comments": "Great game.",
		"replay_count": 0,
		"platforms": [{"id": 3, "name": "Microsoft Windows"}],
		"stores": [{"id": 2, "name": "Steam"}]
	}`)
	if g.Title != "Half-Life 2" {
		t.Errorf("title = %q", g.Title)
	}
	if g.PlayStatus != "completed" {
		t.Errorf("play_status = %q, want completed", g.PlayStatus)
	}
	if g.PersonalRating == nil || *g.PersonalRating != 5 {
		t.Errorf("rating = %v, want 5", g.PersonalRating)
	}
	if g.HoursPlayed == nil || *g.HoursPlayed != 12.5 {
		t.Errorf("hours = %v, want 12.5", g.HoursPlayed)
	}
	if g.PersonalNotes == nil || *g.PersonalNotes != "Great game." {
		t.Errorf("notes = %v, want verbatim comments", g.PersonalNotes)
	}
	if g.IsLoved {
		t.Errorf("is_loved = true, want false")
	}
	if g.CreatedAt != "" {
		t.Errorf("created_at = %q, want empty", g.CreatedAt)
	}
}

func TestParse_SkipsNamelessEntryButKeepsValidOnes(t *testing.T) {
	games, err := Parse([]byte(`[
		{"game":{"name":""},"platforms":[],"stores":[]},
		{"game":{"name":"Real Game"},"platforms":[],"stores":[]}
	]`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(games) != 1 || games[0].Title != "Real Game" {
		t.Fatalf("games = %+v, want only Real Game", games)
	}
}

func TestMapCompletionStatus(t *testing.T) {
	cases := map[string]string{
		"unplayed":        "not_started",
		"in_progress":     "in_progress",
		"paused":          "shelved",
		"dropped":         "dropped",
		"completed":       "completed",
		"fully_completed": "mastered",
		"not_applicable":  "not_started",
		"":                "not_started",
		"something_weird": "not_started",
	}
	for in, want := range cases {
		if got := mapCompletionStatus(in); got != want {
			t.Errorf("mapCompletionStatus(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMapRating(t *testing.T) {
	cases := []struct {
		in   *int32
		want *int32
	}{
		{nil, nil},
		{i32(0), nil},
		{i32(100), i32(5)},
		{i32(95), i32(5)},
		{i32(50), i32(3)},
		{i32(30), i32(2)},
		{i32(10), i32(1)},
		{i32(5), i32(1)},
		{i32(1), i32(1)},
	}
	for _, c := range cases {
		got := mapRating(c.in)
		switch {
		case c.want == nil && got != nil:
			t.Errorf("mapRating(%v) = %v, want nil", derefI32(c.in), *got)
		case c.want != nil && (got == nil || *got != *c.want):
			t.Errorf("mapRating(%v) = %v, want %v", derefI32(c.in), got, *c.want)
		}
	}
}

func TestParse_NullCompletionAndRatingAndHours(t *testing.T) {
	g := parseOne(t, `{"game":{"name":"X"},"completion_status":null,"rating":null,"hours_played":null,"comments":"","replay_count":0,"platforms":[],"stores":[]}`)
	if g.PlayStatus != "not_started" {
		t.Errorf("play_status = %q, want not_started", g.PlayStatus)
	}
	if g.PersonalRating != nil {
		t.Errorf("rating = %v, want nil", g.PersonalRating)
	}
	if g.HoursPlayed != nil {
		t.Errorf("hours = %v, want nil", g.HoursPlayed)
	}
	if g.PersonalNotes != nil {
		t.Errorf("notes = %v, want nil", g.PersonalNotes)
	}
}

func TestPlatform_UnmappedGoesToNote(t *testing.T) {
	g := parseOne(t, `{"game":{"name":"X"},"comments":"","platforms":[{"name":"Sega Saturn"}],"stores":[]}`)
	if len(g.Platforms) != 0 {
		t.Errorf("platforms = %+v, want none", g.Platforms)
	}
	if g.PersonalNotes == nil || !strings.Contains(*g.PersonalNotes, "Sega Saturn") {
		t.Errorf("unmapped platform not in notes: %v", g.PersonalNotes)
	}
}

func TestPlatform_AliasesMapToSlugs(t *testing.T) {
	cases := map[string]string{
		"Microsoft Windows": "pc-windows",
		"Windows":           "pc-windows",
		"PC":                "pc-windows",
		"Linux":             "pc-linux",
		"macOS":             "mac",
		"Mac OS X":          "mac",
		"PlayStation 4":     "playstation-4",
		"PlayStation Vita":  "playstation-vita",
		"Nintendo Switch":   "nintendo-switch",
		"Xbox 360":          "xbox-360",
		"Android":           "android",
		"iOS":               "ios",
	}
	for name, slug := range cases {
		g := parseOne(t, `{"game":{"name":"X"},"comments":"","platforms":[{"name":"`+name+`"}],"stores":[]}`)
		if len(g.Platforms) != 1 || g.Platforms[0].Platform != slug {
			t.Errorf("platform %q → %+v, want slug %q", name, g.Platforms, slug)
		}
		if g.Platforms[0].Storefront != nil {
			t.Errorf("platform %q got storefront %v, want nil", name, g.Platforms[0].Storefront)
		}
	}
}

func TestStore_PCStorePairsToDefaultPlatformWhenNoPlatformListed(t *testing.T) {
	g := parseOne(t, `{"game":{"name":"X"},"comments":"","platforms":[],"stores":[{"name":"Steam"}]}`)
	if len(g.Platforms) != 1 {
		t.Fatalf("platforms = %+v, want 1 synthesized", g.Platforms)
	}
	p := g.Platforms[0]
	if p.Platform != "pc-windows" || p.Storefront == nil || *p.Storefront != "steam" {
		t.Errorf("entry = %+v, want (pc-windows, steam)", p)
	}
}

func TestStore_PCStoreUpgradesExistingPlatformNotDuplicate(t *testing.T) {
	g := parseOne(t, `{"game":{"name":"X"},"comments":"","platforms":[{"name":"Microsoft Windows"}],"stores":[{"name":"Steam"}]}`)
	if len(g.Platforms) != 1 {
		t.Fatalf("platforms = %+v, want 1 (no bare duplicate)", g.Platforms)
	}
	if sf := g.Platforms[0].Storefront; sf == nil || *sf != "steam" {
		t.Errorf("pc-windows storefront = %v, want steam", sf)
	}
}

func TestStore_PCStorePrefersListedLinuxOverDefault(t *testing.T) {
	// Steam is compatible with pc-linux; when the entry lists Linux (and no
	// Windows), Steam attaches to pc-linux rather than synthesizing pc-windows.
	g := parseOne(t, `{"game":{"name":"X"},"comments":"","platforms":[{"name":"Linux"}],"stores":[{"name":"Steam"}]}`)
	m := platMap(g)
	if len(g.Platforms) != 1 {
		t.Fatalf("platforms = %+v, want 1", g.Platforms)
	}
	if sf, ok := m["pc-linux"]; !ok || sf == nil || *sf != "steam" {
		t.Errorf("pc-linux storefront = %v, want steam", sf)
	}
}

func TestStore_ConsoleStorePairsToCompatibleGamePlatform(t *testing.T) {
	g := parseOne(t, `{"game":{"name":"X"},"comments":"","platforms":[{"name":"PlayStation 4"}],"stores":[{"name":"PlayStation Network"}]}`)
	m := platMap(g)
	if sf, ok := m["playstation-4"]; !ok || sf == nil || *sf != "playstation-store" {
		t.Errorf("ps4 storefront = %v, want playstation-store", sf)
	}
}

func TestStore_ConsoleStoreNoCompatiblePlatformGoesToNote(t *testing.T) {
	// Nintendo eShop with only a PC platform listed → no compatible platform,
	// no default platform → store preserved as a note, PC platform stays bare.
	g := parseOne(t, `{"game":{"name":"X"},"comments":"","platforms":[{"name":"Microsoft Windows"}],"stores":[{"name":"Nintendo eShop"}]}`)
	m := platMap(g)
	if sf, ok := m["pc-windows"]; !ok || sf != nil {
		t.Errorf("pc-windows = %v, want bare (nil storefront)", sf)
	}
	if g.PersonalNotes == nil || !strings.Contains(*g.PersonalNotes, "Nintendo eShop") {
		t.Errorf("unmatched console store not in notes: %v", g.PersonalNotes)
	}
}

func TestStore_MultipleStoresOnOnePlatformYieldTwoEntries(t *testing.T) {
	g := parseOne(t, `{"game":{"name":"X"},"comments":"","platforms":[{"name":"Windows"}],"stores":[{"name":"Steam"},{"name":"GOG"}]}`)
	if len(g.Platforms) != 2 {
		t.Fatalf("platforms = %+v, want 2 (steam + gog)", g.Platforms)
	}
	sfs := map[string]bool{}
	for _, p := range g.Platforms {
		if p.Platform != "pc-windows" {
			t.Errorf("platform = %q, want pc-windows", p.Platform)
		}
		if p.Storefront != nil {
			sfs[*p.Storefront] = true
		}
	}
	if !sfs["steam"] || !sfs["gog"] {
		t.Errorf("storefronts = %v, want steam+gog", sfs)
	}
}

func TestStore_UnmappedStoreGoesToNote(t *testing.T) {
	g := parseOne(t, `{"game":{"name":"X"},"comments":"","platforms":[{"name":"Windows"}],"stores":[{"name":"Battle.net"}]}`)
	if g.PersonalNotes == nil || !strings.Contains(*g.PersonalNotes, "Battle.net") {
		t.Errorf("unmapped store not in notes: %v", g.PersonalNotes)
	}
}

func TestStore_BarePlatformWhenNoStore(t *testing.T) {
	g := parseOne(t, `{"game":{"name":"X"},"comments":"","platforms":[{"name":"Nintendo Switch"}],"stores":[]}`)
	m := platMap(g)
	if sf, ok := m["nintendo-switch"]; !ok || sf != nil {
		t.Errorf("nintendo-switch = %v, want bare (nil storefront)", sf)
	}
}

func TestProvenanceNotes_DatesAndReplayCount(t *testing.T) {
	g := parseOne(t, `{"game":{"name":"X"},"comments":"base note","start_date":"2024-01-10","completion_date":"2024-02-01","replay_count":2,"platforms":[],"stores":[]}`)
	if g.PersonalNotes == nil {
		t.Fatal("notes nil")
	}
	notes := *g.PersonalNotes
	for _, want := range []string{"base note", "2024-01-10", "2024-02-01", "Replayed 2"} {
		if !strings.Contains(notes, want) {
			t.Errorf("notes missing %q in: %s", want, notes)
		}
	}
	// comments must come first, verbatim.
	if !strings.HasPrefix(notes, "base note") {
		t.Errorf("notes should start with comments verbatim: %s", notes)
	}
}

func TestNoStorefrontHasAcquiredDate(t *testing.T) {
	g := parseOne(t, `{"game":{"name":"X"},"comments":"","platforms":[{"name":"Windows"}],"stores":[{"name":"Steam"}]}`)
	for _, p := range g.Platforms {
		if p.AcquiredDate != "" {
			t.Errorf("acquired_date = %q, want empty (vglist has no purchase date)", p.AcquiredDate)
		}
	}
}

func i32(v int32) *int32 { return &v }

func derefI32(p *int32) any {
	if p == nil {
		return "nil"
	}
	return *p
}
