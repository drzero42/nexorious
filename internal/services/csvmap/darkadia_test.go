package csvmap

import (
	"bytes"
	"encoding/csv"
	"errors"
	"strings"
	"testing"

	"github.com/drzero42/nexorious/internal/services/importmodel"
)

// dkHeader is the canonical 29-column required Darkadia header.
var dkHeader = []string{
	"Name", "Added", "Loved", "Owned", "Played", "Playing", "Finished",
	"Mastered", "Dominated", "Shelved", "Rating", "Copy label", "Copy Release",
	"Copy platform", "Copy media", "Copy media other", "Copy source",
	"Copy source other", "Copy purchase date", "Copy box", "Copy box condition",
	"Copy box notes", "Copy manual", "Copy manual condition", "Copy manual notes",
	"Copy complete", "Copy complete notes", "Platforms", "Notes",
}

// dkExtHeader appends the 5 optional feature-toggle columns an extended export adds.
var dkExtHeader = append(append([]string{}, dkHeader...),
	"Tags", "Time played", "Review subject", "Review", "Copy notes")

// dkRow builds one CSV record for header h: Name plus column overrides by header name.
func dkRow(h []string, name string, set map[string]string) []string {
	idx := map[string]int{}
	for i, c := range h {
		idx[c] = i
	}
	rec := make([]string, len(h))
	rec[idx["Name"]] = name
	for col, val := range set {
		rec[idx[col]] = val
	}
	return rec
}

// dkCSV renders header h plus rows into RFC-4180 CSV bytes.
func dkCSV(h []string, rows ...[]string) []byte {
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	if err := w.Write(h); err != nil {
		panic(err)
	}
	for _, r := range rows {
		if err := w.Write(r); err != nil {
			panic(err)
		}
	}
	w.Flush()
	return b.Bytes()
}

// parseDK parses CSV bytes with the Darkadia preset, failing the test on error.
func parseDK(t *testing.T, raw []byte) []importmodel.Game {
	t.Helper()
	games, err := Parse(raw, Darkadia())
	if err != nil {
		t.Fatalf("Parse(Darkadia): %v", err)
	}
	return games
}

func TestDarkadia_AcceptsExtendedHeaderAndStatusByName(t *testing.T) {
	// Played=1, not finished -> "shelved"; PC copy bought on Steam, 148h, tagged.
	row := dkRow(dkExtHeader, "Game X", map[string]string{
		"Owned": "1", "Played": "1", "Copy platform": "PC", "Copy media": "Digital",
		"Copy source": "Steam", "Copy purchase date": "2013-06-05",
		"Platforms": "PC", "Time played": "148:00", "Tags": "Co-op, VR", "Notes": "my note",
	})
	games := parseDK(t, dkCSV(dkExtHeader, row))
	if len(games) != 1 {
		t.Fatalf("games = %d, want 1", len(games))
	}
	g := games[0]
	if g.Title != "Game X" {
		t.Errorf("title = %q", g.Title)
	}
	if len(g.Platforms) == 0 || g.Platforms[0].Platform != "pc-windows" {
		t.Errorf("platforms = %+v", g.Platforms)
	}
	if g.PersonalNotes == nil || !strings.Contains(*g.PersonalNotes, "my note") {
		t.Errorf("notes = %v", g.PersonalNotes)
	}
	if g.PlayStatus != "shelved" {
		t.Errorf("play_status = %q, want shelved", g.PlayStatus)
	}
}

func TestDarkadia_RejectsNonDarkadiaHeader(t *testing.T) {
	_, err := Parse([]byte("foo,bar,baz\n1,2,3\n"), Darkadia())
	if !errors.Is(err, importmodel.ErrInvalidSignature) {
		t.Fatalf("err = %v, want wrapping ErrInvalidSignature", err)
	}
}

func TestDarkadia_GroupsRowsIntoGamesAndCopies(t *testing.T) {
	gameA := dkRow(dkHeader, "Game A", map[string]string{
		"Owned": "1", "Copy platform": "PC", "Copy media": "Digital", "Copy source": "Steam",
		"Copy purchase date": "2013-06-05", "Platforms": "PC", "Notes": "note A",
	})
	contA := dkRow(dkHeader, "", map[string]string{
		"Copy platform": "Mac", "Copy media": "Digital", "Copy source": "GOG",
		"Copy purchase date": "2014-01-01",
	})
	gameB := dkRow(dkHeader, "Game B", map[string]string{
		"Owned": "1", "Played": "1", "Loved": "1", "Rating": "4.5",
		"Copy platform": "PlayStation 4", "Copy media": "Digital",
		"Copy source": "Sony Entertainment Network", "Copy purchase date": "2015-02-02",
		"Platforms": "PlayStation 4", "Notes": "note B",
	})
	games := parseDK(t, dkCSV(dkHeader, gameA, contA, gameB))
	if len(games) != 2 {
		t.Fatalf("games = %d, want 2", len(games))
	}
	if games[0].Title != "Game A" || games[1].Title != "Game B" {
		t.Fatalf("titles = %q, %q", games[0].Title, games[1].Title)
	}
}

func TestDarkadia_ToleratesRaggedRowsAndEmbeddedNewline(t *testing.T) {
	// A ragged short row (10 fields) and a row with an embedded newline in Notes.
	raw := []byte(strings.Join(dkHeader, ",") + "\n" +
		`Ragged,2013-06-05,0,1,0,0,0,0,0,0` + "\n" +
		`Multi,2013-06-05,0,1,0,0,0,0,0,0,,"","","PC","Digital","","Steam","","2013-06-05","","","","","","","","","PC","line one` + "\n" + `line two"` + "\n")
	games := parseDK(t, raw)
	if len(games) != 2 {
		t.Fatalf("games = %d, want 2", len(games))
	}
	if games[1].PersonalNotes == nil || !strings.Contains(*games[1].PersonalNotes, "line one\nline two") {
		t.Fatalf("embedded newline not preserved: %+v", games[1].PersonalNotes)
	}
}

func TestDarkadia_PlayStatusPrecedence(t *testing.T) {
	cases := []struct {
		on   map[string]string
		want string
	}{
		{map[string]string{"Owned": "1"}, "not_started"},
		{map[string]string{"Owned": "1", "Played": "1"}, "shelved"},
		{map[string]string{"Owned": "1", "Played": "1", "Playing": "1"}, "in_progress"},
		{map[string]string{"Owned": "1", "Shelved": "1"}, "dropped"},
		{map[string]string{"Owned": "1", "Finished": "1"}, "completed"},
		{map[string]string{"Mastered": "1", "Finished": "1"}, "mastered"},
		{map[string]string{"Dominated": "1", "Mastered": "1"}, "dominated"},
		{map[string]string{"Shelved": "1", "Playing": "1"}, "dropped"},
		{map[string]string{"Finished": "1", "Shelved": "1"}, "completed"},
	}
	for i, c := range cases {
		row := dkRow(dkHeader, "G", c.on)
		g := parseDK(t, dkCSV(dkHeader, row))[0]
		if g.PlayStatus != c.want {
			t.Errorf("case %d: play_status = %q, want %q", i, g.PlayStatus, c.want)
		}
	}
}

func TestDarkadia_RatingTruncatedLovedCreatedAtNotes(t *testing.T) {
	row := dkRow(dkHeader, "G", map[string]string{
		"Owned": "1", "Loved": "1", "Rating": "4.5", "Added": "2013-06-05", "Notes": "my note",
	})
	g := parseDK(t, dkCSV(dkHeader, row))[0]
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

func TestDarkadia_EmptyAndZeroRatingUnrated(t *testing.T) {
	for _, rating := range []string{"", "0"} {
		row := dkRow(dkHeader, "G", map[string]string{"Owned": "1", "Rating": rating})
		g := parseDK(t, dkCSV(dkHeader, row))[0]
		if g.PersonalRating != nil {
			t.Errorf("rating %q -> %v, want nil", rating, g.PersonalRating)
		}
	}
}

func TestDarkadia_PCWithGOGCopy_MacNoCopy(t *testing.T) {
	row := dkRow(dkHeader, "Anodyne", map[string]string{
		"Owned": "1", "Platforms": "PC, Mac",
		"Copy platform": "PC", "Copy media": "Digital", "Copy source": "GOG",
		"Copy purchase date": "2014-03-01",
	})
	g := parseDK(t, dkCSV(dkHeader, row))[0]
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

func TestDarkadia_PS3andPS4_viaPSN(t *testing.T) {
	named := dkRow(dkHeader, "Aaru's Awakening", map[string]string{
		"Owned": "1", "Platforms": "PlayStation Network (PS3), PlayStation 4",
		"Copy platform": "PlayStation Network (PS3)", "Copy media": "Digital",
		"Copy source": "Sony Entertainment Network", "Copy purchase date": "2015-02-02",
	})
	cont := dkRow(dkHeader, "", map[string]string{
		"Copy platform": "PlayStation 4", "Copy media": "Digital",
		"Copy source": "Sony Entertainment Network",
	})
	g := parseDK(t, dkCSV(dkHeader, named, cont))[0]
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

func TestDarkadia_StorefrontRules(t *testing.T) {
	// Physical media -> "physical" storefront + retailer provenance note.
	phys := dkRow(dkHeader, "Phys", map[string]string{
		"Owned": "1", "Platforms": "PlayStation 4", "Copy platform": "PlayStation 4",
		"Copy media": "Physical", "Copy source": "GameStop",
	})
	g := parseDK(t, dkCSV(dkHeader, phys))[0]
	if g.Platforms[0].Storefront == nil || *g.Platforms[0].Storefront != "physical" {
		t.Errorf("physical storefront = %v", g.Platforms[0].Storefront)
	}
	if g.PersonalNotes == nil || !strings.Contains(*g.PersonalNotes, "GameStop") {
		t.Errorf("physical retailer not in notes: %v", g.PersonalNotes)
	}

	// Unrecognized digital source -> nil storefront + provenance note.
	unrec := dkRow(dkHeader, "Unrec", map[string]string{
		"Owned": "1", "Platforms": "PC", "Copy platform": "PC", "Copy media": "Digital",
		"Copy source": "Fanatical",
	})
	g = parseDK(t, dkCSV(dkHeader, unrec))[0]
	if g.Platforms[0].Storefront != nil {
		t.Errorf("unrecognized digital storefront = %v, want nil", g.Platforms[0].Storefront)
	}
	if g.PersonalNotes == nil || !strings.Contains(*g.PersonalNotes, "Fanatical") {
		t.Errorf("unrecognized source not in notes: %v", g.PersonalNotes)
	}

	// Empty source, digital -> nil storefront, no note.
	empty := dkRow(dkHeader, "Empty", map[string]string{
		"Owned": "1", "Platforms": "PC", "Copy platform": "PC", "Copy media": "Digital",
	})
	g = parseDK(t, dkCSV(dkHeader, empty))[0]
	if g.Platforms[0].Storefront != nil || g.PersonalNotes != nil {
		t.Errorf("empty source: storefront=%v notes=%v, want nil/nil", g.Platforms[0].Storefront, g.PersonalNotes)
	}

	// "Other" sentinel -> Copy source other; spelling variant maps to epic.
	epic := dkRow(dkHeader, "Epic", map[string]string{
		"Owned": "1", "Platforms": "PC", "Copy platform": "PC", "Copy media": "Digital",
		"Copy source": "Other", "Copy source other": "Epic Game Store",
	})
	g = parseDK(t, dkCSV(dkHeader, epic))[0]
	if g.Platforms[0].Storefront == nil || *g.Platforms[0].Storefront != "epic-games-store" {
		t.Errorf("epic variant storefront = %v, want epic-games-store", g.Platforms[0].Storefront)
	}
}

func TestDarkadia_UnmappedPlatformGoesToNotes(t *testing.T) {
	row := dkRow(dkHeader, "Weird", map[string]string{"Owned": "1", "Platforms": "Sega Saturn"})
	g := parseDK(t, dkCSV(dkHeader, row))[0]
	if len(g.Platforms) != 0 {
		t.Errorf("platforms = %+v, want none (unmapped -> note)", g.Platforms)
	}
	if g.PersonalNotes == nil || !strings.Contains(*g.PersonalNotes, "Sega Saturn") {
		t.Errorf("unmapped platform not preserved in notes: %v", g.PersonalNotes)
	}
}

func TestDarkadia_NoPlatformGame(t *testing.T) {
	row := dkRow(dkHeader, "Bare", map[string]string{"Owned": "1"})
	g := parseDK(t, dkCSV(dkHeader, row))[0]
	if len(g.Platforms) != 0 {
		t.Errorf("platforms = %+v, want none", g.Platforms)
	}
}

func TestDarkadia_DedupOnPlatformStorefrontKeepsEarliest(t *testing.T) {
	named := dkRow(dkHeader, "Dup", map[string]string{
		"Owned": "1", "Platforms": "PC", "Copy platform": "PC", "Copy media": "Digital",
		"Copy source": "Steam", "Copy purchase date": "2013-01-01",
	})
	cont := dkRow(dkHeader, "", map[string]string{
		"Copy platform": "PC", "Copy media": "Digital", "Copy source": "Steam",
		"Copy purchase date": "2014-01-01",
	})
	g := parseDK(t, dkCSV(dkHeader, named, cont))[0]
	if len(g.Platforms) != 1 {
		t.Fatalf("platforms = %+v, want 1 deduped", g.Platforms)
	}
	if g.Platforms[0].AcquiredDate != "2013-01-01" {
		t.Errorf("kept date = %q, want earliest 2013-01-01", g.Platforms[0].AcquiredDate)
	}
}

func TestDarkadia_RecognizedSourceWithTrailingFreeText(t *testing.T) {
	row := dkRow(dkHeader, "Uplay", map[string]string{
		"Owned": "1", "Platforms": "PC", "Copy platform": "PC", "Copy media": "Digital",
		"Copy source": "Uplay (coupon w/ GTX 970)",
	})
	g := parseDK(t, dkCSV(dkHeader, row))[0]
	if g.Platforms[0].Storefront == nil || *g.Platforms[0].Storefront != "uplay" {
		t.Errorf("storefront = %v, want uplay", g.Platforms[0].Storefront)
	}
}

func TestDarkadia_NoCopyAggregateUsesInferredStorefront(t *testing.T) {
	row := dkRow(dkHeader, "NoCopyPSP", map[string]string{
		"Owned": "1", "Platforms": "PlayStation Network (PSP)", // no Copy platform
	})
	g := parseDK(t, dkCSV(dkHeader, row))[0]
	if len(g.Platforms) != 1 {
		t.Fatalf("platforms = %+v, want 1", g.Platforms)
	}
	if g.Platforms[0].Platform != "playstation-psp" {
		t.Errorf("platform = %q, want playstation-psp", g.Platforms[0].Platform)
	}
	if g.Platforms[0].Storefront == nil || *g.Platforms[0].Storefront != "playstation-store" {
		t.Errorf("storefront = %v, want playstation-store (inferred)", g.Platforms[0].Storefront)
	}
}

func TestDarkadia_DuplicateProvenanceNoteDeduped(t *testing.T) {
	named := dkRow(dkHeader, "DupNote", map[string]string{
		"Owned": "1", "Platforms": "PlayStation 4", "Copy platform": "PlayStation 4",
		"Copy media": "Physical", "Copy source": "GameStop",
	})
	cont := dkRow(dkHeader, "", map[string]string{
		"Copy platform": "PlayStation 4", "Copy media": "Physical", "Copy source": "GameStop",
	})
	g := parseDK(t, dkCSV(dkHeader, named, cont))[0]
	if g.PersonalNotes == nil {
		t.Fatalf("expected a provenance note")
	}
	if strings.Count(*g.PersonalNotes, "GameStop") != 1 {
		t.Errorf("GameStop mentioned %d times, want 1 (deduped): %q",
			strings.Count(*g.PersonalNotes, "GameStop"), *g.PersonalNotes)
	}
}

func TestDarkadia_TagsPlaytimeReviewCopyNotes(t *testing.T) {
	row := dkRow(dkExtHeader, "G", map[string]string{
		"Owned": "1", "Tags": "Co-op, VR", "Time played": "10:30",
		"Review subject": "Loved it", "Review": "Best game ever", "Notes": "my note",
		"Copy platform": "PC", "Copy notes": "PS Plus",
	})
	g := parseDK(t, dkCSV(dkExtHeader, row))[0]
	if len(g.Tags) != 2 || g.Tags[0] != "Co-op" || g.Tags[1] != "VR" {
		t.Fatalf("tags = %v, want [Co-op VR]", g.Tags)
	}
	if g.HoursPlayed == nil || *g.HoursPlayed != 10.5 {
		t.Errorf("hours = %v, want 10.5", g.HoursPlayed)
	}
	if g.PersonalNotes == nil {
		t.Fatal("notes nil")
	}
	for _, want := range []string{"my note", "Loved it", "Best game ever", "PS Plus"} {
		if !strings.Contains(*g.PersonalNotes, want) {
			t.Errorf("notes missing %q in: %s", want, *g.PersonalNotes)
		}
	}
}
