package importmodel

import (
	"encoding/json"
	"strings"
	"testing"
)

// A Game with IsWishlisted=false must serialize WITHOUT the is_wishlisted key,
// so every existing mapper's source_metadata stays byte-identical (omitempty).
func TestGame_IsWishlistedOmittedWhenFalse(t *testing.T) {
	b, err := json.Marshal(Game{Title: "X", PlayStatus: "not_started"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "is_wishlisted") {
		t.Errorf("is_wishlisted must be omitted when false, got %s", b)
	}
}

func TestGame_IsWishlistedPresentWhenTrue(t *testing.T) {
	b, err := json.Marshal(Game{Title: "X", PlayStatus: "not_started", IsWishlisted: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"is_wishlisted":true`) {
		t.Errorf("is_wishlisted:true must serialize, got %s", b)
	}
}
