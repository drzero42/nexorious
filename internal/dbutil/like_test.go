package dbutil

import "testing"

func TestEscapeLike(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain text untouched", "bobfilt", "bobfilt"},
		{"percent escaped", "100%", `100\%`},
		{"underscore escaped", "a_b", `a\_b`},
		{"backslash escaped first", `a\b`, `a\\b`},
		{"all metachars", `\%_`, `\\\%\_`},
		{"empty string", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := EscapeLike(tc.in); got != tc.want {
				t.Errorf("EscapeLike(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestLikeContains(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain substring", "bob", "%bob%"},
		{"wildcard input cannot widen match", "%", `%\%%`},
		{"underscore input cannot widen match", "_", `%\_%`},
		{"empty matches anything", "", "%%"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := LikeContains(tc.in); got != tc.want {
				t.Errorf("LikeContains(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
