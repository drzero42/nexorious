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

func TestPlatformToSlug_PCLinux(t *testing.T) {
	slug, ok := platformresolution.PlatformToSlug("pc-linux")
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

func TestPlatformToSlug_PSN4(t *testing.T) {
	slug, ok := platformresolution.PlatformToSlug("playstation-4")
	if !ok {
		t.Fatal("expected playstation-4 to resolve")
	}
	if slug != "playstation-4" {
		t.Errorf("got %q, want %q", slug, "playstation-4")
	}
}

func TestPlatformToSlug_PSN5(t *testing.T) {
	slug, ok := platformresolution.PlatformToSlug("playstation-5")
	if !ok {
		t.Fatal("expected playstation-5 to resolve")
	}
	if slug != "playstation-5" {
		t.Errorf("got %q, want %q", slug, "playstation-5")
	}
}
