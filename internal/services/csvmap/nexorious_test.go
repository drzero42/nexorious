package csvmap

import (
	"math"
	"testing"
)

// nexoriousFixture: a Nexorious CSV export header + three rows.
// Portal: two platforms, completed, loved, rated, two tags, hours.
// RDR2: single platform, shelved, not loved, no rating, one tag.
// Blank: empty play_status (-> not_started), no platforms, no tags, no hours.
const nexoriousFixture = `title,igdb_id,play_status,personal_rating,is_loved,hours_played,personal_notes,platforms,tags,created_at,updated_at
Portal,71,completed,5,true,10.5,Loved it,pc-windows;playstation-5,puzzle;favorite,2017-07-18T13:48:26Z,2020-02-02T00:00:00Z
Red Dead Redemption 2,25076,shelved,,false,,,playstation-4,western,2026-06-15T14:38:06Z,2026-06-15T14:38:06Z
Untitled Game,314246,,,false,,,,,2026-06-15T21:04:27Z,2026-06-15T21:04:27Z
`

func TestNexoriousCSV_MapsRealFixture(t *testing.T) {
	games, err := Parse([]byte(nexoriousFixture), NexoriousCSV())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(games) != 3 {
		t.Fatalf("want 3 games, got %d", len(games))
	}

	// Portal: id match, completed, rating 5, loved, 10.5h, two platforms, two tags.
	p := games[0]
	if p.Title != "Portal" || p.IGDBID == nil || *p.IGDBID != 71 {
		t.Fatalf("portal title/igdb = %q/%v", p.Title, p.IGDBID)
	}
	if p.PlayStatus != "completed" {
		t.Errorf("portal status = %q, want completed", p.PlayStatus)
	}
	if p.PersonalRating == nil || *p.PersonalRating != 5 {
		t.Errorf("portal rating = %v, want 5", p.PersonalRating)
	}
	if !p.IsLoved {
		t.Errorf("portal should be loved")
	}
	if p.HoursPlayed == nil || math.Abs(*p.HoursPlayed-10.5) > 1e-9 {
		t.Errorf("portal hours = %v, want 10.5", p.HoursPlayed)
	}
	if len(p.Platforms) != 2 || p.Platforms[0].Platform != "pc-windows" || p.Platforms[1].Platform != "playstation-5" {
		t.Errorf("portal platforms = %+v, want [pc-windows playstation-5]", p.Platforms)
	}
	if len(p.Tags) != 2 || p.Tags[0] != "puzzle" || p.Tags[1] != "favorite" {
		t.Errorf("portal tags = %v, want [puzzle favorite]", p.Tags)
	}
	if p.PersonalNotes == nil || *p.PersonalNotes != "Loved it" {
		t.Errorf("portal notes = %v, want \"Loved it\"", p.PersonalNotes)
	}
	if p.CreatedAt != "2017-07-18" {
		t.Errorf("portal created = %q, want 2017-07-18", p.CreatedAt)
	}
	if p.IsWishlisted {
		t.Errorf("portal should not be wishlisted")
	}

	// RDR2: shelved (non-default canonical value round-trips), one platform, no rating/hours.
	r := games[1]
	if r.PlayStatus != "shelved" {
		t.Errorf("rdr2 status = %q, want shelved", r.PlayStatus)
	}
	if r.PersonalRating != nil {
		t.Errorf("rdr2 rating = %v, want nil", r.PersonalRating)
	}
	if r.HoursPlayed != nil {
		t.Errorf("rdr2 hours = %v, want nil", r.HoursPlayed)
	}
	if len(r.Platforms) != 1 || r.Platforms[0].Platform != "playstation-4" {
		t.Errorf("rdr2 platforms = %+v, want [playstation-4]", r.Platforms)
	}
	if r.IsLoved {
		t.Errorf("rdr2 should not be loved")
	}

	// Blank play_status -> not_started; no platforms/tags.
	b := games[2]
	if b.PlayStatus != "not_started" {
		t.Errorf("blank status = %q, want not_started", b.PlayStatus)
	}
	if len(b.Platforms) != 0 {
		t.Errorf("blank platforms = %+v, want none", b.Platforms)
	}
	if len(b.Tags) != 0 {
		t.Errorf("blank tags = %v, want none", b.Tags)
	}
}

func TestNexoriousCSV_SignatureRejectsUnrelated(t *testing.T) {
	_, err := Parse([]byte("name,foo,bar\nX,1,2\n"), NexoriousCSV())
	if err == nil {
		t.Fatal("want signature rejection for a non-Nexorious header")
	}
}
