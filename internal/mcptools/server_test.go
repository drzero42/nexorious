package mcptools

import (
	"errors"
	"fmt"
	"testing"

	"github.com/drzero42/nexorious/internal/cliclient"
)

func TestToolError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, ""},
		{"403", &cliclient.HTTPError{Status: 403, Message: "forbidden"},
			"game_edit: this API key is read-only; mint a write-scoped key to modify the collection"},
		{"403 wrapped", fmt.Errorf("add platform: %w", &cliclient.HTTPError{Status: 403}),
			"game_edit: this API key is read-only; mint a write-scoped key to modify the collection"},
		{"404 with message", &cliclient.HTTPError{Status: 404, Message: "user game not found"},
			"game_edit: not found: user game not found"},
		{"404 bare", &cliclient.HTTPError{Status: 404}, "game_edit: not found"},
		{"422", &cliclient.HTTPError{Status: 422, Message: "rating must be between 1 and 5"},
			"game_edit: rating must be between 1 and 5"},
		{"422 without message", &cliclient.HTTPError{Status: 422}, "game_edit: server returned 422"},
		// Step context on a wrapped 404/422 is kept, not replaced.
		{"422 wrapped", fmt.Errorf("add platform: %w", &cliclient.HTTPError{Status: 422, Message: "bad storefront"}),
			"game_edit: add platform: server returned 422: bad storefront"},
		{"500", &cliclient.HTTPError{Status: 500, Message: "boom"}, "game_edit: server returned 500: boom"},
		{"non-HTTP", errors.New("dial tcp: refused"), "game_edit: dial tcp: refused"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := toolError("game_edit", tc.err)
			if tc.want == "" {
				if got != nil {
					t.Fatalf("got %v, want nil", got)
				}
				return
			}
			if got == nil || got.Error() != tc.want {
				t.Fatalf("got %v, want %q", got, tc.want)
			}
		})
	}
}

// Errors that are not rewritten keep the HTTPError reachable for callers.
func TestToolErrorKeepsChain(t *testing.T) {
	err := toolError("x", &cliclient.HTTPError{Status: 500})
	var he *cliclient.HTTPError
	if !errors.As(err, &he) || he.Status != 500 {
		t.Fatalf("HTTPError not reachable from %v", err)
	}
}
