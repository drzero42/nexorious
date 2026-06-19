package cliclient

import (
	"net/http"
	"net/url"
)

// ListExternalGames returns all external games for the given storefront.
func (c *Client) ListExternalGames(key, storefront string) ([]ExternalGame, error) {
	var out []ExternalGame
	if err := c.doBearer(http.MethodGet, "/api/sync/"+url.PathEscape(storefront)+"/external-games", key, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// RematchExternalGame re-matches an external game to the given IGDB id.
// When orphanAction is non-empty it is included in the request body.
func (c *Client) RematchExternalGame(key, id string, igdbID int, orphanAction string) error {
	body := map[string]any{"igdb_id": igdbID}
	if orphanAction != "" {
		body["orphan_action"] = orphanAction
	}
	return c.doBearer(http.MethodPost, "/api/sync/external-games/"+url.PathEscape(id)+"/rematch", key, body, nil)
}

// RetryFailedExternalGames re-enqueues all failed external game items for a storefront.
func (c *Client) RetryFailedExternalGames(key, storefront string) error {
	return c.doBearer(http.MethodPost, "/api/sync/"+url.PathEscape(storefront)+"/external-games/retry-failed", key, nil, nil)
}

// SkipExternalGame marks an external game as skipped.
func (c *Client) SkipExternalGame(key, id string) error {
	return c.doBearer(http.MethodPost, "/api/sync/ignored/"+url.PathEscape(id), key, nil, nil)
}
