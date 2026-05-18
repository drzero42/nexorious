package platformresolution_test

import (
	"testing"

	"github.com/drzero42/nexorious-go/internal/services/platformresolution"
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
