package darkadia

import (
	"encoding/json"
	"strings"
	"testing"
)

// canonicalHeaderLine is the real 29-column header (quoting is incidental).
const canonicalHeaderLine = `Name,Added,Loved,Owned,Played,Playing,Finished,Mastered,Dominated,Shelved,Rating,"Copy label","Copy Release","Copy platform","Copy media","Copy media other","Copy source","Copy source other","Copy purchase date","Copy box","Copy box condition","Copy box notes","Copy manual","Copy manual condition","Copy manual notes","Copy complete","Copy complete notes",Platforms,Notes`

func TestParse_RejectsNonDarkadiaHeader(t *testing.T) {
	_, err := Parse([]byte("foo,bar,baz\n1,2,3\n"))
	if err == nil || !strings.Contains(err.Error(), "Darkadia") {
		t.Fatalf("want Darkadia header error, got %v", err)
	}
}

func TestParse_GroupsRowsIntoGamesAndCopies(t *testing.T) {
	csv := canonicalHeaderLine + "\n" +
		`Game A,2013-06-05,0,1,0,0,0,0,0,0,,"","","PC","Digital","","Steam","","2013-06-05","","","","","","","","","PC","note A"` + "\n" +
		`,,,,,,,,,,,"","","Mac","Digital","","GOG","","2014-01-01","","","","","","","","","",""` + "\n" +
		`Game B,2015-02-02,1,1,1,0,0,0,0,0,4.5,"","","PlayStation 4","Digital","","Sony Entertainment Network","","2015-02-02","","","","","","","","","PlayStation 4","note B"` + "\n"
	games, err := Parse([]byte(csv))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(games) != 2 {
		t.Fatalf("games = %d, want 2", len(games))
	}
	if games[0].Title != "Game A" || games[1].Title != "Game B" {
		t.Fatalf("titles = %q, %q", games[0].Title, games[1].Title)
	}
}

func TestParse_ToleratesRaggedRowsAndEmbeddedNewline(t *testing.T) {
	csv := canonicalHeaderLine + "\n" +
		`Ragged,2013-06-05,0,1,0,0,0,0,0,0` + "\n" + // only 10 fields
		`Multi,2013-06-05,0,1,0,0,0,0,0,0,,"","","PC","Digital","","Steam","","2013-06-05","","","","","","","","","PC","line one` + "\n" + `line two"` + "\n"
	games, err := Parse([]byte(csv))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(games) != 2 {
		t.Fatalf("games = %d, want 2", len(games))
	}
	if games[1].PersonalNotes == nil || !strings.Contains(*games[1].PersonalNotes, "line one\nline two") {
		t.Fatalf("embedded newline not preserved: %+v", games[1].PersonalNotes)
	}
}

func TestConsolidate_PlayStatusPrecedence(t *testing.T) {
	cases := []struct {
		flags map[int]string // column index → "1"
		want  string
	}{
		{map[int]string{colOwned: "1"}, "not_started"},
		{map[int]string{colOwned: "1", colPlayed: "1"}, "shelved"},
		{map[int]string{colOwned: "1", colPlayed: "1", colPlaying: "1"}, "in_progress"},
		{map[int]string{colOwned: "1", colShelved: "1"}, "dropped"},
		{map[int]string{colOwned: "1", colFinished: "1"}, "completed"},
		{map[int]string{colMastered: "1", colFinished: "1"}, "mastered"},
		{map[int]string{colDominated: "1", colMastered: "1"}, "dominated"},
		{map[int]string{colShelved: "1", colPlaying: "1"}, "dropped"},
		{map[int]string{colFinished: "1", colShelved: "1"}, "completed"},
	}
	for i, c := range cases {
		row := make([]string, len(header))
		row[colName] = "G"
		for idx, v := range c.flags {
			row[idx] = v
		}
		got := consolidate(rawGame{named: row, copies: [][]string{row}})
		if got.PlayStatus != c.want {
			t.Errorf("case %d: play_status = %q, want %q", i, got.PlayStatus, c.want)
		}
	}
}

func TestConsolidate_RatingTruncatedAndLovedAndCreatedAt(t *testing.T) {
	row := make([]string, len(header))
	row[colName] = "G"
	row[colOwned] = "1"
	row[colLoved] = "1"
	row[colRating] = "4.5"
	row[colAdded] = "2013-06-05"
	row[colNotes] = "my note"
	g := consolidate(rawGame{named: row, copies: [][]string{row}})
	if g.PersonalRating == nil || *g.PersonalRating != 4 {
		t.Errorf("rating = %v, want 4", g.PersonalRating)
	}
	if !g.IsLoved {
		t.Errorf("is_loved = false, want true")
	}
	if g.CreatedAt != "2013-06-05" {
		t.Errorf("created_at = %q, want 2013-06-05", g.CreatedAt)
	}
	if g.PersonalNotes == nil || *g.PersonalNotes != "my note" {
		t.Errorf("notes = %v, want verbatim", g.PersonalNotes)
	}
}

func TestConsolidate_EmptyRatingIsUnrated(t *testing.T) {
	row := make([]string, len(header))
	row[colName] = "G"
	row[colOwned] = "1"
	row[colRating] = ""
	g := consolidate(rawGame{named: row, copies: [][]string{row}})
	if g.PersonalRating != nil {
		t.Errorf("rating = %v, want nil", g.PersonalRating)
	}
	row[colRating] = "0"
	g = consolidate(rawGame{named: row, copies: [][]string{row}})
	if g.PersonalRating != nil {
		t.Errorf("rating 0 → %v, want nil", g.PersonalRating)
	}
}

func mkRow(name string, fields map[int]string) []string {
	row := make([]string, len(header))
	row[colName] = name
	row[colOwned] = "1"
	for i, v := range fields {
		row[i] = v
	}
	return row
}

func TestConsolidate_Anodyne_PCWithGOGCopy_MacNoCopy(t *testing.T) {
	named := mkRow("Anodyne", map[int]string{
		colPlatforms:    "PC, Mac",
		colCopyPlatform: "PC", colCopyMedia: "Digital", colCopySource: "GOG",
		colCopyPurchase: "2014-03-01",
	})
	g := consolidate(rawGame{named: named, copies: [][]string{named}})
	got := map[string]*string{}
	dates := map[string]string{}
	for _, p := range g.Platforms {
		got[p.Platform] = p.Storefront
		dates[p.Platform] = p.AcquiredDate
	}
	if len(g.Platforms) != 2 {
		t.Fatalf("platforms = %+v, want pc-windows+mac", g.Platforms)
	}
	if got["pc-windows"] == nil || *got["pc-windows"] != "gog" || dates["pc-windows"] != "2014-03-01" {
		t.Errorf("pc-windows = %v (%q), want gog/2014-03-01", got["pc-windows"], dates["pc-windows"])
	}
	if sf, ok := got["mac"]; !ok || sf != nil {
		t.Errorf("mac = %v, want present with nil storefront", sf)
	}
}

func TestConsolidate_Aaru_PS3andPS4_viaPSN(t *testing.T) {
	named := mkRow("Aaru's Awakening", map[int]string{
		colPlatforms:    "PlayStation Network (PS3), PlayStation 4",
		colCopyPlatform: "PlayStation Network (PS3)", colCopyMedia: "Digital",
		colCopySource: "Sony Entertainment Network", colCopyPurchase: "2015-02-02",
	})
	cont := make([]string, len(header))
	cont[colCopyPlatform] = "PlayStation 4"
	cont[colCopyMedia] = "Digital"
	cont[colCopySource] = "Sony Entertainment Network"
	g := consolidate(rawGame{named: named, copies: [][]string{named, cont}})
	got := map[string]*string{}
	for _, p := range g.Platforms {
		got[p.Platform] = p.Storefront
	}
	if got["playstation-3"] == nil || *got["playstation-3"] != "playstation-store" {
		t.Errorf("ps3 = %v, want playstation-store", got["playstation-3"])
	}
	if got["playstation-4"] == nil || *got["playstation-4"] != "playstation-store" {
		t.Errorf("ps4 = %v, want playstation-store", got["playstation-4"])
	}
}

func TestConsolidate_StorefrontRules(t *testing.T) {
	phys := mkRow("Phys", map[int]string{
		colPlatforms: "PlayStation 4", colCopyPlatform: "PlayStation 4",
		colCopyMedia: "Physical", colCopySource: "GameStop",
	})
	g := consolidate(rawGame{named: phys, copies: [][]string{phys}})
	if g.Platforms[0].Storefront == nil || *g.Platforms[0].Storefront != "physical" {
		t.Errorf("physical storefront = %v", g.Platforms[0].Storefront)
	}
	if g.PersonalNotes == nil || !strings.Contains(*g.PersonalNotes, "GameStop") {
		t.Errorf("physical retailer not in notes: %v", g.PersonalNotes)
	}

	unrec := mkRow("Unrec", map[int]string{
		colPlatforms: "PC", colCopyPlatform: "PC", colCopyMedia: "Digital",
		colCopySource: "Fanatical",
	})
	g = consolidate(rawGame{named: unrec, copies: [][]string{unrec}})
	if g.Platforms[0].Storefront != nil {
		t.Errorf("unrecognized digital storefront = %v, want nil", g.Platforms[0].Storefront)
	}
	if g.PersonalNotes == nil || !strings.Contains(*g.PersonalNotes, "Fanatical") {
		t.Errorf("unrecognized source not in notes: %v", g.PersonalNotes)
	}

	empty := mkRow("Empty", map[int]string{
		colPlatforms: "PC", colCopyPlatform: "PC", colCopyMedia: "Digital",
	})
	g = consolidate(rawGame{named: empty, copies: [][]string{empty}})
	if g.Platforms[0].Storefront != nil || g.PersonalNotes != nil {
		t.Errorf("empty source: storefront=%v notes=%v, want nil/nil", g.Platforms[0].Storefront, g.PersonalNotes)
	}

	epic := mkRow("Epic", map[int]string{
		colPlatforms: "PC", colCopyPlatform: "PC", colCopyMedia: "Digital",
		colCopySource: "Other", colCopySourceOther: "Epic Game Store",
	})
	g = consolidate(rawGame{named: epic, copies: [][]string{epic}})
	if g.Platforms[0].Storefront == nil || *g.Platforms[0].Storefront != "epic-games-store" {
		t.Errorf("epic variant storefront = %v, want epic-games-store", g.Platforms[0].Storefront)
	}
}

func TestConsolidate_UnmappedPlatform_GoesToNotesNotFailure(t *testing.T) {
	named := mkRow("Weird", map[int]string{
		colPlatforms: "Sega Saturn",
	})
	g := consolidate(rawGame{named: named, copies: [][]string{named}})
	if len(g.Platforms) != 0 {
		t.Errorf("platforms = %+v, want none (unmapped → note)", g.Platforms)
	}
	if g.PersonalNotes == nil || !strings.Contains(*g.PersonalNotes, "Sega Saturn") {
		t.Errorf("unmapped platform not preserved in notes: %v", g.PersonalNotes)
	}
}

func TestConsolidate_NoPlatformGame(t *testing.T) {
	named := mkRow("Bare", nil)
	g := consolidate(rawGame{named: named, copies: [][]string{named}})
	if len(g.Platforms) != 0 {
		t.Errorf("platforms = %+v, want none", g.Platforms)
	}
}

func TestConsolidate_DedupOnPlatformStorefront(t *testing.T) {
	named := mkRow("Dup", map[int]string{
		colPlatforms: "PC", colCopyPlatform: "PC", colCopyMedia: "Digital", colCopySource: "Steam",
		colCopyPurchase: "2013-01-01",
	})
	cont := make([]string, len(header))
	cont[colCopyPlatform] = "PC"
	cont[colCopyMedia] = "Digital"
	cont[colCopySource] = "Steam"
	cont[colCopyPurchase] = "2014-01-01"
	g := consolidate(rawGame{named: named, copies: [][]string{named, cont}})
	if len(g.Platforms) != 1 {
		t.Fatalf("platforms = %+v, want 1 deduped", g.Platforms)
	}
	if g.Platforms[0].AcquiredDate != "2013-01-01" {
		t.Errorf("kept date = %q, want earliest 2013-01-01", g.Platforms[0].AcquiredDate)
	}
}

func TestGame_JSONRoundTrip(t *testing.T) {
	named := mkRow("J", map[int]string{colPlatforms: "PC", colCopyPlatform: "PC", colCopyMedia: "Digital", colCopySource: "Steam"})
	g := consolidate(rawGame{named: named, copies: [][]string{named}})
	b, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	var back Game
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.Platforms[0].Platform != "pc-windows" {
		t.Errorf("round-trip platform = %q", back.Platforms[0].Platform)
	}
}
