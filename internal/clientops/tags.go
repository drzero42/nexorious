package clientops

import (
	"context"
	"fmt"
	"strings"

	"github.com/drzero42/nexorious/internal/cliclient"
)

// ResolveTagRef resolves a tag by id (UUID) or name (case-insensitive).
func ResolveTagRef(ctx context.Context, c *cliclient.Client, key, ref string) (*cliclient.Tag, error) {
	if LooksLikeUUID(ref) {
		// No single-tag GET endpoint; fetch the list and match by id.
		tags, err := c.ListTags(ctx, key)
		if err != nil {
			return nil, err
		}
		for i := range tags {
			if tags[i].ID == ref {
				return &tags[i], nil
			}
		}
		return nil, fmt.Errorf("no tag with id %s", ref)
	}
	tags, err := c.ListTags(ctx, key)
	if err != nil {
		return nil, err
	}
	for i := range tags {
		if strings.EqualFold(tags[i].Name, ref) {
			return &tags[i], nil
		}
	}
	return nil, fmt.Errorf("no tag named %q", ref)
}

// ResolveTagID maps a tag name to its id (case-insensitive), erroring if unknown.
func ResolveTagID(ctx context.Context, c *cliclient.Client, key, name string) (string, error) {
	tags, err := c.ListTags(ctx, key)
	if err != nil {
		return "", fmt.Errorf("resolve tag %q: %w", name, err)
	}
	for _, t := range tags {
		if strings.EqualFold(t.Name, name) {
			return t.ID, nil
		}
	}
	return "", fmt.Errorf("no tag named %q", name)
}
