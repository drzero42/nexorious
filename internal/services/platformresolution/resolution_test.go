package platformresolution_test

import (
	"testing"

	"github.com/drzero42/nexorious-go/internal/services/platformresolution"
)

func TestRawPlatformToSlug(t *testing.T) {
	cases := []struct {
		raw  string
		want string
		ok   bool
	}{
		{"pc-windows", "pc-windows", true},
		{"playstation-5", "ps5", true},
		{"playstation-4", "ps4", true},
		{"unknown", "", false},
	}
	for _, tc := range cases {
		got, ok := platformresolution.RawPlatformToSlug(tc.raw)
		if ok != tc.ok || got != tc.want {
			t.Errorf("RawPlatformToSlug(%q) = (%q, %v), want (%q, %v)", tc.raw, got, ok, tc.want, tc.ok)
		}
	}
}

func TestStorefrontToCollectionSlug(t *testing.T) {
	cases := []struct {
		sf   string
		want string
		ok   bool
	}{
		{"steam", "steam", true},
		{"psn", "playstation-store", true},
		{"epic", "", false},
		{"gog", "", false},
	}
	for _, tc := range cases {
		got, ok := platformresolution.StorefrontToCollectionSlug(tc.sf)
		if ok != tc.ok || got != tc.want {
			t.Errorf("StorefrontToCollectionSlug(%q) = (%q, %v), want (%q, %v)", tc.sf, got, ok, tc.want, tc.ok)
		}
	}
}
