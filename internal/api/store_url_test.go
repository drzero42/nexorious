package api

import "testing"

func TestBuildStoreURL(t *testing.T) {
	cases := []struct {
		name       string
		storefront string
		link       string
		wantURL    string
		wantOK     bool
	}{
		{"steam", "steam", "440", "https://store.steampowered.com/app/440/", true},
		{"gog", "gog", "the-witcher-3-wild-hunt", "https://www.gog.com/game/the-witcher-3-wild-hunt", true},
		{"epic", "epic-games-store", "fortnite", "https://store.epicgames.com/en-US/p/fortnite", true},
		{"psn", "playstation-store", "10002694", "https://store.playstation.com/en-us/concept/10002694", true},
		{"humble null", "humble-bundle", "", "", false},
		{"empty link", "steam", "", "", false},
		{"unknown storefront", "itch", "abc", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotURL, gotOK := buildStoreURL(c.storefront, c.link)
			if gotURL != c.wantURL || gotOK != c.wantOK {
				t.Fatalf("buildStoreURL(%q,%q) = (%q,%v), want (%q,%v)",
					c.storefront, c.link, gotURL, gotOK, c.wantURL, c.wantOK)
			}
		})
	}
}
