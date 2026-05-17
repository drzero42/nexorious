package psn

import "testing"

func TestParseDurationHours(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{"PT0S", 0},
		{"PT340H46M13S", 340},
		{"PT1H", 1},
		{"PT30M", 0},
		{"PT2H0M0S", 2},
		{"PT99H59M59S", 99},
		{"", 0},
		{"invalid", 0},
		{"P1DT2H", 0}, // days component not supported — returns 0
	}
	for _, tc := range cases {
		got := parseDurationHours(tc.input)
		if got != tc.want {
			t.Errorf("parseDurationHours(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}
