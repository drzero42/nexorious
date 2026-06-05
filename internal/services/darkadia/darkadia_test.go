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
