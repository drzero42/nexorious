package clientops

import (
	"context"

	"github.com/drzero42/nexorious/internal/cliclient"
)

// CollectFlaggedIDs returns every flagged user_game_id for a check, across pages.
func CollectFlaggedIDs(ctx context.Context, c *cliclient.Client, key, checkID string) ([]string, error) {
	var ids []string
	for page := 1; ; page++ {
		res, err := c.ListSmellItems(ctx, key, checkID, page, 200)
		if err != nil {
			return nil, err
		}
		for i := range res.Items {
			ids = append(ids, res.Items[i].UserGameID)
		}
		if res.Pages <= page {
			break
		}
	}
	return ids, nil
}
