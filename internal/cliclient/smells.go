package cliclient

import (
	"net/http"
	"net/url"
	"strconv"
)

// SmellSummaryItem is one row of GET /api/library/smells (per-check counts).
type SmellSummaryItem struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Tier        string `json:"tier"`
	AutoFixable bool   `json:"auto_fixable"`
	Count       int    `json:"count"`
}

// FlaggedItem is one flagged game returned by GET /api/library/smells/:checkID.
type FlaggedItem struct {
	UserGameID      string  `json:"user_game_id"`
	GameID          int32   `json:"game_id"`
	Title           string  `json:"title"`
	CoverArtURL     *string `json:"cover_art_url,omitempty"`
	SuggestedStatus *string `json:"suggested_status,omitempty"`
	Detail          *string `json:"detail,omitempty"`
}

// FlaggedListResponse is the paginated flagged-items response.
type FlaggedListResponse struct {
	Items   []FlaggedItem `json:"items"`
	Total   int           `json:"total"`
	Page    int           `json:"page"`
	PerPage int           `json:"per_page"`
	Pages   int           `json:"pages"`
}

// IgnoredItem is one dismissed game from GET /api/library/smells/:checkID/ignored.
type IgnoredItem struct {
	UserGameID string `json:"user_game_id"`
	Title      string `json:"title"`
	CreatedAt  string `json:"created_at"`
}

// IgnoredListResponse is the paginated dismissed-items response.
type IgnoredListResponse struct {
	Items   []IgnoredItem `json:"items"`
	Total   int           `json:"total"`
	Page    int           `json:"page"`
	PerPage int           `json:"per_page"`
	Pages   int           `json:"pages"`
}

// SmellApplyResult is the POST .../apply response.
type SmellApplyResult struct {
	Applied int `json:"applied"`
	Skipped int `json:"skipped"`
}

func smellPagePath(base string, page, perPage int) string {
	q := url.Values{}
	if page > 0 {
		q.Set("page", strconv.Itoa(page))
	}
	if perPage > 0 {
		q.Set("per_page", strconv.Itoa(perPage))
	}
	if enc := q.Encode(); enc != "" {
		return base + "?" + enc
	}
	return base
}

// ListSmells returns the per-check summary (counts post-ignore).
func (c *Client) ListSmells(key string) ([]SmellSummaryItem, error) {
	var out []SmellSummaryItem
	if err := c.doBearer(http.MethodGet, "/api/library/smells", key, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListSmellItems returns one page of flagged items for a check.
func (c *Client) ListSmellItems(key, checkID string, page, perPage int) (*FlaggedListResponse, error) {
	var out FlaggedListResponse
	path := smellPagePath("/api/library/smells/"+url.PathEscape(checkID), page, perPage)
	if err := c.doBearer(http.MethodGet, path, key, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ApplySmell applies the auto-fix for a check to the given user-game ids.
func (c *Client) ApplySmell(key, checkID string, userGameIDs []string) (*SmellApplyResult, error) {
	var out SmellApplyResult
	body := map[string][]string{"user_game_ids": userGameIDs}
	path := "/api/library/smells/" + url.PathEscape(checkID) + "/apply"
	if err := c.doBearer(http.MethodPost, path, key, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// IgnoreSmell dismisses the given user-games for a check; returns the count newly ignored.
func (c *Client) IgnoreSmell(key, checkID string, userGameIDs []string) (int, error) {
	var out struct {
		Ignored int `json:"ignored"`
	}
	body := map[string][]string{"user_game_ids": userGameIDs}
	path := "/api/library/smells/" + url.PathEscape(checkID) + "/ignore"
	if err := c.doBearer(http.MethodPost, path, key, body, &out); err != nil {
		return 0, err
	}
	return out.Ignored, nil
}

// RestoreSmell un-dismisses the given user-games for a check; returns the count restored.
func (c *Client) RestoreSmell(key, checkID string, userGameIDs []string) (int, error) {
	var out struct {
		Restored int `json:"restored"`
	}
	body := map[string][]string{"user_game_ids": userGameIDs}
	path := "/api/library/smells/" + url.PathEscape(checkID) + "/ignore"
	if err := c.doBearer(http.MethodDelete, path, key, body, &out); err != nil {
		return 0, err
	}
	return out.Restored, nil
}

// ListIgnoredSmells returns one page of dismissed items for a check.
func (c *Client) ListIgnoredSmells(key, checkID string, page, perPage int) (*IgnoredListResponse, error) {
	var out IgnoredListResponse
	path := smellPagePath("/api/library/smells/"+url.PathEscape(checkID)+"/ignored", page, perPage)
	if err := c.doBearer(http.MethodGet, path, key, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
