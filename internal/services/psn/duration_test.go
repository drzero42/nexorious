package psn

import (
	"math"
	"testing"
)

func TestParseDurationFractionalHours(t *testing.T) {
	cases := []struct {
		input string
		want  float64
	}{
		{"PT0S", 0},
		{"PT0H", 0},
		{"PT1H", 1},
		{"PT30M", 0.5},
		{"PT2H30M", 2.5},
		{"PT1H59M", 1 + 59.0/60.0},
		{"PT340H46M13S", 340 + 46.0/60.0}, // seconds dropped — minutes-only resolution
		{"PT2H0M0S", 2},
		{"PT99H59M59S", 99 + 59.0/60.0},
		{"", 0},
		{"invalid", 0},
		{"P1DT2H", 0}, // days component not supported
	}
	for _, tc := range cases {
		got := parseDurationFractionalHours(tc.input)
		if math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("parseDurationFractionalHours(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}
