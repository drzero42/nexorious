package api_test

// A decision on one edition alone: Rematch, Skip and Unskip with
// only_this_edition=true (#1452).

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/drzero42/nexorious/internal/externalgame"
)

const onlyThis = "?only_this_edition=true"

func (f *rmFixture) rematchOne(id string, igdbID int, orphan string) *httptest.ResponseRecorder {
	f.t.Helper()
	body := map[string]any{"igdb_id": igdbID}
	if orphan != "" {
		body["orphan_action"] = orphan
	}
	return postJSONAuth(f.t, f.e, "/api/sync/external-games/"+id+"/rematch"+onlyThis, body, f.token)
}

func (f *rmFixture) skipOne(id string) *httptest.ResponseRecorder {
	f.t.Helper()
	return postJSONAuth(f.t, f.e, "/api/sync/ignored/"+id+onlyThis, nil, f.token)
}

func (f *rmFixture) unskipOne(id string) *httptest.ResponseRecorder {
	f.t.Helper()
	return deleteAuth(f.t, f.e, "/api/sync/ignored/"+id+onlyThis, f.token)
}

// match is egID's resolved IGDB id, 0 when unmatched.
func (f *rmFixture) match(egID string) int {
	f.t.Helper()
	return f.count(`SELECT COALESCE(resolved_igdb_id, 0) FROM external_games WHERE id = ?`, egID)
}

func (f *rmFixture) skipped(egID string) bool {
	f.t.Helper()
	return f.count(`SELECT COUNT(*) FROM external_games WHERE id = ? AND is_skipped`, egID) == 1
}

func (f *rmFixture) userGames() int {
	f.t.Helper()
	return f.count(`SELECT COUNT(*) FROM user_games WHERE user_id = ?`, f.userID)
}

// holder is the External game that holds user game ugID's link on platform.
func (f *rmFixture) holder(ugID, platform string) string {
	f.t.Helper()
	return f.str(`SELECT COALESCE(external_game_id, '') FROM user_game_platforms WHERE user_game_id = ? AND platform = ?`, ugID, platform)
}

func wantOrphanConflict(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	wantCode(t, rec, http.StatusConflict)
	// The web UI and nexctl tell this 409 from the route's others by its
	// message's start.
	var body struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !strings.HasPrefix(body.Message, "orphan_action required") {
		t.Fatalf("409 message: want the orphan_action required prefix, got %q", body.Message)
	}
}

// ── Rematch ───────────────────────────────────────────────────────────────────

// Each edition on its own platform, both on one library game.
func (f *rmFixture) twoPlatformSet() {
	f.game("eg", 100, false, "playstation-4")
	f.game("ed", 100, false, "playstation-5")
	f.link("ug", 100, "eg", "playstation-4")
	f.link("ug", 100, "ed", "playstation-5")
}

// Rematching the non-representative edition alone gives two entries and two
// library games, with no orphan question while the other edition holds the
// old library game; the representative is not changed.
func TestRematchOne_TwoEntriesTwoLibraryGames(t *testing.T) {
	f := newRM(t, "one-rematch")
	f.twoPlatformSet()

	wantCode(t, f.rematchOne("ed", 200, ""), http.StatusNoContent)

	if m := f.match("eg"); m != 100 {
		t.Errorf("representative's match: want 100, got %d", m)
	}
	if m := f.match("ed"); m != 200 {
		t.Errorf("edition's match: want 200, got %d", m)
	}
	if l := f.list(); len(l) != 2 {
		t.Errorf("entries: want 2, got %+v", l)
	}
	if n := f.userGames(); n != 2 {
		t.Errorf("user games: want 2, got %d", n)
	}
	if f.onGame("eg", 100) != 1 || f.onGame("ed", 200) != 1 {
		t.Errorf("links: eg on 100 %d, ed on 200 %d; want 1 each", f.onGame("eg", 100), f.onGame("ed", 200))
	}

	// It holds: a later Rematch of the one does not move the other, and the
	// old library game, now linked to eg alone, is an orphan.
	wantOrphanConflict(t, f.rematch("eg", 300, ""))
	wantCode(t, f.rematch("eg", 300, "keep"), http.StatusNoContent)
	if m := f.match("ed"); m != 200 {
		t.Errorf("the other edition moved: match %d", m)
	}
}

// With the staying edition on another platform, it has its own link on the
// library game: no orphan question, whichever edition leaves.
func TestRematchOne_StayingEditionOnAnotherPlatform_NoOrphanQuestion(t *testing.T) {
	for _, leaving := range []string{"eg", "ed"} {
		t.Run(leaving, func(t *testing.T) {
			f := newRM(t, "one-other-platform")
			f.twoPlatformSet()
			wantCode(t, f.rematchOne(leaving, 200, ""), http.StatusNoContent)
			if n := f.count(`SELECT COUNT(*) FROM user_games WHERE id = 'ug'`); n != 1 {
				t.Error("the staying library game is gone")
			}
		})
	}
}

// Rematching one edition back onto the other's game gives one entry again,
// and the next Rematch through either id moves both. The library game it was
// on alone is then an orphan.
func TestRematchOne_BackOntoTheOthersGame_JoinsTheSetAgain(t *testing.T) {
	for _, tc := range []struct {
		action    string
		userGames int
	}{{"remove", 1}, {"keep", 2}} {
		t.Run(tc.action, func(t *testing.T) {
			f := newRM(t, "one-rejoin")
			f.twoPlatformSet()
			wantCode(t, f.rematchOne("ed", 200, ""), http.StatusNoContent)

			wantOrphanConflict(t, f.rematchOne("ed", 100, ""))
			wantCode(t, f.rematchOne("ed", 100, tc.action), http.StatusNoContent)
			if l := f.list(); len(l) != 1 {
				t.Errorf("entries: want 1, got %+v", l)
			}
			if n := f.userGames(); n != tc.userGames {
				t.Errorf("user games: want %d, got %d", tc.userGames, n)
			}

			wantCode(t, f.rematch("ed", 300, "keep"), http.StatusNoContent)
			if f.match("eg") != 300 || f.match("ed") != 300 {
				t.Errorf("matches after a set Rematch: eg %d, ed %d; want 300", f.match("eg"), f.match("ed"))
			}
		})
	}
}

// Both editions in review: rematching one alone completes only its item; the
// other stays its own entry, the badge counts it, and the Job waits for it.
func TestRematchOne_BothInReview_CompletesOnlyItsItem(t *testing.T) {
	f := newRM(t, "one-review")
	f.game("eg", 0, false, "playstation-4")
	f.game("ed", 0, false, "playstation-5")
	f.job("job", "processing")
	f.item("ji", "job", "eg", "pending_review", 0)
	f.item("ji-ed", "job", "ed", "pending_review", 0)

	wantCode(t, f.rematchOne("eg", 200, ""), http.StatusNoContent)

	if s := f.itemStatus("ji"); s != "completed" {
		t.Errorf("own item: want completed, got %s", s)
	}
	if s := f.itemStatus("ji-ed"); s != "pending_review" {
		t.Errorf("other edition's item: want pending_review, got %s", s)
	}
	if s := f.status("ed"); s != "needs_review" {
		t.Errorf("other edition's entry: want needs_review, got %s", s)
	}
	if n := f.badge(); n != 1 {
		t.Errorf("badge: want 1, got %d", n)
	}
	if s := f.str(`SELECT status FROM jobs WHERE id = 'job'`); s != "processing" {
		t.Errorf("job: want processing, got %s", s)
	}
}

// Two editions on one platform share one link row, held by eg.
func (f *rmFixture) sharedLinkSet(skipped bool, stayingPlatform string) {
	f.game("eg", 100, skipped, "playstation-4")
	f.game("ed", 100, skipped, stayingPlatform)
	f.link("ug", 100, "eg", "playstation-4")
}

// The leaving edition held the shared link: the staying edition takes it over,
// with its dates and hours, and its library game survives with no orphan
// question.
func TestRematchOne_LeavingHeldTheSharedLink_StayingTakesItOver(t *testing.T) {
	f := newRM(t, "one-shared")
	f.sharedLinkSet(false, "playstation-4")
	f.exec(`UPDATE user_game_platforms SET created_at = '2020-01-02', acquired_date = '2020-01-03', hours_played = 42 WHERE id = 'ug-playstation-4'`)

	wantCode(t, f.rematchOne("eg", 200, ""), http.StatusNoContent)

	if h := f.holder("ug", "playstation-4"); h != "ed" {
		t.Errorf("shared link holder: want ed, got %q", h)
	}
	row := f.str(`SELECT created_at::date || ' ' || acquired_date || ' ' || hours_played::int FROM user_game_platforms WHERE id = 'ug-playstation-4'`)
	if row != "2020-01-02 2020-01-03 42" {
		t.Errorf("shared link row: want its dates and hours kept, got %q", row)
	}
	if f.onGame("eg", 200) != 1 {
		t.Error("the leaving edition is not on its new match")
	}
	if g := f.list()["ed"]; !g.HasUserGame {
		t.Error("the staying entry shows no library game")
	}
}

// The leaving edition held the link and no staying edition can take it: the
// orphan question is asked.
func TestRematchOne_NoStayingEditionCanTakeTheLink_AsksTheOrphanQuestion(t *testing.T) {
	cases := []struct {
		name            string
		skipped         bool
		stayingPlatform string
	}{
		{"set skipped", true, "playstation-4"},
		{"staying edition without platforms", false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newRM(t, "one-no-taker")
			f.sharedLinkSet(tc.skipped, tc.stayingPlatform)
			wantOrphanConflict(t, f.rematchOne("eg", 200, ""))
			if h := f.holder("ug", "playstation-4"); h != "eg" {
				t.Errorf("refused rematch changed the link: holder %q", h)
			}
		})
	}
}

// On an entry of one edition the option changes nothing.
func TestRematchOne_SingleEdition_SameAsTheSet(t *testing.T) {
	f := newRM(t, "one-single")
	f.game("eg", 100, false, "playstation-4")
	f.link("ug", 100, "eg", "playstation-4")

	wantOrphanConflict(t, f.rematchOne("eg", 200, ""))
	wantCode(t, f.rematchOne("eg", 200, "remove"), http.StatusNoContent)
	if f.match("eg") != 200 || f.userGames() != 1 {
		t.Errorf("match %d, user games %d; want 200 and 1", f.match("eg"), f.userGames())
	}
}

// ── Skip ──────────────────────────────────────────────────────────────────────

// Skipping one edition alone: it is its own entry in Skipped, the other
// edition is unchanged, only its own open item is skipped, and its library
// link stays.
func TestSkipOne_OwnEntryOwnItemLinkKept(t *testing.T) {
	f := newRM(t, "one-skip")
	f.twoPlatformSet()
	f.job("job", "processing")
	f.item("ji", "job", "eg", "pending_review", 0)
	f.item("ji-ed", "job", "ed", "pending_review", 0)

	wantCode(t, f.skipOne("ed"), http.StatusNoContent)

	if !f.skipped("ed") || f.skipped("eg") {
		t.Errorf("skips: eg %v, ed %v; want ed alone", f.skipped("eg"), f.skipped("ed"))
	}
	if s := f.status("ed"); s != "skipped" {
		t.Errorf("skipped edition's entry: want skipped, got %s", s)
	}
	if s := f.itemStatus("ji-ed"); s != "skipped" {
		t.Errorf("own item: want skipped, got %s", s)
	}
	if s := f.itemStatus("ji"); s != "pending_review" {
		t.Errorf("other edition's item: want pending_review, got %s", s)
	}
	if n := f.count(`SELECT COUNT(*) FROM changes WHERE job_id = 'job' AND external_game_id = 'ed' AND change_type = 'skipped'`); n != 1 {
		t.Errorf("skipped change: want 1, got %d", n)
	}
	if f.onGame("ed", 100) != 1 {
		t.Error("the skipped edition lost its library link")
	}
}

// A Skip with the option of a game with open items in two Jobs skips both.
func TestSkipOne_OpenItemsInTwoJobs_SkipsBoth(t *testing.T) {
	f := newRM(t, "one-skip-two-jobs")
	f.game("eg", 0, false, "playstation-4")
	f.game("ed", 0, false, "playstation-5")
	f.job("job-a", "processing")
	f.job("job-b", "processing")
	f.item("ji-a", "job-a", "ed", "pending_review", 0)
	f.item("ji-b", "job-b", "ed", "pending_review", 0)
	f.item("ji-eg", "job-a", "eg", "pending_review", 0)

	wantCode(t, f.skipOne("ed"), http.StatusNoContent)

	for _, id := range []string{"ji-a", "ji-b"} {
		if s := f.itemStatus(id); s != "skipped" {
			t.Errorf("%s: want skipped, got %s", id, s)
		}
	}
	if s := f.itemStatus("ji-eg"); s != "pending_review" {
		t.Errorf("other edition's item: want pending_review, got %s", s)
	}
}

// The skipped edition held the shared link: the staying edition holds it
// afterwards, its entry still shows the library game, and a Rematch of it asks
// the orphan question. Unskipping the edition alone makes it one entry again.
func TestSkipOne_HeldTheSharedLink_StayingTakesItOver(t *testing.T) {
	f := newRM(t, "one-skip-shared")
	f.sharedLinkSet(false, "playstation-4")

	wantCode(t, f.skipOne("eg"), http.StatusNoContent)

	if h := f.holder("ug", "playstation-4"); h != "ed" {
		t.Errorf("shared link holder: want ed, got %q", h)
	}
	if g := f.list()["ed"]; !g.HasUserGame {
		t.Error("the staying entry shows no library game")
	}
	wantOrphanConflict(t, f.rematch("ed", 200, ""))

	wantCode(t, f.unskipOne("eg"), http.StatusNoContent)
	if l := f.list(); len(l) != 1 {
		t.Errorf("entries after the unskip: want 1, got %+v", l)
	}
	if n := f.count(`SELECT COUNT(*) FROM external_games WHERE id = 'eg' AND library_written_at IS NOT NULL`); n != 1 {
		t.Error("the unskipped edition was not written to the library")
	}
}

// A Skip with the option on an edition of a skipped, matched set that holds
// the link: no rewrite, no error.
func TestSkipOne_SkippedSet_NoRewrite(t *testing.T) {
	f := newRM(t, "one-skip-skipped")
	f.sharedLinkSet(true, "playstation-4")

	wantCode(t, f.skipOne("eg"), http.StatusNoContent)

	if h := f.holder("ug", "playstation-4"); h != "eg" {
		t.Errorf("link holder: want eg, got %q", h)
	}
}

// ── Unskip ────────────────────────────────────────────────────────────────────

// Unskipping one edition alone of a skipped set: only it is unskipped; a
// matched one is written to the library.
func TestUnskipOne_SkippedSet(t *testing.T) {
	for _, matched := range []bool{true, false} {
		name := "unmatched"
		resolved := 0
		if matched {
			name, resolved = "matched", 100
		}
		t.Run(name, func(t *testing.T) {
			f := newRM(t, "one-unskip")
			f.game("eg", resolved, true, "playstation-4")
			f.game("ed", resolved, true, "playstation-5")

			wantCode(t, f.unskipOne("ed"), http.StatusNoContent)

			if f.skipped("ed") || !f.skipped("eg") {
				t.Errorf("skips: eg %v, ed %v; want eg alone", f.skipped("eg"), f.skipped("ed"))
			}
			want := 0
			if matched {
				want = 1
			}
			if n := f.onGame("ed", resolved); n != want {
				t.Errorf("unskipped edition's links: want %d, got %d", want, n)
			}
			if n := f.onGame("eg", resolved); n != 0 {
				t.Errorf("skipped edition was written: %d links", n)
			}
		})
	}
}

// An unmatched edition unskipped alone has no decision, so the next sync gives
// it the skip of the same-title game back.
func TestUnskipOne_Unmatched_TakesTheSkipBackAtTheNextSync(t *testing.T) {
	f := newRM(t, "one-unskip-precedent")
	f.game("eg", 0, true, "playstation-4")
	f.game("ed", 0, true, "playstation-5")
	wantCode(t, f.unskipOne("ed"), http.StatusNoContent)

	took, err := externalgame.TakePrecedent(context.Background(), testDB, "ed")
	if err != nil || !took {
		t.Fatalf("take precedent: took %v, %v", took, err)
	}
	if !f.skipped("ed") {
		t.Error("the edition did not take the skip back")
	}
}

// ── The list ──────────────────────────────────────────────────────────────────

type listedEdition struct {
	ID          string   `json:"id"`
	ExternalID  string   `json:"external_id"`
	Platforms   []string `json:"platforms"`
	StoreURL    *string  `json:"store_url"`
	IsAvailable bool     `json:"is_available"`
}

func (f *rmFixture) editions() map[string][]listedEdition {
	f.t.Helper()
	rec := getAuth(f.t, f.e, "/api/sync/"+rmStore+"/external-games", f.token)
	wantCode(f.t, rec, http.StatusOK)
	var games []struct {
		ID       string          `json:"id"`
		Editions []listedEdition `json:"editions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &games); err != nil {
		f.t.Fatalf("unmarshal list: %v", err)
	}
	out := map[string][]listedEdition{}
	for _, g := range games {
		out[g.ID] = g.Editions
	}
	return out
}

func TestListExternalGames_Editions(t *testing.T) {
	f := newRM(t, "list-editions")
	f.game("eg", 100, false, "playstation-4")
	f.game("ed", 100, false, "playstation-5")
	f.exec(`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at) VALUES ('ed-p2', 'ed', 'playstation-4', 0, now())`)
	f.exec(`UPDATE external_games SET store_link = 'concept-ed' WHERE id = 'ed'`)
	f.titled("solo", "Other", 0, false, "")

	got := f.editions()
	set := got["eg"]
	if len(set) != 2 || set[0].ID != "ed" || set[1].ID != "eg" {
		t.Fatalf("editions of the set: want ed, eg in id order, got %+v", set)
	}
	if strings.Join(set[0].Platforms, ",") != "playstation-4,playstation-5" || strings.Join(set[1].Platforms, ",") != "playstation-4" {
		t.Errorf("per-edition platforms: %+v", set)
	}
	if set[0].ExternalID != "ed" || !set[0].IsAvailable {
		t.Errorf("edition fields: %+v", set[0])
	}
	if set[0].StoreURL == nil || *set[0].StoreURL != "https://store.playstation.com/en-us/concept/concept-ed" || set[1].StoreURL != nil {
		t.Errorf("store urls: ed %v, eg %v", set[0].StoreURL, set[1].StoreURL)
	}
	if solo := got["solo"]; len(solo) != 1 || solo[0].ID != "solo" || len(solo[0].Platforms) != 0 {
		t.Errorf("single edition: %+v", solo)
	}
}

// ── Concurrency ───────────────────────────────────────────────────────────────

// A Skip of one edition racing a Skip, an Unskip or a Rematch of the set does
// not deadlock.
func TestSkipOne_RacingASetDecision_NoDeadlock(t *testing.T) {
	ops := map[string]func(f *rmFixture) *httptest.ResponseRecorder{
		"skip":    func(f *rmFixture) *httptest.ResponseRecorder { return f.skip("eg") },
		"unskip":  func(f *rmFixture) *httptest.ResponseRecorder { return f.unskip("eg") },
		"rematch": func(f *rmFixture) *httptest.ResponseRecorder { return f.rematch("eg", 200, "keep") },
	}
	for name, op := range ops {
		t.Run(name, func(t *testing.T) {
			for range 5 {
				f := newRM(t, "one-skip-race")
				f.twoPlatformSet()
				f.igdbGame(200)
				var wg sync.WaitGroup
				recs := make([]*httptest.ResponseRecorder, 2)
				wg.Add(2)
				go func() { defer wg.Done(); recs[0] = f.skipOne("ed") }()
				go func() { defer wg.Done(); recs[1] = op(f) }()
				wg.Wait()
				for _, rec := range recs {
					if rec.Code != http.StatusNoContent && rec.Code != http.StatusConflict {
						t.Fatalf("%d %s", rec.Code, rec.Body.String())
					}
				}
			}
		})
	}
}

// A Rematch of one edition racing a Rematch of the set through another id
// does not deadlock and ends as either order would: the two orders end the
// same here.
func TestRematchOne_RacingASetRematch_EndsAsOneOrder(t *testing.T) {
	for range 10 {
		f := newRM(t, "one-rematch-race")
		f.twoPlatformSet()
		f.igdbGame(200)
		f.igdbGame(300)
		var wg sync.WaitGroup
		recs := make([]*httptest.ResponseRecorder, 2)
		wg.Add(2)
		go func() { defer wg.Done(); recs[0] = f.rematchOne("ed", 200, "keep") }()
		go func() { defer wg.Done(); recs[1] = f.rematch("eg", 300, "keep") }()
		wg.Wait()
		both := true
		for _, rec := range recs {
			switch rec.Code {
			case http.StatusNoContent:
			case http.StatusConflict:
				both = false // the lock gave up: only the other ran
			default:
				t.Fatalf("%d %s", rec.Code, rec.Body.String())
			}
		}
		if both && (f.match("eg") != 300 || f.match("ed") != 200) {
			t.Fatalf("end state: eg %d, ed %d; want 300 and 200", f.match("eg"), f.match("ed"))
		}
	}
}
