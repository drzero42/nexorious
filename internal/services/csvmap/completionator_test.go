package csvmap

import "testing"

// completionatorFixture is a fully quote-wrapped 24-column Completionator export
// (the real malformed-quote shape) with three games: a rated/Incomplete PC
// title, the bare-quoted "Done Running" title, and a Finished PlayStation 5/GOG
// title. Column order matches a real export.
const completionatorFixture = `"Name","Edition","Platform","Format","Region","Now Playing","Backlogged","Ownership Status","Progress Status","Est. Value","Amt. Paid","Tags","Box/Case","Cart/Disc","Manual","Extras","Acquisition Type","Acquisition Source","Acquisition Date","Rating","Initial Release Date","Item Release Date","Added On","Genre"
"A Hat in Time","","PC / Windows","Digital (Steam)","EU","No","Yes","Owned","Incomplete","","","","","","","","Purchase","","","10","10/5/2017","","1/17/2022","Platformer"
"The Walking Dead: The Final Season - Episode 1: "Done Running"","","PC / Windows","Digital (Steam)","EU","No","Yes","Owned","Incomplete","","","","","","","","Purchase","","","","","","1/17/2022",""
"Batman: Arkham Asylum - Game of the Year Edition","","PlayStation 5","Digital (GOG)","EU","No","No","Owned","Finished","","","","","","","","Purchase","","","3","","","1/17/2022","Action"
`

func TestCompletionator_MapsRealFixture(t *testing.T) {
	games, err := Parse([]byte(completionatorFixture), Completionator())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(games) != 3 {
		t.Fatalf("want 3 games, got %d: %+v", len(games), games)
	}

	// Game 1: A Hat in Time — Incomplete -> not_started, rating 10/10 -> 5 stars,
	// pc-windows / steam, Added On -> CreatedAt.
	g := games[0]
	if g.Title != "A Hat in Time" {
		t.Fatalf("g0 title = %q", g.Title)
	}
	if g.PlayStatus != "not_started" {
		t.Errorf("g0 status = %q, want not_started", g.PlayStatus)
	}
	if g.PersonalRating == nil || *g.PersonalRating != 5 {
		t.Errorf("g0 rating = %v, want 5", g.PersonalRating)
	}
	if g.CreatedAt != "2022-01-17" {
		t.Errorf("g0 created = %q, want 2022-01-17", g.CreatedAt)
	}
	if len(g.Platforms) != 1 || g.Platforms[0].Platform != "pc-windows" {
		t.Fatalf("g0 platforms = %+v", g.Platforms)
	}
	if g.Platforms[0].Storefront == nil || *g.Platforms[0].Storefront != "steam" {
		t.Errorf("g0 storefront = %v, want steam", g.Platforms[0].Storefront)
	}
	if len(g.Tags) != 0 {
		t.Errorf("g0 tags = %v, want none (Genre is not mapped)", g.Tags)
	}

	// Game 2: bare-quoted title recovered exactly; no rating.
	g = games[1]
	if g.Title != `The Walking Dead: The Final Season - Episode 1: "Done Running"` {
		t.Fatalf("g1 title = %q", g.Title)
	}
	if g.PersonalRating != nil {
		t.Errorf("g1 rating = %v, want nil", g.PersonalRating)
	}

	// Game 3: Finished -> completed, rating 3/10 -> 2 stars, playstation-5 / gog.
	g = games[2]
	if g.Title != "Batman: Arkham Asylum - Game of the Year Edition" {
		t.Fatalf("g2 title = %q", g.Title)
	}
	if g.PlayStatus != "completed" {
		t.Errorf("g2 status = %q, want completed", g.PlayStatus)
	}
	if g.PersonalRating == nil || *g.PersonalRating != 2 {
		t.Errorf("g2 rating = %v, want 2", g.PersonalRating)
	}
	if len(g.Platforms) != 1 || g.Platforms[0].Platform != "playstation-5" {
		t.Fatalf("g2 platforms = %+v", g.Platforms)
	}
	if g.Platforms[0].Storefront == nil || *g.Platforms[0].Storefront != "gog" {
		t.Errorf("g2 storefront = %v, want gog", g.Platforms[0].Storefront)
	}
}

func TestCompletionator_Signature(t *testing.T) {
	cfg := Completionator()
	completionatorHeader := []string{
		"Name", "Edition", "Platform", "Format", "Region", "Now Playing",
		"Backlogged", "Ownership Status", "Progress Status", "Added On",
	}
	if !MatchesSignature(completionatorHeader, cfg) {
		t.Error("Completionator signature should match a real header")
	}
	if MatchesSignature([]string{"Title", "Console", "Status"}, cfg) {
		t.Error("Completionator signature should not match an unrelated header")
	}
}
