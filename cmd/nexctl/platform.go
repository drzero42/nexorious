package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/drzero42/nexorious/internal/cliclient"
)

// validatePlatform checks that slug is a seeded platform name, returning an
// actionable error that lists the valid slugs when it is not. An empty slug is
// allowed (callers that require a platform enforce that separately) — the
// platform column is a NOT-NULL FK to platforms(name), so an invalid value
// would otherwise surface as an opaque 500. Shared by the CLI commands and the
// MCP tools so the two front-ends validate identically.
func validatePlatform(c *cliclient.Client, key, slug string) error {
	if slug == "" {
		return nil
	}
	plats, err := c.ListPlatforms(key)
	if err != nil {
		return fmt.Errorf("list platforms failed: %w", err)
	}
	names := make([]string, 0, len(plats))
	for _, p := range plats {
		if p.Name == slug {
			return nil
		}
		names = append(names, p.Name)
	}
	sort.Strings(names)
	return fmt.Errorf("unknown platform %q; valid platforms: %s", slug, strings.Join(names, ", "))
}
