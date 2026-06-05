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

func TestStorefrontToCollectionSlug_GOG(t *testing.T) {
	slug, ok := platformresolution.StorefrontToCollectionSlug("gog")
	if !ok {
		t.Fatal("expected gog to resolve")
	}
	if slug != "gog" {
		t.Errorf("got %q", slug)
	}
}

func TestStorefrontToCollectionSlug_HumbleBundle(t *testing.T) {
	slug, ok := platformresolution.StorefrontToCollectionSlug("humble-bundle")
	if !ok {
		t.Fatal("expected ok=true for humble-bundle")
	}
	if slug != "humble-bundle" {
		t.Errorf("got %q, want %q", slug, "humble-bundle")
	}
}
