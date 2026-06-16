package csvmap

import (
	"math"
	"testing"
)

// grouveeFixture: real 20-column Grouvee header + three trimmed rows. Portal is a
// finished 100%-completion row (tier overrides Played -> dominated, with hours
// and a PC platform); RDR2 is a Backlog row whose Main Story tier overrides to
// completed; Borderlands 4 is a Wish List row (wishlisted, no platforms).
const grouveeFixture = `id,name,shelves,platforms,rating,review_title,review,review_platform,dates,statuses,genres,franchises,series,developers,publishers,release_date,date_added_to_collection,url,giantbomb_id,igdb_id
107168,Portal,"{""Played"": {""date_added"": ""2017-07-18T13:48:26Z""}}","{""PC (Microsoft Windows)"": {""url"": ""x""}}",5,Great,Loved it,,"[{""date_started"": ""2010-01-01"", ""date_finished"": ""2020-02-02"", ""seconds_played"": 36300, ""level_of_completion"": ""100% Completion""}]",[],{},{},{},{},{},2007-10-10,2017-07-18,x,,71
117835,Red Dead Redemption 2,"{""Backlog"": {""date_added"": ""2026-06-15T14:38:06Z"", ""order"": 1}}","{""PlayStation 4"": {""url"": ""x""}}",4,,,,"[{""date_started"": ""None"", ""date_finished"": ""None"", ""seconds_played"": 0, ""level_of_completion"": ""Main Story""}]",[],{},{},{},{},{},2018-10-26,2026-06-15,x,,25076
196974,Borderlands 4,"{""Wish List"": {""date_added"": ""2026-06-15T21:04:27Z"", ""order"": 1}}",{},,,,,[],[],{},{},{},{},{},2025-09-11,2026-06-15,x,,314246
`

func TestGrouvee_MapsRealFixture(t *testing.T) {
	games, err := Parse([]byte(grouveeFixture), Grouvee())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(games) != 3 {
		t.Fatalf("want 3 games, got %d", len(games))
	}

	// Portal: tier 100% Completion overrides Played -> dominated; rating 5;
	// hours 36300/3600; pc-windows; igdb 71; review note assembled.
	p := games[0]
	if p.Title != "Portal" || p.IGDBID == nil || *p.IGDBID != 71 {
		t.Fatalf("portal title/igdb = %q/%v", p.Title, p.IGDBID)
	}
	if p.PlayStatus != "dominated" {
		t.Errorf("portal status = %q, want dominated (tier override)", p.PlayStatus)
	}
	if p.PersonalRating == nil || *p.PersonalRating != 5 {
		t.Errorf("portal rating = %v, want 5", p.PersonalRating)
	}
	if p.HoursPlayed == nil || math.Abs(*p.HoursPlayed-36300.0/3600.0) > 1e-9 {
		t.Errorf("portal hours = %v, want %v", p.HoursPlayed, 36300.0/3600.0)
	}
	if len(p.Platforms) != 1 || p.Platforms[0].Platform != "pc-windows" {
		t.Errorf("portal platforms = %+v", p.Platforms)
	}
	if p.PersonalNotes == nil || *p.PersonalNotes != "**Great**\n\nLoved it" {
		t.Errorf("portal notes = %v", p.PersonalNotes)
	}
	if p.CreatedAt != "2017-07-18" {
		t.Errorf("portal created = %q", p.CreatedAt)
	}
	if p.IsWishlisted {
		t.Errorf("portal should not be wishlisted")
	}

	// RDR2: Backlog shelf, but Main Story tier overrides -> completed; ps4; no hours.
	r := games[1]
	if r.PlayStatus != "completed" {
		t.Errorf("rdr2 status = %q, want completed (tier override of Backlog)", r.PlayStatus)
	}
	if r.HoursPlayed != nil {
		t.Errorf("rdr2 hours = %v, want nil (0 seconds)", r.HoursPlayed)
	}
	if len(r.Platforms) != 1 || r.Platforms[0].Platform != "playstation-4" {
		t.Errorf("rdr2 platforms = %+v", r.Platforms)
	}

	// Borderlands 4: Wish List -> wishlisted, default status, no platforms, no rating.
	b := games[2]
	if !b.IsWishlisted {
		t.Errorf("borderlands should be wishlisted")
	}
	if b.PlayStatus != "not_started" {
		t.Errorf("borderlands status = %q, want not_started", b.PlayStatus)
	}
	if len(b.Platforms) != 0 {
		t.Errorf("borderlands platforms = %+v, want none", b.Platforms)
	}
	if b.PersonalRating != nil {
		t.Errorf("borderlands rating = %v, want nil", b.PersonalRating)
	}
}

func TestGrouvee_SignatureRejectsUnrelated(t *testing.T) {
	_, err := Parse([]byte("name,foo,bar\nX,1,2\n"), Grouvee())
	if err == nil {
		t.Fatal("want signature rejection for a non-Grouvee header")
	}
}
