package logging

import "testing"

func TestRedact(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"empty", "", ""},
		{"short", "abcd", "****"},
		{"long", "supersecretvalue", "supe…[redacted]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Redact(tc.in); got != tc.want {
				t.Errorf("Redact(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestScrubURLQueries(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"empty", "", ""},
		{"no_url", "connection refused", "connection refused"},
		{"url_without_query", `Get "https://api.steampowered.com/path": EOF`, `Get "https://api.steampowered.com/path": EOF`},
		{
			"steam_url_error",
			`sync failed: get owned games: Get "https://api.steampowered.com/IPlayerService/GetOwnedGames/v0001/?key=SECRET&steamid=123": connection refused`,
			`sync failed: get owned games: Get "https://api.steampowered.com/IPlayerService/GetOwnedGames/v0001/": connection refused`,
		},
		{
			"gog_token_url_error",
			`gog: token request: Get "https://auth.gog.com/token?client_secret=abc&refresh_token=xyz": EOF`,
			`gog: token request: Get "https://auth.gog.com/token": EOF`,
		},
		{"bare_url_with_query", "https://example.com/a?b=c", "https://example.com/a"},
		{"plain_http", `Get "http://example.com/a?b=c": timeout`, `Get "http://example.com/a": timeout`},
		{
			"multiple_urls",
			"first https://a.example/p?k=1 then https://b.example/q?k=2 done",
			"first https://a.example/p then https://b.example/q done",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ScrubURLQueries(tc.in); got != tc.want {
				t.Errorf("ScrubURLQueries(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
