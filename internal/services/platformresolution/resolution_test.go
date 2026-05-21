package platformresolution_test

import (
	"testing"

	"github.com/drzero42/nexorious/internal/services/platformresolution"
)

func TestStorefrontToCollectionSlug_Epic(t *testing.T) {
	slug, ok := platformresolution.StorefrontToCollectionSlug("epic")
	if !ok {
		t.Fatal("expected epic storefront to be resolved, got false")
	}
	if slug != "epic-games-store" {
		t.Errorf("got %q, want %q", slug, "epic-games-store")
	}
}

func TestRawPlatformToSlug_PCLinux(t *testing.T) {
	slug, ok := platformresolution.RawPlatformToSlug("pc-linux")
	if !ok {
		t.Fatal("expected pc-linux to resolve")
	}
	if slug != "pc-linux" {
		t.Errorf("got %q", slug)
	}
}

func TestStorefrontToCollectionSlug_GOG(t *testing.T) {
	slug, ok := platformresolution.StorefrontToCollectionSlug("gog")
	if !ok {
		t.Fatal("expected gog to resolve")
	}
	if slug != "gog" {
		t.Errorf("got %q", slug)
	}
}

func TestRawPlatformToSlug_PCMac(t *testing.T) {
	slug, ok := platformresolution.RawPlatformToSlug("pc-mac")
	if !ok {
		t.Fatal("expected ok=true for pc-mac")
	}
	if slug != "mac" {
		t.Errorf("expected slug=mac, got %s", slug)
	}
}
