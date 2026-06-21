package updatecheck

import "testing"

func TestUpdateAvailable(t *testing.T) {
	cases := []struct {
		name            string
		running, latest string
		want            bool
	}{
		{"newer patch", "0.9.0", "0.9.1", true},
		{"newer minor", "0.9.0", "0.10.0", true},
		{"newer major", "0.9.0", "1.0.0", true},
		{"equal", "0.9.0", "0.9.0", false},
		{"older latest", "0.10.0", "0.9.0", false},
		{"v-prefixed running", "v0.9.0", "0.10.0", true},
		{"v-prefixed latest", "0.9.0", "v0.10.0", true},
		{"both v-prefixed equal", "v0.9.0", "v0.9.0", false},
		{"dev running", "dev", "0.10.0", false},
		{"garbage running", "not-a-version", "0.10.0", false},
		{"empty running", "", "0.10.0", false},
		{"garbage latest", "0.9.0", "next", false},
		{"empty latest", "0.9.0", "", false},
		{"prerelease latest older than stable", "1.0.0", "1.1.0-rc1", true},
		{"stable latest newer than prerelease running", "1.1.0-rc1", "1.1.0", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := UpdateAvailable(tc.running, tc.latest); got != tc.want {
				t.Errorf("UpdateAvailable(%q, %q) = %v, want %v", tc.running, tc.latest, got, tc.want)
			}
		})
	}
}

func TestIsValidVersion(t *testing.T) {
	cases := []struct {
		name string
		v    string
		want bool
	}{
		{"plain semver", "0.9.0", true},
		{"v-prefixed", "v0.9.0", true},
		{"prerelease", "1.2.3-rc1", true},
		{"dev build", "dev", false},
		{"empty", "", false},
		{"garbage", "not-a-version", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsValidVersion(tc.v); got != tc.want {
				t.Errorf("IsValidVersion(%q) = %v, want %v", tc.v, got, tc.want)
			}
		})
	}
}

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.2.0", "0.1.0", 1},
		{"0.1.0", "0.2.0", -1},
		{"1.0.0", "1.0.0", 0},
		{"v0.2.0", "0.1.0", 1}, // leading v tolerated on either side
		{"0.90.0", "0.17.1", 1},
		{"0.1.0", "", 1}, // valid sorts after invalid/empty
		{"", "0.1.0", -1},
		{"dev", "also-bad", 0}, // two invalid compare equal
	}
	for _, tc := range cases {
		if got := Compare(tc.a, tc.b); got != tc.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}
