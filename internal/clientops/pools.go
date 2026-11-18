package clientops

import (
	"context"
	"fmt"
	"strings"

	"github.com/drzero42/nexorious/internal/cliclient"
)

// FindPoolsByRef returns every pool matching ref: a UUID is matched exactly; a
// name is matched case-insensitively (zero or more results). No cmd, no picker —
// callers own disambiguation.
func FindPoolsByRef(ctx context.Context, c *cliclient.Client, key, ref string) ([]cliclient.PoolListItem, error) {
	pools, err := c.ListPools(ctx, key)
	if err != nil {
		return nil, err
	}
	if LooksLikeUUID(ref) {
		for i := range pools {
			if pools[i].ID == ref {
				return []cliclient.PoolListItem{pools[i]}, nil
			}
		}
		return nil, fmt.Errorf("no pool with id %s", ref)
	}
	var matches []cliclient.PoolListItem
	for i := range pools {
		if strings.EqualFold(pools[i].Name, ref) {
			matches = append(matches, pools[i])
		}
	}
	return matches, nil
}
