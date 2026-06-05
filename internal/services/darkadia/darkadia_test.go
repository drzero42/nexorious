package darkadia

import (
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
