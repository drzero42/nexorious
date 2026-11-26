package cliclient

import (
	"context"
	"net/http"
	"net/url"
)

// ListExternalGames returns all external games for the given storefront.
func (c *Client) ListExternalGames(ctx context.Context, key, storefront string) ([]ExternalGame, error) {
	var out []ExternalGame
	if err := c.doBearer(ctx, http.MethodGet, "/api/sync/"+url.PathEscape(storefront)+"/external-games", key, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// onlyThisEditionQuery is the query string of a decision on the edition id
// alone (#1452), else empty: the decision acts on its whole set.
func onlyThisEditionQuery(onlyThisEdition bool) string {
	if onlyThisEdition {
		return "?only_this_edition=true"
	}
	return ""
}

// RematchExternalGame re-matches an external game and its editions (with
// onlyThisEdition, the game id alone) to the given IGDB id. When orphanAction
// is non-empty it is included in the request body.
func (c *Client) RematchExternalGame(ctx context.Context, key, id string, igdbID int, orphanAction string, onlyThisEdition bool) error {
	body := map[string]any{"igdb_id": igdbID}
	if orphanAction != "" {
		body["orphan_action"] = orphanAction
	}
	return c.doBearer(ctx, http.MethodPost, "/api/sync/external-games/"+url.PathEscape(id)+"/rematch"+onlyThisEditionQuery(onlyThisEdition), key, body, nil)
}

// RetryFailedExternalGames re-enqueues all failed external game items for a storefront.
func (c *Client) RetryFailedExternalGames(ctx context.Context, key, storefront string) error {
	return c.doBearer(ctx, http.MethodPost, "/api/sync/"+url.PathEscape(storefront)+"/external-games/retry-failed", key, nil, nil)
}

// SkipExternalGame skips an external game and its editions (with
// onlyThisEdition, the game id alone).
func (c *Client) SkipExternalGame(ctx context.Context, key, id string, onlyThisEdition bool) error {
	return c.doBearer(ctx, http.MethodPost, "/api/sync/ignored/"+url.PathEscape(id)+onlyThisEditionQuery(onlyThisEdition), key, nil, nil)
}
