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
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := UpdateAvailable(tc.running, tc.latest); got != tc.want {
				t.Errorf("UpdateAvailable(%q, %q) = %v, want %v", tc.running, tc.latest, got, tc.want)
			}
		})
	}
}
